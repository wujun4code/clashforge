package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/wujun4code/clashforge/internal/publish"
)

type cfResourceAuthRequest struct {
	Token     string `json:"token"`
	AccountID string `json:"account_id"`
}

type cfResourceDeleteRequest struct {
	Token              string   `json:"token"`
	AccountID          string   `json:"account_id"`
	WorkerNames        []string `json:"worker_names"`
	NamespaceIDs       []string `json:"namespace_ids"`
	PurgeLocalConfigs  bool     `json:"purge_local_configs,omitempty"`
	PurgeLocalRuleSets bool     `json:"purge_local_rulesets,omitempty"`
}

type cfManagedWorkerRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfWorkerResourceView struct {
	Name           string               `json:"name"`
	CreatedOn      string               `json:"created_on,omitempty"`
	ModifiedOn     string               `json:"modified_on,omitempty"`
	KVNamespaceIDs []string             `json:"kv_namespace_ids,omitempty"`
	Managed        bool                 `json:"managed"`
	ManagedBy      []cfManagedWorkerRef `json:"managed_by,omitempty"`
}

type cfNamespaceResourceView struct {
	ID                  string               `json:"id"`
	Title               string               `json:"title"`
	SupportsURLEncoding bool                 `json:"supports_url_encoding,omitempty"`
	BoundWorkers        []string             `json:"bound_workers,omitempty"`
	Managed             bool                 `json:"managed"`
	ManagedBy           []cfManagedWorkerRef `json:"managed_by,omitempty"`
}

type cfDeleteResultItem struct {
	Kind    string `json:"kind"` // worker | namespace
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
	Error   string `json:"error,omitempty"`
}

func handleListCloudflareResources(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req cfResourceAuthRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		token, accountID, err := resolveCFResourceCredentials(deps, req.Token, req.AccountID)
		if err != nil {
			Err(w, http.StatusBadRequest, "CF_CREDENTIALS_REQUIRED", err.Error())
			return
		}

		client, err := publish.NewCloudflareClient(token)
		if err != nil {
			Err(w, http.StatusBadRequest, "PUBLISH_CF_CLIENT_FAILED", err.Error())
			return
		}

		workers, err := client.ListWorkerScripts(r.Context(), accountID)
		if err != nil {
			Err(w, http.StatusBadGateway, "CF_LIST_WORKERS_FAILED", err.Error())
			return
		}
		namespaces, err := client.ListKVNamespaces(r.Context(), accountID)
		if err != nil {
			Err(w, http.StatusBadGateway, "CF_LIST_NAMESPACES_FAILED", err.Error())
			return
		}

		workerRefs, namespaceRefs := collectCFManagedRefs(deps.PublishStore)
		nsBoundWorkers := make(map[string][]string, len(workers))

		workerViews := make([]cfWorkerResourceView, 0, len(workers))
		managedWorkers := 0
		boundPairs := 0
		for _, item := range workers {
			refs := dedupeManagedRefs(workerRefs[strings.TrimSpace(item.Name)])
			managed := len(refs) > 0
			if managed {
				managedWorkers++
			}
			nsIDs := normalizeCFIDs(item.KVNamespaceIDs)
			boundPairs += len(nsIDs)
			for _, nsID := range nsIDs {
				nsBoundWorkers[nsID] = append(nsBoundWorkers[nsID], item.Name)
			}
			workerViews = append(workerViews, cfWorkerResourceView{
				Name:           item.Name,
				CreatedOn:      item.CreatedOn,
				ModifiedOn:     item.ModifiedOn,
				KVNamespaceIDs: nsIDs,
				Managed:        managed,
				ManagedBy:      refs,
			})
		}

		namespaceViews := make([]cfNamespaceResourceView, 0, len(namespaces))
		managedNamespaces := 0
		for _, item := range namespaces {
			refs := dedupeManagedRefs(namespaceRefs[strings.TrimSpace(item.ID)])
			managed := len(refs) > 0
			if managed {
				managedNamespaces++
			}
			boundWorkers := normalizeCFIDs(nsBoundWorkers[strings.TrimSpace(item.ID)])
			namespaceViews = append(namespaceViews, cfNamespaceResourceView{
				ID:                  item.ID,
				Title:               item.Title,
				SupportsURLEncoding: item.SupportsURLEncoding,
				BoundWorkers:        boundWorkers,
				Managed:             managed,
				ManagedBy:           refs,
			})
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"account_id": accountID,
			"workers":    workerViews,
			"namespaces": namespaceViews,
			"summary": map[string]int{
				"workers_total":      len(workerViews),
				"workers_managed":    managedWorkers,
				"namespaces_total":   len(namespaceViews),
				"namespaces_managed": managedNamespaces,
				"bindings_total":     boundPairs,
			},
		})
	}
}

