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
// When direct credentials are provided (no node_id), the node is upserted into
// the node store on success so users can reuse it without re-entering credentials.
func handleQuickStartValidateVPS(store *nodes.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req quickstart.ValidateVPSRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		creds, credErr := resolveValidateVPSCreds(req, store)
		if credErr != nil {
			JSON(w, http.StatusOK, quickstart.ValidateVPSResult{
				Valid: false,
				Error: credErr.Error(),
			})
			return
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

		result := quickstart.ValidateVPSResult{
			Valid:     true,
			OS:        env.OS,
			OSVersion: env.OSVersion,
			Arch:      env.Arch,
		}

		// Save to node store when the user typed fresh credentials (not selecting an existing node).
		// This lets them reuse the node in future QuickStart sessions without re-entering credentials.
		if strings.TrimSpace(req.NodeID) == "" && store != nil &&
			strings.EqualFold(strings.TrimSpace(req.AuthType), "password") &&
			strings.TrimSpace(req.Password) != "" {
			nodeID := upsertSSHNode(store, req)
			result.NodeID = nodeID
		}

		JSON(w, http.StatusOK, result)
	}
}

// upsertSSHNode creates or updates a managed node entry from validated SSH credentials.
// Returns the node ID (new or existing).
func upsertSSHNode(store *nodes.Store, req quickstart.ValidateVPSRequest) string {
	host := strings.TrimSpace(req.Host)
	port := req.Port
	if port == 0 {
		port = 22
	}
	user := strings.TrimSpace(req.User)
	if user == "" {
		user = "root"
	}

	// Find existing node by host:port
	for _, item := range store.List() {
		if item.Kind == nodes.NodeKindExternal {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.Host), host) && item.Port == port {
			existing, ok := store.Get(item.ID)
			if !ok {
				continue
			}
			existing.Username = user
			existing.Password = req.Password
			existing.Kind = nodes.NodeKindManaged
			_ = store.Update(item.ID, existing)
			return item.ID
		}
	}

	n := &nodes.Node{
		Name:     host,
		Host:     host,
		Port:     port,
		Username: user,
		Password: req.Password,
		Kind:     nodes.NodeKindManaged,
	}
	if err := store.Create(n); err != nil {
		log.Warn().Err(err).Str("host", host).Msg("quickstart: failed to save node on SSH validate")
		return ""
	}
	return n.ID
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
		if req.DeployType == quickstart.DeployTypeVPS {
			if err := hydrateQuickStartVPSRequest(&req, deps.NodeStore); err != nil {
				Err(w, http.StatusBadRequest, "INVALID_VPS_REQUEST", err.Error())
				return
			}
		}

		// Normalize zone context so quickstart publish worker always binds to a
		// random subdomain under the node's base domain (never accidental workers.dev fallback).
		preferredDomain := quickStartNodeDomain(&req)
		if req.DeployType == quickstart.DeployTypeVPS && preferredDomain == "" && req.VPSNodeID != "" && deps.NodeStore != nil {
			if n, ok := deps.NodeStore.Get(strings.TrimSpace(req.VPSNodeID)); ok {
				preferredDomain = strings.TrimSpace(n.Domain)
			}
		}
		if err := normalizeQuickStartCFZone(&req, preferredDomain); err != nil {
			Err(w, http.StatusBadRequest, "CF_ZONE_MISMATCH", err.Error())
			return
		}
		if strings.TrimSpace(req.Cloudflare.ZoneID) == "" || strings.TrimSpace(req.Cloudflare.ZoneName) == "" {
			Err(w, http.StatusBadRequest, "MISSING_CF_ZONE", "cloudflare zone_id and zone_name are required")
			return
		}
		if strings.TrimSpace(preferredDomain) != "" &&
			!quickstart.DomainMatchesZone(preferredDomain, req.Cloudflare.ZoneName) {
			Err(w, http.StatusBadRequest, "CF_ZONE_MISMATCH",
				fmt.Sprintf("节点域名 %s 与订阅托管一级域名 %s 不一致，请使用相同一级域名",
					preferredDomain, req.Cloudflare.ZoneName))
			return
		}

		// Determine display name and host for state record
		nodeHost := ""
		switch req.DeployType {
		case quickstart.DeployTypeCFWorkers:
			nodeHost = req.Cloudflare.ZoneName // will be auto-filled with generated subdomain later
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
			DataDir:      deps.Config.Core.DataDir,
			ConfigPath:   deps.ConfigPath,
			Config:       deps.Config,
			Core:         deps.Core,
			SubManager:   deps.SubManager,
			WorkerStore:  deps.WorkerNodeStore,
			PublishStore: deps.PublishStore,
			Netfilter:    deps.Netfilter,
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

		if req.DeployType == quickstart.DeployTypeVPS {
			if err := persistQuickStartVPSNode(deps.NodeStore, &req, pipelineErr); err != nil {
				log.Warn().Err(err).Str("deploy_id", state.ID).Msg("quickstart: failed to persist vps node auth")
			}
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

// handleQuickStartCheckNode synchronously runs the existing-deployment check
// (DNS + TCP + TLS cert validity) and returns the result.
func handleQuickStartCheckNode() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req quickstart.CheckNodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}
		if req.Domain == "" {
			Err(w, http.StatusBadRequest, "MISSING_DOMAIN", "domain is required")
			return
		}
		result := quickstart.CheckExistingDeployment(r.Context(), req.Domain, req.VPSIP, func(quickstart.Event) {})
		JSON(w, http.StatusOK, result)
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

func resolveValidateVPSCreds(req quickstart.ValidateVPSRequest, store *nodes.Store) (*quickstart.VPSCredentials, error) {
	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID != "" {
		if store == nil {
			return nil, fmt.Errorf("node store unavailable")
		}
		node, ok := store.Get(nodeID)
		if !ok {
			return nil, fmt.Errorf("托管节点不存在")
		}
		if node.IsExternal() {
			return nil, fmt.Errorf("当前仅支持选择托管节点")
		}
		if strings.TrimSpace(node.Password) == "" {
			return nil, fmt.Errorf("该托管节点未保存密码，请先在托管节点页更新密码")
		}

		port := node.Port
		if port == 0 {
			port = 22
		}
		user := strings.TrimSpace(node.Username)
		if user == "" {
			user = "root"
		}
		return &quickstart.VPSCredentials{
			Host:     strings.TrimSpace(node.Host),
			Port:     port,
			User:     user,
			AuthType: "password",
			Password: node.Password,
		}, nil
	}

	host := strings.TrimSpace(req.Host)
	if host == "" {
		return nil, fmt.Errorf("host is required")
	}
	port := req.Port
	if port == 0 {
		port = 22
	}
	user := strings.TrimSpace(req.User)
	if user == "" {
		user = "root"
	}
	authType := strings.TrimSpace(req.AuthType)
	if authType == "" {
		authType = "password"
	}

	return &quickstart.VPSCredentials{
		Host:     host,
		Port:     port,
		User:     user,
		AuthType: authType,
		Password: req.Password,
		PrivKey:  req.PrivKey,
	}, nil
}

func hydrateQuickStartVPSRequest(req *quickstart.DeployRequest, store *nodes.Store) error {
	if req == nil || req.DeployType != quickstart.DeployTypeVPS {
		return nil
	}

	req.VPSNodeID = strings.TrimSpace(req.VPSNodeID)
	if req.VPSNodeID != "" {
		if store == nil {
			return fmt.Errorf("node store unavailable")
		}
		node, ok := store.Get(req.VPSNodeID)
		if !ok {
			return fmt.Errorf("托管节点不存在")
		}
		if node.IsExternal() {
			return fmt.Errorf("当前仅支持选择托管节点")
		}

		if strings.TrimSpace(req.Cloudflare.ZoneID) == "" && strings.TrimSpace(node.CFZoneID) != "" {
			req.Cloudflare.ZoneID = strings.TrimSpace(node.CFZoneID)
		}

		// Always update the node's domain and proxy config from this deploy request.
		updateNodeProxyConfig(store, node, req)

		if strings.TrimSpace(req.NodeName) == "" {
			req.NodeName = strings.TrimSpace(node.Name)
		}

		// Pre-fill existing proxy credentials so the pipeline can reuse them
		// (ForceImport and CheckExistingDeployment reusable paths).
		if node.ProxyUser != "" {
			req.ProxyUser = node.ProxyUser
			req.ProxyPassword = node.ProxyPassword
		}

		// ForceImport skips all SSH phases — no credentials needed.
		if req.ForceImport {
			if strings.TrimSpace(req.ProxyUser) == "" || strings.TrimSpace(req.ProxyPassword) == "" {
				return fmt.Errorf("所选托管节点缺少代理用户名/密码，请先执行完整部署以生成认证信息")
			}
			return nil
		}

		if strings.TrimSpace(node.Password) == "" {
			return fmt.Errorf("所选托管节点未保存密码，请先在托管节点页更新密码")
		}

		port := node.Port
		if port == 0 {
			port = 22
		}
		user := strings.TrimSpace(node.Username)
		if user == "" {
			user = "root"
		}
		req.VPS = &quickstart.VPSCredentials{
			Host:     strings.TrimSpace(node.Host),
			Port:     port,
			User:     user,
			AuthType: "password",
			Password: node.Password,
		}
		return nil
	}

	if req.VPS == nil {
		return fmt.Errorf("vps credentials required")
	}
	req.VPS.Host = strings.TrimSpace(req.VPS.Host)
	req.VPS.User = strings.TrimSpace(req.VPS.User)
	req.VPS.AuthType = strings.TrimSpace(req.VPS.AuthType)
	if req.VPS.Host == "" {
		return fmt.Errorf("vps host is required")
	}
	if req.VPS.Port == 0 {
		req.VPS.Port = 22
	}
	if req.VPS.User == "" {
		req.VPS.User = "root"
	}
	if req.VPS.AuthType == "" {
		req.VPS.AuthType = "password"
	}

	if err := upsertQuickStartManagedNode(store, req); err != nil {
		log.Warn().Err(err).Str("host", req.VPS.Host).Msg("quickstart: failed to upsert managed node from vps input")
	}
	return nil
}

// updateNodeProxyConfig updates domain, name, ProxyType, and ProxyPort on an existing node.
func updateNodeProxyConfig(store *nodes.Store, node *nodes.Node, req *quickstart.DeployRequest) {
	domain := ""
	if req.NodePrefix != "" && req.Cloudflare.ZoneName != "" {
		domain = req.NodePrefix + "." + req.Cloudflare.ZoneName
	} else if req.Cloudflare.ZoneName != "" {
		domain = req.Cloudflare.ZoneName
	}
	if domain != "" {
		node.Domain = domain
	}
	if n := strings.TrimSpace(req.NodeName); n != "" {
		node.Name = n
	}
	node.ProxyType = "http"
	node.ProxyPort = 443
	if z := strings.TrimSpace(req.Cloudflare.ZoneID); z != "" {
		node.CFZoneID = z
	}
	if a := strings.TrimSpace(req.Cloudflare.AccountID); a != "" {
		node.CFAccountID = a
	}
	_ = store.Update(node.ID, node)
}

func upsertQuickStartManagedNode(store *nodes.Store, req *quickstart.DeployRequest) error {
	if store == nil || req == nil || req.VPS == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(req.VPS.AuthType), "password") || strings.TrimSpace(req.VPS.Password) == "" {
		// Only password-based credentials are persisted in node store today.
		return nil
	}

	host := strings.TrimSpace(req.VPS.Host)
	port := req.VPS.Port
	if port == 0 {
		port = 22
	}
	user := strings.TrimSpace(req.VPS.User)
	if user == "" {
		user = "root"
	}
	name := strings.TrimSpace(req.NodeName)
	if name == "" {
		name = host
	}

	// Compute the FQDN that will be deployed (prefix.zone or zone).
	domain := ""
	if req.NodePrefix != "" && req.Cloudflare.ZoneName != "" {
		domain = req.NodePrefix + "." + req.Cloudflare.ZoneName
	} else if req.Cloudflare.ZoneName != "" {
		domain = req.Cloudflare.ZoneName
	}

	var existingID string
	for _, item := range store.List() {
		if item.Kind == nodes.NodeKindExternal {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.Host), host) && item.Port == port {
			existingID = item.ID
			break
		}
	}
	if existingID != "" {
		existing, ok := store.Get(existingID)
		if !ok {
			return nil
		}
		existing.Name = name
		existing.Host = host
		existing.Port = port
		existing.Username = user
		existing.Password = req.VPS.Password
		existing.Kind = nodes.NodeKindManaged
		if domain != "" {
			existing.Domain = domain
		}
		existing.ProxyType = "http"
		existing.ProxyPort = 443
		if z := strings.TrimSpace(req.Cloudflare.ZoneID); z != "" {
			existing.CFZoneID = z
		}
		if a := strings.TrimSpace(req.Cloudflare.AccountID); a != "" {
			existing.CFAccountID = a
		}
		return store.Update(existingID, existing)
	}

	return store.Create(&nodes.Node{
		Name:        name,
		Host:        host,
		Port:        port,
		Username:    user,
		Password:    req.VPS.Password,
		Kind:        nodes.NodeKindManaged,
		Domain:      domain,
		CFAccountID: strings.TrimSpace(req.Cloudflare.AccountID),
		CFZoneID:    strings.TrimSpace(req.Cloudflare.ZoneID),
		ProxyType:   "http",
		ProxyPort:   443,
	})
}

