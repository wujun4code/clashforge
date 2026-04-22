package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).Hook(api.NewZerologHook(logBuf))

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
	nfManager := netfilter.NewManager(netfilter.Config{
		Mode:            cfg.Network.Mode,
		FirewallBackend: cfg.Network.FirewallBackend,
		TProxyPort:      cfg.Ports.TProxy,
		DNSPort:         cfg.Ports.DNS,
		BypassCIDR:      cfg.Network.BypassCIDR,
	})
	coreStarted := false
	if err := coreManager.Start(context.Background()); err != nil {
		log.Error().Err(err).Msg("start mihomo failed")
	} else {
		coreStarted = true
	}
	if coreStarted && cfg.Network.ApplyOnStart && cfg.Network.Mode != "none" {
		if err := nfManager.Apply(); err != nil {
			log.Warn().Err(err).Msg("apply netfilter rules (continuing without)")
		}
	} else if cfg.Network.Mode != "none" {
		log.Info().Str("mode", cfg.Network.Mode).Msg("transparent proxy takeover disabled on startup")
	}

	// DNS / dnsmasq coexistence
	dnsMode := dns.DnsmasqMode(cfg.DNS.DnsmasqMode)
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
	if dnsManaged {
		if err := dns.Restore(dnsMode); err != nil {
			log.Error().Err(err).Msg("dns restore failed")
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

func writeRuntimeMihomoConfig(cfg *config.MetaclashConfig, subManager *subscription.Manager) error {
	nodes := []subscription.ProxyNode{}
	if subManager != nil {
		nodes = subManager.GetAllCachedNodes()
	}

	generated, err := config.Generate(cfg, nodes)
	if err != nil {
		return err
	}

	overridesPath := filepath.Join(cfg.Core.DataDir, "overrides.yaml")
	overridesData, _ := os.ReadFile(overridesPath)
	merged, err := config.MergeWithOverrides(generated, overridesData)
	if err != nil {
		return err
	}

	merged = config.ApplyManagedRuntimeSettings(cfg, merged)

	data, err := config.MarshalYAML(merged)
	if err != nil {
		return err
	}

	outPath := filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml")
	if err := os.MkdirAll(cfg.Core.RuntimeDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, data, 0o644)
}
