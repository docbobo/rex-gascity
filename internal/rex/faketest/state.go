package faketest

import (
	"fmt"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/fakerex/fakestate"
)

// This file is the adapter between the fake's exported, test-seeded fields
// (Sessions, View, Views) and fakestate, the single owner of Rex
// session/window/block transitions. Every state-touching method hydrates a
// fakestate view of the acting session, calls one transition, and projects the
// result back. There is no cached *fakestate.State: a test's direct field
// write stays authoritative for the next call (REQ-007/BR-6).

// viewLocked returns the facet view a call acts on: Views[sid] when set, else
// the single View, exactly as SessionView reads them.
func (f *Fake) viewLocked(sid rex.SessionID) rex.SessionView {
	if v, ok := f.Views[sid]; ok {
		return v
	}
	return f.View
}

// knownSessionLocked reports whether sid names a session the fake holds. With
// no Sessions and no Views, sid names the single facet View, which is how
// faketest's field-seeded tests have always called it.
func (f *Fake) knownSessionLocked(sid rex.SessionID) bool {
	if len(f.Sessions) == 0 && len(f.Views) == 0 {
		return true
	}
	for _, s := range f.Sessions {
		if s.ID == sid {
			return true
		}
	}
	_, ok := f.Views[sid]
	return ok
}

// hydrationLocked builds the fakestate session a transition acts on. f.mu must
// be held.
func (f *Fake) hydrationLocked(sid rex.SessionID) (*fakestate.State, error) {
	if !f.knownSessionLocked(sid) {
		return nil, rex.ErrNotFound
	}
	st := fakestate.NewState()
	ses := &fakestate.Session{ID: string(sid)}
	for _, s := range f.Sessions {
		if s.ID == sid {
			ses.Label = s.Label
		}
	}
	v := f.viewLocked(sid)
	ses.ActiveWindow = string(v.ActiveWindowID)
	for _, w := range v.Windows {
		fw := &fakestate.Window{
			ID:           string(w.ID),
			Label:        w.Label,
			FocusedBlock: string(w.FocusedBlockID),
		}
		for _, b := range w.Blocks {
			fw.Blocks = append(fw.Blocks, &fakestate.Block{
				ID:    string(b.ID),
				Label: b.Label,
				Owner: b.Owner,
			})
		}
		ses.Windows = append(ses.Windows, fw)
	}
	st.Sessions = append(st.Sessions, ses)
	return st, nil
}

// projectLocked writes the acting session's facet view back to Views[sid] when
// that key exists, else to View. Everything but Windows and ActiveWindowID is
// preserved, and each block's Rect is carried over by id because fakestate
// does not model geometry. Revision is never projected. f.mu must be held.
func (f *Fake) projectLocked(st *fakestate.State, sid rex.SessionID) {
	ses, err := st.Session(string(sid))
	if err != nil {
		return
	}
	prior := f.viewLocked(sid)
	rects := make(map[rex.BlockID][4]float64, len(prior.Windows))
	for _, w := range prior.Windows {
		for _, b := range w.Blocks {
			rects[b.ID] = b.Rect
		}
	}
	v := prior
	v.Windows = make([]rex.Window, 0, len(ses.Windows))
	for _, w := range ses.Windows {
		rw := rex.Window{
			ID:             rex.WindowID(w.ID),
			Label:          w.Label,
			FocusedBlockID: rex.BlockID(w.FocusedBlock),
		}
		for _, b := range w.Blocks {
			rw.Blocks = append(rw.Blocks, rex.Block{
				ID:    rex.BlockID(b.ID),
				Label: b.Label,
				Owner: b.Owner,
				Rect:  rects[rex.BlockID(b.ID)],
			})
		}
		v.Windows = append(v.Windows, rw)
	}
	v.ActiveWindowID = rex.WindowID(ses.ActiveWindow)
	if _, ok := f.Views[sid]; ok {
		f.Views[sid] = v
		return
	}
	f.View = v
}

// projectSessionsLocked rewrites f.Sessions from st, the session records a
// transition leaves behind. Sessions carry only an id and a label, so the
// projection is total. f.mu must be held.
func (f *Fake) projectSessionsLocked(st *fakestate.State) {
	sessions := make([]rex.Session, 0, len(st.Sessions))
	for _, s := range st.Sessions {
		sessions = append(sessions, rex.Session{ID: rex.SessionID(s.ID), Label: s.Label})
	}
	f.Sessions = sessions
}

// anchorWindowLocked finds the window holding anchor in ses. One lookup path:
// fakestate's transitions address windows by resolved value, so the adapter
// resolves the anchor here instead of searching for it in each method.
func anchorWindowLocked(ses *fakestate.Session, anchor rex.BlockID) (*fakestate.Window, error) {
	for _, w := range ses.Windows {
		for _, b := range w.Blocks {
			if b.ID == string(anchor) {
				return w, nil
			}
		}
	}
	return nil, fmt.Errorf("anchor %s: %w", anchor, rex.ErrNotFound)
}
