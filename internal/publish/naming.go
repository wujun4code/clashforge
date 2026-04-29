package publish

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var nonFileNameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func SanitizeBaseName(raw string) string {
	name := strings.TrimSpace(raw)
	name = strings.TrimSuffix(name, ".yaml")
	name = strings.TrimSuffix(name, ".yml")
	if name == "" {
		name = "clash-config"
	}
	name = nonFileNameChars.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-.")
	if name == "" {
		name = "clash-config"
	}
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

func VersionedFileName(baseName string, version int, now time.Time) string {
	if version < 1 {
		version = 1
	}
	return fmt.Sprintf("%s.v%d.%s.yaml", SanitizeBaseName(baseName), version, now.Format("20060102"))
}

func PickWorkerBaseURL(workerURL, workerDevURL string) string {
	if strings.TrimSpace(workerURL) != "" {
		return strings.TrimRight(strings.TrimSpace(workerURL), "/")
	}
	return strings.TrimRight(strings.TrimSpace(workerDevURL), "/")
}
