Captured `ps` output from macOS BSD ps (macOS 26, 2026-11-23):

    ps -axo pid=,ppid=,stat=,lstart=,comm=
    ps -ww -axo pid=,ppid=,args=

Rows for `launchd`, `logd`, `fseventsd`, and `accessoryupdater` were kept. The
`accessoryupdater` row is the truncation guard: with the old
`pid=,ppid=,stat=,comm=,lstart=` order, BSD ps cut its comm to
`(accessoryupdate`. `comm=` is now the final column, so the value stays whole.
The edge cases (command name with spaces, missing `lstart`, malformed pid) stay
synthetic in `ps_test.go`.
