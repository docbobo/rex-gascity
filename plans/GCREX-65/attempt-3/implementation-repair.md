---
{
  "schema": "gc.build.repair.v1",
  "workflow": {
    "id": "gcrex-shev",
    "formula": "build-from-review"
  },
  "methodology": {
    "pack": "gascity",
    "name": "review"
  },
  "producer": {
    "formula": "build-from-review",
    "stage": "repair",
    "attempt": 3
  },
  "status": "repaired",
  "trace": {
    "upstream": [
      {
        "path": "plans/GCREX-65/requirements.md",
        "hash": "sha256:8b632fa0473af6341af3bc685f8a2910cab59cc494dda8bcdf4086f0f171782d"
      },
      {
        "path": "plans/GCREX-65/implementation-plan.md",
        "hash": "sha256:234c8df5445b178963757973d2fbb2beaab25ba080e4c961b707ceab05dc26dc"
      },
      {
        "path": "plans/GCREX-65/plan-review.md",
        "hash": "sha256:9128d290f91cf9f2517b434a4e64dd721a9e48d901b9e633c26c5cf8b603ddd1"
      },
      {
        "path": "plans/GCREX-65/decomposition.md",
        "hash": "sha256:a4713293d0458cdea3bacd2b32c08a4c8b2c02e42c954a92a4f48613bc2fa161"
      },
      {
        "path": "plans/GCREX-65/implementation-summary.md",
        "hash": "sha256:2eb5e8f4afc99ad67316d2d45fe1de4828207e2b130587e763aa768903f53935"
      },
      {
        "path": "plans/GCREX-65/attempt-1/implementation-review.md",
        "hash": "sha256:d03c36c9eb4d7a1b1fad1a04acf68648c828394a2272d1f9650b3bb49210f5c1"
      }
    ],
    "coverage": [
      {
        "id": "REQ-001",
        "status": "covered"
      },
      {
        "id": "REQ-002",
        "status": "covered"
      },
      {
        "id": "REQ-003",
        "status": "covered"
      },
      {
        "id": "REQ-004",
        "status": "covered"
      },
      {
        "id": "REQ-005",
        "status": "covered"
      },
      {
        "id": "REQ-006",
        "status": "blocked",
        "rationale": "SPEC-3: approved skip outcome and its negative proof/reconciliation are not implemented."
      },
      {
        "id": "REQ-007",
        "status": "blocked",
        "rationale": "SPEC-3: approved skip outcome and its negative proof/reconciliation are not implemented."
      },
      {
        "id": "REQ-008",
        "status": "blocked",
        "rationale": "SPEC-3: approved skip outcome and its negative proof/reconciliation are not implemented."
      }
    ]
  },
  "subject": {
    "branch": "gcrex-65-destroy-empty-session",
    "worktree": "/Volumes/Sources/rex-gas-city/worktrees/gcrex-pbcz",
    "base": "2de7a6e10aa0aa28690e3917f815113ba8262c97",
    "head": "1ae24af",
    "diff": "git diff 2de7a6e...1ae24af",
    "repair_diff": "git diff 912f392..1ae24af",
    "dirty": false
  },
  "review": {
    "mode": "agent",
    "fix_attempt_count": 2,
    "max_iterations": 10,
    "findings_addressed": [
      "SPEC-3",
      "STD-A2"
    ]
  },
  "findings": [
    {
      "id": "SPEC-3",
      "axis": "spec",
      "severity": "high",
      "kind": "blocking_plan_drift",
      "requirements": [
        "REQ-001",
        "REQ-005",
        "REQ-006",
        "REQ-007",
        "REQ-008"
      ],
      "summary": "Replacement live evidence triggers the approved global skip branch, but both Stop paths still enable teardown.",
      "locations": [
        "worktrees/gcrex-pbcz/docs/REX-CONTRACT.md:507-512",
        "worktrees/gcrex-pbcz/docs/REX-CONTRACT.md:541,558,594-596",
        "worktrees/gcrex-pbcz/internal/provider/provider.go:733,741"
      ],
      "spec": [
        "plans/GCREX-65/implementation-plan.md:102-105",
        "plans/GCREX-65/implementation-plan.md:205-208",
        "plans/GCREX-65/plan-review.md:72-76"
      ],
      "safety_qualification": "Not evidence of an unsafe production destroy; the existing per-call app guard remains intact."
    }
  ],
  "fix_handoff": {
    "formula": "fix-loop-base",
    "owner_stage": "repair-review",
    "implementation_target": "gc.implementation-worker",
    "implementation_formula": "implement",
    "implementation_item_formula": "do-work-item",
    "restart_entrypoint": "build-from-review",
    "items": [
      {
        "id": "SPEC-3",
        "requirements": [
          "REQ-001",
          "REQ-005",
          "REQ-006",
          "REQ-007",
          "REQ-008"
        ],
        "files": [
          "internal/provider/provider.go",
          "internal/provider/teardown_test.go",
          "internal/provider/e2e_test.go",
          "docs/REX-CONTRACT.md",
          "docs/rex-provider.md",
          "docs/provider-herdr-parity.md",
          "plans/GCREX-65/implementation-summary.md"
        ],
        "action": "Follow the approved skip outcome: disable both Stop teardown invocations, reconcile successful-destroy expectations/docs/summary with that outcome, and preserve truthful live evidence. Do not narrow the app guard or amend the approved plan without explicit authorization.",
        "acceptance": "Named tests for both pane and no-pane Stop branches on an empty canonical session with CLI-only clients must prove zero DestroySession calls: red with current calls, green after disabling them. Reconcile affected tests for the approved skip outcome; make test must pass.",
        "alternative": "Retaining teardown requires an explicitly approved plan amendment addressing the replacement server-wide attachment evidence; repair-review owns any blocked or restart state."
      }
    ],
    "optional_items": [
      {
        "id": "STD-A2",
        "files": [
          "docs/REX-CONTRACT.md"
        ],
        "action": "Correct the probe timezone from verifiable records, or document the actual clock discrepancy; do not fabricate evidence."
      }
    ]
  },
  "repair": {
    "attempt": 3,
    "finding": "SPEC-3",
    "action": "Followed the already-approved skip outcome: removed both handleStop teardown calls (pane present and vanished pane) in internal/provider/provider.go, deleted destroyEmptyCitySession and the destroy transport seam (rex.Transport.DestroySession, CLI command, faketest, teardown tests). No plan amendment was requested or invented.",
    "files_changed": [
      "internal/provider/provider.go",
      "internal/provider/teardown_test.go",
      "internal/provider/teardown_spy_test.go",
      "internal/provider/e2e_test.go",
      "internal/provider/provider_test.go",
      "internal/rex/client.go",
      "internal/rex/cli/cli.go",
      "internal/rex/cli/cli_test.go",
      "internal/rex/faketest/fake.go",
      "docs/REX-CONTRACT.md",
      "docs/rex-provider.md",
      "docs/provider-herdr-parity.md"
    ],
    "notification_scope": "Narrowed documented reach: self-exit-then-Stop leaves the empty canonical session; the next Start adopts it (find-or-create). Recorded in docs/REX-CONTRACT.md \u00a712.",
    "tests": [
      {
        "name": "TestStopCallsNoDestroy",
        "file": "internal/provider/teardown_spy_test.go",
        "result": "pass",
        "covers": "both Stop paths (pane present, vanished pane) on a canonical session; asserts zero DestroySession calls; red when either handleStop destroy call is restored (spy records any call)"
      },
      {
        "name": "TestStopClosesOnlyTheStoppedPane",
        "file": "internal/provider/teardown_test.go",
        "result": "pass"
      },
      {
        "name": "TestStopToleratesVanishedPane",
        "file": "internal/provider/teardown_test.go",
        "result": "pass"
      },
      {
        "name": "TestStopNeverDestroysLegacyLabelledSession",
        "file": "internal/provider/teardown_test.go",
        "result": "pass"
      },
      {
        "name": "TestStartTwiceCreatesNoDuplicatePane",
        "file": "internal/provider/teardown_test.go",
        "result": "pass"
      }
    ],
    "advisory_std_a2": "docs/REX-CONTRACT.md:503-507 now documents the probe clock as the operator's wall-clock reading and the 1-minute gap to commit 912f392 (07:10:44 CET) explicitly; no time was rewritten.",
    "verification": [
      "make test: pass (go test ./... all packages)",
      "golangci-lint run ./internal/...: 0 issues",
      "go build ./...: pass",
      "make test: see .gc/attempt-3 \u2014 see git commit f1bd04c; live e2e not rerun (shared server)"
    ],
    "commit": "f1bd04c (repair) + 1ae24af (STD-A2 doc)",
    "branch": "gcrex-65-destroy-empty-session"
  }
}
---

