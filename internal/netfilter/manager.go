package netfilter

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

// Applier is the interface implemented by both backends.
type Applier interface {
	Apply() error
	Cleanup() error
}

// Manager coordinates netfilter rule management.
type Manager struct {
	backend Applier
	kind    Backend
	applied bool
	cfg     Config
}

// Config holds configuration for netfilter setup.
type Config struct {
	Mode              string // tproxy | redir | none
	FirewallBackend   string // auto | nftables | iptables | none
	TProxyPort        int
	DNSPort           int
	EnableDNSRedirect bool
	BypassFakeIP      bool
	BypassCIDR        []string
}

// NewManager creates a Manager and detects the appropriate backend.
func NewManager(cfg Config) *Manager {
	kind := Detect(cfg.FirewallBackend)
	var applier Applier
	switch kind {
	case BackendNftables:
		applier = &NftablesBackend{TProxyPort: cfg.TProxyPort, DNSPort: cfg.DNSPort, EnableDNSRedirect: cfg.EnableDNSRedirect, BypassFakeIP: cfg.BypassFakeIP, BypassCIDR: cfg.BypassCIDR}
	case BackendIptables:
		applier = &IptablesBackend{TProxyPort: cfg.TProxyPort, DNSPort: cfg.DNSPort, EnableDNSRedirect: cfg.EnableDNSRedirect}
	default:
		applier = &noopBackend{}
	}
	return &Manager{backend: applier, kind: kind, cfg: cfg}
}

// Apply applies firewall rules if mode != "none".
func (m *Manager) Apply() error {
	if m.cfg.Mode == "none" {
		log.Info().Msg("netfilter: mode=none, skipping rule setup")
		return nil
	}
	log.Info().Str("backend", m.kind.String()).Str("mode", m.cfg.Mode).Msg("applying netfilter rules")
	if err := m.backend.Apply(); err != nil {
		return fmt.Errorf("apply netfilter rules: %w", err)
	}
	m.applied = true
	log.Info().Msg("netfilter rules applied")
	return nil
}

// Cleanup removes all managed firewall rules.
func (m *Manager) Cleanup() error {
	if !m.applied {
		return nil
	}
	log.Info().Str("backend", m.kind.String()).Msg("cleaning up netfilter rules")
	if err := m.backend.Cleanup(); err != nil {
		return fmt.Errorf("cleanup netfilter rules: %w", err)
	}
	m.applied = false
	return nil
}

// Backend returns the detected backend name.
func (m *Manager) BackendName() string {
	return m.kind.String()
}

// IsApplied reports whether rules are currently applied.
func (m *Manager) IsApplied() bool {
	return m.applied
}

type noopBackend struct{}

func (n *noopBackend) Apply() error   { return nil }
func (n *noopBackend) Cleanup() error { return nil }
