package api

import "strings"

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

func sourceKeyFromActiveSource(as *ActiveSource) string {
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
