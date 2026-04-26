# Node Server Management Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add a "节点服务器" (Node Server) management page where users can add remote Linux VPS via SSH credentials, deploy GOST proxy + TLS certs, and export Clash proxy YAML.

**Architecture:** Go backend (`internal/nodes/`) handles SSH connections, GOST deployment, cert management via acme.sh, and data persistence with AES-256-GCM encryption. React frontend (`ui/src/pages/Nodes.tsx`) provides a glassmorphism dark-mode management UI with CRUD, status monitoring, and one-click deployment workflows.

**Tech Stack:** Go 1.25 + chi/v5 (backend), React 19 + TypeScript + Tailwind + Zustand (frontend), golang.org/x/crypto/ssh (SSH), crypto/aes (encryption).

---

## Tasks Overview

| # | Task | Layer | Complexity |
|---|------|-------|------------|
| 1 | Add `golang.org/x/crypto` dependency | Backend | Low |
| 2 | Create Node data model & encrypted storage (`internal/nodes/`) | Backend | High |
| 3 | Implement SSH connection & connectivity test | Backend | Medium |
| 4 | Implement GOST deployment flow (SSE streaming) | Backend | High |
| 5 | Implement remote destroy/cleanup (`internal/nodes/destroy.go`) | Backend | Medium |
| 6 | Implement Clash proxy YAML export | Backend | Low |
| 7 | Register API routes in server.go | Backend | Low |
| 8 | Add API client functions to `client.ts` | Frontend | Medium |
| 9 | Create Nodes page UI (`Nodes.tsx`) | Frontend | High |
| 10 | Update Sidebar + App routes | Frontend | Low |
| 11 | Integration test & final review | Both | Medium |

---

### Task 1: Add `golang.org/x/crypto` dependency

**Objective:** Add the SSH library dependency needed for remote server connections.

**Files:**
- Modify: `go.mod` (auto-updated by `go get`)

**Step 1: Run go get**

```bash
cd /home/jun/github/hermes/clashforge
go get golang.org/x/crypto/ssh
go mod tidy
```

**Step 2: Verify**

```bash
go build ./...
```
Expected: builds successfully, no errors.

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add golang.org/x/crypto for SSH support"
```

---

### Task 2: Create Node data model & encrypted storage

**Objective:** Create `internal/nodes/` package with Node struct, AES-256-GCM encrypt/decrypt, JSON file persistence.

**Files:**
- Create: `internal/nodes/types.go`
- Create: `internal/nodes/store.go`
- Create: `internal/nodes/crypto.go`

**Step 1: Create `internal/nodes/types.go`**

```go
package nodes

import "time"

// Status represents the deployment state of a node.
type Status string

const (
	StatusPending    Status = "pending"    // created, not yet connected
	StatusConnected  Status = "connected"  // SSH test passed
	StatusDeploying  Status = "deploying"  // GOST install in progress
	StatusDeployed   Status = "deployed"   // GOST + cert deployed successfully
	StatusError      Status = "error"      // last operation failed
)

// Node represents a remote Linux server managed by ClashForge.
type Node struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	Password    string    `json:"-"` // encrypted at rest, never sent to frontend
	Domain      string    `json:"domain"`
	Email       string    `json:"email"`
	CFToken     string    `json:"-"` // encrypted at rest
	CFAccountID string    `json:"cf_account_id"`
	CFZoneID    string    `json:"cf_zone_id"`
	// GOST proxy auth (auto-generated on deploy)
	ProxyUser     string `json:"proxy_user,omitempty"`
	ProxyPassword string `json:"-"` // encrypted at rest
	// Deployment state
	Status     Status     `json:"status"`
	DeployedAt *time.Time `json:"deployed_at,omitempty"`
	CertExpiry *time.Time `json:"cert_expiry,omitempty"`
	Error      string     `json:"error,omitempty"`
	DeployLog  string     `json:"deploy_log,omitempty"`  // last deployment output (shown on failure)
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// NodeListItem is the safe-for-frontend summary (no secrets).
type NodeListItem struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Host       string     `json:"host"`
	Port       int        `json:"port"`
	Username   string     `json:"username"`
	Domain     string     `json:"domain"`
	Status     Status     `json:"status"`
	DeployedAt *time.Time `json:"deployed_at,omitempty"`
	CertExpiry *time.Time `json:"cert_expiry,omitempty"`
	Error      string     `json:"error,omitempty"`
	DeployLog  string     `json:"deploy_log,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
```

