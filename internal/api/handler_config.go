package api

import (
	"encoding/json"
	"fmt"
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
		if newCfg.Security.APISecret == "***" && deps.Config.Security.APISecret != "" {
			newCfg.Security.APISecret = deps.Config.Security.APISecret
		}
		if err := config.Save(deps.ConfigPath, &newCfg); err != nil {
			Err(w, http.StatusInternalServerError, "CONFIG_WRITE_FAILED", err.Error())
			return
		}
		*deps.Config = newCfg
		refreshNetfilterManager(deps)
		generated, genErr := generateMihomoConfig(deps)
		if genErr != nil {
			JSON(w, http.StatusOK, map[string]interface{}{
				"updated":          true,
				"config_generated": false,
				"needs_restart":    true,
				"warning":          genErr.Error(),
			})
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"updated":          true,
			"config_generated": generated,
			"needs_restart":    true,
		})
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
		// Auto-generate mihomo config after overrides update
		generated, err := generateMihomoConfig(deps)
		if err != nil {
			// Non-fatal: config saved but generation failed
			JSON(w, http.StatusOK, map[string]interface{}{
				"updated":         true,
				"config_generated": false,
				"warning":         err.Error(),
			})
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"updated":         true,
			"config_generated": generated,
		})
	}
}

// handleGenerateConfig generates the mihomo YAML from current config + subscriptions + overrides
func handleGenerateConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		generated, err := generateMihomoConfig(deps)
		if err != nil {
			Err(w, http.StatusInternalServerError, "CONFIG_GENERATE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"generated":   generated,
			"config_file": deps.Config.Core.RuntimeDir + "/mihomo-config.yaml",
		})
	}
}

// generateMihomoConfig generates and writes the mihomo config, returns true if successful
func generateMihomoConfig(deps Dependencies) (bool, error) {
	if deps.SubManager == nil {
		return false, nil
	}
	nodes := deps.SubManager.GetAllCachedNodes()

	generated, err := config.Generate(deps.Config, nodes)
	if err != nil {
		return false, err
	}

	// Load overrides
	overridesPath := deps.Config.Core.DataDir + "/overrides.yaml"
	overridesData, _ := os.ReadFile(overridesPath)

	merged, err := config.MergeWithOverrides(generated, overridesData)
	if err != nil {
		return false, err
	}

	// Always enforce ports from config.toml — do not allow overrides to steal ports
	merged["port"] = deps.Config.Ports.HTTP
	merged["socks-port"] = deps.Config.Ports.SOCKS
	merged["mixed-port"] = deps.Config.Ports.Mixed
	merged["redir-port"] = deps.Config.Ports.Redir
	merged["tproxy-port"] = deps.Config.Ports.TProxy
	merged["external-controller"] = fmt.Sprintf("127.0.0.1:%d", deps.Config.Ports.MihomoAPI)

	data, err := config.MarshalYAML(merged)
	if err != nil {
		return false, err
	}

	outPath := deps.Config.Core.RuntimeDir + "/mihomo-config.yaml"
	if err := os.MkdirAll(deps.Config.Core.RuntimeDir, 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return false, err
	}
	return true, nil
}
