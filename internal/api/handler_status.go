package api

import (
	"net/http"
	"time"
)

func handleStatus(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		coreStatus := deps.Core.Status()
		JSON(w, http.StatusOK, map[string]interface{}{
			"metaclash": map[string]interface{}{
				"version":     deps.Version,
				"uptime":      int64(time.Since(deps.StartedAt).Seconds()),
				"config_file": deps.ConfigPath,
			},
			"core": map[string]interface{}{
				"state":    coreStatus.State,
				"pid":      coreStatus.PID,
				"restarts": coreStatus.Restarts,
				"uptime":   coreStatus.Uptime,
			},
			"network": map[string]interface{}{
				"mode":             deps.Config.Network.Mode,
				"firewall_backend": deps.Config.Network.FirewallBackend,
				"rules_applied":    false,
			},
			"subscriptions": func() map[string]interface{} {
				total, enabled := 0, 0
				if deps.SubManager != nil {
					subs := deps.SubManager.GetAll()
					total = len(subs)
					for _, s := range subs {
						if s.Enabled {
							enabled++
						}
					}
				}
				return map[string]interface{}{
					"total":        total,
					"enabled":      enabled,
					"last_updated": nil,
				}
			}(),
		})
	}
}
