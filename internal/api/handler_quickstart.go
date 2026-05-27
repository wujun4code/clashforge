package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/wujun4code/clashforge/internal/nodes"
	"github.com/wujun4code/clashforge/internal/quickstart"
)

// ── validate endpoints ────────────────────────────────────────────────────────

// handleQuickStartValidateCF validates a CF token and returns the zone list.
func handleQuickStartValidateCF() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req quickstart.ValidateCFRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		req.Token = strings.TrimSpace(req.Token)
		req.AccountID = strings.TrimSpace(req.AccountID)
		if req.Token == "" {
			Err(w, http.StatusBadRequest, "MISSING_TOKEN", "token is required")
			return
		}

		zones, err := nodes.CloudflareListZones(req.Token, req.AccountID)
		if err != nil {
			JSON(w, http.StatusOK, quickstart.ValidateCFResult{
				Valid: false,
				Error: err.Error(),
			})
			return
		}

		cfZones := make([]quickstart.CFZone, 0, len(zones))
		for _, z := range zones {
			cfZones = append(cfZones, quickstart.CFZone{ID: z.ID, Name: z.Name})
		}
		JSON(w, http.StatusOK, quickstart.ValidateCFResult{
			Valid: true,
			Zones: cfZones,
		})
	}
}

// handleQuickStartValidateVPS attempts an SSH connection and returns OS info.
func handleQuickStartValidateVPS() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req quickstart.ValidateVPSRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		if req.Host == "" {
			Err(w, http.StatusBadRequest, "MISSING_HOST", "host is required")
			return
		}
		if req.Port == 0 {
			req.Port = 22
		}
		if req.User == "" {
			req.User = "root"
		}

		creds := &quickstart.VPSCredentials{
			Host:     req.Host,
			Port:     req.Port,
			User:     req.User,
			AuthType: req.AuthType,
			Password: req.Password,
			PrivKey:  req.PrivKey,
		}

		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		_ = ctx

		sshClient, err := quickstart.DialSSH(creds)
		if err != nil {
			JSON(w, http.StatusOK, quickstart.ValidateVPSResult{
				Valid: false,
				Error: err.Error(),
			})
			return
		}
		defer sshClient.Close()

		env, err := quickstart.DetectEnv(sshClient)
		if err != nil {
			// Connection OK but detect failed — still report valid
			JSON(w, http.StatusOK, quickstart.ValidateVPSResult{
				Valid: true,
				OS:    "unknown",
			})
			return
		}

		JSON(w, http.StatusOK, quickstart.ValidateVPSResult{
			Valid:     true,
			OS:        env.OS,
			OSVersion: env.OSVersion,
			Arch:      env.Arch,
		})
	}
}

// ── deploy endpoint ───────────────────────────────────────────────────────────

// handleQuickStartDeploy starts a pipeline and streams events over SSE.
// The HTTP POST body is parsed once; the response is an SSE stream that remains
// open until the pipeline finishes or the client disconnects.
func handleQuickStartDeploy(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		var req quickstart.DeployRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		if req.DeployType == "" {
			Err(w, http.StatusBadRequest, "MISSING_DEPLOY_TYPE", "deploy_type is required")
			return
		}

		// Determine display name and host for state record
		nodeHost := ""
		switch req.DeployType {
		case quickstart.DeployTypeCFWorkers:
			nodeHost = req.WorkersDomain.CustomDomain
		case quickstart.DeployTypeVPS:
			if req.VPS != nil {
				nodeHost = req.VPS.Host
			}
		}
		if req.NodeName == "" {
			req.NodeName = nodeHost
		}

		// Create persistent state record
		state, err := deps.QuickStartStore.Create(req.DeployType, req.NodeName, nodeHost)
		if err != nil {
			Err(w, http.StatusInternalServerError, "STATE_CREATE_FAILED", err.Error())
			return
		}

		// Build pipeline
		pipelineDeps := quickstart.Deps{
			DataDir:     deps.Config.Core.DataDir,
			ConfigPath:  deps.ConfigPath,
			Config:      deps.Config,
			Core:        deps.Core,
			SubManager:  deps.SubManager,
			WorkerStore: deps.WorkerNodeStore,
		}
		pipeline, err := quickstart.NewPipeline(req.DeployType, pipelineDeps)
		if err != nil {
			Err(w, http.StatusBadRequest, "INVALID_DEPLOY_TYPE", err.Error())
			return
		}

		// Switch to SSE mode
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		// Send the deploy ID so the client can poll state later
		writeSSEEvent(w, flusher, "deploy_id", map[string]string{"id": state.ID})

		// Channel for pipeline → HTTP writer
		events := make(chan quickstart.Event, 64)

		pipelineCtx, pipelineCancel := context.WithTimeout(r.Context(), 20*time.Minute)
		defer pipelineCancel()

		// Run pipeline in background goroutine
		done := make(chan error, 1)
		go func() {
			done <- pipeline.Run(pipelineCtx, &req, func(e quickstart.Event) {
				select {
				case events <- e:
				case <-pipelineCtx.Done():
				}
			})
			close(events)
		}()

		// Stream events to the SSE client
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()

	streamLoop:
		for {
			select {
			case e, more := <-events:
				if !more {
					break streamLoop
				}
				writeSSEEvent(w, flusher, "event", e)
			case <-ticker.C:
				fmt.Fprintf(w, ": ping\n\n")
				flusher.Flush()
			case <-r.Context().Done():
				pipelineCancel()
				break streamLoop
			}
		}

		// Wait for pipeline to finish and record final state
		var pipelineErr error
		select {
		case pipelineErr = <-done:
		case <-time.After(5 * time.Second):
			pipelineErr = fmt.Errorf("pipeline did not finish in time after stream closed")
		}

		now := time.Now()
		finalStatus := "done"
		lastErr := ""
		if pipelineErr != nil {
			finalStatus = "failed"
			lastErr = pipelineErr.Error()
			log.Error().Err(pipelineErr).Str("deploy_id", state.ID).Msg("quickstart: pipeline failed")
		}

		_ = deps.QuickStartStore.Update(state.ID, func(s *quickstart.DeployState) {
			s.Status = finalStatus
			s.FinishedAt = &now
			s.LastError = lastErr
		})

		// Send final SSE event
		writeSSEEvent(w, flusher, "done", map[string]interface{}{
			"id":     state.ID,
			"status": finalStatus,
			"error":  lastErr,
		})
	}
}

// handleQuickStartGetDeploy returns a historical deploy record.
func handleQuickStartGetDeploy(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		state, ok := deps.QuickStartStore.Get(id)
		if !ok {
			Err(w, http.StatusNotFound, "NOT_FOUND", "deploy record not found")
			return
		}
		JSON(w, http.StatusOK, state)
	}
}

// handleQuickStartListDeploys returns all historical deploy records.
func handleQuickStartListDeploys(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]interface{}{
			"deploys": deps.QuickStartStore.List(),
		})
	}
}

// writeSSEEvent writes a single SSE event as JSON and flushes.
func writeSSEEvent(w http.ResponseWriter, f http.Flusher, eventType string, data interface{}) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, payload)
	f.Flush()
}
