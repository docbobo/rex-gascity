package provider

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"

	"github.com/docbobo/rex-gascity/internal/launch"
)

// startPlan builds the one launch plan for this start and its attempt token.
// It runs first because the plan projects pool routing for every later step and
// owns the Rex block layout: the launch label must come from the same projected
// env as the start labels and the record, or pool routing makes them disagree
// and a restart launches a duplicate pane. cfg.Env is replaced with the
// projected env for the steps that follow.
//
// One token is minted per start call. The provisional record this start writes
// is writable only by this attempt, so a stale survivor write from another
// launch cannot take it, and launchPane's retry reuses the same token.
func (p *Provider) startPlan(name string, cfg *Config) (launch.Plan, string) {
	n := SessionName(name)
	attempt := rand.Text()
	env := launch.Project(cfg.Env)
	plan := launch.NewPlan(launch.Config{
		Argv:    cfg.Argv,
		Env:     env,
		WorkDir: cfg.WorkDir,
		Label:   newLaunchIdentity(n, env, "").PaneLabel(),
	})
	cfg.Env = plan.Env
	// gc's exec-session-provider contract makes env optional and omitted when
	// empty, so launch.Project can return nil and start must treat it as an
	// empty env. A nil map is not equivalent downstream: ResolveFresh reads nil
	// as "no start env" and rescans under a stale record's labels, missing a
	// live pane whose env-derived labels differ and launching a duplicate. The
	// launch spec is already built above, so an empty map here changes nothing
	// on the wire (it stays an omitted env).
	if cfg.Env == nil {
		cfg.Env = map[string]string{}
	}
	return plan, attempt
}

// prepareWorkDir readies the work_dir for the launch. It runs four steps in a
// fixed order and stops at the first error, which it returns as is:
//
//  1. stageStartFiles stages overlay_dir and copy_files before pre_start, so a
//     hook sees the staged work_dir.
//  2. runPreStartPhase runs the pre_start hooks.
//  3. requireWorkDir checks the work_dir after pre_start, because pre_start may
//     create it.
//  4. stagePackOverlayPhase stages the pack overlays after pre_start, because
//     their per-provider subtree overwrites a managed file the hook may have
//     rewritten.
//
// It takes no lock of its own. It runs under the per-name lock its caller
// holds. The city-wide state lock is not held, so a slow hook does not stall
// the rest of the city.
func (p *Provider) prepareWorkDir(ctx context.Context, plan launch.Plan, cfg Config) error {
	if err := p.stageStartFiles(cfg); err != nil {
		return err
	}
	if err := p.runPreStartPhase(ctx, plan, cfg); err != nil {
		return err
	}
	if err := p.requireWorkDir(cfg.WorkDir); err != nil {
		return err
	}
	return p.stagePackOverlayPhase(cfg)
}

// stageStartFiles stages gc's overlay_dir and copy_files before pre_start, so a
// hook sees the staged work_dir (herdrcore/lifecycle.py:789).
// stagePackOverlayPhase stays after pre_start because its per-provider subtree
// deliberately overwrites a managed hook the hook may have rewritten.
func (p *Provider) stageStartFiles(cfg Config) error {
	if err := stageOverlayDir(cfg.WorkDir, cfg.OverlayDir, providerName(cfg.Env)); err != nil {
		return err
	}
	return stageCopyFiles(cfg.WorkDir, cfg.CopyFiles, func(msg string) { p.warnf("%s", msg) })
}

// runPreStartPhase runs the pre_start hooks before identity or a provisional
// binding is seeded. gc's exec-session-provider contract makes a failing
// pre_start non-fatal: warn on stderr and keep starting
// (docs/reference/exec-session-provider.md at v1.5.1, "pre_start"). Only
// cancellation of the whole start aborts; a hook's own failure or timeout does
// not. The state lock is not held here: a slow hook must not stall the rest of
// the city.
func (p *Provider) runPreStartPhase(ctx context.Context, plan launch.Plan, cfg Config) error {
	return p.runPreStartHooks(ctx, plan, cfg)
}

// requireWorkDir checks the configured work dir after pre_start. A pre_start
// hook is the documented way to create the launch work dir (the gastown
// worktree recipe points work_dir at a worktree that pre_start creates). Mirror
// runtime-herdr-core: once the hooks have run, a configured work_dir must exist
// or the launch would run elsewhere.
func (p *Provider) requireWorkDir(workDir string) error {
	if workDir == "" {
		return nil
	}
	if info, err := os.Stat(workDir); err != nil || !info.IsDir() {
		return fmt.Errorf("work_dir %q is not an existing directory after pre_start", workDir)
	}
	return nil
}

// stagePackOverlayPhase writes each pack's per-provider overlay subtree after
// pre_start. gc's managed-file preserve predicate can keep an existing current
// file, so the controller may never write a city-configured pack overlay. The
// write runs here so the override lands even when the workdir already holds
// that managed file.
func (p *Provider) stagePackOverlayPhase(cfg Config) error {
	return stagePackOverlays(cfg.WorkDir, cfg.PackOverlayDirs, providerName(cfg.Env))
}
