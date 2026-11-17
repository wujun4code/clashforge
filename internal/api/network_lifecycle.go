package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/dns"
)

// startConfiguredCore is used only AFTER config generation and preparation.
// Restore DNS while the old core/rules are still usable, then retire the old
// manager before restarting. A config-save must never retire a live owner.
func startConfiguredCore(ctx context.Context, deps Dependencies) (string, error) {
	if err := config.ValidateDNSCoexistence(deps.Config); err != nil {
		return "", err
	}
	if deps.Core.Status().Ready {
		for _, mode := range []dns.DnsmasqMode{dns.ModeReplace, dns.ModeUpstream} {
			if err := dns.Restore(mode); err != nil {
				return "", fmt.Errorf("切换前恢复 DNS 失败: %w", err)
			}
		}
	}
	if deps.Netfilter != nil {
		if err := refreshNetfilterManager(deps); err != nil {
			return "", err
		}
	} else if deps.Config.Network.ApplyOnStart && deps.Config.Network.Mode != "none" {
		return "", fmt.Errorf("netfilter manager is not initialized")
	}
	if err := deps.Core.Start(ctx); err != nil {
		if !errors.Is(err, core.ErrAlreadyRunning) {
			return "", err
		}
		if err := deps.Core.Restart(ctx); err != nil {
			return "", err
		}
		return "重启", nil
	}
	return "启动", nil
}

// applyNetworkTakeover never installs DNS changes after a failed firewall apply.
func applyNetworkTakeover(deps Dependencies) error {
	if deps.Netfilter == nil {
		return fmt.Errorf("netfilter manager is not initialized")
	}
	if err := refreshNetfilterManager(deps); err != nil {
		return err
	}
	if !deps.Config.Network.ApplyOnStart || deps.Config.Network.Mode == "none" {
		return nil
	}
	return deps.Netfilter.Apply()
}
