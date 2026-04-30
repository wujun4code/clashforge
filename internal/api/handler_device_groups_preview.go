package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
)

type deviceGroupsPreviewBody struct {
	DeviceGroups []config.DeviceGroup `json:"device_groups,omitempty"`
	SourceKey    string               `json:"source_key,omitempty"`
}

func handlePreviewDeviceGroupsConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body deviceGroupsPreviewBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			Err(w, http.StatusBadRequest, "CONFIG_PARSE_ERROR", err.Error())
			return
		}

		activeSourceKey := currentActiveSourceKey(deps.Config.Core.DataDir)
		sourceKey := strings.TrimSpace(body.SourceKey)
		if sourceKey == "" {
			sourceKey = strings.TrimSpace(r.URL.Query().Get("source_key"))
		}
		if sourceKey == "" {
			sourceKey = activeSourceKey
		}
		if sourceKey == "" {
			Err(w, http.StatusBadRequest, "SOURCE_KEY_REQUIRED", "source_key is required")
			return
		}

		groups := body.DeviceGroups
		if groups == nil {
			path := config.DeviceGroupsPath(deps.Config.Core.DataDir)
			loaded, err := config.LoadDeviceGroupsForSource(path, sourceKey)
			if err != nil {
				Err(w, http.StatusInternalServerError, "DEVICE_GROUPS_READ_FAILED", err.Error())
				return
			}
			groups = loaded
		}

		baseYAML, nodes, err := loadPreviewSourceYAML(deps, sourceKey)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				Err(w, http.StatusNotFound, "SOURCE_CACHE_NOT_FOUND", err.Error())
				return
			}
			Err(w, http.StatusBadRequest, "SOURCE_KEY_INVALID", err.Error())
			return
		}

		generated, err := config.GenerateFromBase(deps.Config, baseYAML, nodes)
		if err != nil {
			Err(w, http.StatusInternalServerError, "CONFIG_PREVIEW_FAILED", err.Error())
			return
		}
		generated = config.ApplyManagedRuntimeSettings(deps.Config, generated)
		generated, _ = config.ApplyPerDeviceSubRulesWithProviders(generated, groups)

		data, err := config.MarshalYAML(generated)
		if err != nil {
			Err(w, http.StatusInternalServerError, "CONFIG_PREVIEW_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"source_key":        sourceKey,
			"active_source_key": activeSourceKey,
			"profile_active":    sourceKey == "" || sourceKey == activeSourceKey,
			"content":           string(data),
		})
	}
}

func loadPreviewSourceYAML(deps Dependencies, sourceKey string) ([]byte, []subscription.ProxyNode, error) {
	switch {
	case strings.HasPrefix(sourceKey, sourceKeyFilePrefix):
		filename := strings.TrimSpace(strings.TrimPrefix(sourceKey, sourceKeyFilePrefix))
		if filename == "" || strings.Contains(filename, "/") || strings.Contains(filename, "..") || strings.Contains(filename, string(filepath.Separator)) {
			return nil, nil, errors.New("invalid file source key")
		}
		dir := sourcesDirPath(deps.Config.Core.DataDir)
		path := filepath.Join(dir, filename)
		clean := filepath.Clean(path)
		if !strings.HasPrefix(clean, filepath.Clean(dir)) {
			return nil, nil, errors.New("invalid file source key")
		}
		data, err := os.ReadFile(clean)
		if err != nil {
			return nil, nil, err
		}
		return data, nil, nil

	case strings.HasPrefix(sourceKey, sourceKeySubscriptionPrefix):
		subID := strings.TrimSpace(strings.TrimPrefix(sourceKey, sourceKeySubscriptionPrefix))
		if subID == "" {
			return nil, nil, errors.New("invalid subscription source key")
		}
		if deps.SubManager == nil {
			return nil, nil, errors.New("subscription manager unavailable")
		}
		if _, ok := deps.SubManager.GetByID(subID); !ok {
			return nil, nil, os.ErrNotExist
		}
		raw, err := deps.SubManager.GetRawYAML(subID)
		if err != nil {
			return nil, nil, err
		}
		nodes, err := deps.SubManager.GetCachedNodes(subID)
		if err != nil {
			nodes = nil
		} else {
			for i := range nodes {
				nodes[i].SourceSubID = subID
			}
		}
		return raw, nodes, nil
	}

	return nil, nil, errors.New("unsupported source key")
}
