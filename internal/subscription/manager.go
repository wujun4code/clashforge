package subscription

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
// URL-based config subscriptions are stored in subscriptions.toml.
// Imported proxy node sets (type=static) are stored in node_imports.toml.
type Manager struct {
	mu      sync.RWMutex
	dataDir string
	list    SubscriptionList // URL-based config subscriptions only
	imports SubscriptionList // imported proxy node sets only
}

func NewManager(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Load URL-based config subscriptions.
	subsPath := filepath.Join(m.dataDir, "subscriptions.toml")
	if _, err := os.Stat(subsPath); err == nil {
		var list SubscriptionList
		if _, err := toml.DecodeFile(subsPath, &list); err != nil {
			return fmt.Errorf("load subscriptions: %w", err)
		}
		m.list = list
	}

	// Load imported proxy node sets.
	importsPath := filepath.Join(m.dataDir, "node_imports.toml")
	if _, err := os.Stat(importsPath); err == nil {
		var imports SubscriptionList
		if _, err := toml.DecodeFile(importsPath, &imports); err != nil {
			return fmt.Errorf("load node_imports: %w", err)
		}
		m.imports = imports
	}

	// Migration: if subscriptions.toml had any type=static entries (old format),
	// move them to node_imports.toml and re-save both files cleanly.
	var configSubs, staticSubs []Subscription
	for _, s := range m.list.Subscriptions {
		if s.Type == "static" {
			staticSubs = append(staticSubs, s)
		} else {
			configSubs = append(configSubs, s)
		}
	}
	if len(staticSubs) > 0 {
		m.list.Subscriptions = configSubs
		// Merge into imports, avoiding duplicates.
		existing := make(map[string]bool)
		for _, s := range m.imports.Subscriptions {
			existing[s.ID] = true
		}
		for _, s := range staticSubs {
			if !existing[s.ID] {
				m.imports.Subscriptions = append(m.imports.Subscriptions, s)
			}
		}
		_ = m.saveLocked()
		_ = m.saveImportsLocked()
	}

	return nil
}

func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveLocked()
}

func (m *Manager) saveLocked() error {
	// Only save URL-based config subscriptions (never static/imported).
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

func (m *Manager) saveImportsLocked() error {
	path := filepath.Join(m.dataDir, "node_imports.toml")
	f, err := os.CreateTemp(filepath.Dir(path), ".node_imports-*.toml")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := toml.NewEncoder(f).Encode(m.imports); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, path)
}

// GetAll returns URL-based config subscriptions only (never imported node sets).
func (m *Manager) GetAll() []Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Subscription, len(m.list.Subscriptions))
	copy(result, m.list.Subscriptions)
	return result
}

