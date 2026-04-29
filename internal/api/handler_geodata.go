package api

import (
	"encoding/json"
	"net/http"

	"github.com/wujun4code/clashforge/internal/config"
)

func handleGetGeoDataStatus(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]interface{}{
			"files":      deps.GeoDataManager.FileStatuses(),
			"latest":     deps.GeoDataManager.LatestRecord(),
			"is_running": deps.GeoDataManager.IsRunning(),
		})
	}
}

func handleGetGeoDataConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := deps.Config.Update
		JSON(w, http.StatusOK, map[string]interface{}{
			"auto_geoip":       u.AutoGeoIP,
			"geoip_interval":   u.GeoIPInterval,
			"auto_geosite":     u.AutoGeosite,
			"geosite_interval": u.GeositeInterval,
			"proxy_server":     u.GeoDataProxyServer,
			"geoip_url":        u.GeoIPURL,
			"geosite_url":      u.GeositeURL,
		})
	}
}

func handleUpdateGeoDataConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AutoGeoIP       *bool  `json:"auto_geoip"`
			GeoIPInterval   string `json:"geoip_interval"`
			AutoGeosite     *bool  `json:"auto_geosite"`
			GeositeInterval string `json:"geosite_interval"`
			ProxyServer     string `json:"proxy_server"`
			GeoIPURL        string `json:"geoip_url"`
			GeositeURL      string `json:"geosite_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			Err(w, http.StatusBadRequest, "PARSE_ERROR", err.Error())
			return
		}
		cfg := deps.Config
		if body.AutoGeoIP != nil {
			cfg.Update.AutoGeoIP = *body.AutoGeoIP
		}
		if body.GeoIPInterval != "" {
			cfg.Update.GeoIPInterval = body.GeoIPInterval
		}
		if body.AutoGeosite != nil {
			cfg.Update.AutoGeosite = *body.AutoGeosite
		}
		if body.GeositeInterval != "" {
			cfg.Update.GeositeInterval = body.GeositeInterval
		}
		cfg.Update.GeoDataProxyServer = body.ProxyServer
		if body.GeoIPURL != "" {
			cfg.Update.GeoIPURL = body.GeoIPURL
		}
		if body.GeositeURL != "" {
			cfg.Update.GeositeURL = body.GeositeURL
		}
		if err := config.Save(deps.ConfigPath, cfg); err != nil {
			Err(w, http.StatusInternalServerError, "SAVE_ERROR", err.Error())
			return
		}
		deps.GeoDataManager.UpdateConfig(cfg)
		JSON(w, http.StatusOK, map[string]bool{"updated": true})
	}
}

func handleTriggerGeoDataUpdate(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ProxyServer string `json:"proxy_server"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		proxyServer := body.ProxyServer
		if proxyServer == "" {
			proxyServer = deps.Config.Update.GeoDataProxyServer
		}

		rec, started := deps.GeoDataManager.TriggerAsync(proxyServer)
		if !started {
			Err(w, http.StatusConflict, "ALREADY_RUNNING", "geodata update is already running")
			return
		}
		JSON(w, http.StatusAccepted, map[string]interface{}{
			"id":     rec.ID,
			"status": rec.Status,
		})
	}
}

func handleGetGeoDataLogs(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]interface{}{
			"records":    deps.GeoDataManager.Records(),
			"is_running": deps.GeoDataManager.IsRunning(),
		})
	}
}
