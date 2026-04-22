package scheduler

import (
	"time"

	"github.com/rs/zerolog/log"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
)

// Scheduler manages periodic tasks.
type Scheduler struct {
	cfg        *config.MetaclashConfig
	subManager *subscription.Manager
	stopCh     chan struct{}
}

// New creates a Scheduler.
func New(cfg *config.MetaclashConfig, subManager *subscription.Manager) *Scheduler {
	return &Scheduler{
		cfg:        cfg,
		subManager: subManager,
		stopCh:     make(chan struct{}),
	}
}

// Start launches background goroutines for each scheduled task.
func (s *Scheduler) Start() {
	if s.cfg.Update.AutoSubscription {
		interval := parseDuration(s.cfg.Update.SubscriptionInterval, 6*time.Hour)
		go s.loop("subscription-update", interval, func() {
			log.Info().Msg("scheduler: triggering subscription update")
			_ = s.subManager.TriggerUpdateAll()
		})
	}
	if s.cfg.Update.AutoGeoIP {
		interval := parseDuration(s.cfg.Update.GeoIPInterval, 168*time.Hour)
		go s.loop("geoip-update", interval, func() {
			log.Info().Str("url", s.cfg.Update.GeoIPURL).Msg("scheduler: geoip update scheduled (not yet implemented)")
		})
	}
	if s.cfg.Update.AutoGeosite {
		interval := parseDuration(s.cfg.Update.GeositeInterval, 168*time.Hour)
		go s.loop("geosite-update", interval, func() {
			log.Info().Str("url", s.cfg.Update.GeositeURL).Msg("scheduler: geosite update scheduled (not yet implemented)")
		})
	}
}

// Stop signals all loops to exit.
func (s *Scheduler) Stop() {
	close(s.stopCh)
}

func (s *Scheduler) loop(name string, interval time.Duration, fn func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	log.Info().Str("task", name).Dur("interval", interval).Msg("scheduler task started")
	for {
		select {
		case <-ticker.C:
			fn()
		case <-s.stopCh:
			log.Info().Str("task", name).Msg("scheduler task stopped")
			return
		}
	}
}

func parseDuration(s string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}