func handleDeleteCloudflareResources(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req cfResourceDeleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}

		token, accountID, err := resolveCFResourceCredentials(deps, req.Token, req.AccountID)
		if err != nil {
			Err(w, http.StatusBadRequest, "CF_CREDENTIALS_REQUIRED", err.Error())
			return
		}

		workerNames := normalizeCFIDs(req.WorkerNames)
		namespaceIDs := normalizeCFIDs(req.NamespaceIDs)
		if len(workerNames) == 0 && len(namespaceIDs) == 0 {
			Err(w, http.StatusBadRequest, "CF_DELETE_TARGETS_REQUIRED", "worker_names or namespace_ids is required")
			return
		}

		client, err := publish.NewCloudflareClient(token)
		if err != nil {
			Err(w, http.StatusBadRequest, "PUBLISH_CF_CLIENT_FAILED", err.Error())
			return
		}

		items := make([]cfDeleteResultItem, 0, len(workerNames)+len(namespaceIDs))
		deletedCount := 0
		failedCount := 0

		for _, name := range workerNames {
			item := cfDeleteResultItem{Kind: "worker", ID: name}
			if err := client.DeleteWorkerScript(r.Context(), accountID, name); err != nil {
				item.Error = err.Error()
				failedCount++
			} else {
				item.Deleted = true
				deletedCount++
			}
			items = append(items, item)
		}

		for _, nsID := range namespaceIDs {
			item := cfDeleteResultItem{Kind: "namespace", ID: nsID}
			if err := client.DeleteKVNamespace(r.Context(), accountID, nsID); err != nil {
				item.Error = err.Error()
				failedCount++
			} else {
				item.Deleted = true
				deletedCount++
			}
			items = append(items, item)
		}

		warnings := make([]string, 0)
		if (req.PurgeLocalConfigs || req.PurgeLocalRuleSets) && deps.PublishStore != nil {
			warnings = append(warnings, purgeLocalPublishRecords(deps.PublishStore, workerNames, namespaceIDs, req.PurgeLocalConfigs, req.PurgeLocalRuleSets)...)
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"account_id": accountID,
			"results":    items,
			"summary": map[string]int{
				"total":   len(items),
				"deleted": deletedCount,
				"failed":  failedCount,
			},
			"warnings": warnings,
		})
	}
}

func resolveCFResourceCredentials(deps Dependencies, rawToken, rawAccountID string) (string, string, error) {
	token := strings.TrimSpace(rawToken)
	accountID := strings.TrimSpace(rawAccountID)
	if token != "" && accountID != "" {
		return token, accountID, nil
	}

	if deps.Config == nil || deps.Config.Core.DataDir == "" {
		return "", "", fmt.Errorf("缺少 Cloudflare 凭据，请先在页面顶部配置")
	}
	path := cfConfigPath(deps.Config.Core.DataDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("缺少 Cloudflare 凭据，请先在页面顶部配置")
	}

	var cfg CFConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", "", fmt.Errorf("读取 Cloudflare 凭据失败: %v", err)
	}
	if token == "" {
		token = strings.TrimSpace(cfg.CFToken)
	}
	if accountID == "" {
		accountID = strings.TrimSpace(cfg.CFAccountID)
	}
	if token == "" || accountID == "" {
		return "", "", fmt.Errorf("缺少 Cloudflare 凭据，请先在页面顶部配置")
	}
	return token, accountID, nil
}

func collectCFManagedRefs(store *publish.Store) (map[string][]cfManagedWorkerRef, map[string][]cfManagedWorkerRef) {
	workerRefs := make(map[string][]cfManagedWorkerRef)
	namespaceRefs := make(map[string][]cfManagedWorkerRef)
	if store == nil {
		return workerRefs, namespaceRefs
	}
	for _, cfg := range store.ListWorkerConfigs() {
		ref := cfManagedWorkerRef{ID: cfg.ID, Name: cfg.Name}
		workerName := strings.TrimSpace(cfg.WorkerName)
		if workerName != "" {
			workerRefs[workerName] = append(workerRefs[workerName], ref)
		}
		nsID := strings.TrimSpace(cfg.NamespaceID)
		if nsID != "" {
			namespaceRefs[nsID] = append(namespaceRefs[nsID], ref)
		}
	}
	return workerRefs, namespaceRefs
}

func dedupeManagedRefs(items []cfManagedWorkerRef) []cfManagedWorkerRef {
	if len(items) <= 1 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]cfManagedWorkerRef, 0, len(items))
	for _, item := range items {
		key := item.ID + ":" + item.Name
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func normalizeCFIDs(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func purgeLocalPublishRecords(store *publish.Store, workerNames, namespaceIDs []string, purgeConfigs, purgeRuleSets bool) []string {
	cfgs := store.ListWorkerConfigs()
	if len(cfgs) == 0 {
		return nil
	}

	workerSet := make(map[string]struct{}, len(workerNames))
	for _, w := range workerNames {
		workerSet[w] = struct{}{}
	}
	nsSet := make(map[string]struct{}, len(namespaceIDs))
	for _, id := range namespaceIDs {
		nsSet[id] = struct{}{}
	}

	targetCfgIDs := make([]string, 0)
	for _, cfg := range cfgs {
		if _, ok := workerSet[strings.TrimSpace(cfg.WorkerName)]; ok {
			targetCfgIDs = append(targetCfgIDs, cfg.ID)
			continue
		}
		if _, ok := nsSet[strings.TrimSpace(cfg.NamespaceID)]; ok {
			targetCfgIDs = append(targetCfgIDs, cfg.ID)
			continue
		}
	}
	if len(targetCfgIDs) == 0 {
		return nil
	}

	warnings := make([]string, 0)
	if purgeRuleSets {
		for _, rs := range store.ListRuleSets() {
			for _, cfgID := range targetCfgIDs {
				if rs.WorkerConfigID != cfgID {
					continue
				}
				if err := store.DeleteRuleSet(rs.ID); err != nil {
					warnings = append(warnings, fmt.Sprintf("删除本地规则集 %s 失败: %v", rs.Name, err))
				}
				break
			}
		}
	}
	if purgeConfigs {
		for _, cfgID := range targetCfgIDs {
			if err := store.DeleteWorkerConfig(cfgID); err != nil {
				warnings = append(warnings, fmt.Sprintf("删除本地 Worker 配置 %s 失败: %v", cfgID, err))
			}
		}
	}
	return warnings
}
