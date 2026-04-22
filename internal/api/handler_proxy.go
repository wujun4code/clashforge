package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// proxyToMihomo fetches from mihomo, decodes the JSON, and re-encodes it in
// the standard {"ok":true,"data":...} envelope so the frontend request() helper works.
func proxyToMihomo(deps Dependencies, mihomoPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doProxyWrapped(w, r, deps.Config.Ports.MihomoAPI, mihomoPath)
	}
}

// proxyMihomoWithParam is like proxyToMihomo but interpolates a chi URL param into the path.
func proxyMihomoWithParam(deps Dependencies, prefix, suffix, paramName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		param := chi.URLParam(r, paramName)
		doProxyWrapped(w, r, deps.Config.Ports.MihomoAPI, prefix+param+suffix)
	}
}

func doProxyWrapped(w http.ResponseWriter, r *http.Request, port int, mihomoPath string) {
	target := fmt.Sprintf("http://127.0.0.1:%d%s", port, mihomoPath)
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		Err(w, http.StatusInternalServerError, "PROXY_ERROR", err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		Err(w, http.StatusBadGateway, "MIHOMO_UNAVAILABLE", "mihomo API unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		Err(w, http.StatusBadGateway, "MIHOMO_READ_ERROR", err.Error())
		return
	}

	// No-content responses (e.g. DELETE /connections returns 204)
	if resp.StatusCode == http.StatusNoContent || len(body) == 0 {
		JSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		Err(w, http.StatusBadGateway, "MIHOMO_PARSE_ERROR", err.Error())
		return
	}
	JSON(w, http.StatusOK, payload)
}
