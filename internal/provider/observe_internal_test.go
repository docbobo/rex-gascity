package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// failingKeyStore fails Read for one key only, so a test can hold one identity
// key unreadable while the rest stay readable.
type failingKeyStore struct {
	Store[metaKey, string]
	fail string
}

func (s *failingKeyStore) Read(k metaKey) (string, bool, error) {
	if k.key == s.fail {
		return "", false, errors.New("corrupt entry")
	}
	return s.Store.Read(k)
}

// TestObserveUnreadableIdentityIsNull proves review finding 2: one unreadable
// identity key makes observe answer null, not a partial identity, because gc
// reads a partial one as the complete set and ends the read early with keys
// missing.
func TestObserveUnreadableIdentityIsNull(t *testing.T) {
	ctx := context.Background()
	name := "pi-mc-wisp-abc"
	store := &failingKeyStore{Store: NewMemoryMetaStore(), fail: "GC_INSTANCE_TOKEN"}
	p := New("acme", faketest.New(), WithMetaStore(store))
	cfg := Config{Argv: []string{"pi"}, Env: map[string]string{
		"GC_SESSION_ID":     "mc-wisp-abc",
		"GC_INSTANCE_TOKEN": "token-abc",
	}}
	if err := p.Start(ctx, name, cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ans, err := p.Observe(ctx, name, nil)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	raw, err := json.Marshal(ans)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"identity":null`)) {
		t.Errorf("answer = %s, want null identity while a key is unreadable", raw)
	}

	// The whole set returns once every key is readable again.
	store.fail = ""
	again, err := p.Observe(ctx, name, nil)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if again.Identity["GC_SESSION_ID"] != "mc-wisp-abc" || again.Identity["GC_INSTANCE_TOKEN"] != "token-abc" {
		t.Errorf("identity = %v, want the whole set once readable", again.Identity)
	}
}
