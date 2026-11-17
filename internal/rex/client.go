// Package rex is the transport boundary to the Rex server.
//
// The provider core depends only on Client. v1 implements Client by shelling
// out to the `rex` CLI; a future implementation can talk Rex's unix socket
// directly. No other package should call the CLI.
package rex

import (
	"context"
	"encoding/json"
)

type (
	SessionID string
	WindowID  string
	BlockID   string
)

// Session is one Rex session (D1: one per gc city).
type Session struct {
	ID    SessionID
	Label string
}

// LaunchSpec is the launch configuration for a terminal block/pane.
type LaunchSpec struct {
	Command     []string // argv to run; empty starts the pane's shell
	Shell       string   // "" default shell, "none" runs Command directly
	Cwd         string
	Login       *bool
	Interactive bool
	Label       string // the pane label (the gc agent name)
}

// Block is a Rex pane.
type Block struct {
	ID    BlockID
	Label string
}

// Window is a Rex tab. Blocks is every block in the window flattened across
// its layers; FocusedBlockID is the block that receives typing.
type Window struct {
	ID             WindowID
	Label          string
	FocusedBlockID BlockID
	Blocks         []Block
}

// ClientInfo identifies a client attached to a session.
type ClientInfo struct {
	ID string
}

// SessionView is the session snapshot from session.view.
type SessionView struct {
	SessionID      SessionID
	Label          string
	ActiveWindowID WindowID
	Revision       int64
	Windows        []Window
	Clients        []ClientInfo
}

// ExitStatus is how a terminal's program exited.
type ExitStatus struct {
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error"`
}

// Process is one process in a terminal's process tree.
type Process struct {
	PID    int    `json:"pid"`
	UserID int    `json:"user_id"`
	User   string `json:"user"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Argv0  string `json:"argv0"`
	Cwd    string `json:"cwd"`
}

// ProcessInfo is the result of the terminal's `process` method.
type ProcessInfo struct {
	LastExit   *ExitStatus // nil while the program is still running
	Foreground *Process
	Ancestors  []Process
}

// Event is a Rex event. Payload is the raw event fields.
type Event struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Client is the Rex control surface the provider uses. It is SDK-style: one
// method per Rex operation, never a generic run(args...).
type Client interface {
	CreateSession(ctx context.Context, label string) (SessionID, error)
	NewWindow(ctx context.Context, sessionID SessionID, label string, spec LaunchSpec) (WindowID, BlockID, error)
	NewSplit(ctx context.Context, sessionID SessionID, anchor BlockID, direction, side string, spec LaunchSpec) (BlockID, error)
	CloseBlock(ctx context.Context, sessionID SessionID, blockID BlockID) error
	SessionView(ctx context.Context, sessionID SessionID) (SessionView, error)
	ListSessions(ctx context.Context) ([]Session, error)

	// Block methods address a block through the session that holds it: Rex
	// requires session_id on block.method and block.event.
	BlockProcess(ctx context.Context, sessionID SessionID, blockID BlockID) (ProcessInfo, error)
	BlockFormat(ctx context.Context, sessionID SessionID, blockID BlockID) (string, error)
	BlockWrite(ctx context.Context, sessionID SessionID, blockID BlockID, data string) error
	// SendKeys presses named keys in the pane using Rex's key-binding syntax
	// ("ctrl+c", "enter", "up"), without appending a submit.
	SendKeys(ctx context.Context, sessionID SessionID, blockID BlockID, keys ...string) error
	BlockEvent(ctx context.Context, sessionID SessionID, blockID BlockID, ev Event) error
	BlockClear(ctx context.Context, sessionID SessionID, blockID BlockID) error

	Events(ctx context.Context, sessionID SessionID) (<-chan Event, error)
}
