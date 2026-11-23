Captured `ps` output from macOS BSD ps (macOS 26, 2026-11-23):

    ps -axo pid=,ppid=,stat=,comm=,lstart=
    ps -ww -axo pid=,ppid=,args=

Rows for `launchd`, `logd`, and `fseventsd` were kept. The edge cases
(command name with spaces, missing `lstart`, malformed pid) stay synthetic in
`ps_test.go`.
