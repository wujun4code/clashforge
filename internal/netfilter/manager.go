package netfilter

import (
	"errors"
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
	// Cleanup ownership survives partial Apply and failed Cleanup.
	tproxyPending bool
	// Successful ownership and unfinished rollback are separate from readiness.
	tunManaged        bool
	tunPendingForward bool
	tunPendingRoutes  []tunRouteSpec
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
	EnableIPv6        bool   // intercept IPv6 traffic via tproxy as well
	DropQUIC          bool   // block UDP 443 when true; otherwise use normal UDP TProxy (fallback is client-dependent)
	WANInterface      string // WAN-facing interface name (e.g. "pppoe-wan", "eth1")
	TunDevice         string // TUN interface name when Mode == "tun" (e.g. "Meta"); empty uses mihomo's default
	// FakeIPRange is mihomo's actually-configured dns.fake-ip-range (see
	// config.SelectFakeIPRange / config.ReadRuntimeFakeIPRange) — passed
	// through to both TProxy backends so router-originated traffic to fake-ip
	// destinations is matched and redirected correctly even when it isn't
	// the historical 198.18.0.0/16 default. Empty falls back to that default.
	FakeIPRange string
}

// NewManager creates a Manager and detects the appropriate backend.
func NewManager(cfg Config) *Manager {
	kind := Detect(cfg.FirewallBackend)
	var applier Applier
	switch kind {
	case BackendNftables:
		applier = &NftablesBackend{TProxyPort: cfg.TProxyPort, DNSPort: cfg.DNSPort, EnableDNSRedirect: cfg.EnableDNSRedirect, BypassFakeIP: cfg.BypassFakeIP, BypassCIDR: cfg.BypassCIDR, EnableIPv6: cfg.EnableIPv6, DropQUIC: cfg.DropQUIC, WANInterface: cfg.WANInterface, FakeIPRange: cfg.FakeIPRange}
	case BackendIptables:
		applier = &IptablesBackend{TProxyPort: cfg.TProxyPort, DNSPort: cfg.DNSPort, EnableDNSRedirect: cfg.EnableDNSRedirect, BypassFakeIP: cfg.BypassFakeIP, BypassCIDR: cfg.BypassCIDR, EnableIPv6: cfg.EnableIPv6, DropQUIC: cfg.DropQUIC, FakeIPRange: cfg.FakeIPRange}
	default:
		applier = &noopBackend{}
	}
	return &Manager{backend: applier, kind: kind, cfg: cfg}
}

// Apply installs the configured backend. TUN currently verifies supplemental
// IPv4 rules and fw4 forward_lan only, not end-to-end connectivity or all zones.
func (m *Manager) Apply() error {
	if m.cfg.Mode == "tun" {
		return m.applyTUN()
	}
	if m.cfg.Mode == "none" {
		m.applied = false
		log.Info().Str("mode", m.cfg.Mode).Msg("netfilter: skipping rule setup")
		return nil
	}
	m.applied = false
	if m.kind == BackendNone {
		return fmt.Errorf("netfilter mode %s requires an available nftables or iptables backend (configured %s)", m.cfg.Mode, m.cfg.FirewallBackend)
	}
	log.Info().Str("backend", m.kind.String()).Str("mode", m.cfg.Mode).Msg("applying netfilter rules")
	// Apply may replace previously installed rules before failing. Do not retain
	// a stale success status after an unsuccessful re-apply.
	m.tproxyPending = true
	if err := m.backend.Apply(); err != nil {
		return fmt.Errorf("apply netfilter rules: %w", err)
	}
	m.applied = true
	log.Info().Msg("netfilter rules applied")
	return nil
}

// Cleanup removes all managed firewall rules.
func (m *Manager) Cleanup() error {
	if m.cfg.Mode == "tun" {
		m.applied = false
		if !m.tunManaged {
			return m.rollbackTUN()
		}
		err := errors.Join(RemoveTunRouteRule(), RemoveTunForwardAccept(m.cfg.TunDevice))
		// Keep ownership until all cleanup succeeds; callers can retry safely.
		if err != nil {
			return fmt.Errorf("cleanup TUN resources: %w", err)
		}
		m.applied = false
		m.tunManaged = false
		m.tunPendingForward = false
		m.tunPendingRoutes = nil
		return nil
	}
	if !m.tproxyPending {
		return nil
	}
	m.applied = false
	log.Info().Str("backend", m.kind.String()).Msg("cleaning up netfilter rules")
	if err := m.backend.Cleanup(); err != nil {
		return fmt.Errorf("cleanup netfilter rules: %w", err)
	}
	m.tproxyPending = false
	return nil
}

func (m *Manager) applyTUN() error {
	m.applied = false
	if m.cfg.EnableIPv6 {
		return fmt.Errorf("TUN IPv6 takeover is unsupported: only IPv4 fw4 forward_lan is implemented; refusing to claim dual-stack support")
	}
	if m.kind != BackendNftables {
		return fmt.Errorf("TUN LAN takeover requires nftables/fw4 forward_lan; backend %s is unsupported", m.kind.String())
	}
	if m.tunPendingForward || len(m.tunPendingRoutes) != 0 {
		return fmt.Errorf("TUN rollback remains unfinished; retry Cleanup before Apply")
	}
	device, err := tunDevice(m.cfg.TunDevice)
	if err != nil {
		return err
	}
	if err := checkTunRoute(device); err != nil {
		return err
	}
	added, err := ensureTunForwardAccept(device)
	m.tunPendingForward = added
	if err == nil {
		m.tunPendingRoutes, err = ensureTunRouteRules()
	}
	if err != nil {
		return errors.Join(fmt.Errorf("apply TUN supplemental rules: %w", err), m.rollbackTUN())
	}
	m.tunPendingForward = false
	m.tunPendingRoutes = nil
	m.tunManaged = true
	m.applied = true
	log.Warn().Str("device", device).Msg("TUN 补充规则已验证：仅 IPv4/fw4 forward_lan；不包含 IPv6 防绕行、guest/自定义 zone、fw4 reload 持久化或端到端流量保证")
	return nil
}

func (m *Manager) rollbackTUN() error {
	var errs []error
	if err := removeTunRouteRules(m.tunPendingRoutes); err != nil {
		errs = append(errs, fmt.Errorf("rollback newly added TUN routes: %w", err))
	} else {
		m.tunPendingRoutes = nil
	}
	if m.tunPendingForward {
		if err := RemoveTunForwardAccept(m.cfg.TunDevice); err != nil {
			errs = append(errs, fmt.Errorf("rollback newly added TUN forward rule: %w", err))
		} else {
			m.tunPendingForward = false
		}
	}
	return errors.Join(errs...)
}

// Backend returns the detected backend name.
func (m *Manager) BackendName() string {
	return m.kind.String()
}

// IsApplied reports that the last Apply verified its managed rules. For TUN it
// is limited to IPv4/fw4 forward_lan; it is not a live data-plane health probe.
// A false result does not mean cleanup ownership is empty after an error.
func (m *Manager) IsApplied() bool {
	return m.applied
}

type noopBackend struct{}

func (n *noopBackend) Apply() error   { return nil }
func (n *noopBackend) Cleanup() error { return nil }
