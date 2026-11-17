// Package cli implements rex.Client by shelling out to the `rex` CLI.
//
// It is the v1 transport. The provider core depends only on rex.Client, so a
// future wire client can replace this package without touching provider logic.
// Every operation is one short-lived `rex` process.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/docbobo/rex-gas-city/internal/rex"
)

// terminalFlavor is the Rex flavor that runs a shell or a chosen command.
const terminalFlavor = "com.superlogical.terminal.shell"

// Client drives the Rex server through the rex CLI. Bin defaults to "rex".
type Client struct {
	Bin string
}

// New returns a Client that runs `rex` from PATH.
func New() *Client { return &Client{Bin: "rex"} }

func (c *Client) bin() string {
	if c.Bin == "" {
		return "rex"
	}
	return c.Bin
}

// run executes rex and returns stdout. A non-zero exit becomes an error that
// carries the command's stderr.
func (c *Client) run(ctx context.Context, argv ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.bin(), argv...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 3 {
			if msg != "" {
				return nil, fmt.Errorf("rex %s: %w: %s", strings.Join(argv, " "), rex.ErrNotFound, msg)
			}
			return nil, fmt.Errorf("rex %s: %w", strings.Join(argv, " "), rex.ErrNotFound)
		}
		if msg != "" {
			return nil, fmt.Errorf("rex %s: %w: %s", strings.Join(argv, " "), err, msg)
		}
		return nil, fmt.Errorf("rex %s: %w", strings.Join(argv, " "), err)
	}
	return out, nil
}

// call runs `rex api call <method> [<json>]` and decodes the reply into out
// when out is non-nil.
func (c *Client) call(ctx context.Context, method string, args, out any) error {
	argv := []string{"api", "call", method}
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return fmt.Errorf("marshal %s args: %w", method, err)
		}
		argv = append(argv, string(b))
	}
	raw, err := c.run(ctx, argv...)
	if err != nil {
		return err
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode %s reply: %w", method, err)
		}
	}
	return nil
}

// CreateSession creates the city session.
func (c *Client) CreateSession(ctx context.Context, label string) (rex.SessionID, error) {
	var reply struct {
		SessionID string `json:"session_id"`
	}
	if err := c.call(ctx, "session.create", map[string]any{"label": label}, &reply); err != nil {
		return "", err
	}
	return rex.SessionID(reply.SessionID), nil
}

// ListSessions lists every session on the server.
func (c *Client) ListSessions(ctx context.Context) ([]rex.Session, error) {
	var reply struct {
		Sessions []struct {
			ID    string `json:"session_id"`
			Label string `json:"label"`
		} `json:"sessions"`
	}
	if err := c.call(ctx, "session.list", nil, &reply); err != nil {
		return nil, err
	}
	sessions := make([]rex.Session, 0, len(reply.Sessions))
	for _, s := range reply.Sessions {
		sessions = append(sessions, rex.Session{ID: rex.SessionID(s.ID), Label: s.Label})
	}
	return sessions, nil
}

