package provider

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// Binding ties a gc session name to its Rex address.
type Binding struct {
	Name         SessionName
	SessionID    rex.SessionID
	SessionLabel string
	WindowID     rex.WindowID
	BlockID      rex.BlockID
	// TabLabel is the Rex window (tab) label the pane opened under. Empty on
	// legacy records, which fall back to Name.TabLabel().
	TabLabel string
	// PaneLabel is the Rex block (pane) label the pane opened under: the
	// env-derived "<agent> (<GC_SESSION_NAME>)" for rig-scoped starts, else
	// the name-derived label. Empty on legacy records, which fall back to
	// Name.PaneLabel().
	PaneLabel string
	// Aliases are the extra gc session names this pane answers to. gc 1.5.1
	// stops putting the rig in pool/wisp session names and instead probes the
	// rig-qualified identity, which is recorded here.
	Aliases []SessionName
	// LaunchMode is how the pane was started (for example "fresh" or
	// "resume"); empty when unknown.
	LaunchMode string
	// CreatedAt is when the pane was started; zero when unknown.
	CreatedAt time.Time
	// GCBin is the basename of the GC_BIN binary the agent was launched with,
	// when the start config carried one. gc's configured process_names hint for
	// the control dispatcher is "gc", but a wrapped gc runs as e.g.
	// ".gc-wrapped"; process-alive expands the "gc" hint with this basename so
	// gc's reconciler does not mistake a live dispatcher for a dead seat. Empty
	// on legacy records (fall back to the provider's own GC_BIN).
	GCBin string
	// WorkDir is the directory the agent was launched in. Stop uses it to
	// leave a diagnostic when a drained worker held uncommitted edits. Empty
	// on legacy records.
	WorkDir string
	// ClaimSessionID is the gc session bead id (the start config's
	// GC_SESSION_ID) the agent was launched under. The provider reads the
	// session's live claim back through `gc hook current --id-only` with this
	// id before it delivers gc's fallback pool-claim nudge, so a seat that
	// already holds a claim is not re-prompted mid-task (GCREX-33). Empty on
	// legacy records, where the guard cannot resolve the claim and delivers.
	ClaimSessionID string
}

// tabLabel returns the Rex tab label for the pane: the stored label when the
// record has one, otherwise the label the name derives with town as the town
// label (a legacy record has no start env, so the rig comes from the name
// split).
func (b Binding) tabLabel(town string) string {
	if b.TabLabel != "" {
		return b.TabLabel
	}
	return tabLabel(b.Name, nil, town)
}

// acceptsTabLabel reports whether label is the binding's tab label: its stored
// or derived label, or — for a record that has no stored label — the label gc
// wrote before GCREX-72, which a window started by an older build still has.
func (b Binding) acceptsTabLabel(label, town string) bool {
	if label == b.tabLabel(town) {
		return true
	}
	return b.TabLabel == "" && label == legacyTabLabel(b.Name)
}

// paneLabel returns the Rex pane label for the pane: the stored env-derived
// label when present, otherwise the name-only label for legacy records, which
// carry no start-config env to derive from.
func (b Binding) paneLabel() string {
	if b.PaneLabel != "" {
		return b.PaneLabel
	}
	return b.Name.PaneLabel()
}

// sanitizeQualified converts a gc qualified name (rig/agent, with dots) to its
// session-name form, matching gc's SanitizeQualifiedNameForSession: "/"
// becomes "--" and "." becomes "__".
func sanitizeQualified(name string) string {
	name = strings.ReplaceAll(name, "/", nameSep)
	return strings.ReplaceAll(name, ".", "__")
}

// aliasesFor lists the extra gc session names a started pane answers to,
// derived from the start-config env. Names are sanitized, deduped, and any
// equal to the start name (the primary) are dropped.
func aliasesFor(name SessionName, env map[string]string) []SessionName {
	seen := map[SessionName]bool{name: true}
	var out []SessionName
	add := func(v string) {
		if v == "" {
			return
		}
		sn := SessionName(v)
		if seen[sn] {
			return
		}
		seen[sn] = true
		out = append(out, sn)
	}
	if t := env["GC_TEMPLATE"]; t != "" {
		add(sanitizeQualified(t))
	}
	if a := env["GC_ALIAS"]; a != "" {
		add(sanitizeQualified(a))
	}
	if g := env["GC_AGENT"]; g != "" && strings.ContainsAny(g, "/.") {
		add(sanitizeQualified(g))
	}
	if rig := env["GC_RIG"]; rig != "" && name.IsTown() {
		add(sanitizeQualified(rig + "/" + string(name)))
	}
	return out
}

