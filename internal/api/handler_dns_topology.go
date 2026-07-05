package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/wujun4code/clashforge/internal/config"
)

// handleDNSTopology serves GET /api/discovery/dns-topology — deliberately
// registered outside the authenticated /api/v1 group (see server.go) since
// it exists specifically for a downstream ClashForge instance with no shared
// credential to query. It exposes ONLY the DNS enhanced-mode and fake-ip
// range actually in effect on this router (read from the generated
// mihomo-config.yaml, not just the config.toml choice, so it reflects any
// network-mode override). It must never return proxy nodes, subscription
// URLs/tokens, device-group rules, or anything else sensitive — if you're
// adding a field here, ask whether a stranger on the LAN should be able to
// read it without authentication.
func handleDNSTopology(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"clashforge": true,
			"version":    deps.Version,
		}
		if deps.Config == nil {
			JSON(w, http.StatusOK, resp)
			return
		}
		mihomoConfigPath := filepath.Join(deps.Config.Core.RuntimeDir, "mihomo-config.yaml")
		data, err := os.ReadFile(mihomoConfigPath)
		if err != nil {
			JSON(w, http.StatusOK, resp)
			return
		}
		topo := config.ExtractDNSTopology(data)
		resp["dns"] = topo
		JSON(w, http.StatusOK, resp)
	}
}