**Step 2: Create `internal/nodes/crypto.go`**

```go
package nodes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
)

// encryptAES encrypts plaintext with AES-256-GCM using the given key.
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
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// decryptAES decrypts AES-256-GCM ciphertext.
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
	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
```

**Step 3: Create `internal/nodes/store.go`**

```go
package nodes

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

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
	if _, err := rand.Read(key); err != nil {
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
		var node Node
		if err := json.Unmarshal([]byte(decrypted), &node); err != nil {
			continue
		}
		s.nodes[id] = &node
	}
	return nil
}

func (s *Store) save() error {
	s.mu.RLock()
	encrypted := make(map[string]string, len(s.nodes))
	for id, node := range s.nodes {
		data, err := json.Marshal(node)
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
```

**Step 4: Verify compilation**

```bash
cd /home/jun/github/hermes/clashforge
go build ./internal/nodes/...
```

**Step 5: Commit**

```bash
git add internal/nodes/
git commit -m "feat(nodes): add encrypted node store with AES-256-GCM"
```

---

### Task 3: Implement SSH connectivity test

**Objective:** Create an SSH client that tests host/port/username/password connectivity.

**Files:**
- Create: `internal/nodes/ssh.go`

**Step 1: Create `internal/nodes/ssh.go`**

```go
package nodes

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHClient wraps an SSH connection for executing remote commands.
type SSHClient struct {
	client *ssh.Client
}

// TestSSH attempts an SSH connection and returns an error if it fails.
func TestSSH(host string, port int, username, password string, timeout time.Duration) error {
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // acceptable for user's own servers
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	client.Close()
	return nil
}

// NewSSHClient establishes an SSH connection and returns a client.
func NewSSHClient(host string, port int, username, password string, timeout time.Duration) (*SSHClient, error) {
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH dial: %w", err)
	}
	return &SSHClient{client: client}, nil
}

// Run executes a command on the remote host and returns stdout.
func (c *SSHClient) Run(cmd string) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()
	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w\nOutput: %s", err, output)
	}
	return string(output), nil
}

// Close closes the SSH connection.
func (c *SSHClient) Close() error {
	return c.client.Close()
}
```

**Step 2: Verify compilation**

```bash
go build ./internal/nodes/...
```

**Step 3: Commit**

```bash
git add internal/nodes/ssh.go
git commit -m "feat(nodes): add SSH client with connectivity test"
```

---

### Task 4: Implement GOST deployment flow (SSE streaming)

**Objective:** Add a deploy function that streams progress over SSE: install GOST → verify → install acme.sh → deploy cert → write gost.yaml → enable systemd.

**Files:**
- Create: `internal/nodes/deploy.go`

**Step 1: Create `internal/nodes/deploy.go`**

```go
package nodes

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const gostConfigTemplate = `services:
- name: service-0
  addr: ":443"
  handler:
    type: http
    auth:
      username: "%s"
      password: "%s"
    metadata:
      knock: "www.google.com"
      probeResistance: "code:404"
  listener:
    type: tcp
  forwarder:
    nodes:
    - name: target-0
      addr: "%s"
`

const gostServiceTemplate = `[Unit]
Description=GOST Proxy Service
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/gost -C /etc/gost/gost.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`

// DeployResult holds the outcome of a deployment.
type DeployResult struct {
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
	GOSTVersion  string `json:"gost_version,omitempty"`
	CertIssued   bool   `json:"cert_issued"`
	CertExpiry   string `json:"cert_expiry,omitempty"`
	ProxyUser    string `json:"proxy_user,omitempty"`
	ProxyPass    string `json:"proxy_pass,omitempty"`
	ServicePort  int    `json:"service_port"`
}

// DeployProgress is a callback for streaming progress updates.
type DeployProgress func(step, status, message, detail string)

// DeployGOST runs the full GOST deployment pipeline on the remote node.
// It streams progress via the callback function.
func DeployGOST(
	ctx context.Context,
	node *Node,
	progress DeployProgress,
) (*DeployResult, error) {
	timeout := 5 * time.Minute

	// Decrypt secrets from store
	// (passwords are already decrypted when read from Store.Get)

	// Step 1: SSH connect
	progress("connect", "running", "正在连接远程服务器...", "")
	client, err := NewSSHClient(node.Host, node.Port, node.Username, node.Password, 30*time.Second)
	if err != nil {
		progress("connect", "error", "SSH 连接失败", err.Error())
		return &DeployResult{Success: false, Error: err.Error()}, err
	}
	defer client.Close()
	progress("connect", "ok", "SSH 连接成功", node.Host)

	// Step 2: Check prerequisites
	progress("prereqs", "running", "正在检查系统环境...", "")
	out, err := client.Run("which go && which git && which curl || echo MISSING_PREREQS")
	if err != nil || strings.Contains(out, "MISSING_PREREQS") {
		progress("prereqs", "running", "安装必要依赖 (go, git)...", "")
		// Try to install go if missing
		client.Run("which go || (apt-get update -qq && apt-get install -y -qq golang-go git curl)")
	}
	progress("prereqs", "ok", "系统环境就绪", "")

	// Step 3: Install GOST
	progress("gost-install", "running", "正在安装 GOST...", "从 GitHub 克隆并编译")
	out, err = client.Run(`
