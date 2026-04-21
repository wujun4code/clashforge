package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
)

type Dependencies struct {
	Version    string
	StartedAt  time.Time
	ConfigPath string
	Config     *config.MetaclashConfig
	Core       *core.CoreManager
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RealIP)
	r.Use(recoverMiddleware)
	r.Use(loggerMiddleware)
	r.Use(corsMiddleware)
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(authMiddleware(deps.Config.Security.APISecret))
		api.Get("/status", handleStatus(deps))
		api.Get("/config", handleGetConfig(deps))
		api.Post("/core/start", handleCoreStart(deps))
		api.Post("/core/stop", handleCoreStop(deps))
		api.Post("/core/restart", handleCoreRestart(deps))
		api.Post("/core/reload", handleCoreReload(deps))
		api.Get("/core/version", handleCoreVersion(deps))
	})
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return r
}
