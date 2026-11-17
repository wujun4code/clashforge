package runtimecfg

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
	"gopkg.in/yaml.v3"
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
	if strings.EqualFold(as.Type, "file") {
		return buildConfigSourceKey(as.Type, as.Filename)
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

func resolveGenerationInputs(cfg *config.MetaclashConfig, subMgr *subscription.Manager) ([]subscription.ProxyNode, [][]byte, string, error) {
	as, err := readActiveSource(cfg.Core.DataDir)
	if err != nil {
		// Absence is intentional only on a genuinely unconfigured installation.
		if !os.IsNotExist(err) || len(subMgr.GetAll()) != 0 {
			return nil, nil, "", errors.New("active source missing or corrupt; select source again")
		}
		if overrides, readErr := readGenerationOverrides(cfg.Core.DataDir, false); readErr != nil || len(overrides) != 0 {
			return nil, nil, "", errors.New("active source missing; select source again")
		}
		if _, statErr := os.Stat(filepath.Join(cfg.Core.DataDir, "device-groups.json")); !os.IsNotExist(statErr) {
			return nil, nil, "", errors.New("active source missing; select source again")
		}
		return nil, nil, "", nil
	}
	key := sourceKeyFromActiveSource(as)
	if key == "" || (as.Type != "file" && as.Type != "subscription") {
		return nil, nil, "", errors.New("active source invalid; select source again")
	}
	var nodes []subscription.ProxyNode
	var raws [][]byte
	if as.Type == "subscription" {
		sub, ok := subMgr.GetByID(as.SubID)
		if !ok || !sub.Enabled {
			return nil, nil, "", errors.New("selected subscription unavailable; select source again")
		}
		n, raw, err := subMgr.GenerationInput(as.SubID)
		if err != nil {
			return nil, nil, "", err
		}
		nodes = n
		if len(raw) > 0 {
			raws = append(raws, raw)
		}
	} else {
		// File input is independent; cached subscription nodes are optional
		// historical extras, not mandatory sources of its rules/groups.
		nodes = subMgr.GetAllCachedNodes()
	}
	return nodes, raws, key, nil
}

// readGenerationOverrides treats installer-created empty/comment-only files as
// absent optional overrides. File sources still require a nonempty YAML mapping.
func readGenerationOverrides(dataDir string, required bool) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, "overrides.yaml"))
	if err != nil {
		if os.IsNotExist(err) && !required {
			return nil, nil
		}
		return nil, errors.New("persistent overrides unavailable; select source again")
	}
	var root yaml.Node
	if yaml.Unmarshal(data, &root) == nil {
		if len(root.Content) == 0 && !required {
			return nil, nil
		}
		if len(root.Content) == 1 && root.Content[0].Kind == yaml.MappingNode {
			var mapping map[string]interface{}
			if yaml.Unmarshal(data, &mapping) == nil && (!required || len(mapping) > 0) {
				return data, nil
			}
		}
	}
	return nil, errors.New("persistent overrides invalid; select source again")
}

// GenerateAndWrite generates and writes runtime mihomo config using the same
// selection/merge rules as /setup and /config/generate.
func GenerateAndWrite(cfg *config.MetaclashConfig, subMgr *subscription.Manager) (bool, error) {
	if subMgr == nil {
		log.Warn().Msg("config-gen: SubManager 为空，无法生成配置")
		return false, nil
	}
	if cfg == nil {
		return false, errors.New("configuration unavailable")
	}
	nodes, rawYAMLs, sourceKey, err := resolveGenerationInputs(cfg, subMgr)
	if err != nil {
		return false, err
	}

	log.Info().
		Int("node_count", len(nodes)).
		Int("raw_yaml_count", len(rawYAMLs)).
		Msg("config-gen: 开始生成 mihomo 配置")

	if len(nodes) == 0 && len(rawYAMLs) == 0 {
		log.Warn().Msg("config-gen: ⚠️ 没有可用的代理节点和订阅 YAML！生成的配置将缺少代理，国际流量将走 DIRECT")
	}

	overridesData, err := readGenerationOverrides(cfg.Core.DataDir, strings.HasPrefix(sourceKey, sourceKeyFilePrefix))
	if err != nil {
		return false, err
	}

	var generated map[string]interface{}

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
	if err := config.ValidateRuntimeDNS(cfg, generated); err != nil {
		return false, err
	}

	deviceGroupsPath := config.DeviceGroupsPath(cfg.Core.DataDir)
	deviceGroups, err := config.LoadDeviceGroupsForSource(deviceGroupsPath, sourceKey)
	if err != nil {
		return false, errors.New("persistent device profiles unavailable or corrupt")
	}
	generated, providerSpecs := config.ApplyPerDeviceSubRulesWithProviders(generated, deviceGroups)
	if err := config.SyncDeviceRuleProviderFiles(cfg.Core.DataDir, providerSpecs); err != nil {
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
	// Must run before the overwrite below — it needs to read the *previous*
	// generation's fake-ip-range to detect a change.
	invalidateStaleFakeIPCache(cfg, outPath, fakeIPRangeFromGenerated(generated))
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return false, err
	}

	log.Info().
		Str("path", outPath).
		Int("size", len(data)).
		Msg("config-gen: mihomo 配置文件已写入 ✓")

	return true, nil
}

// fakeIPRangeFromGenerated reads dns.fake-ip-range out of the in-memory
// generated config map (avoids re-parsing YAML for the value we just built).
func fakeIPRangeFromGenerated(generated map[string]interface{}) string {
	dnsSection, ok := generated["dns"].(map[string]interface{})
	if !ok {
		return ""
	}
	v, _ := dnsSection["fake-ip-range"].(string)
	return v
}

// invalidateStaleFakeIPCache deletes Mihomo's persisted cache.db when the
// fake-ip-range is about to change from whatever the last-generated config
// used. Mihomo's "profile.store-fake-ip: true" persists fake-ip↔domain
// mappings across restarts for connection stability — but that means a
// domain already cached under the OLD range keeps its OLD (now out-of-range)
// fake-ip indefinitely after fake-ip-range changes, silently reproducing
// whatever collision the range change was meant to fix (e.g. the historical
// 198.18.0.0/16 default overlapping the lancidr rule-set — see SelectFakeIPRange).
// There is no partial-invalidation API for this, so the whole cache is
// dropped; the only user-visible cost is proxy-group selections resetting to
// their config default and DNS answers needing to be re-learned, both of
// which Mihomo does transparently on the next queries/connections.
func invalidateStaleFakeIPCache(cfg *config.MetaclashConfig, outPath, newRange string) {
	if newRange == "" {
		return
	}
	oldData, err := os.ReadFile(outPath)
	if err != nil {
		return // no previous generation to compare against (first run)
	}
	oldRange := config.ExtractFakeIPRange(oldData)
	if oldRange == "" || oldRange == newRange {
		return
	}
	cachePath := filepath.Join(cfg.Core.DataDir, "cache.db")
	if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Str("path", cachePath).Msg("config-gen: fake-ip-range 变更后清理 cache.db 失败")
		return
	}
	log.Warn().
		Str("old_range", oldRange).
		Str("new_range", newRange).
		Str("cache_path", cachePath).
		Msg("config-gen: fake-ip-range 已变更，已清空 Mihomo 的 cache.db——否则旧域名会继续沿用旧网段的 fake-ip，重新引发同样的冲突")
}
