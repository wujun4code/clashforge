package azure

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ListLocations returns the Azure regions available for the subscription.
func ListLocations(token, subscriptionID string) ([]Location, error) {
	path := fmt.Sprintf("/subscriptions/%s/locations?api-version=2022-12-01", subscriptionID)
	var resp locationList
	if err := armGetJSON(path, token, &resp); err != nil {
		return nil, err
	}
	out := make([]Location, 0, len(resp.Value))
	for _, v := range resp.Value {
		out = append(out, Location{Name: v.Name, DisplayName: v.DisplayName})
	}
	return out, nil
}

// ListResourceGroups returns all resource groups in the subscription.
func ListResourceGroups(token, subscriptionID string) ([]ResourceGroup, error) {
	path := fmt.Sprintf("/subscriptions/%s/resourcegroups?api-version=%s", subscriptionID, resourceAPIVersion)
	var resp rgList
	if err := armGetJSON(path, token, &resp); err != nil {
		return nil, err
	}
	out := make([]ResourceGroup, 0, len(resp.Value))
	for _, v := range resp.Value {
		out = append(out, ResourceGroup{Name: v.Name, Location: v.Location})
	}
	return out, nil
}

// ListVMSizes returns available VM sizes in a region.
func ListVMSizes(token, subscriptionID, location string) ([]VMSize, error) {
	path := fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Compute/locations/%s/vmSizes?api-version=%s",
		subscriptionID, location, computeAPIVersion)
	var resp vmSizeList
	if err := armGetJSON(path, token, &resp); err != nil {
		return nil, err
	}
	return resp.Value, nil
}

// ValidateCredentials attempts to list resource groups as a quick credential check.
func ValidateCredentials(token, subscriptionID string) error {
	path := fmt.Sprintf("/subscriptions/%s?api-version=%s", subscriptionID, resourceAPIVersion)
	var result map[string]any
	return armGetJSON(path, token, &result)
}

