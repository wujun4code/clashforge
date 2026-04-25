package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/wujun4code/clashforge/internal/api"
	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/daemon"
	"github.com/wujun4code/clashforge/internal/dns"
	"github.com/wujun4code/clashforge/internal/netfilter"
	"github.com/wujun4code/clashforge/internal/scheduler"
	"github.com/wujun4code/clashforge/internal/subscription"
)

const version = "0.1.0-dev"

var buildVersion = version // overridden by ldflags: -X main.buildVersion=v1.0.0

func main() {
	cfgPath := flag.String("config", "/etc/metaclash/config.toml", "path to config file")
	showVersion := flag.Bool("v", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(buildVersion)
		os.Exit(0)
	}

	logBuf := api.NewLogBuffer(500)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	log.Logger = zerolog.New(zerolog.MultiLevelWriter(consoleWriter, logBuf)).With().Timestamp().Logger()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatal().Err(err).Msg("load config")
	}

	if level, err := zerolog.ParseLevel(cfg.Log.Level); err == nil {
		zerolog.SetGlobalLevel(level)
	}

	for _, adjustment := range config.SelectCompatiblePorts(cfg, config.PortSelectionOptions{PreferCommunityDefaults: true}) {
		log.Info().Str("port", adjustment.Name).Int("from", adjustment.From).Int("to", adjustment.To).Msg("adjusting port profile for compatibility")
	}

	for _, dir := range []string{cfg.Core.RuntimeDir, cfg.Core.DataDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal().Err(err).Str("dir", dir).Msg("create directory")
		}
	}

	pidfile, err := daemon.AcquirePIDFile(cfg.Core.RuntimeDir + "/metaclash.pid")
	if err != nil {
		log.Fatal().Err(err).Msg("acquire pidfile")
	}
	defer pidfile.Close()

	// Core (mihomo process manager)
	coreManager := core.NewManager(core.CoreManagerConfig{
		Binary:      cfg.Core.Binary,
		ConfigFile:  cfg.Core.RuntimeDir + "/mihomo-config.yaml",
		HomeDir:     cfg.Core.DataDir, // persistent dir so GeoIP.dat survives reboots
		APIPort:     cfg.Ports.MihomoAPI,
		MaxRestarts: cfg.Core.MaxRestarts,
	})

	// Subscriptions
	subManager := subscription.NewManager(cfg.Core.DataDir)
	if err := subManager.Load(); err != nil {
		log.Warn().Err(err).Msg("load subscriptions")
	}
	if err := writeRuntimeMihomoConfig(cfg, subManager); err != nil {
		log.Error().Err(err).Msg("generate mihomo config")
	}

	// Netfilter
	dnsMode := dns.DnsmasqMode(cfg.DNS.DnsmasqMode)
	nfManager := netfilter.NewManager(netfilter.Config{
		Mode:              cfg.Network.Mode,
		FirewallBackend:   cfg.Network.FirewallBackend,
		TProxyPort:        cfg.Ports.TProxy,
		DNSPort:           cfg.Ports.DNS,
		EnableDNSRedirect: shouldRedirectDNSOnStartup(cfg, dnsMode),
		BypassFakeIP:      shouldBypassFakeIPOnStartup(cfg),
		BypassCIDR:        cfg.Network.BypassCIDR,
	})

	// Crash watchdog: when mihomo exits unexpectedly (e.g. OOM kill), immediately
	// release DNS/nft so clients can still resolve DNS via plain dnsmasq.
	// If auto-restart succeeds, re-apply the rules automatically.
	coreManager.SetCrashCallback(func() {
		log.Warn().Msg("mihomo crashed, releasing DNS/nft to prevent DNS blackout")
		for _, mode := range []dns.DnsmasqMode{dns.ModeReplace, dns.ModeUpstream} {
			_ = dns.Restore(mode)
		}
		_ = (&netfilter.NftablesBackend{}).Cleanup()
		_ = (&netfilter.IptablesBackend{DNSPort: cfg.Ports.DNS}).Cleanup()
	})
	coreManager.SetRestartSuccessCallback(func() {
		currentDNSMode := dns.DnsmasqMode(cfg.DNS.DnsmasqMode)
		if !cfg.DNS.Enable || !cfg.DNS.ApplyOnStart || currentDNSMode == dns.ModeNone {
			return
		}
		log.Info().Str("dnsmasq_mode", string(currentDNSMode)).Msg("mihomo restarted, re-applying DNS/nft rules")
		if err := dns.Setup(currentDNSMode, cfg.Ports.DNS); err != nil {
			log.Warn().Err(err).Msg("dns re-apply after restart failed")
		}
		if cfg.Network.ApplyOnStart && cfg.Network.Mode != "none" {
			if err := nfManager.Apply(); err != nil {
				log.Warn().Err(err).Msg("netfilter re-apply after restart failed")
			}
		}
	})

	coreStarted := false
	if cfg.Core.AutoStartCore {
		if err := coreManager.Start(context.Background()); err != nil {
			log.Error().Err(err).Msg("auto-start mihomo failed")
		} else {
			coreStarted = true
		}
	} else {
		log.Info().Msg("mihomo auto-start disabled (auto_start_core = false); start via Setup Wizard or API")
	}
	if coreStarted && cfg.Network.ApplyOnStart && cfg.Network.Mode != "none" {
		if err := nfManager.Apply(); err != nil {
			log.Warn().Err(err).Msg("apply netfilter rules (continuing without)")
		}
	} else if cfg.Network.Mode != "none" {
		log.Info().Str("mode", cfg.Network.Mode).Msg("transparent proxy takeover disabled on startup")
	}

	// DNS / dnsmasq coexistence
	dnsManaged := false
	if coreStarted && cfg.DNS.Enable && cfg.DNS.ApplyOnStart && dnsMode != dns.ModeNone {
		if err := dns.Setup(dnsMode, cfg.Ports.DNS); err != nil {
			log.Warn().Err(err).Msg("dns setup failed (continuing)")
		} else {
			dnsManaged = true
		}
	} else if cfg.DNS.Enable && dnsMode != dns.ModeNone {
		log.Info().Str("dnsmasq_mode", cfg.DNS.DnsmasqMode).Msg("dns takeover disabled on startup")
	}

	logStartupHealth(*cfgPath, cfg, coreManager, nfManager, dnsManaged, coreStarted, dnsMode)

	// SSE broker
	sseBroker := api.NewSSEBroker()

	// Scheduler
	sched := scheduler.New(cfg, subManager)
	sched.Start()

	// HTTP server
	router := api.NewRouter(api.Dependencies{
		Version:    buildVersion,
		StartedAt:  time.Now(),
		ConfigPath: *cfgPath,
		Config:     cfg,
		Core:       coreManager,
		SubManager: subManager,
		Netfilter:  nfManager,
		SSEBroker:  sseBroker,
		LogBuffer:  logBuf,
	})

	addr := cfg.UIListenAddr()
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info().Str("addr", addr).Str("version", version).Str("firewall", nfManager.BackendName()).Msg("clashforge started")
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("http server failed")
		}
	}()

	// Wait for signal
	sig := daemon.Wait()
	log.Info().Str("signal", sig.String()).Msg("shutdown requested")

	sched.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := coreManager.Stop(); err != nil && !errors.Is(err, core.ErrNotRunning) {
		log.Error().Err(err).Msg("stop core")
	}
	if dnsManaged || (cfg.DNS.Enable && cfg.DNS.ApplyOnStart) {
		currentDNSMode := dns.DnsmasqMode(cfg.DNS.DnsmasqMode)
		if currentDNSMode != dns.ModeNone {
			if err := dns.Restore(currentDNSMode); err != nil {
				log.Error().Err(err).Msg("dns restore failed")
			}
		}
	}
	if err := nfManager.Cleanup(); err != nil {
		log.Error().Err(err).Msg("cleanup netfilter")
	}
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("http shutdown")
	}
	log.Info().Msg("clashforge exited cleanly")
}

