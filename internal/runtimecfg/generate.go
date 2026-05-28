package runtimecfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
)

type activeSource struct {
	Type     string `json:"type"`               // "file" | "subscription"
	Filename string `json:"filename,omitempty"` // Type=file
	SubID    string `json:"sub_id,omitempty"`   // Type=subscription
}

func activeSourceFilePath(dataDir string) string {
	return filepath.Join(dataDir, "active_source.json")
}

func readActiveSource(dataDir string) (*activeSource, error) {
	data, err := os.ReadFile(activeSourceFilePath(dataDir))
	if err != nil {
		return nil, err
	}
	var as activeSource
	if err := json.Unmarshal(data, &as); err != nil {
		return nil, err
	}
	return &as, nil
}

const (
	sourceKeyFilePrefix         = "file:"
	sourceKeySubscriptionPrefix = "subscription:"
)

func buildConfigSourceKey(sourceType, sourceID string) string {
	id := strings.TrimSpace(sourceID)
	if id == "" {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(sourceType)) {
	case "file":
		return sourceKeyFilePrefix + id
	case "subscription":
		return sourceKeySubscriptionPrefix + id
	default:
		return ""
	}
}

func sourceKeyFromActiveSource(as *activeSource) string {
	if as == nil {
		return ""
	}
	if key := buildConfigSourceKey(as.Type, as.Filename); key != "" {
		return key
	}
	return buildConfigSourceKey(as.Type, as.SubID)
}

func currentActiveSourceKey(dataDir string) string {
	activeSource, err := readActiveSource(dataDir)
	if err != nil {
		return ""
	}
	return sourceKeyFromActiveSource(activeSource)
}

func resolveGenerationInputs(cfg *config.MetaclashConfig, subMgr *subscription.Manager) ([]subscription.ProxyNode, [][]byte) {
	nodes := subMgr.GetAllCachedNodes()
	rawYAMLs := subMgr.GetRawYAMLForEnabled()
	if cfg == nil {
		return nodes, rawYAMLs
	}

	activeSource, err := readActiveSource(cfg.Core.DataDir)
	if err != nil || activeSource == nil {
		return nodes, rawYAMLs
	}

	switch strings.ToLower(strings.TrimSpace(activeSource.Type)) {
	case "file":
		rawYAMLs = nil
	case "subscription":
		subID := strings.TrimSpace(activeSource.SubID)
		if subID == "" {
			return nodes, rawYAMLs
		}
		selectedNodes, err := subMgr.GetCachedNodes(subID)
		if err == nil {
			for i := range selectedNodes {
				selectedNodes[i].SourceSubID = subID
			}
			nodes = selectedNodes
		} else {
			nodes = nil
		}
		raw, err := subMgr.GetRawYAML(subID)
		if err == nil {
			rawYAMLs = [][]byte{raw}
		} else {
			rawYAMLs = nil
		}
	}

	return nodes, rawYAMLs
}

// GenerateAndWrite generates and writes runtime mihomo config using the same
// selection/merge rules as /setup and /config/generate.
func GenerateAndWrite(cfg *config.MetaclashConfig, subMgr *subscription.Manager) (bool, error) {
	if subMgr == nil {
		log.Warn().Msg("config-gen: SubManager 为空，无法生成配置")
		return false, nil
	}
	nodes, rawYAMLs := resolveGenerationInputs(cfg, subMgr)

	log.Info().
		Int("node_count", len(nodes)).
		Int("raw_yaml_count", len(rawYAMLs)).
		Msg("config-gen: 开始生成 mihomo 配置")

	if len(nodes) == 0 && len(rawYAMLs) == 0 {
		log.Warn().Msg("config-gen: ⚠️ 没有可用的代理节点和订阅 YAML！生成的配置将缺少代理，国际流量将走 DIRECT")
	}

	overridesPath := cfg.Core.DataDir + "/overrides.yaml"
	overridesData, _ := os.ReadFile(overridesPath)

	var generated map[string]interface{}
	var err error

	if len(rawYAMLs) > 0 {
		generated, err = config.GenerateFromBase(cfg, rawYAMLs[0], nodes)
		if err != nil {
			return false, err
		}
		if len(overridesData) > 0 {
			generated, err = config.MergeWithOverrides(generated, overridesData)
			if err != nil {
				return false, err
			}
		}
	} else if len(overridesData) > 0 {
		generated, err = config.GenerateFromBase(cfg, overridesData, nodes)
		if err != nil {
			return false, err
		}
	} else {
		generated, err = config.Generate(cfg, nodes)
		if err != nil {
			return false, err
		}
	}

	generated = config.ApplyManagedRuntimeSettings(cfg, generated)

	deviceGroupsPath := config.DeviceGroupsPath(cfg.Core.DataDir)
	sourceKey := currentActiveSourceKey(cfg.Core.DataDir)
	deviceGroups, err := config.LoadDeviceGroupsForSource(deviceGroupsPath, sourceKey)
	if err != nil {
		return false, err
	}
	generated, providerSpecs := config.ApplyPerDeviceSubRulesWithProviders(generated, deviceGroups)
	if err := config.SyncDeviceRuleProviderFiles(cfg.Core.RuntimeDir, providerSpecs); err != nil {
		return false, err
	}

	data, err := config.MarshalYAML(generated)
	if err != nil {
		return false, err
	}

	outPath := cfg.Core.RuntimeDir + "/mihomo-config.yaml"
	if err := os.MkdirAll(cfg.Core.RuntimeDir, 0o755); err != nil {
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
