// Package faketest provides a fake rex.Client for provider-core tests. It
// stands in for the external Rex server; the real client is exercised
// separately against a fake `rex` binary on PATH.
package faketest

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// CreatedWindow records a NewWindow call.
type CreatedWindow struct {
	SessionID rex.SessionID
	Label     string
	Spec      rex.LaunchSpec
}

// CreatedSplit records a NewSplit call.
type CreatedSplit struct {
	SessionID rex.SessionID
	Anchor    rex.BlockID
	Direction string
	Side      string
	Spec      rex.LaunchSpec
}

// SentKeys records a SendKeys call.
type SentKeys struct {
	SessionID rex.SessionID
	BlockID   rex.BlockID
	Keys      []string
}

// Fake is a recording rex.Client. Its methods are safe for concurrent use.
// Tests set and read its fields directly; do that only while no method call
// is in flight (before starting goroutines, or after they finish).
type Fake struct {
	mu sync.Mutex

	Sessions []rex.Session
	View     rex.SessionView

	// Processes is the process state BlockProcess reports, keyed by block.
	Processes map[rex.BlockID]rex.ProcessInfo

	// ProcessFunc, when non-nil, supersedes Processes. It is called with the
	// fake's lock held, so it must not call Fake methods; tests use it to model
	// a pane whose program exits between calls.
	ProcessFunc func(rex.BlockID) rex.ProcessInfo

	// Formats is the rendered output BlockFormat reports, keyed by block.
	Formats map[rex.BlockID]string

	// FormatFunc, when non-nil, supersedes Formats. It is called with the
	// fake's lock held, so it must not call Fake methods; tests use it to
	// model output that changes between calls.
	FormatFunc func(rex.BlockID) string

	// FormatErr, when set, is returned by BlockFormat instead of output.
	FormatErr error

	// BlockCalls records each block-method call with the session that held the
	// block, so tests can assert the provider scopes the call correctly.
	BlockCalls []BlockCall

	CreatedSessions []string
	CreatedWindows  []CreatedWindow
	CreatedSplits   []CreatedSplit
	ClosedBlocks    []rex.BlockID
	SentKeys        []SentKeys

	// WriteErr, ProcessErr and SendKeysErr, when set, are returned by those
	// methods.
	WriteErr    error
	ProcessErr  error
	SendKeysErr error

	// CloseBlockErr, when set, is returned by CloseBlock before it changes
	// anything.
	CloseBlockErr error

	// DestroySessionErr, when set, is returned by DestroySession before it
	// changes anything. DestroyedSessions records each destroyed session.
	DestroySessionErr error
	DestroyedSessions []rex.SessionID

	// ListSessionsCalls and SessionViewCalls count calls to those methods.
	ListSessionsCalls int
	SessionViewCalls  int

	// ListSessionsErr and SessionViewErr, when set, are returned by those
	// methods.
	ListSessionsErr error
	SessionViewErr  error

	// NotFoundSessions lists sessions for which SessionView returns
	// rex.ErrNotFound, as Rex does for a session that no longer exists.
	NotFoundSessions map[rex.SessionID]bool

	// Clients is what ListClients reports for every session.
	Clients []rex.ClientInfo

	// ProgramStatuses is the program status BlockProgramStatus reports, keyed by
	// block.
	ProgramStatuses map[rex.BlockID]rex.ProgramStatus

	// ProgramStatusFunc and WriteFunc, when non-nil, supersede the plain
	// maps for those methods. They are called with the fake's lock held, so
	// they must not call Fake methods; tests use them to model a program
	// that reacts to typed input between calls (ProgramStatusFunc) or to
	// observe the order of writes (WriteFunc).
	ProgramStatusFunc func(rex.BlockID) rex.ProgramStatus
	WriteFunc         func(sid rex.SessionID, block rex.BlockID, text string)

	// ListClientsErr and ProgramStatusErr, when set, are returned by those
	// methods.
	ListClientsErr   error
	ProgramStatusErr error
}

// BlockCall records one block-method call.
type BlockCall struct {
	SessionID rex.SessionID
	BlockID   rex.BlockID
	Op        string
	// Text is the written text for Op "write"; empty for other ops.
	Text string
}

func New() *Fake { return &Fake{} }

func (f *Fake) CreateSession(_ context.Context, label string) (rex.SessionID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CreatedSessions = append(f.CreatedSessions, label)
	id := rex.SessionID("session:" + label)
	f.Sessions = append(f.Sessions, rex.Session{ID: id, Label: label})
	return id, nil
}

func (f *Fake) NewWindow(_ context.Context, sid rex.SessionID, label string, spec rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CreatedWindows = append(f.CreatedWindows, CreatedWindow{SessionID: sid, Label: label, Spec: spec})
	wid, block := rex.WindowID("window:"+label), rex.BlockID("block:"+spec.Label)
	f.View.Windows = append(f.View.Windows, rex.Window{
		ID:     wid,
		Label:  label,
		Blocks: []rex.Block{{ID: block, Label: spec.Label}},
	})
	return wid, block, nil
}

func (f *Fake) NewSplit(_ context.Context, sid rex.SessionID, anchor rex.BlockID, direction, side string, spec rex.LaunchSpec) (rex.BlockID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CreatedSplits = append(f.CreatedSplits, CreatedSplit{SessionID: sid, Anchor: anchor, Direction: direction, Side: side, Spec: spec})
	block := rex.BlockID("block:" + spec.Label)
	for i, w := range f.View.Windows {
		for _, b := range w.Blocks {
			if b.ID == anchor {
				f.View.Windows[i].Blocks = append(w.Blocks, rex.Block{ID: block, Label: spec.Label})
				return block, nil
			}
		}
	}
	return block, nil
}

