package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wujun4code/clashforge/internal/nodes"
	"github.com/wujun4code/clashforge/internal/publish"
	"github.com/wujun4code/clashforge/internal/subscription"
	"github.com/wujun4code/clashforge/internal/workernode"
	"gopkg.in/yaml.v3"
)

type publishNodeItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Host           string `json:"host"`
	Domain         string `json:"domain"`
	Status         string `json:"status"`
	HasCredentials bool   `json:"has_credentials"`
	NodeType       string `json:"node_type"` // "ssh" | "worker"
}

func isPublishNodeStatusAllowed(status nodes.Status) bool {
	switch status {
	case nodes.StatusConnected, nodes.StatusDeployed:
		return true
	default:
		return false
	}
}

func handleGetPublishNodes(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items := make([]publishNodeItem, 0)

		// SSH nodes
		if deps.NodeStore != nil {
			for _, n := range deps.NodeStore.List() {
				if !isPublishNodeStatusAllowed(n.Status) {
					continue
				}
				full, ok := deps.NodeStore.Get(n.ID)
				if !ok {
					continue
				}
				hasCred := strings.TrimSpace(full.ProxyUser) != "" && strings.TrimSpace(full.ProxyPassword) != ""
				items = append(items, publishNodeItem{
					ID:             n.ID,
					Name:           n.Name,
					Host:           n.Host,
					Domain:         n.Domain,
					Status:         string(n.Status),
					HasCredentials: hasCred,
					NodeType:       "ssh",
				})
			}
		}

		// Imported proxy node sets — each ProxyNode becomes its own selectable item.
		// ID format: "imported:{subID}/{index}"
		if deps.SubManager != nil {
			for _, sub := range deps.SubManager.GetAllImports() {
				if !sub.Enabled {
					continue
				}
				nodes, err := deps.SubManager.GetCachedNodes(sub.ID)
				if err != nil {
					continue
				}
				for idx, pn := range nodes {
					items = append(items, publishNodeItem{
						ID:             fmt.Sprintf("imported:%s/%d", sub.ID, idx),
						Name:           pn.Name,
						Host:           pn.Server,
						Domain:         pn.Server,
						Status:         "imported",
						HasCredentials: true,
						NodeType:       "imported",
					})
				}
			}
		}

		// Worker nodes
		if deps.WorkerNodeStore != nil {
			for _, n := range deps.WorkerNodeStore.List() {
				if n.Status != workernode.StatusDeployed {
					continue
				}
				items = append(items, publishNodeItem{
					ID:             n.ID,
					Name:           n.Name,
					Host:           n.Hostname,
					Domain:         n.Hostname,
					Status:         string(n.Status),
					HasCredentials: true,
					NodeType:       "worker",
				})
			}
		}

		sort.SliceStable(items, func(i, j int) bool {
			return items[i].Name < items[j].Name
		})
		JSON(w, http.StatusOK, map[string]interface{}{
			"nodes": items,
		})
	}
}

func handleGetPublishTemplates() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]interface{}{
			"templates": publish.ListTemplatePresets(),
		})
	}
}

func handlePublishPreview(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.NodeStore == nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_NODES_UNAVAILABLE", "node store not initialized")
			return
		}

		var req publish.PublishPreviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}

		merged, nodeCount, templateMode, err := buildPublishMergedYAML(deps, req, true)
		if err != nil {
			Err(w, http.StatusBadRequest, "PUBLISH_PREVIEW_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"content":        merged,
			"node_count":     nodeCount,
			"template_mode":  templateMode,
			"managed_groups": []string{"🚀 节点选择", "♻️ 自动选择"},
		})
	}
}

func handleGetPublishWorkerConfigs(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.PublishStore == nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_UNAVAILABLE", "publish store not initialized")
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"configs": deps.PublishStore.ListWorkerConfigs(),
		})
	}
}

func handleUpsertPublishWorkerConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.PublishStore == nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_UNAVAILABLE", "publish store not initialized")
			return
		}
		var req publish.WorkerConfigInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		if id := strings.TrimSpace(chi.URLParam(r, "id")); id != "" {
			req.ID = id
		}
		cfg, err := deps.PublishStore.UpsertWorkerConfig(req)
		if err != nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_WRITE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"config": cfg,
		})
	}
}

func handleDeletePublishWorkerConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.PublishStore == nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_UNAVAILABLE", "publish store not initialized")
			return
		}
		id := strings.TrimSpace(chi.URLParam(r, "id"))
		if id == "" {
			Err(w, http.StatusBadRequest, "PUBLISH_CONFIG_ID_REQUIRED", "worker config id is required")
			return
		}
		if err := deps.PublishStore.DeleteWorkerConfig(id); err != nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_DELETE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

func handleDestroyPublishWorkerConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.PublishStore == nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_UNAVAILABLE", "publish store not initialized")
			return
		}
		id := strings.TrimSpace(chi.URLParam(r, "id"))
		if id == "" {
			Err(w, http.StatusBadRequest, "PUBLISH_CONFIG_ID_REQUIRED", "worker config id is required")
			return
		}
		cfg, token, err := deps.PublishStore.GetWorkerConfigWithToken(id)
		var warnings []string
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("获取 Token 失败（将跳过 CF 清理）: %s", err.Error()))
		} else if strings.TrimSpace(token) != "" && strings.TrimSpace(cfg.AccountID) != "" {
			client, cfErr := publish.NewCloudflareClient(token)
			if cfErr != nil {
				warnings = append(warnings, fmt.Sprintf("CF 客户端初始化失败: %s", cfErr.Error()))
			} else {
				if cfg.WorkerName != "" {
					if wErr := client.DeleteWorkerScript(r.Context(), cfg.AccountID, cfg.WorkerName); wErr != nil {
						warnings = append(warnings, fmt.Sprintf("删除 Worker 脚本失败: %s", wErr.Error()))
					}
				}
				if cfg.NamespaceID != "" {
					if nErr := client.DeleteKVNamespace(r.Context(), cfg.AccountID, cfg.NamespaceID); nErr != nil {
						warnings = append(warnings, fmt.Sprintf("删除 KV Namespace 失败: %s", nErr.Error()))
					}
				}
			}
		}
		if err := deps.PublishStore.DeleteWorkerConfig(id); err != nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_DELETE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{"deleted": true, "warnings": warnings})
	}
}

func handlePublishWorkerCheckPermissions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req publish.WorkerPermissionCheckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		if strings.TrimSpace(req.AccountID) == "" || strings.TrimSpace(req.Token) == "" {
			Err(w, http.StatusBadRequest, "PUBLISH_WORKER_PARAMS_REQUIRED", "token and account_id are required")
			return
		}

		client, err := publish.NewCloudflareClient(req.Token)
		if err != nil {
			Err(w, http.StatusBadRequest, "PUBLISH_CF_CLIENT_FAILED", err.Error())
			return
		}
		checks := client.CheckPermissions(r.Context(), req.AccountID, req.ZoneID)
		allOK := true
		for _, item := range checks {
			if !item.OK {
				allOK = false
				break
			}
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"ok":         allOK,
			"checks":     checks,
			"account_id": strings.TrimSpace(req.AccountID),
		})
	}
}

func handlePublishWorkerCreateNamespace() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req publish.WorkerCreateNamespaceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		client, err := publish.NewCloudflareClient(req.Token)
		if err != nil {
			Err(w, http.StatusBadRequest, "PUBLISH_CF_CLIENT_FAILED", err.Error())
			return
		}
		result, err := client.CreateOrReuseNamespace(r.Context(), req.AccountID, req.WorkerName)
		if err != nil {
			Err(w, http.StatusBadRequest, "PUBLISH_NAMESPACE_CREATE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"namespace_id": result.NamespaceID,
			"reused":       result.Reused,
			"title":        result.Title,
		})
	}
}

func handlePublishWorkerDeployScript() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req publish.WorkerDeployScriptRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		client, err := publish.NewCloudflareClient(req.Token)
		if err != nil {
			Err(w, http.StatusBadRequest, "PUBLISH_CF_CLIENT_FAILED", err.Error())
			return
		}
		result, err := client.DeployWorkerScript(
			r.Context(),
			req.AccountID,
			req.WorkerName,
			req.NamespaceID,
			req.AccessToken,
		)
		if err != nil {
			Err(w, http.StatusBadRequest, "PUBLISH_WORKER_DEPLOY_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"worker_dev_url":    result.WorkerDevURL,
			"workers_subdomain": result.WorkersSubdomain,
		})
	}
}

