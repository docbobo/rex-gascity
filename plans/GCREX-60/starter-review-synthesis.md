# Starter review synthesis — GCREX-60

**Fix-lane verdict: iterate.** Two required fixes. Test evidence approved the recorded suite; that approval does not clear the mail-path gaps below.

Do not edit the launcher root `/Volumes/Sources/rex-gas-city`. It has no implementation. `gc.work_dir` was unset on `gcrex-140`; the launcher rig root is contrast only.

## Where to edit

| Source anchor | Worktree | Owns |
| --- | --- | --- |
| `gcrex-hvlf` | `/Users/boris/my-city` (recovered; HEAD `4c1318c86d80aadd96dcb4e85f25c0d85ee9c178`) | both required fixes |
| `gcrex-72t8` | `/Users/boris/my-city` | no finding. Leave the uncommitted `orders/rex-backlog.toml` comment and the operator `enabled = false` hunk untouched |

Before editing: `cd /Users/boris/my-city` and check `pwd -P` is that path. Resolve every path below against that worktree.

## Required fixes

### RF-1 — A failed send must not look like success

Lanes: acceptance **A1** (both helpers); simplicity **SIM-001** (blocked helper only). Same defect.

- `/Users/boris/my-city/assets/scripts/rex-backlog-blocked.sh:72-77` — `gc mail send human --dedup rex-backlog-blocked ... || true`, then `exit 0`.
- `/Users/boris/my-city/assets/scripts/rex-backlog-noop.sh:33-36` — same `|| true`. Stamp is already written at line 27. Lines 39-44 then close `gc.outcome=pass`.

Do this, and nothing broader:

- Blocked: delete `|| true` so the script's `set -e` keeps the send's nonzero status. No retry framework. No shared notifier.
- No-op: delete `|| true` so a failed send does not reach the pass/no-op close. Keep the line 27 stamp (backoff stays). On a successful send, keep today's pass/no-op close. Do not change success-path bead-close or backoff.
- A failed send creates no live dedup message, so the next fire can retry. That is the pending-notification state. Do not add a retry subsystem.

Required tests, through a failing fake `gc` on `PATH`, not `REX_BACKLOG_MAIL_LOG`:

- `cd /Users/boris/my-city && python3 /Volumes/Sources/rex-gas-city/plans/GCREX-60/acceptance-probes.py` — today `MAIL_FAILURE_noop` and `MAIL_FAILURE_blocked` exit 0. Must exit 0 only after both helpers fail closed.
- `bash /Volumes/Sources/rex-gas-city/plans/GCREX-60/simplicity-mail-failure.check.sh` — expected failure today; must pass after the blocked edit.
- While on the production send branch, assert recipient `human` and the dedup key (simplicity **SIM-A01**, acceptance A2 aside for the no-op key). No-op key stays `rex-backlog-noop`. Do not add no-op episode state; no probe showed a no-op episode bug.

### RF-2 — One mail per blocked episode

Lane: acceptance **A2** only. Simplicity did not require this. Test evidence did not probe it.

File: `/Users/boris/my-city/assets/scripts/rex-backlog-blocked.sh` in worktree `/Users/boris/my-city`, anchor `gcrex-hvlf`.

- Lines 41-43 return on an empty, ready, or busy board and reset nothing.
- Lines 72-74 always send `--dedup rex-backlog-blocked`.

Pinned gc `v1.5.1` dedup is the live lifetime of a message, not a board episode (`/Volumes/Sources/gascity/internal/mail/mail.go:90-110`). A fixed key cannot do both of the following. The requirements text that says `--dedup` means one mail per episode is the conflict to reconcile in the fix evidence. Do not claim the constant key already implements episodes.

Smallest change that passes both probes:

- Persist "this blocked episode was notified" next to the existing no-op stamp. Clear it on the not-blocked exit at lines 41-43.
- If that marker is set, do not send again (archive/recheck of the same episode stays at one mail).
- Put an episode token in the dedup key so a still-live message from the previous episode cannot suppress the next one. Keep `--dedup`. Do not leave the bare key `rex-backlog-blocked` if `EPISODE_RESET` still fails.

Proof, same acceptance probe, no real mail:

- `EPISODE_RESET` (blocked, then ready, then blocked): deliveries=1, expected=2.
- `ARCHIVE_SAME_EPISODE` (archive, then rerun while still blocked): deliveries=2, expected=1.

## Missing evidence

### ME-1 — Production send arguments are untested

Lanes: acceptance A1/A2; simplicity **SIM-A01**.

`/Users/boris/my-city/tests/rex-backlog-blocked.test.sh:60` and `/Users/boris/my-city/tests/rex-backlog-noop.test.sh:38` set `REX_BACKLOG_MAIL_LOG`, so they never reach `/Users/boris/my-city/assets/scripts/rex-backlog-blocked.sh:72` or `/Users/boris/my-city/assets/scripts/rex-backlog-noop.sh:33`. Green suites do not prove recipient, dedup key, or send failure. Close this by the RF-1 tests above. No separate product change.

## Residual risks

Do not block on these. Do not send real mail or touch the live board.

- **RR-1** (simplicity SIM-A01). After the RF-1 fake-`gc` tests exist, the `REX_BACKLOG_MAIL_LOG` branches in the two scripts (blocked lines 68-70, no-op lines 29-31) can be deleted. Leaving them is safe if the new tests cover the real branch.
- **RR-2** (acceptance A2). The deployed mail provider/version was not probed. Dedup behavior above is pinned `v1.5.1` only. Unverified against the running provider.
- **RR-3** (test evidence, bead `gcrex-jve`). Convoy item `gcrex-hvlf` is `deferred` in the implementation summary because it stayed open. The proof is in the summary and was reproduced. No code change.
- **RR-4**. Old-gate replay counts differ (acceptance isolated replay 8/2 after path parameterization; test evidence and simplicity 6/4 from `git show 4c1318c~1:scripts/rex-backlog-ready.sh`). Both saw the named "blocked labeled To Do" case fail on the old gate and pass on HEAD. Not a product defect.

## Lane verdicts

| Lane | Bead | Verdict |
| --- | --- | --- |
| Acceptance | `gcrex-3fc` | iterate — A1, A2 |
| Simplicity | `gcrex-qrj` | iterate — SIM-001; SIM-A01 non-blocking |
| Test evidence | `gcrex-jve` | approve — `make test` 27/27, red/green gate, typo grep. Does not cover production send failure or episode reset |

## After the fixes

From `/Users/boris/my-city` with `pwd -P` matching:

1. `python3 /Volumes/Sources/rex-gas-city/plans/GCREX-60/acceptance-probes.py` exits 0.
2. `bash /Volumes/Sources/rex-gas-city/plans/GCREX-60/simplicity-mail-failure.check.sh` exits 0.
3. `make test` still passes (27, or more if the new cases live in the suites).

Out of scope: launcher checkout, overlays, re-enabling the order, relabeling prerequisites, gc suppression, and the operator `enabled = false` hunk.
