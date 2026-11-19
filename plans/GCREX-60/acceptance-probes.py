#!/usr/bin/env python3
"""Read-only acceptance probes. All script state and CLI fakes live in temp dirs."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

WORKTREE = Path('/Users/boris/my-city')
assert WORKTREE.resolve() == WORKTREE
os.chdir(WORKTREE)
assert Path.cwd().resolve() == WORKTREE


def run(args, env):
    return subprocess.run(args, env=env, text=True, capture_output=True, cwd=WORKTREE)


failures = []
with tempfile.TemporaryDirectory(prefix='gcrex-3fc-acceptance-') as tmp:
    root = Path(tmp)
    bin_dir = root / 'bin'
    bin_dir.mkdir()
    state = root / 'state'
    state.mkdir()
    rig = root / 'rig'
    rig.mkdir()
    board = root / 'board'
    board.write_text('blocked')
    calls = root / 'calls'
    sent = root / 'sent'
    live = root / 'live'
    (bin_dir / 'backlog').write_text('''#!/bin/sh
case "$*" in
  *--plain*) echo '  [HIGH] [bug] GCREX-59 - blocked task' ;;
  *--ready*) if [ "$(cat "$BOARD")" = ready ]; then echo 1; else echo 0; fi ;;
  *'In Progress'*) echo 0 ;;
  *) echo 1 ;;
esac
''')
    (bin_dir / 'gc').write_text('''#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
if [ "$1 $2" != 'mail send' ]; then exit 0; fi
if [ "${MAIL_FAIL:-0}" = 1 ]; then echo 'fake mail delivery failed' >&2; exit 1; fi
key=''
while [ $# -gt 0 ]; do
  if [ "$1" = --dedup ]; then shift; key="$1"; fi
  shift
done
# Pinned gc v1.5.1: suppress while this key has an open message.
if [ -f "$LIVE" ] && grep -qxF "$key" "$LIVE"; then exit 0; fi
printf '%s\n' "$key" >> "$LIVE"
printf '%s\n' "$key" >> "$SENT"
''')
    for executable in bin_dir.iterdir():
        executable.chmod(0o755)
    env = os.environ.copy()
    for name in ('GC_BEAD_ID', 'REX_BACKLOG_MAIL_LOG'):
        env.pop(name, None)
    env.update(PATH=str(bin_dir) + os.pathsep + env['PATH'],
               REX_BACKLOG_STATE_DIR=str(state), REX_BACKLOG_RIG_DIR=str(rig),
               BOARD=str(board), CALLS=str(calls), SENT=str(sent), LIVE=str(live))

    # Replay the old gate only with its paths parameterized for isolation.
    # The old gate predates the env seams, so an unmodified temp copy would
    # still use the live rig/state, invalidating a fake-board red proof.
    old = subprocess.check_output(
        ['git', 'show', '4c1318c~1:scripts/rex-backlog-ready.sh'], text=True)
    for line, replacement in (
        ('RIG_DIR="/Volumes/Sources/rex-gas-city"',
         'RIG_DIR="${REX_BACKLOG_RIG_DIR:-/Volumes/Sources/rex-gas-city}"'),
        ('STAMP="/Users/boris/my-city/.gc/state/rex-backlog.last"',
         'STAMP="${REX_BACKLOG_STAMP:-/Users/boris/my-city/.gc/state/rex-backlog.last}"'),
        ('MIN_INTERVAL=300', 'MIN_INTERVAL="${REX_BACKLOG_MIN_INTERVAL:-300}"'),
    ):
        assert old.count(line) == 1
        old = old.replace(line, replacement)
    (root / 'scripts').mkdir()
    (root / 'tests').mkdir()
    (root / 'scripts/rex-backlog-ready.sh').write_text(old)
    shutil.copy(WORKTREE / 'tests/rex-backlog-ready.test.sh', root / 'tests')
    red = run(['bash', str(root / 'tests/rex-backlog-ready.test.sh')], env)
    assert red.returncode != 0
    assert 'FAIL - blocked labeled To Do (todo=1, ready=0) does not fire' in red.stdout
    print('RED: pre-fix gate, paths isolated; predicate/backoff unchanged')
    print(red.stdout)
    green = run(['bash', 'tests/rex-backlog-ready.test.sh'], env)
    assert green.returncode == 0, green.stdout + green.stderr
    print('GREEN: current gate\n' + green.stdout)

    for name in ('noop', 'blocked'):
        error_env = dict(env, MAIL_FAIL='1')
        if name == 'noop':
            error_env['GC_BEAD_ID'] = 'acceptance-fake-step'
        result = run(['bash', f'assets/scripts/rex-backlog-{name}.sh'], error_env)
        assert 'fake mail delivery failed' in result.stderr
        failed_delivery_closed_pass = name == 'noop' and 'gc.outcome=pass' in calls.read_text()
        print(f'MAIL_FAILURE_{name}: exit={result.returncode}, '
              f'closed_pass={failed_delivery_closed_pass}, stderr={result.stderr.strip()}')
        if result.returncode == 0:
            failures.append(f'{name}: mail delivery failure reported as success')

    def deliveries():
        return len(sent.read_text().splitlines()) if sent.exists() else 0

    detector = ['bash', 'assets/scripts/rex-backlog-blocked.sh']
    assert run(detector, env).returncode == 0
    assert deliveries() == 1
    board.write_text('ready')
    assert run(detector, env).returncode == 0
    board.write_text('blocked')
    assert run(detector, env).returncode == 0
    print(f'EPISODE_RESET: blocked -> ready -> blocked; deliveries={deliveries()}, expected=2')
    if deliveries() != 2:
        failures.append('blocked: a new episode cannot notify while the previous message is live')
    # Start a fresh independent scenario: notify, archive, remain blocked.
    live.unlink()
    sent.unlink()
    for marker in state.glob('rex-backlog.*'):  # fresh scenario: no carried episode state
        marker.unlink()
    assert run(detector, env).returncode == 0
    assert deliveries() == 1
    live.unlink()
    assert run(detector, env).returncode == 0
    print(f'ARCHIVE_SAME_EPISODE: archive then rerun; deliveries={deliveries()}, expected=1 total for this continuing episode')
    if deliveries() > 1:
        failures.append('blocked: archiving the notification allows another mail in the same episode')

print('Acceptance gaps:')
for failure in failures:
    print('FAIL - ' + failure)
assert not failures, '; '.join(failures)
