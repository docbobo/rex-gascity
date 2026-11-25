## Subsystem notes

The rules in this file are repo-wide. Each subsystem also has a nested
`AGENTS.md` with the local invariants and the docs it depends on. Pi loads a
nested context file only when the working directory is inside that subtree, so
read the file before you touch that area:

- `internal/provider/AGENTS.md` — RPP v0 runtime semantics: sessions, panes,
  state, meta, overlays.
- `internal/rex/AGENTS.md` — the `rex` CLI client and the fake the e2e suites
  drive.
- `ui/AGENTS.md` — the Gas City UI Rex extension: its tests, its version
  rule, and the single installer.

## Autonomous runs

Agents may work this repo with no person at the keyboard. These rules define
what such a run may decide alone. They also define how it asks for a decision
it cannot make.

### Authority order

1. The task brief sets the work. That is the bead body, the issue, or the PR
   description.
2. This file sets the limits.
3. A task brief cannot grant an exception to a limit here.
4. When the two conflict, the limit wins.

On a conflict, record it in the task notes and do the part the limit allows. Do
not resolve the conflict by guessing what a person would want.

### Asking for a decision

A chat prompt may have no reader. Do not end a run on a question and wait.

When a decision needs a person:

- Do every part that does not need the decision.
- Write the decision, the options, and your recommendation in the task notes.
- Set the task to blocked, or close it with `work_outcome=blocked` and the
  reason.
- Send one mail to the mayor with the task id and a link to the notes.
- Stop. Do not retry in a loop. Do not pick an option that a limit forbids.

A missing tool, a missing credential, or a failing network call is not
permission to skip a check. Say what you could not verify.

### Done in an autonomous run

Done means three things. The change is committed. Every check you can run
passes. The task notes carry the evidence.

Merging is not yours unless the brief says so. Tagging and publishing are never
yours.

Never merge with admin rights, with a bypassed rule, or with a required check
skipped. Never disable branch protection to make a merge possible.

A pending re-review is not an approval. When the verdict is CHANGES_REQUESTED,
the merge waits for a new verdict from that reviewer. A reply to a review
comment does not change a verdict.

## Overlay changes need approval

Every file under an `overlay` directory MUST be approved by Boris before it
lands. This covers this pack's `pack/overlay/`, any `pack_overlay_dirs` entry a
city configures, and their `per-provider/<provider>` subtrees.

Adding, editing, moving, or deleting one of those files needs that approval. So
does adding an overlay directory itself.

An overlay is a patched copy of a file someone else owns. The runtime stages it
over its managed file. It drifts when that managed file changes. It can also
defeat the runtime's own preserve/upgrade check. A pinned version marker in the
overlay stops the runtime from reverting it.

An overlay is a managed exception, not a normal edit. The default answer is an
upstream fix in the owner's repo. An overlay needs a reason that fix cannot
wait.

In an autonomous run: prepare the diff on a branch. Write the managed file, the
reason, and why upstream cannot carry the fix in the task notes. Then stop at
blocked as described above.

Record `Overlay-Approved-By: <who>, <date>` in the commit message only when a
person wrote that approval in the task notes. Never infer approval from a task
brief.

## Verifying claims

Every claim about the Rex provider, RPP v0, or gc behavior needs two sources.
Check it before it lands in code, docs, task notes, or a review. The provider
sources are listed in `internal/provider/AGENTS.md`.

- Cite file and line (gc) or section and verdict (parity doc) for each claim.
- When the two disagree, report the conflict instead of picking one. In an
  autonomous run, repeat both readings in the task notes and in the PR body.
- Label a claim `unverified` when neither source settles it. Do not round it up
  to fact.
- Change a parity verdict only after re-running the check against both sources.

When a source is absent (CI, a sandbox, a machine with no gc checkout), do not
block on the read. Mark the claim `unverified`. Say which source was missing.
Then continue. The PR reviewer sees the gap.

## Test-driven development

