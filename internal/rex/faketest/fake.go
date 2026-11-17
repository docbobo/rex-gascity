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

	CreatedSessions []string
	CreatedWindows  []CreatedWindow
	CreatedSplits   []CreatedSplit
	ClosedBlocks    []rex.BlockID
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

func (f *Fake) BlockProcess(_ context.Context, _ rex.BlockID) (rex.ProcessInfo, error) {
	return rex.ProcessInfo{}, nil
}

func (f *Fake) BlockFormat(_ context.Context, _ rex.BlockID) (string, error) { return "", nil }

func (f *Fake) BlockWrite(_ context.Context, _ rex.BlockID, _ string) error { return nil }

func (f *Fake) BlockEvent(_ context.Context, _ rex.BlockID, _ rex.Event) error { return nil }

func (f *Fake) BlockClear(_ context.Context, _ rex.BlockID) error { return nil }

func (f *Fake) Events(_ context.Context, _ rex.SessionID) (<-chan rex.Event, error) {
	return nil, errors.New("events: not implemented")
}
