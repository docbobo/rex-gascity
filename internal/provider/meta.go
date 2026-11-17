package provider

import "context"

// SetMeta stores value under key for the named session, byte for byte. It
// never contacts Rex; ctx is unused and kept for the dispatcher's call shape.
// A failed write is returned, not swallowed.
func (p *Provider) SetMeta(_ context.Context, name, key, value string) error {
	return p.meta.Set(SessionName(name), key, value)
}

// GetMeta returns the value stored under key, or "" and nil if it was never
// set. It never contacts Rex; ctx is unused.
func (p *Provider) GetMeta(_ context.Context, name, key string) (string, error) {
	v, _ := p.meta.Get(SessionName(name), key)
	return v, nil
}

// RemoveMeta deletes only key for the named session. It never contacts Rex;
// ctx is unused.
func (p *Provider) RemoveMeta(_ context.Context, name, key string) error {
	return p.meta.Delete(SessionName(name), key)
}
