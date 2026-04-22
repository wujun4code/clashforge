package core

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

type CoreState string

const (
	StateStopped  CoreState = "stopped"
	StateStarting CoreState = "starting"
	StateRunning  CoreState = "running"
	StateStopping CoreState = "stopping"
	StateError    CoreState = "error"
)

var (
	ErrAlreadyRunning = fmt.Errorf("core already running")
	ErrNotRunning     = fmt.Errorf("core not running")
)

type CoreManagerConfig struct {
	Binary      string
	ConfigFile  string
	APIPort     int
	MaxRestarts int
}

type Status struct {
	State      CoreState     `json:"state"`
	PID        int           `json:"pid"`
	Restarts   int           `json:"restarts"`
	Uptime     int64         `json:"uptime"`
	StartedAt  time.Time     `json:"started_at,omitempty"`
	LastError  string        `json:"last_error,omitempty"`
	ConfigFile string        `json:"config_file,omitempty"`
	Binary     string        `json:"binary,omitempty"`
	Ready      bool          `json:"ready"`
	APIPort    int           `json:"api_port"`
	Duration   time.Duration `json:"-"`
}

type CoreManager struct {
	mu            sync.Mutex
	cmd           *exec.Cmd
	state         CoreState
	pid           int
	startTime     time.Time
	restartCount  int
	restartTimes  []time.Time
	cfg           CoreManagerConfig
	onStateChange func(state CoreState, pid int)
	deathCh       chan error
	stopCh        chan struct{}
	lastError     string
}

func NewManager(cfg CoreManagerConfig) *CoreManager {
	return &CoreManager{cfg: cfg, state: StateStopped}
}

func (m *CoreManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateRunning || m.state == StateStarting {
		return ErrAlreadyRunning
	}
	m.stopCh = make(chan struct{})
	return m.startLocked(ctx)
}

func (m *CoreManager) startLocked(ctx context.Context) error {
	m.setState(StateStarting, 0)
	// The caller context only scopes readiness waiting, not mihomo's lifetime.
	// Using CommandContext here would kill the long-running child as soon as an
	// HTTP request context is canceled or times out.
	cmd := exec.Command(m.cfg.Binary, "-d", filepath.Dir(m.cfg.ConfigFile), "-f", m.cfg.ConfigFile)
	cmd.Stdout = newCoreLogWriter("stdout")
	cmd.Stderr = newCoreLogWriter("stderr")
	if err := cmd.Start(); err != nil {
		m.lastError = err.Error()
		m.setState(StateError, 0)
		return fmt.Errorf("failed to start mihomo: %w", err)
	}
	m.cmd = cmd
	m.pid = cmd.Process.Pid
	m.startTime = time.Now()
	m.deathCh = make(chan error, 1)
	go func() {
		err := cmd.Wait()
		m.deathCh <- err
		m.handleDeath(err)
	}()
	if err := m.waitAPIReady(ctx, 5*time.Second); err != nil {
		log.Warn().Err(err).Msg("mihomo API not ready within timeout")
	}
	m.lastError = ""
	m.setState(StateRunning, m.pid)
	log.Info().Int("pid", m.pid).Msg("mihomo started")
	return nil
}

func (m *CoreManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateStopped || m.cmd == nil {
		return ErrNotRunning
	}
	if m.stopCh != nil {
		close(m.stopCh)
		m.stopCh = nil
	}
	return m.stopLocked()
}

func (m *CoreManager) stopLocked() error {
	m.setState(StateStopping, m.pid)
	process := m.cmd.Process
	if process == nil {
		m.cmd = nil
		m.pid = 0
		m.setState(StateStopped, 0)
		return nil
	}
	_ = process.Signal(syscall.SIGTERM)
	select {
	case <-m.deathCh:
	case <-time.After(5 * time.Second):
		log.Warn().Int("pid", m.pid).Msg("mihomo did not stop gracefully, killing")
		_ = process.Kill()
		<-m.deathCh
	}
	m.cmd = nil
	m.pid = 0
	m.setState(StateStopped, 0)
	log.Info().Msg("mihomo stopped")
	return nil
}

func (m *CoreManager) Restart(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateRunning || m.state == StateStopping {
		if err := m.stopLocked(); err != nil {
			return err
		}
	}
	m.stopCh = make(chan struct{})
	return m.startLocked(ctx)
}

func (m *CoreManager) Reload(configFile string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != StateRunning {
		return ErrNotRunning
	}
	out, err := exec.Command(m.cfg.Binary, "-t", "-d", filepath.Dir(configFile), "-f", configFile).CombinedOutput()
	if err != nil {
		return fmt.Errorf("config validation failed: %s", string(out))
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/configs?force=false", m.cfg.APIPort)
	body := fmt.Sprintf(`{"path":"%s"}`, configFile)
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("reload request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("reload rejected by mihomo: %s", string(b))
	}
	return nil
}

func (m *CoreManager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := Status{State: m.state, PID: m.pid, Restarts: m.restartCount, ConfigFile: m.cfg.ConfigFile, Binary: m.cfg.Binary, APIPort: m.cfg.APIPort, LastError: m.lastError}
	if !m.startTime.IsZero() {
		status.StartedAt = m.startTime
		status.Duration = time.Since(m.startTime)
		status.Uptime = int64(status.Duration.Seconds())
	}
	status.Ready = m.state == StateRunning
	return status
}

func (m *CoreManager) CurrentVersion(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, m.cfg.Binary, "-v")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func (m *CoreManager) handleDeath(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateStopping || m.state == StateStopped {
		return
	}
	if err != nil {
		m.lastError = err.Error()
		log.Error().Err(err).Int("pid", m.pid).Msg("mihomo exited unexpectedly")
	}
	m.cmd = nil
	m.pid = 0
	m.setState(StateError, 0)
	m.restartTimes = append(m.restartTimes, time.Now())
	cutoff := time.Now().Add(-60 * time.Second)
	recent := 0
	for _, t := range m.restartTimes {
		if t.After(cutoff) {
			recent++
		}
	}
	if m.cfg.MaxRestarts > 0 && recent > m.cfg.MaxRestarts {
		log.Error().Int("recent_restarts", recent).Int("max_restarts", m.cfg.MaxRestarts).Msg("mihomo restart limit exceeded")
		return
	}
	stopCh := m.stopCh
	go func() {
		select {
		case <-time.After(2 * time.Second):
		case <-stopCh:
			return
		}
		if err := m.Start(context.Background()); err != nil && err != ErrAlreadyRunning {
			log.Error().Err(err).Msg("auto-restart failed")
		}
	}()
}

func (m *CoreManager) waitAPIReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/version", m.cfg.APIPort)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("API not ready after %v", timeout)
}

func (m *CoreManager) setState(s CoreState, pid int) {
	m.state = s
	m.pid = pid
	if m.onStateChange != nil {
		go m.onStateChange(s, pid)
	}
}

type coreLogWriter struct {
	stream string
	mu     sync.Mutex
	buf    bytes.Buffer
}

func newCoreLogWriter(stream string) io.Writer { return &coreLogWriter{stream: stream} }

func (w *coreLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			w.buf.WriteString(line)
			break
		}
		line = strings.TrimSpace(line)
		if line != "" {
			log.Info().Str("component", "mihomo").Str("stream", w.stream).Msg(line)
		}
	}
	return len(p), nil
}
