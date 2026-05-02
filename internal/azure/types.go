package azure

// Location represents an Azure region.
type Location struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

// ResourceGroup is an Azure resource group.
type ResourceGroup struct {
	Name     string `json:"name"`
	Location string `json:"location"`
}

// VMSize describes an available VM SKU.
type VMSize struct {
	Name                 string `json:"name"`
	NumberOfCores        int    `json:"numberOfCores"`
	MemoryInMB           int    `json:"memoryInMB"`
	OSDiskSizeInMB       int    `json:"osDiskSizeInMB"`
	ResourceDiskSizeInMB int    `json:"resourceDiskSizeInMB"`
}

// ProvisionRequest captures user intent for VM creation.
type ProvisionRequest struct {
	// Azure credentials
	Token          string
	SubscriptionID string

	// Location
	Location string // e.g. "eastasia"

	// Resource group (created if not existing)
	ResourceGroup string

	// VM settings
	VMName        string
	VMSize        string // e.g. "Standard_B1s"
	AdminUsername string
	SSHPublicKey  string // authorized_keys entry

	// Name prefix used for derived resource names
	// (vnet, subnet, public-ip, nsg, nic)
	Prefix string
}

// ProvisionResult holds the output after a successful provision.
type ProvisionResult struct {
	PublicIP string
	VMID     string
}

// ProgressEvent is emitted during provisioning.
type ProgressEvent struct {
	Step    string // step identifier
	Status  string // "running" | "ok" | "error"
	Message string
	Detail  string
}

// locationList is the ARM response for listing locations.
type locationList struct {
	Value []struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"value"`
}

// rgList is the ARM response for listing resource groups.
type rgList struct {
	Value []struct {
		Name     string `json:"name"`
		Location string `json:"location"`
	} `json:"value"`
}

// vmSizeList is the ARM response for listing VM sizes.
type vmSizeList struct {
	Value []VMSize `json:"value"`
}
