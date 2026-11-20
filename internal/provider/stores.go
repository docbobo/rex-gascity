package provider

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ActivityRecord is the last observed output fingerprint of a pane and when
// it was taken.
type ActivityRecord struct {
	// Identity is the block id of the pane the fingerprint was taken from.
	Identity    string
	Fingerprint string
	At          time.Time
	// InputAt is the last time the pack typed into the pane. It is a floor
	// for the reported activity, so input whose output has not rendered yet
	// is not read as idle. Zero means no input was recorded for this
	// incarnation; it is ignored when Identity is not the current pane.
	InputAt time.Time
}

// Store is the one persistence seam behind every sidecar record: bindings,
// activity fingerprints and metadata. K addresses one value and V is the
// value. The memory, file and composite adapters all implement this single
// interface, so the provider holds one store shape instead of one interface
// per record kind.
type Store[K comparable, V any] interface {
	// Read returns the value for k and whether it is set. A value that is not
	// set is (zero, false, nil); a store that cannot be read returns an error.
	Read(k K) (V, bool, error)
	// Write stores v under k, replacing any earlier value.
	Write(k K, v V) error
	// Drop removes any value under k. Missing values are not an error.
	Drop(k K) error
}

// Lister is the optional enumeration capability of a store. Only the binding
// store implements it.
type Lister[V any] interface {
	// List returns every value, sorted by the store's own key order.
	List() []V
}

// clearer is the optional per-session sweep of the metadata store: it removes
// every key of one session in one call.
type clearer interface {
	Clear(name SessionName) error
}

// snapshotter is the optional per-session bulk read of the metadata store: it
// returns every key of one session in one pass. The file store keeps one file
// per key under the session's directory, so a snapshot is one directory listing
// instead of one read per key.
type snapshotter interface {
	Snapshot(name SessionName) map[string]string
}

// memTable is the one in-memory table behind every memory store: a mutex
// around a map. The stores only add their key and value types.
type memTable[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

func newMemTable[K comparable, V any]() *memTable[K, V] {
	return &memTable[K, V]{m: make(map[K]V)}
}

func (t *memTable[K, V]) get(k K) (V, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	v, ok := t.m[k]
	return v, ok
}

func (t *memTable[K, V]) put(k K, v V) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[k] = v
}

func (t *memTable[K, V]) remove(k K) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, k)
}

// removeWhere deletes every entry whose key matches.
func (t *memTable[K, V]) removeWhere(match func(K) bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for k := range t.m {
		if match(k) {
			delete(t.m, k)
		}
	}
}

// values returns every value in no particular order.
func (t *memTable[K, V]) values() []V {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]V, 0, len(t.m))
	for _, v := range t.m {
		out = append(out, v)
	}
	return out
}

// memStore is the one in-memory adapter behind every memory store.
type memStore[K comparable, V any] struct {
	t *memTable[K, V]
}

func newMemStore[K comparable, V any]() *memStore[K, V] {
	return &memStore[K, V]{t: newMemTable[K, V]()}
}

func (s *memStore[K, V]) Read(k K) (V, bool, error) {
	v, ok := s.t.get(k)
	return v, ok, nil
}

func (s *memStore[K, V]) Write(k K, v V) error {
	s.t.put(k, v)
	return nil
}

func (s *memStore[K, V]) Drop(k K) error {
	s.t.remove(k)
	return nil
}

// MemoryStore is an in-memory Store for bindings. It is safe for concurrent
// use.
type MemoryStore struct {
	*memStore[SessionName, Binding]
}

// NewMemoryStore returns an empty in-memory binding store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{newMemStore[SessionName, Binding]()}
}

// Load returns the binding for name, if any. It is the historical binding API
// kept for callers that do not need the generic seam.
func (s *MemoryStore) Load(name SessionName) (Binding, bool) {
	b, ok, _ := s.Read(name)
	return b, ok
}

// Save stores b, replacing any binding with the same name.
func (s *MemoryStore) Save(b Binding) { _ = s.Write(b.Name, b) }

// Delete removes the binding for name.
func (s *MemoryStore) Delete(name SessionName) { _ = s.Drop(name) }

