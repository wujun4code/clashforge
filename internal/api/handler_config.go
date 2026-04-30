package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
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
		adjustments := config.SelectCompatiblePorts(&newCfg, compatibilityPortSelectionOptions(deps))
		if err := config.Save(deps.ConfigPath, &newCfg); err != nil {
			Err(w, http.StatusInternalServerError, "CONFIG_WRITE_FAILED", err.Error())
			return
		}
		*deps.Config = newCfg
		refreshNetfilterManager(deps)
		generated, genErr := generateMihomoConfig(deps)
		if genErr != nil {
			response := map[string]interface{}{
				"updated":          true,
				"config_generated": false,
				"needs_restart":    true,
				"warning":          genErr.Error(),
			}
			if len(adjustments) > 0 {
				response["port_adjustments"] = adjustments
			}
			JSON(w, http.StatusOK, response)
			return
		}
		response := map[string]interface{}{
			"updated":          true,
			"config_generated": generated,
			"needs_restart":    true,
		}
		if len(adjustments) > 0 {
			response["port_adjustments"] = adjustments
		}
		JSON(w, http.StatusOK, response)
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

		portAdjustments := config.SelectCompatiblePorts(deps.Config, compatibilityPortSelectionOptions(deps))
		if len(portAdjustments) > 0 {
			if err := config.Save(deps.ConfigPath, deps.Config); err != nil {
				Err(w, http.StatusInternalServerError, "CONFIG_WRITE_FAILED", err.Error())
				return
			}
			refreshNetfilterManager(deps)
		}

		// Auto-generate mihomo config after overrides update
		generated, err := generateMihomoConfig(deps)
		if err != nil {
			// Non-fatal: config saved but generation failed
			response := map[string]interface{}{
				"updated":          true,
				"config_generated": false,
				"warning":          err.Error(),
			}
			if len(portAdjustments) > 0 {
				response["port_adjustments"] = portAdjustments
			}
			JSON(w, http.StatusOK, response)
			return
		}
		response := map[string]interface{}{
			"updated":          true,
			"config_generated": generated,
		}
		if len(portAdjustments) > 0 {
			response["port_adjustments"] = portAdjustments
		}
		JSON(w, http.StatusOK, response)
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

func resolveGenerationInputs(deps Dependencies) ([]subscription.ProxyNode, [][]byte) {
	nodes := deps.SubManager.GetAllCachedNodes()
	rawYAMLs := deps.SubManager.GetRawYAMLForEnabled()
	if deps.Config == nil {
		return nodes, rawYAMLs
	}

	activeSource, err := readActiveSource(deps.Config.Core.DataDir)
	if err != nil || activeSource == nil {
		return nodes, rawYAMLs
	}

	switch strings.ToLower(strings.TrimSpace(activeSource.Type)) {
	case "file":
		// File source means overrides.yaml is the selected base, so do not fall back
		// to any enabled subscription raw YAML.
		rawYAMLs = nil
	case "subscription":
		subID := strings.TrimSpace(activeSource.SubID)
		if subID == "" {
			return nodes, rawYAMLs
		}
		// Prefer only the selected subscription to avoid cross-source pollution.
		selectedNodes, err := deps.SubManager.GetCachedNodes(subID)
		if err == nil {
			for i := range selectedNodes {
				selectedNodes[i].SourceSubID = subID
			}
			nodes = selectedNodes
		} else {
			nodes = nil
		}
		raw, err := deps.SubManager.GetRawYAML(subID)
		if err == nil {
			rawYAMLs = [][]byte{raw}
		} else {
			rawYAMLs = nil
		}
	}

	return nodes, rawYAMLs
}

// generateMihomoConfig generates and writes the mihomo config, returns true if successful
func generateMihomoConfig(deps Dependencies) (bool, error) {
	if deps.SubManager == nil {
		log.Warn().Msg("config-gen: SubManager 为空，无法生成配置")
		return false, nil
	}
	nodes, rawYAMLs := resolveGenerationInputs(deps)

	log.Info().
		Int("node_count", len(nodes)).
		Int("raw_yaml_count", len(rawYAMLs)).
		Msg("config-gen: 开始生成 mihomo 配置")

	if len(nodes) == 0 && len(rawYAMLs) == 0 {
		log.Warn().Msg("config-gen: ⚠️ 没有可用的代理节点和订阅 YAML！生成的配置将缺少代理，国际流量将走 DIRECT")
	}

	overridesPath := deps.Config.Core.DataDir + "/overrides.yaml"
	overridesData, _ := os.ReadFile(overridesPath)

	var generated map[string]interface{}
	var err error

	if len(rawYAMLs) > 0 {
		// Subscription with full YAML: use as base, preserving rules / proxy-groups /
		// rule-providers and everything else.  ClashForge only rewrites DNS and geodata.
		generated, err = config.GenerateFromBase(deps.Config, rawYAMLs[0], nodes)
		if err != nil {
			return false, err
		}
		// Allow user tweaks in overrides.yaml to win (e.g. custom rules prepended).
		if len(overridesData) > 0 {
			generated, err = config.MergeWithOverrides(generated, overridesData)
			if err != nil {
				return false, err
			}
		}
	} else if len(overridesData) > 0 {
		// No subscription YAML (paste / file import mode): the overrides IS the full
		// user config.  Use it as the base so rules and proxy-groups are preserved
		// exactly; ClashForge only injects DNS settings and managed ports.
		generated, err = config.GenerateFromBase(deps.Config, overridesData, nodes)
		if err != nil {
			return false, err
		}
	} else {
		// No user config at all: generate a minimal fallback from available nodes.
		generated, err = config.Generate(deps.Config, nodes)
		if err != nil {
			return false, err
		}
	}

	generated = config.ApplyManagedRuntimeSettings(deps.Config, generated)

	deviceGroupsPath := config.DeviceGroupsPath(deps.Config.Core.DataDir)
	sourceKey := currentActiveSourceKey(deps.Config.Core.DataDir)
	deviceGroups, err := config.LoadDeviceGroupsForSource(deviceGroupsPath, sourceKey)
	if err != nil {
		return false, err
	}
	generated, providerSpecs := config.ApplyPerDeviceSubRulesWithProviders(generated, deviceGroups)
	if err := config.SyncDeviceRuleProviderFiles(deps.Config.Core.RuntimeDir, providerSpecs); err != nil {
		return false, err
	}

	data, err := config.MarshalYAML(generated)
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

	log.Info().
		Str("path", outPath).
		Int("size", len(data)).
		Msg("config-gen: mihomo 配置文件已写入 ✓")

	return true, nil
}

func compatibilityPortSelectionOptions(deps Dependencies) config.PortSelectionOptions {
	options := config.PortSelectionOptions{PreferCommunityDefaults: true}
	if !deps.Core.Status().Ready || deps.Config == nil {
		return options
	}

	options.IgnoreOccupiedPorts = map[int]bool{
		deps.Config.Ports.HTTP:      true,
		deps.Config.Ports.SOCKS:     true,
		deps.Config.Ports.Mixed:     true,
		deps.Config.Ports.Redir:     true,
		deps.Config.Ports.TProxy:    true,
		deps.Config.Ports.DNS:       true,
		deps.Config.Ports.MihomoAPI: true,
	}
	return options
}
