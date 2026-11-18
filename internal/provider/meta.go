package provider

import "context"

// metaName maps a gc session name to the primary binding that answers to it,
// so metadata written under a runtime name is readable under its rig-qualified
// alias: gc reads keys such as GC_SESSION_ID and suspended under the canonical
// identity even for a pane started under the short pool name.
func (p *Provider) metaName(name string) SessionName {
	n := SessionName(name)
	if bd, ok, _ := p.binder.Resolve(context.Background(), ResolveRequest{Name: n}); ok {
		return bd.Name
	}
	return n
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

// bindingRead is the provider's one place to read a stored binding without
// listing or contacting Rex.
func (p *Provider) bindingRead(name SessionName) (Binding, bool) {
	b, ok, _ := p.bindings.Read(name)
	return b, ok
}

func (p *Provider) metaClear(name SessionName) error {
	if c, ok := p.meta.(clearer); ok {
		return c.Clear(name)
	}
	return nil
}

// SetMeta stores value under key for the named session, byte for byte. It
// resolves rig-qualified aliases to the pane's primary name. It never contacts
// Rex; ctx is unused and kept for the dispatcher's call shape. A failed write is
// returned, not swallowed.
func (p *Provider) SetMeta(_ context.Context, name, key, value string) error {
	return p.metaSet(p.metaName(name), key, value)
}

// GetMeta returns the value stored under key, or "" and nil if it was never
// set. A store that cannot be read is an error. It never contacts Rex; ctx is
// unused.
func (p *Provider) GetMeta(_ context.Context, name, key string) (string, error) {
	v, ok, err := p.metaGet(p.metaName(name), key)
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
	return p.metaDelete(p.metaName(name), key)
}
