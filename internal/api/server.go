package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/netfilter"
	"github.com/wujun4code/clashforge/internal/subscription"
)

//go:embed ui_dist
var uiDist embed.FS

// Dependencies holds all injected services.
type Dependencies struct {
	Version    string
	StartedAt  time.Time
	ConfigPath string
	Config     *config.MetaclashConfig
	Core       *core.CoreManager
	SubManager *subscription.Manager
	Netfilter  *netfilter.Manager
	SSEBroker  *SSEBroker
	LogBuffer  *LogBuffer
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
		api.Get("/status", handleStatus(deps))
		api.Get("/overview", handleOverview(deps))
		api.Get("/overview/core", handleOverviewCore(deps))
		api.Get("/overview/probes", handleOverviewProbes(deps))
		api.Get("/overview/resources", handleOverviewResources(deps))
		api.Post("/overview/takeover", handleTakeoverOverviewModule(deps))
		api.Post("/overview/release", handleReleaseOverviewTakeover(deps))
		api.Get("/health/check", handleHealthCheck(deps))
		api.Get("/config", handleGetConfig(deps))
		api.Put("/config", handleUpdateConfig(deps))
		api.Get("/config/mihomo", handleGetMihomoConfig(deps))
		api.Get("/config/overrides", handleGetOverrides(deps))
		api.Put("/config/overrides", handleUpdateOverrides(deps))
		api.Post("/config/generate", handleGenerateConfig(deps))
		api.Get("/config/sources", handleGetSources(deps))
		api.Post("/config/sources", handleSaveSource(deps))
		api.Get("/config/sources/{filename}", handleGetSourceFile(deps))
		api.Delete("/config/sources/{filename}", handleDeleteSourceFile(deps))
		api.Get("/config/active-source", handleGetActiveSource(deps))
		api.Put("/config/active-source", handleSetActiveSource(deps))
		api.Post("/core/start", handleCoreStart(deps))
		api.Post("/core/stop", handleCoreStop(deps))
		api.Post("/core/restart", handleCoreRestart(deps))
		api.Post("/core/reload", handleCoreReload(deps))
		api.Get("/core/version", handleCoreVersion(deps))
		api.Post("/service/enable", handleServiceEnable(deps))
		api.Get("/subscriptions", handleGetSubscriptions(deps))
		api.Post("/subscriptions", handleAddSubscription(deps))
		api.Post("/subscriptions/update-all", handleTriggerUpdateAll(deps))
		api.Put("/subscriptions/{id}", handleUpdateSubscription(deps))
		api.Delete("/subscriptions/{id}", handleDeleteSubscription(deps))
		api.Post("/subscriptions/{id}/update", handleTriggerSubscriptionUpdate(deps))
		api.Post("/subscriptions/{id}/sync-update", handleSyncSubscriptionUpdate(deps))
		api.Get("/logs", handleGetLogs(deps))
		// Proxy pass-through to mihomo API
		api.Get("/proxies", proxyToMihomo(deps, "/proxies"))
		api.Put("/proxies/{group}/select", proxyMihomoWithParam(deps, "/proxies/", "", "group"))
		api.Get("/proxies/{name}/delay", proxyMihomoWithParam(deps, "/proxies/", "/delay", "name"))
		api.Get("/connections", proxyToMihomo(deps, "/connections"))
		api.Delete("/connections", proxyToMihomo(deps, "/connections"))
		// Rule providers
		api.Get("/rules/providers", handleGetRuleProviders(deps))
		api.Post("/rules/providers/sync-all", handleSyncAllRuleProviders(deps))
		api.Post("/rules/providers/{name}/sync", handleSyncRuleProvider(deps))
		api.Get("/rules/search", handleSearchRules(deps))
		if deps.SSEBroker != nil {
			api.Get("/events", deps.SSEBroker.Handler())
		}
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Serve embedded React SPA
	registerUI(r)

	return r
}

func registerUI(r *chi.Mux) {
	uiFS, err := fs.Sub(uiDist, "ui_dist")
	if err != nil {
		return
	}
	fileServer := http.FileServer(http.FS(uiFS))

	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/")

		f, err := uiFS.Open(path)
		if err == nil {
			f.Close()
			// Hashed assets (e.g. index-ABC123.js) can be cached forever.
			// Everything else (index.html, manifest, icons) must revalidate.
			if strings.HasPrefix(path, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			}
			fileServer.ServeHTTP(w, req)
			return
		}
		// SPA fallback: serve index.html, always revalidate
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		req.URL.Path = "/"
		fileServer.ServeHTTP(w, req)
	})
}