// List returns every binding, sorted by Name.
func (s *MemoryStore) List() []Binding {
	out := s.t.values()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// activityRecord is the on-disk shape of an ActivityRecord. It has the same
// fields, so Go converts between the two and a new field fails to compile
// until the record has it too.
type activityRecord struct {
	Identity    string    `json:"identity"`
	Fingerprint string    `json:"fingerprint"`
	At          time.Time `json:"updated_at"`
	InputAt     time.Time `json:"input_updated_at,omitempty"`
}

// memoryActivityStore is an in-memory activity store.
type memoryActivityStore struct {
	*memStore[SessionName, ActivityRecord]
}

// NewMemoryActivityStore returns an empty in-memory activity store.
func NewMemoryActivityStore() *memoryActivityStore {
	return &memoryActivityStore{newMemStore[SessionName, ActivityRecord]()}
}

// Load returns the record for name, if any.
func (s *memoryActivityStore) Load(name SessionName) (ActivityRecord, bool) {
	rec, ok, _ := s.Read(name)
	return rec, ok
}

// Save stores rec, replacing any earlier record for name.
func (s *memoryActivityStore) Save(name SessionName, rec ActivityRecord) {
	_ = s.Write(name, rec)
}

// Delete removes any record for name.
func (s *memoryActivityStore) Delete(name SessionName) error { return s.Drop(name) }

// fileActivityStore is a file-backed activity store.
type fileActivityStore struct {
	*fileStore[SessionName, ActivityRecord, activityRecord]
}

// NewFileActivityStore returns an activity store that keeps one JSON file per
// name under dir.
func NewFileActivityStore(dir string) *fileActivityStore {
	return &fileActivityStore{newFileStore[SessionName, ActivityRecord, activityRecord](dir, flatPath(dir),
		func(_ SessionName, rec ActivityRecord) activityRecord { return activityRecord(rec) },
		func(r activityRecord) (ActivityRecord, error) { return ActivityRecord(r), nil },
		nil,
	)}
}

// Load returns the record for name, if any.
func (s *fileActivityStore) Load(name SessionName) (ActivityRecord, bool) {
	rec, ok, _ := s.Read(name)
	return rec, ok
}

// Save stores rec, replacing any earlier record for name.
func (s *fileActivityStore) Save(name SessionName, rec ActivityRecord) {
	_ = s.Write(name, rec)
}

// Delete removes any record for name.
func (s *fileActivityStore) Delete(name SessionName) error { return s.Drop(name) }

// metaKey addresses one key of one session in a meta store.
type metaKey struct {
	name SessionName
	key  string
}

// memoryMetaStore is an in-memory metadata store.
type memoryMetaStore struct {
	*memStore[metaKey, string]
}

// NewMemoryMetaStore returns an empty in-memory metadata store.
func NewMemoryMetaStore() *memoryMetaStore {
	return &memoryMetaStore{newMemStore[metaKey, string]()}
}

// Get returns the value for key and whether it is set.
func (s *memoryMetaStore) Get(name SessionName, key string) (string, bool, error) {
	return s.Read(metaKey{name, key})
}

// Set stores value under key.
func (s *memoryMetaStore) Set(name SessionName, key, value string) error {
	return s.Write(metaKey{name, key}, value)
}

// Delete removes key only.
func (s *memoryMetaStore) Delete(name SessionName, key string) error {
	return s.Drop(metaKey{name, key})
}

// Clear removes every key for name.
func (s *memoryMetaStore) Clear(name SessionName) error {
	s.t.removeWhere(func(k metaKey) bool { return k.name == name })
	return nil
}

// Snapshot returns every key stored for name.
func (s *memoryMetaStore) Snapshot(name SessionName) map[string]string {
	out := map[string]string{}
	s.t.mu.RLock()
	defer s.t.mu.RUnlock()
	for k, v := range s.t.m {
		if k.name == name {
			out[k.key] = v
		}
	}
	return out
}

// metaEntry is the on-disk shape of one metadata key.
type metaEntry struct {
	Key string `json:"key"`
	// ValueB64 is the value in standard base64, so any byte string (including
	// invalid UTF-8) survives JSON. It is a pointer so a file without it reads
	// as not set rather than as an empty value.
	ValueB64 *string `json:"value_b64"`
}

// fileMetaStore is a file-backed metadata store. It keeps one file per key so
// that separate processes setting different keys never overwrite each other.
type fileMetaStore struct {
	*fileStore[metaKey, string, metaEntry]
}

// NewFileMetaStore returns a metadata store that keeps one JSON file per key
// under dir/<name hash>/.
func NewFileMetaStore(dir string) *fileMetaStore {
	return &fileMetaStore{newFileStore[metaKey, string, metaEntry](dir,
		func(k metaKey) string {
			sum := sha256.Sum256([]byte(string(k.name) + "\x00" + k.key))
			return filepath.Join(dir, nameKey(k.name), hex.EncodeToString(sum[:])+".json")
		},
		func(k metaKey, value string) metaEntry {
			enc := base64.StdEncoding.EncodeToString([]byte(value))
			return metaEntry{Key: k.key, ValueB64: &enc}
		},
		decodeMetaEntry,
		func(e metaEntry) bool { return e.ValueB64 != nil },
	)}
}

func decodeMetaEntry(e metaEntry) (string, error) {
	if e.ValueB64 == nil {
		return "", nil
	}
	v, err := base64.StdEncoding.DecodeString(*e.ValueB64)
	if err != nil {
		return "", fmt.Errorf("meta %s: decode value: %w", e.Key, err)
	}
	return string(v), nil
}

func (s *fileMetaStore) path(name SessionName, key string) string {
	return s.t.path(metaKey{name, key})
}

// Get reads the entry for key. A missing file, or an old-format entry with no
// value_b64, is not set. An unreadable or corrupt file, or a value that is not
// valid base64, is an error.
func (s *fileMetaStore) Get(name SessionName, key string) (string, bool, error) {
	return s.Read(metaKey{name, key})
}

// Set stores value under key.
func (s *fileMetaStore) Set(name SessionName, key, value string) error {
	return s.Write(metaKey{name, key}, value)
}

// Delete removes key only.
func (s *fileMetaStore) Delete(name SessionName, key string) error {
	return s.Drop(metaKey{name, key})
}

// Clear removes every key for name. One directory per name holds every key
// file, so removing it drops the whole sidecar. A missing directory is success.
func (s *fileMetaStore) Clear(name SessionName) error {
	return s.t.removeAll(filepath.Join(s.t.dir, nameKey(name)))
}

// Snapshot reads every key file of name in one directory pass. Unreadable or
// corrupt entries are skipped, as a per-key read would report them.
func (s *fileMetaStore) Snapshot(name SessionName) map[string]string {
	dir := filepath.Join(s.t.dir, nameKey(name))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, entry := range entries {
		var r metaEntry
		if ok, err := readJSON(filepath.Join(dir, entry.Name()), &r); err != nil || !ok || r.ValueB64 == nil {
			continue
		}
		if v, err := decodeMetaEntry(r); err == nil {
			out[r.Key] = v
		}
	}
	return out
}
