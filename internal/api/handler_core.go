package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/dns"
)

func handleCoreStart(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := generateMihomoConfig(deps); err != nil {
			Err(w, http.StatusInternalServerError, "CORE_CONFIG_GENERATE_FAILED", err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if _, err := startConfiguredCore(ctx, deps); err != nil {
			Err(w, http.StatusInternalServerError, "CORE_START_FAILED", err.Error())
			return
		}

		// Persist auto_start_core = true so the proxy stack survives upgrades.
		deps.Config.Core.AutoStartCore = true
		_ = saveRuntimeConfig(deps)

		applied, warnings := applyConfiguredStartupTakeover(deps)
		if len(warnings) > 0 {
			Err(w, http.StatusInternalServerError, "TAKEOVER_FAILED", strings.Join(warnings, "; "))
			return
		}

		resp := map[string]any{"pid": deps.Core.Status().PID}
		if len(applied) > 0 {
			resp["takeover_applied"] = applied
		}
		JSON(w, http.StatusOK, resp)
	}
}

func handleCoreStop(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Restore DNS and release owned rules before stopping the core. This
		// is availability-first ordering, not an end-to-end no-blackout proof.
		if _, err := releaseAllTakeover(deps); err != nil {
			Err(w, http.StatusInternalServerError, "TAKEOVER_CLEANUP_FAILED", err.Error())
			return
		}

		if err := deps.Core.Stop(); err != nil && !errors.Is(err, core.ErrNotRunning) {
			Err(w, http.StatusInternalServerError, "CORE_STOP_FAILED", err.Error())
			return
		}

		// Persist auto_start_core = false so the proxy stack stays stopped across upgrades.
		deps.Config.Core.AutoStartCore = false
		_ = saveRuntimeConfig(deps)

		JSON(w, http.StatusOK, map[string]any{"stopped": true})
	}
}

func handleCoreRestart(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := generateMihomoConfig(deps); err != nil {
			Err(w, http.StatusInternalServerError, "CORE_CONFIG_GENERATE_FAILED", err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if _, err := startConfiguredCore(ctx, deps); err != nil {
			Err(w, http.StatusInternalServerError, "CORE_RESTART_FAILED", err.Error())
			return
		}
		applied, warnings := applyConfiguredStartupTakeover(deps)
		if len(warnings) > 0 {
			Err(w, http.StatusInternalServerError, "TAKEOVER_FAILED", strings.Join(warnings, "; "))
			return
		}

		resp := map[string]any{"pid": deps.Core.Status().PID}
		if len(applied) > 0 {
			resp["takeover_applied"] = applied
		}
		JSON(w, http.StatusOK, resp)
	}
}

func applyConfiguredStartupTakeover(deps Dependencies) ([]string, []string) {
	applied := []string{}
	if deps.Config == nil {
		return applied, nil
	}
	if err := config.ValidateDNSCoexistence(deps.Config); err != nil {
		return applied, []string{err.Error()}
	}
	// Firewall first: never disable dnsmasq until the replacement :53 path is
	// installed, and never mutate DNS after failed TUN/transparent takeover.
	if deps.Netfilter != nil || (deps.Config.Network.ApplyOnStart && deps.Config.Network.Mode != "none") {
		if err := applyNetworkTakeover(deps); err != nil {
			return applied, []string{fmt.Sprintf("透明代理 / 防火墙规则接管失败: %v", err)}
		}
		if deps.Config.Network.ApplyOnStart && deps.Config.Network.Mode != "none" {
			applied = append(applied, "transparent_proxy", "nft_firewall")
		}
	}
	if deps.Config.DNS.Enable && deps.Config.DNS.ApplyOnStart && deps.Config.DNS.DnsmasqMode != "none" {
		if err := dns.Setup(dns.DnsmasqMode(deps.Config.DNS.DnsmasqMode), deps.Config.Ports.DNS); err != nil {
			return applied, []string{fmt.Sprintf("DNS 入口接管失败: %v", err)}
		}
		applied = append(applied, "dns_entry")
	}
	return applied, nil
}

func handleCoreReload(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := deps.Core.Reload(deps.Core.Status().ConfigFile); err != nil {
			Err(w, http.StatusBadRequest, "CONFIG_INVALID", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]any{"reloaded": true})
	}
}

func handleServiceEnable(_ Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := exec.Command("/etc/init.d/clashforge", "enable").CombinedOutput()
		if err != nil {
			Err(w, http.StatusInternalServerError, "SERVICE_ENABLE_FAILED", strings.TrimSpace(string(out)))
			return
		}
		JSON(w, http.StatusOK, map[string]bool{"enabled": true})
	}
}

func handleCoreVersion(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		JSON(w, http.StatusOK, map[string]any{
			"current":      deps.Core.CurrentVersion(ctx),
			"latest":       "",
			"has_update":   false,
			"download_url": "",
		})
	}
}
