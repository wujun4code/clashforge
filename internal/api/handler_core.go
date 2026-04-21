package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/wujun4code/clashforge/internal/core"
)

func handleCoreStart(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := deps.Core.Start(ctx); err != nil {
			if errors.Is(err, core.ErrAlreadyRunning) {
				Err(w, http.StatusConflict, "CORE_ALREADY_RUNNING", err.Error())
				return
			}
			Err(w, http.StatusInternalServerError, "CORE_START_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]int{"pid": deps.Core.Status().PID})
	}
}

func handleCoreStop(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := deps.Core.Stop(); err != nil {
			if errors.Is(err, core.ErrNotRunning) {
				Err(w, http.StatusNotFound, "CORE_NOT_RUNNING", err.Error())
				return
			}
			Err(w, http.StatusInternalServerError, "CORE_STOP_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]any{"stopped": true})
	}
}

func handleCoreRestart(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := deps.Core.Restart(ctx); err != nil {
			Err(w, http.StatusInternalServerError, "CORE_RESTART_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]int{"pid": deps.Core.Status().PID})
	}
}

func handleCoreReload(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := deps.Core.Reload(deps.Core.Status().ConfigFile); err != nil {
			Err(w, http.StatusBadRequest, "CONFIG_INVALID", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]any{"reloaded": true})
	}
}

func handleCoreVersion(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		JSON(w, http.StatusOK, map[string]any{
			"current":      deps.Core.CurrentVersion(ctx),
			"latest":       "",
			"has_update":   false,
			"download_url": "",
		})
	}
}