// ProvisionVM creates all necessary Azure resources and a Linux VM.
// It streams progress events on ch and returns the result when done.
func ProvisionVM(ctx context.Context, req ProvisionRequest, ch chan<- ProgressEvent) (ProvisionResult, error) {
	emit := func(step, status, msg, detail string) {
		select {
		case ch <- ProgressEvent{Step: step, Status: status, Message: msg, Detail: detail}:
		default:
		}
	}

	prefix := req.Prefix
	if prefix == "" {
		prefix = req.VMName
	}
	sub := req.SubscriptionID
	rg := req.ResourceGroup
	loc := req.Location

	// ── Step 1: Ensure Resource Group ────────────────────────────────────────
	emit("rg", "running", "准备资源组", rg)
	rgPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s?api-version=%s", sub, rg, resourceAPIVersion)
	_, err := armPutJSON(rgPath, req.Token, map[string]any{
		"location": loc,
	}, nil)
	if err != nil {
		emit("rg", "error", "创建资源组失败", err.Error())
		return ProvisionResult{}, fmt.Errorf("创建资源组: %w", err)
	}
	emit("rg", "ok", "资源组就绪", rg)

	if ctx.Err() != nil {
		return ProvisionResult{}, ctx.Err()
	}

	// ── Step 2: Virtual Network + Subnet ─────────────────────────────────────
	vnetName := prefix + "-vnet"
	subnetName := prefix + "-subnet"
	emit("vnet", "running", "创建虚拟网络", vnetName)
	vnetPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s?api-version=%s",
		sub, rg, vnetName, networkAPIVersion)
	asyncURL, err := armPutJSONAsync(vnetPath, req.Token, map[string]any{
		"location": loc,
		"properties": map[string]any{
			"addressSpace": map[string]any{
				"addressPrefixes": []string{"10.0.0.0/16"},
			},
			"subnets": []map[string]any{
				{
					"name": subnetName,
					"properties": map[string]any{
						"addressPrefix": "10.0.0.0/24",
					},
				},
			},
		},
	})
	if err != nil {
		emit("vnet", "error", "创建 VNet 失败", err.Error())
		return ProvisionResult{}, fmt.Errorf("创建 VNet: %w", err)
	}
	if asyncURL != "" {
		if err := armPollAsyncOp(asyncURL, req.Token, 120); err != nil {
			emit("vnet", "error", "等待 VNet 就绪超时", err.Error())
			return ProvisionResult{}, fmt.Errorf("等待 VNet: %w", err)
		}
	}
	emit("vnet", "ok", "虚拟网络已创建", vnetName)

	if ctx.Err() != nil {
		return ProvisionResult{}, ctx.Err()
	}

	// ── Step 3: Public IP ────────────────────────────────────────────────────
	pipName := prefix + "-pip"
	emit("pip", "running", "分配公网 IP", pipName)
	pipPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/publicIPAddresses/%s?api-version=%s",
		sub, rg, pipName, networkAPIVersion)
	asyncURL, err = armPutJSONAsync(pipPath, req.Token, map[string]any{
		"location": loc,
		"sku":      map[string]any{"name": "Standard"},
		"properties": map[string]any{
			"publicIPAllocationMethod": "Static",
		},
	})
	if err != nil {
		emit("pip", "error", "创建公网 IP 失败", err.Error())
		return ProvisionResult{}, fmt.Errorf("创建公网 IP: %w", err)
	}
	if asyncURL != "" {
		if err := armPollAsyncOp(asyncURL, req.Token, 60); err != nil {
			emit("pip", "error", "等待公网 IP 超时", err.Error())
			return ProvisionResult{}, fmt.Errorf("等待公网 IP: %w", err)
		}
	}
	emit("pip", "ok", "公网 IP 已分配", "")

	if ctx.Err() != nil {
		return ProvisionResult{}, ctx.Err()
	}

	// ── Step 4: Network Security Group (allow SSH 22) ─────────────────────────
	nsgName := prefix + "-nsg"
	emit("nsg", "running", "配置安全组", nsgName)
	nsgPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s?api-version=%s",
		sub, rg, nsgName, networkAPIVersion)
	asyncURL, err = armPutJSONAsync(nsgPath, req.Token, map[string]any{
		"location": loc,
		"properties": map[string]any{
			"securityRules": []map[string]any{
				{
					"name": "allow-ssh",
					"properties": map[string]any{
						"priority":                 100,
						"protocol":                 "Tcp",
						"access":                   "Allow",
						"direction":                "Inbound",
						"sourceAddressPrefix":      "*",
						"sourcePortRange":          "*",
						"destinationAddressPrefix": "*",
						"destinationPortRange":     "22",
					},
				},
			},
		},
	})
	if err != nil {
		emit("nsg", "error", "创建安全组失败", err.Error())
		return ProvisionResult{}, fmt.Errorf("创建 NSG: %w", err)
	}
	if asyncURL != "" {
		if err := armPollAsyncOp(asyncURL, req.Token, 60); err != nil {
			emit("nsg", "error", "等待安全组超时", err.Error())
			return ProvisionResult{}, fmt.Errorf("等待 NSG: %w", err)
		}
	}
	emit("nsg", "ok", "安全组已配置（SSH 22 开放）", "")

	if ctx.Err() != nil {
		return ProvisionResult{}, ctx.Err()
	}

	// Build ARM resource IDs
	pipID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/publicIPAddresses/%s", sub, rg, pipName)
	subnetID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s/subnets/%s", sub, rg, vnetName, subnetName)
	nsgID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s", sub, rg, nsgName)

	// ── Step 5: Network Interface ────────────────────────────────────────────
	nicName := prefix + "-nic"
	emit("nic", "running", "创建网络接口", nicName)
	nicPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkInterfaces/%s?api-version=%s",
		sub, rg, nicName, networkAPIVersion)
	asyncURL, err = armPutJSONAsync(nicPath, req.Token, map[string]any{
		"location": loc,
		"properties": map[string]any{
			"networkSecurityGroup": map[string]any{"id": nsgID},
			"ipConfigurations": []map[string]any{
				{
					"name": "ipconfig1",
					"properties": map[string]any{
						"privateIPAllocationMethod": "Dynamic",
						"publicIPAddress":           map[string]any{"id": pipID},
						"subnet":                    map[string]any{"id": subnetID},
					},
				},
			},
		},
	})
	if err != nil {
		emit("nic", "error", "创建网络接口失败", err.Error())
		return ProvisionResult{}, fmt.Errorf("创建 NIC: %w", err)
	}
	if asyncURL != "" {
		if err := armPollAsyncOp(asyncURL, req.Token, 60); err != nil {
			emit("nic", "error", "等待网络接口超时", err.Error())
			return ProvisionResult{}, fmt.Errorf("等待 NIC: %w", err)
		}
	}
	emit("nic", "ok", "网络接口已创建", nicName)

	if ctx.Err() != nil {
		return ProvisionResult{}, ctx.Err()
	}

	nicID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkInterfaces/%s", sub, rg, nicName)

	// ── Step 6: Create VM ────────────────────────────────────────────────────
	vmName := req.VMName
	emit("vm", "running", "创建虚拟机", vmName)

	sshKeyPath := fmt.Sprintf("/home/%s/.ssh/authorized_keys", req.AdminUsername)
	vmPath := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s?api-version=%s",
		sub, rg, vmName, computeAPIVersion)
	asyncURL, err = armPutJSONAsync(vmPath, req.Token, map[string]any{
		"location": loc,
		"properties": map[string]any{
			"hardwareProfile": map[string]any{
				"vmSize": req.VMSize,
			},
			"storageProfile": map[string]any{
				"imageReference": map[string]any{
					"publisher": "Canonical",
					"offer":     "ubuntu-24_04-lts",
					"sku":       "server",
					"version":   "latest",
				},
				"osDisk": map[string]any{
					"createOption": "FromImage",
					"managedDisk": map[string]any{
						"storageAccountType": "Premium_LRS",
					},
				},
			},
			"osProfile": map[string]any{
				"computerName":  vmName,
				"adminUsername": req.AdminUsername,
				"linuxConfiguration": map[string]any{
					"disablePasswordAuthentication": true,
					"ssh": map[string]any{
						"publicKeys": []map[string]any{
							{
								"path":    sshKeyPath,
								"keyData": req.SSHPublicKey,
							},
						},
					},
				},
			},
			"networkProfile": map[string]any{
				"networkInterfaces": []map[string]any{
					{"id": nicID},
				},
			},
		},
	})
	if err != nil {
		emit("vm", "error", "创建虚拟机失败", err.Error())
		return ProvisionResult{}, fmt.Errorf("创建 VM: %w", err)
	}
	if asyncURL != "" {
		if err := armPollAsyncOp(asyncURL, req.Token, 300); err != nil {
			emit("vm", "error", "等待虚拟机就绪超时", err.Error())
			return ProvisionResult{}, fmt.Errorf("等待 VM: %w", err)
		}
	}
	emit("vm", "ok", "虚拟机已创建", vmName)

	// ── Step 7: Retrieve Public IP ───────────────────────────────────────────
	emit("ip", "running", "获取公网 IP 地址", "")
	publicIP, err := waitForPublicIP(req.Token, sub, rg, pipName, 60)
	if err != nil {
		emit("ip", "error", "获取公网 IP 失败", err.Error())
		return ProvisionResult{}, fmt.Errorf("获取公网 IP: %w", err)
	}
	emit("ip", "ok", "公网 IP 已就绪", publicIP)

	vmID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s", sub, rg, vmName)
	return ProvisionResult{
		PublicIP: publicIP,
		VMID:     vmID,
	}, nil
}

