# CodeQL evaluation for rex-gascity

Status: proposal. Decision owner: Boris.
Bead: `gcrex-6pk2r`.

**Recommendation: adopt, advisory only, Go and Actions only.** One file,
`.github/workflows/codeql.yml` (in this PR). Do not add the check to the
ruleset yet. Re-decide after four weeks of measured alerts.

## How this was checked

- gascity's workflow at the pinned baseline, tag `v1.5.1`, and at HEAD
  (`890a58a3eddb14393f9d5abbb11025b9e979e2c7`, branch `edge`). The two differ.
- Live GitHub API, read-only: rulesets, workflow runs, branch protection.
- CodeQL build modes and language support: GitHub Docs, linked below.

Claims about gc behaviour cite the file and line in the pinned checkout. The
one place where tag `v1.5.1` and HEAD disagree is called out in
"Gating or advisory".

## 1. What CodeQL adds beyond the gates we run

Today's gates, from `.github/workflows/ci.yml` and `.golangci.yml`:

| Gate | Covers | Does not cover |
| --- | --- | --- |
| `go vet` | print-format, copies, build tags | dataflow, untrusted input |
| `golangci-lint` v2.15.0, with only errcheck, govet, ineffassign, staticcheck and unused enabled | unchecked errors, dead code | dataflow, security patterns |
| `go test`, `pack/tests/verify.sh`, fake-backed e2e, conformance | behaviour | source security |
| `release.yml` | build and asset upload | source security |

Three gaps matter:

1. **No taint or dataflow analysis.** Nothing follows a value from a protocol
   or config field to a sink. This repository starts shells and turns names
   from another process into filesystem paths.
2. **No workflow analysis.** `.github/workflows/` runs shell commands from
   inputs. Nothing lints it.
3. **No `govulncheck`.** gascity has `.github/workflows/govulncheck.yml`.
   We do not. A grep over `.github/`, `Makefile` and `pack/tests/` finds no
   reference. The gap is real but has near-zero value today. `go.mod` has no
   `require` block and there is no `go.sum`, so no third-party module can
   match an advisory.

### Query classes that can matter here, with evidence

| Query | Evidence in this tree | Verdict |
| --- | --- | --- |
| `go/command-injection` | `internal/provider/provider.go:1304` runs `exec.CommandContext(ctx, "/bin/sh", "-c", command)` in `runPreStart`. The `command` argument is `cfg.PreStart[i]`, and `opStart` (`cmd/gc-runtime-rex/main.go:415`) fills that field by JSON-decoding the `pre_start` array from the start op's stdin (`startConfig.PreStart`, `main.go:154`, copied at `main.go:433`). gc v1.5.1 populates the field from agent config plus engine-generated hooks (`cmd/gc/session_lifecycle_parallel.go:1085-1089,1501-1538`) and sends it in the same JSON (`internal/runtime/exec/json.go:35,59` and `docs/reference/exec-session-provider.md:230,328-333`). | The only high-value query. Pack/city config is one upstream source, not the only one: the value reaches the sink over the protocol message, so a bead or agent field that reaches `pre_start` would arrive by the same path. No alert is expected today, but not because the source is trusted config: the value crosses a process boundary as stdin JSON, so CodeQL sees no same-process source-to-sink path from the `exec.CommandContext` call. Whether CodeQL models `os.Stdin` as a remote source is `unverified`. An alert would appear only for a future in-process path that passes a value straight to `runPreStart`. |
| `go/path-injection` | 1218 `filepath.Join` uses in first-party Go. State and lock files sit under the user's city dir, for example `internal/provider/lock.go:120` and `:163` (mode `0o644`). | Medium. Session and block identifiers arrive over the protocol. This is the query most likely to give a first alert. |
| `go/clear-text-logging` | 406 `os.Getenv` uses. Trace and state files record argv and environment. | Medium and worth having. Env values may carry tokens. |
| `go/log-injection` | no network log sink | Low. |
| `go/weak-crypto` | no `crypto/md5`, no `crypto/sha1`, no `InsecureSkipVerify` (the grep is empty) | Low. |
| `go/ssrf` | no `net/http`, no `http.Client` in first-party code (the grep is empty) | Skip. |
| `actions/*` | 2 workflows. No `${{ github.event.* }}` or `${{ github.head_ref }}` inside a `run:` block today. | Backstop. Cheap: the `actions` leg takes 33 s in gascity. |

