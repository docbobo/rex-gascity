#!/usr/bin/env bash
# Run with: bash /Volumes/Sources/rex-gas-city/plans/GCREX-60/simplicity-mail-failure.check.sh
set -euo pipefail
WORKTREE=/Users/boris/my-city
cd "$WORKTREE"
[ "$(pwd -P)" = "$WORKTREE" ]
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/rig" "$tmp/state"
cat > "$tmp/bin/backlog" <<'EOF'
#!/bin/sh
case "$*" in
  *--plain*) echo '  [HIGH] [bug] GCREX-59 - blocked task' ;;
  *--ready*|*'In Progress'*) echo 0 ;;
  *) echo 1 ;;
esac
EOF
cat > "$tmp/bin/gc" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" > "$CHECK_GC_LOG"
echo 'injected mail transport failure' >&2
exit 73
EOF
chmod +x "$tmp/bin/backlog" "$tmp/bin/gc"
unset REX_BACKLOG_MAIL_LOG
export CHECK_GC_LOG="$tmp/gc.log"
rc=0
PATH="$tmp/bin:$PATH" REX_BACKLOG_RIG_DIR="$tmp/rig" REX_BACKLOG_STATE_DIR="$tmp/state" \
  bash assets/scripts/rex-backlog-blocked.sh || rc=$?
grep -q '^mail send human --dedup rex-backlog-blocked-[0-9-]* ' "$CHECK_GC_LOG"
printf 'Blocked detector exit after injected send failure: %s\n' "$rc"
if [ "$rc" -eq 0 ]; then
  echo 'FAIL: blocked alert delivery failure is reported as success' >&2
  exit 1
fi
echo 'PASS: blocked alert delivery failure remains visible'