// waitForPublicIP polls until the public IP is allocated.
func waitForPublicIP(token, sub, rg, pipName string, maxSec int) (string, error) {
	path := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/publicIPAddresses/%s?api-version=%s",
		sub, rg, pipName, networkAPIVersion)
	deadline := time.Now().Add(time.Duration(maxSec) * time.Second)
	for time.Now().Before(deadline) {
		var pip struct {
			Properties struct {
				IPAddress           string `json:"ipAddress"`
				ProvisioningState   string `json:"provisioningState"`
			} `json:"properties"`
		}
		if err := armGetJSON(path, token, &pip); err != nil {
			return "", err
		}
		if pip.Properties.IPAddress != "" {
			return pip.Properties.IPAddress, nil
		}
		time.Sleep(5 * time.Second)
	}
	return "", fmt.Errorf("等待公网 IP 分配超时（%d 秒）", maxSec)
}

// SuggestedVMSizes returns a curated list of cost-effective VM sizes for proxy use.
func SuggestedVMSizes() []struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	vCPUs       int
	MemGB       float64
} {
	return []struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		vCPUs       int
		MemGB       float64
	}{
		{Name: "Standard_B1s", DisplayName: "B1s · 1 vCPU · 1 GB RAM"},
		{Name: "Standard_B1ms", DisplayName: "B1ms · 1 vCPU · 2 GB RAM"},
		{Name: "Standard_B2s", DisplayName: "B2s · 2 vCPU · 4 GB RAM"},
		{Name: "Standard_B2ms", DisplayName: "B2ms · 2 vCPU · 8 GB RAM"},
		{Name: "Standard_B4ms", DisplayName: "B4ms · 4 vCPU · 16 GB RAM"},
		{Name: "Standard_D2s_v3", DisplayName: "D2s_v3 · 2 vCPU · 8 GB RAM"},
		{Name: "Standard_F1s", DisplayName: "F1s · 1 vCPU · 2 GB RAM (计算优化)"},
	}
}