Honest verdict: expect few findings. The value is a dataflow backstop. This
code spawns shells and handles names from another process. CodeQL will not find
a bug on day one.

## 2. Which query suite gascity runs

Default. Not `security-extended`.

Evidence: `.github/workflows/codeql.yml` at `v1.5.1` calls
`github/codeql-action/init` with only `languages` and `build-mode` (lines
66-71). There is no `queries:` input. The `v1.5.1` tree holds one CodeQL
file, `.github/workflows/codeql.yml`, plus an unrelated release-gate note
(`release-gates/ga-x1ogtd-native-beads-store-codeql-fix-gate.md`). There is
no `codeql-config` file, no custom query pack, and no `paths-ignore`. A grep
for `security-extended` and `security-and-quality` over the `v1.5.1` and
HEAD trees returns nothing. `.github/CODEOWNERS` exists but sets no CodeQL
configuration.

Matrix (`.github/workflows/codeql.yml:51-60` at `v1.5.1`): `actions` with
build-mode `none`, `go` with `autobuild`, `javascript-typescript` with
`none`, and `python` with `none`. Runner: 16 vCPU, `timeout-minutes: 30`
(line 48). Permissions (lines 17-20): `actions: read`, `contents: read`,
`security-events: write`.

## 3. Gating or advisory

Gating, at two levels.

- Classic branch protection on `main` is absent. The API returns 404 with
  "Branch not protected".
- Ruleset `main` (id `14017226`, live API) has a status-check rule. It lists
  `Analyze (actions)`, `Analyze (go)`, `Analyze (javascript-typescript)`
  and `Analyze (python)` besides `Check`, `bazel test (side-by-side)`,
  `BUILD files in sync` and `storeclass baseline and growth gate`. A red
  Analyze leg blocks the merge.
- The same ruleset has a `code_scanning` rule. Tool: `CodeQL`. The alert
  threshold is `errors_and_warnings` and the security alert threshold is
  `high_or_higher`. A new alert at or above those thresholds blocks the pull
  request even when every check is green. This rule makes findings gate. The
  check status alone does not.

Recorded conflict, per the two-source rule. The bead context says the workflow
triggers on `merge_group (checks_requested)` and that a comment declares the
four Analyze checks as needed. That is true at HEAD only. Tag `v1.5.1` has no
`merge_group` trigger and no such comment. Its `on:` block is lines 3-15.
HEAD adds both, 84 insertions against `v1.5.1`. `TESTING.md:400` and
`:430` at HEAD describe the merge-group behaviour the bead describes. The live
ruleset lists the checks either way. Use HEAD for present-day behaviour and
`v1.5.1` only for the stored baseline.

## 4. Alert history

**Unverified.** The call
`gh api repos/gastownhall/gascity/code-scanning/alerts` returns 403 "You are
not authorized to read code scanning alerts" for the `docbobo` token. The
token needs the `security_events` scope. No public alert summary was found to
substitute. Alert counts by state and severity stay unknown. So does the answer
to "how many of these did a human act on".

## 5. Cost

Measured on gascity, on a Blacksmith 16 vCPU runner, from the runs API:

- One representative PR run (id `45338703389`, 2026-11-24). Runner policy
  11 s. Analyze (go) 114 s, Analyze (javascript-typescript) 49 s, Analyze
  (python) 39 s and Analyze (actions) 33 s, all in parallel. Wall clock 2 min
  20 s.
