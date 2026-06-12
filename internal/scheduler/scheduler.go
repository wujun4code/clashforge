package scheduler

import (
	"time"

	"github.com/rs/zerolog/log"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/geodata"
	"github.com/wujun4code/clashforge/internal/selfupdate"
	"github.com/wujun4code/clashforge/internal/subscription"
)

// Scheduler manages periodic tasks.
type Scheduler struct {
	cfg         *config.MetaclashConfig
	subManager  *subscription.Manager
	geoManager  *geodata.Manager
	selfUpdater *selfupdate.Updater
	stopCh      chan struct{}
}

// New creates a Scheduler.
func New(cfg *config.MetaclashConfig, subManager *subscription.Manager, geoManager *geodata.Manager, selfUpdater *selfupdate.Updater) *Scheduler {
	return &Scheduler{
		cfg:         cfg,
		subManager:  subManager,
		geoManager:  geoManager,
		selfUpdater: selfUpdater,
		stopCh:      make(chan struct{}),
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

	if (s.cfg.Update.AutoGeoIP || s.cfg.Update.AutoGeosite) && s.geoManager != nil {
		interval := parseDuration(s.cfg.Update.GeoIPInterval, 168*time.Hour)
		go s.loop("geodata-update", interval, func() {
			proxyServer := s.cfg.Update.GeoDataProxyServer
			log.Info().Str("proxy_server", proxyServer).Msg("scheduler: triggering geodata update")
			rec := s.geoManager.TriggerSync(proxyServer)
			if rec != nil {
				log.Info().Str("status", rec.Status).Str("id", rec.ID).Msg("scheduler: geodata update finished")
			}
		})
	}

	if s.cfg.Update.AutoSelfUpdate && s.selfUpdater != nil {
		hour, minute := selfupdate.ParseUpdateTime(s.cfg.Update.SelfUpdateTime)
		go s.loopAtTime("self-update", hour, minute, func() {
			log.Info().Int("hour", hour).Int("minute", minute).Msg("scheduler: triggering self-update")
			s.selfUpdater.Run()
		})
	}
}

// Stop signals all loops to exit.
func (s *Scheduler) Stop() {
	close(s.stopCh)
}

// loop fires fn every interval on a ticker.
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

// loopAtTime fires fn once per day at the given local-time hour:minute.
// On the first run it waits until the next occurrence of that time; then
// it waits exactly 24 h for each subsequent run.
func (s *Scheduler) loopAtTime(name string, hour, minute int, fn func()) {
	first := selfupdate.NextFireDuration(hour, minute)
	log.Info().Str("task", name).
		Str("scheduled_time", formatHHMM(hour, minute)).
		Dur("first_fire_in", first.Truncate(time.Second)).
		Msg("scheduler task started (daily)")

	timer := time.NewTimer(first)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			fn()
			// After each firing reset to exactly 24 h (handles DST shifts
			// by re-anchoring from real-now rather than adding 24 h to the
			// theoretical fire time).
			next := selfupdate.NextFireDuration(hour, minute)
			timer.Reset(next)
		case <-s.stopCh:
			log.Info().Str("task", name).Msg("scheduler task stopped")
			return
		}
	}
}

func formatHHMM(h, m int) string {
	return time.Date(0, 1, 1, h, m, 0, 0, time.UTC).Format("15:04")
}

func parseDuration(s string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}
