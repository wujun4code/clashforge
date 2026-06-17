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
	EnableIPv6        bool   // intercept IPv6 traffic via tproxy as well
	DropQUIC          bool   // drop UDP 443 to force immediate TCP fallback
	WANInterface      string // WAN-facing interface name (e.g. "pppoe-wan", "eth1")
	TunDevice         string // TUN interface name when Mode == "tun" (e.g. "Meta"); empty uses mihomo's default
}

// NewManager creates a Manager and detects the appropriate backend.
func NewManager(cfg Config) *Manager {
	kind := Detect(cfg.FirewallBackend)
	var applier Applier
	switch kind {
	case BackendNftables:
		applier = &NftablesBackend{TProxyPort: cfg.TProxyPort, DNSPort: cfg.DNSPort, EnableDNSRedirect: cfg.EnableDNSRedirect, BypassFakeIP: cfg.BypassFakeIP, BypassCIDR: cfg.BypassCIDR, EnableIPv6: cfg.EnableIPv6, DropQUIC: cfg.DropQUIC, WANInterface: cfg.WANInterface}
	case BackendIptables:
		applier = &IptablesBackend{TProxyPort: cfg.TProxyPort, DNSPort: cfg.DNSPort, EnableDNSRedirect: cfg.EnableDNSRedirect}
	default:
		applier = &noopBackend{}
	}
	return &Manager{backend: applier, kind: kind, cfg: cfg}
}

// Apply applies firewall rules for tproxy/redir modes.
// TUN mode is handled mostly by mihomo (TUN device + auto-route installs the
// kernel routes), but the OS firewall's forward chain still needs an explicit
// accept rule for the TUN device: stock OpenWrt fw4/iptables zone rules only
// allowlist known zone devices (lan/wan), so without this, LAN-client traffic
// that mihomo correctly routes into the TUN device gets dropped/rejected by
// the firewall before mihomo's TUN reader ever sees it.
// It still counts as "applied" so health checks (and auto-repair logic that
// gates on IsApplied()) don't mistake a working TUN setup for a missing takeover.
func (m *Manager) Apply() error {
	if m.cfg.Mode == "tun" {
		if m.kind == BackendNftables {
			if err := EnsureTunForwardAccept(m.cfg.TunDevice); err != nil {
				log.Warn().Err(err).Msg("netfilter: 无法为 TUN 设备添加 forward 放行规则，LAN 客户端可能无法通过 TUN 上网")
			}
		}
		if err := EnsureTunRouteRule(); err != nil {
			log.Warn().Err(err).Msg("netfilter: 无法补充 LAN 转发流量进入 TUN 路由表的 ip rule，LAN 客户端流量可能绕过 TUN 直接出网")
		}
		log.Info().Str("mode", m.cfg.Mode).Msg("netfilter: skipping rule setup (handled by mihomo TUN)")
		m.applied = true
		return nil
	}
	if m.cfg.Mode == "none" {
		log.Info().Str("mode", m.cfg.Mode).Msg("netfilter: skipping rule setup")
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
	if m.cfg.Mode == "tun" {
		if m.kind == BackendNftables {
			if err := RemoveTunForwardAccept(m.cfg.TunDevice); err != nil {
				log.Warn().Err(err).Msg("netfilter: 移除 TUN forward 放行规则失败")
			}
		}
		if err := RemoveTunRouteRule(); err != nil {
			log.Warn().Err(err).Msg("netfilter: 移除 TUN ip rule 失败")
		}
		m.applied = false
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
