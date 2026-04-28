package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wujun4code/clashforge/internal/nodes"
)

// nodeCreateRequest is the POST body for creating a node.
type nodeCreateRequest struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Domain      string `json:"domain"`
	Email       string `json:"email"`
	CFToken     string `json:"cf_token"`
	CFAccountID string `json:"cf_account_id"`
	CFZoneID    string `json:"cf_zone_id"`
}

type nodeProbeRequest struct {
	Mode string `json:"mode"`
}

type cloudflareZonesRequest struct {
	CFToken     string `json:"cf_token"`
	CFAccountID string `json:"cf_account_id"`
}

type nodeDeployRequest struct {
	Mode string `json:"mode"`
}

func handleListNodes(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]interface{}{
			"nodes": store.List(),
		})
	}
}

func handleCreateNode(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req nodeCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}

		if req.Port == 0 {
			req.Port = 22
		}

		node := &nodes.Node{
			Name:        req.Name,
			Host:        req.Host,
			Port:        req.Port,
			Username:    req.Username,
			Password:    req.Password,
			Domain:      req.Domain,
			Email:       req.Email,
			CFToken:     req.CFToken,
			CFAccountID: req.CFAccountID,
			CFZoneID:    req.CFZoneID,
		}

		if err := store.Create(node); err != nil {
			Err(w, http.StatusInternalServerError, "NODE_CREATE_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusCreated, map[string]interface{}{
			"node": nodeListItem(node),
		})
	}
}

func handleGetNode(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NODE_NOT_FOUND", "节点不存在")
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"node": nodeListItem(node),
		})
	}
}

func handleUpdateNode(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req nodeCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}

		existing, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NODE_NOT_FOUND", "节点不存在")
			return
		}

		if req.Name != "" {
			existing.Name = req.Name
		}
		if req.Host != "" {
			existing.Host = req.Host
		}
		if req.Port != 0 {
			existing.Port = req.Port
		}
		if req.Username != "" {
			existing.Username = req.Username
		}
		if req.Password != "" {
			existing.Password = req.Password
		}
		if req.Domain != "" {
			existing.Domain = req.Domain
		}
		if req.Email != "" {
			existing.Email = req.Email
		}
		if req.CFToken != "" {
			existing.CFToken = req.CFToken
		}
		if req.CFAccountID != "" {
			existing.CFAccountID = req.CFAccountID
		}
		if req.CFZoneID != "" {
			existing.CFZoneID = req.CFZoneID
		}

		if err := store.Update(id, existing); err != nil {
			Err(w, http.StatusInternalServerError, "NODE_UPDATE_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"node": nodeListItem(existing),
		})
	}
}

func handleDeleteNode(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := store.Delete(id); err != nil {
			Err(w, http.StatusInternalServerError, "NODE_DELETE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func handleGetSSHPubKey(kp *nodes.KeyPair) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if kp == nil {
			Err(w, http.StatusInternalServerError, "KEYPAIR_UNAVAILABLE", "SSH key pair not initialized")
			return
		}
		JSON(w, http.StatusOK, map[string]string{
			"public_key": kp.PublicKeyString(),
		})
	}
}

func handleTestNode(store *nodes.Store, kp *nodes.KeyPair) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NODE_NOT_FOUND", "节点不存在")
			return
		}

		if err := nodes.TestSSH(node.Host, node.Port, node.Username, nodes.BuildAuthMethods(node.Password, kp), 15*time.Second); err != nil {
			JSON(w, http.StatusOK, map[string]interface{}{
				"ok":      false,
				"message": err.Error(),
			})
			return
		}

		node.Status = nodes.StatusConnected
		node.Error = ""
		store.Update(id, node)

		JSON(w, http.StatusOK, map[string]interface{}{
			"ok":      true,
			"message": "连接成功",
		})
	}
}