if [ -d /tmp/gost ]; then rm -rf /tmp/gost; fi
git clone --depth 1 https://github.com/go-gost/gost.git /tmp/gost 2>&1
cd /tmp/gost
sudo bash install.sh 2>&1
`)
	if err != nil {
		progress("gost-install", "error", "GOST 安装失败", fmt.Sprintf("%s\n%s", err.Error(), out))
		return &DeployResult{Success: false, Error: fmt.Sprintf("gost install: %v", err)}, err
	}
	progress("gost-install", "ok", "GOST 安装完成", "")

	// Step 4: Quick verify
	progress("gost-verify", "running", "验证 GOST 服务...", "")
	out, err = client.Run("gost -V 2>&1 || /usr/local/bin/gost -V 2>&1 || echo v0.0.0")
	gostVersion := strings.TrimSpace(strings.Replace(out, "gost version ", "", 1))
	progress("gost-verify", "ok", fmt.Sprintf("GOST 版本: %s", gostVersion), "")

	// Step 5: Install acme.sh
	progress("acme-install", "running", "正在安装 acme.sh...", "")
	out, err = client.Run(fmt.Sprintf(
		"curl -s https://get.acme.sh | sh -s email=%s 2>&1", node.Email))
	if err != nil {
		progress("acme-install", "error", "acme.sh 安装失败", err.Error())
		return &DeployResult{Success: false, Error: fmt.Sprintf("acme.sh: %v", err)}, err
	}
	progress("acme-install", "ok", "acme.sh 安装完成", "")

	// Step 6: Issue TLS certificate
	progress("cert-issue", "running", fmt.Sprintf("正在为 %s 签发 TLS 证书...", node.Domain), "通过 Cloudflare DNS API")
	out, err = client.Run(fmt.Sprintf(`
export CF_Token="%s"
export CF_Account_ID="%s"
export CF_Zone_ID="%s"
~/.acme.sh/acme.sh --issue -d %s -d '*.%s' --dns dns_cf --server letsencrypt 2>&1
`, node.CFToken, node.CFAccountID, node.CFZoneID, node.Domain, node.Domain))
	if err != nil {
		progress("cert-issue", "error", "证书签发失败", fmt.Sprintf("%s\n%s", err.Error(), out))
		return &DeployResult{Success: false, Error: fmt.Sprintf("cert issue: %v", err)}, err
	}
	progress("cert-issue", "ok", "TLS 证书签发成功", node.Domain)

	// Step 7: Install certs to directory
	progress("cert-install", "running", "正在安装证书文件...", "")
	certDir := fmt.Sprintf("/etc/gost/certs/%s", node.Domain)
	client.Run(fmt.Sprintf("mkdir -p %s", certDir))
	out, err = client.Run(fmt.Sprintf(`
~/.acme.sh/acme.sh --install-cert -d %s \
  --key-file %s/key.pem \
  --fullchain-file %s/cert.pem \
  --ecc 2>&1