# GCREX-65 implementation review after repair 1

## Verdict

**changes_required**. Standards: **approved** (no hard violations, one advisory).
Spec: **changes_required** (one blocking plan-drift finding). Both prior findings,
SPEC-1 and SPEC-2, are resolved. SPEC-3 is a newly exposed conflict between the
replacement evidence and the approved plan, not a demonstrated unsafe destroy.

Subject: branch `gcrex-65-destroy-empty-session` in
`/Volumes/Sources/rex-gas-city/worktrees/gcrex-pbcz`, base
`2de7a6e10aa0aa28690e3917f815113ba8262c97`, HEAD
`912f392dad3df238bca111d42334f1da822631ec`. The subject worktree was clean
and HEAD was rechecked after verification. This review changed only this report;
no implementation, test, overlay, PR, or live runtime was modified.

Existing implementation fix-attempt count: **1**, from the supplied integrated
summary's Repair Attempt 1. This is the current workflow's producer/artifact
attempt **1**; the `attempt-2` directory denotes the second implementation review,
not a validator retry. This stage does not increment the repair count.

## Findings

### Standards — approved

No hard documented-standard violations or actionable baseline smells found.
No overlay changes, no new dependency, and no parity-verdict changes.
The transport/provider item summaries name red/green proofs for
`TestDestroySession`, `TestStopDestroysEmptyOwnedSession`, and
`TestStartRecreatesSessionDestroyedBeforeLaunch`. Commit `912f392` names
three live tests that fail with the app guard bypassed and pass restored.
These are author-recorded red/green results; this reviewer independently ran
normal tests, not the destructive mutation proof or live probes.

