package quickstart

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Store persists deploy state records to a JSON file.
type Store struct {
	mu      sync.RWMutex
	dir     string
	records map[string]*DeployState
}

// NewStore creates or opens the quickstart state store.
func NewStore(dataDir string) (*Store, error) {
	dir := filepath.Join(dataDir, "quickstart")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("quickstart store mkdir: %w", err)
	}
	s := &Store{
		dir:     dir,
		records: make(map[string]*DeployState),
	}
	_ = s.load() // ignore error on first start (file may not exist yet)
	return s, nil
}

// Create allocates a new deploy record and persists it.
func (s *Store) Create(deployType DeployType, nodeName, nodeHost string) (*DeployState, error) {
	id := uuid.New().String()
	rec := &DeployState{
		ID:         id,
		DeployType: deployType,
		Status:     "running",
		StartedAt:  time.Now(),
		NodeName:   nodeName,
		NodeHost:   nodeHost,
	}
	s.mu.Lock()
	s.records[id] = rec
	s.mu.Unlock()
	return rec, s.save()
}

// Update modifies an existing record in place and persists.
func (s *Store) Update(id string, fn func(*DeployState)) error {
	s.mu.Lock()
	rec, ok := s.records[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("deploy record %s not found", id)
	}
	fn(rec)
	s.mu.Unlock()
	return s.save()
}

// Get retrieves a deploy record by ID.
func (s *Store) Get(id string) (*DeployState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.records[id]
	return r, ok
}

// List returns all deploy records ordered by start time (newest first).
func (s *Store) List() []*DeployState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*DeployState, 0, len(s.records))
	for _, r := range s.records {
		out = append(out, r)
	}
	return out
}

func (s *Store) load() error {
	path := filepath.Join(s.dir, "deploys.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Unmarshal(data, &s.records)
}

func (s *Store) save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.records, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, "deploys.json")
	return os.WriteFile(path, data, 0o600)
}
