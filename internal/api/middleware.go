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

func loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Info().Str("method", r.Method).Str("path", r.URL.Path).Dur("duration", time.Since(start)).Msg("http request")
	})
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
