package publish

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreEncryptsWorkerTokenAndCanDecrypt(t *testing.T) {
	dataDir := t.TempDir()
	s, err := NewStore(dataDir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	view, err := s.UpsertWorkerConfig(WorkerConfigInput{
		Name:         "Prod",
		WorkerName:   "cf-worker-prod",
		WorkerURL:    "https://sub.example.com",
		WorkerDevURL: "https://cf-worker-prod.example.workers.dev",
		Hostname:     "sub.example.com",
		AccountID:    "acc_1",
		NamespaceID:  "ns_1",
		ZoneID:       "zone_1",
		Token:        "token-secret-123",
	})
	if err != nil {
		t.Fatalf("UpsertWorkerConfig() error = %v", err)
	}
	if !view.HasToken {
		t.Fatal("expected HasToken=true after upsert")
	}

	configPath := filepath.Join(dataDir, "publish-worker-configs.json")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", configPath, err)
	}
	if strings.Contains(string(raw), "token-secret-123") {
		t.Fatalf("plaintext token leaked into %s", configPath)
	}

	s2, err := NewStore(dataDir)
	if err != nil {
		t.Fatalf("NewStore(reload) error = %v", err)
	}
	token, err := s2.GetWorkerConfigToken(view.ID)
	if err != nil {
		t.Fatalf("GetWorkerConfigToken() error = %v", err)
	}
	if token != "token-secret-123" {
		t.Fatalf("token mismatch: got %q want %q", token, "token-secret-123")
	}
}
