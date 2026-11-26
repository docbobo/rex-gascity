package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/docbobo/rex-gascity/internal/launch"
)

// hookBatch is one list of shell hooks to run for one hook site.
type hookBatch struct {
	Commands []string
	Phase    string // "pre_start", "session_setup", "session_setup_script"
	WorkDir  string
	Env      []string
	Budget   time.Time     // now + startBudget, already capped by the ctx deadline
	Timeout  time.Duration // the per-hook timeout
}

// hookResult is the outcome of one hook in a batch.
type hookResult struct {
	Index           int
	Command         string
	Err             error // nil on success, the hook's error otherwise
	Cancelled       bool  // the caller's ctx was cancelled or timed out
	BudgetExhausted bool  // the budget was spent before the hook ran
	Remaining       time.Duration
}

// runHooks runs each command under the per-hook timeout, capped by what
// remains of the batch budget. It returns one result per command, in order.
func runHooks(ctx context.Context, r Runner, batch hookBatch) []hookResult {
	results := make([]hookResult, 0, len(batch.Commands))
	for i, command := range batch.Commands {
		remaining := time.Until(batch.Budget)
		if remaining <= 0 {
			results = append(results, hookResult{
				Index: i, Command: command, BudgetExhausted: true, Remaining: remaining,
			})
			continue
		}
		timeout := min(batch.Timeout, remaining)
		// Resolve the cwd per hook: an earlier hook may create a WorkDir that
		// did not exist yet, and the later hooks must run inside it.
		cwd := preStartCwd(batch.WorkDir)
		hctx, cancel := context.WithTimeout(ctx, timeout)
		err := r.RunHook(hctx, HookCall{Command: command, Dir: cwd, Env: batch.Env})
		cancelled := ctx.Err() != nil
		cancel()
		if errors.Is(err, context.DeadlineExceeded) && !cancelled {
			err = fmt.Errorf("%s: timed out after %s", command, timeout)
		}
		results = append(results, hookResult{
			Index: i, Command: command, Err: err, Cancelled: cancelled, Remaining: remaining,
		})
	}
	return results
}

// hookBatch builds the batch for one hook site. The budget is the start budget
// or the earlier op context deadline. The timeout is the per-hook timeout.
func (p *Provider) hookBatch(ctx context.Context, commands []string, phase, workDir string, env []string) hookBatch {
	budget := time.Now().Add(p.startBudget)
	if d, ok := ctx.Deadline(); ok && d.Before(budget) {
		budget = d
	}
	return hookBatch{
		Commands: commands,
		Phase:    phase,
		WorkDir:  workDir,
		Env:      env,
		Budget:   budget,
		Timeout:  p.preStartHookTimeout,
	}
}

// runPreStartHooks runs the start config's pre_start hooks through the Runner
// seam. A failing or timed-out hook warns and the start continues; only a
// cancelled start aborts.
func (p *Provider) runPreStartHooks(ctx context.Context, plan launch.Plan, cfg Config) error {
	if len(cfg.PreStart) == 0 {
		return nil
	}
	batch := p.hookBatch(ctx, cfg.PreStart, "pre_start", cfg.WorkDir, plan.MergedEnv(os.Environ()))
	for _, r := range runHooks(ctx, p.runner, batch) {
		if r.BudgetExhausted {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("pre_start[%d]: start canceled: %w", r.Index, err)
			}
			p.warnf("pre_start[%d] start budget %s exhausted before the hook ran", r.Index, p.startBudget)
			break
		}
		if r.Err != nil {
			if r.Cancelled {
				return fmt.Errorf("pre_start[%d]: start canceled: %w", r.Index, ctx.Err())
			}
			p.warnf("pre_start[%d] %v", r.Index, r.Err)
		}
	}
	return nil
}
