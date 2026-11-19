## Overlay changes need approval

Every file under an `overlay` directory — this pack's `pack/overlay/`, any
`pack_overlay_dirs` entry a city configures, and their
`per-provider/<provider>` subtrees — MUST be approved by Boris before it lands.
Adding, editing, moving, or deleting one of those files needs that approval, and
so does adding an overlay directory itself.

An overlay is a patched copy of a file someone else owns. The runtime stages it
over its managed file, it drifts as that managed file changes, and it can defeat
the runtime's own preserve/upgrade predicate:
`pack/overlay/per-provider/pi/.pi/extensions/gc-hooks.js` pins
`GC_PI_HOOK_VERSION = 100` precisely to keep gc from reverting it. So an overlay
is a managed exception, not a normal edit. The default answer is an upstream fix
in the owner's repo; an overlay needs a reason that fix cannot wait.

Present the change to Boris with the diff, the managed file it overrides, and
why upstream cannot carry it. Record the approval in the task notes and the
commit message (`Overlay-Approved-By: <who>, <date>`).

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
