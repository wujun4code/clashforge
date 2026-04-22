package api

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/wujun4code/clashforge/internal/config"
)

func handleGetConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, deps.Config.Redacted())
	}
}

func handleUpdateConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var patch map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			Err(w, http.StatusBadRequest, "CONFIG_PARSE_ERROR", err.Error())
			return
		}
		// Simple partial update via JSON round-trip
		data, err := json.Marshal(deps.Config)
		if err != nil {
			Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		var current map[string]interface{}
		if err := json.Unmarshal(data, &current); err != nil {
			Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		merged := config.DeepMergeAny(current, patch)
		mergedData, err := json.Marshal(merged)
		if err != nil {
			Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		var newCfg config.MetaclashConfig
		if err := json.Unmarshal(mergedData, &newCfg); err != nil {
			Err(w, http.StatusBadRequest, "CONFIG_PARSE_ERROR", err.Error())
			return
		}
		if err := config.ValidateStruct(&newCfg); err != nil {
			Err(w, http.StatusBadRequest, "CONFIG_INVALID", err.Error())
			return
		}
		*deps.Config = newCfg
		JSON(w, http.StatusOK, map[string]bool{"updated": true})
	}
}

func handleGetMihomoConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := deps.Config.Core.RuntimeDir + "/mihomo-config.yaml"
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				JSON(w, http.StatusOK, map[string]string{"content": ""})
				return
			}
			Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]string{"content": string(data)})
	}
}

func handleGetOverrides(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := deps.Config.Core.DataDir + "/overrides.yaml"
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				JSON(w, http.StatusOK, map[string]string{"content": ""})
				return
			}
			Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]string{"content": string(data)})
	}
}

func handleUpdateOverrides(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			Err(w, http.StatusBadRequest, "CONFIG_PARSE_ERROR", err.Error())
			return
		}
		if err := config.ValidateYAML([]byte(body.Content)); err != nil {
			Err(w, http.StatusBadRequest, "YAML_PARSE_ERROR", err.Error())
			return
		}
		path := deps.Config.Core.DataDir + "/overrides.yaml"
		if err := os.WriteFile(path, []byte(body.Content), 0o644); err != nil {
			Err(w, http.StatusInternalServerError, "CONFIG_WRITE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]bool{"updated": true})
	}
}
