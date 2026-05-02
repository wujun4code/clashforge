package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/wujun4code/clashforge/internal/dns"
)

type setupDNSProbeRequest struct {
	// Nameservers overrides the configured DNS nameservers for the probe.
	// When empty the server's configured DNS.Nameservers list is used.
	Nameservers []string `json:"nameservers"`
}

// handleSetupDNSProbe probes each configured (or request-supplied) nameserver
// to detect whether upstream DNS is returning fake-ip answers for proxy-node
// hostnames — a sign of transparent DNS hijacking by an upstream router.
//
// POST /api/v1/setup/dns-probe
func handleSetupDNSProbe(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setupDNSProbeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			Err(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}

		nameservers := req.Nameservers
		if len(nameservers) == 0 {
			nameservers = deps.Config.DNS.Nameservers
		}

		hostnames := extractProxyHostnames(deps)

		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		report := dns.ProbeNameservers(ctx, nameservers, hostnames)

		JSON(w, http.StatusOK, map[string]interface{}{
			"report":           report,
			"node_count":       len(hostnames),
			"nameserver_count": len(nameservers),
		})
	}
}
