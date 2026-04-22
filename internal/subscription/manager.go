package subscription

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
			// remove cache
			_ = os.Remove(filepath.Join(m.dataDir, "cache", id+".json"))
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
