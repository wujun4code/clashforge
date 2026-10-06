package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
)

func TestSetupPreviewUsesRequestedDNSStrategyWithoutChangingSavedSettings(t *testing.T) {
	cfg := config.Default()
	cfg.Core.DataDir = t.TempDir()
	cfg.Core.RuntimeDir = filepath.Join(cfg.Core.DataDir, "runtime")
	deps := Dependencies{Config: cfg, SubManager: subscription.NewManager(cfg.Core.DataDir)}
	body := `{"dns":{"enable":true,"mode":"fake-ip","strategy":"anti-hijack"},"network":{"mode":"none","firewall_backend":"none"}}`
	req := httptest.NewRequest(http.MethodPost, "/setup/final-config-preview", strings.NewReader(body))
	response := httptest.NewRecorder()
	handleSetupFinalConfigPreview(deps)(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("preview failed: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "https://86.54.11.100/dns-query#DIRECT") || strings.Contains(response.Body.String(), "dhcp://") {
		t.Fatalf("preview did not use requested strict DNS strategy: %s", response.Body.String())
	}
	if cfg.DNS.Strategy != config.DNSStrategysplit {
		t.Fatalf("preview mutated saved DNS strategy: %s", cfg.DNS.Strategy)
	}
}
