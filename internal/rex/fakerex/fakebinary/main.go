// Command fakebinary is the fake rex executable built by the fakerex test
// helper. It speaks the small CLI surface the real cli.Client uses -- `api
// call`, `api describe`, `version`, `send-key`, `session kill`, `block call`
// and `block inspect` -- over a state dir named by FAKEREX_STATE.
//
// State lives in one JSON file guarded by a flock, so every short-lived
// invocation is a safe read-modify-write. Panes are real child processes:
// session.new_window and session.new_split start the requested argv detached
// (setsid) with stdout and stderr appended to a file and stdin on a FIFO. The
// argv runs under /bin/sh only to record its exit code for `process`; the
// fake reports the launched argv as the foreground process.
//
// send-key maps the documented subset: "ctrl+c" sends SIGINT to the pane's
// process group, "enter"/"tab"/"space"/arrow/Esc keys and ctrl+<letter> write
// the usual bytes, and any other key is written literally to stdin. It does
// not emulate a PTY.
//
// Exit codes follow docs/REX-CONTRACT.md section 8: 2 usage or validation,
// 3 missing target, 4 no server, 1 server-side failure.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex/fakerex/fakestate"
)

// formatCap is rex's format page size: a reply never carries more than
// 983040 content bytes and pages larger captures through blob_read.
const formatCap = 983040

// blobDefault is blob_read's default page size, clamped to formatCap.
const blobDefault = 49152

// faultDelay is how long FaultDelay stalls a call.
const faultDelay = 2 * time.Second

// defaultAPIVersion is the api revision the fake reports when the state does
// not override it. It is also the top of the CLI's supported server range.
const defaultAPIVersion = 14

var errUnreachable = errors.New("no Rex server is running")

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// apiRevision reads the server's reported api_revision from the state dir.
// A missing dir or an unset override falls back to defaultAPIVersion.
func apiRevision(dir string) int {
	if dir == "" {
		return defaultAPIVersion
	}
	st, err := fakestate.Load(dir)
	if err != nil || st.APIVersion == 0 {
		return defaultAPIVersion
	}
	return st.APIVersion
}

// run dispatches one CLI invocation. The dispatch is separate from main so the
// helper's tests can drive it in-process if they need to.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return fail(stderr, fmt.Errorf("%w: no command", fakestate.ErrBadRequest))
	}
	dir := os.Getenv("FAKEREX_STATE")
	var err error
	switch args[0] {
	case "version":
		rev := apiRevision(dir)
		jsonOut := len(args) > 1 && (args[1] == "--json" || args[1] == "-j")
		if jsonOut {
			err = printJSON(stdout, map[string]any{"version": "0.1.0", "api_revision": rev})
		} else {
			fmt.Fprintln(stdout, "fakerex 0.1.0")
			fmt.Fprintf(stdout, "api: revision %d (supports 0 to %d)\n", rev, defaultAPIVersion)
		}
	case "api":
		err = api(dir, args[1:], stdout)
	case "send-key":
		err = sendKey(dir, args[1:])
	case "session":
		err = sessionCmd(dir, args[1:], stdout)
	case "block":
		err = blockCmd(dir, args[1:], stdout)
	default:
		err = fmt.Errorf("%w: unknown command %q", fakestate.ErrBadRequest, args[0])
	}
	if err != nil {
		return fail(stderr, err)
	}
	return 0
}

// fail maps one error to a contract exit code.
func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "Error: %v\n", err)
	switch {
	case errors.Is(err, errUnreachable):
		return 4
	case errors.Is(err, fakestate.ErrBadRequest):
		return 2
	case errors.Is(err, fakestate.ErrNotFound):
		return 3
	default:
		return 1
	}
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	return enc.Encode(v)
}

// callArgs decodes api call's optional JSON object argument.
func callArgs(raw string) (map[string]any, error) {
	if raw == "" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON args: %v", fakestate.ErrBadRequest, err)
	}
	return m, nil
}

