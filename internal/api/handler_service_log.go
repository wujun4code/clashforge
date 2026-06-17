package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func handleGetServiceLog(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		maxLines := 1000
		if s := r.URL.Query().Get("lines"); s != "" {
			if v, err := strconv.Atoi(s); err == nil && v > 0 && v <= 10000 {
				maxLines = v
			}
		}

		path := deps.LogFilePath
		if path == "" {
			// No disk file: fall back to in-memory LogBuffer (service events),
			// re-serialized as NDJSON lines so ServiceLogPanel can parse them.
			lines := []string{}
			if deps.LogBuffer != nil {
				entries := deps.LogBuffer.Recent(maxLines)
				for _, e := range entries {
					m := map[string]any{
						"level":   e.Level,
						"message": e.Msg,
						"time":    e.Ts,
					}
					for k, v := range e.Fields {
						m[k] = v
					}
					b, err := json.Marshal(m)
					if err == nil {
						lines = append(lines, string(b))
					}
				}
			}
			JSON(w, http.StatusOK, map[string]any{
				"lines":      lines,
				"file":       "",
				"size_bytes": 0,
				"warning":    "日志文件路径未配置，以下为内存缓冲（重启后丢失）。请在「高级管理 → 日志」中设置 log.file 后重启服务",
			})
			return
		}

		info, statErr := os.Stat(path)
		if statErr != nil {
			JSON(w, http.StatusOK, map[string]any{
				"lines":      []string{},
				"file":       path,
				"size_bytes": 0,
				"warning":    "日志文件尚未生成（服务首次启动后将自动创建）",
			})
			return
		}

		data, err := os.ReadFile(path)
		if err != nil {
			JSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		raw := strings.TrimRight(string(data), "\n")
		lines := []string{}
		if raw != "" {
			lines = strings.Split(raw, "\n")
			if len(lines) > maxLines {
				lines = lines[len(lines)-maxLines:]
			}
		}

		JSON(w, http.StatusOK, map[string]any{
			"lines":      lines,
			"file":       path,
			"size_bytes": info.Size(),
		})
	}
}

func handleClearServiceLog(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.LogFilePath == "" {
			// No disk file: clear the in-memory LogBuffer instead
			if deps.LogBuffer != nil {
				deps.LogBuffer.Clear()
			}
			JSON(w, http.StatusOK, map[string]any{"ok": true, "warning": "日志文件路径未配置，已清空内存缓冲"})
			return
		}
		if err := os.Truncate(deps.LogFilePath, 0); err != nil {
			if os.IsNotExist(err) {
				JSON(w, http.StatusOK, map[string]any{"ok": true})
				return
			}
			JSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		JSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
