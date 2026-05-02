package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// AzureConfig holds Azure Service Principal credentials persisted to disk.
// Stored at /etc/metaclash/azure-config.json with mode 0600.
type AzureConfig struct {
	TenantID       string `json:"tenant_id"`
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	SubscriptionID string `json:"subscription_id"`
}

func azureConfigPath(dataDir string) string {
	return filepath.Join(dataDir, "azure-config.json")
}

func handleGetAzureConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := azureConfigPath(deps.Config.Core.DataDir)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			JSON(w, http.StatusOK, AzureConfig{})
			return
		}
		if err != nil {
			Err(w, http.StatusInternalServerError, "internal", "read azure-config: "+err.Error())
			return
		}
		var cfg AzureConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			Err(w, http.StatusInternalServerError, "internal", "parse azure-config: "+err.Error())
			return
		}
		// Never send client_secret back to frontend
		cfg.ClientSecret = ""
		JSON(w, http.StatusOK, cfg)
	}
}

func handlePutAzureConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cfg AzureConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			Err(w, http.StatusBadRequest, "bad_request", "invalid JSON: "+err.Error())
			return
		}
		if cfg.TenantID == "" || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.SubscriptionID == "" {
			Err(w, http.StatusBadRequest, "bad_request", "tenant_id, client_id, client_secret, subscription_id 均不能为空")
			return
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			Err(w, http.StatusInternalServerError, "internal", "marshal azure-config: "+err.Error())
			return
		}
		path := azureConfigPath(deps.Config.Core.DataDir)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			Err(w, http.StatusInternalServerError, "internal", "write azure-config: "+err.Error())
			return
		}
		// Return sanitized (no secret)
		cfg.ClientSecret = ""
		JSON(w, http.StatusOK, cfg)
	}
}

func handleDeleteAzureConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := azureConfigPath(deps.Config.Core.DataDir)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			Err(w, http.StatusInternalServerError, "internal", "delete azure-config: "+err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// loadAzureConfig reads the persisted AzureConfig (including secret) for internal use.
func loadAzureConfig(dataDir string) (*AzureConfig, error) {
	path := azureConfigPath(dataDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg AzureConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
