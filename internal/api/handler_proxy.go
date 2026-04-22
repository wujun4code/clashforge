package api

import (
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func proxyToMihomo(deps Dependencies, mihomoPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doProxy(w, r, deps.Config.Ports.MihomoAPI, mihomoPath)
	}
}

// proxyMihomoWithParam builds the mihomo path as prefix + urlParam + suffix.
func proxyMihomoWithParam(deps Dependencies, prefix, suffix, paramName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		param := chi.URLParam(r, paramName)
		doProxy(w, r, deps.Config.Ports.MihomoAPI, prefix+param+suffix)
	}
}

func doProxy(w http.ResponseWriter, r *http.Request, port int, mihomoPath string) {
	target := fmt.Sprintf("http://127.0.0.1:%d%s", port, mihomoPath)
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		Err(w, http.StatusInternalServerError, "PROXY_ERROR", err.Error())
		return
	}
	for k, vv := range r.Header {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		Err(w, http.StatusBadGateway, "MIHOMO_UNAVAILABLE", "mihomo API unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