func handleDeployNode(store *nodes.Store, kp *nodes.KeyPair) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NODE_NOT_FOUND", "节点不存在")
			return
		}
		var req nodeDeployRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		mode := strings.TrimSpace(strings.ToLower(req.Mode))
		deployNode := *node
		if mode == "bootstrap" {
			deployNode.Domain = ""
			deployNode.Email = ""
			deployNode.CFToken = ""
			deployNode.CFAccountID = ""
			deployNode.CFZoneID = ""
		}

		// Set up SSE streaming
		flusher, ok := w.(http.Flusher)
		if !ok {
			Err(w, http.StatusInternalServerError, "SSE_UNSUPPORTED", "SSE not supported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		// Collect deploy log in a buffer
		var deployLogBuf bytes.Buffer

		sendSSE := func(step, status, message, detail string) {
			data, _ := json.Marshal(map[string]string{
				"step":    step,
				"status":  status,
				"message": message,
				"detail":  detail,
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			// Accumulate log
			if detail != "" {
				fmt.Fprintf(&deployLogBuf, "[%s] %s: %s (%s)\n", status, step, message, detail)
			} else {
				fmt.Fprintf(&deployLogBuf, "[%s] %s: %s\n", status, step, message)
			}
		}

		node.Status = nodes.StatusDeploying
		node.Error = ""
		node.DeployLog = ""
		store.Update(id, node)

		result, err := nodes.DeployGOST(r.Context(), &deployNode, kp, sendSSE)
		if err != nil || !result.Success {
			node.Status = nodes.StatusError
			if err != nil {
				node.Error = err.Error()
			} else {
				node.Error = result.Error
			}
			node.DeployLog = deployLogBuf.String()
			store.Update(id, node)
			errData, _ := json.Marshal(map[string]interface{}{
				"type":       "done",
				"success":    false,
				"error":      node.Error,
				"deploy_log": node.DeployLog,
			})
			fmt.Fprintf(w, "data: %s\n\n", errData)
			flusher.Flush()
			return
		}

		now := time.Now()
		node.Status = nodes.StatusDeployed
		node.DeployedAt = &now
		node.ProxyUser = result.ProxyUser
		node.ProxyPassword = result.ProxyPass
		if strings.TrimSpace(deployNode.CFZoneID) != "" {
			node.CFZoneID = deployNode.CFZoneID
		}
		node.DeployLog = ""
		store.Update(id, node)

		doneData, _ := json.Marshal(map[string]interface{}{
			"type":          "done",
			"success":       true,
			"phase":         result.Phase,
			"cert_issued":   result.CertIssued,
			"probe_results": result.ProbeResults,
		})
		fmt.Fprintf(w, "data: %s\n\n", doneData)
		flusher.Flush()
	}
}

func handleDestroyNode(store *nodes.Store, kp *nodes.KeyPair) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NODE_NOT_FOUND", "节点不存在")
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			Err(w, http.StatusInternalServerError, "SSE_UNSUPPORTED", "SSE not supported")
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

		result, err := nodes.DestroyGOST(r.Context(), node, kp, sendSSE)
		if err != nil || !result.Success {
			node.Status = nodes.StatusError
			if err != nil {
				node.Error = err.Error()
			} else {
				node.Error = result.Error
			}
			store.Update(id, node)
			errData, _ := json.Marshal(map[string]interface{}{
				"type":    "done",
				"success": false,
				"error":   node.Error,
			})
			fmt.Fprintf(w, "data: %s\n\n", errData)
			flusher.Flush()
			return
		}

		// Clear deployment state after successful destroy
		node.Status = nodes.StatusPending
		node.DeployedAt = nil
		node.CertExpiry = nil
		node.ProxyUser = ""
		node.ProxyPassword = ""
		node.Error = ""
		node.DeployLog = ""
		store.Update(id, node)

		doneData, _ := json.Marshal(map[string]interface{}{
			"type":    "done",
			"success": true,
		})
		fmt.Fprintf(w, "data: %s\n\n", doneData)
		flusher.Flush()
	}
}

func handleExportProxyConfig(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NODE_NOT_FOUND", "节点不存在")
			return
		}

		yamlData, err := nodes.ExportClashProxy(node)
		if err != nil {
			Err(w, http.StatusInternalServerError, "EXPORT_FAILED", err.Error())
			return
		}

		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Write([]byte(yamlData))
	}
}

func handleProbeNode(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		node, ok := store.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NODE_NOT_FOUND", "节点不存在")
			return
		}

		var req nodeProbeRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		mode := strings.TrimSpace(req.Mode)
		if mode == "" {
			mode = "ip"
		}

		proxyHost := node.Host
		if mode == "domain" {
			proxyHost = node.Domain
		}
		if strings.TrimSpace(proxyHost) == "" {
			Err(w, http.StatusBadRequest, "NODE_PROBE_INVALID", "缺少可用的代理地址")
			return
		}
		if strings.TrimSpace(node.ProxyUser) == "" || strings.TrimSpace(node.ProxyPassword) == "" {
			Err(w, http.StatusBadRequest, "NODE_PROBE_INVALID", "节点尚未生成代理账号密码，请先执行部署")
			return
		}

		results := nodes.TestHTTPProxy(proxyHost, 443, node.ProxyUser, node.ProxyPassword, 10*time.Second, nodes.DefaultProbeTargets())
		okCount := 0
		for _, item := range results {
			if item.OK {
				okCount++
			}
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"mode":          mode,
			"proxy_host":    proxyHost,
			"proxy_port":    443,
			"probe_results": results,
			"summary": map[string]interface{}{
				"ok":      okCount,
				"total":   len(results),
				"success": okCount == len(results),
			},
		})
	}
}

func handleCloudflareZones() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req cloudflareZonesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		zones, err := nodes.CloudflareListZones(strings.TrimSpace(req.CFToken), strings.TrimSpace(req.CFAccountID))
		if err != nil {
			Err(w, http.StatusBadRequest, "CF_ZONES_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{"zones": zones})
	}
}

func nodeListItem(n *nodes.Node) nodes.NodeListItem {
	return nodes.NodeListItem{
		ID:         n.ID,
		Name:       n.Name,
		Host:       n.Host,
		Port:       n.Port,
		Username:   n.Username,
		Domain:     n.Domain,
		Status:     n.Status,
		DeployedAt: n.DeployedAt,
		CertExpiry: n.CertExpiry,
		Error:      n.Error,
		DeployLog:  n.DeployLog,
		CreatedAt:  n.CreatedAt,
		UpdatedAt:  n.UpdatedAt,
	}
}
