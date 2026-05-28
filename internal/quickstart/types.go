package quickstart

import "time"

// DeployType selects which path the QuickStart pipeline takes.
type DeployType string

const (
	DeployTypeCFWorkers DeployType = "cf_workers" // Cloudflare Workers, no VPS needed
	DeployTypeVPS       DeployType = "vps"        // VPS + Cloudflare DNS + gost
)

// Phase labels each stage of a deployment for the SSE event stream.
type Phase string

const (
	// Shared
	PhaseCFValidate Phase = "cf_validate"
	PhaseImport     Phase = "import"
	PhaseConfigure  Phase = "configure"
	PhaseVerify     Phase = "verify"
	// CF Workers path
	PhaseWorkerDeploy Phase = "worker_deploy"
	PhasePublish      Phase = "publish"
	// VPS path
	PhaseExistingCheck Phase = "existing_check"
	PhaseSSHTest       Phase = "ssh_test"
	PhaseEnvDetect     Phase = "env_detect"
	PhaseProvision     Phase = "provision"
	PhaseCertDNS       Phase = "cert_dns"
)

// EventStatus mirrors the UI color scheme.
type EventStatus string

const (
	StatusRunning EventStatus = "running"
	StatusOK      EventStatus = "ok"
	StatusError   EventStatus = "error"
	StatusInfo    EventStatus = "info"
	StatusWarning EventStatus = "warning"
)

// Event is a single SSE payload emitted by a pipeline stage.
type Event struct {
	Phase   Phase       `json:"phase"`
	Step    string      `json:"step"`
	Status  EventStatus `json:"status"`
	Message string      `json:"message"`
	Detail  string      `json:"detail,omitempty"`
}

// DeployRequest is the unified POST /quickstart/deploy body.
type DeployRequest struct {
	DeployType  DeployType `json:"deploy_type"`
	NodeName    string     `json:"node_name"`              // display name saved in subscription
	NodePrefix  string     `json:"node_prefix"`            // subdomain prefix, e.g. "node1"
	VPSNodeID   string     `json:"vps_node_id,omitempty"`  // optional existing managed node id
	ForceImport bool       `json:"force_import,omitempty"` // skip gost install, go straight to subscription generation
	// EnsureProxyAuth only reconfigures gost auth (username/password) and restarts service,
	// then continues to subscription import. It avoids full reinstall/cert issuance.
	EnsureProxyAuth bool            `json:"ensure_proxy_auth,omitempty"`
	Cloudflare      CFCredentials   `json:"cloudflare"`
	VPS             *VPSCredentials `json:"vps,omitempty"`
	// Runtime-only: set by the pipeline after gost is deployed (full path) or
	// pre-filled by the handler from the node store (force-import path).
	// Never deserialised from the HTTP request body.
	ProxyUser     string `json:"-"`
	ProxyPassword string `json:"-"`
}

// CFCredentials carries Cloudflare API credentials.
type CFCredentials struct {
	Token     string `json:"token"`
	AccountID string `json:"account_id"`
	ZoneID    string `json:"zone_id,omitempty"`
	ZoneName  string `json:"zone_name,omitempty"`
}

// VPSCredentials carries SSH connection parameters.
type VPSCredentials struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`      // default 22
	User     string `json:"user"`      // default "root"
	AuthType string `json:"auth_type"` // "password" | "key"
	Password string `json:"password,omitempty"`
	PrivKey  string `json:"priv_key,omitempty"` // PEM content
}

// CheckNodeRequest is the POST /quickstart/check-node body.
type CheckNodeRequest struct {
	Domain string `json:"domain"`
	VPSIP  string `json:"vps_ip"`
}

// ValidateCFRequest is the POST /quickstart/validate-cf body.
type ValidateCFRequest struct {
	Token     string `json:"token"`
	AccountID string `json:"account_id"`
}

// ValidateCFResult is returned by /validate-cf.
type ValidateCFResult struct {
	Valid bool     `json:"valid"`
	Error string   `json:"error,omitempty"`
	Zones []CFZone `json:"zones,omitempty"`
}

// CFZone is a single entry in the zones list.
type CFZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ValidateVPSRequest is the POST /quickstart/validate-vps body.
type ValidateVPSRequest struct {
	NodeID   string `json:"node_id,omitempty"` // optional existing managed node id
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	AuthType string `json:"auth_type"`
	Password string `json:"password,omitempty"`
	PrivKey  string `json:"priv_key,omitempty"`
}

// ValidateVPSResult is returned by /validate-vps.
type ValidateVPSResult struct {
	Valid     bool   `json:"valid"`
	Error     string `json:"error,omitempty"`
	OS        string `json:"os,omitempty"`
	OSVersion string `json:"os_version,omitempty"`
	Arch      string `json:"arch,omitempty"`
	NodeID    string `json:"node_id,omitempty"` // set when credentials were saved to the node store
}

// DeployState is persisted to disk so deploy history survives restarts.
type DeployState struct {
	ID         string     `json:"id"`
	DeployType DeployType `json:"deploy_type"`
	Status     string     `json:"status"` // "running" | "done" | "failed"
	NodeID     string     `json:"node_id,omitempty"`
	SubID      string     `json:"sub_id,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	// Sanitised summary (no secrets)
	NodeName string `json:"node_name"`
	NodeHost string `json:"node_host,omitempty"` // VPS IP or Worker hostname
}

// VerifyResult summarises post-deploy connectivity checks.
type VerifyResult struct {
	OutboundIP string `json:"outbound_ip"`
	Google     bool   `json:"google"`
	YouTube    bool   `json:"youtube"`
	Baidu      bool   `json:"baidu"`
	DNSClean   bool   `json:"dns_clean"`
}

// EnvInfo is the detected VPS environment.
type EnvInfo struct {
	OS         string // "ubuntu" | "debian" | "centos" | "alpine" | "unknown"
	OSVersion  string // "24.04"
	Arch       string // "amd64" | "arm64"
	Firewall   string // "ufw" | "firewalld" | "iptables" | "none"
	Port443In  bool   // true if 443 is already in use
	HasSystemd bool
}
