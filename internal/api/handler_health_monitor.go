package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func handleHealthSummary(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.HealthMonitor == nil {
			Err(w, http.StatusServiceUnavailable, "HEALTH_MONITOR_DISABLED", "health monitor is not initialized")
			return
		}
		JSON(w, http.StatusOK, deps.HealthMonitor.Summary())
	}
}

func handleHealthIncidents(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.HealthMonitor == nil {
			Err(w, http.StatusServiceUnavailable, "HEALTH_MONITOR_DISABLED", "health monitor is not initialized")
			return
		}
		limit := 50
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		JSON(w, http.StatusOK, map[string]any{"incidents": deps.HealthMonitor.Incidents(limit)})
	}
}

func handleHealthBrowserReport(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.HealthMonitor == nil {
			Err(w, http.StatusServiceUnavailable, "HEALTH_MONITOR_DISABLED", "health monitor is not initialized")
			return
		}
		var req healthBrowserReportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "HEALTH_BROWSER_REPORT_PARSE_FAILED", err.Error())
			return
		}
		if strings.TrimSpace(req.UserAgent) == "" {
			req.UserAgent = r.UserAgent()
		}
		browser, err := deps.HealthMonitor.SubmitBrowserReport(req)
		if err != nil {
			Err(w, http.StatusInternalServerError, "HEALTH_BROWSER_REPORT_SAVE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"browser": browser,
			"summary": deps.HealthMonitor.Summary(),
		})
	}
}
