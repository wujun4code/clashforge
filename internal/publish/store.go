package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type workerConfigFile struct {
	WorkerConfigs []WorkerConfig `json:"worker_configs"`
}

type publishRecordFile struct {
	PublishRecords []PublishRecord `json:"publish_records"`
}

type Store struct {
	mu           sync.RWMutex
	key          []byte
	keyPath      string
	configsPath  string
	recordsPath  string
	workerConfig map[string]WorkerConfig
	records      map[string]PublishRecord
}

func NewStore(dataDir string) (*Store, error) {
	keyPath := filepath.Join(dataDir, "publish.key")
	configsPath := filepath.Join(dataDir, "publish-worker-configs.json")
	recordsPath := filepath.Join(dataDir, "publish-records.json")

	key, err := loadOrGenerateKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("load publish key: %w", err)
	}

	s := &Store{
		key:          key,
		keyPath:      keyPath,
		configsPath:  configsPath,
		recordsPath:  recordsPath,
		workerConfig: make(map[string]WorkerConfig),
		records:      make(map[string]PublishRecord),
	}

	if err := s.loadWorkerConfigs(); err != nil {
		return nil, err
	}
	if err := s.loadPublishRecords(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) loadWorkerConfigs() error {
	data, err := os.ReadFile(s.configsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read worker configs: %w", err)
	}

	var wrapped workerConfigFile
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.WorkerConfigs != nil {
		for _, cfg := range wrapped.WorkerConfigs {
			if strings.TrimSpace(cfg.ID) == "" {
				continue
			}
			s.workerConfig[cfg.ID] = cfg
		}
		return nil
	}

	var plain []WorkerConfig
	if err := json.Unmarshal(data, &plain); err != nil {
		return fmt.Errorf("parse worker configs: %w", err)
	}
	for _, cfg := range plain {
		if strings.TrimSpace(cfg.ID) == "" {
			continue
		}
		s.workerConfig[cfg.ID] = cfg
	}
	return nil
}

func (s *Store) loadPublishRecords() error {
	data, err := os.ReadFile(s.recordsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read publish records: %w", err)
	}

	var wrapped publishRecordFile
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.PublishRecords != nil {
		for _, rec := range wrapped.PublishRecords {
			if strings.TrimSpace(rec.ID) == "" {
				continue
			}
			s.records[rec.ID] = rec
		}
		return nil
	}

	var plain []PublishRecord
	if err := json.Unmarshal(data, &plain); err != nil {
		return fmt.Errorf("parse publish records: %w", err)
	}
	for _, rec := range plain {
		if strings.TrimSpace(rec.ID) == "" {
			continue
		}
		s.records[rec.ID] = rec
	}
	return nil
}

func (s *Store) saveWorkerConfigsLocked() error {
	items := make([]WorkerConfig, 0, len(s.workerConfig))
	for _, cfg := range s.workerConfig {
		items = append(items, cfg)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	payload := workerConfigFile{WorkerConfigs: items}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal worker configs: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.configsPath), 0o755); err != nil {
		return fmt.Errorf("mkdir worker configs dir: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(s.configsPath), ".publish-worker-configs-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, s.configsPath)
}

func (s *Store) savePublishRecordsLocked() error {
	items := make([]PublishRecord, 0, len(s.records))
	for _, rec := range s.records {
		items = append(items, rec)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})
	payload := publishRecordFile{PublishRecords: items}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal publish records: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.recordsPath), 0o755); err != nil {
		return fmt.Errorf("mkdir publish records dir: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(s.recordsPath), ".publish-records-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, s.recordsPath)
}

func (s *Store) ListWorkerConfigs() []WorkerConfigView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]WorkerConfigView, 0, len(s.workerConfig))
	for _, cfg := range s.workerConfig {
		items = append(items, toWorkerConfigView(cfg))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items
}

