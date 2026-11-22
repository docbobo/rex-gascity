// Package fakestate is the on-disk state of the fake rex binary: one JSON
// document per state dir plus append-only pane output files. A file lock makes
// every short-lived rex invocation a safe read-modify-write, so concurrent CLI
// calls see a consistent server. It is test-only and never shipped.
package fakestate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Errors the CLI maps to its contract exit codes.
var (
	// ErrNotFound is a missing session, window, block, result or method
	// (exit 3).
	ErrNotFound = errors.New("not found")
	// ErrBadRequest is a client-side usage or validation error (exit 2).
	ErrBadRequest = errors.New("bad request")
	// ErrFail is a server-side failure (exit 1).
	ErrFail = errors.New("server failure")
)

// Methods is every api method the fake answers. fakebinary gates `api call`
// and `api describe` on it, and TestMethodTableMatchesContract compares it
// with docs/REX-CONTRACT.md section 3.
var Methods = []string{
	"server.version",
	"session.create", "session.destroy", "session.list",
	"session.list_clients", "session.view",
	"session.new_window", "session.new_split",
	"session.focus_block", "session.focus_window",
	"session.set_block_label", "session.set_window_label", "session.set_label",
	"block.close", "block.event", "block.method",
	"target.resolve", "target.complete",
}

// BlockMethods is every block.method sub-method the fake answers. fakebinary
// gates block calls on it, and the drift guard compares it with
// docs/REX-CONTRACT.md section 4.
var BlockMethods = []string{
	"process", "format", "blob_read", "write", "clear", "reset",
	"program_status", "input_mode", "size", "title", "pwd",
}

// Fault kinds a test can queue per method call.
const (
	// FaultError makes the call fail server-side (exit 1) before it runs.
	FaultError = "error"
	// FaultUnreachable makes the CLI report no server (exit 4).
	FaultUnreachable = "unreachable"
	// FaultLostReply runs the call, then drops the reply (exit 1).
	FaultLostReply = "lost_reply"
	// FaultDelay stalls the call before it runs, so a context can cancel it.
	FaultDelay = "delay"
	// FaultGarbled runs the call, then writes invalid JSON as the reply.
	FaultGarbled = "garbled"
)

// Fault is one scripted misbehaviour for a method call. Count is how many more
// calls it applies to; 0 or 1 means the next call only.
type Fault struct {
	Method string `json:"method"`
	Kind   string `json:"kind"`
	Count  int    `json:"count,omitempty"`
}

// State is the whole fake server.
type State struct {
	Revision int64 `json:"revision"`
	Next     int   `json:"next"`
	// APIVersion overrides the server's reported api_revision. Zero means the
	// fake's default (14). Tests set it to drive the version-skew case.
	APIVersion int        `json:"api_version,omitempty"`
	Sessions   []*Session `json:"sessions,omitempty"`
	// Results holds paged block output. The fake keeps one result per call, so
	// a single counter is enough.
	Results map[string]int64 `json:"results,omitempty"`
}

// Session is one rex session.
type Session struct {
	ID           string    `json:"session_id"`
	Label        string    `json:"label"`
	ActiveWindow string    `json:"active_window_id,omitempty"`
	Windows      []*Window `json:"windows,omitempty"`
	Clients      []Client  `json:"clients,omitempty"`
}

// Window is one rex tab: a layout tree and the blocks it holds.
type Window struct {
	ID           string   `json:"window_id"`
	Label        string   `json:"label"`
	FocusedBlock string   `json:"focused_block_id,omitempty"`
	Layout       *Node    `json:"layout,omitempty"`
	Blocks       []*Block `json:"blocks,omitempty"`
}

// Block is one rex pane and the process running in it.
type Block struct {
	ID            string   `json:"block_id"`
	Label         string   `json:"label,omitempty"`
	Owner         string   `json:"owner,omitempty"`
	Flavor        string   `json:"flavor,omitempty"`
	Command       []string `json:"command,omitempty"`
	Cwd           string   `json:"cwd,omitempty"`
	PID           int      `json:"pid,omitempty"`
	ProgramStatus []Record `json:"program_status,omitempty"`
}

