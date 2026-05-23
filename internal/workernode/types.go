package workernode

import "time"

type Status string

const (
	StatusPending  Status = "pending"
	StatusDeployed Status = "deployed"
	StatusError    Status = "error"
)

// WorkerNode is a Cloudflare Worker-based VLESS-over-WebSocket proxy node.
type WorkerNode struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	WorkerName   string     `json:"worker_name"`
	WorkerUUID   string     `json:"-"` // secret: VLESS UUID, encrypted at rest
	CFToken      string     `json:"-"` // secret, encrypted at rest
	AesKey       string     `json:"-"` // secret: 32-byte hex key for /sub encryption, encrypted at rest
	CFAccountID  string     `json:"cf_account_id"`
	CFZoneID     string     `json:"cf_zone_id"`
	Hostname     string     `json:"hostname"`
	WorkerURL    string     `json:"worker_url"`
	WorkerDevURL string     `json:"worker_dev_url"`
	Status       Status     `json:"status"`
	Error        string     `json:"error,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	DeployedAt   *time.Time `json:"deployed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// WorkerNodeListItem is the safe-for-frontend summary.
type WorkerNodeListItem struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	WorkerName   string     `json:"worker_name"`
	CFAccountID  string     `json:"cf_account_id"`
	Hostname     string     `json:"hostname"`
	WorkerURL    string     `json:"worker_url"`
	WorkerDevURL string     `json:"worker_dev_url"`
	Status       Status     `json:"status"`
	Error        string     `json:"error,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	DeployedAt   *time.Time `json:"deployed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CreateRequest is the body for POST /api/v1/worker-nodes.
type CreateRequest struct {
	Name          string `json:"name"`
	WorkerName    string `json:"worker_name"`
	CFToken       string `json:"cf_token"`
	CFAccountID   string `json:"cf_account_id"`
	CFZoneID      string `json:"cf_zone_id"`
	Hostname      string `json:"hostname"`
	ExpiresInDays int    `json:"expires_in_days,omitempty"` // 0 = no expiry
}

// RenewExpiryRequest is the body for POST /api/v1/worker-nodes/:id/renew-expiry.
type RenewExpiryRequest struct {
	ExpiresInDays int `json:"expires_in_days"` // days from now
}

// FreeTierInfo contains the secrets needed by CI to bake the free node into the APK.
// Only returned by GET /api/v1/worker-nodes/:id/free-tier-info.
type FreeTierInfo struct {
	SubURL    string `json:"sub_url"`    // https://<hostname>/sub
	AesKey    string `json:"aes_key"`    // 64-char hex (32 bytes)
	ExpiresAt string `json:"expires_at"` // RFC3339 or empty
}

// ClashProxyConfig is the Clash proxy YAML snippet for this node.
type ClashProxyConfig struct {
	YAML string `json:"yaml"`
	Name string `json:"name"`
}
