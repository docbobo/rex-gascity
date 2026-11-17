
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
