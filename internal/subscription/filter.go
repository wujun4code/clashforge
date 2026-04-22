package subscription

import (
	"fmt"
	"strings"
)

// ApplyFilter filters and deduplicates nodes according to the given filter config.
func ApplyFilter(nodes []ProxyNode, f SubscriptionFilter) []ProxyNode {
	var result []ProxyNode
	for _, node := range nodes {
		lower := strings.ToLower(node.Name)
		excluded := false
		for _, kw := range f.Exclude {
			if strings.Contains(lower, strings.ToLower(kw)) {
				excluded = true
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
				continue
			}
		}
		result = append(result, node)
	}

	// Deduplicate by server:port
	seen := map[string]bool{}
	deduped := result[:0]
	for _, node := range result {
		key := fmt.Sprintf("%s:%d", node.Server, node.Port)
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, node)
		}
	}

	if f.MaxNodes > 0 && len(deduped) > f.MaxNodes {
		deduped = deduped[:f.MaxNodes]
	}
	return deduped
}
