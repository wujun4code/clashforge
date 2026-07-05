package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/wujun4code/clashforge/internal/config"
)

// discoveryPort is ClashForge's default web/API port. Upstream-discovery only
// tries this default — if an upstream ClashForge instance was installed on a
// custom port, discovery simply finds nothing and SelectFakeIPRange falls
// back to its IANA-only check, same as before this feature existed.
const discoveryPort = 7777

// defaultGatewayIP reads /proc/net/route (Linux-only, present on every
// OpenWrt target ClashForge ships for) to find the router's own upstream
// gateway — the candidate address for "is my upstream also ClashForge".
// Returns "" if there's no default route or the platform doesn't have
// /proc/net/route (e.g. local development on Windows/macOS); callers must
// treat that as "skip discovery", not an error.
func defaultGatewayIP() string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	return parseDefaultGatewayFromRouteTable(data)
}

// parseDefaultGatewayFromRouteTable extracts the gateway IP from the
// contents of /proc/net/route. Split out from defaultGatewayIP so the
// parsing logic is testable without a real Linux routing table.
func parseDefaultGatewayFromRouteTable(routeTable []byte) string {
	lines := strings.Split(string(routeTable), "\n")
	if len(lines) < 2 {
		return ""
	}
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[1] != "00000000" {
			continue
		}
		b, err := hex.DecodeString(fields[2])
		if err != nil || len(b) != 4 {
			continue
		}
		// /proc/net/route stores the gateway as a little-endian uint32.
		return net.IPv4(b[3], b[2], b[1], b[0]).String()
	}
	return ""
}

// queryUpstreamDNSTopology calls GET <baseURL>/api/discovery/dns-topology
// and returns the reported DNS topology plus whether the response confirmed
// a fake-ip-running ClashForge instance. baseURL is a full "http://host:port"
// — split out as a parameter (rather than building it from a bare host here)
// so this is testable against an httptest.Server instead of a real upstream
// router; discoverUpstreamDNSTopology is what builds the real router's URL.
func queryUpstreamDNSTopology(baseURL string) (config.DNSTopology, bool) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(baseURL + "/api/discovery/dns-topology")
	if err != nil {
		return config.DNSTopology{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return config.DNSTopology{}, false
	}

	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			ClashForge bool               `json:"clashforge"`
			DNS        config.DNSTopology `json:"dns"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return config.DNSTopology{}, false
	}
	if !body.OK || !body.Data.ClashForge || body.Data.DNS.EnhancedMode != "fake-ip" || body.Data.DNS.FakeIPRange == "" {
		return config.DNSTopology{}, false
	}
	return body.Data.DNS, true
}

// discoverUpstreamDNSTopology best-effort queries the default gateway's
// GET /api/discovery/dns-topology — if it answers as another ClashForge
// instance running fake-ip, its fake-ip-range is persisted to
// config.UpstreamTopologyPath so SelectFakeIPRange can avoid colliding with
// it precisely, instead of only guessing from the IANA-reserved baseline.
// Failure at any step (no gateway, unreachable, not ClashForge, redir-host
// mode, malformed response) is silent and non-fatal — this is a nice-to-have
// refinement, not a requirement for apply to succeed.
func discoverUpstreamDNSTopology(dataDir string) {
	gw := defaultGatewayIP()
	if gw == "" {
		return
	}
	topo, ok := queryUpstreamDNSTopology(fmt.Sprintf("http://%s:%d", gw, discoveryPort))
	if !ok {
		return
	}

	payload, err := json.Marshal(map[string]string{"fake_ip_range": topo.FakeIPRange})
	if err != nil {
		return
	}
	if err := os.WriteFile(config.UpstreamTopologyPath(dataDir), payload, 0o644); err != nil {
		log.Warn().Err(err).Msg("config: 写入上游 fake-ip 拓扑缓存失败")
		return
	}
	log.Info().
		Str("upstream_gateway", gw).
		Str("upstream_fake_ip_range", topo.FakeIPRange).
		Msg("config: 检测到上游也是 ClashForge（fake-ip 模式），已记录其网段用于避免选段冲突")
}