// ListClients returns the clients attached to a session.
func (c *Client) ListClients(ctx context.Context, sessionID rex.SessionID) ([]rex.ClientInfo, error) {
	var reply struct {
		Clients []struct {
			ID   string `json:"client_id"`
			Info struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"info"`
		} `json:"clients"`
	}
	if err := c.call(ctx, "session.list_clients", map[string]any{"session_id": string(sessionID)}, &reply); err != nil {
		return nil, err
	}
	clients := make([]rex.ClientInfo, 0, len(reply.Clients))
	for _, cl := range reply.Clients {
		clients = append(clients, rex.ClientInfo{ID: cl.ID, Kind: cl.Info.Kind, Name: cl.Info.Name})
	}
	return clients, nil
}

// NewWindow creates a new tab holding one agent pane.
func (c *Client) NewWindow(ctx context.Context, sessionID rex.SessionID, label string, spec rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	var reply struct {
		WindowID string   `json:"window_id"`
		BlockIDs []string `json:"block_ids"`
	}
	args := map[string]any{
		"session_id":   string(sessionID),
		"window_label": label,
		"focus":        spec.Focus,
		"layout":       blockLayout(spec),
	}
	if err := c.call(ctx, "session.new_window", args, &reply); err != nil {
		return "", "", err
	}
	var block rex.BlockID
	if len(reply.BlockIDs) > 0 {
		block = rex.BlockID(reply.BlockIDs[0])
	}
	return rex.WindowID(reply.WindowID), block, nil
}

// NewSplit adds an agent pane beside an existing pane in the rig's tab.
func (c *Client) NewSplit(ctx context.Context, sessionID rex.SessionID, anchor rex.BlockID, direction, side string, spec rex.LaunchSpec) (rex.BlockID, error) {
	var reply struct {
		BlockIDs []string `json:"block_ids"`
	}
	args := map[string]any{
		"session_id": string(sessionID),
		"anchor_id":  string(anchor),
		"direction":  direction,
		"side":       side,
		"layout":     blockLayout(spec),
		"focus":      spec.Focus,
	}
	if err := c.call(ctx, "session.new_split", args, &reply); err != nil {
		return "", err
	}
	if len(reply.BlockIDs) == 0 {
		return "", nil
	}
	return rex.BlockID(reply.BlockIDs[0]), nil
}

// CloseBlock closes an agent pane.
func (c *Client) CloseBlock(ctx context.Context, sessionID rex.SessionID, block rex.BlockID) error {
	return c.call(ctx, "block.close", map[string]any{
		"session_id": string(sessionID),
		"block_id":   string(block),
	}, nil)
}

// SessionView returns the session's windows and their blocks.
func (c *Client) SessionView(ctx context.Context, sessionID rex.SessionID) (rex.SessionView, error) {
	var reply struct {
		SessionID      string `json:"session_id"`
		Label          string `json:"label"`
		ActiveWindowID string `json:"active_window_id"`
		Revision       int64  `json:"revision"`
		Windows        []struct {
			ID             string `json:"window_id"`
			Label          string `json:"label"`
			FocusedBlockID string `json:"focused_block_id"`
			Layers         []struct {
				Blocks []struct {
					ID    string `json:"block_id"`
					Label string `json:"label"`
				} `json:"blocks"`
			} `json:"layers"`
		} `json:"windows"`
	}
	if err := c.call(ctx, "session.view", map[string]any{"session_id": string(sessionID)}, &reply); err != nil {
		return rex.SessionView{}, err
	}
	view := rex.SessionView{
		SessionID:      rex.SessionID(reply.SessionID),
		Label:          reply.Label,
		ActiveWindowID: rex.WindowID(reply.ActiveWindowID),
		Revision:       reply.Revision,
	}
	for _, w := range reply.Windows {
		win := rex.Window{
			ID:             rex.WindowID(w.ID),
			Label:          w.Label,
			FocusedBlockID: rex.BlockID(w.FocusedBlockID),
		}
		for _, layer := range w.Layers {
			for _, b := range layer.Blocks {
				win.Blocks = append(win.Blocks, rex.Block{ID: rex.BlockID(b.ID), Label: b.Label})
			}
		}
		view.Windows = append(view.Windows, win)
	}
	return view, nil
}

// BlockProcess returns the process tree of a pane.
func (c *Client) BlockProcess(ctx context.Context, sessionID rex.SessionID, block rex.BlockID) (rex.ProcessInfo, error) {
	var reply struct {
		LastExit *struct {
			ExitCode int    `json:"exit_code"`
			Error    string `json:"error"`
		} `json:"last_exit"`
		Foreground *rex.Process  `json:"foreground"`
		Ancestors  []rex.Process `json:"ancestors"`
	}
	if err := c.blockMethod(ctx, sessionID, block, "process", nil, &reply); err != nil {
		return rex.ProcessInfo{}, err
	}
	info := rex.ProcessInfo{Foreground: reply.Foreground, Ancestors: reply.Ancestors}
	if reply.LastExit != nil {
		info.LastExit = &rex.ExitStatus{ExitCode: reply.LastExit.ExitCode, Error: reply.LastExit.Error}
	}
	return info, nil
}

// formatMaxBytes is the most content a single format reply returns. Rex caps
// it at 983040 (960 KiB); asking for it makes Rex page large captures instead
// of failing with too_large, and the paging flag (`id`) then drives blob_read.
const formatMaxBytes = 983040

// formatPage is one format or blob_read reply. id is set only while more
// output remains; eof marks the last page.
type formatPage struct {
	Content string `json:"content,omitempty"`
	Data    string `json:"data,omitempty"`
	ID      int64  `json:"id"`
	Total   int64  `json:"total"`
	EOF     bool   `json:"eof"`
}

// BlockFormat returns a pane's whole rendered output, following paged replies
// with blob_read until the result is complete.
func (c *Client) BlockFormat(ctx context.Context, sessionID rex.SessionID, block rex.BlockID) (string, error) {
	var first formatPage
	args := map[string]any{"format": "text", "trim": true, "max_bytes": formatMaxBytes}
	if err := c.blockMethod(ctx, sessionID, block, "format", args, &first); err != nil {
		return "", err
	}

	var out strings.Builder
	out.WriteString(first.Content)
	offset := len(first.Content)
	// The format reply carries an id only while more output remains. Read the
	// rest one page at a time, always from the same result id.
	for id := first.ID; id != 0; {
		var page formatPage
		args := map[string]any{"id": id, "offset": offset}
		if err := c.blockMethod(ctx, sessionID, block, "blob_read", args, &page); err != nil {
			return "", err
		}
		out.WriteString(page.Data)
		offset += len(page.Data)
		if page.EOF || page.Data == "" {
			break
		}
		id = page.ID
	}
	return out.String(), nil
}

// BlockWrite types text into a pane.
func (c *Client) BlockWrite(ctx context.Context, sessionID rex.SessionID, block rex.BlockID, data string) error {
	return c.blockMethod(ctx, sessionID, block, "write", map[string]any{"data": data}, nil)
}

// SendKeys presses named keys in a pane via `rex send-key`. Rex resolves the
// key bindings, so the adapter passes Rex key syntax straight through; the
// session scope is required next to the block. No keys is a no-op.
func (c *Client) SendKeys(ctx context.Context, sessionID rex.SessionID, block rex.BlockID, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	argv := []string{"send-key", "-s", string(sessionID), "-b", string(block), "--"}
	argv = append(argv, keys...)
	_, err := c.run(ctx, argv...)
	return err
}

// BlockEvent sends an input event to a pane.
func (c *Client) BlockEvent(ctx context.Context, sessionID rex.SessionID, block rex.BlockID, ev rex.Event) error {
	return c.call(ctx, "block.event", map[string]any{
		"session_id": string(sessionID),
		"block_id":   string(block),
		"event":      ev,
	}, nil)
}

// BlockClear clears a pane's scrollback.
func (c *Client) BlockClear(ctx context.Context, sessionID rex.SessionID, block rex.BlockID) error {
	return c.blockMethod(ctx, sessionID, block, "clear", nil, nil)
}

// BlockProgramStatus returns the program-reported status records of a pane.
// Records with an absent updated_at keep the zero time.
func (c *Client) BlockProgramStatus(ctx context.Context, sessionID rex.SessionID, block rex.BlockID) (rex.ProgramStatus, error) {
	var reply struct {
		Records []struct {
			ID        string `json:"id"`
			State     string `json:"state"`
			App       string `json:"app"`
			UpdatedAt string `json:"updated_at"`
		} `json:"records"`
	}
	if err := c.blockMethod(ctx, sessionID, block, "program_status", nil, &reply); err != nil {
		return rex.ProgramStatus{}, err
	}
	status := rex.ProgramStatus{Records: make([]rex.ProgramStatusRecord, 0, len(reply.Records))}
	for _, r := range reply.Records {
		rec := rex.ProgramStatusRecord{ID: r.ID, State: r.State, App: r.App}
		if r.UpdatedAt != "" {
			t, err := time.Parse(time.RFC3339, r.UpdatedAt)
			if err != nil {
				return rex.ProgramStatus{}, fmt.Errorf("decode program_status updated_at %q: %w", r.UpdatedAt, err)
			}
			rec.UpdatedAt = t
		}
		status.Records = append(status.Records, rec)
	}
	return status, nil
}

// Events is not implemented in v1; activity tracking arrives in Phase 3.
func (c *Client) Events(context.Context, rex.SessionID) (<-chan rex.Event, error) {
	return nil, fmt.Errorf("cli: Events is not implemented")
}

// blockMethod calls a terminal block method via the block.method API. Rex
// requires session_id next to block_id, even though the block id is unique.
func (c *Client) blockMethod(ctx context.Context, sessionID rex.SessionID, block rex.BlockID, method string, args, out any) error {
	call := map[string]any{
		"session_id": string(sessionID),
		"block_id":   string(block),
		"method":     method,
	}
	if args != nil {
		call["args"] = args
	}
	return c.call(ctx, "block.method", call, out)
}

// layoutNode is one node of a Rex layout: a block or a split.
type layoutNode struct {
	Block *layoutBlock `json:"block,omitempty"`
}

// layoutBlock is a block node: a flavor plus its options and label.
type layoutBlock struct {
	Flavor  string         `json:"flavor"`
	Options map[string]any `json:"options,omitempty"`
	Label   string         `json:"label,omitempty"`
}

// blockLayout builds a single-pane layout from a launch spec.
func blockLayout(spec rex.LaunchSpec) layoutNode {
	opts := map[string]any{}
	if len(spec.Command) > 0 {
		opts["command"] = spec.Command
	}
	if spec.Shell != "" {
		opts["shell"] = spec.Shell
	}
	if spec.Cwd != "" {
		opts["cwd"] = spec.Cwd
	}
	if spec.Login != nil {
		opts["login"] = *spec.Login
	}
	if spec.Interactive {
		opts["interactive"] = true
	}
	node := layoutBlock{Flavor: terminalFlavor}
	if len(opts) > 0 {
		node.Options = opts
	}
	if spec.Label != "" {
		node.Label = spec.Label
	}
	return layoutNode{Block: &node}
}