func api(dir string, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: api needs a verb", fakestate.ErrBadRequest)
	}
	switch args[0] {
	case "describe":
		if len(args) < 2 {
			return fmt.Errorf("%w: describe needs a method", fakestate.ErrBadRequest)
		}
		if !knownMethod(args[1]) {
			return fmt.Errorf("%w: no such method %q", fakestate.ErrNotFound, args[1])
		}
		return printJSON(stdout, map[string]any{
			"method": args[1], "input": map[string]any{}, "output": map[string]any{},
		})
	case "call":
		if len(args) < 2 {
			return fmt.Errorf("%w: api call needs a method", fakestate.ErrBadRequest)
		}
		raw := ""
		if len(args) > 2 {
			raw = args[2]
		}
		argv, err := callArgs(raw)
		if err != nil {
			return err
		}
		if !knownMethod(args[1]) {
			return fmt.Errorf("%w: no such method %q", fakestate.ErrNotFound, args[1])
		}
		key := args[1]
		if key == "block.method" {
			key += "." + str(argv, "method")
		}
		return callWithFault(dir, key, stdout, func() (any, error) {
			return call(dir, args[1], argv)
		})
	default:
		return fmt.Errorf("%w: unknown api verb %q", fakestate.ErrBadRequest, args[0])
	}
}

func knownMethod(m string) bool {
	return slices.Contains(fakestate.Methods, m)
}

// callWithFault runs one method body with any scripted fault for key applied.
// A pre-fault replaces the call; a post-fault spoils the reply after the body
// ran, so tests can separate a failed request from a lost reply.
func callWithFault(dir, key string, stdout io.Writer, body func() (any, error)) error {
	fault, err := fakestate.TakeFault(dir, key)
	if err != nil {
		return err
	}
	if fault != nil {
		switch fault.Kind {
		case fakestate.FaultError:
			return fmt.Errorf("%w: scripted failure for %s", fakestate.ErrFail, key)
		case fakestate.FaultUnreachable:
			return errUnreachable
		case fakestate.FaultDelay:
			time.Sleep(faultDelay)
		}
	}
	reply, err := body()
	if err != nil {
		return err
	}
	if fault != nil {
		switch fault.Kind {
		case fakestate.FaultLostReply:
			return fmt.Errorf("%w: reply to %s was lost", fakestate.ErrFail, key)
		case fakestate.FaultGarbled:
			_, err := io.WriteString(stdout, "{\"unterminated\": \n")
			return err
		}
	}
	if reply == nil {
		return nil
	}
	return printJSON(stdout, reply)
}

// idPrefix is the id syntax Rex validates client-side before a call. A JSON
// argument with the wrong prefix is a usage error (exit 2), not a lookup miss.
var idPrefix = map[string]string{
	"session_id": "session:",
	"block_id":   "block:",
	"anchor_id":  "block:",
	"window_id":  "window:",
}

func validateIDs(in map[string]any) error {
	for key, prefix := range idPrefix {
		s, ok := in[key].(string)
		if !ok || s == "" {
			continue
		}
		if !strings.HasPrefix(s, prefix) {
			return fmt.Errorf("%w: %s %q must start with %q", fakestate.ErrBadRequest, key, s, prefix)
		}
	}
	return nil
}

