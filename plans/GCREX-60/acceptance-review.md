# GCREX-60 acceptance and correctness review

- Claimed review bead: `gcrex-3fc`; workflow root: `gcrex-140`.
- Acceptance verdict: **iterate** (two required correctness fixes).
- Execution outcome: **pass** — the review completed; findings are review output.
- Authority: `/Volumes/Sources/rex-gas-city/plans/GCREX-60/build-basic-review-context.md`, **Implementation Worktrees**.
- Reviewed implementation: `/Users/boris/my-city`, HEAD `4c1318c86d80aadd96dcb4e85f25c0d85ee9c178`, plus the uncommitted comment correction in `orders/rex-backlog.toml`.
- Source anchors recorded by context: `gcrex-hvlf` (verification) and `gcrex-72t8` (comment correction). Missing anchor metadata/item summaries are disclosed setup warnings, not evidence that the implementation is missing.

Commands began with `cd /Users/boris/my-city` and verification that `pwd -P` equals that path. The launcher checkout was not inspected as implementation code. No implementation files, index, history, live board, mail, or implementation state were mutated. Review probes use isolated temporary state and fake CLIs; outputs live under the build artifact root.

## Required findings

### A1 — Major: notification failures are silently converted to success

**Requirement/task:** REQ-002 / AC-2 (no-op mails the human and closes pass/no-op), REQ-003 / AC-3 (blocked board is reported to the human); implementation plan §2–3; verification item `gcrex-hvlf`.

**Source evidence:**

- `/Users/boris/my-city/assets/scripts/rex-backlog-noop.sh:33-43`: the real `gc mail send` ends in `|| true`; the script then records `gc.outcome=pass` and closes the step.
- `/Users/boris/my-city/assets/scripts/rex-backlog-blocked.sh:72-77`: the real send likewise ends in `|| true`, followed by successful exit.

**Runnable proof:**

```sh
cd /Users/boris/my-city
[ "$(pwd -P)" = /Users/boris/my-city ] && \
  python3 /Volumes/Sources/rex-gas-city/plans/GCREX-60/acceptance-probes.py
```

The fake mail CLI returns exit 1 and `fake mail delivery failed`. Actual results:

```text
MAIL_FAILURE_noop: exit=0, closed_pass=True, stderr=fake mail delivery failed
MAIL_FAILURE_blocked: exit=0, closed_pass=False, stderr=fake mail delivery failed
```

The no-op stamp is already written at `/Users/boris/my-city/assets/scripts/rex-backlog-noop.sh:27`, so the human can receive no notification while the dispatch is closed as successful and the gate backs off. The blocked detector also reports success without delivering its only requested signal.

**Smallest required fix:** stop discarding real mail errors. Expose/record notification failure and preserve enough pending-notification state for retry, without losing the backoff or falsely claiming a successful notification. Add named tests through a failing fake `gc mail send`, not the mail-log bypass, for both helpers. Preserve the successful pass/no-op close behavior when delivery succeeds.

**Pinned authority check:** `/Volumes/Sources/gascity/cmd/gc/cmd_mail.go:1919-1923` at tag `v1.5.1` returns 1 on a send error. `/Volumes/Sources/rex-gas-city/docs/provider-herdr-parity.md`, **Coverage** and **gc baseline**, was checked first; mail is outside its provider-method coverage and has no mail verdict. No conflict or parity-verdict change. This finding is proved by the implementation's shell control flow and isolated failing-CLI probe, not an assumption about the deployed mail service.

### A2 — Major: fixed-key mail deduplication is not once per blocked episode

**Requirement/task:** REQ-003 technical story requires “one mail per blocked episode”; AC-3 requires notification naming blocked work; implementation plan §3; verification item `gcrex-hvlf`.

**Source evidence:** `/Users/boris/my-city/assets/scripts/rex-backlog-blocked.sh:41-43` returns on a healthy/busy board without resetting any episode state; `/Users/boris/my-city/assets/scripts/rex-backlog-blocked.sh:72-74` always uses the same `rex-backlog-blocked` key. There is no persisted episode/sent marker.

**Runnable proof:** the same acceptance probe models the pinned mail dedup rule (one live message per recipient/key), without sending actual mail:

```text
EPISODE_RESET: blocked -> ready -> blocked; deliveries=1, expected=2
ARCHIVE_SAME_EPISODE: archive then rerun; deliveries=2, expected=1 total for this continuing episode
```

These are independent scenarios. An unarchived previous notification suppresses a later episode, potentially hiding newly blocked work. Archiving the notification while the board remains blocked permits a second mail for that same episode.

