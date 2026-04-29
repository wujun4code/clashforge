package workernode

import (
	cryptorand "crypto/rand"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// persistedNode is the on-disk representation (secrets stored as encrypted hex).
type persistedNode struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	WorkerName   string     `json:"worker_name"`
	WorkerUUID   string     `json:"worker_uuid,omitempty"` // encrypted
	CFToken      string     `json:"cf_token,omitempty"`    // encrypted
	CFAccountID  string     `json:"cf_account_id"`
	CFZoneID     string     `json:"cf_zone_id"`
	Hostname     string     `json:"hostname"`
	WorkerURL    string     `json:"worker_url"`
	WorkerDevURL string     `json:"worker_dev_url"`
	Status       Status     `json:"status"`
	Error        string     `json:"error,omitempty"`
	DeployedAt   *time.Time `json:"deployed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Store persists WorkerNode data to an AES-encrypted JSON file.
type Store struct {
	mu       sync.RWMutex
	filePath string
	encKey   []byte
	nodes    map[string]*WorkerNode
}

func NewStore(dataDir string) (*Store, error) {
	keyPath := filepath.Join(dataDir, "worker-nodes.key")
	filePath := filepath.Join(dataDir, "worker-nodes.json")

	key, err := loadOrGenerateKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("worker-nodes encryption key: %w", err)
	}

	s := &Store{filePath: filePath, encKey: key, nodes: make(map[string]*WorkerNode)}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load worker-nodes: %w", err)
	}
	return s, nil
}

func (s *Store) List() []WorkerNodeListItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]WorkerNodeListItem, 0, len(s.nodes))
	for _, n := range s.nodes {
		items = append(items, ToListItem(n))
	}
	return items
}

func (s *Store) Get(id string) (*WorkerNode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[id]
	return n, ok
}

func (s *Store) Create(n *WorkerNode) error {
	s.mu.Lock()
	n.ID = uuid.New().String()
	now := time.Now()
	n.CreatedAt = now
	n.UpdatedAt = now
	s.nodes[n.ID] = n
	s.mu.Unlock()
	return s.save()
}

func (s *Store) Update(id string, n *WorkerNode) error {
	s.mu.Lock()
	existing, ok := s.nodes[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("worker-node %s not found", id)
	}
	n.ID = id
	n.CreatedAt = existing.CreatedAt
	n.UpdatedAt = time.Now()
	if n.CFToken == "" {
		n.CFToken = existing.CFToken
	}
	if n.WorkerUUID == "" {
		n.WorkerUUID = existing.WorkerUUID
	}
	s.nodes[id] = n
	s.mu.Unlock()
	return s.save()
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	delete(s.nodes, id)
	s.mu.Unlock()
	return s.save()
}

// load reads and decrypts the persisted file.
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
		plain, err := decryptAES(enc, s.encKey)
		if err != nil {
			continue
		}
		var p persistedNode
		if err := json.Unmarshal([]byte(plain), &p); err != nil {
			continue
		}
		n := fromPersisted(p)
		if n.ID == "" {
			n.ID = id
		}
		s.nodes[id] = n
	}
	return nil
}

// save encrypts and writes all nodes.
func (s *Store) save() error {
	s.mu.RLock()
	encrypted := make(map[string]string, len(s.nodes))
	for id, n := range s.nodes {
		raw, err := json.Marshal(toPersisted(n))
		if err != nil {
			s.mu.RUnlock()
			return err
		}
		enc, err := encryptAES(string(raw), s.encKey)
		if err != nil {
			s.mu.RUnlock()
			return err
		}
		encrypted[id] = enc
	}
	s.mu.RUnlock()

	out, err := json.MarshalIndent(encrypted, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, out, 0o600)
}

func toPersisted(n *WorkerNode) persistedNode {
	return persistedNode{
		ID: n.ID, Name: n.Name, WorkerName: n.WorkerName,
		WorkerUUID: n.WorkerUUID, CFToken: n.CFToken,
		CFAccountID: n.CFAccountID, CFZoneID: n.CFZoneID,
		Hostname: n.Hostname, WorkerURL: n.WorkerURL, WorkerDevURL: n.WorkerDevURL,
		Status: n.Status, Error: n.Error, DeployedAt: n.DeployedAt,
		CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
	}
}

func fromPersisted(p persistedNode) *WorkerNode {
	return &WorkerNode{
		ID: p.ID, Name: p.Name, WorkerName: p.WorkerName,
		WorkerUUID: p.WorkerUUID, CFToken: p.CFToken,
		CFAccountID: p.CFAccountID, CFZoneID: p.CFZoneID,
		Hostname: p.Hostname, WorkerURL: p.WorkerURL, WorkerDevURL: p.WorkerDevURL,
		Status: p.Status, Error: p.Error, DeployedAt: p.DeployedAt,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// ToListItem converts a WorkerNode to a safe-for-frontend summary.
func ToListItem(n *WorkerNode) WorkerNodeListItem {
	return WorkerNodeListItem{
		ID: n.ID, Name: n.Name, WorkerName: n.WorkerName,
		CFAccountID: n.CFAccountID, Hostname: n.Hostname,
		WorkerURL: n.WorkerURL, WorkerDevURL: n.WorkerDevURL,
		Status: n.Status, Error: n.Error, DeployedAt: n.DeployedAt,
		CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
	}
}

// ── crypto helpers ────────────────────────────────────────────────────────────

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

func encryptAES(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(cryptorand.Reader, nonce); err != nil {
		return "", err
	}
	ct := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ct), nil
}

func decryptAES(encrypted string, key []byte) (string, error) {
	data, err := hex.DecodeString(encrypted)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := aesGCM.NonceSize()
	if len(data) < ns {
		return "", errors.New("ciphertext too short")
	}
	plain, err := aesGCM.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