func shouldRedirectDNSOnStartup(cfg *config.MetaclashConfig, dnsMode dns.DnsmasqMode) bool {
	if cfg == nil {
		return false
	}
	if !cfg.DNS.Enable || !cfg.DNS.ApplyOnStart {
		return false
	}
	// Only redirect port-53 via nftables/iptables in replace mode.
	// In upstream mode dnsmasq stays on port 53 and forwards to mihomo itself.
	return dnsMode == dns.ModeReplace
}

func shouldBypassFakeIPOnStartup(cfg *config.MetaclashConfig) bool {
	if cfg == nil {
		return true
	}
	if !cfg.DNS.Enable || !cfg.DNS.ApplyOnStart {
		return true
	}
	return strings.ToLower(strings.TrimSpace(cfg.DNS.Mode)) != "fake-ip"
}

func logStartupHealth(
	cfgPath string,
	cfg *config.MetaclashConfig,
	coreManager *core.CoreManager,
	nfManager *netfilter.Manager,
	dnsManaged, coreStarted bool,
	dnsMode dns.DnsmasqMode,
) {
	const side = "system"

	// Phase 1: configuration loaded
	log.Info().Str("side", side).Str("phase", "config").Str("status", "loaded").
		Str("config_file", cfgPath).Str("binary", cfg.Core.Binary).
		Str("data_dir", cfg.Core.DataDir).Str("runtime_dir", cfg.Core.RuntimeDir).
		Msg("startup_health")

	// Phase 2: proxy core (mihomo)
	if coreStarted {
		st := coreManager.Status()
		log.Info().Str("side", side).Str("phase", "proxy_core").Str("status", "started").
			Int("pid", st.PID).Str("binary", cfg.Core.Binary).
			Int("api_port", cfg.Ports.MihomoAPI).
			Msg("startup_health")
	} else {
		log.Info().Str("side", side).Str("phase", "proxy_core").Str("status", "skipped").
			Bool("auto_start_core", cfg.Core.AutoStartCore).
			Msg("startup_health")
	}

	// Phase 3: transparent proxy + firewall
	switch {
	case cfg.Network.Mode == "none":
		log.Info().Str("side", side).Str("phase", "transparent_proxy").Str("status", "disabled").
			Str("mode", "none").Msg("startup_health")
	case !coreStarted:
		log.Info().Str("side", side).Str("phase", "transparent_proxy").Str("status", "skipped").
			Str("reason", "core_not_started").Msg("startup_health")
	case !cfg.Network.ApplyOnStart:
		log.Info().Str("side", side).Str("phase", "transparent_proxy").Str("status", "skipped").
			Str("reason", "apply_on_start=false").Msg("startup_health")
	default:
		statusStr := "applied"
		if !nfManager.IsApplied() {
			statusStr = "failed"
		}
		log.Info().Str("side", side).Str("phase", "transparent_proxy").Str("status", statusStr).
			Str("mode", cfg.Network.Mode).Str("firewall_backend", nfManager.BackendName()).
			Int("tproxy_port", cfg.Ports.TProxy).Bool("bypass_lan", cfg.Network.BypassLAN).
			Msg("startup_health")
	}

	// Phase 4: DNS redirect (dnsmasq coordination)
	switch {
	case !cfg.DNS.Enable:
		log.Info().Str("side", side).Str("phase", "dns_redirect").Str("status", "disabled").
			Str("reason", "dns.enable=false").Msg("startup_health")
	case dnsMode == dns.ModeNone:
		log.Info().Str("side", side).Str("phase", "dns_redirect").Str("status", "disabled").
			Str("dnsmasq_mode", "none").Msg("startup_health")
	case !coreStarted:
		log.Info().Str("side", side).Str("phase", "dns_redirect").Str("status", "skipped").
			Str("reason", "core_not_started").Msg("startup_health")
	case !cfg.DNS.ApplyOnStart:
		log.Info().Str("side", side).Str("phase", "dns_redirect").Str("status", "skipped").
			Str("reason", "apply_on_start=false").Msg("startup_health")
	default:
		statusStr := "applied"
		if !dnsManaged {
			statusStr = "failed"
		}
		log.Info().Str("side", side).Str("phase", "dns_redirect").Str("status", statusStr).
			Str("dnsmasq_mode", cfg.DNS.DnsmasqMode).Int("dns_port", cfg.Ports.DNS).
			Msg("startup_health")
	}

	// Phase 5: DNS resolution engine (mihomo DNS)
	if !coreStarted {
		log.Info().Str("side", side).Str("phase", "dns_engine").Str("status", "skipped").
			Str("reason", "core_not_started").Msg("startup_health")
	} else if cfg.DNS.Enable {
		log.Info().Str("side", side).Str("phase", "dns_engine").Str("status", "configured").
			Str("mode", cfg.DNS.Mode).Int("port", cfg.Ports.DNS).
			Msg("startup_health")
	} else {
		log.Info().Str("side", side).Str("phase", "dns_engine").Str("status", "disabled").
			Msg("startup_health")
	}

	// Phase 6: ports summary
	log.Info().Str("side", side).Str("phase", "ports").Str("status", "ok").
		Int("mixed", cfg.Ports.Mixed).Int("tproxy", cfg.Ports.TProxy).
		Int("dns", cfg.Ports.DNS).Int("mihomo_api", cfg.Ports.MihomoAPI).
		Int("ui", cfg.Ports.UI).
		Msg("startup_health")

	// Summary
	log.Info().Str("side", side).
		Bool("core_running", coreStarted).
		Bool("transparent_proxy", coreStarted && cfg.Network.ApplyOnStart &&
			cfg.Network.Mode != "none" && nfManager.IsApplied()).
		Bool("dns_redirect", dnsManaged).
		Bool("dns_engine", coreStarted && cfg.DNS.Enable).
		Msg("startup_summary")
}

