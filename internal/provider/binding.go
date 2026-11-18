package provider

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
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
	// PaneLabel is the Rex block (pane) label the pane opened under. Empty on
	// legacy records, which fall back to Name.PaneLabel().
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
	// ReadyPrompt is the prompt text the agent shows when ready for input.
	ReadyPrompt string
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
}

// tabLabel returns the Rex tab label for the pane: the stored label when the
// record has one, otherwise the name-derived label for legacy records.
func (b Binding) tabLabel() string {
	if b.TabLabel != "" {
		return b.TabLabel
	}
	return b.Name.TabLabel()
}

// paneLabel returns the Rex pane label for the pane: the stored label when
// present, otherwise the name-derived label for legacy records.
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

// tabLabelFor returns the Rex tab label for a started session: GC_RIG when the
// start config carries a non-empty rig (gc 1.5.1 does not put the rig in pool
// names), otherwise the name-derived rig.
func tabLabelFor(name SessionName, env map[string]string) string {
	if rig := env["GC_RIG"]; rig != "" {
		return rig
	}
	return name.TabLabel()
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

// BindingStore persists bindings. MemoryStore is the default; FileStore keeps
// them on disk across restarts.
type BindingStore interface {
	// Load returns the binding for name, if any.
	Load(name SessionName) (Binding, bool)
	// Save stores b, replacing any binding with the same name.
	Save(b Binding)
	// Delete removes the binding for name.
	Delete(name SessionName)
}

// BindingLister is implemented by stores that can enumerate their bindings.
type BindingLister interface {
	// List returns all bindings, sorted by Name.
	List() []Binding
}

// MemoryStore is a BindingStore held in memory. It is safe for concurrent use.
type MemoryStore struct {
	mu sync.RWMutex
	m  map[SessionName]Binding
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{m: make(map[SessionName]Binding)}
}

// Load implements BindingStore.
func (s *MemoryStore) Load(name SessionName) (Binding, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.m[name]
	return b, ok
}

// Save implements BindingStore.
func (s *MemoryStore) Save(b Binding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[b.Name] = b
}

// List implements BindingLister.
func (s *MemoryStore) List() []Binding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Binding, 0, len(s.m))
	for _, b := range s.m {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Delete implements BindingStore.
func (s *MemoryStore) Delete(name SessionName) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, name)
}

// Binder resolves gc session names to Rex addresses, using a store as a
// cache and the Rex session view as the source of truth.
type Binder struct {
	client       rex.Client
	store        BindingStore
	sessionLabel string
}

// NewBinder builds a Binder that looks for the Rex session labelled
// sessionLabel and caches bindings in store.
func NewBinder(client rex.Client, store BindingStore, sessionLabel string) *Binder {
	return &Binder{client: client, store: store, sessionLabel: sessionLabel}
}

// FindSession returns the city's Rex session. The bool is false when it does
// not exist.
func (b *Binder) FindSession(ctx context.Context) (rex.SessionID, bool, error) {
	sessions, err := b.client.ListSessions(ctx)
	if err != nil {
		return "", false, err
	}
	for _, s := range sessions {
		if s.Label == b.sessionLabel {
			return s.ID, true, nil
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
	if bd, ok := b.store.Load(name); ok {
		out = append(out, bd)
		seen[bd.Name] = true
	}
	lister, ok := b.store.(BindingLister)
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
	if bd.Name == "" || bd.SessionLabel != b.sessionLabel {
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
	case stillBound(view, bd):
		return bd, true, nil
	default:
		return Binding{}, false, nil
	}
}

// Resolve returns the binding for name, resolving aliases to their primary
// binding. A stored binding is checked against the requested name, the
// configured session label and the session view; if any check fails, or Rex
// reports the session as not found, it is deleted and the session is scanned
// by its stored (or name-derived) labels. Any other view error is returned as
// is and leaves the stored binding alone.
// A name with no pane is reported as false with a nil error.
func (b *Binder) Resolve(ctx context.Context, name SessionName) (Binding, bool, error) {
	return b.ResolveHint(ctx, name, Binding{})
}

// ResolveHint is Resolve with an optional hint carrying the env-derived tab and
// pane labels a lost sidecar record used. gc 1.5.1 starts pool/wisp instances
// under a name with no rig, so without the hint a lost record cannot be
// rediscovered from the name alone and a live pane looks dead.
func (b *Binder) ResolveHint(ctx context.Context, name SessionName, envHint Binding) (Binding, bool, error) {
	var stale []Binding
	var hint Binding
	for _, bd := range b.lookup(name) {
		if !matchesName(bd, name) {
			// The record stored under this key answers to a different name:
			// drop the key, never the record's own name.
			b.store.Delete(name)
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
	for _, bd := range stale {
		b.store.Delete(bd.Name)
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
	return b.scan(ctx, scanName, scanHint)
}

// stillBound reports whether view still holds the binding's block under its
// stored tab and pane labels (falling back to the name-derived labels for
// legacy records).
func stillBound(view rex.SessionView, bd Binding) bool {
	if view.Label != bd.SessionLabel {
		return false
	}
	for _, w := range view.Windows {
		if w.ID != bd.WindowID || w.Label != bd.tabLabel() {
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
func (b *Binder) scan(ctx context.Context, name SessionName, hint Binding) (Binding, bool, error) {
	sid, ok, err := b.FindSession(ctx)
	if err != nil || !ok {
		return Binding{}, false, err
	}
	view, err := b.client.SessionView(ctx, sid)
	if err != nil {
		return Binding{}, false, err
	}
	tab := hint.TabLabel
	if tab == "" {
		tab = name.TabLabel()
	}
	pane := hint.PaneLabel
	if pane == "" {
		pane = name.PaneLabel()
	}
	for _, w := range view.Windows {
		if w.Label != tab {
			continue
		}
		for _, blk := range w.Blocks {
			if blk.Label == pane {
				bd := Binding{
					Name:         name,
					SessionID:    sid,
					SessionLabel: b.sessionLabel,
					WindowID:     w.ID,
					BlockID:      blk.ID,
					TabLabel:     tab,
					PaneLabel:    pane,
					Aliases:      hint.Aliases,
					// Launch details the labels do not carry: dropping them
					// would silently disable the Stop-time drain diagnostic.
					LaunchMode:  hint.LaunchMode,
					CreatedAt:   hint.CreatedAt,
					ReadyPrompt: hint.ReadyPrompt,
					GCBin:       hint.GCBin,
					WorkDir:     hint.WorkDir,
				}
				b.store.Save(bd)
				return bd, true, nil
			}
		}
	}
	return Binding{}, false, nil
}

// Record stores binding, replacing any earlier one for the same name.
func (b *Binder) Record(binding Binding) { b.store.Save(binding) }

// Forget drops the stored binding that answers to name, clearing the whole
// primary record including its aliases. An alias drops the primary it names.
func (b *Binder) Forget(name SessionName) {
	if bd, ok := b.store.Load(name); ok {
		b.store.Delete(bd.Name)
		b.store.Delete(name)
		return
	}
	if cands := b.lookup(name); len(cands) > 0 {
		b.store.Delete(cands[0].Name)
	}
}