// Binder resolves gc session names to Rex addresses, using a store as a
// cache and the Rex session view as the source of truth.
type Binder struct {
	client rex.Client
	store  Store[SessionName, Binding]
	// sessionLabels are the accepted labels, canonical first. A new session
	// and a fresh record use the first; a legacy record under another
	// candidate label still revalidates.
	sessionLabels []string
	// townTab is the town-level tab label, used to derive the tab label for
	// a legacy record or a discovery scan that has no stored start env.
	townTab string
}

// NewBinder builds a Binder that looks for the Rex session labelled
// sessionLabel and caches bindings in store.
func NewBinder(client rex.Client, store Store[SessionName, Binding], sessionLabel, town string) *Binder {
	return NewBinderWithLabels(client, store, []string{sessionLabel}, town)
}

// NewBinderWithLabels builds a Binder that accepts any of labels (canonical
// first, duplicates and empty entries dropped) and caches bindings in store.
func NewBinderWithLabels(client rex.Client, store Store[SessionName, Binding], labels []string, town string) *Binder {
	return &Binder{client: client, store: store, sessionLabels: normalizeSessionLabels(labels), townTab: town}
}

// normalizeSessionLabels trims, dedupes and drops empty labels, preserving
// order. It never returns an empty slice for a non-empty input.
func normalizeSessionLabels(labels []string) []string {
	out := make([]string, 0, len(labels))
	seen := map[string]bool{}
	for _, label := range labels {
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	return out
}

// canonicalLabel is the label new sessions and records use.
func (b *Binder) canonicalLabel() string {
	if len(b.sessionLabels) == 0 {
		return ""
	}
	return b.sessionLabels[0]
}

// acceptsLabel reports whether label belongs to this city's namespace set.
func (b *Binder) acceptsLabel(label string) bool {
	return slices.Contains(b.sessionLabels, label)
}

// FindSession returns the city's Rex session. The bool is false when it does
// not exist.
func (b *Binder) FindSession(ctx context.Context) (rex.SessionID, bool, error) {
	sessions, err := b.client.ListSessions(ctx)
	if err != nil {
		return "", false, err
	}
	// Prefer the canonical label, then every other accepted namespace, so a
	// legacy session remains reachable without letting the candidate order
	// depend on the server's listing order.
	for _, label := range b.sessionLabels {
		for _, s := range sessions {
			if s.Label == label {
				return s.ID, true, nil
			}
		}
	}
	return "", false, nil
}

// lookup returns the stored bindings that answer to name: the primary binding
// stored under name first, then any primary binding that lists name as an
// alias. The result is empty when nothing is known. It does not contact Rex.
func (b *Binder) lookup(name SessionName) []Binding {
	var out []Binding
	seen := map[SessionName]bool{}
	if bd, ok, _ := b.store.Read(name); ok {
		out = append(out, bd)
		seen[bd.Name] = true
	}
	lister, ok := b.store.(Lister[Binding])
	if !ok {
		return out
	}
	for _, bd := range lister.List() {
		if seen[bd.Name] {
			continue
		}
		if slices.Contains(bd.Aliases, name) {
			out = append(out, bd)
			seen[bd.Name] = true
		}
	}
	return out
}

// ResolveMode selects how much a resolution does.
type ResolveMode int

const (
	// ResolveCached returns the stored binding only; it never contacts Rex and
	// does not check that the pane still exists.
	ResolveCached ResolveMode = iota
	// ResolveLive checks stored candidates against the live pane, rediscovering
	// a recreated pane under the labels its record had. With nothing cached it
	// scans by label first.
	ResolveLive
	// ResolveFresh revalidates and drops stale records, then scans by the
	// requested name (or the recorded labels when they are known).
	ResolveFresh
)

// ResolveRequest is one resolution request.
type ResolveRequest struct {
	Name SessionName
	// Hint carries the env-derived tab and pane labels a lost sidecar record
	// used, so the fresh scan can rediscover a live pane started under a short
	// pool name with no rig.
	Hint Binding
	// LegacyTabs are the pre-GCREX-72 tab labels the caller's start env
	// implies (a non-empty GC_RIG, or the name-derived rig). They are tried
	// after the hint and name-derived labels so an upgrade finds the pane
	// instead of opening a second tab.
	LegacyTabs []string
	// Mode selects the resolution behavior. The zero value is ResolveCached.
	Mode ResolveMode
}

// Resolve is the one resolution entry point. It replaces the previous
// Cached/Live/ResolveHint surfaces: ResolveCached, ResolveLive and
// ResolveFresh reproduce their behavior.
func (b *Binder) Resolve(ctx context.Context, req ResolveRequest) (Binding, bool, error) {
	switch req.Mode {
	case ResolveLive:
		return b.resolveLive(ctx, req.Name)
	case ResolveFresh:
		return b.resolveFresh(ctx, req.Name, req.Hint, req.LegacyTabs)
	default:
		cands := b.lookup(req.Name)
		if len(cands) == 0 {
			return Binding{}, false, nil
		}
		return cands[0], true, nil
	}
}

// resolveLive returns the first stored binding that answers to name whose pane
// is alive. Each candidate is checked against the session view; a stale
// address is re-scanned under the labels the record had (or the requested name
// when nothing is cached), and a record whose pane cannot be found is deleted.
func (b *Binder) resolveLive(ctx context.Context, name SessionName) (Binding, bool, error) {
	cands := b.lookup(name)
	if len(cands) == 0 {
		// Nothing cached: fall back to the label scan a fresh resolve performs.
		bd, ok, err := b.resolveFresh(ctx, name, Binding{}, nil)
		if err != nil || !ok {
			return Binding{}, false, err
		}
		return b.liveOnly(ctx, bd)
	}
	for _, bd := range cands {
		if !matchesName(bd, name) {
			continue
		}
		got, ok, err := b.revalidate(ctx, bd)
		if err != nil {
			return Binding{}, false, err
		}
		if !ok {
			// The stored address is stale; rediscover the pane under the
			// labels it had, so a recreated instance still counts as alive.
			if got, ok, err = b.scan(ctx, bd.Name, bd, nil); err != nil {
				return Binding{}, false, err
			}
		}
		if !ok {
			_ = b.store.Drop(bd.Name)
			continue
		}
		if got, ok, err = b.liveOnly(ctx, got); err != nil || ok {
			return got, ok, err
		}
	}
	return Binding{}, false, nil
}

// liveOnly reports bd with true when its pane still has a live terminal. A pane
// Rex no longer knows has exited and closed its block, so it is not alive.
func (b *Binder) liveOnly(ctx context.Context, bd Binding) (Binding, bool, error) {
	info, err := b.client.BlockProcess(ctx, bd.SessionID, bd.BlockID)
	if errors.Is(err, rex.ErrNotFound) {
		return Binding{}, false, nil
	}
	if err != nil {
		return Binding{}, false, err
	}
	if info.LastExit != nil {
		return Binding{}, false, nil
	}
	return bd, true, nil
}

// matchesName reports whether bd legitimately answers to name: it is stored
// under name, or name is one of its aliases.
func matchesName(bd Binding, name SessionName) bool {
	return bd.Name == name || slices.Contains(bd.Aliases, name)
}

// revalidate checks a stored binding against the live Rex session view. A
// binding whose session, tab or block is gone reports ok=false so the caller
// can drop it; any other view error is returned as is and leaves the binding
// alone.
func (b *Binder) revalidate(ctx context.Context, bd Binding) (Binding, bool, error) {
	if bd.Name == "" || !b.acceptsLabel(bd.SessionLabel) {
		// Written for no name or another city: do not trust it.
		return Binding{}, false, nil
	}
	view, err := b.client.SessionView(ctx, bd.SessionID)
	switch {
	case errors.Is(err, rex.ErrNotFound):
		// The session is gone, for example after a restore under a new
		// session id. Re-resolve by label.
		return Binding{}, false, nil
	case err != nil:
		return Binding{}, false, err
	case stillBound(view, bd, b.townTab):
		return bd, true, nil
	default:
		return Binding{}, false, nil
	}
}

// resolveFresh returns the binding for name, resolving aliases to their
// primary binding. A stored binding is checked against the requested name, the
// configured session label and the session view; if any check fails, or Rex
// reports the session as not found, it is deleted and the session is scanned by
// its stored (or name-derived) labels. Any other view error is returned as is
// and leaves the stored binding alone. A name with no pane is reported as false
// with a nil error.
func (b *Binder) resolveFresh(ctx context.Context, name SessionName, envHint Binding, legacy []string) (Binding, bool, error) {
	var stale []Binding
	var hint Binding
	for _, bd := range b.lookup(name) {
		if !matchesName(bd, name) {
			// The record stored under this key answers to a different name:
			// drop the key, never the record's own name.
			_ = b.store.Drop(name)
			continue
		}
		got, ok, err := b.revalidate(ctx, bd)
		if err != nil {
			return Binding{}, false, err
		}
		if ok {
			return got, true, nil
		}
		stale = append(stale, bd)
		if hint.Name == "" {
			hint = bd
		}
	}
	// Re-scan under the labels the caller knows (a lost record) or the stale
	// record used, so a pane recreated with the same env-derived labels is
	// rediscovered; otherwise scan by the requested name.
	scanName, scanHint := name, envHint
	if scanHint.TabLabel == "" && scanHint.PaneLabel == "" {
		if hint.Name != "" {
			scanName, scanHint = hint.Name, hint
		}
	}
	got, ok, err := b.scan(ctx, scanName, scanHint, legacy)
	if err != nil {
		// Rex could not answer the scan: keep the stale records so a retry
		// still sees that a binding existed.
		return Binding{}, false, err
	}
	for _, bd := range stale {
		if !ok || bd.Name != got.Name { // scan already rewrote got.Name
			_ = b.store.Drop(bd.Name)
		}
	}
	return got, ok, nil
}

// stillBound reports whether view still holds the binding's block under its
// stored tab and pane labels (falling back to the name-derived labels for
// legacy records).
func stillBound(view rex.SessionView, bd Binding, town string) bool {
	if view.Label != bd.SessionLabel {
		return false
	}
	for _, w := range view.Windows {
		if w.ID != bd.WindowID || !bd.acceptsTabLabel(w.Label, town) {
			continue
		}
		for _, blk := range w.Blocks {
			if blk.ID == bd.BlockID && blk.Label == bd.paneLabel() {
				return true
			}
		}
	}
	return false
}

// scan finds name by session label, tab label and pane label, and saves it.
// hint supplies the labels an earlier record used, so a pane recreated under
// env-derived labels is found again; empty fields fall back to the labels the
// name implies.
func (b *Binder) scan(ctx context.Context, name SessionName, hint Binding, legacy []string) (Binding, bool, error) {
	sid, ok, err := b.FindSession(ctx)
	if err != nil || !ok {
		return Binding{}, false, err
	}
	view, err := b.client.SessionView(ctx, sid)
	if err != nil {
		return Binding{}, false, err
	}
	primary := hint.TabLabel
	if primary == "" {
		primary = tabLabel(name, nil, b.townTab)
	}
	// A window started by a build before GCREX-72 still carries the old
	// label; find it rather than start a second tab. Start always supplies
	// a hint label, so this cannot be limited to the empty-hint path.
	tabs := []string{primary}
	addTab := func(label string) {
		for _, seen := range tabs {
			if seen == label {
				return
			}
		}
		tabs = append(tabs, label)
	}
	addTab(legacyTabLabelFor(name, nil))
	for _, label := range legacy {
		addTab(label)
	}
	pane := hint.PaneLabel
	if pane == "" {
		pane = name.PaneLabel()
	}
	// IsRunning and Stop resolve without a start env, so the fallback scan
	// must recognise the session identity alone: a pane labelled
	// "<agent> (<GC_SESSION_NAME>)" — or, when that suffix does not fit, the
	// bare session name — still belongs to name even though the agent prefix
	// is unknown here.
	identity := " (" + string(name) + ")"
	identityMatch := func(label string) bool {
		return label == string(name) || strings.HasSuffix(label, identity)
	}
	paneMatches := func(label string) bool {
		return label == pane || identityMatch(label)
	}
	accept := func(tab string, w rex.Window, blk rex.Block) (Binding, bool, error) {
		bd := Binding{
			Name:         name,
			SessionID:    sid,
			SessionLabel: view.Label,
			WindowID:     w.ID,
			BlockID:      blk.ID,
			TabLabel:     tab,
			PaneLabel:    blk.Label,
			Aliases:      hint.Aliases,
			// Launch details the labels do not carry: dropping them
			// would silently disable the Stop-time drain diagnostic.
			LaunchMode: hint.LaunchMode,
			CreatedAt:  hint.CreatedAt,
			GCBin:      hint.GCBin,
			WorkDir:    hint.WorkDir,
		}
		_ = b.store.Write(bd.Name, bd)
		return bd, true, nil
	}
	for _, tab := range tabs {
		for _, w := range view.Windows {
			if w.Label != tab {
				continue
			}
			for _, blk := range w.Blocks {
				if paneMatches(blk.Label) {
					return accept(tab, w, blk)
				}
			}
		}
	}
	// A recordless resolve has no start env and so no tab to scope the search:
	// a no-"--" session started with GC_RIG lives in a "Rig: <rig>" tab the
	// name cannot derive. The session name is unique per city, so an identity
	// match in any tab is that session's pane. The stored-record/hint path
	// stays tab-scoped above.
	if hint.TabLabel == "" {
		for _, w := range view.Windows {
			for _, blk := range w.Blocks {
				if identityMatch(blk.Label) {
					return accept(w.Label, w, blk)
				}
			}
		}
	}
	return Binding{}, false, nil
}

// Record stores binding, replacing any earlier one for the same name.
func (b *Binder) Record(binding Binding) { _ = b.store.Write(binding.Name, binding) }

// Forget drops the stored binding that answers to name, clearing the whole
// primary record including its aliases. An alias drops the primary it names.
func (b *Binder) Forget(name SessionName) {
	if bd, ok, _ := b.store.Read(name); ok {
		_ = b.store.Drop(bd.Name)
		_ = b.store.Drop(name)
		return
	}
	if cands := b.lookup(name); len(cands) > 0 {
		_ = b.store.Drop(cands[0].Name)
	}
}
