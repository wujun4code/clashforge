package workernode

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wujun4code/clashforge/internal/publish"
)

// Deploy creates a VLESS-WS Worker on Cloudflare and binds the custom hostname.
// It returns an updated WorkerNode with Status, URLs, UUID, and AesKey populated.
// If req.ExpiresInDays > 0, EXPIRES_AT is injected as an env binding.
func Deploy(ctx context.Context, req *CreateRequest) (*WorkerNode, error) {
	req.CFToken = strings.TrimSpace(req.CFToken)
	req.CFAccountID = strings.TrimSpace(req.CFAccountID)
	req.CFZoneID = strings.TrimSpace(req.CFZoneID)
	req.WorkerName = strings.TrimSpace(req.WorkerName)
	req.Hostname = strings.TrimSpace(req.Hostname)

	if req.CFToken == "" || req.CFAccountID == "" || req.WorkerName == "" || req.Hostname == "" {
		return nil, fmt.Errorf("cf_token, cf_account_id, worker_name and hostname are required")
	}

	cf, err := publish.NewCloudflareClient(req.CFToken)
	if err != nil {
		return nil, err
	}

	nodeUUID := uuid.New().String()
	aesKey, err := generateAESKey()
	if err != nil {
		return nil, fmt.Errorf("generate aes key: %w", err)
	}

	bindings := map[string]string{
		"UUID":    nodeUUID,
		"AES_KEY": aesKey,
	}

	var expiresAt *time.Time
	if req.ExpiresInDays > 0 {
		t := time.Now().UTC().Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour)
		expiresAt = &t
		bindings["EXPIRES_AT"] = t.Format(time.RFC3339)
	}

	deployRes, err := cf.DeployRawWorkerScript(ctx, req.CFAccountID, req.WorkerName, VlessWorkerScript, bindings)
	if err != nil {
		return nil, fmt.Errorf("deploy worker script: %w", err)
	}

	bindRes, err := cf.BindWorkerDomain(ctx, req.CFAccountID, req.CFZoneID, req.WorkerName, req.Hostname)
	if err != nil {
		return nil, fmt.Errorf("bind worker domain: %w", err)
	}

	node := &WorkerNode{
		Name:         req.Name,
		WorkerName:   req.WorkerName,
		WorkerUUID:   nodeUUID,
		AesKey:       aesKey,
		CFToken:      req.CFToken,
		CFAccountID:  req.CFAccountID,
		CFZoneID:     req.CFZoneID,
		Hostname:     req.Hostname,
		WorkerURL:    bindRes.WorkerURL,
		WorkerDevURL: deployRes.WorkerDevURL,
		Status:       StatusDeployed,
		ExpiresAt:    expiresAt,
	}
	return node, nil
}

// Redeploy re-uploads the Worker script preserving the existing UUID, AesKey, and ExpiresAt.
func Redeploy(ctx context.Context, node *WorkerNode) error {
	cf, err := publish.NewCloudflareClient(node.CFToken)
	if err != nil {
		return err
	}
	bindings := map[string]string{
		"UUID":    node.WorkerUUID,
		"AES_KEY": node.AesKey,
	}
	if node.ExpiresAt != nil {
		bindings["EXPIRES_AT"] = node.ExpiresAt.UTC().Format(time.RFC3339)
	}
	_, err = cf.DeployRawWorkerScript(ctx, node.CFAccountID, node.WorkerName, VlessWorkerScript, bindings)
	return err
}

// RenewExpiry re-deploys the Worker script with a new EXPIRES_AT binding set to
// now + expiresInDays. All other bindings (UUID, AES_KEY) are preserved.
// Returns the new expiry time.
func RenewExpiry(ctx context.Context, node *WorkerNode, expiresInDays int) (time.Time, error) {
	if expiresInDays <= 0 {
		return time.Time{}, fmt.Errorf("expires_in_days must be > 0")
	}
	cf, err := publish.NewCloudflareClient(node.CFToken)
	if err != nil {
		return time.Time{}, err
	}
	newExpiry := time.Now().UTC().Add(time.Duration(expiresInDays) * 24 * time.Hour)
	bindings := map[string]string{
		"UUID":       node.WorkerUUID,
		"AES_KEY":    node.AesKey,
		"EXPIRES_AT": newExpiry.Format(time.RFC3339),
	}
	if _, err := cf.DeployRawWorkerScript(ctx, node.CFAccountID, node.WorkerName, VlessWorkerScript, bindings); err != nil {
		return time.Time{}, fmt.Errorf("renew expiry: %w", err)
	}
	return newExpiry, nil
}

// Destroy deletes the CF Worker script (domain binding is cleaned up by CF automatically).
func Destroy(ctx context.Context, node *WorkerNode) error {
	cf, err := publish.NewCloudflareClient(node.CFToken)
	if err != nil {
		return err
	}
	return cf.DeleteWorkerScript(ctx, node.CFAccountID, node.WorkerName)
}

// generateAESKey returns a cryptographically random 32-byte key as a 64-char hex string.
func generateAESKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}
