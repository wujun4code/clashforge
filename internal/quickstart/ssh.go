package quickstart

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHClient wraps a connected SSH session.
type SSHClient struct {
	client *ssh.Client
}

// DialSSH connects to a VPS using password or private-key authentication.
func DialSSH(creds *VPSCredentials) (*SSHClient, error) {
	port := creds.Port
	if port == 0 {
		port = 22
	}
	user := creds.User
	if user == "" {
		user = "root"
	}

	var authMethods []ssh.AuthMethod
	switch creds.AuthType {
	case "key":
		signer, err := parsePrivateKey(creds.PrivKey)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	default: // "password" or empty
		authMethods = append(authMethods, ssh.Password(creds.Password))
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec — operator-selected VPS
		Timeout:         15 * time.Second,
	}

	addr := net.JoinHostPort(creds.Host, fmt.Sprintf("%d", port))
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	return &SSHClient{client: client}, nil
}

// Close closes the SSH connection.
func (c *SSHClient) Close() { _ = c.client.Close() }

// Run executes a command and returns its combined stdout+stderr output.
func (c *SSHClient) Run(cmd string) (string, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer sess.Close()

	var buf bytes.Buffer
	sess.Stdout = &buf
	sess.Stderr = &buf

	if err := sess.Run(cmd); err != nil {
		out := strings.TrimSpace(buf.String())
		return out, fmt.Errorf("run %q: %w — %s", cmd, err, out)
	}
	return strings.TrimSpace(buf.String()), nil
}

// RunOutput executes a command and returns stdout only (for value extraction).
func (c *SSHClient) RunOutput(cmd string) (string, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	if err := sess.Run(cmd); err != nil {
		return "", fmt.Errorf("run %q: %w — %s", cmd, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// WriteFile uploads arbitrary content to a remote path via base64 encoding.
// mode is an octal string like "0644"; defaults to "0644".
func (c *SSHClient) WriteFile(remotePath, content string, mode string) error {
	if mode == "" {
		mode = "0644"
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	// Write via printf to avoid shell quoting issues with arbitrary content.
	cmd := fmt.Sprintf(
		`printf '%%s' '%s' | base64 -d > %s && chmod %s %s`,
		encoded, remotePath, mode, remotePath,
	)
	_, err := c.Run(cmd)
	return err
}

// MkdirP creates a directory (and parents) on the remote host.
func (c *SSHClient) MkdirP(remotePath string) error {
	_, err := c.Run("mkdir -p " + remotePath)
	return err
}

// parsePrivateKey parses a PEM-encoded private key (RSA, EC, ED25519).
func parsePrivateKey(pemContent string) (ssh.Signer, error) {
	pemContent = strings.TrimSpace(pemContent)
	if pemContent == "" {
		return nil, fmt.Errorf("private key is empty")
	}
	signer, err := ssh.ParsePrivateKey([]byte(pemContent))
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return signer, nil
}
