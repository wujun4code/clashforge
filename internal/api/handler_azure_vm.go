package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wujun4code/clashforge/internal/azure"
	"github.com/wujun4code/clashforge/internal/nodes"
)

// ── Request / Response types ──────────────────────────────────────────────────

type azureValidateRequest struct {
	TenantID       string `json:"tenant_id"`
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	SubscriptionID string `json:"subscription_id"`
}

type azureLocationsRequest struct {
	// Uses saved azure-config; no extra body needed.
}

type azureVMSizesRequest struct {
	Location string `json:"location"`
}

type azureResourceGroupsRequest struct {
	// Uses saved azure-config; no extra body needed.
}

type azureCreateVMRequest struct {
	// VM placement
	Location      string `json:"location"`
	ResourceGroup string `json:"resource_group"`
	// VM settings
	VMName        string `json:"vm_name"`
	VMSize        string `json:"vm_size"`
	AdminUsername string `json:"admin_username"`
	// Node display name
	NodeName string `json:"node_name"`
}

// ── Handlers ─────────────────────────────────────────────────────────────────

// handleValidateAzureCredentials validates the supplied (or saved) Azure Service
// Principal by attempting to fetch the subscription record.
func handleValidateAzureCredentials(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req azureValidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "bad_request", "invalid JSON: "+err.Error())
			return
		}

		// Allow empty body → use saved config
		if req.TenantID == "" {
			cfg, err := loadAzureConfig(deps.Config.Core.DataDir)
			if err != nil {
				Err(w, http.StatusBadRequest, "azure_config_missing", "请先配置 Azure 凭据")
				return
			}
			req.TenantID = cfg.TenantID
			req.ClientID = cfg.ClientID
			req.ClientSecret = cfg.ClientSecret
			req.SubscriptionID = cfg.SubscriptionID
		}

		azCfg := azure.Config{
			TenantID:       req.TenantID,
			ClientID:       req.ClientID,
			ClientSecret:   req.ClientSecret,
			SubscriptionID: req.SubscriptionID,
		}

		token, err := azure.GetAccessToken(azCfg)
		if err != nil {
			Err(w, http.StatusUnauthorized, "azure_auth_failed", err.Error())
			return
		}

		if err := azure.ValidateCredentials(token, req.SubscriptionID); err != nil {
			Err(w, http.StatusUnauthorized, "azure_subscription_invalid", err.Error())
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"ok":              true,
			"subscription_id": req.SubscriptionID,
		})
	}
}

// handleListAzureLocations returns Azure regions available to the subscription.
func handleListAzureLocations(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := loadAzureConfig(deps.Config.Core.DataDir)
		if err != nil {
			Err(w, http.StatusBadRequest, "azure_config_missing", "请先配置 Azure 凭据")
			return
		}

		token, err := azure.GetAccessToken(azure.Config{
			TenantID:       cfg.TenantID,
			ClientID:       cfg.ClientID,
			ClientSecret:   cfg.ClientSecret,
			SubscriptionID: cfg.SubscriptionID,
		})
		if err != nil {
			Err(w, http.StatusUnauthorized, "azure_auth_failed", err.Error())
			return
		}

		locs, err := azure.ListLocations(token, cfg.SubscriptionID)
		if err != nil {
			Err(w, http.StatusInternalServerError, "azure_locations_failed", err.Error())
			return
		}

		// Annotate with friendly Chinese display names
		type locItem struct {
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
		}
		out := make([]locItem, 0, len(locs))
		for _, l := range locs {
			friendly := azure.FriendlyLocationName(l.Name)
			if friendly == l.Name && l.DisplayName != "" {
				friendly = l.DisplayName
			}
			out = append(out, locItem{Name: l.Name, DisplayName: friendly})
		}

		JSON(w, http.StatusOK, map[string]interface{}{"locations": out})
	}
}

