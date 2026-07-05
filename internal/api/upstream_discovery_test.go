package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
)

// Real /proc/net/route line for default route via 192.168.10.1 on eth1:
// gateway "010AA8C0" is the little-endian hex encoding of 192.168.10.1
// (bytes 01,0A,A8,C0 reversed to display order C0.A8.0A.01).
const sampleRouteTable = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth1	00000000	010AA8C0	0003	0	0	0	00000000	0	0	0
eth1	000AA8C0	00000000	0001	0	0	0	00FFFFFF	0	0	0
`

func TestParseDefaultGatewayFromRouteTable(t *testing.T) {
	got := parseDefaultGatewayFromRouteTable([]byte(sampleRouteTable))
	want := "192.168.10.1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseDefaultGatewayFromRouteTable_NoDefaultRoute(t *testing.T) {
	table := "Iface\tDestination\tGateway\n" +
		"eth1\t000AA8C0\t00000000\n" // only a non-default route
	if got := parseDefaultGatewayFromRouteTable([]byte(table)); got != "" {
		t.Fatalf("expected empty string when no default route present, got %q", got)
	}
}

func TestParseDefaultGatewayFromRouteTable_Empty(t *testing.T) {
	if got := parseDefaultGatewayFromRouteTable([]byte("")); got != "" {
		t.Fatalf("expected empty string for empty input, got %q", got)
	}
}

func TestQueryUpstreamDNSTopology_ConfirmedFakeIP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]interface{}{
			"clashforge": true,
			"version":    "0.1.0-dev",
			"dns": config.DNSTopology{
				EnhancedMode: "fake-ip",
				FakeIPRange:  "198.18.0.1/16",
			},
		})
	}))
	defer srv.Close()

	topo, ok := queryUpstreamDNSTopology(srv.URL)
	if !ok {
		t.Fatal("expected ok=true for a confirmed ClashForge fake-ip response")
	}
	if topo.FakeIPRange != "198.18.0.1/16" {
		t.Fatalf("got fake_ip_range %q, want 198.18.0.1/16", topo.FakeIPRange)
	}
}

func TestQueryUpstreamDNSTopology_RedirHostIgnored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]interface{}{
			"clashforge": true,
			"dns": config.DNSTopology{
				EnhancedMode: "redir-host",
				FakeIPRange:  "",
			},
		})
	}))
	defer srv.Close()

	if _, ok := queryUpstreamDNSTopology(srv.URL); ok {
		t.Fatal("expected ok=false for redir-host (no fake-ip-range to collide with)")
	}
}

func TestQueryUpstreamDNSTopology_NotClashForge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html>some other device's admin page</html>"))
	}))
	defer srv.Close()

	if _, ok := queryUpstreamDNSTopology(srv.URL); ok {
		t.Fatal("expected ok=false for a non-JSON / non-ClashForge response")
	}
}

func TestQueryUpstreamDNSTopology_Unreachable(t *testing.T) {
	if _, ok := queryUpstreamDNSTopology("http://127.0.0.1:1"); ok {
		t.Fatal("expected ok=false when the upstream is unreachable")
	}
}

func TestHandleDNSTopology_NeverExposesProxyFields(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}
	mihomoYAML := []byte(`
proxies:
  - name: sg-02
    server: sales.example.com
    password: super-secret
dns:
  enhanced-mode: fake-ip
  fake-ip-range: 28.18.0.1/16
`)
	if err := os.WriteFile(filepath.Join(runtimeDir, "mihomo-config.yaml"), mihomoYAML, 0o644); err != nil {
		t.Fatalf("write mihomo-config.yaml: %v", err)
	}

	deps := Dependencies{
		Version: "0.1.0-dev",
		Config: &config.MetaclashConfig{
			Core: config.CoreConfig{RuntimeDir: runtimeDir},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/discovery/dns-topology", nil)
	w := httptest.NewRecorder()
	handleDNSTopology(deps)(w, req)

	body := w.Body.String()
	for _, forbidden := range []string{"sg-02", "sales.example.com", "super-secret", "proxies"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked sensitive field %q: %s", forbidden, body)
		}
	}

	var parsed struct {
		OK   bool `json:"ok"`
		Data struct {
			ClashForge bool               `json:"clashforge"`
			DNS        config.DNSTopology `json:"dns"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if !parsed.OK || !parsed.Data.ClashForge {
		t.Fatalf("expected ok=true, clashforge=true, got %+v", parsed)
	}
	if parsed.Data.DNS.EnhancedMode != "fake-ip" || parsed.Data.DNS.FakeIPRange != "28.18.0.1/16" {
		t.Fatalf("unexpected DNS topology: %+v", parsed.Data.DNS)
	}
}

