package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wujun4code/clashforge/internal/workernode"
)

func handleListWorkerNodes(store *workernode.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]interface{}{"nodes": store.List()})
	}
}

func handleCreateWorkerNode(store *workernode.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req workernode.CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()

		node, err := workernode.Deploy(ctx, &req)
		if err != nil {
			Err(w, http.StatusBadGateway, "DEPLOY_FAILED", err.Error())
			return
		}

		if err := store.Create(node); err != nil {
			Err(w, http.StatusInternalServerError, "STORE_FAILED", err.Error())
			return
		}
		clashYAML, err := workernode.ExportClashProxy(node)
		if err != nil {
			Err(w, http.StatusInternalServerError, "EXPORT_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusCreated, map[string]interface{}{
			"node":         workernode.ToListItem(node),
			"clash_config": clashYAML,
		})
	}
}

func handleRedeployWorkerNode(store *workernode.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NOT_FOUND", "worker node not found")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()

		if err := workernode.Redeploy(ctx, node); err != nil {
			node.Status = workernode.StatusError
			node.Error = err.Error()
			_ = store.Update(id, node)
			Err(w, http.StatusBadGateway, "REDEPLOY_FAILED", err.Error())
			return
		}

		now := time.Now()
		node.Status = workernode.StatusDeployed
		node.Error = ""
		node.DeployedAt = &now
		if err := store.Update(id, node); err != nil {
			Err(w, http.StatusInternalServerError, "STORE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{"node": workernode.ToListItem(node)})
	}
}

func handleDeleteWorkerNode(store *workernode.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NOT_FOUND", "worker node not found")
			return
		}

		// Best-effort CF cleanup (don't fail the delete if CF call fails)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		_ = workernode.Destroy(ctx, node)

		if err := store.Delete(id); err != nil {
			Err(w, http.StatusInternalServerError, "STORE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func handleGetWorkerNodeClashConfig(store *workernode.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NOT_FOUND", "worker node not found")
			return
		}
		clashYAML, err := workernode.ExportClashProxy(node)
		if err != nil {
			Err(w, http.StatusInternalServerError, "EXPORT_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"yaml": clashYAML,
			"name": node.Name,
		})
	}
}

// handleGetWorkerNodeFreeTierInfo returns the AES key and /sub URL for CI use.
// The response contains secrets — protect this endpoint with appropriate auth.
func handleGetWorkerNodeFreeTierInfo(store *workernode.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NOT_FOUND", "worker node not found")
			return
		}
		if node.AesKey == "" {
			Err(w, http.StatusConflict, "NO_AES_KEY", "node has no AES key; redeploy to generate one")
			return
		}

		expiresAt := ""
		if node.ExpiresAt != nil {
			expiresAt = node.ExpiresAt.UTC().Format(time.RFC3339)
		}

		subURL := "https://" + node.Hostname + "/sub"
		JSON(w, http.StatusOK, workernode.FreeTierInfo{
			SubURL:    subURL,
			AesKey:    node.AesKey,
			ExpiresAt: expiresAt,
		})
	}
}

// handleRenewWorkerNodeExpiry re-deploys the worker with a new EXPIRES_AT binding.
func handleRenewWorkerNodeExpiry(store *workernode.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NOT_FOUND", "worker node not found")
			return
		}

		var req workernode.RenewExpiryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		if req.ExpiresInDays <= 0 {
			Err(w, http.StatusBadRequest, "INVALID_DAYS", "expires_in_days must be > 0")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()

		newExpiry, err := workernode.RenewExpiry(ctx, node, req.ExpiresInDays)
		if err != nil {
			Err(w, http.StatusBadGateway, "RENEW_FAILED", err.Error())
			return
		}

		node.ExpiresAt = &newExpiry
		if err := store.Update(id, node); err != nil {
			Err(w, http.StatusInternalServerError, "STORE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"node":       workernode.ToListItem(node),
			"expires_at": newExpiry.UTC().Format(time.RFC3339),
		})
	}
}
