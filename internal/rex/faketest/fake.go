// Package faketest provides a fake rex.Client for provider-core tests. It
// stands in for the external Rex server; the real client is exercised
// separately against a fake `rex` binary on PATH.
package faketest

import (
	"context"
	"errors"

	"github.com/docbobo/rex-gas-city/internal/rex"
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

// Fake is a recording rex.Client.
type Fake struct {
	Sessions []rex.Session
	View     rex.SessionView

	// Processes is the process state BlockProcess reports, keyed by block.
	Processes map[rex.BlockID]rex.ProcessInfo

	// Formats is the rendered output BlockFormat reports, keyed by block.
	Formats map[rex.BlockID]string

	// BlockCalls records each block-method call with the session that held the
	// block, so tests can assert the provider scopes the call correctly.
	BlockCalls []BlockCall

	CreatedSessions []string
	CreatedWindows  []CreatedWindow
	CreatedSplits   []CreatedSplit
	ClosedBlocks    []rex.BlockID
}

// BlockCall records one block-method call.
type BlockCall struct {
	SessionID rex.SessionID
	BlockID   rex.BlockID
	Op        string
}

func New() *Fake { return &Fake{} }

func (f *Fake) CreateSession(_ context.Context, label string) (rex.SessionID, error) {
	f.CreatedSessions = append(f.CreatedSessions, label)
	id := rex.SessionID("session:" + label)
	f.Sessions = append(f.Sessions, rex.Session{ID: id, Label: label})
	return id, nil
}

func (f *Fake) NewWindow(_ context.Context, sid rex.SessionID, label string, spec rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	f.CreatedWindows = append(f.CreatedWindows, CreatedWindow{SessionID: sid, Label: label, Spec: spec})
	return rex.WindowID("window:" + label), rex.BlockID("block:" + spec.Label), nil
}

func (f *Fake) NewSplit(_ context.Context, sid rex.SessionID, anchor rex.BlockID, direction, side string, spec rex.LaunchSpec) (rex.BlockID, error) {
	f.CreatedSplits = append(f.CreatedSplits, CreatedSplit{SessionID: sid, Anchor: anchor, Direction: direction, Side: side, Spec: spec})
	return rex.BlockID("block:" + spec.Label), nil
}

func (f *Fake) CloseBlock(_ context.Context, _ rex.SessionID, block rex.BlockID) error {
	f.ClosedBlocks = append(f.ClosedBlocks, block)
	return nil
}

func (f *Fake) SessionView(_ context.Context, sid rex.SessionID) (rex.SessionView, error) {
	v := f.View
	v.SessionID = sid
	return v, nil
}

func (f *Fake) ListSessions(_ context.Context) ([]rex.Session, error) { return f.Sessions, nil }

func (f *Fake) BlockProcess(_ context.Context, sid rex.SessionID, block rex.BlockID) (rex.ProcessInfo, error) {
	f.record(sid, block, "process")
	return f.Processes[block], nil
}

func (f *Fake) BlockFormat(_ context.Context, sid rex.SessionID, block rex.BlockID) (string, error) {
	f.record(sid, block, "format")
	return f.Formats[block], nil
}

func (f *Fake) BlockWrite(_ context.Context, sid rex.SessionID, block rex.BlockID, _ string) error {
	f.record(sid, block, "write")
	return nil
}

func (f *Fake) BlockEvent(_ context.Context, sid rex.SessionID, block rex.BlockID, _ rex.Event) error {
	f.record(sid, block, "event")
	return nil
}

func (f *Fake) BlockClear(_ context.Context, sid rex.SessionID, block rex.BlockID) error {
	f.record(sid, block, "clear")
	return nil
}

// record appends a block-method call to the recording.
func (f *Fake) record(sid rex.SessionID, block rex.BlockID, op string) {
	f.BlockCalls = append(f.BlockCalls, BlockCall{SessionID: sid, BlockID: block, Op: op})
}

func (f *Fake) Events(_ context.Context, _ rex.SessionID) (<-chan rex.Event, error) {
	return nil, errors.New("events: not implemented")
}