func writeRuntimeMihomoConfig(cfg *config.MetaclashConfig, subManager *subscription.Manager) error {
	nodes := []subscription.ProxyNode{}
	rawYAMLs := [][]byte{}
	if subManager != nil {
		nodes = subManager.GetAllCachedNodes()
		rawYAMLs = subManager.GetRawYAMLForEnabled()
	}

	overridesPath := filepath.Join(cfg.Core.DataDir, "overrides.yaml")
	overridesData, _ := os.ReadFile(overridesPath)

	var generated map[string]interface{}
	var err error

	if len(rawYAMLs) > 0 {
		// Full subscription YAML available: preserve rules, proxy-groups, rule-providers.
		generated, err = config.GenerateFromBase(cfg, rawYAMLs[0], nodes)
		if err != nil {
			return err
		}
		if len(overridesData) > 0 {
			generated, err = config.MergeWithOverrides(generated, overridesData)
			if err != nil {
				return err
			}
		}
	} else if len(overridesData) > 0 {
		// No subscription YAML: treat overrides.yaml as the full user config.
		generated, err = config.GenerateFromBase(cfg, overridesData, nodes)
		if err != nil {
			return err
		}
	} else {
		// Fallback: generate a minimal config from available nodes.
		generated, err = config.Generate(cfg, nodes)
		if err != nil {
			return err
		}
		if len(overridesData) > 0 {
			generated, err = config.MergeWithOverrides(generated, overridesData)
			if err != nil {
				return err
			}
		}
	}

	generated = config.ApplyManagedRuntimeSettings(cfg, generated)

	data, err := config.MarshalYAML(generated)
	if err != nil {
		return err
	}

	outPath := filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml")
	if err := os.MkdirAll(cfg.Core.RuntimeDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, data, 0o644)
}