// GetAllImports returns imported proxy node sets (type=static), stored separately
// from URL-based config subscriptions.
func (m *Manager) GetAllImports() []Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Subscription, len(m.imports.Subscriptions))
	copy(result, m.imports.Subscriptions)
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
	for _, s := range m.imports.Subscriptions {
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
			_ = os.Remove(filepath.Join(m.dataDir, "cache", id+".json"))
			_ = os.Remove(filepath.Join(m.dataDir, "cache", id+".raw.yaml"))
			return m.saveLocked()
		}
	}
	// Also check imported node sets.
	for i, s := range m.imports.Subscriptions {
		if s.ID == id {
			m.imports.Subscriptions = append(m.imports.Subscriptions[:i], m.imports.Subscriptions[i+1:]...)
			_ = os.Remove(filepath.Join(m.dataDir, "cache", id+".json"))
			_ = os.Remove(filepath.Join(m.dataDir, "cache", id+".raw.yaml"))
			return m.saveImportsLocked()
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

// GetAllCachedNodes returns nodes from URL-based config subscriptions only.
// Imported proxy node sets are excluded — they are not config sources.
func (m *Manager) GetAllCachedNodes() []ProxyNode {
	subs := m.GetAll() // config subs only
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

// autoNameFromNodes derives a display name from the first node in the list.
func autoNameFromNodes(nodes []ProxyNode) string {
	if len(nodes) == 0 {
		return "导入节点"
	}
	if len(nodes) == 1 {
		return nodes[0].Name
	}
	return fmt.Sprintf("%s 等 %d 个节点", nodes[0].Name, len(nodes))
}

// ImportStatic creates a static subscription from inline YAML content (no URL).
// The subscription name is auto-derived from the parsed node names.
// The parsed nodes are written directly to the cache; the subscription is enabled
// immediately and will not be updated by the auto-refresh scheduler.
func (m *Manager) ImportStatic(content string) (string, int, []ProxyNode, error) {
	nodes, err := Parse([]byte(content))
	if err != nil {
		return "", 0, nil, fmt.Errorf("parse: %w", err)
	}

	id, err := generateID()
	if err != nil {
		return "", 0, nil, err
	}

	now := time.Now()
	sub := Subscription{
		ID:          id,
		Name:        autoNameFromNodes(nodes),
		Type:        "static",
		Enabled:     true,
		LastUpdated: now,
		NodeCount:   len(nodes),
	}

	// Write cache
	cacheDir := filepath.Join(m.dataDir, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", 0, nil, err
	}
	data, err := json.Marshal(nodes)
	if err != nil {
		return "", 0, nil, err
	}
	if err := os.WriteFile(filepath.Join(cacheDir, id+".json"), data, 0o644); err != nil {
		return "", 0, nil, err
	}
	// Always persist raw YAML for static subscriptions so users can view and edit
	// the original content regardless of whether it is a full Clash map or a bare
	// proxy list (sequence).
	_ = os.WriteFile(filepath.Join(cacheDir, id+".raw.yaml"), []byte(content), 0o644)

	m.mu.Lock()
	m.imports.Subscriptions = append(m.imports.Subscriptions, sub)
	_ = m.saveImportsLocked()
	m.mu.Unlock()

	return id, len(nodes), nodes, nil
}

// UpdateStaticContent re-parses inline YAML and replaces the cached nodes for an
// existing static subscription in-place, preserving the subscription ID.
func (m *Manager) UpdateStaticContent(id string, content string) (int, []ProxyNode, error) {
	nodes, err := Parse([]byte(content))
	if err != nil {
		return 0, nil, fmt.Errorf("parse: %w", err)
	}

	m.mu.Lock()
	found := false
	for i, s := range m.imports.Subscriptions {
		if s.ID == id {
			m.imports.Subscriptions[i].NodeCount = len(nodes)
			m.imports.Subscriptions[i].Name = autoNameFromNodes(nodes)
			m.imports.Subscriptions[i].LastUpdated = time.Now()
			found = true
			break
		}
	}
	if !found {
		m.mu.Unlock()
		return 0, nil, fmt.Errorf("static subscription %s not found", id)
	}
	_ = m.saveImportsLocked()
	m.mu.Unlock()

	cacheDir := filepath.Join(m.dataDir, "cache")
	data, err := json.Marshal(nodes)
	if err != nil {
		return 0, nil, err
	}
	if err := os.WriteFile(filepath.Join(m.dataDir, "cache", id+".json"), data, 0o644); err != nil {
		return 0, nil, err
	}
	// Always persist raw YAML so the edit modal can reload the content.
	_ = os.WriteFile(filepath.Join(cacheDir, id+".raw.yaml"), []byte(content), 0o644)
	return len(nodes), nodes, nil
}

func (m *Manager) doUpdate(sub Subscription) error {
	if sub.Type == "static" {
		// Static subscriptions have no URL; the cache was written at import time.
		return nil
	}
	log.Info().Str("id", sub.ID).Str("name", sub.Name).Str("url", sub.URL).Msg("updating subscription")
	content, err := Fetch(sub.URL, sub.UserAgent)
	if err != nil {
		log.Error().Err(err).Str("id", sub.ID).Str("name", sub.Name).
			Msg("subscription: ⚠️ 订阅下载失败！无法获取代理节点，国际流量可能无法代理")
		return fmt.Errorf("fetch: %w", err)
	}
	nodes, err := Parse(content)
	if err != nil {
		log.Error().Err(err).Str("id", sub.ID).Str("name", sub.Name).Int("content_size", len(content)).
			Msg("subscription: ⚠️ 订阅内容解析失败！")
		return fmt.Errorf("parse: %w", err)
	}
	log.Info().
		Str("id", sub.ID).
		Int("parsed_nodes", len(nodes)).
		Msg("subscription: 订阅解析完成，开始过滤")

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
// GetRawYAMLForEnabled returns raw YAML from URL-based config subscriptions only.
// Imported node sets are excluded — they are not config sources.
func (m *Manager) GetRawYAMLForEnabled() [][]byte {
	subs := m.GetAll() // config subs only
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
