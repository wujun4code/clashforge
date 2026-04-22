package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/netfilter"
	"github.com/wujun4code/clashforge/internal/subscription"
)

// Dependencies holds all injected services.
type Dependencies struct {
	Version       string
	StartedAt     time.Time
	ConfigPath    string
	Config        *config.MetaclashConfig
	Core          *core.CoreManager
	SubManager    *subscription.Manager
	Netfilter     *netfilter.Manager
	SSEBroker     *SSEBroker
}

// NewRouter builds the HTTP router with all routes registered.
func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RealIP)
	r.Use(recoverMiddleware)
	r.Use(loggerMiddleware)
	r.Use(corsMiddleware)

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(authMiddleware(deps.Config.Security.APISecret))

		// Status
		api.Get("/status", handleStatus(deps))

		// Config
		api.Get("/config", handleGetConfig(deps))
		api.Put("/config", handleUpdateConfig(deps))
		api.Get("/config/mihomo", handleGetMihomoConfig(deps))
		api.Get("/config/overrides", handleGetOverrides(deps))
		api.Put("/config/overrides", handleUpdateOverrides(deps))

		// Core management
		api.Post("/core/start", handleCoreStart(deps))
		api.Post("/core/stop", handleCoreStop(deps))
		api.Post("/core/restart", handleCoreRestart(deps))
		api.Post("/core/reload", handleCoreReload(deps))
		api.Get("/core/version", handleCoreVersion(deps))

		// Subscriptions
		api.Get("/subscriptions", handleGetSubscriptions(deps))
		api.Post("/subscriptions", handleAddSubscription(deps))
		api.Post("/subscriptions/update-all", handleTriggerUpdateAll(deps))
		api.Put("/subscriptions/{id}", handleUpdateSubscription(deps))
		api.Delete("/subscriptions/{id}", handleDeleteSubscription(deps))
		api.Post("/subscriptions/{id}/update", handleTriggerSubscriptionUpdate(deps))

		// Real-time events
		if deps.SSEBroker != nil {
			api.Get("/events", deps.SSEBroker.Handler())
		}
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return r
}
