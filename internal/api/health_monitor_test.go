package api

import (
	"testing"
	"time"
)

func TestHealthMonitorStableStateKeepsSingleIncidentPeriod(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	m := &HealthMonitor{failureThreshold: 1, recoveryThreshold: 1}
	m.ensureStateDefaultsLocked(now)

	m.state.Router = healthProbeSummary{HasData: true, Healthy: false, Error: "router failed"}
	m.state.Browser = healthProbeSummary{HasData: true, Healthy: true, IPOK: true, CheckedAt: now.Format(time.RFC3339)}
	m.evaluateStateLocked(now, "test")

	if m.state.Current.State != healthStateDegraded {
		t.Fatalf("expected state degraded, got %s", m.state.Current.State)
	}
	if len(m.state.Incidents) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(m.state.Incidents))
	}
	first := m.state.Incidents[0]
	if first.Status != "open" {
		t.Fatalf("expected first incident open, got %s", first.Status)
	}

	later := now.Add(10 * time.Minute)
	m.state.Router.CheckedAt = later.Format(time.RFC3339)
	m.evaluateStateLocked(later, "test")

	if len(m.state.Incidents) != 1 {
		t.Fatalf("expected still 1 incident while state unchanged, got %d", len(m.state.Incidents))
	}
	after := m.state.Incidents[0]
	if after.ID != first.ID {
		t.Fatalf("expected same incident id, got %s vs %s", after.ID, first.ID)
	}
	if after.OpenedAt != first.OpenedAt {
		t.Fatalf("opened_at should stay unchanged, got %s vs %s", after.OpenedAt, first.OpenedAt)
	}
	if after.UpdatedAt != first.UpdatedAt {
		t.Fatalf("updated_at should not advance while state unchanged, got %s vs %s", after.UpdatedAt, first.UpdatedAt)
	}
}

func TestHealthMonitorStateChangeCreatesNewIncidentPeriod(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	m := &HealthMonitor{failureThreshold: 1, recoveryThreshold: 1}
	m.ensureStateDefaultsLocked(now)

	// First period: degraded (router failed, browser healthy).
	m.state.Router = healthProbeSummary{HasData: true, Healthy: false, Error: "router failed"}
	m.state.Browser = healthProbeSummary{HasData: true, Healthy: true, IPOK: true, CheckedAt: now.Format(time.RFC3339)}
	m.evaluateStateLocked(now, "test")

	if len(m.state.Incidents) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(m.state.Incidents))
	}
	firstID := m.state.Incidents[0].ID

	// Second period: unhealthy (router + browser both failed).
	later := now.Add(2 * time.Minute)
	m.state.Browser = healthProbeSummary{HasData: true, Healthy: false, IPOK: false, CheckedAt: later.Format(time.RFC3339), Error: "browser failed"}
	m.evaluateStateLocked(later, "test")

	if m.state.Current.State != healthStateUnhealthy {
		t.Fatalf("expected state unhealthy, got %s", m.state.Current.State)
	}
	if len(m.state.Incidents) != 2 {
		t.Fatalf("expected 2 incidents after state change, got %d", len(m.state.Incidents))
	}
	if m.state.Incidents[0].Status != "resolved" {
		t.Fatalf("expected first incident resolved, got %s", m.state.Incidents[0].Status)
	}
	if m.state.Incidents[0].ResolvedAt == "" {
		t.Fatal("expected first incident resolved_at to be set")
	}
	if m.state.Incidents[1].Status != "open" {
		t.Fatalf("expected second incident open, got %s", m.state.Incidents[1].Status)
	}
	if m.state.Incidents[1].ID == firstID {
		t.Fatal("expected new incident id for new state period")
	}
	if m.state.Current.ActiveIncidentID != m.state.Incidents[1].ID {
		t.Fatalf("active incident mismatch: %s vs %s", m.state.Current.ActiveIncidentID, m.state.Incidents[1].ID)
	}
}

func TestHealthMonitorQueueNotificationDedup(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	m := &HealthMonitor{webhookURL: "http://example.com/webhook"}
	m.ensureStateDefaultsLocked(now)
	incident := healthIncident{
		ID:        "inc-1",
		Status:    "open",
		State:     healthStateDegraded,
		Reason:    "router failed",
		OpenedAt:  now.Format(time.RFC3339),
		UpdatedAt: now.Format(time.RFC3339),
		Router:    healthProbeSummary{HasData: true, Healthy: false},
		Browser:   healthProbeSummary{HasData: true, Healthy: true},
	}

	m.queueNotificationLocked(healthEventOpened, incident, "test", now)
	m.queueNotificationLocked(healthEventOpened, incident, "test", now.Add(5*time.Second))
	if len(m.state.Outbox) != 1 {
		t.Fatalf("expected deduped outbox size 1, got %d", len(m.state.Outbox))
	}

	m.queueNotificationLocked(healthEventResolved, incident, "test", now.Add(10*time.Second))
	if len(m.state.Outbox) != 2 {
		t.Fatalf("expected resolved event to enqueue separately, got %d", len(m.state.Outbox))
	}
}
