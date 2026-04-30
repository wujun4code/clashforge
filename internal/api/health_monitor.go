package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	healthStateUnknown   = "unknown"
	healthStateHealthy   = "healthy"
	healthStateDegraded  = "degraded"
	healthStateUnhealthy = "unhealthy"

	healthEventOpened   = "opened"
	healthEventResolved = "resolved"

	healthOutboxPending = "pending"
	healthOutboxSent    = "sent"

	healthStoreVersion = 1
	healthStoreFile    = "health_state.json"
)

var (
	healthDefaultRouterInterval   = 90 * time.Second
	healthDefaultDispatchInterval = 10 * time.Second
	healthDefaultBrowserTTL       = 2 * time.Minute
	healthDefaultCanaryURL        = "https://www.gstatic.com/generate_204"
)

type healthBrowserIPCheck struct {
	Provider string `json:"provider"`
	Group    string `json:"group,omitempty"`
	OK       bool   `json:"ok"`
	IP       string `json:"ip,omitempty"`
	Location string `json:"location,omitempty"`
	Error    string `json:"error,omitempty"`
}

type healthBrowserAccessCheck struct {
	Name      string `json:"name"`
	Group     string `json:"group,omitempty"`
	URL       string `json:"url,omitempty"`
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
	Stage     string `json:"stage,omitempty"`
}

type healthBrowserReportRequest struct {
	SessionID    string                     `json:"session_id"`
	CheckedAt    string                     `json:"checked_at"`
	UserAgent    string                     `json:"user_agent,omitempty"`
	IPChecks     []healthBrowserIPCheck     `json:"ip_checks"`
	AccessChecks []healthBrowserAccessCheck `json:"access_checks"`
}

