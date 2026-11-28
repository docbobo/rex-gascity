Show a status update in a floating, kept-open pane of a Rex session.

Usage:

    gc <binding> status-update --template
    gc <binding> status-update --session ID [--server SERVER] [--display text] [--body-file FILE]

--template        Print the Markdown template to fill in, then exit.
--session ID      The Rex session to open the pane in. Required.
--server SERVER   The Rex server that owns the session. Without it, rex uses
                  its own default server. The Gas City action fills this in
                  from $REX_SERVER, then rex home --json's home.endpoint on
                  this machine, then rex whoami --json's endpoint.
--display MODE    How to show the update. Only `text` exists today.
--body-file FILE  Read the update from FILE instead of stdin.

Example:

    gc rex status-update --template > update.md
    gc rex status-update --session "$REX_SESSION" < update.md

Exit status: 0 when the pane opened, 1 for an empty update or an unknown
display, 2 for a usage error, 127 when rex is missing, and rex's own status
when rex fails.