func handlePublishWorkerBindDomain() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req publish.WorkerBindDomainRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		client, err := publish.NewCloudflareClient(req.Token)
		if err != nil {
			Err(w, http.StatusBadRequest, "PUBLISH_CF_CLIENT_FAILED", err.Error())
			return
		}
		result, err := client.BindWorkerDomain(r.Context(), req.AccountID, req.ZoneID, req.WorkerName, req.Hostname)
		if err != nil {
			Err(w, http.StatusBadRequest, "PUBLISH_WORKER_BIND_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"hostname":   result.Hostname,
			"worker_url": result.WorkerURL,
		})
	}
}

func handlePublishWorkerVerifySave(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.PublishStore == nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_UNAVAILABLE", "publish store not initialized")
			return
		}

		var req publish.WorkerVerifySaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		result, err := publish.VerifyWorkerEndpoint(r.Context(), req.WorkerURL, req.WorkerDevURL, req.AccessToken)
		if err != nil {
			Err(w, http.StatusBadRequest, "PUBLISH_WORKER_VERIFY_FAILED", err.Error())
			return
		}

		var cfg publish.WorkerConfigView
		if result.OK {
			now := time.Now().Format(time.RFC3339)
			name := strings.TrimSpace(req.Name)
			if name == "" {
				name = strings.TrimSpace(req.WorkerName)
			}
			cfg, err = deps.PublishStore.UpsertWorkerConfig(publish.WorkerConfigInput{
				Name:          name,
				WorkerName:    req.WorkerName,
				WorkerURL:     req.WorkerURL,
				WorkerDevURL:  req.WorkerDevURL,
				Hostname:      req.Hostname,
				AccountID:     req.AccountID,
				NamespaceID:   req.NamespaceID,
				ZoneID:        req.ZoneID,
				Token:         req.AccessToken,
				InitializedAt: now,
			})
			if err != nil {
				Err(w, http.StatusInternalServerError, "PUBLISH_STORE_WRITE_FAILED", err.Error())
				return
			}
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"result": result,
			"config": cfg,
		})
	}
}

func handlePublishUpload(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.PublishStore == nil || deps.NodeStore == nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_UNAVAILABLE", "publish dependencies are not initialized")
			return
		}

		var req publish.PublishUploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}

		workerConfigID := strings.TrimSpace(req.WorkerConfigID)
		if workerConfigID == "" {
			Err(w, http.StatusBadRequest, "PUBLISH_WORKER_CONFIG_REQUIRED", "worker_config_id is required")
			return
		}

		content := strings.TrimSpace(req.Content)
		if content == "" {
			merged, _, _, err := buildPublishMergedYAML(deps, publish.PublishPreviewRequest{
				NodeIDs:         req.NodeIDs,
				TemplateMode:    req.TemplateMode,
				TemplateID:      req.TemplateID,
				TemplateContent: req.TemplateContent,
				RuleSetIDs:      req.RuleSetIDs,
			}, false)
			if err != nil {
				Err(w, http.StatusBadRequest, "PUBLISH_CONTENT_BUILD_FAILED", err.Error())
				return
			}
			content = merged
		}

		if err := validatePublishYAML(content); err != nil {
			Err(w, http.StatusBadRequest, "PUBLISH_CONTENT_INVALID", err.Error())
			return
		}

		cfg, token, err := deps.PublishStore.GetWorkerConfigWithToken(workerConfigID)
		if err != nil {
			Err(w, http.StatusBadRequest, "PUBLISH_WORKER_CONFIG_NOT_FOUND", err.Error())
			return
		}
		if strings.TrimSpace(token) == "" {
			Err(w, http.StatusBadRequest, "PUBLISH_WORKER_TOKEN_MISSING", "worker config is missing access token")
			return
		}
		workerBase := publish.PickWorkerBaseURL(cfg.WorkerURL, cfg.WorkerDevURL)
		if workerBase == "" {
			Err(w, http.StatusBadRequest, "PUBLISH_WORKER_URL_MISSING", "worker url is missing")
			return
		}

		baseName := publish.SanitizeBaseName(req.BaseName)
		nextVersion := deps.PublishStore.NextVersion(cfg.ID, baseName)
		fileName := publish.VersionedFileName(baseName, nextVersion, time.Now())

		if err := publish.UploadContentViaWorker(r.Context(), workerBase, fileName, token, content); err != nil {
			Err(w, http.StatusBadGateway, "PUBLISH_UPLOAD_FAILED", err.Error())
			return
		}

		accessURL := fmt.Sprintf("%s/%s?token=%s", workerBase, url.PathEscape(fileName), url.QueryEscape(token))
		record, err := deps.PublishStore.AddPublishRecord(publish.PublishRecordInput{
			WorkerConfigID: cfg.ID,
			WorkerName:     cfg.WorkerName,
			Hostname:       cfg.Hostname,
			BaseName:       baseName,
			Version:        nextVersion,
			FileName:       fileName,
			AccessURL:      accessURL,
		})
		if err != nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_RECORD_WRITE_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"record":     record,
			"file_name":  fileName,
			"version":    nextVersion,
			"access_url": accessURL,
		})
	}
}

