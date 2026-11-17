package provider

import "context"

// metaName maps a gc session name to the primary binding that answers to it,
// so metadata written under a runtime name is readable under its rig-qualified
// alias: gc reads keys such as GC_SESSION_ID and suspended under the canonical
// identity even for a pane started under the short pool name.
func (p *Provider) metaName(name string) SessionName {
	n := SessionName(name)
	if bd, ok := p.bindings.Load(n); ok && bd.Name == n {
		return n
	}
	if cands := p.binder.lookup(n); len(cands) > 0 {
		return cands[0].Name
	}
	return n
}

// SetMeta stores value under key for the named session, byte for byte. It
// resolves rig-qualified aliases to the pane's primary name. It never contacts
// Rex; ctx is unused and kept for the dispatcher's call shape. A failed write is
// returned, not swallowed.
func (p *Provider) SetMeta(_ context.Context, name, key, value string) error {
	return p.meta.Set(p.metaName(name), key, value)
}

// GetMeta returns the value stored under key, or "" and nil if it was never
// set. A store that cannot be read is an error. It never contacts Rex; ctx is
// unused.
func (p *Provider) GetMeta(_ context.Context, name, key string) (string, error) {
	v, ok, err := p.meta.Get(p.metaName(name), key)
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
	return p.meta.Delete(p.metaName(name), key)
}
