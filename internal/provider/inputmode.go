package provider

import (
	"context"
	"errors"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// Secret-read guard (issue #48). Before the provider types a nudge or presses
// send-keys input, it reads the pane's terminal input mode and withholds the
// input while the program is reading a secret (a sudo, ssh, read -s or getpass
// password prompt). Typed text there would be submitted as a password attempt.
//
// The predicate canonical && !echo is UNVERIFIED: neither
// docs/provider-herdr-parity.md nor gc v1.5.1 covers input_mode. Its basis is
// docs/REX-CONTRACT.md §4 (input_mode, "detect a password/raw prompt") and the
// issue #48 live probe (rex 0.1.0-preview.21, api 18). A bare echo:false is
// readline (bash, python REPLs) and must not withhold. Raw-mode secret prompts
// (pinentry-curses) read canonical:false and are a known gap.
//
// The read fails open: any read error, decode error or probe timeout reports
// no secret read, so the guarded op behaves as it did before this guard. A
// reply that arrives after the probe context ended is discarded for the same
// reason.

// ErrSecretRead reports that input was withheld because the pane's program is
// reading a secret. Nudge answers it as the RPP outcome pending_interaction,
// send-keys as not_sent.
var ErrSecretRead = errors.New("provider: pane is reading a secret")

// defaultInputModeProbeTimeout bounds one input_mode read when the provider's
// context has no earlier deadline. It is short: a stalled read must not hold
// up the input it guards (precedent: claimProbeTimeout).
const defaultInputModeProbeTimeout = 2 * time.Second

// WithInputModeProbeTimeout sets the budget of one input_mode read. A value
// <= 0 keeps the default. It exists so tests can shrink the bound.
func WithInputModeProbeTimeout(d time.Duration) Option {
	return func(p *Provider) {
		if d > 0 {
			p.inputModeProbeTimeout = d
		}
	}
}

// secretRead reports whether m is a cooked read with echo off. signals takes
// no part.
func secretRead(m rex.InputMode) bool {
	return m.Canonical && !m.Echo
}

// readingSecret reports whether the pane b is reading a secret. It is the
// provider's only caller of BlockInputMode, so the nudge path, SendKeys and the
// startup pre-clear check share one bound.
//
// The read runs on its own probe context derived from ctx (never
// context.WithoutCancel), so a parent cancellation still stops it. Its budget
// is inputModeProbeTimeout, capped at half of the parent's remaining time when
// ctx has a deadline, so time is left for the input path. The live CLI's op
// context has no deadline (cmd/gc-runtime-rex/main.go signal.NotifyContext),
// and gc's 30 s adapter timeout is a separate process clock the provider
// cannot see, so there the fixed budget applies uncapped.
//
// Any error, decode failure or timeout answers false: the guarded op then
// resumes its original input path on the parent ctx, as it did before #48.
func (p *Provider) readingSecret(ctx context.Context, b Binding) bool {
	budget := p.inputModeProbeTimeout
	if budget <= 0 {
		budget = defaultInputModeProbeTimeout
	}
	if dl, ok := ctx.Deadline(); ok {
		if half := time.Until(dl) / 2; half < budget {
			budget = half
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	mode, err := p.client.BlockInputMode(probeCtx, b.SessionID, b.BlockID)
	if err != nil {
		return false
	}
	// A reply that arrives after the probe context ended is late: the probe
	// timed out or the parent was cancelled while the read was in flight. It
	// must not decide the guard. A probe timeout fails open and lets the
	// parent's live path type; a cancelled parent then stops at the caller's
	// own cancellation check. The CLI client can decode a response after its
	// command completes, so a late reply is reachable.
	if probeCtx.Err() != nil {
		return false
	}
	return secretRead(mode)
}