func call(dir, method string, in map[string]any) (any, error) {
	if dir == "" {
		return nil, fmt.Errorf("%w: FAKEREX_STATE is unset", errUnreachable)
	}
	if err := validateIDs(in); err != nil {
		return nil, err
	}
	switch method {
	case "server.version":
		return map[string]any{"version": "0.1.0", "api_revision": apiRevision(dir)}, nil
	case "session.create":
		return sessionCreate(dir, str(in, "label"))
	case "session.destroy":
		return setterReply(dir, func(st *fakestate.State) error {
			s, err := st.Session(str(in, "session_id"))
			if err != nil {
				return err
			}
			killSession(dir, s)
			st.Sessions = removeSession(st.Sessions, s.ID)
			st.Revision++
			return nil
		})
	case "session.list":
		st, err := load(dir)
		if err != nil {
			return nil, err
		}
		sessions := make([]map[string]any, 0, len(st.Sessions))
		for _, s := range st.Sessions {
			sessions = append(sessions, map[string]any{"session_id": s.ID, "label": s.Label})
		}
		return map[string]any{"sessions": sessions}, nil
	case "session.list_clients":
		var reply any
		err := read(dir, func(st *fakestate.State) error {
			s, err := st.Session(str(in, "session_id"))
			if err != nil {
				return err
			}
			clients := make([]map[string]any, 0, len(s.Clients))
			for _, c := range s.Clients {
				clients = append(clients, map[string]any{
					"client_id": c.ID, "principal": "",
					"info":         map[string]any{"kind": c.Kind, "name": c.Name},
					"connected_at": "", "connection_state": "",
				})
			}
			reply = map[string]any{"clients": clients}
			return nil
		})
		return reply, err
	case "session.view":
		return sessionView(dir, str(in, "session_id"))
	case "session.new_window":
		return newWindow(dir, in)
	case "session.new_split":
		return newSplit(dir, in)
	case "session.focus_block":
		return setterReply(dir, func(st *fakestate.State) error {
			s, err := st.Session(str(in, "session_id"))
			if err != nil {
				return err
			}
			for _, w := range s.Windows {
				for _, b := range w.Blocks {
					if b.ID == str(in, "block_id") {
						w.FocusedBlock = b.ID
						s.ActiveWindow = w.ID
						return nil
					}
				}
			}
			return fmt.Errorf("%w: block %s", fakestate.ErrNotFound, str(in, "block_id"))
		})
	case "session.focus_window":
		return setterReply(dir, func(st *fakestate.State) error {
			s, err := st.Session(str(in, "session_id"))
			if err != nil {
				return err
			}
			for _, w := range s.Windows {
				if w.ID == str(in, "window_id") {
					s.ActiveWindow = w.ID
					return nil
				}
			}
			return fmt.Errorf("%w: window %s", fakestate.ErrNotFound, str(in, "window_id"))
		})
	case "session.set_block_label":
		return setterReply(dir, func(st *fakestate.State) error {
			_, _, b, err := st.Block(str(in, "block_id"))
			if err != nil {
				return err
			}
			b.Label = str(in, "label")
			return nil
		})
	case "session.set_window_label":
		return setterReply(dir, func(st *fakestate.State) error {
			s, err := st.Session(str(in, "session_id"))
			if err != nil {
				return err
			}
			for _, w := range s.Windows {
				if w.ID == str(in, "window_id") {
					w.Label = str(in, "label")
					return nil
				}
			}
			return fmt.Errorf("%w: window %s", fakestate.ErrNotFound, str(in, "window_id"))
		})
	case "session.set_label":
		return setterReply(dir, func(st *fakestate.State) error {
			s, err := st.Session(str(in, "session_id"))
			if err != nil {
				return err
			}
			s.Label = str(in, "label")
			return nil
		})
	case "block.close":
		return blockClose(dir, in)
	case "block.event":
		return setterReply(dir, func(st *fakestate.State) error { return blockEvent(dir, in) })
	case "block.method":
		args, _ := in["args"].(map[string]any)
		return blockMethod(dir, str(in, "session_id"), str(in, "block_id"), str(in, "method"), args)
	case "target.resolve", "target.complete":
		return targetResolve(dir, in)
	}
	return nil, fmt.Errorf("%w: no such method %q", fakestate.ErrNotFound, method)
}

// setterReply runs one mutation and answers with Rex's ack shape, {"revision":N}.
// cli.Client ignores the body of these calls; the reply exists so the fake's
// wire shape matches the real server.
func setterReply(dir string, fn func(*fakestate.State) error) (any, error) {
	var reply any
	err := update(dir, func(st *fakestate.State) error {
		if err := fn(st); err != nil {
			return err
		}
		st.Revision++
		reply = map[string]any{"revision": st.Revision}
		return nil
	})
	return reply, err
}

func load(dir string) (*fakestate.State, error) {
	st, err := fakestate.Load(dir)
	if err != nil {
		if errors.Is(err, fakestate.ErrNotFound) {
			return nil, fmt.Errorf("%w at %s", errUnreachable, dir)
		}
		return nil, err
	}
	return st, nil
}

func read(dir string, fn func(*fakestate.State) error) error {
	if _, err := load(dir); err != nil {
		return err
	}
	return fakestate.Read(dir, fn)
}

