package nodes

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestLoadOrGenerateKeyPair_PersistsExistingKey(t *testing.T) {
	dataDir := t.TempDir()

	kp1, err := LoadOrGenerateKeyPair(dataDir)
	if err != nil {
		t.Fatalf("first load failed: %v", err)
	}
	if kp1.PublicKeyString() == "" {
		t.Fatalf("expected non-empty public key")
	}

	privPath := filepath.Join(dataDir, keyFilename)
	firstBytes, err := os.ReadFile(privPath)
	if err != nil {
		t.Fatalf("read generated key failed: %v", err)
	}

	kp2, err := LoadOrGenerateKeyPair(dataDir)
	if err != nil {
		t.Fatalf("second load failed: %v", err)
	}
	secondBytes, err := os.ReadFile(privPath)
	if err != nil {
		t.Fatalf("read persisted key failed: %v", err)
	}

	if kp1.PublicKeyString() != kp2.PublicKeyString() {
		t.Fatalf("public key changed across loads:\nfirst:  %s\nsecond: %s", kp1.PublicKeyString(), kp2.PublicKeyString())
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("private key file changed across loads")
	}
}

func TestLoadOrGenerateKeyPair_MigratesLegacyDataDirKey(t *testing.T) {
	// In the new model the key lives in dataDir, so "migration" means:
	// a pre-existing key in dataDir is loaded as-is (no move needed).
	dataDir := t.TempDir()

	expectedPub, err := writeED25519PrivateKey(filepath.Join(dataDir, keyFilename))
	if err != nil {
		t.Fatalf("write pre-existing key failed: %v", err)
	}

	kp, err := LoadOrGenerateKeyPair(dataDir)
	if err != nil {
		t.Fatalf("load with pre-existing key failed: %v", err)
	}

	if kp.PublicKeyString() != expectedPub {
		t.Fatalf("loaded key mismatch:\nexpected: %s\ngot:      %s", expectedPub, kp.PublicKeyString())
	}

	if _, err := os.Stat(filepath.Join(dataDir, keyFilename)); err != nil {
		t.Fatalf("key missing in dataDir: %v", err)
	}
}

func TestLoadOrGenerateKeyPair_InvalidExistingKeyReturnsError(t *testing.T) {
	dataDir := t.TempDir()

	privPath := filepath.Join(dataDir, keyFilename)
	if err := os.WriteFile(privPath, []byte("not-a-valid-private-key"), 0o600); err != nil {
		t.Fatalf("write invalid key failed: %v", err)
	}

	if _, err := LoadOrGenerateKeyPair(dataDir); err == nil {
		t.Fatalf("expected parse error for invalid key, got nil")
	}

	after, err := os.ReadFile(privPath)
	if err != nil {
		t.Fatalf("read invalid key after failed load: %v", err)
	}
	if string(after) != "not-a-valid-private-key" {
		t.Fatalf("invalid key should not be overwritten on parse error")
	}
}

func writeED25519PrivateKey(path string) (string, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}

	block, err := ssh.MarshalPrivateKey(priv, "test@clashforge")
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return "", err
	}

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(ssh.MarshalAuthorizedKey(signer.PublicKey()))), nil
}