func (f *Fake) CloseBlock(_ context.Context, _ rex.SessionID, block rex.BlockID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.CloseBlockErr != nil {
		return f.CloseBlockErr
	}
	f.ClosedBlocks = append(f.ClosedBlocks, block)
	// A closed pane has no process: drop any recorded liveness so a block ID
	// reused by a relaunched pane starts live, as a fresh Rex block does.
	delete(f.Processes, block)
	var windows []rex.Window
	for _, w := range f.View.Windows {
		var kept []rex.Block
		for _, b := range w.Blocks {
			if b.ID != block {
				kept = append(kept, b)
			}
		}
		// Rex removes a tab when its last block closes.
		if len(kept) == 0 {
			continue
		}
		w.Blocks = kept
		windows = append(windows, w)
	}
	f.View.Windows = windows
	return nil
}

func (f *Fake) DestroySession(_ context.Context, sid rex.SessionID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.DestroySessionErr != nil {
		return f.DestroySessionErr
	}
	f.DestroyedSessions = append(f.DestroyedSessions, sid)
	f.Sessions = slices.DeleteFunc(f.Sessions, func(s rex.Session) bool { return s.ID == sid })
	f.View.Windows = nil
	return nil
}

func (f *Fake) SessionView(_ context.Context, sid rex.SessionID) (rex.SessionView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SessionViewCalls++
	if f.NotFoundSessions[sid] {
		return rex.SessionView{}, rex.ErrNotFound
	}
	if f.SessionViewErr != nil {
		return rex.SessionView{}, f.SessionViewErr
	}
	v := f.View
	v.Windows = cloneWindows(f.View.Windows)
	v.SessionID = sid
	if v.Label == "" {
		for _, s := range f.Sessions {
			if s.ID == sid {
				v.Label = s.Label
			}
		}
	}
	return v, nil
}

func (f *Fake) ListSessions(_ context.Context) ([]rex.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ListSessionsCalls++
	if f.ListSessionsErr != nil {
		return nil, f.ListSessionsErr
	}
	return slices.Clone(f.Sessions), nil
}

func (f *Fake) BlockProcess(_ context.Context, sid rex.SessionID, block rex.BlockID) (rex.ProcessInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(sid, block, "process")
	if f.ProcessErr != nil {
		return rex.ProcessInfo{}, f.ProcessErr
	}
	if f.ProcessFunc != nil {
		return f.ProcessFunc(block), nil
	}
	return f.Processes[block], nil
}

func (f *Fake) BlockFormat(_ context.Context, sid rex.SessionID, block rex.BlockID) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(sid, block, "format")
	if f.FormatErr != nil {
		return "", f.FormatErr
	}
	if f.FormatFunc != nil {
		return f.FormatFunc(block), nil
	}
	return f.Formats[block], nil
}

func (f *Fake) BlockWrite(_ context.Context, sid rex.SessionID, block rex.BlockID, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.BlockCalls = append(f.BlockCalls, BlockCall{SessionID: sid, BlockID: block, Op: "write", Text: text})
	if f.WriteFunc != nil {
		f.WriteFunc(sid, block, text)
	}
	return f.WriteErr
}

func (f *Fake) SendKeys(_ context.Context, sid rex.SessionID, block rex.BlockID, keys ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(sid, block, "send-key")
	f.SentKeys = append(f.SentKeys, SentKeys{SessionID: sid, BlockID: block, Keys: keys})
	return f.SendKeysErr
}

func (f *Fake) ListClients(_ context.Context, _ rex.SessionID) ([]rex.ClientInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ListClientsErr != nil {
		return nil, f.ListClientsErr
	}
	return slices.Clone(f.Clients), nil
}

func (f *Fake) BlockProgramStatus(_ context.Context, sid rex.SessionID, block rex.BlockID) (rex.ProgramStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(sid, block, "program_status")
	if f.ProgramStatusErr != nil {
		return rex.ProgramStatus{}, f.ProgramStatusErr
	}
	if f.ProgramStatusFunc != nil {
		return f.ProgramStatusFunc(block), nil
	}
	return f.ProgramStatuses[block], nil
}

func (f *Fake) BlockEvent(_ context.Context, sid rex.SessionID, block rex.BlockID, _ rex.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(sid, block, "event")
	return nil
}

func (f *Fake) BlockClear(_ context.Context, sid rex.SessionID, block rex.BlockID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(sid, block, "clear")
	return nil
}

// record appends a block-method call to the recording. f.mu must be held.
func (f *Fake) record(sid rex.SessionID, block rex.BlockID, op string) {
	f.BlockCalls = append(f.BlockCalls, BlockCall{SessionID: sid, BlockID: block, Op: op})
}

func (f *Fake) Events(_ context.Context, _ rex.SessionID) (<-chan rex.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return nil, errors.New("events: not implemented")
}

// cloneWindows copies ws and each window's block slice, so a returned view
// does not alias the fake's state.
func cloneWindows(ws []rex.Window) []rex.Window {
	if ws == nil {
		return nil
	}
	out := make([]rex.Window, len(ws))
	for i, w := range ws {
		w.Blocks = slices.Clone(w.Blocks)
		out[i] = w
	}
	return out
}