func handleGetPublishRecords(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.PublishStore == nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_UNAVAILABLE", "publish store not initialized")
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"records": deps.PublishStore.ListPublishRecords(),
		})
	}
}

func handleDeletePublishRecord(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.PublishStore == nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_UNAVAILABLE", "publish store not initialized")
			return
		}
		id := strings.TrimSpace(chi.URLParam(r, "id"))
		if id == "" {
			Err(w, http.StatusBadRequest, "PUBLISH_RECORD_ID_REQUIRED", "publish record id is required")
			return
		}

		record, ok := deps.PublishStore.GetPublishRecord(id)
		if !ok {
			Err(w, http.StatusNotFound, "PUBLISH_RECORD_NOT_FOUND", "publish record not found")
			return
		}

		warning := ""
		cfg, token, err := deps.PublishStore.GetWorkerConfigWithToken(record.WorkerConfigID)
		if err != nil {
			warning = "worker config not found, skipped remote cleanup"
		} else {
			workerBase := publish.PickWorkerBaseURL(cfg.WorkerURL, cfg.WorkerDevURL)
			if workerBase == "" {
				warning = "worker url missing, skipped remote cleanup"
			} else if strings.TrimSpace(token) == "" {
				warning = "worker token missing, skipped remote cleanup"
			} else if err := publish.DeleteContentViaWorker(r.Context(), workerBase, record.FileName, token); err != nil {
				warning = "remote delete failed: " + err.Error()
			}
		}

		if err := deps.PublishStore.DeletePublishRecord(id); err != nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_DELETE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"deleted": true,
			"warning": warning,
		})
	}
}

