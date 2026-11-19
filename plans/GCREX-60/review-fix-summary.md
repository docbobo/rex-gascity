# Review-fix summary — GCREX-60 (iteration 1, bead gcrex-73j)

Worktree: `/Users/boris/my-city` (`pwd -P` verified; anchor `gcrex-hvlf`, HEAD `4c1318c`). Launcher root untouched.
Changes are uncommitted in that worktree. The `orders/rex-backlog.toml` comment/`enabled = false` hunk and other operator edits were not touched.

## RF-1 — failed send no longer looks like success
- `assets/scripts/rex-backlog-blocked.sh`, `assets/scripts/rex-backlog-noop.sh`: removed `|| true` on `gc mail send`; `set -e` propagates the failure.
- No-op keeps its line-27 backoff stamp; a failed send never reaches the pass/no-op bead close. Success path unchanged. No-op dedup key stays `rex-backlog-noop`.

## RF-2 — one mail per blocked episode
- `rex-backlog-blocked.sh`: new `$STATE_DIR/rex-backlog.blocked` marker (`REX_BACKLOG_STATE_DIR`, same default as no-op). Cleared on every not-blocked exit; if present, no send. Written only after a successful send, so a failed send retries next fire.
- Dedup key is now `rex-backlog-blocked-<epoch>-<pid>`, so a still-live message from a prior episode cannot suppress the next. gc `--dedup` is live-message scoped (gascity `internal/mail/mail.go:90-110`, v1.5.1); the marker, not the key, defines the episode. Deployed provider behavior remains unverified (RR-2).

## ME-1 — production send args now tested
- `tests/rex-backlog-blocked.test.sh` (+ prod section, fake `gc` on PATH, no `REX_BACKLOG_MAIL_LOG`): recipient `human`, key shape, same-episode no resend, new episode new key, failed send exit nonzero and retried.
- `tests/rex-backlog-noop.test.sh` (+ prod section): failed send exits nonzero, no bead close, stamp kept; success sends `human`/`rex-backlog-noop` and closes pass/no-op.
- Red first: the new cases failed before the script edits (blocked 10/4, noop 2 failing), green after.

## Evidence files adjusted (not product code)
- `simplicity-mail-failure.check.sh`: grep now accepts the episode-suffixed key; uses a temp `REX_BACKLOG_STATE_DIR` (the detector now writes state and the check must not touch live state).
- `acceptance-probes.py`: the "fresh independent scenario" now clears the temp state dir, since episode state is now persisted.

## Proof (from `/Users/boris/my-city`)
- `python3 .../acceptance-probes.py` exit 0 (MAIL_FAILURE exit=1 both; EPISODE_RESET deliveries=2; ARCHIVE_SAME_EPISODE deliveries=1).
- `bash .../simplicity-mail-failure.check.sh` exit 0.
- `make test`: blocked 14/14, noop 16/16, ready 10/10, exit 0.

## Verdict: iterate
Fixes are applied, but acceptance/simplicity lanes have not re-reviewed them. RR-1 (delete the `REX_BACKLOG_MAIL_LOG` seam) left in place, not required.