// readState reads the whole state under the shared lock.
func readState(dir string) (*fakestate.State, error) {
	var st *fakestate.State
	err := read(dir, func(s *fakestate.State) error { st = s; return nil })
	return st, err
}

func update(dir string, fn func(*fakestate.State) error) error {
	if _, err := load(dir); err != nil {
		return err
	}
	return fakestate.Update(dir, fn)
}

func sessionCreate(dir, label string) (any, error) {
	if label == "" {
		return nil, fmt.Errorf("%w: label is required", fakestate.ErrBadRequest)
	}
	var reply any
	err := update(dir, func(st *fakestate.State) error {
		s := &fakestate.Session{ID: nextID(st, "session"), Label: label}
		st.Sessions = append(st.Sessions, s)
		st.Revision++
		reply = map[string]any{"session_id": s.ID, "initial_windows": []any{}, "revision": st.Revision}
		return nil
	})
	return reply, err
}

func sessionView(dir, sid string) (any, error) {
	var reply any
	err := read(dir, func(st *fakestate.State) error {
		s, err := st.Session(sid)
		if err != nil {
			return err
		}
		windows := make([]map[string]any, 0, len(s.Windows))
		for _, w := range s.Windows {
			blocks := make([]map[string]any, 0, len(w.Blocks))
			for _, b := range w.Blocks {
				blocks = append(blocks, map[string]any{
					"block_id": b.ID, "flavor": b.Flavor, "label": b.Label, "owner": b.Owner,
				})
			}
			windows = append(windows, map[string]any{
				"window_id": w.ID, "label": w.Label, "focused_block_id": w.FocusedBlock,
				"layers": []any{map[string]any{"layout": w.Layout, "blocks": blocks}},
			})
		}
		reply = map[string]any{
			"session_id": s.ID, "label": s.Label, "active_window_id": s.ActiveWindow,
			"revision": st.Revision, "windows": windows,
		}
		return nil
	})
	return reply, err
}

type launchInput struct {
	command []string
	cwd     string
	flavor  string
	label   string
}

// launchFromLayout reads the block layout the provider sends.
func launchFromLayout(in map[string]any) (launchInput, error) {
	layout, _ := in["layout"].(map[string]any)
	block, _ := layout["block"].(map[string]any)
	if block == nil {
		return launchInput{}, fmt.Errorf("%w: layout.block is required", fakestate.ErrBadRequest)
	}
	li := launchInput{label: str(block, "label"), flavor: str(block, "flavor")}
	if opts, ok := block["options"].(map[string]any); ok {
		li.cwd = str(opts, "cwd")
		if cmd, ok := opts["command"].([]any); ok {
			for _, a := range cmd {
				li.command = append(li.command, fmt.Sprint(a))
			}
		}
	}
	return li, nil
}

func newWindow(dir string, in map[string]any) (any, error) {
	li, err := launchFromLayout(in)
	if err != nil {
		return nil, err
	}
	var reply any
	err = update(dir, func(st *fakestate.State) error {
		s, err := st.Session(str(in, "session_id"))
		if err != nil {
			return err
		}
		b := &fakestate.Block{
			ID: nextID(st, "block"), Label: li.label, Flavor: li.flavor,
			Command: li.command, Cwd: li.cwd,
		}
		w := &fakestate.Window{
			ID: nextID(st, "window"), Label: str(in, "window_label"),
			FocusedBlock: b.ID,
			Layout:       &fakestate.Node{Rect: [4]float64{0, 0, 1, 1}, BlockID: b.ID},
			Blocks:       []*fakestate.Block{b},
		}
		s.AddWindow(w)
		if _, err := startPane(dir, b); err != nil {
			return err
		}
		st.Revision++
		reply = map[string]any{"window_id": w.ID, "block_ids": []string{b.ID}, "revision": st.Revision}
		return nil
	})
	return reply, err
}