// Node is one layout tree node: a block leaf, or a binary split.
type Node struct {
	Rect    [4]float64 `json:"rect"`
	BlockID string     `json:"block_id,omitempty"`
	Split   *Split     `json:"split,omitempty"`
}

// Split is the two children of a layout split.
type Split struct {
	Before *Node `json:"before"`
	After  *Node `json:"after"`
}

// Client is one client attached to a session.
type Client struct {
	ID   string `json:"client_id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// Record is one program-status row (OSC 7501).
type Record struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	App       string `json:"app,omitempty"`
	Kind      string `json:"kind,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// Load reads the state in dir. A missing file is an empty state; a missing dir
// means no fake server is running.
func Load(dir string) (*State, error) {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("%w: no state dir at %s", ErrNotFound, dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return &State{Next: 1, Results: map[string]int64{}}, nil
	}
	if err != nil {
		return nil, err
	}
	st := &State{}
	if err := json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("decode state: %w", err)
	}
	if st.Next == 0 {
		st.Next = 1
	}
	if st.Results == nil {
		st.Results = map[string]int64{}
	}
	return st, nil
}

// save writes st atomically, so a reader never sees a partial document.
func save(dir string, st *State) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "state.json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "state.json"))
}

// Update locks dir, loads the state, runs fn and saves the result.
func Update(dir string, fn func(*State) error) error {
	return lock(dir, func() error {
		st, err := Load(dir)
		if err != nil {
			return err
		}
		if err := fn(st); err != nil {
			return err
		}
		return save(dir, st)
	})
}

// lock runs fn under dir's exclusive flock, so concurrent invocations
// serialize. State and fault writes share it.
func lock(dir string, fn func() error) error {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("%w: no state dir at %s", ErrNotFound, dir)
	}
	lock, err := os.OpenFile(filepath.Join(dir, "state.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	for {
		err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	return fn()
}

// AddFaults appends faults to dir's queue. The fake binary consumes them one
// method call at a time.
func AddFaults(dir string, faults ...Fault) error {
	if len(faults) == 0 {
		return nil
	}
	return lock(dir, func() error {
		cur, err := loadFaults(dir)
		if err != nil {
			return err
		}
		return saveFaults(dir, append(cur, faults...))
	})
}

// TakeFault removes and returns the next queued fault for method, or nil when
// none is queued. A fault with Count > 1 stays queued with a lower count.
func TakeFault(dir, method string) (*Fault, error) {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		// No state dir is an unreachable server (exit 4). The method body
		// reports that, so this must not turn into a not-found lookup miss.
		return nil, nil
	}
	var taken *Fault
	err := lock(dir, func() error {
		faults, err := loadFaults(dir)
		if err != nil {
			return err
		}
		for i, f := range faults {
			if f.Method != method {
				continue
			}
			taken = &f
			if f.Count > 1 {
				faults[i].Count--
			} else {
				faults = append(faults[:i], faults[i+1:]...)
			}
			return saveFaults(dir, faults)
		}
		return nil
	})
	return taken, err
}

