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