func (s *Store) GetWorkerConfig(id string) (WorkerConfigView, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return WorkerConfigView{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.workerConfig[id]
	if !ok {
		return WorkerConfigView{}, false
	}
	return toWorkerConfigView(cfg), true
}

func toWorkerConfigView(cfg WorkerConfig) WorkerConfigView {
	return WorkerConfigView{
		ID:            cfg.ID,
		Name:          cfg.Name,
		WorkerName:    cfg.WorkerName,
		WorkerURL:     cfg.WorkerURL,
		WorkerDevURL:  cfg.WorkerDevURL,
		Hostname:      cfg.Hostname,
		AccountID:     cfg.AccountID,
		NamespaceID:   cfg.NamespaceID,
		ZoneID:        cfg.ZoneID,
		HasToken:      strings.TrimSpace(cfg.TokenEnc) != "",
		InitializedAt: cfg.InitializedAt,
		CreatedAt:     cfg.CreatedAt,
		UpdatedAt:     cfg.UpdatedAt,
	}
}

func (s *Store) UpsertWorkerConfig(input WorkerConfigInput) (WorkerConfigView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	id := strings.TrimSpace(input.ID)
	cfg := WorkerConfig{}
	if id == "" {
		id = "wc_" + uuid.NewString()
		cfg.ID = id
		cfg.CreatedAt = now
	} else if existing, ok := s.workerConfig[id]; ok {
		cfg = existing
	} else {
		cfg.ID = id
		cfg.CreatedAt = now
	}

	cfg.Name = strings.TrimSpace(input.Name)
	cfg.WorkerName = strings.TrimSpace(input.WorkerName)
	cfg.WorkerURL = strings.TrimSpace(input.WorkerURL)
	cfg.WorkerDevURL = strings.TrimSpace(input.WorkerDevURL)
	cfg.Hostname = strings.TrimSpace(input.Hostname)
	cfg.AccountID = strings.TrimSpace(input.AccountID)
	cfg.NamespaceID = strings.TrimSpace(input.NamespaceID)
	cfg.ZoneID = strings.TrimSpace(input.ZoneID)
	if cfg.ID == "" {
		cfg.ID = id
	}
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}

	if ts := strings.TrimSpace(input.InitializedAt); ts != "" {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			cfg.InitializedAt = parsed
		}
	}
	if cfg.InitializedAt.IsZero() {
		cfg.InitializedAt = now
	}

	token := strings.TrimSpace(input.Token)
	if token != "" {
		tokenEnc, err := encryptString(token, s.key)
		if err != nil {
			return WorkerConfigView{}, err
		}
		cfg.TokenEnc = tokenEnc
	}

	cfg.UpdatedAt = now
	s.workerConfig[cfg.ID] = cfg

	if err := s.saveWorkerConfigsLocked(); err != nil {
		return WorkerConfigView{}, err
	}
	return toWorkerConfigView(cfg), nil
}

func (s *Store) DeleteWorkerConfig(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("worker config id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workerConfig, id)
	for rid, rec := range s.records {
		if rec.WorkerConfigID == id {
			delete(s.records, rid)
		}
	}
	if err := s.saveWorkerConfigsLocked(); err != nil {
		return err
	}
	return s.savePublishRecordsLocked()
}

func (s *Store) GetWorkerConfigToken(id string) (string, error) {
	s.mu.RLock()
	cfg, ok := s.workerConfig[id]
	s.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("worker config %s not found", id)
	}
	return decryptString(cfg.TokenEnc, s.key)
}

func (s *Store) GetWorkerConfigWithToken(id string) (WorkerConfig, string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return WorkerConfig{}, "", fmt.Errorf("worker config id required")
	}
	s.mu.RLock()
	cfg, ok := s.workerConfig[id]
	s.mu.RUnlock()
	if !ok {
		return WorkerConfig{}, "", fmt.Errorf("worker config %s not found", id)
	}
	token, err := decryptString(cfg.TokenEnc, s.key)
	if err != nil {
		return WorkerConfig{}, "", err
	}
	return cfg, token, nil
}

func (s *Store) ListPublishRecords() []PublishRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]PublishRecord, 0, len(s.records))
	for _, rec := range s.records {
		items = append(items, rec)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})
	return items
}

func (s *Store) NextVersion(workerConfigID, baseName string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	maxVersion := 0
	for _, rec := range s.records {
		if rec.WorkerConfigID != workerConfigID || rec.BaseName != baseName {
			continue
		}
		if rec.Version > maxVersion {
			maxVersion = rec.Version
		}
	}
	return maxVersion + 1
}

func (s *Store) AddPublishRecord(input PublishRecordInput) (PublishRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	rec := PublishRecord{
		ID:             "pr_" + uuid.NewString(),
		WorkerConfigID: strings.TrimSpace(input.WorkerConfigID),
		WorkerName:     strings.TrimSpace(input.WorkerName),
		Hostname:       strings.TrimSpace(input.Hostname),
		BaseName:       strings.TrimSpace(input.BaseName),
		Version:        input.Version,
		FileName:       strings.TrimSpace(input.FileName),
		AccessURL:      strings.TrimSpace(input.AccessURL),
		PublishedAt:    now,
	}
	s.records[rec.ID] = rec
	if err := s.savePublishRecordsLocked(); err != nil {
		return PublishRecord{}, err
	}
	return rec, nil
}

func (s *Store) DeletePublishRecord(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("publish record id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return s.savePublishRecordsLocked()
}

func (s *Store) GetPublishRecord(id string) (PublishRecord, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return PublishRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[id]
	return rec, ok
}