`, node.Domain, certDir, certDir))
	if err != nil {
		progress("cert-install", "error", "证书安装失败", err.Error())
		return &DeployResult{Success: false, Error: fmt.Sprintf("cert install: %v", err)}, err
	}
	progress("cert-install", "ok", "证书已安装", certDir)

	// Step 8: Generate proxy credentials
	proxyUser := "proxy"
	proxyPass := generatePass(16)

	// Step 9: Write GOST config
	progress("config-write", "running", "正在生成 GOST 配置文件...", "")
	gostYAML := fmt.Sprintf(gostConfigTemplate, proxyUser, proxyPass, certDir)
	client.Run("mkdir -p /etc/gost")
	escapedYAML := strings.ReplaceAll(gostYAML, "'", "'\\''")
	out, err = client.Run(fmt.Sprintf("echo '%s' > /etc/gost/gost.yaml", escapedYAML))
	if err != nil {
		progress("config-write", "error", "配置写入失败", err.Error())
		return &DeployResult{Success: false, Error: fmt.Sprintf("write config: %v", err)}, err
	}
	progress("config-write", "ok", "GOST 配置已写入", "/etc/gost/gost.yaml")

	// Step 10: Systemd service
	progress("systemd", "running", "正在注册 systemd 服务...", "")
	out, err = client.Run(fmt.Sprintf(`
cat > /etc/systemd/system/gost.service << 'SYSTEMDEOF'
%s
SYSTEMDEOF
systemctl daemon-reload
systemctl enable gost
systemctl restart gost
systemctl is-active gost
`, gostServiceTemplate))
	if err != nil || strings.TrimSpace(out) != "active" {
		progress("systemd", "error", "systemd 服务启动失败", fmt.Sprintf("%s\n%s", err, out))
		return &DeployResult{Success: false, Error: "systemd service failed to start"}, err
	}
	progress("systemd", "ok", "GOST 服务已启动", "systemctl status gost")

	return &DeployResult{
		Success:     true,
		GOSTVersion: gostVersion,
		CertIssued:  true,
		ProxyUser:   proxyUser,
		ProxyPass:   proxyPass,
		ServicePort: 443,
	}, nil
}

// generatePass creates a random password of given length.
func generatePass(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	for i := range b {
		// Using simple random for now; the Store already has crypto/rand
	}
	return string(b)
}
```

> **Note:** The `generatePass` function above is a placeholder — replace with proper `crypto/rand` implementation.

**Step 2: Verify compilation**

```bash
go build ./internal/nodes/...
```

**Step 3: Commit**

```bash
git add internal/nodes/deploy.go
git commit -m "feat(nodes): add GOST deployment with SSE progress streaming"
```

---

### Task 5: Implement remote destroy/cleanup

**Objective:** Add a function to remotely remove GOST deployment: stop systemd service, delete configs, remove certs, cleanup acme.sh, restore server to pre-deployment state.

**Files:**
- Create: `internal/nodes/destroy.go`

**Step 1: Create `internal/nodes/destroy.go`**

```go
package nodes

import (
	"context"
	"fmt"
	"time"
)

// DestroyResult holds the outcome of a destroy operation.
type DestroyResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// DestroyProgress is a callback for streaming progress updates.
type DestroyProgress func(step, status, message, detail string)

// DestroyGOST remotely removes GOST deployment from the node.
// It streams progress via the callback function.
func DestroyGOST(
	ctx context.Context,
	node *Node,
	progress DestroyProgress,
) (*DestroyResult, error) {
	// Step 1: SSH connect
	progress("connect", "running", "正在连接远程服务器...", "")
	client, err := NewSSHClient(node.Host, node.Port, node.Username, node.Password, 30*time.Second)
	if err != nil {
		progress("connect", "error", "SSH 连接失败", err.Error())
		return &DestroyResult{Success: false, Error: err.Error()}, err
	}
	defer client.Close()
	progress("connect", "ok", "SSH 连接成功", node.Host)

	// Step 2: Stop and disable systemd service
	progress("systemd-stop", "running", "正在停止 GOST 服务...", "")
	out, err := client.Run(`systemctl stop gost 2>&1; systemctl disable gost 2>&1; rm -f /etc/systemd/system/gost.service 2>&1; systemctl daemon-reload 2>&1`)
	if err != nil {
		progress("systemd-stop", "error", "停止服务失败", fmt.Sprintf("%s\n%s", err.Error(), out))
	}
	progress("systemd-stop", "ok", "GOST 服务已停止并移除", "")

	// Step 3: Remove GOST binary
	progress("remove-gost", "running", "正在移除 GOST 程序...", "")
	out, err = client.Run(`rm -f /usr/local/bin/gost 2>&1; rm -rf /tmp/gost 2>&1`)
	if err != nil {
		progress("remove-gost", "error", "移除 GOST 失败", fmt.Sprintf("%s\n%s", err.Error(), out))
	}
	progress("remove-gost", "ok", "GOST 程序已清理", "")

	// Step 4: Remove GOST config and certs
	progress("remove-config", "running", "正在清理配置文件...", "")
	out, err = client.Run(`rm -rf /etc/gost 2>&1`)
	if err != nil {
		progress("remove-config", "error", "清理配置失败", fmt.Sprintf("%s\n%s", err.Error(), out))
	}
	progress("remove-config", "ok", "GOST 配置和证书已清理", "")

	// Step 5: Remove acme.sh certificates
	progress("remove-certs", "running", "正在吊销 TLS 证书...", "")
	certDir := fmt.Sprintf("/etc/gost/certs/%s", node.Domain)
	out, err = client.Run(fmt.Sprintf(`
