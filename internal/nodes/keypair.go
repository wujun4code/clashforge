package nodes

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// KeyPair holds the router's SSH key pair used to authenticate to managed servers.
type KeyPair struct {
	signer    ssh.Signer
	pubKeyStr string // authorized_keys line, e.g. "ssh-ed25519 AAAA... clashforge@openwrt"
}

// LoadOrGenerateKeyPair loads the router's ED25519 key pair from dataDir,
// generating one on first run and persisting it as clashforge_ed25519.
// If an existing key file is unreadable or not ED25519, it is overwritten.
func LoadOrGenerateKeyPair(dataDir string) (*KeyPair, error) {
	privPath := filepath.Join(dataDir, "clashforge_ed25519")

	var privKey ed25519.PrivateKey

	if data, err := os.ReadFile(privPath); err == nil {
		raw, parseErr := ssh.ParseRawPrivateKey(data)
		if parseErr == nil {
			if k, ok := raw.(ed25519.PrivateKey); ok {
				privKey = k
			}
			// if wrong type, fall through to regenerate
		}
		// parse error or wrong type: fall through to regenerate below
	}

	if privKey == nil {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate SSH key: %w", err)
		}
		privKey = priv

		block, err := ssh.MarshalPrivateKey(privKey, "clashforge@openwrt")
		if err != nil {
			return nil, fmt.Errorf("marshal SSH private key: %w", err)
		}
		if err := os.MkdirAll(dataDir, 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(privPath, pem.EncodeToMemory(block), 0o600); err != nil {
			return nil, fmt.Errorf("write SSH private key: %w", err)
		}
	}

	signer, err := ssh.NewSignerFromKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("create SSH signer: %w", err)
	}

	pubKeyStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))

	return &KeyPair{
		signer:    signer,
		pubKeyStr: pubKeyStr,
	}, nil
}

// PublicKeyString returns the public key in authorized_keys format.
func (kp *KeyPair) PublicKeyString() string {
	return kp.pubKeyStr
}

// SSHAuthMethod returns the SSH auth method using this key pair.
func (kp *KeyPair) SSHAuthMethod() ssh.AuthMethod {
	return ssh.PublicKeys(kp.signer)
}
