package workernode

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/wujun4code/clashforge/internal/publish"
)

// Deploy creates a VLESS-WS Worker on Cloudflare and binds the custom hostname.
// It returns an updated WorkerNode with Status, URLs, and UUID populated.
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

	deployRes, err := cf.DeployRawWorkerScript(ctx, req.CFAccountID, req.WorkerName, VlessWorkerScript,
		map[string]string{"UUID": nodeUUID})
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
		CFToken:      req.CFToken,
		CFAccountID:  req.CFAccountID,
		CFZoneID:     req.CFZoneID,
		Hostname:     req.Hostname,
		WorkerURL:    bindRes.WorkerURL,
		WorkerDevURL: deployRes.WorkerDevURL,
		Status:       StatusDeployed,
	}
	return node, nil
}

// Redeploy re-uploads the Worker script preserving the existing UUID.
func Redeploy(ctx context.Context, node *WorkerNode) error {
	cf, err := publish.NewCloudflareClient(node.CFToken)
	if err != nil {
		return err
	}
	_, err = cf.DeployRawWorkerScript(ctx, node.CFAccountID, node.WorkerName, VlessWorkerScript,
		map[string]string{"UUID": node.WorkerUUID})
	return err
}

// Destroy deletes the CF Worker script (domain binding is cleaned up by CF automatically).
func Destroy(ctx context.Context, node *WorkerNode) error {
	cf, err := publish.NewCloudflareClient(node.CFToken)
	if err != nil {
		return err
	}
	return cf.DeleteWorkerScript(ctx, node.CFAccountID, node.WorkerName)
}
