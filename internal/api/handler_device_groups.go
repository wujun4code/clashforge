package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
)

type deviceGroupsBody struct {
	DeviceGroups []config.DeviceGroup `json:"device_groups"`
	SourceKey    string               `json:"source_key,omitempty"`
}

func handleGetDeviceGroups(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestedSourceKey := strings.TrimSpace(r.URL.Query().Get("source_key"))
		activeSourceKey := currentActiveSourceKey(deps.Config.Core.DataDir)
		sourceKey := requestedSourceKey
		if sourceKey == "" {
			sourceKey = activeSourceKey
		}

		path := config.DeviceGroupsPath(deps.Config.Core.DataDir)
		groups, err := config.LoadDeviceGroupsForSource(path, sourceKey)
		if err != nil {
			Err(w, http.StatusInternalServerError, "DEVICE_GROUPS_READ_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"device_groups":       groups,
			"source_key":          sourceKey,
			"active_source_key":   activeSourceKey,
			"requested_by_source": requestedSourceKey != "",
		})
	}
}

func handlePutDeviceGroups(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			Err(w, http.StatusBadRequest, "CONFIG_PARSE_ERROR", err.Error())
			return
		}
		rawText := strings.TrimSpace(string(raw))
		if rawText == "" {
			Err(w, http.StatusBadRequest, "CONFIG_PARSE_ERROR", "request body is empty")
			return
		}

		var body deviceGroupsBody
		if decodeErr := json.Unmarshal(raw, &body); decodeErr != nil || body.DeviceGroups == nil {
			var plain []config.DeviceGroup
			if err2 := json.Unmarshal(raw, &plain); err2 != nil {
				msg := err2.Error()
				if decodeErr != nil {
					msg = decodeErr.Error()
				}
				Err(w, http.StatusBadRequest, "CONFIG_PARSE_ERROR", msg)
				return
			}
			body.DeviceGroups = plain
		}
		if body.SourceKey == "" {
			body.SourceKey = strings.TrimSpace(r.URL.Query().Get("source_key"))
		}

		activeSourceKey := currentActiveSourceKey(deps.Config.Core.DataDir)
		sourceKey := strings.TrimSpace(body.SourceKey)
		if sourceKey == "" {
			sourceKey = activeSourceKey
		}

		path := config.DeviceGroupsPath(deps.Config.Core.DataDir)
		if err := config.SaveDeviceGroupsForSource(path, body.DeviceGroups, sourceKey); err != nil {
			Err(w, http.StatusInternalServerError, "DEVICE_GROUPS_WRITE_FAILED", err.Error())
			return
		}

		profileActive := sourceKey == "" || sourceKey == activeSourceKey
		if !profileActive {
			JSON(w, http.StatusOK, map[string]interface{}{
				"updated":            true,
				"config_generated":   false,
				"profile_active":     false,
				"profile_source_key": sourceKey,
				"active_source_key":  activeSourceKey,
				"core_running":       deps.Core != nil && deps.Core.Status().State == core.StateRunning,
				"message":            "设备分组已全局保存，策略覆盖已写入当前配置档案；当前运行配置未变更。",
			})
			return
		}

		generated, genErr := generateMihomoConfig(deps)
		if genErr != nil {
			JSON(w, http.StatusOK, map[string]interface{}{
				"updated":            true,
				"config_generated":   false,
				"warning":            genErr.Error(),
				"profile_active":     true,
				"profile_source_key": sourceKey,
				"active_source_key":  activeSourceKey,
			})
			return
		}

		coreRunning := deps.Core != nil && deps.Core.Status().State == core.StateRunning
		coreReloaded := false
		reloadErrText := ""
		if generated && coreRunning {
			if err := deps.Core.Reload(deps.Core.Status().ConfigFile); err != nil {
				reloadErrText = err.Error()
			} else {
				coreReloaded = true
			}
		}

		resp := map[string]interface{}{
			"updated":            true,
			"config_generated":   generated,
			"core_running":       coreRunning,
			"core_reloaded":      coreReloaded,
			"profile_active":     true,
			"profile_source_key": sourceKey,
			"active_source_key":  activeSourceKey,
		}
		if reloadErrText != "" {
			resp["reload_error"] = reloadErrText
			resp["warning"] = "设备路由已保存并生成配置，但热加载失败，请尝试重载或重启内核。"
		}
		JSON(w, http.StatusOK, resp)
	}
}
