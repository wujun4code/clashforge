package api

import "net/http"

func handleGetConfig(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, deps.Config.Redacted())
	}
}
