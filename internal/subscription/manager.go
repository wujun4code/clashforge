package subscription

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

// Manager handles all subscription operations.
type Manager struct {
	mu      sync.RWMutex
	dataDir string
	list    SubscriptionList
}

func NewManager(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	path := filepath.Join(m.dataDir, "subscriptions.toml")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	var list SubscriptionList
	if _, err := toml.DecodeFile(path, &list); err != nil {
		return fmt.Errorf("load subscriptions: %w", err)
	}
	m.list = list
	return nil
}

func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveLocked()
}

func (m *Manager) saveLocked() error {
	path := filepath.Join(m.dataDir, "subscriptions.toml")
	f, err := os.CreateTemp(filepath.Dir(path), ".subscriptions-*.toml")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := toml.NewEncoder(f).Encode(m.list); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, path)
}

func (m *Manager) GetAll() []Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Subscription, len(m.list.Subscriptions))
	copy(result, m.list.Subscriptions)
	return result
}

func (m *Manager) GetByID(id string) (Subscription, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.list.Subscriptions {
		if s.ID == id {
			return s, true
		}
	}
	return Subscription{}, false
}

func (m *Manager) Add(sub Subscription) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If a subscription with the same URL already exists, overwrite its metadata
	// in-place and reuse the existing ID so cache files on disk remain a single copy.
	if sub.URL != "" {
		for i, s := range m.list.Subscriptions {
			if s.URL == sub.URL {
				m.list.Subscriptions[i].Name = sub.Name
				if sub.UserAgent != "" {
					m.list.Subscriptions[i].UserAgent = sub.UserAgent
				}
				if sub.Interval != "" {
					m.list.Subscriptions[i].Interval = sub.Interval
				}
				m.list.Subscriptions[i].Filter = sub.Filter
				m.list.Subscriptions[i].Enabled = sub.Enabled
				return s.ID, m.saveLocked()
			}
		}
	}

	id, err := generateID()
	if err != nil {
		return "", err
	}
	sub.ID = id
	if sub.UserAgent == "" {
		sub.UserAgent = defaultUserAgent
	}
	if sub.Interval == "" {
		sub.Interval = "6h"
	}
	m.list.Subscriptions = append(m.list.Subscriptions, sub)
	return id, m.saveLocked()
}

func (m *Manager) Update(id string, patch map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, s := range m.list.Subscriptions {
		if s.ID == id {
			if name, ok := patch["name"].(string); ok {
				m.list.Subscriptions[i].Name = name
			}
			if url, ok := patch["url"].(string); ok {
				m.list.Subscriptions[i].URL = url
			}
			if ua, ok := patch["user_agent"].(string); ok {
				m.list.Subscriptions[i].UserAgent = ua
			}
			if iv, ok := patch["interval"].(string); ok {
				m.list.Subscriptions[i].Interval = iv
			}
			if enabled, ok := patch["enabled"].(bool); ok {
				m.list.Subscriptions[i].Enabled = enabled
			}
			return m.saveLocked()
		}
	}
	return fmt.Errorf("subscription %s not found", id)
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, s := range m.list.Subscriptions {
		if s.ID == id {
			m.list.Subscriptions = append(m.list.Subscriptions[:i], m.list.Subscriptions[i+1:]...)
			// remove cache files
			_ = os.Remove(filepath.Join(m.dataDir, "cache", id+".json"))
			_ = os.Remove(filepath.Join(m.dataDir, "cache", id+".raw.yaml"))
			return m.saveLocked()
		}
	}
	return fmt.Errorf("subscription %s not found", id)
}

func (m *Manager) TriggerUpdate(id string) error {
	sub, ok := m.GetByID(id)
	if !ok {
		return fmt.Errorf("subscription %s not found", id)
	}
	go func() {
		if err := m.doUpdate(sub); err != nil {
			log.Error().Err(err).Str("id", id).Msg("subscription update failed")
		}
	}()
	return nil
}

// SyncUpdate fetches and caches the subscription synchronously, waiting for completion.
func (m *Manager) SyncUpdate(id string) error {
	sub, ok := m.GetByID(id)
	if !ok {
		return fmt.Errorf("subscription %s not found", id)
	}
	return m.doUpdate(sub)
}

