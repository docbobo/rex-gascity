// Package cli implements rex.Client by shelling out to the `rex` CLI.
//
// It is the v1 transport. The provider core depends only on rex.Client, so a
// future wire client can replace this package without touching provider logic.
// Every operation is one short-lived `rex` process.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

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

// NewWindow creates a new tab holding one agent pane.
func (c *Client) NewWindow(ctx context.Context, sessionID rex.SessionID, label string, spec rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	var reply struct {
		WindowID string   `json:"window_id"`
		BlockIDs []string `json:"block_ids"`
	}
	args := map[string]any{
		"session_id":   string(sessionID),
		"window_label": label,
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
func (c *Client) BlockProcess(ctx context.Context, block rex.BlockID) (rex.ProcessInfo, error) {
	var reply struct {
		LastExit *struct {
			ExitCode int    `json:"exit_code"`
			Error    string `json:"error"`
		} `json:"last_exit"`
		Foreground *rex.Process  `json:"foreground"`
		Ancestors  []rex.Process `json:"ancestors"`
	}
	if err := c.blockMethod(ctx, block, "process", nil, &reply); err != nil {
		return rex.ProcessInfo{}, err
	}
	info := rex.ProcessInfo{Foreground: reply.Foreground, Ancestors: reply.Ancestors}
	if reply.LastExit != nil {
		info.LastExit = &rex.ExitStatus{ExitCode: reply.LastExit.ExitCode, Error: reply.LastExit.Error}
	}
	return info, nil
}

// BlockFormat returns a pane's rendered output, following paged replies with
// blob_read until the whole result has been read.
func (c *Client) BlockFormat(ctx context.Context, block rex.BlockID) (string, error) {
	var first struct {
		Content string `json:"content"`
		ID      int64  `json:"id"`
	}
	args := map[string]any{"format": "text", "trim": true}
	if err := c.blockMethod(ctx, block, "format", args, &first); err != nil {
		return "", err
	}
	if first.ID == 0 {
		return first.Content, nil
	}

	var out strings.Builder
	out.WriteString(first.Content)
	offset := len(first.Content)
	id := first.ID
	for {
		var page struct {
			Data  string `json:"data"`
			ID    int64  `json:"id"`
			Total int    `json:"total"`
		}
		args := map[string]any{"id": id, "offset": offset}
		if err := c.blockMethod(ctx, block, "blob_read", args, &page); err != nil {
			return "", err
		}
		out.WriteString(page.Data)
		offset += len(page.Data)
		if page.Data == "" || page.Total <= offset {
			break
		}
		id = page.ID
	}
	return out.String(), nil
}

// BlockWrite types text into a pane.
func (c *Client) BlockWrite(ctx context.Context, block rex.BlockID, data string) error {
	return c.blockMethod(ctx, block, "write", map[string]any{"data": data}, nil)
}

// BlockEvent sends an input event to a pane.
func (c *Client) BlockEvent(ctx context.Context, block rex.BlockID, ev rex.Event) error {
	return c.call(ctx, "block.event", map[string]any{
		"block_id": string(block),
		"event":    ev,
	}, nil)
}

// BlockClear clears a pane's scrollback.
func (c *Client) BlockClear(ctx context.Context, block rex.BlockID) error {
	return c.blockMethod(ctx, block, "clear", nil, nil)
}

// Events is not implemented in v1; activity tracking arrives in Phase 3.
func (c *Client) Events(context.Context, rex.SessionID) (<-chan rex.Event, error) {
	return nil, fmt.Errorf("cli: Events is not implemented")
}

// blockMethod calls a terminal block method via the block.method API.
func (c *Client) blockMethod(ctx context.Context, block rex.BlockID, method string, args, out any) error {
	call := map[string]any{
		"block_id": string(block),
		"method":   method,
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
