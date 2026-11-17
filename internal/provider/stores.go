package provider

import (
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
	// Get returns the value for key, if set.
	Get(name SessionName, key string) (string, bool)
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
	return ActivityRecord{Identity: r.Identity, Fingerprint: r.Fingerprint, At: r.UpdatedAt}, true
}

func (s *fileActivityStore) Save(name SessionName, rec ActivityRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = writeJSON(s.path(name), activityRecord{Identity: rec.Identity, Fingerprint: rec.Fingerprint, UpdatedAt: rec.At})
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

func (s *memoryMetaStore) Get(name SessionName, key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[name][key]
	return v, ok
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

// metaRecord is the on-disk shape of a session's metadata.
type metaRecord struct {
	Values map[string]string `json:"values"`
}

// fileMetaStore is a file-backed MetaStore.
type fileMetaStore struct {
	mu  sync.Mutex
	dir string
}

// NewFileMetaStore returns a MetaStore that keeps one JSON file per name
// under dir.
func NewFileMetaStore(dir string) MetaStore { return &fileMetaStore{dir: dir} }

func (s *fileMetaStore) path(name SessionName) string {
	return filepath.Join(s.dir, nameKey(name)+".json")
}

// read returns the stored values; a missing or corrupt file reads as empty.
func (s *fileMetaStore) read(name SessionName) map[string]string {
	var r metaRecord
	if ok, err := readJSON(s.path(name), &r); err != nil || !ok || r.Values == nil {
		return make(map[string]string)
	}
	return r.Values
}

func (s *fileMetaStore) Get(name SessionName, key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.read(name)[key]
	return v, ok
}

func (s *fileMetaStore) Set(name SessionName, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	vals := s.read(name)
	vals[key] = value
	return writeJSON(s.path(name), metaRecord{Values: vals})
}

func (s *fileMetaStore) Delete(name SessionName, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	vals := s.read(name)
	if _, ok := vals[key]; !ok {
		return nil
	}
	delete(vals, key)
	return writeJSON(s.path(name), metaRecord{Values: vals})
}