func newSplit(dir string, in map[string]any) (any, error) {
	li, err := launchFromLayout(in)
	if err != nil {
		return nil, err
	}
	var reply any
	err = update(dir, func(st *fakestate.State) error {
		s, err := st.Session(str(in, "session_id"))
		if err != nil {
			return err
		}
		as, w, anchor, err := st.Block(str(in, "anchor_id"))
		if err != nil {
			return err
		}
		if as.ID != s.ID {
			return fmt.Errorf("%w: block %s is not in session %s", fakestate.ErrNotFound, anchor.ID, s.ID)
		}
		b := &fakestate.Block{
			ID: nextID(st, "block"), Label: li.label, Flavor: li.flavor,
			Command: li.command, Cwd: li.cwd,
		}
		split(w, anchor.ID, b.ID, str(in, "direction"), str(in, "side"))
		w.Blocks = append(w.Blocks, b)
		if _, err := startPane(dir, b); err != nil {
			return err
		}
		st.Revision++
		reply = map[string]any{"block_ids": []string{b.ID}, "revision": st.Revision}
		return nil
	})
	return reply, err
}

// split replaces anchor's leaf with a two-leaf split. Rex's "horizontal"
// direction places panes side by side (it cuts the width); "vertical" stacks
// them. side=before puts the new pane first.
func split(w *fakestate.Window, anchor, id, direction, side string) {
	leaf := w.Layout.Leaf(anchor)
	if leaf == nil {
		return
	}
	r := leaf.Rect
	var before, after [4]float64
	if direction == "horizontal" {
		half := r[2] / 2
		before = [4]float64{r[0], r[1], half, r[3]}
		after = [4]float64{r[0] + half, r[1], half, r[3]}
	} else {
		half := r[3] / 2
		before = [4]float64{r[0], r[1], r[2], half}
		after = [4]float64{r[0], r[1] + half, r[2], half}
	}
	anchorNode := &fakestate.Node{Rect: before}
	newNode := &fakestate.Node{Rect: after}
	if side == "before" {
		anchorNode.BlockID = id
		newNode.BlockID = anchor
	} else {
		anchorNode.BlockID = anchor
		newNode.BlockID = id
	}
	*leaf = fakestate.Node{Rect: r, Split: &fakestate.Split{Before: anchorNode, After: newNode}}
}

