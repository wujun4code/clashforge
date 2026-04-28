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
}

func handleGetDeviceGroups(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := config.DeviceGroupsPath(deps.Config.Core.DataDir)
		groups, err := config.LoadDeviceGroups(path)
		if err != nil {
			Err(w, http.StatusInternalServerError, "DEVICE_GROUPS_READ_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"device_groups": groups,
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

		path := config.DeviceGroupsPath(deps.Config.Core.DataDir)
		if err := config.SaveDeviceGroups(path, body.DeviceGroups); err != nil {
			Err(w, http.StatusInternalServerError, "DEVICE_GROUPS_WRITE_FAILED", err.Error())
			return
		}

		generated, genErr := generateMihomoConfig(deps)
		if genErr != nil {
			JSON(w, http.StatusOK, map[string]interface{}{
				"updated":          true,
				"config_generated": false,
				"warning":          genErr.Error(),
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
			"updated":          true,
			"config_generated": generated,
			"core_running":     coreRunning,
			"core_reloaded":    coreReloaded,
		}
		if reloadErrText != "" {
			resp["reload_error"] = reloadErrText
			resp["warning"] = "设备路由已保存并生成配置，但热加载失败，请尝试重载或重启内核。"
		}
		JSON(w, http.StatusOK, resp)
	}
}
