package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/docbobo/rex-gas-city/internal/rex"
)

// nameKey is the file stem every file store uses for name: the lowercase hex
// sha256 of the gc session name.
func nameKey(name SessionName) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:])
}

// writeJSON writes v to path atomically: it creates the parent dir, writes a
// temp file in the same dir, then renames it over path.
func writeJSON(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// readJSON decodes the file at path into v. A missing file is (false, nil); a
// corrupt file is an error so the caller decides what to do.
func readJSON(path string, v any) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return false, err
	}
	return true, nil
}

// bindingRecord is the on-disk shape of a Binding.
type bindingRecord struct {
	GCName       string    `json:"gc_name"`
	SessionID    string    `json:"rex_session_id"`
	SessionLabel string    `json:"rex_session_label"`
	WindowID     string    `json:"window_id"`
	BlockID      string    `json:"block_id"`
	LaunchMode   string    `json:"launch_mode"`
	CreatedAt    time.Time `json:"created_at"`
	ReadyPrompt  string    `json:"ready_prompt"`
}

// FileStore is a BindingStore that keeps one JSON file per binding under a
// directory, so bindings survive a process restart. It is safe for concurrent
// use within a process; writes are atomic renames.
type FileStore struct {
	mu  sync.Mutex
	dir string
}

// NewFileStore returns a FileStore rooted at dir. The directory is created on
// the first Save.
func NewFileStore(dir string) *FileStore { return &FileStore{dir: dir} }

func (s *FileStore) path(name SessionName) string {
	return filepath.Join(s.dir, nameKey(name)+".json")
}

// Load implements BindingStore. A missing, unreadable or corrupt file reads
// as not found.
func (s *FileStore) Load(name SessionName) (Binding, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r bindingRecord
	ok, err := readJSON(s.path(name), &r)
	if err != nil || !ok {
		return Binding{}, false
	}
	return Binding{
		Name:         SessionName(r.GCName),
		SessionID:    rex.SessionID(r.SessionID),
		SessionLabel: r.SessionLabel,
		WindowID:     rex.WindowID(r.WindowID),
		BlockID:      rex.BlockID(r.BlockID),
		LaunchMode:   r.LaunchMode,
		CreatedAt:    r.CreatedAt,
		ReadyPrompt:  r.ReadyPrompt,
	}, true
}

// Save implements BindingStore. The interface has no error return, so a
// failed write is dropped; the next Resolve rebuilds the binding by scanning.
func (s *FileStore) Save(b Binding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = writeJSON(s.path(b.Name), bindingRecord{
		GCName:       string(b.Name),
		SessionID:    string(b.SessionID),
		SessionLabel: b.SessionLabel,
		WindowID:     string(b.WindowID),
		BlockID:      string(b.BlockID),
		LaunchMode:   b.LaunchMode,
		CreatedAt:    b.CreatedAt,
		ReadyPrompt:  b.ReadyPrompt,
	})
}

// Delete implements BindingStore. A missing file is not an error.
func (s *FileStore) Delete(name SessionName) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = os.Remove(s.path(name))
}
