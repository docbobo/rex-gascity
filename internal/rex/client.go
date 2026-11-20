// Package rex is the transport boundary to the Rex server.
//
// The provider core depends only on Client. v1 implements Client by shelling
// out to the `rex` CLI; a future implementation can talk Rex's unix socket
// directly. No other package should call the CLI.
package rex

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrNotFound is returned when Rex reports no such session, window, block or
// stored result (the CLI exits 3).
var ErrNotFound = errors.New("rex: not found")

// ErrUnavailable is returned when the Rex client cannot reach a server: the
// socket is absent, the server is down, or the connection fails. The rex CLI
// reports this as exit 4 ("no Rex server is running at ..."), distinct from a
// server-side failure (exit 1) and a missing target (exit 3).
var ErrUnavailable = errors.New("rex: server unavailable")

type (
	SessionID string
	WindowID  string
	BlockID   string
)

// Layout is the Rex wire layout of a new pane: a tree whose leaf is a block.
// A single-pane launch carries one block node.
type Layout struct {
	Block *LayoutBlock `json:"block,omitempty"`
}

// LayoutBlock is a block node: a flavor plus its options and label.
type LayoutBlock struct {
	Flavor  string         `json:"flavor"`
	Options map[string]any `json:"options,omitempty"`
	Label   string         `json:"label,omitempty"`
}

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
	// Focus makes the new tab active. The provider sets it only when it creates
	// a brand-new session's tab, so the operator lands there; re-attaching or
	// relaunching an existing session leaves the user's view alone.
	Focus bool
	// Layout is the prepared Rex block layout for this launch. internal/launch
	// builds it; the transport only serializes it.
	Layout Layout
}

// Block is a Rex pane.
type Block struct {
	ID    BlockID
	Label string
	// Owner is the client the block is displayed for, empty when no client has
	// displayed it: an app client can be attached to the session while a box it
	// never showed still has no owner.
	Owner string
	// Rect is the pane's normalized position in its window: x, y, width,
	// height, each in 0..1. It is the zero value when the layout is unknown.
	Rect [4]float64
}

// Window is a Rex tab. Blocks is every block in the window flattened across
// its layers; FocusedBlockID is the block that receives typing.
type Window struct {
	ID             WindowID
	Label          string
	FocusedBlockID BlockID
	Blocks         []Block
}

// ClientInfo identifies a client attached to a session. Kind is Rex info.kind:
// "app" is a human app client, "cli" is a CLI connection. Name is info.name.
type ClientInfo struct {
	ID   string
	Kind string
	Name string
}

// ProgramStatusRecord is one program-reported status row (OSC 7501).
type ProgramStatusRecord struct {
	ID    string
	State string // idle|working|done|blocked|error
	App   string
	// Kind refines a state, most importantly the interaction a blocked
	// program waits on: permission, question or auth. Empty when the
	// program reports none.
	Kind      string
	UpdatedAt time.Time
}

// ProgramStatus is a pane's program_status method reply.
type ProgramStatus struct {
	Records []ProgramStatusRecord
}

// SessionView is the session snapshot from session.view.
type SessionView struct {
	SessionID      SessionID
	Label          string
	ActiveWindowID WindowID
	Revision       int64
	Windows        []Window
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
	Script string `json:"script,omitempty"` // interpreter script path, when the process runs one
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
	// ListClients returns the clients attached to a session. It is the
	// attachment source: session.view has no clients field.
	ListClients(ctx context.Context, sessionID SessionID) ([]ClientInfo, error)

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
	// BlockProgramStatus returns the program-reported status of a pane.
	BlockProgramStatus(ctx context.Context, sessionID SessionID, blockID BlockID) (ProgramStatus, error)

	Events(ctx context.Context, sessionID SessionID) (<-chan Event, error)
}
