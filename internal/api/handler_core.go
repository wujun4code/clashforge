package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

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
		if err := deps.Core.Start(ctx); err != nil {
			if errors.Is(err, core.ErrAlreadyRunning) {
				Err(w, http.StatusConflict, "CORE_ALREADY_RUNNING", err.Error())
				return
			}
			Err(w, http.StatusInternalServerError, "CORE_START_FAILED", err.Error())
			return
		}

		// Persist auto_start_core = true so the proxy stack survives upgrades.
		deps.Config.Core.AutoStartCore = true
		_ = saveRuntimeConfig(deps)

		applied, warnings := applyConfiguredStartupTakeover(deps)

		resp := map[string]any{"pid": deps.Core.Status().PID}
		if len(applied) > 0 {
			resp["takeover_applied"] = applied
		}
		if len(warnings) > 0 {
			resp["takeover_warnings"] = warnings
		}
		JSON(w, http.StatusOK, resp)
	}
}

func handleCoreStop(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Release DNS + nftables BEFORE stopping the core so there is no DNS
		// blackout window.  Restoring dnsmasq first keeps mihomo's dns_redirect
		// chain alive during dnsmasq restart; by the time we remove the metaclash
		// nft table dnsmasq is already listening on :53.
		_, _ = releaseAllTakeover(deps)

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
		if err := deps.Core.Restart(ctx); err != nil {
			Err(w, http.StatusInternalServerError, "CORE_RESTART_FAILED", err.Error())
			return
		}
		applied, warnings := applyConfiguredStartupTakeover(deps)

		resp := map[string]any{"pid": deps.Core.Status().PID}
		if len(applied) > 0 {
			resp["takeover_applied"] = applied
		}
		if len(warnings) > 0 {
			resp["takeover_warnings"] = warnings
		}
		JSON(w, http.StatusOK, resp)
	}
}

func applyConfiguredStartupTakeover(deps Dependencies) ([]string, []string) {
	applied := []string{}
	warnings := []string{}

	if deps.Config == nil {
		return applied, warnings
	}

	if deps.Config.DNS.Enable && deps.Config.DNS.ApplyOnStart {
		mode := dns.DnsmasqMode(deps.Config.DNS.DnsmasqMode)
		if mode != dns.ModeNone {
			log.Info().
				Str("mode", string(mode)).
				Int("mihomo_dns_port", deps.Config.Ports.DNS).
				Msg("core/start: applying configured DNS takeover")
			if err := dns.Setup(mode, deps.Config.Ports.DNS); err != nil {
				msg := fmt.Sprintf("DNS 入口接管失败: %v", err)
				log.Warn().Err(err).Str("mode", string(mode)).Msg("core/start: DNS takeover failed")
				warnings = append(warnings, msg)
			} else {
				applied = append(applied, "dns_entry")
			}
		}
	}

	if deps.Config.Network.ApplyOnStart && deps.Config.Network.Mode != "none" {
		log.Info().
			Str("mode", deps.Config.Network.Mode).
			Str("backend", deps.Config.Network.FirewallBackend).
			Msg("core/start: applying configured transparent-proxy takeover")

		refreshNetfilterManager(deps)
		if deps.Netfilter == nil {
			msg := "透明代理接管失败: netfilter manager is not initialized"
			log.Warn().Msg("core/start: netfilter manager missing")
			warnings = append(warnings, msg)
		} else if err := deps.Netfilter.Apply(); err != nil {
			msg := fmt.Sprintf("透明代理 / 防火墙规则接管失败: %v", err)
			log.Warn().Err(err).Msg("core/start: transparent-proxy takeover failed")
			warnings = append(warnings, msg)
		} else if deps.Config.Network.Mode != "tun" {
			applied = append(applied, "transparent_proxy", "nft_firewall")
		}
	}

	return applied, warnings
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
