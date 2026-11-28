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
	// Adopted marks a record whose pane this provider did not start: a
	// restored or operator-made pane that answered to the name. Its sidecar
	// identity belongs to a retired incarnation, so Observe answers {} for it.
	Adopted bool
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
	// Survivors are the descendants a stop could not reap. The closed pane
	// no longer reports them, so the retry that herdr-core's stop allows
	// reaps these instead of clearing the record blind. Empty when none.
	Survivors []Survivor
	// Attempt identifies the start call that wrote a provisional record. The
	// provisional record (no BlockID) is writable only by that same start, so
	// a stale survivor write cannot replace another launch's pending record.
	Attempt string
}

// Survivor is one descendant a stop could not reap.
type Survivor struct {
	PID   int    `json:"pid"`
	Name  string `json:"name,omitempty"`
	Start string `json:"start,omitempty"` // ps lstart, the pid-reuse identity
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
// or derived label, the pre-change town tab label (the bare pin or "Gas
// City") for a record that has no stored label, or — for a record that has no
// stored label — the label gc wrote before GCREX-72, which a window started by
// an older build still has. That includes both the bare pre-GCREX-72 tab and
// the "Rig: <rig>" label builds between GCREX-72 and the bracketed tags
// wrote; without it a legacy binding cannot be confirmed against its own tab.
func (b Binding) acceptsTabLabel(label, town string) bool {
	if label == b.tabLabel(town) {
		return true
	}
	if b.TabLabel != "" {
		return false
	}
	if town != "" && label == town {
		return true
	}
	if tab := legacyRigTabFor(b.Name, nil); tab != "" && label == tab {
		return true
	}
	return label == legacyTabLabel(b.Name)
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

// Binder resolves gc session names to Rex addresses, using a store as a
// cache and the Rex session view as the source of truth.
type Binder struct {
	client rex.Client
	store  Store[SessionName, Binding]
	// sessionLabels are the accepted labels, canonical first. A new session
	// and a fresh record use the first; a legacy record under another
	// candidate label still revalidates.
	sessionLabels []string
	// townTab is the town tab's bare name (the pin, or the fallback). It
	// derives the tagged town label, and it is the pre-change town tab label
	// a window started by an older build still carries.
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
	// StartEnv is the projected start env that a fresh Start owns.
	// Binder derives the tab, pane, alias and legacy labels from it.
	// A lost sidecar record therefore still rediscovers its pane.
	// Read ops and record-driven rescans leave it nil.
	StartEnv map[string]string
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
		return b.resolveFresh(ctx, req.Name, req.StartEnv)
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
		bd, ok, err := b.resolveFresh(ctx, name, nil)
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
			if got, ok, err = b.findLive(ctx, bd.Name, scanPolicy{record: bd, save: true}); err != nil {
				return Binding{}, false, err
			}
		}
		if !ok {
			b.dropStaleIfUnchanged(bd)
			continue
		}
		if got, ok, err = b.liveOnly(ctx, got); err != nil || ok {
			return got, ok, err
		}
	}
	return Binding{}, false, nil
}