func persistQuickStartVPSNode(store *nodes.Store, req *quickstart.DeployRequest, pipelineErr error) error {
	if store == nil || req == nil || req.DeployType != quickstart.DeployTypeVPS {
		return nil
	}

	nodeID := strings.TrimSpace(req.VPSNodeID)
	if nodeID == "" {
		nodeID = findManagedNodeIDByVPS(store, req.VPS)
	}
	if nodeID == "" {
		return nil
	}

	node, ok := store.Get(nodeID)
	if !ok {
		return nil
	}
	if node.IsExternal() {
		return nil
	}

	if n := strings.TrimSpace(req.NodeName); n != "" {
		node.Name = n
	}
	if domain := quickStartNodeDomain(req); domain != "" {
		node.Domain = domain
	}
	node.ProxyType = "http"
	node.ProxyPort = 443
	if z := strings.TrimSpace(req.Cloudflare.ZoneID); z != "" {
		node.CFZoneID = z
	}
	if a := strings.TrimSpace(req.Cloudflare.AccountID); a != "" {
		node.CFAccountID = a
	}
	if strings.TrimSpace(req.ProxyUser) != "" && strings.TrimSpace(req.ProxyPassword) != "" {
		node.ProxyUser = req.ProxyUser
		node.ProxyPassword = req.ProxyPassword
	}

	if pipelineErr != nil {
		node.Status = nodes.StatusError
		node.Error = pipelineErr.Error()
	} else {
		now := time.Now()
		node.Status = nodes.StatusDeployed
		node.DeployedAt = &now
		node.Error = ""
	}
	return store.Update(node.ID, node)
}

