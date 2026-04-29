package geodata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/wujun4code/clashforge/internal/config"
)

const maxRecords = 100

// FileSpec describes a single GeoData file and its download mirrors.
type FileSpec struct {
	Name     string
	Filename string
	URLs     []string
}

// FileResult is the outcome of downloading one GeoData file.
type FileResult struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"` // ok | error
	SizeBytes int64   `json:"size_bytes,omitempty"`
	Message   string  `json:"message,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// UpdateRecord tracks one full GeoData update run (both files).
type UpdateRecord struct {
	ID          string       `json:"id"`
	StartedAt   time.Time    `json:"started_at"`
	FinishedAt  *time.Time   `json:"finished_at,omitempty"`
	Status      string       `json:"status"` // running | ok | error
	ProxyServer string       `json:"proxy_server"`
	Files       []FileResult `json:"files"`
	Error       string       `json:"error,omitempty"`
}

// FileStatus holds on-disk metadata for one GeoData file.
type FileStatus struct {
	Name      string    `json:"name"`
	Filename  string    `json:"filename"`
	Path      string    `json:"path"`
	Exists    bool      `json:"exists"`
	SizeBytes int64     `json:"size_bytes,omitempty"`
	ModTime   time.Time `json:"mod_time,omitempty"`
}

// Manager owns the GeoData update lifecycle: download, log, schedule.
type Manager struct {
	mu      sync.RWMutex
	cfg     *config.MetaclashConfig
	records []*UpdateRecord
	running bool
}

// New creates a Manager.
func New(cfg *config.MetaclashConfig) *Manager {
	return &Manager{cfg: cfg}
}

// UpdateConfig atomically replaces the live config reference (called after settings save).
func (m *Manager) UpdateConfig(cfg *config.MetaclashConfig) {
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
}

// IsRunning reports whether an update is currently in progress.
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// Records returns a copy of the update history (newest last).
func (m *Manager) Records() []*UpdateRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*UpdateRecord, len(m.records))
	copy(out, m.records)
	return out
}

// LatestRecord returns the most recent update record, or nil if none.
func (m *Manager) LatestRecord() *UpdateRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.records) == 0 {
		return nil
	}
	return m.records[len(m.records)-1]
}

// TriggerAsync kicks off an update in the background.
// Returns (record, true) if started, or (nil, false) if already running.
func (m *Manager) TriggerAsync(proxyServer string) (*UpdateRecord, bool) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil, false
	}
	rec := m.newRecord(proxyServer)
	m.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		m.execute(ctx, rec)
	}()
	return rec, true
}

// TriggerSync runs an update synchronously (used by the scheduler).
func (m *Manager) TriggerSync(proxyServer string) *UpdateRecord {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	rec := m.newRecord(proxyServer)
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	m.execute(ctx, rec)
	return rec
}

// FileStatuses returns on-disk metadata for each configured GeoData file.
func (m *Manager) FileStatuses() []FileStatus {
	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()
	return fileStatuses(cfg)
}

// --- internals ---

func (m *Manager) newRecord(proxyServer string) *UpdateRecord {
	rec := &UpdateRecord{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		StartedAt:   time.Now(),
		Status:      "running",
		ProxyServer: proxyServer,
	}
	m.running = true
	m.records = append(m.records, rec)
	if len(m.records) > maxRecords {
		m.records = m.records[len(m.records)-maxRecords:]
	}
	return rec
}

func (m *Manager) execute(ctx context.Context, rec *UpdateRecord) {
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	proxyURL := buildProxyURL(rec.ProxyServer, cfg.Ports.Mixed)

	log.Info().
		Str("side", "geodata").
		Str("record_id", rec.ID).
		Str("proxy_server", rec.ProxyServer).
		Str("proxy_url", proxyURL).
		Msg("geodata update started")

	specs := buildSpecs(cfg)
	var results []FileResult
	var firstErr string

	for _, spec := range specs {
		res := downloadSpec(ctx, spec, cfg.Core.DataDir, proxyURL)
		results = append(results, res)
		if res.Status == "error" && firstErr == "" {
			firstErr = fmt.Sprintf("%s: %s", spec.Name, res.Error)
		}
	}

	now := time.Now()
	m.mu.Lock()
	rec.FinishedAt = &now
	rec.Files = results
	if firstErr != "" {
		rec.Status = "error"
		rec.Error = firstErr
	} else {
		rec.Status = "ok"
	}
	m.mu.Unlock()

	if firstErr != "" {
		log.Error().Str("side", "geodata").Str("record_id", rec.ID).Str("error", firstErr).Msg("geodata update failed")
	} else {
		log.Info().Str("side", "geodata").Str("record_id", rec.ID).Msg("geodata update completed")
	}
}

func downloadSpec(ctx context.Context, spec FileSpec, dataDir, proxyURL string) FileResult {
	destPath := filepath.Join(dataDir, spec.Filename)

	log.Info().
		Str("side", "geodata").
		Str("file", spec.Name).
		Msg("downloading")

	var lastErr error
	for i, dlURL := range spec.URLs {
		host := urlHost(dlURL)
		log.Info().
			Str("side", "geodata").
			Str("file", spec.Name).
			Str("mirror", host).
			Int("attempt", i+1).
			Int("total", len(spec.URLs)).
			Msg("trying mirror")

		size, err := downloadFile(ctx, dlURL, destPath, proxyURL, 5*time.Minute)
		if err == nil {
			log.Info().
				Str("side", "geodata").
				Str("file", spec.Name).
				Str("mirror", host).
				Int64("bytes", size).
				Msg("download ok")
			return FileResult{
				Name:      spec.Name,
				Status:    "ok",
				SizeBytes: size,
				Message:   fmt.Sprintf("从 %s 下载成功 (%.1f MB)", host, float64(size)/1024/1024),
			}
		}

		log.Warn().
			Str("side", "geodata").
			Str("file", spec.Name).
			Str("mirror", host).
			Err(err).
			Msg("mirror failed")
		lastErr = err
	}

	return FileResult{
		Name:   spec.Name,
		Status: "error",
		Error:  lastErr.Error(),
	}
}

func downloadFile(ctx context.Context, rawURL, destPath, proxyURL string, timeout time.Duration) (int64, error) {
	transport := &http.Transport{}
	if proxyURL != "" {
		pURL, err := url.Parse(proxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(pURL)
		}
	}
	client := &http.Client{Timeout: timeout, Transport: transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return 0, fmt.Errorf("mkdir: %w", err)
	}

	f, err := os.CreateTemp(filepath.Dir(destPath), ".geodata-*.tmp")
	if err != nil {
		return 0, err
	}
	tmp := f.Name()
	defer os.Remove(tmp)

	n, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		return 0, fmt.Errorf("write: %w", err)
	}
	if n == 0 {
		return 0, fmt.Errorf("empty response body from %s", rawURL)
	}
	return n, os.Rename(tmp, destPath)
}

// buildProxyURL returns the HTTP proxy URL to use for downloads, or "" for direct.
// When proxy is enabled, we route through mihomo's mixed proxy port so that
// the download uses whichever node the user has currently selected in mihomo.
func buildProxyURL(proxyServer string, mixedPort int) string {
	if proxyServer == "" || proxyServer == "DIRECT" {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d", mixedPort)
}

func buildSpecs(cfg *config.MetaclashConfig) []FileSpec {
	geoIPURLs := []string{
		"https://cdn.jsdmirror.com/gh/MetaCubeX/meta-rules-dat@release/geoip.dat",
		"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.dat",
	}
	if u := cfg.Update.GeoIPURL; u != "" {
		geoIPURLs = append([]string{u}, geoIPURLs...)
	}

	geositeURLs := []string{
		"https://cdn.jsdmirror.com/gh/MetaCubeX/meta-rules-dat@release/geosite.dat",
		"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
	}
	if u := cfg.Update.GeositeURL; u != "" {
		geositeURLs = append([]string{u}, geositeURLs...)
	}

	return []FileSpec{
		{Name: "GeoIP.dat", Filename: "GeoIP.dat", URLs: geoIPURLs},
		{Name: "GeoSite.dat", Filename: "GeoSite.dat", URLs: geositeURLs},
	}
}

func fileStatuses(cfg *config.MetaclashConfig) []FileStatus {
	specs := buildSpecs(cfg)
	out := make([]FileStatus, 0, len(specs))
	for _, spec := range specs {
		path := filepath.Join(cfg.Core.DataDir, spec.Filename)
		fs := FileStatus{Name: spec.Name, Filename: spec.Filename, Path: path}
		if info, err := os.Stat(path); err == nil {
			fs.Exists = true
			fs.SizeBytes = info.Size()
			fs.ModTime = info.ModTime()
		}
		out = append(out, fs)
	}
	return out
}

func urlHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}