**STD-A2 — advisory: inconsistent probe timestamp.**
`worktrees/gcrex-pbcz/docs/REX-CONTRACT.md:507` dates the replacement probe
“2026-11-20 07:09 UTC.” Its containing commit `912f392` was authored and
committed at `2026-11-20T07:10:44+01:00` (06:10:44 UTC), before the recorded
observation. Correct the timezone from the real record, or explain the clock
skew. Do not silently invent a time. This is an evidence-provenance advisory
under `AGENTS.md:21-37`, not an implementation safety defect.

### Spec — changes_required

**SPEC-1 resolved (REQ-008).** Attachment is sampled before Stop at
`worktrees/gcrex-pbcz/internal/provider/e2e_test.go:388,579,660`. The assertion
at `worktrees/gcrex-pbcz/internal/provider/e2e_test.go:437-457` rejects both
disappearance and replacement of the originally attached session. Commit
`912f392` cites the red/green bypass-guard proof using
`TestE2EIdentityActivityAttachment`, `TestE2EAgentExitLiveness`, and
`TestE2EParallelStarts`.

**SPEC-2 resolved (REQ-001).**
`worktrees/gcrex-pbcz/docs/REX-CONTRACT.md:515-583` now records executable
commands, valid quoted JSON, and session ls/inspect before and after pane exit,
both destroy responses, and scratch cleanup. The separate unavailable-server
invocation appears at `worktrees/gcrex-pbcz/docs/REX-CONTRACT.md:577-583`.
The elisions in client JSON are explicitly identified; the before-list omission
and malformed-command finding no longer apply. See STD-A2 for timestamp accuracy.

**SPEC-3 — high, blocking plan drift: new verification evidence triggers the
approved skip branch, yet Stop teardown remains enabled.**

The plan explicitly requires: “If step 1's unattended scratch session lists a
`kind==app` client ... do not call `DestroySession` from `stop`”
(`plans/GCREX-65/implementation-plan.md:102-105`). The more specific condition
is that an app is listed when nobody attached it to that scratch session
(`plans/GCREX-65/implementation-plan.md:205-208`); the skip is reaffirmed in
`plans/GCREX-65/implementation-plan.md:399-402,410-419` and
`plans/GCREX-65/plan-review.md:72-76`.

The replacement transcript says the Mac app is attached **to the server**,
therefore appears on scratch too
(`worktrees/gcrex-pbcz/docs/REX-CONTRACT.md:507-512`). Both client-list outputs
show `kind:"app"`
(`worktrees/gcrex-pbcz/docs/REX-CONTRACT.md:541,558`). That satisfies the
approved global skip condition; calling it the “human-attached signal” at
`worktrees/gcrex-pbcz/docs/REX-CONTRACT.md:594-596` does not resolve the
server-wide versus session-specific distinction.

Both Stop branches nevertheless still invoke teardown at
`worktrees/gcrex-pbcz/internal/provider/provider.go:733,741`. The intact guard at
`worktrees/gcrex-pbcz/internal/provider/provider.go:798-801` blocks a particular
call that observes an app. That is not the plan's instruction to omit destruction
from Stop after this verification outcome. The original design and W1 probes
were CLI-only, so this is **newly conflicting acceptance evidence** from repair
`912f392`, not proof that the original implementation disregarded its original
probe. An ordinary later attached human alone would not trigger this global
branch. No production safety bug is demonstrated here.

Affected: REQ-001/005 verification qualification, REQ-006 attachment guard,
REQ-007 alternate-outcome proof, and REQ-008 documentation/test reconciliation.
The following repair stage must follow the approved skip, or obtain an explicit
approved plan amendment. This review grants no such amendment.

