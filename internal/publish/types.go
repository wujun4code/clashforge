package publish

import "time"

type TemplatePreset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PublishPreviewRequest struct {
	NodeIDs         []string `json:"node_ids"`
	TemplateMode    string   `json:"template_mode"` // builtin | runtime | custom
	TemplateID      string   `json:"template_id,omitempty"`
	TemplateContent string   `json:"template_content,omitempty"`
}

type MergeNode struct {
	ID            string
	Name          string
	// SSH/GOST HTTP-proxy fields
	Host          string
	Domain        string
	ProxyUser     string
	ProxyPassword string
	// Worker/VLESS-WS fields (NodeType == "worker")
	NodeType       string // "ssh" | "worker"
	WorkerUUID     string
	WorkerHostname string
}

type WorkerConfig struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	WorkerName    string    `json:"worker_name"`
	WorkerURL     string    `json:"worker_url"`
	WorkerDevURL  string    `json:"worker_dev_url"`
	Hostname      string    `json:"hostname"`
	AccountID     string    `json:"account_id"`
	NamespaceID   string    `json:"namespace_id"`
	ZoneID        string    `json:"zone_id"`
	TokenEnc      string    `json:"token_enc,omitempty"`
	InitializedAt time.Time `json:"initialized_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type WorkerConfigView struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	WorkerName    string    `json:"worker_name"`
	WorkerURL     string    `json:"worker_url"`
	WorkerDevURL  string    `json:"worker_dev_url"`
	Hostname      string    `json:"hostname"`
	AccountID     string    `json:"account_id"`
	NamespaceID   string    `json:"namespace_id"`
	ZoneID        string    `json:"zone_id"`
	HasToken      bool      `json:"has_token"`
	InitializedAt time.Time `json:"initialized_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type WorkerConfigInput struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name"`
	WorkerName    string `json:"worker_name"`
	WorkerURL     string `json:"worker_url"`
	WorkerDevURL  string `json:"worker_dev_url"`
	Hostname      string `json:"hostname"`
	AccountID     string `json:"account_id"`
	NamespaceID   string `json:"namespace_id"`
	ZoneID        string `json:"zone_id"`
	Token         string `json:"token,omitempty"`
	InitializedAt string `json:"initialized_at,omitempty"`
}

type PublishRecord struct {
	ID             string    `json:"id"`
	WorkerConfigID string    `json:"worker_config_id"`
	WorkerName     string    `json:"worker_name"`
	Hostname       string    `json:"hostname"`
	BaseName       string    `json:"base_name"`
	Version        int       `json:"version"`
	FileName       string    `json:"file_name"`
	AccessURL      string    `json:"access_url"`
	PublishedAt    time.Time `json:"published_at"`
}

type PublishRecordInput struct {
	WorkerConfigID string `json:"worker_config_id"`
	WorkerName     string `json:"worker_name"`
	Hostname       string `json:"hostname"`
	BaseName       string `json:"base_name"`
	Version        int    `json:"version"`
	FileName       string `json:"file_name"`
	AccessURL      string `json:"access_url"`
}

type PermissionCheck struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type VerifyTest struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type WorkerVerifyResult struct {
	OK       bool         `json:"ok"`
	Tests    []VerifyTest `json:"tests"`
	UsedURL  string       `json:"used_url,omitempty"`
	HelloURL string       `json:"hello_url,omitempty"`
	Note     string       `json:"note,omitempty"`
}

type WorkerPermissionCheckRequest struct {
	Token     string `json:"token"`
	AccountID string `json:"account_id"`
	ZoneID    string `json:"zone_id,omitempty"`
}

type WorkerCreateNamespaceRequest struct {
	Token      string `json:"token"`
	AccountID  string `json:"account_id"`
	WorkerName string `json:"worker_name"`
}

type WorkerNamespaceResult struct {
	NamespaceID string `json:"namespace_id"`
	Reused      bool   `json:"reused"`
	Title       string `json:"title"`
}

type WorkerDeployScriptRequest struct {
	Token       string `json:"token"`
	AccountID   string `json:"account_id"`
	WorkerName  string `json:"worker_name"`
	NamespaceID string `json:"namespace_id"`
	AccessToken string `json:"access_token"`
}

type WorkerDeployResult struct {
	WorkerDevURL     string `json:"worker_dev_url"`
	WorkersSubdomain string `json:"workers_subdomain,omitempty"`
}

type WorkerBindDomainRequest struct {
	Token      string `json:"token"`
	AccountID  string `json:"account_id"`
	ZoneID     string `json:"zone_id"`
	WorkerName string `json:"worker_name"`
	Hostname   string `json:"hostname"`
}

type WorkerBindResult struct {
	Hostname  string `json:"hostname"`
	WorkerURL string `json:"worker_url"`
}

type WorkerVerifySaveRequest struct {
	Name         string `json:"name"`
	WorkerName   string `json:"worker_name"`
	WorkerURL    string `json:"worker_url"`
	WorkerDevURL string `json:"worker_dev_url"`
	Hostname     string `json:"hostname"`
	AccountID    string `json:"account_id"`
	NamespaceID  string `json:"namespace_id"`
	ZoneID       string `json:"zone_id"`
	AccessToken  string `json:"access_token"`
}

type PublishUploadRequest struct {
	WorkerConfigID  string   `json:"worker_config_id"`
	BaseName        string   `json:"base_name"`
	Content         string   `json:"content,omitempty"`
	NodeIDs         []string `json:"node_ids,omitempty"`
	TemplateMode    string   `json:"template_mode,omitempty"`
	TemplateID      string   `json:"template_id,omitempty"`
	TemplateContent string   `json:"template_content,omitempty"`
}
