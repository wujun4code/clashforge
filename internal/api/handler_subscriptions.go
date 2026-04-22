package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wujun4code/clashforge/internal/subscription"
)

func handleGetSubscriptions(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subs := deps.SubManager.GetAll()
		JSON(w, http.StatusOK, map[string]interface{}{"subscriptions": subs})
	}
}

func handleAddSubscription(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var sub subscription.Subscription
		if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
			Err(w, http.StatusBadRequest, "CONFIG_PARSE_ERROR", err.Error())
			return
		}
		if sub.Name == "" {
			Err(w, http.StatusBadRequest, "SUB_NAME_REQUIRED", "name is required")
			return
		}
		if sub.Type == "url" && sub.URL == "" {
			Err(w, http.StatusBadRequest, "SUB_URL_INVALID", "url is required for type=url")
			return
		}
		id, err := deps.SubManager.Add(sub)
		if err != nil {
			Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		JSON(w, http.StatusCreated, map[string]string{"id": id})
	}
}

func handleUpdateSubscription(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var patch map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			Err(w, http.StatusBadRequest, "CONFIG_PARSE_ERROR", err.Error())
			return
		}
		if err := deps.SubManager.Update(id, patch); err != nil {
			Err(w, http.StatusNotFound, "SUB_NOT_FOUND", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]bool{"updated": true})
	}
}

func handleDeleteSubscription(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := deps.SubManager.Delete(id); err != nil {
			Err(w, http.StatusNotFound, "SUB_NOT_FOUND", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

func handleTriggerSubscriptionUpdate(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := deps.SubManager.TriggerUpdate(id); err != nil {
			Err(w, http.StatusNotFound, "SUB_NOT_FOUND", err.Error())
			return
		}
		// Regenerate config after update completes (async)
		go func() {
			generateMihomoConfig(deps) //nolint:errcheck
		}()
		JSON(w, http.StatusAccepted, map[string]string{"message": "update started"})
	}
}

func handleTriggerUpdateAll(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = deps.SubManager.TriggerUpdateAll()
		go func() {
			generateMihomoConfig(deps) //nolint:errcheck
		}()
		JSON(w, http.StatusAccepted, map[string]string{"message": "update all started"})
	}
}
