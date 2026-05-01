package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wujun4code/clashforge/internal/publish"
)

// buildRulePayload converts a slice of rule strings into a mihomo rule-provider
// payload YAML:
//
//	payload:
//	  - DOMAIN-SUFFIX,example.com
//	  - '+.foo.bar'
func buildRulePayload(rules []string) string {
	var sb strings.Builder
	sb.WriteString("payload:\n")
	for _, r := range rules {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		sb.WriteString("  - ")
		sb.WriteString(r)
		sb.WriteString("\n")
	}
	return sb.String()
}

func handleListRuleSets(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.PublishStore == nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_UNAVAILABLE", "publish store not initialized")
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"rule_sets": deps.PublishStore.ListRuleSets(),
		})
	}
}

func handleCreateRuleSet(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.PublishStore == nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_UNAVAILABLE", "publish store not initialized")
			return
		}

		var input publish.RuleSetInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}

		input.Name = strings.TrimSpace(input.Name)
		input.WorkerConfigID = strings.TrimSpace(input.WorkerConfigID)
		if input.Name == "" {
			Err(w, http.StatusBadRequest, "RULESET_NAME_REQUIRED", "name is required")
			return
		}
		if input.WorkerConfigID == "" {
			Err(w, http.StatusBadRequest, "RULESET_WORKER_CONFIG_REQUIRED", "worker_config_id is required")
			return
		}
		if len(input.Rules) == 0 {
			Err(w, http.StatusBadRequest, "RULESET_RULES_REQUIRED", "at least one rule is required")
			return
		}

		cfg, token, err := deps.PublishStore.GetWorkerConfigWithToken(input.WorkerConfigID)
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

		kvKey := publish.NewRuleSetKVKey()
		content := buildRulePayload(input.Rules)

		if err := publish.UploadContentViaWorker(r.Context(), workerBase, kvKey, token, content); err != nil {
			Err(w, http.StatusBadGateway, "RULESET_UPLOAD_FAILED", err.Error())
			return
		}

		accessURL := fmt.Sprintf("%s/%s?token=%s",
			strings.TrimRight(workerBase, "/"),
			url.PathEscape(kvKey),
			url.QueryEscape(token),
		)

		rs, err := deps.PublishStore.CreateRuleSet(input, cfg.WorkerName, cfg.Hostname, kvKey, accessURL)
		if err != nil {
			Err(w, http.StatusInternalServerError, "RULESET_STORE_WRITE_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"rule_set": rs,
		})
	}
}

func handleUpdateRuleSet(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.PublishStore == nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_UNAVAILABLE", "publish store not initialized")
			return
		}
		id := strings.TrimSpace(chi.URLParam(r, "id"))
		if id == "" {
			Err(w, http.StatusBadRequest, "RULESET_ID_REQUIRED", "ruleset id is required")
			return
		}

		var input publish.RuleSetInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}

		if len(input.Rules) == 0 {
			Err(w, http.StatusBadRequest, "RULESET_RULES_REQUIRED", "at least one rule is required")
			return
		}

		existing, ok := deps.PublishStore.GetRuleSet(id)
		if !ok {
			Err(w, http.StatusNotFound, "RULESET_NOT_FOUND", "rule set not found")
			return
		}

		cfg, token, err := deps.PublishStore.GetWorkerConfigWithToken(existing.WorkerConfigID)
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

		content := buildRulePayload(input.Rules)
		// Upload to the same KV key — URL never changes.
		if err := publish.UploadContentViaWorker(r.Context(), workerBase, existing.KVKey, token, content); err != nil {
			Err(w, http.StatusBadGateway, "RULESET_UPLOAD_FAILED", err.Error())
			return
		}

		rs, err := deps.PublishStore.UpdateRuleSetRules(id, input.Rules)
		if err != nil {
			Err(w, http.StatusInternalServerError, "RULESET_STORE_WRITE_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"rule_set": rs,
		})
	}
}

func handleDeleteRuleSet(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.PublishStore == nil {
			Err(w, http.StatusInternalServerError, "PUBLISH_STORE_UNAVAILABLE", "publish store not initialized")
			return
		}
		id := strings.TrimSpace(chi.URLParam(r, "id"))
		if id == "" {
			Err(w, http.StatusBadRequest, "RULESET_ID_REQUIRED", "ruleset id is required")
			return
		}

		existing, ok := deps.PublishStore.GetRuleSet(id)
		if !ok {
			Err(w, http.StatusNotFound, "RULESET_NOT_FOUND", "rule set not found")
			return
		}

		warning := ""
		cfg, token, err := deps.PublishStore.GetWorkerConfigWithToken(existing.WorkerConfigID)
		if err != nil {
			warning = "worker config not found, skipped remote cleanup"
		} else {
			workerBase := publish.PickWorkerBaseURL(cfg.WorkerURL, cfg.WorkerDevURL)
			if workerBase == "" {
				warning = "worker url missing, skipped remote cleanup"
			} else if strings.TrimSpace(token) == "" {
				warning = "worker token missing, skipped remote cleanup"
			} else if err := publish.DeleteContentViaWorker(r.Context(), workerBase, existing.KVKey, token); err != nil {
				warning = "remote delete failed: " + err.Error()
			}
		}

		if err := deps.PublishStore.DeleteRuleSet(id); err != nil {
			Err(w, http.StatusInternalServerError, "RULESET_STORE_DELETE_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"deleted": true,
			"warning": warning,
		})
	}
}
