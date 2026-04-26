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
