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

// removeIfPresent removes path if present.
func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// fileStore is the one file-backed adapter behind every file store: a
// fileTable of JSON records plus the encode/decode pair that maps a domain
// value to its on-disk record and back.
type fileStore[K comparable, V, R any] struct {
	t       *fileTable[K, R]
	enc     func(K, V) R
	dec     func(R) (V, error)
	present func(R) bool
}

func newFileStore[K comparable, V, R any](dir string, path func(K) string, enc func(K, V) R, dec func(R) (V, error), present func(R) bool) *fileStore[K, V, R] {
	return &fileStore[K, V, R]{t: newFileTable[K, R](dir, path), enc: enc, dec: dec, present: present}
}

func (s *fileStore[K, V, R]) Read(k K) (V, bool, error) {
	r, ok, err := s.t.read(k)
	if err != nil || !ok {
		var zero V
		return zero, false, err
	}
	if s.present != nil && !s.present(r) {
		var zero V
		return zero, false, nil
	}
	v, err := s.dec(r)
	if err != nil {
		var zero V
		return zero, false, err
	}
	return v, true, nil
}

func (s *fileStore[K, V, R]) Write(k K, v V) error {
	return s.t.write(k, s.enc(k, v))
}

func (s *fileStore[K, V, R]) Drop(k K) error { return s.t.remove(k) }

// List decodes every record directly under the store's dir. Missing and
// corrupt files are skipped.
func (s *fileStore[K, V, R]) List() []V {
	records := s.t.list()
	out := make([]V, 0, len(records))
	for _, r := range records {
		v, err := s.dec(r)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return out
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
	// Adopted was added after the first records.
	Adopted bool `json:"adopted,omitempty"`
	// GCBin, WorkDir and ClaimSessionID were added after the first records.
	GCBin          string     `json:"gc_bin,omitempty"`
	WorkDir        string     `json:"work_dir,omitempty"`
	ClaimSessionID string     `json:"claim_session_id,omitempty"`
	Survivors      []Survivor `json:"survivors,omitempty"`
	Attempt        string     `json:"attempt,omitempty"`
}

// bindingFromRecord converts a stored record to a Binding. The cast keeps the
// compile-time check that every Binding field has a record field; an empty
// aliases list is normalized to nil because the pre-refactor field-by-field
// conversion returned nil, and a refactor must not change the value.
func bindingFromRecord(r bindingRecord) Binding {
	b := Binding(r)
	if len(b.Aliases) == 0 {
		b.Aliases = nil
	}
	return b
}

// FileStore is a Store for bindings that keeps one JSON file per binding
// under a directory, so bindings survive a process restart. It is safe for
// concurrent use within a process; writes are atomic renames.
type FileStore struct {
	*fileStore[SessionName, Binding, bindingRecord]
}

// NewFileStore returns a FileStore rooted at dir. The directory is created on
// the first Save.
func NewFileStore(dir string) *FileStore {
	return &FileStore{newFileStore[SessionName, Binding, bindingRecord](dir, flatPath(dir),
		func(_ SessionName, b Binding) bindingRecord { return bindingRecord(b) },
		func(r bindingRecord) (Binding, error) { return bindingFromRecord(r), nil },
		nil,
	)}
}

// Load returns the binding for name, if any. A missing, unreadable or corrupt
// file reads as not found.
func (s *FileStore) Load(name SessionName) (Binding, bool) {
	b, ok, _ := s.Read(name)
	return b, ok
}

// List returns every binding, sorted by Name. Missing and corrupt files are
// skipped.
func (s *FileStore) List() []Binding {
	out := s.fileStore.List()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Save stores b, replacing any binding with the same name. A failed write is
// dropped; the next Resolve rebuilds the binding by scanning.
func (s *FileStore) Save(b Binding) { _ = s.Write(b.Name, b) }

// Delete removes the binding for name. A missing file is not an error.
func (s *FileStore) Delete(name SessionName) { _ = s.Drop(name) }