func quickStartNodeDomain(req *quickstart.DeployRequest) string {
	if req == nil {
		return ""
	}
	prefix := strings.TrimSpace(req.NodePrefix)
	zone := strings.TrimSpace(req.Cloudflare.ZoneName)
	if zone == "" {
		return ""
	}
	if prefix == "" {
		return zone
	}
	return prefix + "." + zone
}

func findManagedNodeIDByVPS(store *nodes.Store, vps *quickstart.VPSCredentials) string {
	if store == nil || vps == nil {
		return ""
	}
	host := strings.TrimSpace(vps.Host)
	if host == "" {
		return ""
	}
	port := vps.Port
	if port == 0 {
		port = 22
	}
	for _, item := range store.List() {
		if item.Kind == nodes.NodeKindExternal {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.Host), host) && item.Port == port {
			return item.ID
		}
	}
	return ""
}

// normalizeQuickStartCFZone resolves and normalizes zone_id + zone_name using
// Cloudflare zones, preferring the zone that best matches preferredDomain.
func normalizeQuickStartCFZone(req *quickstart.DeployRequest, preferredDomain string) error {
	if req == nil {
		return nil
	}
	token := strings.TrimSpace(req.Cloudflare.Token)
	if token == "" {
		return nil
	}

	zoneID := strings.TrimSpace(req.Cloudflare.ZoneID)
	zoneName := strings.ToLower(strings.Trim(strings.TrimSpace(req.Cloudflare.ZoneName), "."))
	preferredDomain = strings.ToLower(strings.Trim(strings.TrimSpace(preferredDomain), "."))

	if zoneID != "" && zoneName != "" && (preferredDomain == "" || quickstart.DomainMatchesZone(preferredDomain, zoneName)) {
		req.Cloudflare.ZoneName = zoneName
		return nil
	}

	zones, err := nodes.CloudflareListZones(token, strings.TrimSpace(req.Cloudflare.AccountID))
	if err != nil {
		return err
	}
	if len(zones) == 0 {
		return fmt.Errorf("cloudflare zones list is empty")
	}

	zoneByID := make(map[string]nodes.CloudflareZone, len(zones))
	for _, z := range zones {
		id := strings.TrimSpace(z.ID)
		if id == "" {
			continue
		}
		zoneByID[id] = z
	}

	if zoneID != "" {
		if z, ok := zoneByID[zoneID]; ok {
			zoneName = strings.ToLower(strings.Trim(strings.TrimSpace(z.Name), "."))
		}
	}

	if preferredDomain != "" {
		z, ok := bestMatchingCloudflareZone(zones, preferredDomain)
		if !ok {
			return fmt.Errorf("cloudflare 账号中未找到与节点域名 %s 匹配的一级域名", preferredDomain)
		}
		zoneID = strings.TrimSpace(z.ID)
		zoneName = strings.ToLower(strings.Trim(strings.TrimSpace(z.Name), "."))
	}

	if zoneID == "" && zoneName != "" {
		if z, ok := bestMatchingCloudflareZone(zones, zoneName); ok {
			zoneID = strings.TrimSpace(z.ID)
			zoneName = strings.ToLower(strings.Trim(strings.TrimSpace(z.Name), "."))
		}
	}

	if zoneName == "" && zoneID != "" {
		if z, ok := zoneByID[zoneID]; ok {
			zoneName = strings.ToLower(strings.Trim(strings.TrimSpace(z.Name), "."))
		}
	}

	if zoneID == "" || zoneName == "" {
		return fmt.Errorf("cloudflare zone resolution failed")
	}
	if preferredDomain != "" && !quickstart.DomainMatchesZone(preferredDomain, zoneName) {
		return fmt.Errorf("节点域名 %s 与订阅托管一级域名 %s 不一致，请使用相同一级域名", preferredDomain, zoneName)
	}

	req.Cloudflare.ZoneID = zoneID
	req.Cloudflare.ZoneName = zoneName
	return nil
}

func bestMatchingCloudflareZone(zones []nodes.CloudflareZone, domain string) (nodes.CloudflareZone, bool) {
	domain = strings.ToLower(strings.Trim(strings.TrimSpace(domain), "."))
	bestIdx := -1
	bestLen := -1
	for i, z := range zones {
		name := strings.ToLower(strings.Trim(strings.TrimSpace(z.Name), "."))
		if !quickstart.DomainMatchesZone(domain, name) {
			continue
		}
		if len(name) > bestLen {
			bestIdx = i
			bestLen = len(name)
		}
	}
	if bestIdx < 0 {
		return nodes.CloudflareZone{}, false
	}
	return zones[bestIdx], true
}
