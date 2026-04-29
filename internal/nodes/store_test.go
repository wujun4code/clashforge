package nodes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorePersistsSecrets(t *testing.T) {
	dataDir := t.TempDir()
	s, err := NewStore(dataDir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	n := &Node{
		Name:          "node-1",
		Host:          "1.2.3.4",
		Port:          22,
		Username:      "root",
		Password:      "ssh-pass-123",
		Domain:        "edge.example.com",
		Email:         "ops@example.com",
		CFToken:       "cf-token-abc",
		CFAccountID:   "acc-id",
		CFZoneID:      "zone-id",
		ProxyUser:     "proxy-user",
		ProxyPassword: "proxy-pass-xyz",
		Status:        StatusDeployed,
	}
	if err := s.Create(n); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	nodesPath := filepath.Join(dataDir, "nodes.json")
	raw, err := os.ReadFile(nodesPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", nodesPath, err)
	}
	plain := string(raw)
	for _, secret := range []string{"ssh-pass-123", "cf-token-abc", "proxy-pass-xyz"} {
		if strings.Contains(plain, secret) {
			t.Fatalf("encrypted store leaked secret %q", secret)
		}
	}

	s2, err := NewStore(dataDir)
	if err != nil {
		t.Fatalf("NewStore(reload) error = %v", err)
	}
	got, ok := s2.Get(n.ID)
	if !ok {
		t.Fatalf("Get(%s) not found after reload", n.ID)
	}
	if got.Password != n.Password {
		t.Fatalf("Password mismatch: got %q want %q", got.Password, n.Password)
	}
	if got.CFToken != n.CFToken {
		t.Fatalf("CFToken mismatch: got %q want %q", got.CFToken, n.CFToken)
	}
	if got.ProxyPassword != n.ProxyPassword {
		t.Fatalf("ProxyPassword mismatch: got %q want %q", got.ProxyPassword, n.ProxyPassword)
	}
}

