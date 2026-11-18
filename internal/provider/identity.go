package provider

import (
	"errors"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// startIdentityMetaKeys are the start-config identity keys the provider seeds
// into its metadata sidecar before the blocking pane launch. gc's pending-create
// ownership check reads GC_SESSION_ID and GC_INSTANCE_TOKEN (and the runtime
// epoch) through get-meta; an empty answer while the pane is still booting makes
// gc read the fresh runtime as "live runtime belongs to another session" and
// roll it back. The durable provider contract remains GC_RIG/GC_TEMPLATE/
// GC_ALIAS/GC_AGENT; these are the incidental identity keys that must be
// visible to gc during the launch window.
var startIdentityMetaKeys = []string{
	"GC_SESSION_ID",
	"GC_INSTANCE_TOKEN",
	"GC_RUNTIME_EPOCH",
	"GC_CONTINUATION_EPOCH",
	"GC_AGENT",
	"GC_TEMPLATE",
	"GC_ALIAS",
	"GC_PROVIDER",
}

// clearStartIdentity removes the identity keys seeded for a start. It is used
// when a start rolls back and when Stop retires the pane, so a previous life's
// GC_SESSION_ID/GC_INSTANCE_TOKEN cannot be read as the current one.
func (p *Provider) clearStartIdentity(n SessionName) error {
	var errs []error
	for _, key := range startIdentityMetaKeys {
		if err := p.meta.Delete(n, key); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// seedStartIdentity replaces any prior life's identity metadata under n with
// the values from this start's cfg.Env. Missing or empty values are left unset,
// after the clear, so a prior token cannot survive a start that carries no
// replacement.
func (p *Provider) seedStartIdentity(n SessionName, env map[string]string) error {
	if err := p.clearStartIdentity(n); err != nil {
		return err
	}
	for _, key := range startIdentityMetaKeys {
		if v := env[key]; v != "" {
			if err := p.meta.Set(n, key, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// recordProvisionalBinding persists the pane's identity before the blocking
// launch. The block id is not known until NewWindow/NewSplit returns, so the
// provisional record has no BlockID; it exists so get-meta can resolve the
// runtime name and its rig-qualified aliases during the launch window. The
// final record replaces it after the launch returns.
func (p *Provider) recordProvisionalBinding(n SessionName, sid rex.SessionID, wid rex.WindowID, cfg Config) {
	p.binder.Record(Binding{
		Name:         n,
		SessionID:    sid,
		SessionLabel: rexSessionLabel(p.city),
		WindowID:     wid,
		TabLabel:     tabLabelFor(n, cfg.Env),
		PaneLabel:    n.PaneLabel(),
		Aliases:      aliasesFor(n, cfg.Env),
		CreatedAt:    time.Now(),
		GCBin:        gcBinBasename(cfg.Env),
		WorkDir:      cfg.WorkDir,
	})
}