func normalizePublishNodeIDs(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		id := strings.TrimSpace(item)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func buildPublishMergeNodes(sshStore *nodes.Store, wStore *workernode.Store, subMgr interface {
	GetAllImports() []subscription.Subscription
	GetCachedNodes(string) ([]subscription.ProxyNode, error)
}, nodeIDs []string) ([]publish.MergeNode, error) {
	ids := normalizePublishNodeIDs(nodeIDs)
	if len(ids) == 0 {
		return nil, fmt.Errorf("at least one node must be selected")
	}
	out := make([]publish.MergeNode, 0, len(ids))
	for _, id := range ids {
		// Imported node: "imported:{subID}/{index}"
		if strings.HasPrefix(id, "imported:") {
			parts := strings.SplitN(strings.TrimPrefix(id, "imported:"), "/", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid imported node id: %s", id)
			}
			subID := parts[0]
			idxStr := parts[1]
			var idx int
			if _, err := fmt.Sscanf(idxStr, "%d", &idx); err != nil {
				return nil, fmt.Errorf("invalid imported node index in id: %s", id)
			}
			if subMgr == nil {
				return nil, fmt.Errorf("subscription manager not available")
			}
			cachedNodes, err := subMgr.GetCachedNodes(subID)
			if err != nil {
				return nil, fmt.Errorf("cannot load imported node cache for sub %s: %w", subID, err)
			}
			if idx < 0 || idx >= len(cachedNodes) {
				return nil, fmt.Errorf("imported node index %d out of range for sub %s", idx, subID)
			}
			pn := cachedNodes[idx]
			// Reconstruct the raw proxy map from ProxyNode fields.
			rawProxy := map[string]interface{}{
				"name":   pn.Name,
				"type":   pn.Type,
				"server": pn.Server,
				"port":   pn.Port,
			}
			for k, v := range pn.Extra {
				rawProxy[k] = v
			}
			out = append(out, publish.MergeNode{
				ID:            id,
				Name:          pn.Name,
				NodeType:      "imported",
				ImportedProxy: rawProxy,
			})
			continue
		}
		// Try SSH store first
		if sshStore != nil {
			if n, ok := sshStore.Get(id); ok {
				if !isPublishNodeStatusAllowed(n.Status) {
					return nil, fmt.Errorf("node is not ready for publish: %s", n.Name)
				}
				out = append(out, publish.MergeNode{
					ID:            n.ID,
					Name:          n.Name,
					Host:          n.Host,
					Domain:        n.Domain,
					ProxyUser:     n.ProxyUser,
					ProxyPassword: n.ProxyPassword,
					NodeType:      "ssh",
				})
				continue
			}
		}
		// Try worker store
		if wStore != nil {
			if n, ok := wStore.Get(id); ok {
				if n.Status != workernode.StatusDeployed {
					return nil, fmt.Errorf("worker node is not deployed: %s", n.Name)
				}
				out = append(out, publish.MergeNode{
					ID:             n.ID,
					Name:           n.Name,
					NodeType:       "worker",
					WorkerUUID:     n.WorkerUUID,
					WorkerHostname: n.Hostname,
				})
				continue
			}
		}
		return nil, fmt.Errorf("node not found: %s", id)
	}
	return out, nil
}

func buildPublishMergedYAML(
	deps Dependencies,
	req publish.PublishPreviewRequest,
	allowEmptyNodes bool,
) (string, int, string, error) {
	mode := strings.ToLower(strings.TrimSpace(req.TemplateMode))
	if mode == "" {
		mode = "builtin"
	}
	runtimeContent := ""
	if mode == "runtime" {
		if deps.Config == nil {
			return "", 0, mode, fmt.Errorf("config not initialized")
		}
		path := filepath.Join(deps.Config.Core.RuntimeDir, "mihomo-config.yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			return "", 0, mode, fmt.Errorf("runtime config not found, please run setup first")
		}
		runtimeContent = string(data)
	}

	templateContent, err := publish.ResolveTemplate(mode, req.TemplateID, req.TemplateContent, runtimeContent)
	if err != nil {
		return "", 0, mode, err
	}

	nodeIDs := normalizePublishNodeIDs(req.NodeIDs)
	nodeCount := 0
	merged := templateContent
	if len(nodeIDs) > 0 {
		mergeNodes, err := buildPublishMergeNodes(deps.NodeStore, deps.WorkerNodeStore, deps.SubManager, nodeIDs)
		if err != nil {
			return "", 0, mode, err
		}
		merged, err = publish.MergeTemplateWithNodes(templateContent, mergeNodes)
		if err != nil {
			return "", 0, mode, err
		}
		nodeCount = len(mergeNodes)
	} else if !allowEmptyNodes {
		return "", 0, mode, fmt.Errorf("at least one node must be selected")
	}

	// Inject hosted rule-providers if requested
	if len(req.RuleSetIDs) > 0 && deps.PublishStore != nil {
		ruleSets := make([]publish.RuleSet, 0, len(req.RuleSetIDs))
		for _, id := range req.RuleSetIDs {
			if rs, ok := deps.PublishStore.GetRuleSet(id); ok {
				ruleSets = append(ruleSets, rs)
			}
		}
		if len(ruleSets) > 0 {
			merged, err = publish.InjectRuleProviders(merged, ruleSets)
			if err != nil {
				return "", 0, mode, err
			}
		}
	}

	return merged, nodeCount, mode, nil
}

func validatePublishYAML(content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("content is empty")
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return fmt.Errorf("yaml parse failed: %w", err)
	}
	if root == nil {
		return fmt.Errorf("yaml root is empty")
	}

	proxies, ok := root["proxies"].([]interface{})
	if !ok || len(proxies) == 0 {
		return fmt.Errorf("yaml missing proxies")
	}
	groups, ok := root["proxy-groups"].([]interface{})
	if !ok || len(groups) == 0 {
		return fmt.Errorf("yaml missing proxy-groups")
	}
	rules, ok := root["rules"].([]interface{})
	if !ok || len(rules) == 0 {
		return fmt.Errorf("yaml missing rules")
	}
	return nil
}
