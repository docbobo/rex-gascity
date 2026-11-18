
<!-- BACKLOG.MD GUIDELINES START -->
<!-- backlog.md-instructions-version: 1.55.3 -->
<CRITICAL_INSTRUCTION>

## Backlog.md Workflow

This project uses Backlog.md for task and project management.

**At the beginning of each conversation in this project, run `backlog instructions overview` before answering or taking action. Re-read it only if you have not read it yet in the current conversation.**

Use the overview to decide whether to search, read, create, or update Backlog tasks.

Before task lifecycle actions, read the matching detailed guide:
- `backlog instructions task-creation` before creating or splitting tasks
- `backlog instructions task-execution` before planning, changing status or assignee, adding a plan or implementation notes, or implementing task work
- `backlog instructions task-finalization` before checking acceptance criteria, writing final summaries, or moving tasks to terminal statuses

Use `backlog <command> --help` before running unfamiliar commands. Help shows options, fields, and examples.

Do not edit Backlog task, draft, document, decision, or milestone markdown files directly. Use the `backlog` CLI so metadata, relationships, and history stay consistent.

</CRITICAL_INSTRUCTION>
<!-- BACKLOG.MD GUIDELINES END -->

## Verifying claims

Every claim about the Rex provider, RPP v0, or gc behavior is checked against
two sources before it lands in code, docs, task notes, or a review:

- `docs/provider-herdr-parity.md` — the method-by-method parity record for our
  provider. First source for anything it covers.
- gc source at `/Volumes/Sources/gascity` (`gastownhall/gascity`) — the
  authority on gc behavior. Read the pinned baseline the parity doc names, tag
  `v1.5.1`; the working tree may be ahead of it. Relevant paths:
  `internal/runtime/exec/`, `internal/runtime/protocol.go`,
  `docs/reference/exec-session-provider.md`.

- Cite file and line (gc) or section and verdict (parity doc) for each claim.
- When the two disagree, report the conflict instead of picking one.
- Label a claim `unverified` when neither source settles it; do not round it up
  to fact.
- Change a parity verdict only after re-running the check against both sources.

## Test-driven development

Work test-first. For a bug, write the test that reproduces the bug before the
fix and watch it go red; the fix makes it green. For a feature, one test at the
chosen seam, then enough code to pass it, then the next slice — not all tests
up front.

Every bug fix and behavior change lands with the test that proves it. Proof
means a named test that fails without the change and passes with it; cite that
test in the task notes, commit, or PR. `make test` must pass before the work is
called done.

## Git hooks

Commit hooks are versioned in `.githooks/`. Enable them once per clone:

```sh
make hooks
```

That sets `core.hooksPath=.githooks`. The `pre-commit` hook runs `golangci-lint` on the Go files staged for commit and blocks the commit on findings. Bypass a run with `git commit --no-verify`.

`.golangci.yml` pins the enabled linters (errcheck, govet, ineffassign, staticcheck, unused) and the exclusion policy. Run the whole repo by hand with:

```sh
golangci-lint run ./...
```
