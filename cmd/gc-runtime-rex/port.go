package main

import (
	"context"

	"github.com/docbobo/rex-gascity/internal/provider"
)

// rppProvider is the provider port of the request module: exactly the
// provider methods the op handlers call, one per handler (REQ-001 of #75).
// runWithContext hands each handler a *provider.Provider; tests hand it a
// recordingProvider to see which provider call a request turns into.
type rppProvider interface {
	StartNew(ctx context.Context, name string, cfg provider.Config) error
	Stop(ctx context.Context, name string) error
	IsRunning(ctx context.Context, name string) (bool, error)
	Interrupt(ctx context.Context, name string) error
	NudgeReport(ctx context.Context, name, text string, mode provider.NudgeMode) (provider.NudgeReport, error)
	SendKeys(ctx context.Context, name string, keys ...string) error
	ClearScrollback(ctx context.Context, name string) error
	ProcessAlive(ctx context.Context, name string, names []string) (provider.Liveness, error)
	PeekLines(ctx context.Context, name string, lines int) (string, error)
	ListRunning(ctx context.Context, prefix string) ([]string, error)
	GetLastActivity(ctx context.Context, name string) (provider.LastActivity, error)
	IdleSnapshot(ctx context.Context, name string) (provider.IdleState, error)
	ProgramStatus(ctx context.Context, name string) (provider.ProgramStatusReading, error)
	IsAttached(ctx context.Context, name string) (bool, error)
	SetMeta(ctx context.Context, name, key, value string) error
	GetMeta(ctx context.Context, name, key string) (string, error)
	RemoveMeta(ctx context.Context, name, key string) error
	Observe(ctx context.Context, name string, names []string) (provider.ObserveAnswer, error)
	Inventory(ctx context.Context) (provider.InventoryAnswer, error)
	KillObject(ctx context.Context, name string, req provider.KillObjectRequest) (string, error)
}

// The production provider must satisfy the port; a signature change in
// internal/provider fails the build here, not at a call site.
var _ rppProvider = (*provider.Provider)(nil)
