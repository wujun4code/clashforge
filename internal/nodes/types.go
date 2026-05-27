package nodes

import "time"

// Status represents the deployment state of a node.
type Status string

const (
	StatusPending   Status = "pending"   // created, not yet connected
	StatusConnected Status = "connected" // SSH test passed
	StatusDeploying Status = "deploying" // GOST install in progress
	StatusDeployed  Status = "deployed"  // GOST + cert deployed successfully
	StatusError     Status = "error"     // last operation failed
)

// NodeKind distinguishes between nodes fully managed by ClashForge and nodes
// that were manually deployed and are only accessible via SSH for diagnostics.
type NodeKind string

const (
	NodeKindManaged  NodeKind = "managed"  // ClashForge created & manages GOST lifecycle
	NodeKindExternal NodeKind = "external" // manually deployed; SSH-only access for diag
)

// Node represents a remote Linux server managed by ClashForge.
type Node struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	Username    string   `json:"username"`
	Password    string   `json:"-"` // encrypted at rest, never sent to frontend
	Domain      string   `json:"domain"`
	Email       string   `json:"email"`
	CFToken     string   `json:"-"` // encrypted at rest
	CFAccountID string   `json:"cf_account_id"`
	CFZoneID    string   `json:"cf_zone_id"`
	Kind        NodeKind `json:"kind,omitempty"` // "" or "managed" treated as managed
	// GOST proxy auth (auto-generated on deploy)
	ProxyUser     string `json:"proxy_user,omitempty"`
	ProxyPassword string `json:"-"` // encrypted at rest
	// Deployment state
	Status     Status     `json:"status"`
	DeployedAt *time.Time `json:"deployed_at,omitempty"`
	CertExpiry *time.Time `json:"cert_expiry,omitempty"`
	Error      string     `json:"error,omitempty"`
	DeployLog  string     `json:"deploy_log,omitempty"` // last deployment log (shown on failure)
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// IsExternal reports whether the node is an externally-managed (non-ClashForge) node.
func (n *Node) IsExternal() bool {
	return n.Kind == NodeKindExternal
}

// NodeListItem is the safe-for-frontend summary (no secrets).
type NodeListItem struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Host       string     `json:"host"`
	Port       int        `json:"port"`
	Username   string     `json:"username"`
	Domain     string     `json:"domain"`
	Kind       NodeKind   `json:"kind,omitempty"`
	Status     Status     `json:"status"`
	DeployedAt *time.Time `json:"deployed_at,omitempty"`
	CertExpiry *time.Time `json:"cert_expiry,omitempty"`
	Error      string     `json:"error,omitempty"`
	DeployLog  string     `json:"deploy_log,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