type healthProbeSummary struct {
	HasData      bool     `json:"has_data"`
	Healthy      bool     `json:"healthy"`
	IPOK         bool     `json:"ip_ok"`
	FailedAccess []string `json:"failed_access,omitempty"`
	CheckedAt    string   `json:"checked_at,omitempty"`
	Stale        bool     `json:"stale,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type healthIncident struct {
	ID         string             `json:"id"`
	Status     string             `json:"status"`
	State      string             `json:"state"`
	Reason     string             `json:"reason"`
	OpenedAt   string             `json:"opened_at"`
	UpdatedAt  string             `json:"updated_at"`
	ResolvedAt string             `json:"resolved_at,omitempty"`
	Router     healthProbeSummary `json:"router"`
	Browser    healthProbeSummary `json:"browser"`
}

type healthCurrentState struct {
	State                string `json:"state"`
	Since                string `json:"since"`
	LastReason           string `json:"last_reason,omitempty"`
	ConsecutiveFailures  int    `json:"consecutive_failures"`
	ConsecutiveSuccesses int    `json:"consecutive_successes"`
	ActiveIncidentID     string `json:"active_incident_id,omitempty"`
	LastRouterCheck      string `json:"last_router_check,omitempty"`
	LastBrowserCheck     string `json:"last_browser_check,omitempty"`
}

type healthNotificationPayload struct {
	Event      string             `json:"event"`
	IncidentID string             `json:"incident_id"`
	State      string             `json:"state"`
	Reason     string             `json:"reason"`
	OpenedAt   string             `json:"opened_at,omitempty"`
	ResolvedAt string             `json:"resolved_at,omitempty"`
	Router     healthProbeSummary `json:"router"`
	Browser    healthProbeSummary `json:"browser"`
	Trigger    string             `json:"trigger,omitempty"`
}

type healthNotificationTask struct {
	ID         string                    `json:"id"`
	IncidentID string                    `json:"incident_id"`
	Event      string                    `json:"event"`
	Channel    string                    `json:"channel"`
	Status     string                    `json:"status"`
	Attempts   int                       `json:"attempts"`
	LastError  string                    `json:"last_error,omitempty"`
	CreatedAt  string                    `json:"created_at"`
	NextRetry  string                    `json:"next_retry_at"`
	SentAt     string                    `json:"sent_at,omitempty"`
	Payload    healthNotificationPayload `json:"payload"`
}

type healthMonitorState struct {
	Version        int                                   `json:"version"`
	Current        healthCurrentState                    `json:"current"`
	Router         healthProbeSummary                    `json:"router"`
	Browser        healthProbeSummary                    `json:"browser"`
	BrowserReports map[string]healthBrowserReportRequest `json:"browser_reports,omitempty"`
	Incidents      []healthIncident                      `json:"incidents,omitempty"`
	Outbox         []healthNotificationTask              `json:"outbox,omitempty"`
}

type healthSummaryResponse struct {
	CheckedAt            string             `json:"checked_at"`
	Current              healthCurrentState `json:"current"`
	Router               healthProbeSummary `json:"router"`
	Browser              healthProbeSummary `json:"browser"`
	OpenIncidents        int                `json:"open_incidents"`
	PendingNotifications int                `json:"pending_notifications"`
	WebhookConfigured    bool               `json:"webhook_configured"`
	NotificationChannel  string             `json:"notification_channel,omitempty"`
	RouterIntervalSec    int64              `json:"router_interval_sec,omitempty"`
	BrowserTTLSec        int64              `json:"browser_ttl_sec,omitempty"`
}

// HealthMonitor periodically evaluates router/browser probe health,
// stores incidents on disk, and retries outbound notifications.
type HealthMonitor struct {
	deps Dependencies

	mu    sync.RWMutex
	state healthMonitorState

	statePath         string
	routerInterval    time.Duration
	dispatchInterval  time.Duration
	browserTTL        time.Duration
	failureThreshold  int
	recoveryThreshold int
	webhookURL        string
	canaryURL         string
	notifyHTTPClient  *http.Client
	canaryHTTPClient  *http.Client

	stopCh  chan struct{}
	started bool
	wg      sync.WaitGroup

	lastOfflineNoticeAt time.Time
}

func NewHealthMonitor(deps Dependencies) *HealthMonitor {
	if deps.Config == nil {
		return nil
	}

	webhookURL := strings.TrimSpace(os.Getenv("CLASHFORGE_HEALTH_WEBHOOK_URL"))
	if webhookURL == "" {
		webhookURL = strings.TrimSpace(os.Getenv("CLASHFORGE_HEALTH_WEBHOOK"))
	}

	interval := healthDefaultRouterInterval
	if parsed, err := time.ParseDuration(strings.TrimSpace(os.Getenv("CLASHFORGE_HEALTH_ROUTER_INTERVAL"))); err == nil && parsed >= 5*time.Second {
		interval = parsed
	}
	dispatchInterval := healthDefaultDispatchInterval
	if parsed, err := time.ParseDuration(strings.TrimSpace(os.Getenv("CLASHFORGE_HEALTH_DISPATCH_INTERVAL"))); err == nil && parsed >= 3*time.Second {
		dispatchInterval = parsed
	}
	browserTTL := healthDefaultBrowserTTL
	if parsed, err := time.ParseDuration(strings.TrimSpace(os.Getenv("CLASHFORGE_HEALTH_BROWSER_TTL"))); err == nil && parsed >= 10*time.Second {
		browserTTL = parsed
	}

	failureThreshold := 2
	if v := strings.TrimSpace(os.Getenv("CLASHFORGE_HEALTH_FAILURE_THRESHOLD")); v != "" {
		if n, err := parsePositiveInt(v); err == nil {
			failureThreshold = n
		}
	}
	recoveryThreshold := 2
	if v := strings.TrimSpace(os.Getenv("CLASHFORGE_HEALTH_RECOVERY_THRESHOLD")); v != "" {
		if n, err := parsePositiveInt(v); err == nil {
			recoveryThreshold = n
		}
	}

	canaryURL := strings.TrimSpace(os.Getenv("CLASHFORGE_HEALTH_CANARY_URL"))
	if canaryURL == "" {
		canaryURL = healthDefaultCanaryURL
	}

	return &HealthMonitor{
		deps:              deps,
		statePath:         filepath.Join(deps.Config.Core.DataDir, healthStoreFile),
		routerInterval:    interval,
		dispatchInterval:  dispatchInterval,
		browserTTL:        browserTTL,
		failureThreshold:  failureThreshold,
		recoveryThreshold: recoveryThreshold,
		webhookURL:        webhookURL,
		canaryURL:         canaryURL,
		notifyHTTPClient:  &http.Client{Timeout: 8 * time.Second},
		canaryHTTPClient:  &http.Client{Timeout: 3 * time.Second},
		stopCh:            make(chan struct{}),
	}
}

func (m *HealthMonitor) Start() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true

	now := time.Now().UTC()
	if err := m.loadStateLocked(); err != nil {
		log.Warn().Err(err).Str("path", m.statePath).Msg("health monitor: load state failed, using defaults")
	}
	m.ensureStateDefaultsLocked(now)
	if err := m.saveStateLocked(); err != nil {
		log.Warn().Err(err).Str("path", m.statePath).Msg("health monitor: save initial state failed")
	}
	m.mu.Unlock()

	log.Info().Dur("router_interval", m.routerInterval).Dur("dispatch_interval", m.dispatchInterval).Dur("browser_ttl", m.browserTTL).Bool("webhook", m.webhookURL != "").Msg("health monitor started")

	m.wg.Add(2)
	go m.routerLoop()
	go m.dispatchLoop()
}

func (m *HealthMonitor) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	m.started = false
	close(m.stopCh)
	m.mu.Unlock()
	m.wg.Wait()
	log.Info().Msg("health monitor stopped")
}

func (m *HealthMonitor) Summary() healthSummaryResponse {
	if m == nil {
		return healthSummaryResponse{CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	openIncidents := 0
	pendingOutbox := 0
	for _, incident := range m.state.Incidents {
		if incident.Status == "open" {
			openIncidents++
		}
	}
	for _, task := range m.state.Outbox {
		if task.Status == healthOutboxPending {
			pendingOutbox++
		}
	}

	channel := ""
	if m.webhookURL != "" {
		channel = "webhook"
	}

	return healthSummaryResponse{
		CheckedAt:            time.Now().UTC().Format(time.RFC3339),
		Current:              m.state.Current,
		Router:               m.state.Router,
		Browser:              m.state.Browser,
		OpenIncidents:        openIncidents,
		PendingNotifications: pendingOutbox,
		WebhookConfigured:    m.webhookURL != "",
		NotificationChannel:  channel,
		RouterIntervalSec:    int64(m.routerInterval / time.Second),
		BrowserTTLSec:        int64(m.browserTTL / time.Second),
	}
}

func (m *HealthMonitor) Incidents(limit int) []healthIncident {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := len(m.state.Incidents)
	if total == 0 {
		return []healthIncident{}
	}
	if limit <= 0 || limit > total {
		limit = total
	}
	out := make([]healthIncident, 0, limit)
	for i := total - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, m.state.Incidents[i])
	}
	return out
}

func (m *HealthMonitor) SubmitBrowserReport(input healthBrowserReportRequest) (healthProbeSummary, error) {
	if m == nil {
		return healthProbeSummary{}, fmt.Errorf("health monitor unavailable")
	}
	if strings.TrimSpace(input.SessionID) == "" {
		input.SessionID = "browser-" + shortToken(4)
	}
	if len(input.SessionID) > 128 {
		input.SessionID = input.SessionID[:128]
	}
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.CheckedAt = normalizeTimestamp(input.CheckedAt, time.Now().UTC())
	input.UserAgent = strings.TrimSpace(input.UserAgent)

	for idx := range input.IPChecks {
		input.IPChecks[idx].Provider = strings.TrimSpace(input.IPChecks[idx].Provider)
		input.IPChecks[idx].Group = strings.TrimSpace(input.IPChecks[idx].Group)
		input.IPChecks[idx].Error = strings.TrimSpace(input.IPChecks[idx].Error)
	}
	for idx := range input.AccessChecks {
		input.AccessChecks[idx].Name = strings.TrimSpace(input.AccessChecks[idx].Name)
		input.AccessChecks[idx].Group = strings.TrimSpace(input.AccessChecks[idx].Group)
		input.AccessChecks[idx].URL = strings.TrimSpace(input.AccessChecks[idx].URL)
		input.AccessChecks[idx].Error = strings.TrimSpace(input.AccessChecks[idx].Error)
		input.AccessChecks[idx].Stage = strings.TrimSpace(input.AccessChecks[idx].Stage)
	}

	now := time.Now().UTC()
	m.mu.Lock()
	m.ensureStateDefaultsLocked(now)
	if m.state.BrowserReports == nil {
		m.state.BrowserReports = make(map[string]healthBrowserReportRequest)
	}
	m.state.BrowserReports[input.SessionID] = input

	m.refreshBrowserSummaryLocked(now)
	m.evaluateStateLocked(now, "browser_report")
	m.compactStateLocked(now)
	browser := m.state.Browser
	err := m.saveStateLocked()
	m.mu.Unlock()

	if err != nil {
		log.Warn().Err(err).Msg("health monitor: save browser report failed")
	}
	return browser, nil
}

func (m *HealthMonitor) routerLoop() {
	defer m.wg.Done()
	m.runRouterCheck("startup")

	ticker := time.NewTicker(m.routerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.runRouterCheck("tick")
		case <-m.stopCh:
			return
		}
	}
}

func (m *HealthMonitor) dispatchLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.dispatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.dispatchOutbox()
		case <-m.stopCh:
			return
		}
	}
}

func (m *HealthMonitor) runRouterCheck(trigger string) {
	router := m.collectRouterSummary()
	now := time.Now().UTC()

	m.mu.Lock()
	m.ensureStateDefaultsLocked(now)
	m.state.Router = router
	m.state.Current.LastRouterCheck = router.CheckedAt
	m.refreshBrowserSummaryLocked(now)
	m.evaluateStateLocked(now, trigger)
	m.compactStateLocked(now)
	err := m.saveStateLocked()
	m.mu.Unlock()
	if err != nil {
		log.Warn().Err(err).Msg("health monitor: save router check failed")
	}
}

func (m *HealthMonitor) collectRouterSummary() healthProbeSummary {
	now := time.Now().UTC()
	if m.deps.Core == nil {
		return healthProbeSummary{HasData: false, Healthy: false, CheckedAt: now.Format(time.RFC3339), Error: "core manager unavailable"}
	}
	coreStatus := m.deps.Core.Status()
	if !coreStatus.Ready {
		return healthProbeSummary{
			HasData:      true,
			Healthy:      false,
			IPOK:         false,
			CheckedAt:    now.Format(time.RFC3339),
			FailedAccess: []string{"proxy core"},
			Error:        "mihomo core not running",
		}
	}

	data := buildOverviewProbeData(m.deps)
	failed := failedAccessCheckNames(data.AccessChecks)
	ipOK := anyIPCheckOK(data.IPChecks)
	healthy := ipOK && len(failed) == 0
	errMsg := ""
	if !healthy {
		parts := make([]string, 0)
		if !ipOK {
			parts = append(parts, "egress IP checks failed")
		}
		if len(failed) > 0 {
			parts = append(parts, "failed access: "+strings.Join(failed, ", "))
		}
		errMsg = strings.Join(parts, "; ")
	}

	return healthProbeSummary{
		HasData:      true,
		Healthy:      healthy,
		IPOK:         ipOK,
		FailedAccess: failed,
		CheckedAt:    normalizeTimestamp(data.CheckedAt, now),
		Error:        errMsg,
	}
}

func (m *HealthMonitor) refreshBrowserSummaryLocked(now time.Time) {
	if m.state.BrowserReports == nil || len(m.state.BrowserReports) == 0 {
		m.state.Browser = healthProbeSummary{HasData: false, Healthy: false, IPOK: false, Stale: true, Error: "no browser report yet"}
		return
	}

	latestReport := healthBrowserReportRequest{}
	latestAt := time.Time{}
	for sessionID, report := range m.state.BrowserReports {
		reportAt := parseTimestamp(report.CheckedAt)
		if reportAt.IsZero() {
			reportAt = now
		}
		if now.Sub(reportAt) > 24*time.Hour {
			delete(m.state.BrowserReports, sessionID)
			continue
		}
		if latestAt.IsZero() || reportAt.After(latestAt) {
			latestAt = reportAt
			latestReport = report
		}
	}

	if latestAt.IsZero() {
		m.state.Browser = healthProbeSummary{HasData: false, Healthy: false, IPOK: false, Stale: true, Error: "browser reports expired"}
		return
	}

	stale := now.Sub(latestAt) > m.browserTTL
	ipOK := false
	for _, check := range latestReport.IPChecks {
		if check.OK {
			ipOK = true
			break
		}
	}
	failed := make([]string, 0)
	for _, check := range latestReport.AccessChecks {
		if !check.OK {
			name := check.Name
			if name == "" {
				name = "unknown"
			}
			failed = append(failed, name)
		}
	}
	failed = dedupeStrings(failed)

	healthy := ipOK && len(failed) == 0
	errorText := ""
	if stale {
		errorText = fmt.Sprintf("browser report is stale (last update %s)", latestAt.Format(time.RFC3339))
	} else if !healthy {
		parts := make([]string, 0)
		if !ipOK {
			parts = append(parts, "browser IP check failed")
		}
		if len(failed) > 0 {
			parts = append(parts, "browser failed access: "+strings.Join(failed, ", "))
		}
		errorText = strings.Join(parts, "; ")
	}

	m.state.Browser = healthProbeSummary{
		HasData:      true,
		Healthy:      healthy,
		IPOK:         ipOK,
		FailedAccess: failed,
		CheckedAt:    latestAt.UTC().Format(time.RFC3339),
		Stale:        stale,
		Error:        errorText,
	}
	m.state.Current.LastBrowserCheck = m.state.Browser.CheckedAt
}

func (m *HealthMonitor) determineDesiredStateLocked() (string, string) {
	router := m.state.Router
	browser := m.state.Browser

	if !router.HasData {
		return healthStateUnknown, "router probe data unavailable"
	}

	if !router.Healthy {
		if browser.HasData && !browser.Stale && !browser.Healthy {
			return healthStateUnhealthy, buildHealthReason(router, browser)
		}
		return healthStateDegraded, buildHealthReason(router, browser)
	}

	if browser.HasData && !browser.Stale && !browser.Healthy {
		return healthStateDegraded, buildHealthReason(router, browser)
	}

	if browser.Stale {
		return healthStateHealthy, "router healthy; browser report not fresh"
	}

	return healthStateHealthy, "router and browser probes healthy"
}

func (m *HealthMonitor) evaluateStateLocked(now time.Time, trigger string) {
	m.ensureStateDefaultsLocked(now)
	desired, reason := m.determineDesiredStateLocked()
	current := m.state.Current.State

	if current == desired {
		m.state.Current.LastReason = reason
		m.state.Current.ConsecutiveFailures = 0
		m.state.Current.ConsecutiveSuccesses = 0
		return
	}

	if current == healthStateUnknown {
		m.transitionStateLocked(current, desired, reason, now, trigger)
		return
	}

	if stateSeverity(desired) > stateSeverity(current) {
		m.state.Current.ConsecutiveFailures++
		m.state.Current.ConsecutiveSuccesses = 0
		if m.state.Current.ConsecutiveFailures < m.failureThreshold {
			m.state.Current.LastReason = reason
			return
		}
	} else {
		m.state.Current.ConsecutiveSuccesses++
		m.state.Current.ConsecutiveFailures = 0
		if m.state.Current.ConsecutiveSuccesses < m.recoveryThreshold {
			m.state.Current.LastReason = reason
			return
		}
	}

	m.transitionStateLocked(current, desired, reason, now, trigger)
}

func (m *HealthMonitor) transitionStateLocked(from, to, reason string, now time.Time, trigger string) {
	m.state.Current.State = to
	m.state.Current.Since = now.Format(time.RFC3339)
	m.state.Current.LastReason = reason
	m.state.Current.ConsecutiveFailures = 0
	m.state.Current.ConsecutiveSuccesses = 0

	var openedIncident *healthIncident
	var resolvedIncident *healthIncident
	if to == healthStateHealthy {
		if m.state.Current.ActiveIncidentID != "" {
			if resolved, ok := m.resolveIncidentLocked(m.state.Current.ActiveIncidentID, reason, now); ok {
				resolvedIncident = &resolved
				m.queueNotificationLocked(healthEventResolved, resolved, trigger, now)
			}
		}
		m.state.Current.ActiveIncidentID = ""
	} else {
		// Status changed while still unhealthy/degraded:
		// close the previous period and open a new period.
		if m.state.Current.ActiveIncidentID != "" {
			for idx := range m.state.Incidents {
				if m.state.Incidents[idx].ID != m.state.Current.ActiveIncidentID {
					continue
				}
				prevReason := m.state.Incidents[idx].Reason
				if prevReason == "" {
					prevReason = "health period ended"
				}
				if resolved, ok := m.resolveIncidentLocked(m.state.Current.ActiveIncidentID, prevReason, now); ok {
					resolvedIncident = &resolved
					m.queueNotificationLocked(healthEventResolved, resolved, trigger, now)
				}
				break
			}
		}

		incident := healthIncident{
			ID:        uuid.NewString(),
			Status:    "open",
			State:     to,
			Reason:    reason,
			OpenedAt:  now.Format(time.RFC3339),
			UpdatedAt: now.Format(time.RFC3339),
			Router:    m.state.Router,
			Browser:   m.state.Browser,
		}
		m.state.Incidents = append(m.state.Incidents, incident)
		m.state.Current.ActiveIncidentID = incident.ID
		openedIncident = &incident
		m.queueNotificationLocked(healthEventOpened, incident, trigger, now)
	}

	if to == healthStateHealthy {
		log.Info().Str("from", from).Str("to", to).Str("reason", reason).Msg("health state transition")
	} else {
		log.Warn().Str("from", from).Str("to", to).Str("reason", reason).Msg("health state transition")
	}

	if m.deps.SSEBroker != nil {
		m.deps.SSEBroker.Publish("health_state", map[string]any{
			"state":      to,
			"from":       from,
			"reason":     reason,
			"trigger":    trigger,
			"checked_at": now.Format(time.RFC3339),
		})
		if resolvedIncident != nil {
			m.deps.SSEBroker.Publish("health_incident_resolved", resolvedIncident)
		}
		if openedIncident != nil {
			m.deps.SSEBroker.Publish("health_incident_opened", openedIncident)
		}
	}
}

func (m *HealthMonitor) resolveIncidentLocked(id, reason string, now time.Time) (healthIncident, bool) {
	for idx := range m.state.Incidents {
		if m.state.Incidents[idx].ID != id {
			continue
		}
		m.state.Incidents[idx].Status = "resolved"
		m.state.Incidents[idx].Reason = reason
		m.state.Incidents[idx].Router = m.state.Router
		m.state.Incidents[idx].Browser = m.state.Browser
		m.state.Incidents[idx].UpdatedAt = now.Format(time.RFC3339)
		m.state.Incidents[idx].ResolvedAt = now.Format(time.RFC3339)
		return m.state.Incidents[idx], true
	}
	return healthIncident{}, false
}

func (m *HealthMonitor) queueNotificationLocked(event string, incident healthIncident, trigger string, now time.Time) {
	if m.webhookURL == "" {
		return
	}
	for idx := range m.state.Outbox {
		task := m.state.Outbox[idx]
		if task.Channel == "webhook" && task.IncidentID == incident.ID && task.Event == event {
			// Keep notification tasks idempotent per (incident, event, channel).
			return
		}
	}

	payload := healthNotificationPayload{
		Event:      event,
		IncidentID: incident.ID,
		State:      incident.State,
		Reason:     incident.Reason,
		OpenedAt:   incident.OpenedAt,
		ResolvedAt: incident.ResolvedAt,
		Router:     incident.Router,
		Browser:    incident.Browser,
		Trigger:    trigger,
	}

	task := healthNotificationTask{
		ID:         uuid.NewString(),
		IncidentID: incident.ID,
		Event:      event,
		Channel:    "webhook",
		Status:     healthOutboxPending,
		Attempts:   0,
		CreatedAt:  now.Format(time.RFC3339),
		NextRetry:  now.Format(time.RFC3339),
		Payload:    payload,
	}
	m.state.Outbox = append(m.state.Outbox, task)
}

func (m *HealthMonitor) dispatchOutbox() {
	if m.webhookURL == "" {
		return
	}

	now := time.Now().UTC()
	m.mu.RLock()
	due := m.dueTasksLocked(now)
	m.mu.RUnlock()
	if len(due) == 0 {
		return
	}

	if !m.internetReachable() {
		m.deferTasksForOffline(due, now)
		return
	}

	for _, task := range due {
		err := m.sendWebhook(task)
		m.applyDispatchResult(task, err, now)
	}
}

func (m *HealthMonitor) dueTasksLocked(now time.Time) []healthNotificationTask {
	out := make([]healthNotificationTask, 0)
	for _, task := range m.state.Outbox {
		if task.Status != healthOutboxPending {
			continue
		}
		nextRetry := parseTimestamp(task.NextRetry)
		if nextRetry.IsZero() || !nextRetry.After(now) {
			out = append(out, task)
		}
	}
	return out
}

func (m *HealthMonitor) internetReachable() bool {
	if m.canaryURL == "" {
		return true
	}
	req, err := http.NewRequest(http.MethodHead, m.canaryURL, nil)
	if err != nil {
		return false
	}
	resp, err := m.canaryHTTPClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}

func (m *HealthMonitor) deferTasksForOffline(tasks []healthNotificationTask, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	for _, due := range tasks {
		for idx := range m.state.Outbox {
			if m.state.Outbox[idx].ID != due.ID || m.state.Outbox[idx].Status != healthOutboxPending {
				continue
			}
			m.state.Outbox[idx].LastError = "network unavailable, notification queued for retry"
			nextRetry := now.Add(30 * time.Second).Format(time.RFC3339)
			if m.state.Outbox[idx].NextRetry != nextRetry {
				m.state.Outbox[idx].NextRetry = nextRetry
				changed = true
			}
			break
		}
	}
	if changed {
		if err := m.saveStateLocked(); err != nil {
			log.Warn().Err(err).Msg("health monitor: save offline outbox failed")
		}
	}
	if m.deps.SSEBroker != nil && (m.lastOfflineNoticeAt.IsZero() || now.Sub(m.lastOfflineNoticeAt) >= 2*time.Minute) {
		m.deps.SSEBroker.Publish("health_notification_retrying", map[string]any{
			"reason": "network_unavailable",
			"count":  len(tasks),
			"at":     now.Format(time.RFC3339),
		})
		m.lastOfflineNoticeAt = now
	}
}

func (m *HealthMonitor) sendWebhook(task healthNotificationTask) error {
	body := map[string]any{
		"id":          task.ID,
		"incident_id": task.IncidentID,
		"event":       task.Event,
		"attempt":     task.Attempts + 1,
		"created_at":  task.CreatedAt,
		"payload":     task.Payload,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, m.webhookURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.notifyHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("webhook http %d: %s", resp.StatusCode, strings.TrimSpace(string(buf)))
}

func (m *HealthMonitor) applyDispatchResult(task healthNotificationTask, dispatchErr error, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	attempt := 0
	for idx := range m.state.Outbox {
		if m.state.Outbox[idx].ID != task.ID {
			continue
		}
		if dispatchErr == nil {
			m.state.Outbox[idx].Status = healthOutboxSent
			m.state.Outbox[idx].SentAt = now.Format(time.RFC3339)
			m.state.Outbox[idx].LastError = ""
			m.state.Outbox[idx].Attempts++
			attempt = m.state.Outbox[idx].Attempts
		} else {
			attempt = m.state.Outbox[idx].Attempts + 1
			m.state.Outbox[idx].Attempts = attempt
			m.state.Outbox[idx].LastError = dispatchErr.Error()
			m.state.Outbox[idx].NextRetry = now.Add(nextBackoff(attempt)).Format(time.RFC3339)
		}
		break
	}
	m.compactStateLocked(now)
	if err := m.saveStateLocked(); err != nil {
		log.Warn().Err(err).Msg("health monitor: save outbox dispatch failed")
	}
	if dispatchErr != nil {
		// Avoid flooding logs when network stays unavailable for a long time.
		if attempt <= 3 || attempt%10 == 0 {
			log.Warn().Err(dispatchErr).Int("attempt", attempt).Str("incident_id", task.IncidentID).Str("event", task.Event).Msg("health monitor notification failed")
		}
	}
}

func (m *HealthMonitor) ensureStateDefaultsLocked(now time.Time) {
	if m.state.Version == 0 {
		m.state.Version = healthStoreVersion
	}
	if m.state.Current.State == "" {
		m.state.Current.State = healthStateUnknown
	}
	if m.state.Current.Since == "" {
		m.state.Current.Since = now.Format(time.RFC3339)
	}
	if m.state.BrowserReports == nil {
		m.state.BrowserReports = make(map[string]healthBrowserReportRequest)
	}
}

func (m *HealthMonitor) compactStateLocked(now time.Time) {
	// Keep incidents bounded by recency.
	if len(m.state.Incidents) > 200 {
		m.state.Incidents = slices.Clone(m.state.Incidents[len(m.state.Incidents)-200:])
	}

	if len(m.state.Outbox) > 0 {
		trimmed := make([]healthNotificationTask, 0, len(m.state.Outbox))
		for _, task := range m.state.Outbox {
			if task.Status == healthOutboxSent {
				sentAt := parseTimestamp(task.SentAt)
				if !sentAt.IsZero() && now.Sub(sentAt) > 7*24*time.Hour {
					continue
				}
			}
			trimmed = append(trimmed, task)
		}
		if len(trimmed) > 500 {
			trimmed = trimmed[len(trimmed)-500:]
		}
		m.state.Outbox = trimmed
	}
}

func (m *HealthMonitor) loadStateLocked() error {
	data, err := os.ReadFile(m.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			m.state = healthMonitorState{}
			return nil
		}
		return err
	}
	if len(data) == 0 {
		m.state = healthMonitorState{}
		return nil
	}
	var parsed healthMonitorState
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	m.state = parsed
	return nil
}

func (m *HealthMonitor) saveStateLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, m.statePath); err != nil {
		return err
	}
	return nil
}

func buildHealthReason(router, browser healthProbeSummary) string {
	parts := make([]string, 0)
	if !router.Healthy {
		if router.Error != "" {
			parts = append(parts, "router: "+router.Error)
		} else if len(router.FailedAccess) > 0 {
			parts = append(parts, "router failed access: "+strings.Join(router.FailedAccess, ", "))
		}
	}
	if browser.HasData && !browser.Stale && !browser.Healthy {
		if browser.Error != "" {
			parts = append(parts, "browser: "+browser.Error)
		} else if len(browser.FailedAccess) > 0 {
			parts = append(parts, "browser failed access: "+strings.Join(browser.FailedAccess, ", "))
		}
	}
	if len(parts) == 0 {
		return "health checks indicate a temporary mismatch"
	}
	return strings.Join(parts, "; ")
}

func stateSeverity(state string) int {
	switch state {
	case healthStateHealthy:
		return 0
	case healthStateUnknown:
		return 1
	case healthStateDegraded:
		return 2
	case healthStateUnhealthy:
		return 3
	default:
		return 1
	}
}

func nextBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	exp := math.Pow(2, float64(minInt(attempt-1, 6)))
	base := time.Duration(exp) * 15 * time.Second
	if base > 30*time.Minute {
		base = 30 * time.Minute
	}
	jitter := time.Duration(rand.Intn(3000)) * time.Millisecond
	return base + jitter
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func normalizeTimestamp(raw string, fallback time.Time) string {
	ts := parseTimestamp(raw)
	if ts.IsZero() {
		ts = fallback
	}
	return ts.UTC().Format(time.RFC3339)
}

func parseTimestamp(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	formats := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"}
	for _, format := range formats {
		if ts, err := time.Parse(format, raw); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}

func shortToken(n int) string {
	if n <= 0 {
		return ""
	}
	letters := "abcdefghijklmnopqrstuvwxyz0123456789"
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = letters[rand.Intn(len(letters))]
	}
	return string(buf)
}

func parsePositiveInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("must be > 0")
	}
	return n, nil
}
