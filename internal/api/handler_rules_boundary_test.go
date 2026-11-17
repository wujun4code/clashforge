package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wujun4code/clashforge/internal/config"
)

// These are ownership-boundary tests, NOT Mihomo payload-validation tests.
// PUT /providers/rules/<name> is a control request: an empty success response
// must not be mistaken for an empty downloaded provider. Mihomo owns that body.
func TestRuleProviderSyncDelegatesWithoutWritingProviderCache(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		for _, status := range []int{http.StatusOK, http.StatusNoContent, http.StatusBadGateway} {
			name := "single"
			if bulk {
				name = "bulk"
			}
			t.Run(name+"/"+http.StatusText(status), func(t *testing.T) {
				dir := t.TempDir()
				runtimeDir := t.TempDir()
				cache := filepath.Join(dir, "provider.yaml")
				original := []byte("payload:\n  - '+.example.test'\n")
				if err := os.WriteFile(cache, original, 0600); err != nil {
					t.Fatal(err)
				}
				var originCalls, updateCalls atomic.Int32
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					originCalls.Add(1)
					w.WriteHeader(http.StatusOK) // deliberately empty origin; API must not fetch it
				}))
				defer origin.Close()
				yaml := "rule-providers:\n  fixture:\n    type: http\n    behavior: domain\n    path: " + cache + "\n    url: " + origin.URL + "/provider.yaml\n"
				if err := os.WriteFile(filepath.Join(runtimeDir, "mihomo-config.yaml"), []byte(yaml), 0600); err != nil {
					t.Fatal(err)
				}
				mihomo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/providers/rules":
						_ = json.NewEncoder(w).Encode(map[string]any{"providers": map[string]any{
							"fixture": map[string]any{"vehicleType": "HTTP"},
							"local":   map[string]any{"vehicleType": "File"},
						}})
					case r.Method == http.MethodPut && r.URL.Path == "/providers/rules/fixture":
						updateCalls.Add(1)
						if status >= 400 {
							http.Error(w, "provider update failed: empty payload", status)
						} else {
							w.WriteHeader(status)
						}
					default:
						t.Errorf("unexpected control request: %s %s", r.Method, r.URL.Path)
						http.NotFound(w, r)
					}
				}))
				defer mihomo.Close()
				deps := Dependencies{Config: &config.MetaclashConfig{
					Core:  config.CoreConfig{RuntimeDir: runtimeDir, DataDir: dir},
					Ports: config.PortsConfig{MihomoAPI: mustServerPort(t, mihomo.URL)},
				}}
				rr := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/rules/providers/fixture/sync", nil)
				if bulk {
					handleSyncAllRuleProviders(deps).ServeHTTP(rr, req)
					var response struct {
						Data struct {
							Results []struct {
								Name  string `json:"name"`
								OK    bool   `json:"ok"`
								Error string `json:"error"`
							} `json:"results"`
						} `json:"data"`
					}
					if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
					if len(response.Data.Results) != 1 {
						t.Fatalf("expected HTTP provider only: %s", rr.Body.String())
					}
					result := response.Data.Results[0]
					if result.Name != "fixture" || result.OK != (status < 400) || (status >= 400 && result.Error == "") {
						t.Fatalf("incorrect per-provider result: %s", rr.Body.String())
					}
				} else {
					route := chi.NewRouteContext()
					route.URLParams.Add("name", "fixture")
					req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
					handleSyncRuleProvider(deps).ServeHTTP(rr, req)
					wantStatus := http.StatusOK
					if status >= 400 {
						wantStatus = http.StatusBadGateway
					}
					if rr.Code != wantStatus {
						t.Fatalf("status=%d want=%d body=%s", rr.Code, wantStatus, rr.Body.String())
					}
					if status >= 400 && !bytes.Contains(rr.Body.Bytes(), []byte("SYNC_FAILED")) {
						t.Fatal("missing failure report")
					}
				}
				if updateCalls.Load() != 1 {
					t.Fatalf("control calls=%d want=1", updateCalls.Load())
				}
				if originCalls.Load() != 0 {
					t.Fatalf("API unexpectedly downloaded origin: %d", originCalls.Load())
				}
				got, err := os.ReadFile(cache)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, original) {
					t.Fatal("API changed provider cache it does not own")
				}
			})
		}
	}
}