// locationDisplayName maps common region codes to friendly names.
var locationDisplayName = map[string]string{
	"eastasia":           "东亚 (香港)",
	"southeastasia":      "东南亚 (新加坡)",
	"japaneast":          "日本东部 (东京)",
	"japanwest":          "日本西部 (大阪)",
	"koreacentral":       "韩国中部 (首尔)",
	"eastus":             "美国东部",
	"eastus2":            "美国东部 2",
	"westus":             "美国西部",
	"westus2":            "美国西部 2",
	"westus3":            "美国西部 3",
	"centralus":          "美国中部",
	"northcentralus":     "美国中北部",
	"southcentralus":     "美国中南部",
	"westcentralus":      "美国中西部",
	"canadacentral":      "加拿大中部",
	"canadaeast":         "加拿大东部",
	"brazilsouth":        "巴西南部 (圣保罗)",
	"northeurope":        "北欧 (爱尔兰)",
	"westeurope":         "西欧 (荷兰)",
	"uksouth":            "英国南部 (伦敦)",
	"ukwest":             "英国西部 (威尔士)",
	"francecentral":      "法国中部 (巴黎)",
	"germanywestcentral": "德国西中部 (法兰克福)",
	"switzerlandnorth":   "瑞士北部 (苏黎世)",
	"australiaeast":      "澳大利亚东部 (悉尼)",
	"australiasoutheast": "澳大利亚东南部 (墨尔本)",
	"centralindia":       "印度中部",
	"southindia":         "印度南部",
	"westindia":          "印度西部",
	"uaenorth":           "阿联酋北部 (迪拜)",
}

// FriendlyLocationName returns a Chinese display name for a region, falling back to raw name.
func FriendlyLocationName(name string) string {
	if d, ok := locationDisplayName[strings.ToLower(name)]; ok {
		return d
	}
	return name
}
