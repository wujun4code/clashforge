package api

import (
	"net/http"
	"strconv"
)

func handleGetLogs(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 500
		if s := r.URL.Query().Get("limit"); s != "" {
			if v, err := strconv.Atoi(s); err == nil && v > 0 && v <= 2000 {
				limit = v
			}
		}
		var logs []RequestLogEntry
		if deps.RequestLogBuffer != nil {
			logs = deps.RequestLogBuffer.Recent(limit)
		}
		if logs == nil {
			logs = []RequestLogEntry{}
		}
		JSON(w, http.StatusOK, map[string]any{"logs": logs})
	}
}

func handleClearLogs(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.RequestLogBuffer != nil {
			deps.RequestLogBuffer.Clear()
		}
		JSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handlePauseLogs(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.RequestLogBuffer != nil {
			deps.RequestLogBuffer.Pause()
		}
		JSON(w, http.StatusOK, map[string]any{"ok": true, "paused": true})
	}
}

func handleResumeLogs(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.RequestLogBuffer != nil {
			deps.RequestLogBuffer.Resume()
		}
		JSON(w, http.StatusOK, map[string]any{"ok": true, "paused": false})
	}
}

func handleLogsStatus(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		paused := false
		if deps.RequestLogBuffer != nil {
			paused = deps.RequestLogBuffer.Paused()
		}
		JSON(w, http.StatusOK, map[string]any{"paused": paused})
	}
}