func blockClose(dir string, in map[string]any) (any, error) {
	var reply any
	var pid int
	var blockID string
	err := update(dir, func(st *fakestate.State) error {
		s, w, b, err := st.Block(str(in, "block_id"))
		if err != nil {
			return err
		}
		if boolOf(in, "keep_last") && len(w.Blocks) == 1 {
			return fmt.Errorf("%w: cannot close the last block in window %s", fakestate.ErrFail, w.ID)
		}
		pid, blockID = b.PID, b.ID
		s.Remove(b.ID)
		st.Revision++
		reply = map[string]any{"block_id": b.ID, "revision": st.Revision}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Stop the pane outside the state lock: the record is gone, so nothing
	// else can reach its process group.
	stopPane(dir, blockID, pid)
	return reply, nil
}

func blockEvent(dir string, in map[string]any) error {
	ev, _ := in["event"].(map[string]any)
	typ := str(ev, "type")
	payload, _ := ev["payload"].(map[string]any)
	switch typ {
	case "paste":
		return writePane(dir, str(in, "block_id"), str(payload, "text"))
	case "key":
		return sendKeys(dir, str(in, "session_id"), str(in, "block_id"), []string{str(payload, "key")})
	case "focus":
		return nil
	}
	return fmt.Errorf("%w: unknown event %q", fakestate.ErrBadRequest, typ)
}

func blockMethod(dir, sid, bid, method string, args map[string]any) (any, error) {
	if bid == "" {
		return nil, fmt.Errorf("%w: block_id is required", fakestate.ErrBadRequest)
	}
	r, err := readState(dir)
	if err != nil {
		return nil, err
	}
	s, _, b, err := r.Block(bid)
	if err != nil {
		return nil, err
	}
	// An empty session is the caller's own pane session in Rex; the fake has
	// no calling pane, so the block's session stands in.
	if sid != "" && s.ID != sid {
		return nil, fmt.Errorf("%w: block %s is not in session %s", fakestate.ErrNotFound, bid, sid)
	}
	switch method {
	case "process":
		info := map[string]any{
			"foreground": foreground(b),
			"ancestors":  []any{},
			"last_exit":  lastExit(dir, b),
		}
		return info, nil
	case "format":
		return format(dir, b, args)
	case "blob_read":
		return blobRead(dir, b, args)
	case "write":
		if err := writePane(dir, b.ID, str(args, "data")); err != nil {
			return nil, err
		}
		return map[string]any{"bytes_written": len(str(args, "data"))}, nil
	case "clear":
		if err := os.Truncate(fakestate.OutPath(dir, b.ID), 0); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		return map[string]any{}, nil
	case "program_status":
		recs := b.ProgramStatus
		if recs == nil {
			recs = []fakestate.Record{}
		}
		return map[string]any{"records": recs}, nil
	case "size":
		return map[string]any{"cols": 80, "rows": 24, "owner": b.Owner}, nil
	case "title":
		return map[string]any{"title": b.Label, "owner": b.Owner}, nil
	case "pwd":
		return map[string]any{"cwd": b.Cwd, "owner": b.Owner}, nil
	case "input_mode":
		return map[string]any{"echo": true, "canonical": false, "signals": true}, nil
	case "reset":
		return map[string]any{}, nil
	}
	return nil, fmt.Errorf("%w: no such block method %q", fakestate.ErrNotFound, method)
}

func format(dir string, b *fakestate.Block, args map[string]any) (any, error) {
	out, err := os.ReadFile(fakestate.OutPath(dir, b.ID))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	total := len(out)
	maxBytes := intOf(args, "max_bytes")
	if maxBytes == 0 {
		if total > formatCap {
			return nil, fmt.Errorf("%w: control response exceeds 1048576 bytes (status=413 code=too_large)", fakestate.ErrFail)
		}
		return map[string]any{"content": string(out), "total": total, "eof": true}, nil
	}
	if maxBytes > formatCap {
		maxBytes = formatCap
	}
	n := min(maxBytes, total)
	reply := map[string]any{"content": string(out[:n]), "total": total}
	if n < total {
		reply["id"] = 1
	} else {
		reply["eof"] = true
	}
	return reply, nil
}

func blobRead(dir string, b *fakestate.Block, args map[string]any) (any, error) {
	if id := intOf(args, "id"); id != 0 && id != 1 {
		return nil, fmt.Errorf("%w: no result is stored", fakestate.ErrNotFound)
	}
	out, err := os.ReadFile(fakestate.OutPath(dir, b.ID))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	total := len(out)
	offset := min(intOf(args, "offset"), total)
	maxBytes := intOf(args, "max_bytes")
	if maxBytes == 0 {
		maxBytes = blobDefault
	}
	if maxBytes > formatCap {
		maxBytes = formatCap
	}
	end := min(offset+maxBytes, total)
	return map[string]any{
		"id": 1, "data": string(out[offset:end]), "total": total, "eof": end >= total,
	}, nil
}

func targetResolve(dir string, in map[string]any) (any, error) {
	st, err := load(dir)
	if err != nil {
		return nil, err
	}
	kinds := stringsOf(in["kinds"])
	queries := stringsOf(in["queries"])
	targets := []map[string]any{}
	for _, q := range queries {
		switch {
		case contains(kinds, "session"):
			for _, s := range st.Sessions {
				if s.ID == q || s.Label == q {
					targets = append(targets, map[string]any{"kind": "session", "id": s.ID, "label": s.Label})
				}
			}
		case contains(kinds, "block"):
			for _, s := range st.Sessions {
				for _, w := range s.Windows {
					for _, b := range w.Blocks {
						if b.ID == q || b.Label == q {
							targets = append(targets, map[string]any{"kind": "block", "id": b.ID, "label": b.Label})
						}
					}
				}
			}
		}
	}
	return map[string]any{"targets": targets}, nil
}

func sessionCmd(dir string, args []string, stdout io.Writer) error {
	if len(args) < 2 || args[0] != "kill" {
		return fmt.Errorf("%w: usage: session kill <id>", fakestate.ErrBadRequest)
	}
	id := args[1]
	err := update(dir, func(st *fakestate.State) error {
		s, err := st.Session(id)
		if err != nil {
			return err
		}
		killSession(dir, s)
		st.Sessions = removeSession(st.Sessions, s.ID)
		st.Revision++
		return nil
	})
	if err == nil {
		_, err = io.WriteString(stdout, "{}\n")
	}
	return err
}

func blockCmd(dir string, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: block needs a verb", fakestate.ErrBadRequest)
	}
	switch args[0] {
	case "call":
		// block call -s <sid> -b <bid> <method> [json]
		var sid, bid, method, raw string
		rest := args[1:]
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "-s":
				if i++; i < len(rest) {
					sid = rest[i]
				}
			case "-b":
				if i++; i < len(rest) {
					bid = rest[i]
				}
			default:
				if method == "" {
					method = rest[i]
				} else if raw == "" {
					raw = rest[i]
				}
			}
		}
		if bid == "" || method == "" {
			return fmt.Errorf("%w: block call needs -b and a method", fakestate.ErrBadRequest)
		}
		if !slices.Contains(fakestate.BlockMethods, method) {
			return fmt.Errorf("%w: no such block method %q", fakestate.ErrNotFound, method)
		}
		argv, err := callArgs(raw)
		if err != nil {
			return err
		}
		return callWithFault(dir, "block.method."+method, stdout, func() (any, error) {
			return blockMethod(dir, sid, bid, method, argv)
		})
	case "inspect":
		if len(args) < 2 {
			return fmt.Errorf("%w: block inspect needs a block id", fakestate.ErrBadRequest)
		}
		bid := args[1]
		var reply any
		err := read(dir, func(st *fakestate.State) error {
			_, _, b, err := st.Block(bid)
			if err != nil {
				return err
			}
			reply = map[string]any{"block_id": b.ID, "label": b.Label, "methods": blockMethods}
			return nil
		})
		if err != nil {
			return err
		}
		return printJSON(stdout, reply)
	}
	return fmt.Errorf("%w: unknown block verb %q", fakestate.ErrBadRequest, args[0])
}

