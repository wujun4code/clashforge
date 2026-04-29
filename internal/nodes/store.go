package nodes

import (
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

type persistedNode struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Host          string     `json:"host"`
	Port          int        `json:"port"`
	Username      string     `json:"username"`
	Password      string     `json:"password,omitempty"`
	Domain        string     `json:"domain"`
	Email         string     `json:"email"`
	CFToken       string     `json:"cf_token,omitempty"`
	CFAccountID   string     `json:"cf_account_id"`
	CFZoneID      string     `json:"cf_zone_id"`
	ProxyUser     string     `json:"proxy_user,omitempty"`
	ProxyPassword string     `json:"proxy_password,omitempty"`
	Status        Status     `json:"status"`
	DeployedAt    *time.Time `json:"deployed_at,omitempty"`
	CertExpiry    *time.Time `json:"cert_expiry,omitempty"`
	Error         string     `json:"error,omitempty"`
	DeployLog     string     `json:"deploy_log,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func toPersistedNode(n *Node) persistedNode {
	if n == nil {
		return persistedNode{}
	}
	return persistedNode{
		ID:            n.ID,
		Name:          n.Name,
		Host:          n.Host,
		Port:          n.Port,
		Username:      n.Username,
		Password:      n.Password,
		Domain:        n.Domain,
		Email:         n.Email,
		CFToken:       n.CFToken,
		CFAccountID:   n.CFAccountID,
		CFZoneID:      n.CFZoneID,
		ProxyUser:     n.ProxyUser,
		ProxyPassword: n.ProxyPassword,
		Status:        n.Status,
		DeployedAt:    n.DeployedAt,
		CertExpiry:    n.CertExpiry,
		Error:         n.Error,
		DeployLog:     n.DeployLog,
		CreatedAt:     n.CreatedAt,
		UpdatedAt:     n.UpdatedAt,
	}
}

func fromPersistedNode(p persistedNode) *Node {
	return &Node{
		ID:            p.ID,
		Name:          p.Name,
		Host:          p.Host,
		Port:          p.Port,
		Username:      p.Username,
		Password:      p.Password,
		Domain:        p.Domain,
		Email:         p.Email,
		CFToken:       p.CFToken,
		CFAccountID:   p.CFAccountID,
		CFZoneID:      p.CFZoneID,
		ProxyUser:     p.ProxyUser,
		ProxyPassword: p.ProxyPassword,
		Status:        p.Status,
		DeployedAt:    p.DeployedAt,
		CertExpiry:    p.CertExpiry,
		Error:         p.Error,
		DeployLog:     p.DeployLog,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}

// Store persists encrypted node data to a JSON file.
type Store struct {
	mu       sync.RWMutex
	filePath string
	keyPath  string
	encKey   []byte
	nodes    map[string]*Node
}

// NewStore creates or opens a node store.
func NewStore(dataDir string) (*Store, error) {
	keyPath := filepath.Join(dataDir, "nodes.key")
	filePath := filepath.Join(dataDir, "nodes.json")

	// Load or generate encryption key
	key, err := loadOrGenerateKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("encryption key: %w", err)
	}

	s := &Store{
		filePath: filePath,
		keyPath:  keyPath,
		encKey:   key,
		nodes:    make(map[string]*Node),
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load nodes: %w", err)
	}

	return s, nil
}

func loadOrGenerateKey(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil && len(data) == sha256.Size {
		return data, nil
	}
	key := make([]byte, sha256.Size)
	if _, err := cryptorand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	var encrypted map[string]string
	if err := json.Unmarshal(data, &encrypted); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, enc := range encrypted {
		decrypted, err := decryptAES(enc, s.encKey)
		if err != nil {
			continue // skip corrupted entries
		}
		var node persistedNode
		if err := json.Unmarshal([]byte(decrypted), &node); err != nil {
			continue
		}
		loaded := fromPersistedNode(node)
		if loaded.ID == "" {
			loaded.ID = id
		}
		s.nodes[id] = loaded
	}
	return nil
}

func (s *Store) save() error {
	s.mu.RLock()
	encrypted := make(map[string]string, len(s.nodes))
	for id, node := range s.nodes {
		data, err := json.Marshal(toPersistedNode(node))
		if err != nil {
			s.mu.RUnlock()
			return err
		}
		enc, err := encryptAES(string(data), s.encKey)
		if err != nil {
			s.mu.RUnlock()
			return err
		}
		encrypted[id] = enc
	}
	s.mu.RUnlock()

	raw, err := json.MarshalIndent(encrypted, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, raw, 0o600)
}

// List returns all nodes as safe-for-frontend list items.
func (s *Store) List() []NodeListItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]NodeListItem, 0, len(s.nodes))
	for _, n := range s.nodes {
		items = append(items, NodeListItem{
			ID:         n.ID,
			Name:       n.Name,
			Host:       n.Host,
			Port:       n.Port,
			Username:   n.Username,
			Domain:     n.Domain,
			Status:     n.Status,
			DeployedAt: n.DeployedAt,
			CertExpiry: n.CertExpiry,
			Error:      n.Error,
			DeployLog:  n.DeployLog,
			CreatedAt:  n.CreatedAt,
			UpdatedAt:  n.UpdatedAt,
		})
	}
	return items
}

// Get returns a node by ID (with decrypted secrets for internal use).
func (s *Store) Get(id string) (*Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[id]
	return n, ok
}

// Create adds a new node.
func (s *Store) Create(n *Node) error {
	s.mu.Lock()
	n.ID = uuid.New().String()
	n.Status = StatusPending
	now := time.Now()
	n.CreatedAt = now
	n.UpdatedAt = now
	s.nodes[n.ID] = n
	s.mu.Unlock()
	return s.save()
}

// Update replaces an existing node.
func (s *Store) Update(id string, n *Node) error {
	s.mu.Lock()
	existing, ok := s.nodes[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("node %s not found", id)
	}
	// Preserve ID and creation time
	n.ID = id
	n.CreatedAt = existing.CreatedAt
	n.UpdatedAt = time.Now()
	// Preserve proxy credentials if not set in update
	if n.ProxyPassword == "" {
		n.ProxyPassword = existing.ProxyPassword
	}
	if n.Password == "" {
		n.Password = existing.Password
	}
	if n.CFToken == "" {
		n.CFToken = existing.CFToken
	}
	s.nodes[id] = n
	s.mu.Unlock()
	return s.save()
}

// Delete removes a node.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	delete(s.nodes, id)
	s.mu.Unlock()
	return s.save()
}
