package config

import (
	"gopkg.in/yaml.v3"
)

// DeepMerge 将 src 深度合并到 dst，返回新 map
// 合并规则按 fieldName 分别处理
func DeepMerge(dst, src map[string]interface{}) map[string]interface{} {
	if dst == nil {
		dst = map[string]interface{}{}
	}
	result := make(map[string]interface{}, len(dst))
	for k, v := range dst {
		result[k] = v
	}
	for k, srcVal := range src {
		dstVal, exists := result[k]
		if !exists {
			result[k] = srcVal
			continue
		}
		result[k] = mergeField(k, dstVal, srcVal)
	}
	return result
}

func mergeField(key string, dst, src interface{}) interface{} {
	srcMap, srcIsMap := toStringMap(src)
	dstMap, dstIsMap := toStringMap(dst)
	if srcIsMap && dstIsMap {
		return DeepMerge(dstMap, srcMap)
	}

	srcSlice, srcIsSlice := toSlice(src)
	dstSlice, dstIsSlice := toSlice(dst)
	if srcIsSlice && dstIsSlice {
		switch key {
		case "proxies", "nameserver", "fallback", "default-nameserver":
			// append src to dst
			return append(dstSlice, srcSlice...)
		case "proxy-groups":
			return mergeProxyGroups(dstSlice, srcSlice)
		case "rules":
			// src inserted at front
			return append(srcSlice, dstSlice...)
		default:
			// src replaces dst
			return srcSlice
		}
	}

	return src
}

func mergeProxyGroups(dst, src []interface{}) []interface{} {
	byName := map[string]int{}
	result := make([]interface{}, len(dst))
	copy(result, dst)
	for i, item := range result {
		if m, ok := toStringMap(item); ok {
			if name, ok := m["name"].(string); ok {
				byName[name] = i
			}
		}
	}
	for _, item := range src {
		m, ok := toStringMap(item)
		if !ok {
			result = append(result, item)
			continue
		}
		name, _ := m["name"].(string)
		if idx, found := byName[name]; found {
			result[idx] = item
		} else {
			result = append(result, item)
		}
	}
	return result
}

func toStringMap(v interface{}) (map[string]interface{}, bool) {
	if m, ok := v.(map[string]interface{}); ok {
		return m, true
	}
	return nil, false
}

func toSlice(v interface{}) ([]interface{}, bool) {
	if s, ok := v.([]interface{}); ok {
		return s, true
	}
	return nil, false
}

// DeepMergeAny merges src into dst for ClashForge TOML config patches.
// Unlike DeepMerge (which appends certain slices for YAML config merging),
// this always replaces slices so that config fields like "fallback" and
// "nameservers" don't accumulate duplicates on every wizard save.
func DeepMergeAny(dst, src map[string]interface{}) map[string]interface{} {
	if dst == nil {
		dst = map[string]interface{}{}
	}
	result := make(map[string]interface{}, len(dst))
	for k, v := range dst {
		result[k] = v
	}
	for k, srcVal := range src {
		dstVal, exists := result[k]
		if !exists {
			result[k] = srcVal
			continue
		}
		srcMap, srcIsMap := toStringMap(srcVal)
		dstMap, dstIsMap := toStringMap(dstVal)
		if srcIsMap && dstIsMap {
			result[k] = DeepMergeAny(dstMap, srcMap)
			continue
		}
		// For slices in config patches, always replace (never append).
		result[k] = srcVal
	}
	return result
}

// MergeWithOverrides 将 overrides YAML 内容深度合并到 generated map
func MergeWithOverrides(generated map[string]interface{}, overridesYAML []byte) (map[string]interface{}, error) {
	if len(overridesYAML) == 0 {
		return generated, nil
	}
	var overrides map[string]interface{}
	if err := yaml.Unmarshal(overridesYAML, &overrides); err != nil {
		return nil, err
	}
	if overrides == nil {
		return generated, nil
	}
	return DeepMerge(generated, overrides), nil
}
