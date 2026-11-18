package provider

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path/filepath"
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

// ActivityStore persists activity fingerprints per session name.
type ActivityStore interface {
	// Load returns the record for name, if any.
	Load(name SessionName) (ActivityRecord, bool)
	// Save stores rec, replacing any earlier record for name.
	Save(name SessionName, rec ActivityRecord)
	// Delete removes any record for name, so a retired incarnation leaves no
	// activity stamp behind for the next one.
	Delete(name SessionName) error
}

// MetaStore is a per-session key/value store. Values are kept byte-for-byte.
type MetaStore interface {
	// Get returns the value for key and whether it is set. A key that is not
	// set is ("", false, nil); a store that cannot be read returns an error.
	Get(name SessionName, key string) (string, bool, error)
	// Set stores value under key.
	Set(name SessionName, key, value string) error
	// Delete removes key only.
	Delete(name SessionName, key string) error
	// Clear removes every key for name, so a retired incarnation leaves no
	// user or provider key behind for the next one.
	Clear(name SessionName) error
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

// memoryActivityStore is an in-memory ActivityStore.
type memoryActivityStore struct {
	t *memTable[SessionName, ActivityRecord]
}

// NewMemoryActivityStore returns an empty in-memory ActivityStore.
func NewMemoryActivityStore() ActivityStore {
	return &memoryActivityStore{t: newMemTable[SessionName, ActivityRecord]()}
}

func (s *memoryActivityStore) Load(name SessionName) (ActivityRecord, bool) { return s.t.get(name) }

func (s *memoryActivityStore) Save(name SessionName, rec ActivityRecord) { s.t.put(name, rec) }

func (s *memoryActivityStore) Delete(name SessionName) error {
	s.t.remove(name)
	return nil
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

// fileActivityStore is a file-backed ActivityStore.
type fileActivityStore struct {
	t *fileTable[SessionName, activityRecord]
}

// NewFileActivityStore returns an ActivityStore that keeps one JSON file per
// name under dir.
func NewFileActivityStore(dir string) ActivityStore {
	return &fileActivityStore{t: newFileTable[SessionName, activityRecord](dir, flatPath(dir))}
}

func (s *fileActivityStore) Load(name SessionName) (ActivityRecord, bool) {
	r, ok, err := s.t.read(name)
	if err != nil || !ok {
		return ActivityRecord{}, false
	}
	return ActivityRecord(r), true
}

func (s *fileActivityStore) Save(name SessionName, rec ActivityRecord) {
	_ = s.t.write(name, activityRecord(rec))
}

func (s *fileActivityStore) Delete(name SessionName) error { return s.t.remove(name) }

// metaKey addresses one key of one session in a meta store.
type metaKey struct {
	name SessionName
	key  string
}

// memoryMetaStore is an in-memory MetaStore.
type memoryMetaStore struct {
	t *memTable[metaKey, string]
}

// NewMemoryMetaStore returns an empty in-memory MetaStore.
func NewMemoryMetaStore() MetaStore {
	return &memoryMetaStore{t: newMemTable[metaKey, string]()}
}

func (s *memoryMetaStore) Get(name SessionName, key string) (string, bool, error) {
	v, ok := s.t.get(metaKey{name, key})
	return v, ok, nil
}

func (s *memoryMetaStore) Set(name SessionName, key, value string) error {
	s.t.put(metaKey{name, key}, value)
	return nil
}

func (s *memoryMetaStore) Delete(name SessionName, key string) error {
	s.t.remove(metaKey{name, key})
	return nil
}

func (s *memoryMetaStore) Clear(name SessionName) error {
	s.t.removeWhere(func(k metaKey) bool { return k.name == name })
	return nil
}

// metaEntry is the on-disk shape of one metadata key.
type metaEntry struct {
	Key string `json:"key"`
	// ValueB64 is the value in standard base64, so any byte string (including
	// invalid UTF-8) survives JSON. It is a pointer so a file without it reads
	// as not set rather than as an empty value.
	ValueB64 *string `json:"value_b64"`
}

// fileMetaStore is a file-backed MetaStore. It keeps one file per key so that
// separate processes setting different keys never overwrite each other.
type fileMetaStore struct {
	t *fileTable[metaKey, metaEntry]
}

// NewFileMetaStore returns a MetaStore that keeps one JSON file per key under
// dir/<name hash>/.
func NewFileMetaStore(dir string) MetaStore {
	return &fileMetaStore{t: newFileTable[metaKey, metaEntry](dir, func(k metaKey) string {
		sum := sha256.Sum256([]byte(string(k.name) + "\x00" + k.key))
		return filepath.Join(dir, nameKey(k.name), hex.EncodeToString(sum[:])+".json")
	})}
}

func (s *fileMetaStore) path(name SessionName, key string) string {
	return s.t.path(metaKey{name, key})
}

// Get reads the entry for key. A missing file, or an old-format entry with no
// value_b64, is not set. An unreadable or corrupt file, or a value that is not
// valid base64, is an error.
func (s *fileMetaStore) Get(name SessionName, key string) (string, bool, error) {
	e, ok, err := s.t.read(metaKey{name, key})
	if err != nil {
		return "", false, err
	}
	if !ok || e.ValueB64 == nil {
		return "", false, nil
	}
	v, err := base64.StdEncoding.DecodeString(*e.ValueB64)
	if err != nil {
		return "", false, fmt.Errorf("meta %s: decode value: %w", key, err)
	}
	return string(v), true, nil
}

func (s *fileMetaStore) Set(name SessionName, key, value string) error {
	enc := base64.StdEncoding.EncodeToString([]byte(value))
	return s.t.write(metaKey{name, key}, metaEntry{Key: key, ValueB64: &enc})
}

func (s *fileMetaStore) Delete(name SessionName, key string) error {
	return s.t.remove(metaKey{name, key})
}

// Clear removes every key for name. One directory per name holds every key
// file, so removing it drops the whole sidecar. A missing directory is success.
func (s *fileMetaStore) Clear(name SessionName) error {
	return s.t.removeAll(filepath.Join(s.t.dir, nameKey(name)))
}
