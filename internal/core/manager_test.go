package core

import (
	"errors"
	"testing"
)

func TestHandleDeathIgnoresStaleRun(t *testing.T) {
	m := NewManager(CoreManagerConfig{MaxRestarts: 3})
	m.state = StateRunning
	m.pid = 2222
	m.runID = 2
	m.stopCh = make(chan struct{})

	m.handleDeath(errors.New("stale exit"), 1, 1111)

	if m.state != StateRunning {
		t.Fatalf("expected state to stay running, got %s", m.state)
	}
	if m.pid != 2222 {
		t.Fatalf("expected pid to stay 2222, got %d", m.pid)
	}
}

func TestHandleDeathCurrentRunMarksError(t *testing.T) {
	m := NewManager(CoreManagerConfig{MaxRestarts: 3})
	m.state = StateRunning
	m.pid = 3333
	m.runID = 4
	m.stopCh = make(chan struct{})
	close(m.stopCh)

	m.handleDeath(errors.New("current exit"), 4, 3333)

	if m.state != StateError {
		t.Fatalf("expected state error, got %s", m.state)
	}
	if m.pid != 0 {
		t.Fatalf("expected pid to reset to 0, got %d", m.pid)
	}
}