var blockMethods = fakestate.BlockMethods

func sendKey(dir string, args []string) error {
	var sid, bid string
	var keys []string
	rest := args
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "-s":
			if i++; i < len(rest) {
				sid = rest[i]
			}
		case "-b":
			if i++; i < len(rest) {
				bid = rest[i]
			}
		case "--":
			keys = append(keys, rest[i+1:]...)
			i = len(rest)
		default:
			keys = append(keys, rest[i])
		}
	}
	if bid == "" {
		return fmt.Errorf("%w: send-key needs -b", fakestate.ErrBadRequest)
	}
	return callWithFault(dir, "send-key", nil, func() (any, error) {
		return nil, sendKeys(dir, sid, bid, keys)
	})
}

func sendKeys(dir, sid, bid string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	st, err := load(dir)
	if err != nil {
		return err
	}
	s, _, b, err := st.Block(bid)
	if err != nil {
		return err
	}
	if s.ID != sid && sid != "" {
		return fmt.Errorf("%w: block %s is not in session %s", fakestate.ErrNotFound, bid, sid)
	}
	for _, key := range keys {
		if err := applyKey(dir, b, key); err != nil {
			return err
		}
	}
	return nil
}

// applyKey maps one rex key name to a signal or a stdin write. The mapping is
// the documented fake subset, not Rex's full key-binding table.
func applyKey(dir string, b *fakestate.Block, key string) error {
	switch key {
	case "ctrl+c":
		signalPane(b, syscall.SIGINT)
		return nil
	case "enter", "return":
		key = "\n"
	case "tab":
		key = "\t"
	case "space":
		key = " "
	case "up":
		key = "\x1b[A"
	case "down":
		key = "\x1b[B"
	case "right":
		key = "\x1b[C"
	case "left":
		key = "\x1b[D"
	case "esc", "escape":
		key = "\x1b"
	default:
		if strings.HasPrefix(key, "ctrl+") {
			c := strings.TrimPrefix(key, "ctrl+")
			if len(c) != 1 || c[0] < 'a' || c[0] > 'z' {
				return fmt.Errorf("%w: unknown key %q", fakestate.ErrBadRequest, key)
			}
			key = string([]byte{c[0] - 'a' + 1})
		}
	}
	return writePane(dir, b.ID, key)
}

