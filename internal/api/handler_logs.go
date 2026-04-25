package api

import (
	"net/http"
	"strconv"
)

func handleGetLogs(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 200
		if s := r.URL.Query().Get("limit"); s != "" {
			if v, err := strconv.Atoi(s); err == nil && v > 0 && v <= 1000 {
				limit = v
			}
		}
		var logs []LogEntry
		if deps.LogBuffer != nil {
			logs = deps.LogBuffer.Recent(limit)
		}
		if logs == nil {
			logs = []LogEntry{}
		}
		JSON(w, http.StatusOK, map[string]any{"logs": logs})
	}
}

func handleClearLogs(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.LogBuffer != nil {
			deps.LogBuffer.Clear()
		}
		JSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handlePauseLogs(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.LogBuffer != nil {
			deps.LogBuffer.Pause()
		}
		JSON(w, http.StatusOK, map[string]any{"ok": true, "paused": true})
	}
}

func handleResumeLogs(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.LogBuffer != nil {
			deps.LogBuffer.Resume()
		}
		JSON(w, http.StatusOK, map[string]any{"ok": true, "paused": false})
	}
}

func handleLogsStatus(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		paused := false
		if deps.LogBuffer != nil {
			paused = deps.LogBuffer.Paused()
		}
		JSON(w, http.StatusOK, map[string]any{"paused": paused})
	}
}
