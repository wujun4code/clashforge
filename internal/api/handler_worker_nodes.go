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