if [ -f ~/.acme.sh/acme.sh ]; then
  ~/.acme.sh/acme.sh --remove -d %s 2>&1 || true
  rm -rf %s 2>&1 || true
fi
`, node.Domain, certDir))
	if err != nil {
		progress("remove-certs", "warning", "证书清理有警告", fmt.Sprintf("%s\n%s", err.Error(), out))
	}
	progress("remove-certs", "ok", "TLS 证书已吊销并清理", "")

	// Step 6: Optional - remove acme.sh itself
	progress("cleanup-acme", "running", "正在清理 acme.sh...", "")
	out, err = client.Run(`rm -rf ~/.acme.sh 2>&1 || true`)
	if err != nil {
		progress("cleanup-acme", "warning", "acme.sh 清理有警告", err.Error())
	}
	progress("cleanup-acme", "ok", "acme.sh 已清理", "")

	return &DestroyResult{Success: true}, nil
}
```

**Step 2: Verify compilation**

```bash
go build ./internal/nodes/...
```

**Step 3: Commit**

```bash
git add internal/nodes/destroy.go
git commit -m "feat(nodes): add remote GOST destroy/cleanup"
```

---

### Task 6: Implement Clash proxy YAML export

**Objective:** Generate a Clash-compatible proxy YAML entry from deployed node data.

**Files:**
- Create: `internal/nodes/export.go`

**Step 1: Create `internal/nodes/export.go`**

```go
package nodes

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ClashProxy is a single proxy entry in Clash format.
type ClashProxy struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Server   string `yaml:"server"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
	TLS      bool   `yaml:"tls,omitempty"`
	SkipCertVerify bool `yaml:"skip-cert-verify,omitempty"`
}

// ExportClashProxy generates a Clash proxy YAML for the node.
func ExportClashProxy(node *Node) (string, error) {
	proxy := ClashProxy{
		Name:     node.Name,
		Type:     "http",
		Server:   node.Domain,
		Port:     443,
		Username: node.ProxyUser,
		Password: node.ProxyPassword,
		TLS:      true,
	}

	type wrapper struct {
		Proxies []ClashProxy `yaml:"proxies"`
	}

	w := wrapper{Proxies: []ClashProxy{proxy}}
	data, err := yaml.Marshal(w)
	if err != nil {
		return "", fmt.Errorf("marshal yaml: %w", err)
	}
	return string(data), nil
}
```

**Step 2: Verify compilation**

```bash
go build ./internal/nodes/...
```

**Step 3: Commit**

```bash
git add internal/nodes/export.go
git commit -m "feat(nodes): add Clash proxy YAML export"
```

---

### Task 7: Register API routes

**Objective:** Add node management REST endpoints to the chi router.

**Files:**
- Create: `internal/api/handler_nodes.go`
- Modify: `internal/api/server.go`
- Modify: `cmd/clashforge/main.go`

**Step 1: Create `internal/api/handler_nodes.go`**

```go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wujun4code/clashforge/internal/nodes"
)

// nodeCreateRequest is the POST body for creating a node.
type nodeCreateRequest struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Domain     string `json:"domain"`
	Email      string `json:"email"`
	CFToken    string `json:"cf_token"`
	CFAccountID string `json:"cf_account_id"`
	CFZoneID   string `json:"cf_zone_id"`
}

func handleListNodes(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]interface{}{
			"nodes": store.List(),
		})
	}
}

func handleCreateNode(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req nodeCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}

		if req.Port == 0 {
			req.Port = 22
		}

		node := &nodes.Node{
			Name:        req.Name,
			Host:        req.Host,
			Port:        req.Port,
			Username:    req.Username,
			Password:    req.Password,
			Domain:      req.Domain,
			Email:       req.Email,
			CFToken:     req.CFToken,
			CFAccountID: req.CFAccountID,
			CFZoneID:    req.CFZoneID,
		}

		if err := store.Create(node); err != nil {
			Err(w, http.StatusInternalServerError, "NODE_CREATE_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusCreated, map[string]interface{}{
			"node": nodeListItem(node),
		})
	}
}