// handleListAzureResourceGroups returns resource groups in the subscription.
func handleListAzureResourceGroups(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := loadAzureConfig(deps.Config.Core.DataDir)
		if err != nil {
			Err(w, http.StatusBadRequest, "azure_config_missing", "请先配置 Azure 凭据")
			return
		}

		token, err := azure.GetAccessToken(azure.Config{
			TenantID:       cfg.TenantID,
			ClientID:       cfg.ClientID,
			ClientSecret:   cfg.ClientSecret,
			SubscriptionID: cfg.SubscriptionID,
		})
		if err != nil {
			Err(w, http.StatusUnauthorized, "azure_auth_failed", err.Error())
			return
		}

		rgs, err := azure.ListResourceGroups(token, cfg.SubscriptionID)
		if err != nil {
			Err(w, http.StatusInternalServerError, "azure_rg_failed", err.Error())
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{"resource_groups": rgs})
	}
}

// handleListAzureVMSizes returns recommended VM sizes for a region.
func handleListAzureVMSizes(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req azureVMSizesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Location == "" {
			Err(w, http.StatusBadRequest, "bad_request", "location 不能为空")
			return
		}

		cfg, err := loadAzureConfig(deps.Config.Core.DataDir)
		if err != nil {
			Err(w, http.StatusBadRequest, "azure_config_missing", "请先配置 Azure 凭据")
			return
		}

		token, err := azure.GetAccessToken(azure.Config{
			TenantID:       cfg.TenantID,
			ClientID:       cfg.ClientID,
			ClientSecret:   cfg.ClientSecret,
			SubscriptionID: cfg.SubscriptionID,
		})
		if err != nil {
			Err(w, http.StatusUnauthorized, "azure_auth_failed", err.Error())
			return
		}

		sizes, err := azure.ListVMSizes(token, cfg.SubscriptionID, req.Location)
		if err != nil {
			Err(w, http.StatusInternalServerError, "azure_vm_sizes_failed", err.Error())
			return
		}

		// Filter to cost-effective options commonly used for proxy servers
		type vmSizeItem struct {
			Name          string `json:"name"`
			NumberOfCores int    `json:"cores"`
			MemoryGB      string `json:"memory_gb"`
		}
		preferred := map[string]bool{
			"Standard_B1s":   true,
			"Standard_B1ms":  true,
			"Standard_B2s":   true,
			"Standard_B2ms":  true,
			"Standard_B4ms":  true,
			"Standard_B8ms":  true,
			"Standard_D2s_v3": true,
			"Standard_D2s_v5": true,
			"Standard_F1s":   true,
			"Standard_F2s":   true,
		}
		out := make([]vmSizeItem, 0)
		for _, s := range sizes {
			if preferred[s.Name] {
				memGB := fmt.Sprintf("%.1f", float64(s.MemoryInMB)/1024)
				out = append(out, vmSizeItem{Name: s.Name, NumberOfCores: s.NumberOfCores, MemoryGB: memGB})
			}
		}
		// If none of the preferred sizes are available in this region, return all
		if len(out) == 0 {
			for _, s := range sizes {
				if s.NumberOfCores <= 4 && s.MemoryInMB <= 16384 {
					memGB := fmt.Sprintf("%.1f", float64(s.MemoryInMB)/1024)
					out = append(out, vmSizeItem{Name: s.Name, NumberOfCores: s.NumberOfCores, MemoryGB: memGB})
					if len(out) >= 20 {
						break
					}
				}
			}
		}

		JSON(w, http.StatusOK, map[string]interface{}{"vm_sizes": out})
	}
}

