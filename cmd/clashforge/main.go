package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/wujun4code/clashforge/internal/api"
	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/daemon"
)

const version = "0.1.0-dev"

func main() {
	cfgPath := flag.String("config", "/etc/metaclash/config.toml", "path to config file")
	flag.Parse()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatal().Err(err).Msg("load config")
	}

	level, err := zerolog.ParseLevel(cfg.Log.Level)
	if err == nil {
		zerolog.SetGlobalLevel(level)
	}

	if err := os.MkdirAll(cfg.Core.RuntimeDir, 0o755); err != nil {
		log.Fatal().Err(err).Msg("create runtime dir")
	}
	if err := os.MkdirAll(cfg.Core.DataDir, 0o755); err != nil {
		log.Fatal().Err(err).Msg("create data dir")
	}

	pidfile, err := daemon.AcquirePIDFile(cfg.Core.RuntimeDir + "/metaclash.pid")
	if err != nil {
		log.Fatal().Err(err).Msg("acquire pidfile")
	}
	defer pidfile.Close()

	coreManager := core.NewManager(core.CoreManagerConfig{
		Binary:      cfg.Core.Binary,
		ConfigFile:  cfg.Core.RuntimeDir + "/mihomo-config.yaml",
		APIPort:     cfg.Ports.MihomoAPI,
		MaxRestarts: cfg.Core.MaxRestarts,
	})

	router := api.NewRouter(api.Dependencies{
		Version:    version,
		StartedAt:  time.Now(),
		ConfigPath: *cfgPath,
		Config:     cfg,
		Core:       coreManager,
	})

	addr := cfg.UIListenAddr()
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info().Str("addr", addr).Msg("http server listening")
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("http server failed")
		}
	}()

	sig := daemon.Wait()
	log.Info().Str("signal", sig.String()).Msg("shutdown requested")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := coreManager.Stop(); err != nil && !errors.Is(err, core.ErrNotRunning) {
		log.Error().Err(err).Msg("stop core")
	}
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("http shutdown")
	}
}