- Last 40 runs. PR runs have a median of 145 s (range 124-191 s). Pushes to
  `main` take 404 s and 490 s, because a push saves the Go caches.
  `merge_group` runs take 35 s and 40 s, because they report without
  analyzing. Three PR runs took 0 s with conclusion `action_required`
  (first-time contributor approval).

Normalised for our repository: gascity has 4416 Go files and 68.2 MB of Go
source at `v1.5.1`. `rex-gascity` at `origin/main` has 128 files and
1.28 MB, about 53x smaller by bytes. The Go extractor cost follows source size.
Expect well under a minute of extraction on 16 vCPU. Expect roughly 2-4 min on
`ubuntu-latest` with 2 vCPU. Two languages, one matrix, no runner-policy job
and no cache steps: the workflow in this PR is 55 lines against gascity's 162.

Budget: `docbobo/rex-gascity` is public (from `gh repo view`), so CodeQL
minutes are free. The real cost is PR latency and one more red-or-green row for
a human to read.

The estimate is not a measurement. The first run of this PR's workflow is the
measurement. Check its duration before deciding on gating.

## 6. Recommendation and plan

Adopt now, advisory. The repository is public, so the minutes are free. The
workflow is 55 lines. The missing capability is dataflow plus workflow
analysis, which matches this codebase.

- **Languages:** `go` and `actions`. Go has no `none` build mode. The
  `database create` reference lists `none` for C#, Java, JavaScript and
  TypeScript, Python, and Ruby only. So `autobuild` is the only non-manual
  choice. Drop `javascript-typescript` and `python`: no such source exists
  here.
- **Triggers:** `pull_request` (opened, reopened, synchronize,
  ready_for_review), `push` to `main`, a weekly schedule, and
  `workflow_dispatch`. No `merge_group`: `rex-gascity` has no merge queue.
  Ruleset `27750059` has no merge-queue rule.
- **Suite:** default. Switch to `security-extended` only if the first month
  shows findings worth the extra runtime.
- **Gating:** advisory first. Do not touch ruleset `27750059`. It lists only
  `Conformance` and `pack-verify` today. After four weeks, read the alert
  count. Then decide whether to add `Analyze (go)` as a status check, or a
  `code_scanning` rule like gascity's. A `code_scanning` rule is the one
  that makes findings block. A status check alone does not.
- **File:** `.github/workflows/codeql.yml`, added in this PR.

Follow-ups, out of scope here:

- No `govulncheck` job exists. Separate bead. Near-zero value while
  `go.mod` has no dependencies.
- If alert noise appears, add a `codeql-config` with `paths-ignore` instead
  of per-alert dismissals.

## Sources

- `gastownhall/gascity` at tag `v1.5.1`, `.github/workflows/codeql.yml`
  lines 1-71 and `.github/workflows/govulncheck.yml`. Pinned local checkout
  `/Volumes/Sources/gascity`.
- `gastownhall/gascity` at HEAD `890a58a3eddb14393f9d5abbb11025b9e979e2c7`,
  same file (merge-group trigger and skip step), and `TESTING.md:400-435`.
- GitHub API, read-only: `repos/gastownhall/gascity/rulesets/14017226`,
  `repos/gastownhall/gascity/actions/runs/45338703389/jobs`,
  `gh run list --workflow codeql.yml`, and
  `repos/docbobo/rex-gascity/rulesets/27750059`.
- GitHub Docs, [CodeQL build options for compiled
  languages](https://docs.github.com/en/code-security/reference/code-scanning/codeql/build-options-for-compiled-languages)
  and [`codeql database
  create`](https://docs.github.com/en/code-security/reference/code-scanning/codeql/codeql-cli-manual/database-create)
  for build modes per language.
- `rex-gascity` at `origin/main` `15c387e`: `.github/workflows/ci.yml`,
  `.github/workflows/release.yml`, `.golangci.yml`, `go.mod`,
  `internal/provider/provider.go:1304`, `internal/provider/lock.go:120,163`.
