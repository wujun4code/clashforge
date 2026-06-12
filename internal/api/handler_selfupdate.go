package api

import (
	"encoding/json"
	"net/http"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/selfupdate"
)

func handleGetSelfUpdateConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := deps.Config.Update
		var lastRun interface{}
		if deps.SelfUpdater != nil {
			lastRun = deps.SelfUpdater.LastResult()
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"auto_self_update":    u.AutoSelfUpdate,
			"self_update_time":    u.SelfUpdateTime,
			"self_update_channel": u.SelfUpdateChannel,
			"is_running":          deps.SelfUpdater != nil && deps.SelfUpdater.IsRunning(),
			"last_run":            lastRun,
		})
	}
}

func handleUpdateSelfUpdateConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AutoSelfUpdate    *bool  `json:"auto_self_update"`
			SelfUpdateTime   string `json:"self_update_time"`
			SelfUpdateChannel string `json:"self_update_channel"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			Err(w, http.StatusBadRequest, "PARSE_ERROR", err.Error())
			return
		}

		cfg := deps.Config
		if body.AutoSelfUpdate != nil {
			cfg.Update.AutoSelfUpdate = *body.AutoSelfUpdate
		}
		if body.SelfUpdateTime != "" {
			// Validate format.
			h, m := selfupdate.ParseUpdateTime(body.SelfUpdateTime)
			_ = h
			_ = m
			cfg.Update.SelfUpdateTime = body.SelfUpdateTime
		}
		if body.SelfUpdateChannel == "stable" || body.SelfUpdateChannel == "preview" {
			cfg.Update.SelfUpdateChannel = body.SelfUpdateChannel
		}

		if err := config.Save(deps.ConfigPath, cfg); err != nil {
			Err(w, http.StatusInternalServerError, "SAVE_ERROR", err.Error())
			return
		}
		if deps.SelfUpdater != nil {
			deps.SelfUpdater.UpdateConfig(cfg)
		}
		JSON(w, http.StatusOK, map[string]bool{"updated": true})
	}
}

func handleTriggerSelfUpdate(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.SelfUpdater == nil {
			Err(w, http.StatusServiceUnavailable, "NOT_AVAILABLE", "self-updater not initialized")
			return
		}
		started := deps.SelfUpdater.TriggerAsync()
		if !started {
			Err(w, http.StatusConflict, "ALREADY_RUNNING", "self-update is already in progress")
			return
		}
		JSON(w, http.StatusAccepted, map[string]bool{"started": true})
	}
}