Work test-first. For a bug, write the test that reproduces the bug before the
fix. Watch it go red. Then write the fix and watch it go green. For a feature,
write one test at the chosen seam. Then write enough code to pass it. Then take
the next slice. Do not write all tests up front.

Every bug fix and behavior change lands with the test that proves it. Proof
means a named test that fails without the change and passes with it. Cite that
test in the task notes, commit, or PR. `make test` must pass before the work is
called done.

An existing test may be the proof when it already fails for the bug. Name it.

The provider and rex e2e suites run against the in-repo fake `rex`, so they
need no Rex install. The fake's coverage and the live-server escape hatch are
in `internal/provider/AGENTS.md` and `internal/rex/AGENTS.md`.

Docs, comments, formatting, and script wiring do not need a new test when the
behavior does not change. Say "no behavior change" in the notes. Name the check
that covers it: `pack/tests/verify.sh`, `actionlint`, or `shellcheck`.

When the environment cannot run the test (no rex binary, no gc on PATH), say so
in the notes and make CI run it. Do not write a test that cannot fail.

## Git hooks

Commit hooks are versioned in `.githooks/`. Enable them once per clone:

```sh
make hooks
```

That sets `core.hooksPath=.githooks`. The `pre-commit` hook runs
`golangci-lint` on the Go files staged for commit. It blocks the commit on
findings. In a fresh clone or CI sandbox, run `make hooks` before the first
commit. A git worktree shares the repo config, so it usually needs no second
run.

Do not bypass a hook with `--no-verify` to get past a finding. Fix the finding.
Use `--no-verify` only when the hook itself is broken. Record why in the task
notes.

`.golangci.yml` pins the enabled linters (errcheck, govet, ineffassign,
staticcheck, unused) and the exclusion policy. Run the whole repo by hand with:

```sh
golangci-lint run ./...
```

## Build workflows

Pick the entrypoint from the artifact that already exists. Do not rerun an
approved upstream stage. A continuation entrypoint starts at its own stage and
hands off downstream.

- Idea only: `build-basic`, or `superpowers-build` for the Superpowers skills.
- Requirements written: `build-from-requirements`.
- Requirements approved: `build-from-plan`.
- Plan approved: `build-from-decompose`.
- Implementation convoy ready: `build-from-convoy`, or `implement` for a
  convoy-only drain.
- Implementation evidence ready: `build-from-review`.
- One bead: plain `gc sling`. A convoy of beads: `implement`.
- GitHub issue: `github-issue-fix`. Diff or PR: `review` or
  `github-pr-review`.

Launch only cataloged formulas. The `*-base`, `*-item`, `planning-base`,
`decomposition-base`, `fix-convoy`, and `superpowers-implementation` entries
are internal contracts. A parent formula selects them.

### Always publish

Every finished task ends with a pushed branch and an open PR. A local commit or
worktree does not count as delivery.

Build launches always pass:

    --var push=true --var open_pr=true

Plain `gc sling` work has no publish stage. The worker pushes its branch and
opens the PR before closing the bead. Record the PR URL in the bead.

If the push or PR fails, do not close the bead. Record the failure in the bead
and report it blocked.

### Superpowers gate is label-driven

The design gate is chosen per tracked issue by a GitHub label. The
`issue-build` order launches `superpowers-build` and reads the issue's
`gc/review-brainstorm` label:

- label present -> `--var brainstorming_approval_mode=interactive` and
  `--var interaction_mode=interactive`. Boris reviews the design and the
  written spec before the build continues.
- label absent -> `--var brainstorming_approval_mode=autonomous` and
  `--var interaction_mode=autonomous`. The design auto-approves and the rest
  of the build runs unattended.

This rule governs launches that carry a tracked GitHub issue. Any other
`superpowers-build` launch (a hand-run, or one with no issue) keeps the old
rule: pass `--var brainstorming_approval_mode=interactive` and leave
`interaction_mode` at `interactive`. If a run cannot reach Boris, stop at the
gate and report blocked under "Asking for a decision" above.