**Smallest required fix:** explicitly track blocked-episode identity and whether that episode was notified; reset on a ready, empty, or busy board. Use episode-aware deduplication so an old live message cannot suppress a new episode, and test both board recovery/reblocking and archive/recheck behavior. The plan's requirement to preserve the exact fixed dedup key conflicts with strict episode semantics under the pinned API; reconcile that constraint in the fix evidence rather than claiming a fixed stream key already implements episodes. The analogous fixed-key assertion for no-op episodes is not independently proven by the existing suite either.

**Pinned authority check:** `/Volumes/Sources/gascity/internal/mail/mail.go:90-110` at `v1.5.1` explicitly defines the dedup horizon as the message's live lifetime, not a board episode; archiving permits re-alert. `/Volumes/Sources/gascity/internal/mail/beadmail/beadmail.go:195-239` probes open messages with the same key and mailbox. `/Volumes/Sources/rex-gas-city/docs/provider-herdr-parity.md`, **Coverage** / **gc baseline**, does not cover mail dedup (no applicable verdict). The pinned mail source settles the API behavior; the current deployed provider/version was not probed and remains **unverified**. No conflicting parity claim was found.

## Acceptance evidence and scope

| Requirement | Result | Evidence |
| --- | --- | --- |
| REQ-001 / AC-1 | Pass | Gate and picker both use `To Do`, `for-gascity`, `--ready`; named `test_blocked_todo_does_not_fire` is red on the pre-fix gate and green on the current gate. |
| REQ-002 / AC-2 | Partial | Existing suite proves stamp, stale-state clearing, no-bead guard, and happy-path close; gate tests prove fresh/expired backoff. Real-send failure handling is missing (A1); mail-log tests do not prove dedup delivery. |
| REQ-003 / AC-3 | Partial | Blocked/ready/busy/empty board tests pass and name the task, but production notification error handling and episode semantics fail A1/A2. |
| REQ-004 / AC-4 | Pass | `make test` passed twice: blocked 7/7, noop 10/10, ready 10/10 (27 checks). |
| Plan §4–5 / `gcrex-72t8` | Pass | `grep -n noon orders/rex-backlog.toml` exits 1. Diff has only the comment correction and pre-existing operator `enabled = false` hunk. Neither hunk was staged or committed by review. |

`/Users/boris/my-city/tests/rex-backlog-noop.test.sh:38` and `/Users/boris/my-city/tests/rex-backlog-blocked.test.sh:60` set `REX_BACKLOG_MAIL_LOG`, bypassing the real mail branch. Their green results therefore cannot settle A1/A2.

The ten committed changed paths match the context's change summary (shell helpers, order/formula wiring, Makefile, three test suites). No changed overlay path exists in that diff. Pre-existing dirty `.pi/extensions/gc-hooks.js`, `city.toml`, `pack.toml`, `packs.lock`, and untracked runtime/operator content were excluded from the review diff and left untouched. Re-enabling the working order, relabeling prerequisites, and changing gc suppression remain out of scope.

## Proof artifacts and evidence clarification

- `/Volumes/Sources/rex-gas-city/plans/GCREX-60/acceptance-make-test.log`: all 27 existing checks green.
- `/Volumes/Sources/rex-gas-city/plans/GCREX-60/acceptance-probes.py`: retained runnable red/green and notification correctness checks; exits 1 on the current required gaps.
- `/Volumes/Sources/rex-gas-city/plans/GCREX-60/acceptance-probes.log`: full red/green output and four failed notification assertions.

The pre-fix gate hardcodes its live rig/stamp paths and predates the test env seams. For a safe, meaningful isolated replay, the probe parameterizes only those three path/interval assignments in the temporary old gate; it leaves the old readiness and backoff logic unchanged. This produces **8 passed / 2 failed**, including `FAIL - blocked labeled To Do (todo=1, ready=0) does not fire (want exit 1, got 0)`. The current gate passes 10/10. The summary/context's historical **6 passed / 4 failed** was not reproduced in this isolated check; use this disclosed replay as the acceptance evidence, not an undisclosed invocation of the hardcoded old gate against live state.

Probe syntax compilation passed. LSP diagnostics were attempted but unavailable because no Python language server is configured; no implementation or tooling configuration was changed to address that.

## Handoff

Return **iterate** for A1 and A2, with named production-mail-path tests and the smallest fixes above. Review execution completed successfully; final synthesis and fix application own the final review verdict. No PR was opened or updated by this lane.