// liveOnly reports bd with true when its pane still has a live terminal. A pane
// Rex no longer knows has exited and closed its block, so it is not alive. The
// read and its error policy are readPaneLife's (panelive.go).
func (b *Binder) liveOnly(ctx context.Context, bd Binding) (Binding, bool, error) {
	alive, err := readPaneLife(ctx, b.client, bd.SessionID, bd.BlockID).alive()
	if err != nil || !alive {
		return Binding{}, false, err
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
func (b *Binder) resolveFresh(ctx context.Context, name SessionName, startEnv map[string]string) (Binding, bool, error) {
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
	// Re-scan under the labels the start env supplies, or the stale record's
	// labels, so a pane recreated with the same env-derived labels is
	// rediscovered; otherwise scan by the requested name.
	scanName, record := name, Binding{}
	if startEnv == nil && hint.Name != "" {
		scanName, record = hint.Name, hint
	}
	got, ok, err := b.findLive(ctx, scanName, scanPolicy{record: record, startEnv: startEnv, save: true})
	if err != nil {
		// Rex could not answer the scan: keep the stale records so a retry
		// still sees that a binding existed.
		return Binding{}, false, err
	}
	for _, bd := range stale {
		if !ok || bd.Name != got.Name { // scan already rewrote got.Name
			b.dropStaleIfUnchanged(bd)
		}
	}
	return got, ok, nil
}

// ResolveRead resolves name without writing or dropping any record. The record
// supplies the labels and launch details of a pane this provider may have
// started; an adopted pane is returned with no CreatedAt.
func (b *Binder) ResolveRead(ctx context.Context, name SessionName, record Binding) (Binding, bool, error) {
	return b.findLive(ctx, name, scanPolicy{record: record})
}

// dropStaleIfUnchanged removes the record under bd.Name only when it is still
// the record bd read and it names no survivor. A stop may have recorded a
// survivor, or a start replaced the record, while the resolve ran: that record
// is the only name for a pid whose pane is gone, and it is not ours to drop.
func (b *Binder) dropStaleIfUnchanged(bd Binding) {
	cur, ok, err := b.store.Read(bd.Name)
	if err != nil || !ok || len(cur.Survivors) > 0 || !bd.sameIncarnation(cur) {
		return
	}
	_ = b.store.Drop(bd.Name)
}

// saveKeepingSurvivors writes bd but keeps any survivor another writer
// recorded under the same name, or any survivor the record already held. A
// stop can record a survivor while a scan runs: the scan's pre-stop snapshot
// is not a reason to forget a pid whose pane is gone. The store applies the
// merge under its lock, so the read-modify-write cannot interleave.
func (b *Binder) saveKeepingSurvivors(bd Binding) error {
	if ms, ok := b.store.(interface {
		Mutate(SessionName, func(Binding) Binding) error
	}); ok {
		return ms.Mutate(bd.Name, func(cur Binding) Binding {
			return keepOtherLaunch(cur, bd)
		})
	}
	cur, _, err := b.store.Read(bd.Name)
	if err != nil {
		return err
	}
	bd = keepOtherLaunch(cur, bd)
	return b.store.Write(bd.Name, bd)
}

// keepOtherLaunch refuses to replace a provisional record written by another
// start: that launch's final write is conditional on the attempt it wrote.
// Otherwise it merges the stored survivor set into bd.
func keepOtherLaunch(cur, bd Binding) Binding {
	if cur.BlockID == "" && cur.Attempt != "" && cur.Attempt != bd.Attempt {
		return cur
	}
	bd.Survivors = mergeSurvivors(bd.Survivors, cur.Survivors)
	return bd
}

// mergeSurvivors returns own plus every survivor of other that is not already
// in own, deduplicated by pid and start time.
func mergeSurvivors(own, other []Survivor) []Survivor {
	out := own
	for _, s := range other {
		dup := false
		for _, have := range out {
			if have.PID == s.PID && have.Start == s.Start {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, s)
		}
	}
	return out
}

// stillBound reports whether view still holds the binding's block under its
// stored tab and pane labels (falling back to the name-derived labels for
// legacy records).
func stillBound(view rex.SessionView, bd Binding, town string) bool {
	// A record written before the bracketed tags names the session "gc:X"
	// while the canonical session is now "[city] X" (and the converse for a
	// record written by a newer provider into a session an older one made).
	// Both labels name the one city, so either settles the session.
	if view.Label != bd.SessionLabel && !sameCityLabel(view.Label, bd.SessionLabel) {
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

// scanPolicy is the caller knowledge one live-pane traversal uses.
type scanPolicy struct {
	// record is the stored binding whose labels and launch details an adopted
	// pane inherits. It is zero when nothing is known.
	record Binding
	// startEnv is the projected start env for a fresh Start. When it is set,
	// it wins over the record labels and supplies the legacy labels.
	startEnv map[string]string
	// save records an adopted pane. A read op leaves it false.
	save bool
}

// resolvedLabels is the label candidate set one scan tries for a name.
type resolvedLabels struct {
	tabs        []string
	pane        string
	legacyPanes []string
	aliases     []SessionName
	// recordless is true when no tab attributes the scan, so a suffix label
	// in any tab may still name the session.
	recordless bool
	// session is the identity the panes are matched against: GC_SESSION_NAME
	// when a start env supplies one, else the name. A start name can differ
	// from the session name (GCREX-...), so every label rung verifies against
	// the session's identity, not the start's.
	session SessionName
}

// identity is the session name a label must verify against.
func (l resolvedLabels) identity(name SessionName) SessionName {
	if l.session != "" {
		return l.session
	}
	return name
}

// resolveLabels returns the label candidates one traversal tries for name. A
// start env supplies the labels a Start owns; without it the record's labels
// apply, falling back to the labels the name implies.
func (b *Binder) resolveLabels(name SessionName, pol scanPolicy) resolvedLabels {
	var lbl resolvedLabels
	if pol.startEnv != nil {
		id := newLaunchIdentity(name, pol.startEnv, b.townTab)
		lbl.session = SessionName(strings.TrimSpace(pol.startEnv["GC_SESSION_NAME"]))
		lbl.pane = id.PaneLabel()
		lbl.aliases = id.Aliases()
		lbl.tabs = appendLabel(nil, id.TabLabel())
		// A pane started before GCREX-72 can sit in the name-derived rig tab,
		// which the pre-GC_RIG start env did not record. Keep that candidate
		// before the env-derived legacy tab.
		for _, tab := range id.LegacyTabLabels() {
			lbl.tabs = appendLabel(lbl.tabs, tab)
		}
		// A window started by an older build carries "Rig: <rig>", which no
		// start env recorded.
		if tab := legacyRigTabFor(name, pol.startEnv); tab != "" {
			lbl.tabs = appendLabel(lbl.tabs, tab)
		}
		if legacy := id.LegacyPaneLabel(); legacy != lbl.pane {
			lbl.legacyPanes = []string{legacy}
		}
		return lbl
	}
	lbl.pane = pol.record.PaneLabel
	if lbl.pane == "" {
		lbl.pane = name.PaneLabel()
	}
	lbl.aliases = pol.record.Aliases
	lbl.recordless = pol.record.TabLabel == ""
	primary := pol.record.TabLabel
	if primary == "" {
		primary = tabLabel(name, nil, b.townTab)
	}
	lbl.tabs = appendLabel(nil, primary)
	// A record's TabLabel is the tab its pane lives in, so it is the only
	// candidate a scan may use: a pane with the same label in another tab is a
	// different binding. The legacy rungs below exist only for a record with no
	// stored tab (or no record at all), where the name has to stand in for it.
	if !lbl.recordless {
		return lbl
	}
	// A town window started before the bracketed tags still carries the bare
	// pin (or "Gas City"); without this rung a recordless scan for a town
	// session would stop finding it. Only a town-level name may claim that
	// tab: for a rig name such as "Indigo--mayor" the bare pane label "mayor"
	// would otherwise be attributed through the unrelated town pane of the
	// town's own mayor.
	if b.townTab != "" && name.IsTown() {
		lbl.tabs = appendLabel(lbl.tabs, b.townTab)
	}
	// A window started by a build before GCREX-72 still carries the old
	// label; find it rather than start a second tab.
	lbl.tabs = appendLabel(lbl.tabs, legacyTabLabelFor(name, nil))
	// The bracketed tags are newer still: a window from before them carries
	// "Rig: <rig>".
	if tab := legacyRigTabFor(name, nil); tab != "" {
		lbl.tabs = appendLabel(lbl.tabs, tab)
	}
	return lbl
}

// appendLabel appends label to labels unless it is already present.
func appendLabel(labels []string, label string) []string {
	if slices.Contains(labels, label) {
		return labels
	}
	return append(labels, label)
}

// findLive is the one traversal of accepted sessions, windows and blocks. It
// returns the binding a scan adopts. With pol.save it saves that binding; a
// read-only policy leaves the store untouched and returns no CreatedAt.
func (b *Binder) findLive(ctx context.Context, name SessionName, pol scanPolicy) (Binding, bool, error) {
	// Search every accepted namespace, canonical first: a pane that exists only
	// in a legacy session must still be found, and the canonical session not
	// holding it is not evidence the city does not (CodeRabbit fix).
	sessions, err := b.acceptedSessions(ctx)
	if err != nil {
		return Binding{}, false, err
	}
	lbl := b.resolveLabels(name, pol)
	for _, sess := range sessions {
		view, err := b.client.SessionView(ctx, sess.ID)
		if errors.Is(err, rex.ErrNotFound) {
			continue
		}
		if err != nil {
			return Binding{}, false, err
		}
		if bd, ok := b.findInView(name, lbl, pol, sess.ID, view); ok {
			return bd, true, nil
		}
	}
	return Binding{}, false, nil
}

// acceptedSessions lists the city's sessions in preference order: canonical
// first, then every other accepted namespace.
func (b *Binder) acceptedSessions(ctx context.Context) ([]rex.Session, error) {
	sessions, err := b.client.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	var out []rex.Session
	for _, label := range b.sessionLabels {
		for _, s := range sessions {
			if s.Label == label {
				out = append(out, s)
				break
			}
		}
	}
	return out, nil
}

// findInView is findLive's search of one session's view.
func (b *Binder) findInView(name SessionName, lbl resolvedLabels, pol scanPolicy, sid rex.SessionID, view rex.SessionView) (Binding, bool) {
	pane := lbl.pane
	// IsRunning and Stop resolve without a start env, so the fallback scan
	// must recognise the session identity alone: a pane labelled
	// "<agent> (<GC_SESSION_NAME>)" still belongs to name even though the agent
	// prefix is unknown here. A bare label matching the session name is not
	// enough in these tabs: a legacy pane of another session can carry exactly
	// that label ("rig--rig--worker" is labelled "rig--worker" and sits in
	// "Rig: rig"), so a bare label is only accepted as the expected pane label
	// from the hint or record above.
	identity := lbl.identity(name)
	paneMatches := func(label string) bool {
		if label == pane || identityMatch(identity, label) {
			return true
		}
		for _, legacyPane := range lbl.legacyPanes {
			if label == legacyPane {
				return true
			}
		}
		return false
	}
	accept := func(tab string, w rex.Window, blk rex.Block) (Binding, bool) {
		bd := Binding{
			Name:         name,
			SessionID:    sid,
			SessionLabel: view.Label,
			WindowID:     w.ID,
			BlockID:      blk.ID,
			TabLabel:     tab,
			PaneLabel:    blk.Label,
			Aliases:      lbl.aliases,
			// Launch details the labels do not carry: dropping them
			// would silently disable the Stop-time drain diagnostic.
			LaunchMode: pol.record.LaunchMode,
			GCBin:      pol.record.GCBin,
			WorkDir:    pol.record.WorkDir,
			// The pending survivor set is cleanup state, not identity: a
			// replacement pane keeps it, or the stop forgets pids a closed
			// pane no longer reports.
			Survivors: pol.record.Survivors,
		}
		if pol.save {
			if pol.record.BlockID != blk.ID {
				// Adopted: a pane this provider did not start. The retired
				// record's creation time and identity are not its, so keep the
				// creation time unknown and mark it adopted; a later Observe
				// then reports the same `created` the read-only scan did.
				bd.CreatedAt = time.Time{}
				bd.Adopted = true
			} else {
				bd.CreatedAt = pol.record.CreatedAt
				bd.Adopted = pol.record.Adopted
			}
			_ = b.saveKeepingSurvivors(bd)
		}
		return bd, true
	}
	// Most specific first, so scan order never decides which pane a tab
	// yields: a suffix-labelled pane names this session outright, while a
	// bare label only belongs to it through the hint/record.
	for _, tab := range lbl.tabs {
		for _, w := range view.Windows {
			if w.Label != tab {
				continue
			}
			for _, blk := range w.Blocks {
				if identityMatch(identity, blk.Label) {
					return accept(tab, w, blk)
				}
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
	// name cannot derive. The session name is unique per city, so a suffix
	// match in any tab is that session's pane. A bare label equal to the
	// session name is NOT accepted here: another session's pane can carry the
	// same bare label (a rigless "other--worker" is labelled "worker"), so a
	// bare label only counts where a tab or record supplies attribution. The
	// stored-record/hint path stays tab-scoped above.
	if lbl.recordless {
		for _, w := range view.Windows {
			for _, blk := range w.Blocks {
				if identityMatch(identity, blk.Label) {
					return accept(w.Label, w, blk)
				}
			}
		}
	}
	return Binding{}, false
}

// sameIncarnation reports whether other records the same pane: the same block
// and window, created at the same time. A start that replaced the binding
// leaves a different incarnation.
func (b Binding) sameIncarnation(other Binding) bool {
	return b.BlockID == other.BlockID && b.WindowID == other.WindowID && b.CreatedAt.Equal(other.CreatedAt)
}

// Record stores binding, replacing any earlier one for the same name. The
// caller must handle the error: an unsaved record leaves a pane or a survivor
// set no later stop can name.
func (b *Binder) Record(binding Binding) error { return b.store.Write(binding.Name, binding) }

// RecordIfCurrent stores binding only when the record under its name still
// describes the same incarnation. A start that replaced the binding leaves a
// different record, and a stale survivor write must not overwrite it. A
// provisional record has no block id yet and is not a replacement, but only
// the launch attempt that wrote it may replace it: an empty block id alone
// does not authorize the write, or a stale writer could take another launch's
// pending record and that launch's own write would be refused. It reports
// whether the write happened.
func (b *Binder) RecordIfCurrent(binding Binding) (bool, error) {
	current, ok, err := b.store.Read(binding.Name)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	if current.BlockID == "" {
		if current.Attempt == "" || current.Attempt != binding.Attempt {
			return false, nil
		}
	} else if !binding.sameIncarnation(current) {
		return false, nil
	}
	return true, b.store.Write(binding.Name, binding)
}

// ForgetIfCurrent drops the stored binding only when it still records
// binding's incarnation. A start that replaced the binding while a lock-free
// retirement ran leaves a different record, and the retirement's Forget must
// not drop it. binding is the incarnation argument: it names the exact pane
// the caller retired. It reports whether the record was dropped.
func (b *Binder) ForgetIfCurrent(binding Binding) (bool, error) {
	current, ok, err := b.store.Read(binding.Name)
	if err != nil || !ok || !binding.sameIncarnation(current) {
		return false, err
	}
	if err := b.store.Drop(binding.Name); err != nil {
		return false, err
	}
	return true, nil
}

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