func writePane(dir, bid, data string) error {
	f, err := os.OpenFile(fakestate.InPath(dir, bid), os.O_RDWR, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: block %s has no stdin", fakestate.ErrNotFound, bid)
		}
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.WriteString(f, data)
	return err
}

func signalPane(b *fakestate.Block, sig syscall.Signal) {
	if b.PID > 0 {
		_ = syscall.Kill(-b.PID, sig)
	}
}

// stopPane ends a closed block's process group. A command that ignores
// SIGTERM must not outlive its record, so a bounded grace period is followed
// by SIGKILL. The wait ends early when the wrapper records the exit.
func stopPane(dir, blockID string, pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(fakestate.ExitPath(dir, blockID)); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

func killSession(dir string, s *fakestate.Session) {
	for _, w := range s.Windows {
		for _, b := range w.Blocks {
			signalPane(b, syscall.SIGTERM)
		}
	}
}

// startPane launches the block's argv detached. The output file is created
// before the child runs, and the FIFO is opened read-write so neither side
// blocks on open after the launching CLI exits.
func startPane(dir string, b *fakestate.Block) (*fakestate.Block, error) {
	if len(b.Command) == 0 {
		b.Command = []string{"/bin/sh"}
	}
	if err := os.MkdirAll(filepath.Join(dir, "out"), 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "in"), 0o700); err != nil {
		return nil, err
	}
	outPath := fakestate.OutPath(dir, b.ID)
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	defer func() { _ = out.Close() }()
	fifo := fakestate.InPath(dir, b.ID)
	_ = os.Remove(fifo)
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		return nil, err
	}
	stdin, err := os.OpenFile(fifo, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stdin.Close() }()

	// The wrapper records the program's exit code, which outlives this CLI
	// process; "$@" is the launched argv. The trap keeps a group signal from
	// killing the wrapper before it can record the exit. The record is moved
	// into place so a poller never reads a half-written file.
	const wrapper = `put_exit() { printf '%d' "$1" > "$FAKEREX_EXIT.tmp" && mv "$FAKEREX_EXIT.tmp" "$FAKEREX_EXIT"; }
trap 'put_exit 130; exit 130' INT TERM
"$@"; put_exit $?`
	cmd := exec.Command("/bin/sh", append([]string{"-c", wrapper, "fakerex"}, b.Command...)...)
	cmd.Env = append(os.Environ(), "FAKEREX_EXIT="+fakestate.ExitPath(dir, b.ID))
	cmd.Dir = b.Cwd
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w: launch %v: %v", fakestate.ErrFail, b.Command, err)
	}
	b.PID = cmd.Process.Pid
	_ = cmd.Process.Release()
	return b, nil
}

func foreground(b *fakestate.Block) map[string]any {
	name := ""
	if len(b.Command) > 0 {
		name = filepath.Base(b.Command[0])
	}
	return map[string]any{
		"pid": b.PID, "user_id": os.Getuid(), "user": "",
		"name": name, "path": b.Command[0], "argv0": b.Command[0],
		"cwd": b.Cwd,
	}
}

// lastExit reads the wrapper's exit file. It is nil while the program runs.
func lastExit(dir string, b *fakestate.Block) any {
	data, err := os.ReadFile(fakestate.ExitPath(dir, b.ID))
	if err != nil {
		return nil
	}
	code := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &code); err != nil {
		return nil
	}
	return map[string]any{"exit_code": code}
}

func nextID(st *fakestate.State, kind string) string {
	id := fmt.Sprintf("%s:%d", kind, st.Next)
	st.Next++
	return id
}

func removeSession(sessions []*fakestate.Session, id string) []*fakestate.Session {
	kept := sessions[:0]
	for _, s := range sessions {
		if s.ID != id {
			kept = append(kept, s)
		}
	}
	return kept
}

func str(m map[string]any, k string) string { return stringOf(m[k]) }

func stringOf(v any) string {
	s, _ := v.(string)
	return s
}

func intOf(m map[string]any, k string) int {
	n, _ := m[k].(float64)
	return int(n)
}

func boolOf(m map[string]any, k string) bool {
	b, _ := m[k].(bool)
	return b
}

func stringsOf(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