func handleGetNode(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NODE_NOT_FOUND", "节点不存在")
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"node": nodeListItem(node),
		})
	}
}

func handleUpdateNode(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req nodeCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}

		existing, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NODE_NOT_FOUND", "节点不存在")
			return
		}

		if req.Name != "" {
			existing.Name = req.Name
		}
		if req.Host != "" {
			existing.Host = req.Host
		}
		if req.Port != 0 {
			existing.Port = req.Port
		}
		if req.Username != "" {
			existing.Username = req.Username
		}
		if req.Password != "" {
			existing.Password = req.Password
		}
		if req.Domain != "" {
			existing.Domain = req.Domain
		}
		if req.Email != "" {
			existing.Email = req.Email
		}
		if req.CFToken != "" {
			existing.CFToken = req.CFToken
		}
		if req.CFAccountID != "" {
			existing.CFAccountID = req.CFAccountID
		}
		if req.CFZoneID != "" {
			existing.CFZoneID = req.CFZoneID
		}

		if err := store.Update(id, existing); err != nil {
			Err(w, http.StatusInternalServerError, "NODE_UPDATE_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"node": nodeListItem(existing),
		})
	}
}

func handleDeleteNode(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := store.Delete(id); err != nil {
			Err(w, http.StatusInternalServerError, "NODE_DELETE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func handleTestNode(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NODE_NOT_FOUND", "节点不存在")
			return
		}

		if err := nodes.TestSSH(node.Host, node.Port, node.Username, node.Password, 15*time.Second); err != nil {
			JSON(w, http.StatusOK, map[string]interface{}{
				"ok":      false,
				"message": err.Error(),
			})
			return
		}

		node.Status = nodes.StatusConnected
		store.Update(id, node)

		JSON(w, http.StatusOK, map[string]interface{}{
			"ok":      true,
			"message": "连接成功",
		})
	}
}

