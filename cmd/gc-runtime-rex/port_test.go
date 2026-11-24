package main

import (
	"context"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
)

// providerCall is one call a handler made through the rppProvider port.
type providerCall struct {
	method string
	args   []any // the call's arguments after ctx
}

// recordingProvider is the test-only rppProvider (TS-2 of #75). Every method
// records its name and arguments and returns zero values with a nil error, so
// a test sees exactly which provider calls one request turned into. The zero
// values need not be valid answers; tests that use it assert the calls, not a
// handler's stdout.
type recordingProvider struct {
	calls []providerCall
}

var _ rppProvider = (*recordingProvider)(nil)

func (r *recordingProvider) record(method string, args ...any) {
	r.calls = append(r.calls, providerCall{method: method, args: args})
}

// methods lists the recorded method names in call order.
func (r *recordingProvider) methods() []string {
	var out []string
	for _, c := range r.calls {
		out = append(out, c.method)
	}
	return out
}

// namesFor returns the name list handed to the single recorded call of
// method (ProcessAlive or Observe), failing the test if there is not exactly
// one call or the call was a different method.
func (r *recordingProvider) namesFor(t *testing.T, method string) []string {
	t.Helper()
	if len(r.calls) != 1 || r.calls[0].method != method {
		t.Fatalf("recorded calls = %q, want exactly one %s call", r.methods(), method)
	}
	names, ok := r.calls[0].args[len(r.calls[0].args)-1].([]string)
	if !ok {
		t.Fatalf("%s: last argument is %T, want []string", method, r.calls[0].args[len(r.calls[0].args)-1])
	}
	return names
}

func (r *recordingProvider) StartNew(_ context.Context, name string, cfg provider.Config) error {
	r.record("StartNew", name, cfg)
	return nil
}

func (r *recordingProvider) Stop(_ context.Context, name string) error {
	r.record("Stop", name)
	return nil
}

func (r *recordingProvider) IsRunning(_ context.Context, name string) (bool, error) {
	r.record("IsRunning", name)
	return false, nil
}

func (r *recordingProvider) Interrupt(_ context.Context, name string) error {
	r.record("Interrupt", name)
	return nil
}

func (r *recordingProvider) NudgeReport(_ context.Context, name, text string, mode provider.NudgeMode) (provider.NudgeReport, error) {
	r.record("NudgeReport", name, text, mode)
	return provider.NudgeReport{}, nil
}

func (r *recordingProvider) SendKeys(_ context.Context, name string, keys ...string) error {
	r.record("SendKeys", name, keys)
	return nil
}

func (r *recordingProvider) ClearScrollback(_ context.Context, name string) error {
	r.record("ClearScrollback", name)
	return nil
}

func (r *recordingProvider) ProcessAlive(_ context.Context, name string, names []string) (provider.Liveness, error) {
	r.record("ProcessAlive", name, names)
	return provider.LivenessUnknown, nil
}

func (r *recordingProvider) PeekLines(_ context.Context, name string, lines int) (string, error) {
	r.record("PeekLines", name, lines)
	return "", nil
}

func (r *recordingProvider) ListRunning(_ context.Context, prefix string) ([]string, error) {
	r.record("ListRunning", prefix)
	return nil, nil
}

func (r *recordingProvider) GetLastActivity(_ context.Context, name string) (provider.LastActivity, error) {
	r.record("GetLastActivity", name)
	return provider.LastActivity{}, nil
}

func (r *recordingProvider) IdleSnapshot(_ context.Context, name string) (provider.IdleState, error) {
	r.record("IdleSnapshot", name)
	return "", nil
}

func (r *recordingProvider) ProgramStatus(_ context.Context, name string) (provider.ProgramStatusReading, error) {
	r.record("ProgramStatus", name)
	return provider.ProgramStatusReading{}, nil
}

func (r *recordingProvider) IsAttached(_ context.Context, name string) (bool, error) {
	r.record("IsAttached", name)
	return false, nil
}

func (r *recordingProvider) SetMeta(_ context.Context, name, key, value string) error {
	r.record("SetMeta", name, key, value)
	return nil
}

func (r *recordingProvider) GetMeta(_ context.Context, name, key string) (string, error) {
	r.record("GetMeta", name, key)
	return "", nil
}

func (r *recordingProvider) RemoveMeta(_ context.Context, name, key string) error {
	r.record("RemoveMeta", name, key)
	return nil
}

func (r *recordingProvider) Observe(_ context.Context, name string, names []string) (provider.ObserveAnswer, error) {
	r.record("Observe", name, names)
	return provider.ObserveAnswer{}, nil
}

func (r *recordingProvider) Inventory(_ context.Context) (provider.InventoryAnswer, error) {
	r.record("Inventory")
	return provider.InventoryAnswer{}, nil
}

func (r *recordingProvider) KillObject(_ context.Context, name string, req provider.KillObjectRequest) (string, error) {
	r.record("KillObject", name, req)
	return "", nil
}
