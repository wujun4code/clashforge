package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/dns"
	"github.com/wujun4code/clashforge/internal/netfilter"
	"github.com/wujun4code/clashforge/internal/subscription"
)

// installStartupWatchdog shares a lock with daemon startup/shutdown; callbacks
// cannot interleave ownership transitions with those daemon entry points.
func installStartupWatchdog(cfg *config.MetaclashConfig, subManager *subscription.Manager, coreManager *core.CoreManager, nfManager *netfilter.Manager) *sync.Mutex {
	// Serialize crash/recovery callbacks. A delayed crash callback must not tear
	// down takeover after a newer process has already recovered.
	recoveryMu := &sync.Mutex{}
	coreManager.SetCrashCallback(func() {
		recoveryMu.Lock()
		defer recoveryMu.Unlock()
		if coreManager.Status().Ready {
			return
		}
		if err := restoreDNSAfterCrash(cfg, dns.Restore); err != nil {
			log.Error().Err(err).Msg("crash DNS restore failed; retaining netfilter ownership for retry")
			return
		}
		if retainDNSAfterCrash(cfg) {
			// replace mode depends on the network DNS redirect. Retain the same
			// owner, including partial resources, for retry or explicit stop.
			return
		}
		if err := nfManager.Cleanup(); err != nil {
			log.Error().Err(err).Msg("crash netfilter cleanup failed; retaining owner for retry")
		}
	})
	coreManager.SetRestartSuccessCallback(func() {
		recoveryMu.Lock()
		defer recoveryMu.Unlock()
		if !coreManager.Status().Ready {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if _, err := recoverStartupTakeover(ctx, cfg, subManager, coreManager, nfManager); err != nil {
			log.Error().Err(err).Msg("watchdog network recovery failed; takeover is not ready")
			if stopErr := coreManager.Stop(); stopErr != nil && !errors.Is(stopErr, core.ErrNotRunning) {
				log.Error().Err(stopErr).Msg("stop core after failed watchdog recovery")
			}
			return
		}
		log.Info().Msg("watchdog network recovery completed")
	})

	return recoveryMu
}

func startupNetfilterConfig(cfg *config.MetaclashConfig) netfilter.Config {
	return netfilter.Config{
		Mode: cfg.Network.Mode, FirewallBackend: cfg.Network.FirewallBackend,
		TProxyPort: cfg.Ports.TProxy, DNSPort: cfg.Ports.DNS,
		EnableDNSRedirect: shouldRedirectDNSOnStartup(cfg, dns.DnsmasqMode(cfg.DNS.DnsmasqMode)),
		BypassFakeIP:      shouldBypassFakeIPOnStartup(cfg), BypassCIDR: cfg.Network.BypassCIDR,
		EnableIPv6: cfg.Network.IPv6, DropQUIC: cfg.Network.DropQUIC,
		WANInterface: cfg.Network.WANInterface, TunDevice: cfg.Network.TUN.Device,
		FakeIPRange: config.ReadRuntimeFakeIPRange(cfg.Core.RuntimeDir),
	}
}

// applyStartupTakeover must succeed before any DNS listener is reconfigured.
// DNS being disabled does not disable network takeover (notably TUN recovery).
func applyStartupTakeover(cfg *config.MetaclashConfig, nf *netfilter.Manager) (bool, error) {
	if err := config.ValidateDNSCoexistence(cfg); err != nil {
		return false, err
	}
	if cfg.Network.ApplyOnStart && cfg.Network.Mode != "none" {
		if nf == nil {
			return false, fmt.Errorf("netfilter manager is not initialized")
		}
		if err := nf.Apply(); err != nil {
			return false, fmt.Errorf("apply startup netfilter: %w", err)
		}
	}
	mode := dns.DnsmasqMode(cfg.DNS.DnsmasqMode)
	if cfg.DNS.Enable && cfg.DNS.ApplyOnStart && mode != dns.ModeNone {
		if err := dns.Setup(mode, cfg.Ports.DNS); err != nil {
			return false, fmt.Errorf("setup startup DNS: %w", err)
		}
		return true, nil
	}
	return false, nil
}

// releaseStartupTakeover deliberately does not gate Cleanup on IsApplied or on
// the current DNS settings: failed operations can still own resources.
func releaseStartupTakeover(nf *netfilter.Manager) error {
	for _, mode := range []dns.DnsmasqMode{dns.ModeReplace, dns.ModeUpstream} {
		if err := dns.Restore(mode); err != nil {
			return fmt.Errorf("restore DNS before cleanup: %w", err)
		}
	}
	if nf != nil {
		if err := nf.Cleanup(); err != nil {
			return fmt.Errorf("cleanup startup takeover: %w", err)
		}
	}
	return nil
}

// recoverStartupTakeover is called after the core watchdog's process restart.
// That process may have read the old runtime. Generate and explicitly Restart
// before re-applying takeover, so a changed configuration is actually loaded.
// Non-strict mode releases DNS first. Strict mode never restores the previous
// resolver here and reuses its owner throughout generation/restart failures.
// This preserves the established crash policy, not global leak-free routing or
// atomic/no-interruption recovery.
func recoverStartupTakeover(ctx context.Context, cfg *config.MetaclashConfig, sm *subscription.Manager, cm *core.CoreManager, nf *netfilter.Manager) (bool, error) {
	if err := config.ValidateDNSCoexistence(cfg); err != nil {
		return false, err
	}
	strict := retainDNSAfterCrash(cfg)
	if !strict {
		if err := releaseStartupTakeover(nf); err != nil {
			return false, err
		}
	}
	if err := writeRuntimeMihomoConfig(cfg, sm); err != nil {
		return false, fmt.Errorf("generate watchdog runtime: %w", err)
	}
	if nf == nil && cfg.Network.ApplyOnStart && cfg.Network.Mode != "none" {
		return false, fmt.Errorf("netfilter manager is not initialized")
	}
	if nf != nil && !strict {
		*nf = *netfilter.NewManager(startupNetfilterConfig(cfg))
	}
	if err := cm.Restart(ctx); err != nil {
		return false, fmt.Errorf("restart watchdog core with generated runtime: %w", err)
	}
	// CoreManager.Ready is process state even when its API wait timed out.
	if err := waitStartupCoreHealthy(ctx, cm); err != nil {
		return false, err
	}
	return applyStartupTakeover(cfg, nf)
}

func waitStartupCoreHealthy(ctx context.Context, cm *core.CoreManager) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	for {
		status := cm.Status()
		if !status.Ready {
			return fmt.Errorf("watchdog core is not running")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/version", status.APIPort), nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("watchdog core API is not healthy: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}