func (m *Manager) TriggerUpdateAll() error {
	subs := m.GetAll()
	var enabled []Subscription
	for _, s := range subs {
		if s.Enabled {
			enabled = append(enabled, s)
		}
	}
	go func() {
		eg := &errgroup.Group{}
		for _, s := range enabled {
			s := s
			eg.Go(func() error {
				return m.doUpdate(s)
			})
		}
		if err := eg.Wait(); err != nil {
			log.Error().Err(err).Msg("update all subscriptions finished with errors")
		}
	}()
	return nil
}

func (m *Manager) GetCachedNodes(id string) ([]ProxyNode, error) {
	path := filepath.Join(m.dataDir, "cache", id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var nodes []ProxyNode
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (m *Manager) GetAllCachedNodes() []ProxyNode {
	subs := m.GetAll()
	var all []ProxyNode
	for _, s := range subs {
		if !s.Enabled {
			continue
		}
		nodes, err := m.GetCachedNodes(s.ID)
		if err != nil {
			continue
		}
		for i := range nodes {
			nodes[i].SourceSubID = s.ID
		}
		all = append(all, nodes...)
	}
	return all
}

func (m *Manager) doUpdate(sub Subscription) error {
	log.Info().Str("id", sub.ID).Str("name", sub.Name).Msg("updating subscription")
	content, err := Fetch(sub.URL, sub.UserAgent)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	nodes, err := Parse(content)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	filtered := ApplyFilter(nodes, sub.Filter)

	cacheDir := filepath.Join(m.dataDir, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cacheDir, sub.ID+".json"), data, 0o644); err != nil {
		return err
	}
	// Save raw YAML content so the caller can extract rule-providers and other
	// sections that are not captured by the node parser (e.g. rule-providers).
	// We only save when the content looks like a YAML map (Clash full config).
	rawPath := filepath.Join(cacheDir, sub.ID+".raw.yaml")
	if isYAMLMap(content) {
		_ = os.WriteFile(rawPath, content, 0o644)
	} else {
		_ = os.Remove(rawPath) // clean up stale file if format changed
	}

	m.mu.Lock()
	for i, s := range m.list.Subscriptions {
		if s.ID == sub.ID {
			m.list.Subscriptions[i].LastUpdated = time.Now()
			m.list.Subscriptions[i].NodeCount = len(nodes)
			break
		}
	}
	_ = m.saveLocked()
	m.mu.Unlock()

	log.Info().Str("id", sub.ID).Int("nodes", len(filtered)).Msg("subscription updated")
	return nil
}

func generateID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "sub_" + hex.EncodeToString(b), nil
}

// HasCache returns true if a cached node list exists on disk for the given subscription.
func (m *Manager) HasCache(id string) bool {
	_, err := os.Stat(filepath.Join(m.dataDir, "cache", id+".json"))
	return err == nil
}

// GetRawYAML returns the saved raw YAML bytes for the given subscription id.
// It returns os.ErrNotExist when no raw YAML cache is available.
func (m *Manager) GetRawYAML(id string) ([]byte, error) {
	return os.ReadFile(filepath.Join(m.dataDir, "cache", id+".raw.yaml"))
}

// GetRawYAMLForEnabled returns the saved raw YAML bytes for every enabled subscription
// that was last fetched as a full Clash YAML (i.e. a .raw.yaml cache file exists).
// This allows callers to extract extra sections (rule-providers, rules, etc.) that
// the node parser does not capture.
func (m *Manager) GetRawYAMLForEnabled() [][]byte {
	subs := m.GetAll()
	var result [][]byte
	for _, s := range subs {
		if !s.Enabled {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(m.dataDir, "cache", s.ID+".raw.yaml"))
		if err != nil {
			continue
		}
		result = append(result, raw)
	}
	return result
}

// isYAMLMap returns true when content looks like a YAML mapping (Clash full config).
// It skips blank lines, comments, and YAML document markers (--- / ...),
// then checks whether the first real content line is a map key vs. a sequence item.
func isYAMLMap(content []byte) bool {
	for _, line := range splitLines(content) {
		s := strings.TrimSpace(line)
		// Skip blank lines, comments, and YAML document/end markers
		if s == "" || s == "---" || s == "..." || strings.HasPrefix(s, "#") {
			continue
		}
		// First meaningful content: sequence item starts with "- ", map key does not
		return !strings.HasPrefix(s, "- ")
	}
	return false
}

func splitLines(b []byte) []string {
	var lines []string
	start := 0
	for i, c := range b {
		if c == '\n' {
			lines = append(lines, string(b[start:i]))
			start = i + 1
		}
	}
	if start < len(b) {
		lines = append(lines, string(b[start:]))
	}
	return lines
}
