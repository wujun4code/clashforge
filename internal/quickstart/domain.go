package quickstart

import "strings"

// DomainMatchesZone reports whether domain belongs to zone.
// Example: "a.example.com" matches "example.com".
func DomainMatchesZone(domain, zone string) bool {
	domain = strings.ToLower(strings.Trim(strings.TrimSpace(domain), "."))
	zone = strings.ToLower(strings.Trim(strings.TrimSpace(zone), "."))
	if domain == "" || zone == "" {
		return false
	}
	return domain == zone || strings.HasSuffix(domain, "."+zone)
}

// ResolvePublishBaseDomain returns the base domain used by publish worker
// hostnames. It prefers an explicit Cloudflare zone; otherwise it falls back to
// removing the left-most label from the node host.
func ResolvePublishBaseDomain(nodeHost, zoneName string) string {
	nodeHost = strings.ToLower(strings.Trim(strings.TrimSpace(nodeHost), "."))
	zoneName = strings.ToLower(strings.Trim(strings.TrimSpace(zoneName), "."))

	if zoneName != "" && DomainMatchesZone(nodeHost, zoneName) {
		return zoneName
	}
	if zoneName != "" && strings.Count(zoneName, ".") >= 1 {
		return zoneName
	}

	if nodeHost != "" {
		parts := strings.Split(nodeHost, ".")
		if len(parts) >= 3 {
			return strings.Join(parts[1:], ".")
		}
		if len(parts) >= 2 {
			return nodeHost
		}
	}
	return zoneName
}

// BuildPublishHostname builds "<workerName>.<base-domain>" for publish workers.
func BuildPublishHostname(workerName, nodeHost, zoneName string) string {
	workerName = strings.ToLower(strings.Trim(strings.TrimSpace(workerName), "."))
	base := ResolvePublishBaseDomain(nodeHost, zoneName)
	if workerName == "" {
		return base
	}
	if base == "" {
		return workerName
	}
	return workerName + "." + base
}
