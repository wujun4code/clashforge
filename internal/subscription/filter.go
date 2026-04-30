package subscription

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

// ApplyFilter filters and deduplicates nodes according to the given filter config.
func ApplyFilter(nodes []ProxyNode, f SubscriptionFilter) []ProxyNode {
	log.Info().
		Int("input_nodes", len(nodes)).
		Strs("exclude_keywords", f.Exclude).
		Strs("include_keywords", f.Include).
		Int("max_nodes", f.MaxNodes).
		Msg("subscription: 开始过滤节点")

	var result []ProxyNode
	excludedCount := 0
	notIncludedCount := 0
	for _, node := range nodes {
		lower := strings.ToLower(node.Name)
		excluded := false
		for _, kw := range f.Exclude {
			if strings.Contains(lower, strings.ToLower(kw)) {
				excluded = true
				excludedCount++
				break
			}
		}
		if excluded {
			continue
		}
		if len(f.Include) > 0 {
			matched := false
			for _, kw := range f.Include {
				if strings.Contains(lower, strings.ToLower(kw)) {
					matched = true
					break
				}
			}
			if !matched {
				notIncludedCount++
				continue
			}
		}
		result = append(result, node)
	}

	// Deduplicate by server:port
	seen := map[string]bool{}
	deduped := result[:0]
	dedupCount := 0
	for _, node := range result {
		key := fmt.Sprintf("%s:%d", node.Server, node.Port)
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, node)
		} else {
			dedupCount++
		}
	}

	if f.MaxNodes > 0 && len(deduped) > f.MaxNodes {
		trimmed := len(deduped) - f.MaxNodes
		deduped = deduped[:f.MaxNodes]
		log.Info().
			Int("trimmed", trimmed).
			Msg("subscription: 超过 max_nodes 限制，已截断")
	}

	log.Info().
		Int("input_nodes", len(nodes)).
		Int("after_filter", len(result)).
		Int("after_dedup", len(deduped)).
		Int("excluded_by_keyword", excludedCount).
		Int("not_included_by_keyword", notIncludedCount).
		Int("deduped_duplicates", dedupCount).
		Msg("subscription: 节点过滤完成")

	if len(deduped) == 0 && len(nodes) > 0 {
		log.Warn().
			Int("original_count", len(nodes)).
			Msg("subscription: ⚠️ 过滤后节点数量为 0！所有国际流量将走 DIRECT，谷歌等境外网站将无法访问")
	}

	return deduped
}