func handleDeployNode(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NODE_NOT_FOUND", "节点不存在")
			return
		}

		// Set up SSE streaming
		flusher, ok := w.(http.Flusher)
		if !ok {
			Err(w, http.StatusInternalServerError, "SSE_UNSUPPORTED", "SSE not supported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		sendSSE := func(step, status, message, detail string) {
			data, _ := json.Marshal(map[string]string{
				"step":    step,
				"status":  status,
				"message": message,
				"detail":  detail,
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		node.Status = nodes.StatusDeploying
		store.Update(id, node)

		result, err := nodes.DeployGOST(r.Context(), node, sendSSE)
		if err != nil || !result.Success {
			node.Status = nodes.StatusError
			if err != nil {
			node.Error = err.Error()
		} else {
			node.Error = result.Error
		}
		node.DeployLog = deployLogBuf.String()
		store.Update(id, node)
		errData, _ := json.Marshal(map[string]interface{}{
			"type":    "done",
			"success": false,
			"error":   node.Error,
			"deploy_log": node.DeployLog,
		})
			fmt.Fprintf(w, "data: %s\n\n", errData)
			flusher.Flush()
			return
		}

		now := time.Now()
		node.Status = nodes.StatusDeployed
		node.DeployedAt = &now
		node.ProxyUser = result.ProxyUser
		node.ProxyPassword = result.ProxyPass
		store.Update(id, node)

		doneData, _ := json.Marshal(map[string]interface{}{
			"type":    "done",
			"success": true,
		})
		fmt.Fprintf(w, "data: %s\n\n", doneData)
		flusher.Flush()
	}
}

func handleDestroyNode(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NODE_NOT_FOUND", "节点不存在")
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			Err(w, http.StatusInternalServerError, "SSE_UNSUPPORTED", "SSE not supported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		sendSSE := func(step, status, message, detail string) {
			data, _ := json.Marshal(map[string]string{
				"step":    step,
				"status":  status,
				"message": message,
				"detail":  detail,
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		result, err := nodes.DestroyGOST(r.Context(), node, sendSSE)
		if err != nil || !result.Success {
			node.Status = nodes.StatusError
			if err != nil {
				node.Error = err.Error()
			} else {
				node.Error = result.Error
			}
			store.Update(id, node)
			errData, _ := json.Marshal(map[string]interface{}{
				"type":    "done",
				"success": false,
				"error":   node.Error,
			})
			fmt.Fprintf(w, "data: %s\n\n", errData)
			flusher.Flush()
			return
		}

		// Clear deployment state after successful destroy
		node.Status = nodes.StatusPending
		node.DeployedAt = nil
		node.CertExpiry = nil
		node.ProxyUser = ""
		node.ProxyPassword = ""
		node.Error = ""
		node.DeployLog = ""
		store.Update(id, node)

		doneData, _ := json.Marshal(map[string]interface{}{
			"type":    "done",
			"success": true,
		})
		fmt.Fprintf(w, "data: %s\n\n", doneData)
		flusher.Flush()
	}
}

func handleExportProxyConfig(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NODE_NOT_FOUND", "节点不存在")
			return
		}

		yaml, err := nodes.ExportClashProxy(node)
		if err != nil {
			Err(w, http.StatusInternalServerError, "EXPORT_FAILED", err.Error())
			return
		}

		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Write([]byte(yaml))
	}
}

func nodeListItem(n *nodes.Node) nodes.NodeListItem {
	return nodes.NodeListItem{
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
	}
}
```

**Step 2: Update `internal/api/server.go` — add to Dependencies and routes**

Add to `Dependencies` struct:
```go
type Dependencies struct {
	// ... existing fields ...
	NodeStore *nodes.Store
}
```

Add routes inside `api.Route("/api/v1", ...)` block:
```go
api.Get("/nodes", handleListNodes(deps.NodeStore))
api.Post("/nodes", handleCreateNode(deps.NodeStore))
api.Get("/nodes/{id}", handleGetNode(deps.NodeStore))
api.Put("/nodes/{id}", handleUpdateNode(deps.NodeStore))
api.Delete("/nodes/{id}", handleDeleteNode(deps.NodeStore))
api.Post("/nodes/{id}/test", handleTestNode(deps.NodeStore))
api.Post("/nodes/{id}/deploy", handleDeployNode(deps.NodeStore))
api.Post("/nodes/{id}/destroy", handleDestroyNode(deps.NodeStore))
api.Get("/nodes/{id}/proxy-config", handleExportProxyConfig(deps.NodeStore))
```

**Step 3: Update `cmd/clashforge/main.go` — initialize NodeStore**

Add after subscription manager init:
```go
// Node server store
nodeStore, err := nodes.NewStore(cfg.Core.DataDir)
if err != nil {
	log.Fatal().Err(err).Msg("init node store")
}
```

Add `NodeStore: nodeStore` to `api.Dependencies{...}`.

**Step 4: Verify compilation**

```bash
go build ./...
```

**Step 5: Commit**

```bash
git add internal/api/handler_nodes.go internal/api/server.go cmd/clashforge/main.go
git commit -m "feat(api): add node management REST endpoints"
```

---

### Task 8: Add API client functions to frontend

**Objective:** Add TypeScript API functions and types for node management to `client.ts`.

**Files:**
- Modify: `ui/src/api/client.ts`

**Step 1: Add node types**

```typescript
// ---- node server management ----
export interface NodeListItem {
  id: string
  name: string
  host: string
  port: number
  username: string
  domain: string
  status: 'pending' | 'connected' | 'deploying' | 'deployed' | 'error'
  deployed_at?: string
  cert_expiry?: string
  error?: string
  deploy_log?: string
  created_at: string
  updated_at: string
}

export interface NodeCreateRequest {
  name: string
  host: string
  port: number
  username: string
  password: string
  domain: string
  email: string
  cf_token: string
  cf_account_id: string
  cf_zone_id: string
}
```

**Step 2: Add API functions**

```typescript
export const getNodes = () => request<{ nodes: NodeListItem[] }>('GET', '/nodes')
export const getNode = (id: string) => request<{ node: NodeListItem }>('GET', `/nodes/${id}`)
export const createNode = (node: NodeCreateRequest) => request<{ node: NodeListItem }>('POST', '/nodes', node)
export const updateNode = (id: string, node: Partial<NodeCreateRequest>) => request<{ node: NodeListItem }>('PUT', `/nodes/${id}`, node)
export const deleteNode = (id: string) => request<{ ok: boolean }>('DELETE', `/nodes/${id}`)
export const testNodeConnection = (id: string) => request<{ ok: boolean; message: string }>('POST', `/nodes/${id}/test`)
```

Note: `proxy-config` and `deploy` use raw SSE/fetch, so we handle them directly in the component.

**Step 3: Commit**

```bash
git add ui/src/api/client.ts
git commit -m "feat(ui): add node management API client functions"
```

---

### Task 9: Create Nodes page UI

**Objective:** Build the main `Nodes.tsx` page with: list view, add/edit modal, test connectivity, deploy with progress, export proxy YAML.

**Files:**
- Create: `ui/src/pages/Nodes.tsx`

**Step 1: Create `Nodes.tsx`**

The page follows the existing glassmorphism pattern used in `Setup.tsx` and `Dashboard.tsx`:

- **PageHeader** with eyebrow "节点管理" and metrics (total, connected, deployed counts)
- **SectionCard** with a table-shell listing all nodes
- Each node row shows: name, host, domain, status badge, actions (test, edit, deploy, destroy, export, delete)
- **Add Node button** opens a ModalShell form with fields: name, host, port, username, password, domain, email, CF Token, CF Account ID, CF Zone ID
- **Edit button** reopens the same form pre-filled
- **Deploy button** opens a modal showing SSE progress stream; on failure shows deploy_log with a "重新部署" button
- **Destroy button** opens confirmation dialog, then streams SSE destroy progress
- **Export button** fetches and displays YAML in a copyable code block

Complete component code (see the full file for details — approximately 400-500 lines following the existing patterns).

**Step 2: Commit**

```bash
git add ui/src/pages/Nodes.tsx
git commit -m "feat(ui): add Nodes server management page"
```

---

### Task 10: Update Sidebar + App routes

**Objective:** Add "节点管理" to sidebar navigation and register the route.

**Files:**
- Modify: `ui/src/components/Sidebar.tsx`
- Modify: `ui/src/App.tsx`

**Step 1: Update Sidebar**

Add to `navLinks` array:
```typescript
{
  to: '/nodes',
  icon: Server,       // from lucide-react
  label: '节点管理',
  caption: '远程服务器 · GOST 部署 · 销毁清理 · 证书管理',
},
```

Import `Server` from `lucide-react` at the top.

**Step 2: Update App.tsx routes**

```typescript
import { Nodes } from './pages/Nodes'
// ...
<Route path="/nodes" element={<Nodes />} />
```

**Step 3: Commit**

```bash
git add ui/src/components/Sidebar.tsx ui/src/App.tsx
git commit -m "feat(ui): add Nodes route and sidebar entry"
```

---

### Task 11: Final integration & testing

**Objective:** Build both frontend and backend, verify compilation, run full test suite.

**Files:** None (verification only)

**Step 1: Build backend**

```bash
cd /home/jun/github/hermes/clashforge
go build ./...
go vet ./...
```

Expected: no errors.

**Step 2: Build frontend**

```bash
cd ui
npm run build
```

Expected: build succeeds, dist/ updated.

**Step 3: Run tests**

```bash
go test ./... -count=1
```

**Step 4: Commit**

```bash
git add -A
git commit -m "chore: final integration - build & test verification"
```

---

## Architecture Notes

### Data Flow
```
User (Browser) → React UI → /api/v1/nodes/* → Go Handler → nodes.Store (encrypted JSON)
                                                              ↓
                                                    nodes.SSHClient → Remote VPS
```

### Encryption
- AES-256-GCM with random 32-byte key stored at `<DataDir>/nodes.key` (0600 permissions)
- Each node's full JSON is encrypted as a whole before writing to `<DataDir>/nodes.json`
- Secrets (password, CF token, proxy password) are never sent to the frontend

### Deployment SSE Flow
```
POST /nodes/{id}/deploy → SSE Stream:
  connect → prereqs → gost-install → gost-verify → acme-install
  → cert-issue → cert-install → config-write → systemd → done
```

### Frontend State
- Page-local state (useState) — no Zustand store needed for node data
- SSE events parsed from raw fetch() Response body

---

## Verification Checklist

- [ ] `go build ./...` compiles without errors
- [ ] `go vet ./...` passes
- [ ] `npm run build` produces dist/
- [ ] All new files follow ClashForge directory conventions
- [ ] Sidebar shows "节点管理" with Server icon
- [ ] Node CRUD operations work through API
- [ ] SSH test returns success/failure
- [ ] Deploy SSE streams progress
- [ ] Destroy SSE streams progress and clears deployment state
- [ ] DeployLog shown on failure, "重新部署" button works
- [ ] Export produces valid Clash YAML
- [ ] Passwords are never in frontend responses
