package provider

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
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
}

// memoryActivityStore is an in-memory ActivityStore.
type memoryActivityStore struct {
	mu sync.Mutex
	m  map[SessionName]ActivityRecord
}

// NewMemoryActivityStore returns an empty in-memory ActivityStore.
func NewMemoryActivityStore() ActivityStore {
	return &memoryActivityStore{m: make(map[SessionName]ActivityRecord)}
}

func (s *memoryActivityStore) Load(name SessionName) (ActivityRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.m[name]
	return r, ok
}

func (s *memoryActivityStore) Save(name SessionName, rec ActivityRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[name] = rec
}

// activityRecord is the on-disk shape of an ActivityRecord.
type activityRecord struct {
	Identity    string    `json:"identity"`
	Fingerprint string    `json:"fingerprint"`
	UpdatedAt   time.Time `json:"updated_at"`
	InputAt     time.Time `json:"input_updated_at,omitempty"`
}

// fileActivityStore is a file-backed ActivityStore.
type fileActivityStore struct {
	mu  sync.Mutex
	dir string
}

// NewFileActivityStore returns an ActivityStore that keeps one JSON file per
// name under dir.
func NewFileActivityStore(dir string) ActivityStore { return &fileActivityStore{dir: dir} }

func (s *fileActivityStore) path(name SessionName) string {
	return filepath.Join(s.dir, nameKey(name)+".json")
}

func (s *fileActivityStore) Load(name SessionName) (ActivityRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r activityRecord
	ok, err := readJSON(s.path(name), &r)
	if err != nil || !ok {
		return ActivityRecord{}, false
	}
	return ActivityRecord{Identity: r.Identity, Fingerprint: r.Fingerprint, At: r.UpdatedAt, InputAt: r.InputAt}, true
}

func (s *fileActivityStore) Save(name SessionName, rec ActivityRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = writeJSON(s.path(name), activityRecord{Identity: rec.Identity, Fingerprint: rec.Fingerprint, UpdatedAt: rec.At, InputAt: rec.InputAt})
}

// memoryMetaStore is an in-memory MetaStore.
type memoryMetaStore struct {
	mu sync.Mutex
	m  map[SessionName]map[string]string
}

// NewMemoryMetaStore returns an empty in-memory MetaStore.
func NewMemoryMetaStore() MetaStore {
	return &memoryMetaStore{m: make(map[SessionName]map[string]string)}
}

func (s *memoryMetaStore) Get(name SessionName, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[name][key]
	return v, ok, nil
}

func (s *memoryMetaStore) Set(name SessionName, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[name] == nil {
		s.m[name] = make(map[string]string)
	}
	s.m[name][key] = value
	return nil
}

func (s *memoryMetaStore) Delete(name SessionName, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m[name], key)
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
	mu  sync.Mutex
	dir string
}

// NewFileMetaStore returns a MetaStore that keeps one JSON file per key under
// dir/<name hash>/.
func NewFileMetaStore(dir string) MetaStore { return &fileMetaStore{dir: dir} }

func (s *fileMetaStore) path(name SessionName, key string) string {
	sum := sha256.Sum256([]byte(string(name) + "\x00" + key))
	return filepath.Join(s.dir, nameKey(name), hex.EncodeToString(sum[:])+".json")
}

// Get reads the entry for key. A missing file, or an old-format entry with no
// value_b64, is not set. An unreadable or corrupt file, or a value that is not
// valid base64, is an error.
func (s *fileMetaStore) Get(name SessionName, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var e metaEntry
	ok, err := readJSON(s.path(name, key), &e)
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
	s.mu.Lock()
	defer s.mu.Unlock()
	enc := base64.StdEncoding.EncodeToString([]byte(value))
	return writeJSON(s.path(name, key), metaEntry{Key: key, ValueB64: &enc})
}

func (s *fileMetaStore) Delete(name SessionName, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path(name, key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