func loadFaults(dir string) ([]Fault, error) {
	data, err := os.ReadFile(filepath.Join(dir, "faults.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var faults []Fault
	if err := json.Unmarshal(data, &faults); err != nil {
		return nil, fmt.Errorf("decode faults: %w", err)
	}
	return faults, nil
}

func saveFaults(dir string, faults []Fault) error {
	data, err := json.Marshal(faults)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "faults.json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "faults.json"))
}

// Read loads the state under the shared lock. It is Update's read-only twin.
func Read(dir string, fn func(*State) error) error {
	return Update(dir, fn)
}

// OutPath is the file a block writes its stdout and stderr to.
func OutPath(dir, blockID string) string {
	return filepath.Join(dir, "out", clean(blockID)+".log")
}

// InPath is the FIFO a block reads its stdin from.
func InPath(dir, blockID string) string {
	return filepath.Join(dir, "in", clean(blockID)+".fifo")
}

// ExitPath is the file the launch wrapper writes the program's exit code to.
func ExitPath(dir, blockID string) string {
	return filepath.Join(dir, "out", clean(blockID)+".exit")
}

func clean(id string) string { return strings.ReplaceAll(id, ":", "_") }

// Session returns the session with id, or ErrNotFound.
func (s *State) Session(id string) (*Session, error) {
	for _, ses := range s.Sessions {
		if ses.ID == id {
			return ses, nil
		}
	}
	return nil, fmt.Errorf("%w: session %s", ErrNotFound, id)
}

// Block finds a block by id and the session and window that hold it.
func (s *State) Block(id string) (*Session, *Window, *Block, error) {
	for _, ses := range s.Sessions {
		for _, w := range ses.Windows {
			for _, b := range w.Blocks {
				if b.ID == id {
					return ses, w, b, nil
				}
			}
		}
	}
	return nil, nil, nil, fmt.Errorf("%w: block %s", ErrNotFound, id)
}

// AddWindow appends a window and returns it.
func (s *Session) AddWindow(w *Window) {
	s.Windows = append(s.Windows, w)
	if s.ActiveWindow == "" {
		s.ActiveWindow = w.ID
	}
}

// Remove drops a block. A window with no blocks is removed, as Rex does.
func (s *Session) Remove(id string) {
	for wi, w := range s.Windows {
		for bi, b := range w.Blocks {
			if b.ID == id {
				w.Blocks = append(w.Blocks[:bi], w.Blocks[bi+1:]...)
				w.Layout = RemoveLeaf(w.Layout, id)
				if len(w.Blocks) == 0 {
					s.Windows = append(s.Windows[:wi], s.Windows[wi+1:]...)
					if s.ActiveWindow == w.ID {
						s.ActiveWindow = ""
					}
				} else if w.FocusedBlock == id {
					w.FocusedBlock = w.Blocks[0].ID
				}
				return
			}
		}
	}
}

// RemoveLeaf drops the leaf for id from the tree, collapsing its parent split
// into the surviving sibling. The survivor takes the removed split's rect, and
// its descendants scale with it. With no removed leaf, all rects stay as they
// are.
func RemoveLeaf(n *Node, id string) *Node {
	n, _ = removeLeaf(n, id)
	return n
}

func removeLeaf(n *Node, id string) (*Node, bool) {
	if n == nil {
		return nil, false
	}
	if n.Split == nil {
		if n.BlockID == id {
			return nil, true
		}
		return n, false
	}
	if before, removed := removeLeaf(n.Split.Before, id); removed {
		if before == nil {
			n.Split.After.expand(n.Rect)
			return n.Split.After, true
		}
		n.Split.Before = before
		return n, true
	}
	after, removed := removeLeaf(n.Split.After, id)
	if !removed {
		return n, false
	}
	if after == nil {
		n.Split.Before.expand(n.Rect)
		return n.Split.Before, true
	}
	n.Split.After = after
	return n, true
}

// expand moves and scales the subtree from its own rect onto r, keeping each
// descendant's relative position and size.
func (n *Node) expand(r [4]float64) {
	from := n.Rect
	n.Rect = r
	if n.Split == nil || from[2] == 0 || from[3] == 0 {
		return
	}
	sx, sy := r[2]/from[2], r[3]/from[3]
	for _, child := range []*Node{n.Split.Before, n.Split.After} {
		child.expand([4]float64{
			r[0] + (child.Rect[0]-from[0])*sx,
			r[1] + (child.Rect[1]-from[1])*sy,
			child.Rect[2] * sx,
			child.Rect[3] * sy,
		})
	}
}

// Leaf returns the layout leaf for block id, or nil.
func (n *Node) Leaf(id string) *Node {
	if n == nil {
		return nil
	}
	if n.Split == nil {
		if n.BlockID == id {
			return n
		}
		return nil
	}
	if leaf := n.Split.Before.Leaf(id); leaf != nil {
		return leaf
	}
	return n.Split.After.Leaf(id)
}
