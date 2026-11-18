package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
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
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
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

// flatPath places a record at dir/<name hash>.json.
func flatPath(dir string) func(SessionName) string {
	return func(name SessionName) string { return filepath.Join(dir, nameKey(name)+".json") }
}

// fileTable is the one file-backed table behind every file store: JSON records
// addressed by a path function, written atomically, guarded by a mutex. The
// stores only add their key and record types.
type fileTable[K, R any] struct {
	mu   sync.Mutex
	dir  string
	path func(K) string
}

func newFileTable[K, R any](dir string, path func(K) string) *fileTable[K, R] {
	return &fileTable[K, R]{dir: dir, path: path}
}

// read returns the record for k. A missing file is (zero, false, nil); a
// corrupt or unreadable one is an error.
func (t *fileTable[K, R]) read(k K) (R, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var r R
	ok, err := readJSON(t.path(k), &r)
	return r, ok, err
}

func (t *fileTable[K, R]) write(k K, r R) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return writeJSON(t.path(k), r)
}

// remove deletes the record for k. A missing file is not an error.
func (t *fileTable[K, R]) remove(k K) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return removeIfPresent(t.path(k))
}

// removeAll deletes path and everything under it. A missing path is success.
func (t *fileTable[K, R]) removeAll(path string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return os.RemoveAll(path)
}

// list returns every record directly under dir, in no particular order.
// Missing and corrupt files are skipped.
func (t *fileTable[K, R]) list() []R {
	t.mu.Lock()
	defer t.mu.Unlock()
	paths, _ := filepath.Glob(filepath.Join(t.dir, "*.json"))
	out := []R{}
	for _, path := range paths {
		var r R
		if ok, err := readJSON(path, &r); err != nil || !ok {
			continue
		}
		out = append(out, r)
	}
	return out
}

func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// bindingRecord is the on-disk shape of a Binding. It has the same fields in
// the same order, so Go converts between the two and a new Binding field
// fails to compile until the record has it too. The newer fields are omitted
// when empty so old and new records stay compatible: a record without them
// decodes with empty values and callers fall back to the name-derived labels.
type bindingRecord struct {
	Name         SessionName   `json:"gc_name"`
	SessionID    rex.SessionID `json:"rex_session_id"`
	SessionLabel string        `json:"rex_session_label"`
	WindowID     rex.WindowID  `json:"window_id"`
	BlockID      rex.BlockID   `json:"block_id"`
	// TabLabel, PaneLabel and Aliases were added after the first records.
	TabLabel   string        `json:"tab_label,omitempty"`
	PaneLabel  string        `json:"pane_label,omitempty"`
	Aliases    []SessionName `json:"aliases,omitempty"`
	LaunchMode string        `json:"launch_mode"`
	CreatedAt  time.Time     `json:"created_at"`
	// GCBin, WorkDir and ClaimSessionID were added after the first records.
	GCBin          string `json:"gc_bin,omitempty"`
	WorkDir        string `json:"work_dir,omitempty"`
	ClaimSessionID string `json:"claim_session_id,omitempty"`
}

// FileStore is a BindingStore that keeps one JSON file per binding under a
// directory, so bindings survive a process restart. It is safe for concurrent
// use within a process; writes are atomic renames.
type FileStore struct {
	t *fileTable[SessionName, bindingRecord]
}

// NewFileStore returns a FileStore rooted at dir. The directory is created on
// the first Save.
func NewFileStore(dir string) *FileStore {
	return &FileStore{t: newFileTable[SessionName, bindingRecord](dir, flatPath(dir))}
}

// Load implements BindingStore. A missing, unreadable or corrupt file reads
// as not found.
func (s *FileStore) Load(name SessionName) (Binding, bool) {
	r, ok, err := s.t.read(name)
	if err != nil || !ok {
		return Binding{}, false
	}
	return Binding(r), true
}

// List implements BindingLister. Missing and corrupt files are skipped.
func (s *FileStore) List() []Binding {
	records := s.t.list()
	out := make([]Binding, 0, len(records))
	for _, r := range records {
		out = append(out, Binding(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Save implements BindingStore. The interface has no error return, so a
// failed write is dropped; the next Resolve rebuilds the binding by scanning.
func (s *FileStore) Save(b Binding) { _ = s.t.write(b.Name, bindingRecord(b)) }

// Delete implements BindingStore. A missing file is not an error.
func (s *FileStore) Delete(name SessionName) { _ = s.t.remove(name) }