## Verification

Independent checks in the subject worktree:

- `make -C worktrees/gcrex-pbcz test`: **pass** (all packages).
- `golangci-lint run ./...`: **pass**, zero issues.
- `go test -race ./...`: **pass**.
- `git diff --check 2de7a6e...HEAD`: **pass**.
- HEAD pin and clean subject worktree: **confirmed** after checks.

Live e2e and live probes were **not independently rerun**. They mutate a shared
Rex server; this review uses the committed transcript and recorded implementation
proof instead. Default make test skips opt-in live tests. The reported red/green
repair evidence is from commit `912f392`, not this review's execution.

Standards and Spec reviews ran independently read-only. Initial Spec confirmed
both repairs; a focused independent Spec follow-up adjudicated the previously
missed global skip condition and returned SPEC-3. All three delegates completed
with usable results. Axes remain separate; findings are not reranked across them.

### Two-source qualification

Checked `worktrees/gcrex-pbcz/docs/provider-herdr-parity.md` and gc source at
`/Volumes/Sources/gascity`, tag **v1.5.1**, peeled commit
`4f698fe2a7eefae49e545c5e38189f1b63fb9b39`, not its newer working tree.

- Parity **§stop, row 3 and overall verdict: intentional** records idempotent
  named teardown; gc `internal/runtime/exec/exec.go:580-583` at v1.5.1 delegates
  to the pack's named Stop. Neither settles native Rex client-list scope.
- Parity **§server-stop, verdict: intentional** preserves the shared-server
  boundary; gc `internal/runtime/runtime.go:533-546` and
  `cmd/gc/cmd_stop.go:465-472` at v1.5.1 describe optional server teardown,
  not native Rex session destruction. No parity verdict is changed here.
- **Unverified by both mandated sources:** native Rex server-wide versus
  session-specific attachment and CLI destruction semantics. SPEC-3 rests on
  the explicit approved plan and supplied live evidence, not invented gc parity.
  The replacement evidence conflicts with the design's assumption that a listed
  app establishes attachment to this particular scratch session. Report the
  conflict; do not select the narrower interpretation as fact.

### Drift and coverage

No new scope drift in canonical-only ownership, Stop-only triggering, or the
accepted TOCTOU window. Self-exit with no lifecycle Stop is the approved non-goal.
The integrated summary now correctly identifies the source worktree and repair
commit; the older review's W2-only-summary observation is superseded.

| ID | Status | Evidence or gap |
| --- | --- | --- |
| REQ-001 | covered | Complete replacement live transcript; global qualification conflict is SPEC-3. |
| REQ-002 | covered | D3 ownership rule and creation-site citation. |
| REQ-003 | covered | Canonical-only predicate; legacy and duplicate-label guard tests. |
| REQ-004 | covered | Fresh empty-view check, accepted TOCTOU, and intended recreation documented. |
| REQ-005 | covered | Destroy responses/exit codes and transport test; attachment-scope assumption conflicts with plan. |
| REQ-006 | blocked | SPEC-3: per-call guard remains, but required verification-triggered skip is absent. |
| REQ-007 | blocked | Normal proofs/tests pass; global skip outcome needs named red/green negative tests. |
| REQ-008 | blocked | Prior e2e defect fixed; docs/test expectations still describe teardown enabled after skip evidence. |

## Structured fix handoff

Machine-readable handoff is in front matter `fix_handoff`.
Owner: **repair-review**. Formula: **fix-loop-base**. Target:
**gc.implementation-worker**; implementation formulas **implement** and
**do-work-item**. Existing fix count **1**, maximum **10**. Restart entrypoint:
**build-from-review**. No repair was dispatched by this review.

Selected repair SPEC-3: follow the already-approved skip outcome. Disable both
Stop teardown invocations, reconcile successful-destroy tests and documentation
with that outcome, and retain honest transport/live evidence. Do not narrow the
app guard, fabricate a CLI-only probe, or silently rewrite the approved decision.

Acceptance: named regressions for **both pane and no-pane Stop paths**, on an
empty canonical session with **CLI-only clients**, prove **zero DestroySession
calls**. They fail with the current invocations and pass after disabling them.
Reconcile all affected positive expectations with the approved negative outcome;
`make test` must pass. If the repair owner elects to retain teardown instead,
it needs an explicit approved plan amendment addressing this evidence; record
blocked/restart state when authorization is absent, without idling for questions.
STD-A2 is optional provenance cleanup, not an additional approval gate.

**Axis totals:** Standards **0 blocking + 1 advisory** (worst: timestamp
provenance); Spec **1 blocking** (worst: SPEC-3 global skip drift).
