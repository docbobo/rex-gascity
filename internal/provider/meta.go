package provider

import (
	"context"
	"fmt"
)

// metaName maps a gc session name to the primary binding that answers to it,
// so metadata written under a runtime name is readable under its rig-qualified
// alias: gc reads keys such as GC_SESSION_ID and suspended under the canonical
// identity even for a pane started under the short pool name. ok reports
// whether a session record answers to name; it is false when nothing is stored
// under name and name is not a recorded alias. A binding read failure is
// returned, never read as an absent record. It never contacts Rex.
func (p *Provider) metaName(name string) (SessionName, bool, error) {
	n := SessionName(name)
	bd, ok, err := p.binder.Resolve(context.Background(), ResolveRequest{Name: n})
	if err != nil {
		return n, false, err
	}
	if ok {
		return bd.Name, true, nil
	}
	return n, false, nil
}

// metaGet, metaSet, metaDelete and metaClear are the provider's one place to
// reach the generic metadata store; the key shape stays private here.
func (p *Provider) metaGet(name SessionName, key string) (string, bool, error) {
	return p.meta.Read(metaKey{name, key})
}

func (p *Provider) metaSet(name SessionName, key, value string) error {
	return p.meta.Write(metaKey{name, key}, value)
}

func (p *Provider) metaDelete(name SessionName, key string) error {
	return p.meta.Drop(metaKey{name, key})
}

// metaClear removes every key stored for name through the binding table, so
// the sweep is never optional: a meta store without Clear cannot be installed.
func (p *Provider) metaClear(name SessionName) error {
	return p.bindings.ClearMeta(name)
}

// SetMeta stores value under key for the named session, byte for byte. It
// resolves rig-qualified aliases to the pane's primary name. It requires a
// session record (the pack started a pane for name), matching herdr-core's
// `set-meta` precondition: an unknown, stopped or unstarted session is an error
// and stores nothing. It never contacts Rex; ctx is unused and kept for the
// dispatcher's call shape. A failed write is returned, not swallowed.
func (p *Provider) SetMeta(_ context.Context, name, key, value string) error {
	n, ok, err := p.metaName(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no session %q", name)
	}
	return p.metaSet(n, key, value)
}

// GetMeta returns the value stored under key, or "" and nil if it was never
// set. A store that cannot be read is an error. It never contacts Rex; ctx is
// unused.
func (p *Provider) GetMeta(_ context.Context, name, key string) (string, error) {
	n, _, err := p.metaName(name)
	if err != nil {
		return "", err
	}
	v, ok, err := p.metaGet(n, key)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}
	return v, nil
}

// RemoveMeta deletes only key for the named session. It never contacts Rex;
// ctx is unused.
func (p *Provider) RemoveMeta(_ context.Context, name, key string) error {
	n, _, err := p.metaName(name)
	if err != nil {
		return err
	}
	return p.metaDelete(n, key)
}
