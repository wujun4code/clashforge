package api

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error().Interface("panic", rec).Bytes("stack", debug.Stack()).Msg("request panic")
				Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder wraps http.ResponseWriter to capture the response status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (sr *statusRecorder) WriteHeader(code int) {
	if !sr.wrote {
		sr.status = code
		sr.wrote = true
		sr.ResponseWriter.WriteHeader(code)
	}
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if !sr.wrote {
		sr.WriteHeader(http.StatusOK)
	}
	return sr.ResponseWriter.Write(b)
}

// Flush forwards to the underlying ResponseWriter's Flusher so SSE endpoints
// (setup launch, quickstart, /api/v1/events) keep working through this middleware.
func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// skipRequestLog returns true for high-frequency polling endpoints and static assets
// that would otherwise dominate the request log with noise.
func skipRequestLog(method, path string) bool {
	if method == http.MethodOptions {
		return true
	}
	// Only skip GET requests to known polling paths; mutations are always logged.
	if method != http.MethodGet {
		return false
	}
	switch path {
	case "/api/v1/logs", "/api/v1/logs/status",
		"/api/v1/service-log",
		"/api/v1/connections",
		"/api/v1/overview", "/api/v1/overview/core",
		"/api/v1/overview/resources", "/api/v1/overview/probes",
		"/api/v1/events",
		"/healthz":
		return true
	}
	return strings.HasPrefix(path, "/assets/") ||
		strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".css") ||
		strings.HasSuffix(path, ".ico") || strings.HasSuffix(path, ".png") ||
		strings.HasSuffix(path, ".svg") || strings.HasSuffix(path, ".woff2")
}

func loggerMiddleware(reqBuf *RequestLogBuffer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			if reqBuf != nil && !skipRequestLog(r.Method, r.URL.Path) {
				reqBuf.Add(RequestLogEntry{
					Method:     r.Method,
					Path:       r.URL.Path,
					Status:     rec.status,
					LatencyMs:  time.Since(start).Milliseconds(),
					RemoteAddr: r.RemoteAddr,
					Ts:         time.Now().Unix(),
				})
			}
		})
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func authMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if strings.TrimSpace(secret) == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := strings.TrimSpace(r.Header.Get("Authorization"))
			if auth == "" {
				auth = "Bearer " + strings.TrimSpace(r.URL.Query().Get("secret"))
			}
			expected := fmt.Sprintf("Bearer %s", secret)
			if auth == "Bearer" || auth == "Bearer " || auth == "" {
				Err(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authorization required")
				return
			}
			if auth != expected {
				Err(w, http.StatusForbidden, "AUTH_INVALID", "invalid authorization")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
