package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// CFConfig is the Cloudflare credential set persisted to /etc/metaclash/cf-config.json.
// The file is readable only by root (mode 0600) so the token is not world-readable.
type CFConfig struct {
	CFToken     string `json:"cf_token"`
	CFAccountID string `json:"cf_account_id"`
	ACMEEmail   string `json:"acme_email"`
}

func cfConfigPath(dataDir string) string {
	return filepath.Join(dataDir, "cf-config.json")
}

func handleGetCFConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := cfConfigPath(deps.Config.Core.DataDir)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			JSON(w, http.StatusOK, CFConfig{})
			return
		}
		if err != nil {
			Err(w, http.StatusInternalServerError, "internal", "read cf-config: "+err.Error())
			return
		}
		var cfg CFConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			Err(w, http.StatusInternalServerError, "internal", "parse cf-config: "+err.Error())
			return
		}
		JSON(w, http.StatusOK, cfg)
	}
}

func handlePutCFConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cfg CFConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			Err(w, http.StatusBadRequest, "bad_request", "invalid JSON: "+err.Error())
			return
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			Err(w, http.StatusInternalServerError, "internal", "marshal cf-config: "+err.Error())
			return
		}
		path := cfConfigPath(deps.Config.Core.DataDir)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			Err(w, http.StatusInternalServerError, "internal", "write cf-config: "+err.Error())
			return
		}
		JSON(w, http.StatusOK, cfg)
	}
}

func handleDeleteCFConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := cfConfigPath(deps.Config.Core.DataDir)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			Err(w, http.StatusInternalServerError, "internal", "delete cf-config: "+err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
