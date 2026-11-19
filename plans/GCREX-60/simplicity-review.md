# GCREX-60 simplicity and maintainability review

**Verdict: iterate** — one required fix, one related non-blocking test simplification.

## Review scope

- Claimed review lane: `gcrex-qrj` (simplicity only).
- Authority: `/Volumes/Sources/rex-gas-city/plans/GCREX-60/build-basic-review-context.md`, **Implementation Worktrees**.
- Implementation: `/Users/boris/my-city`, verified with `pwd -P`; HEAD `4c1318c86d80aadd96dcb4e85f25c0d85ee9c178`.
- Reviewed the ten-file committed change against `4c1318c~1`, plus the uncommitted comment correction in `/Users/boris/my-city/orders/rex-backlog.toml:6`.
- Left the implementation unchanged. The operator's `enabled = false` and unrelated dirty files are not review defects or part of this change. No launcher source inspection, overlay edits, or subagents.

## Required finding

### SIM-001 — Do not report a failed blocked-board notification as success

**Location:** `/Users/boris/my-city/assets/scripts/rex-backlog-blocked.sh:72-77`.

The send command ends with `-m "$body" || true`, followed by `exit 0`. Consequently, even when the command fails, the detector reports successful completion. This alert is the script's only externally useful action on a blocked board; hiding its failure makes a notification problem look like a healthy check and makes diagnosis needlessly difficult.

**Evidence:** The runnable check below substitutes a fake `backlog` reporting one blocked task and a fake `gc` whose send invocation exits 73. It uses the normal notification branch, never sends real mail, and never calls a real bead command. The detector still exits 0:

```text
injected mail transport failure
Blocked detector exit after injected send failure: 0
FAIL: blocked alert delivery failure is reported as success
```

**Smallest useful fix:** Remove `|| true` from this detector's mail-send command so its existing `set -e` preserves a nonzero result. Add a failure-case test using a fake `gc` on `PATH`. No retry framework or shared notification abstraction is needed. This finding does not require changing the no-op helper's bead-close or backoff behavior.

**Proof command:**

```bash
bash /Volumes/Sources/rex-gas-city/plans/GCREX-60/simplicity-mail-failure.check.sh
```

The check currently fails as expected; it should pass after the required fix. This evidence establishes shell error propagation with an injected command failure, not any unverified assertion about the real gc mail implementation, dedup lifetime, or order-history semantics.

## Non-blocking simplification

### SIM-A01 — Test the notification command, not a second implementation of it

**Locations:** `/Users/boris/my-city/assets/scripts/rex-backlog-blocked.sh:68-75`, `/Users/boris/my-city/assets/scripts/rex-backlog-noop.sh:29-37`, `/Users/boris/my-city/tests/rex-backlog-blocked.test.sh:60`, `/Users/boris/my-city/tests/rex-backlog-noop.test.sh:38`.

Both notification suites set `REX_BACKLOG_MAIL_LOG`, which takes a logging-only branch instead of invoking the production send command. The checks therefore remain green if the production send line, recipient, or dedup argument is broken. The no-op suite already supplies a fake `gc` for bead calls.

Reuse that command-boundary approach for mail: unset `REX_BACKLOG_MAIL_LOG`, record fake `gc` arguments, and assert the recipient and dedup key. For the detector, add a similarly small fake command. Once those tests cover the real branch, the test-only logging branches can be deleted. Do not introduce a shared test framework for these small suites.

## Verification

Executed from the verified implementation worktree:

- `make test`: **27 checks passed**, zero failures (blocked 7, no-op 10, ready 10).
- Copied the ready suite and `git show 4c1318c~1:scripts/rex-backlog-ready.sh` into a temporary tree: **6 passed / 4 failed**, including `blocked labeled To Do (todo=1, ready=0) does not fire (want exit 1, got 0)`. The same named regression passes in the current suite. No implementation file was replaced.
- `simplicity-mail-failure.check.sh`: **expected failure**, reproducing SIM-001 through the real send branch with a stubbed command.
- Shell LSP diagnostics were unavailable because no language server is configured for this file type; the runnable check executed successfully and reached its intended failing assertion.

## Maintainability assessment

The readiness gate's two-use `age_ok` helper and environment calibration knobs are justified, and extracting the no-op close into one script is a readable boundary. The Makefile is appropriately small. No new library, general-purpose abstraction, or unrelated launcher change is needed to address the required finding.