// handleCreateAzureVM provisions an Azure VM and registers it as a ClashForge node.
// Streams SSE progress events while provisioning.
func handleCreateAzureVM(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req azureCreateVMRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "bad_request", "invalid JSON: "+err.Error())
			return
		}

		// Validate required fields
		if req.VMName == "" || req.VMSize == "" || req.AdminUsername == "" ||
			req.Location == "" || req.ResourceGroup == "" {
			Err(w, http.StatusBadRequest, "bad_request", "vm_name, vm_size, admin_username, location, resource_group 均不能为空")
			return
		}
		// Sanitize admin username
		req.AdminUsername = strings.ToLower(strings.TrimSpace(req.AdminUsername))
		req.VMName = strings.ToLower(strings.TrimSpace(req.VMName))

		cfg, err := loadAzureConfig(deps.Config.Core.DataDir)
		if err != nil {
			Err(w, http.StatusBadRequest, "azure_config_missing", "请先配置 Azure 凭据")
			return
		}

		token, err := azure.GetAccessToken(azure.Config{
			TenantID:       cfg.TenantID,
			ClientID:       cfg.ClientID,
			ClientSecret:   cfg.ClientSecret,
			SubscriptionID: cfg.SubscriptionID,
		})
		if err != nil {
			Err(w, http.StatusUnauthorized, "azure_auth_failed", err.Error())
			return
		}

		// Ensure we have a ClashForge SSH key pair for the node
		if deps.NodeKeyPair == nil {
			Err(w, http.StatusInternalServerError, "keypair_unavailable", "SSH key pair 未初始化")
			return
		}
		sshPubKey := deps.NodeKeyPair.PublicKeyString()

		// ── Set up SSE ────────────────────────────────────────────────────────
		flusher, ok := w.(http.Flusher)
		if !ok {
			Err(w, http.StatusInternalServerError, "sse_unsupported", "SSE not supported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		sendSSE := func(step, status, message, detail string) {
			data, _ := json.Marshal(map[string]string{
				"step":    step,
				"status":  status,
				"message": message,
				"detail":  detail,
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		// ── Start provisioning ────────────────────────────────────────────────
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
		defer cancel()

		provReq := azure.ProvisionRequest{
			Token:          token,
			SubscriptionID: cfg.SubscriptionID,
			Location:       req.Location,
			ResourceGroup:  req.ResourceGroup,
			VMName:         req.VMName,
			VMSize:         req.VMSize,
			AdminUsername:  req.AdminUsername,
			SSHPublicKey:   sshPubKey,
			Prefix:         req.VMName,
		}

		progressCh := make(chan azure.ProgressEvent, 32)
		var provResult azure.ProvisionResult
		var provErr error

		go func() {
			provResult, provErr = azure.ProvisionVM(ctx, provReq, progressCh)
			close(progressCh)
		}()

		for ev := range progressCh {
			sendSSE(ev.Step, ev.Status, ev.Message, ev.Detail)
		}

		if provErr != nil {
			doneData, _ := json.Marshal(map[string]interface{}{
				"type":    "done",
				"success": false,
				"error":   provErr.Error(),
			})
			fmt.Fprintf(w, "data: %s\n\n", doneData)
			flusher.Flush()
			return
		}

		// ── Register as a ClashForge node ─────────────────────────────────────
		sendSSE("register", "running", "注册为托管节点", provResult.PublicIP)

		nodeName := req.NodeName
		if nodeName == "" {
			nodeName = req.VMName
		}
		node := &nodes.Node{
			Name:     nodeName,
			Host:     provResult.PublicIP,
			Port:     22,
			Username: req.AdminUsername,
			// No password — uses ClashForge key pair (SSH pubkey auth)
			Status: nodes.StatusConnected,
		}
		if err := deps.NodeStore.Create(node); err != nil {
			sendSSE("register", "error", "注册节点失败", err.Error())
			doneData, _ := json.Marshal(map[string]interface{}{
				"type":    "done",
				"success": false,
				"error":   "注册节点失败: " + err.Error(),
			})
			fmt.Fprintf(w, "data: %s\n\n", doneData)
			flusher.Flush()
			return
		}
		sendSSE("register", "ok", "节点已注册", node.ID)

		doneData, _ := json.Marshal(map[string]interface{}{
			"type":      "done",
			"success":   true,
			"node_id":   node.ID,
			"public_ip": provResult.PublicIP,
			"vm_id":     provResult.VMID,
		})
		fmt.Fprintf(w, "data: %s\n\n", doneData)
		flusher.Flush()
	}
}
