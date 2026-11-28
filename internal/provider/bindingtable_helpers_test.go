package provider

import "testing"

// mustBinding returns an unwrapper for Provider.Binding's three results that
// fails the test on a read error, so a healthy-path assertion never reads an
// unreadable store as an absent binding. Call it as
// mustBinding(t)(p.Binding(name)).
func mustBinding(t *testing.T) func(Binding, bool, error) (Binding, bool) {
	return func(bd Binding, ok bool, err error) (Binding, bool) {
		t.Helper()
		if err != nil {
			t.Fatalf("binding read: %v", err)
		}
		return bd, ok
	}
}
