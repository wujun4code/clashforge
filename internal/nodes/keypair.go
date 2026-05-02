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

const keyFilename = "clashforge_ed25519"

// LoadOrGenerateKeyPair loads the router's ED25519 key pair from dataDir,
// generating one on first run. The key lives at dataDir/clashforge_ed25519
// (i.e. /etc/metaclash/clashforge_ed25519) and is backed up / restored by the
// IPK prerm/postinst scripts so it survives opkg upgrades.
//
// Legacy migration: if the key was previously stored in /root/.ssh/ it is
// moved into dataDir on first run of a new binary.
func LoadOrGenerateKeyPair(dataDir string) (*KeyPair, error) {
	privPath := filepath.Join(dataDir, keyFilename)

	// One-time migration from the old /root/.ssh location.
	legacyPath := "/root/.ssh/" + keyFilename
	if _, err := os.Stat(privPath); os.IsNotExist(err) {
		if data, err := os.ReadFile(legacyPath); err == nil {
			if writeErr := os.WriteFile(privPath, data, 0o600); writeErr == nil {
				_ = os.Remove(legacyPath)
			}
		}
	}

	var privKey ed25519.PrivateKey

	data, err := os.ReadFile(privPath)
	if err == nil {
		raw, parseErr := ssh.ParseRawPrivateKey(data)
		if parseErr != nil {
			return nil, fmt.Errorf("parse SSH private key %s: %w", privPath, parseErr)
		}
		switch k := raw.(type) {
		case ed25519.PrivateKey:
			privKey = k
		case *ed25519.PrivateKey:
			if k == nil {
				return nil, fmt.Errorf("parse SSH private key %s: empty ed25519 key", privPath)
			}
			privKey = *k
		default:
			return nil, fmt.Errorf("unsupported SSH private key type %T in %s", raw, privPath)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read SSH private key %s: %w", privPath, err)
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
