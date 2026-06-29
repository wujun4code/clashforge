#Requires -RunAsAdministrator
<#
.SYNOPSIS
    One-click deploy ClashForge OpenWrt virtual router on Hyper-V.

.DESCRIPTION
    Downloads the latest ClashForge Hyper-V VHDX from GitHub Releases,
    creates a Generation-1 VM with two NICs (WAN + LAN), configures an
    internal switch so the Windows host can reach the router at
    192.168.100.1, then starts the VM.

    Network layout after setup:
        eth0 (Default Switch) → WAN, gets internet via Hyper-V NAT
        eth1 (ClashForge-LAN) → LAN 192.168.77.1
        Windows host          → LAN, DHCP lease from OpenWrt dnsmasq

    ClashForge Web UI : http://192.168.77.1:7777
    HTTP proxy        : 192.168.77.1:17890
    SOCKS5 proxy      : 192.168.77.1:17891
    Mixed proxy       : 192.168.77.1:17893

    To use as transparent gateway (no proxy settings needed):
        Disable your physical NIC / WiFi → only ClashForge-LAN remains →
        default gateway becomes 192.168.77.1 → enable TUN mode in ClashForge UI.

.PARAMETER VMName
    Name for the Hyper-V VM. Default: ClashForge-Router

.PARAMETER VHDXDir
    Directory where the VHDX will be stored. Default: $env:USERPROFILE\VMs\ClashForge

.PARAMETER VHDXPath
    Full path to an existing VHDX file. When provided, skips the download step.

.PARAMETER LanSwitchName
    Name of the internal Hyper-V switch for the LAN. Default: ClashForge-LAN

.PARAMETER LanIP
    LAN IP of the OpenWrt VM (used for health-check and summary). Default: 192.168.77.1

.PARAMETER MemoryMB
    VM RAM in MB. Default: 512

.PARAMETER CPUCount
    VM vCPU count. Default: 1

.PARAMETER GitHubRepo
    GitHub repository slug. Default: wujun4code/clashforge

.PARAMETER VHDXUrl
    Direct download URL for the VHDX file. Use this when GitHub Releases is
    inaccessible (e.g. behind a restrictive firewall). Supports any HTTP(S)
    URL: Azure Blob Storage, Cloudflare R2, a CDN, etc.
    When provided, -GitHubRepo and -GithubMirror are ignored.

    Example: https://mycdn.example.com/releases/clashforge-openwrt-24.10.2-hyperv.vhdx

.PARAMETER GithubMirror
    Prefix applied to GitHub release download URLs to route through a mirror
    proxy. Useful when api.github.com is reachable but the actual file download
    (objects.githubusercontent.com / github.com releases) is blocked.

    Example: https://ghproxy.net/
    Result : https://ghproxy.net/https://github.com/.../clashforge-...vhdx

.PARAMETER SubscriptionURL
    Proxy subscription URL. When provided, the script automatically adds the
    subscription and enables TUN (transparent proxy) mode after the VM boots —
    no manual Web UI steps needed. If omitted, the VM starts but proxy is not
    configured; open http://192.168.77.1:7777 manually to set up.

.PARAMETER ApiSecret
    ClashForge API secret (matches [security].api_secret in config.toml).
    Leave empty (default) when api_secret is not set.

.PARAMETER SetSystemProxy
    When specified, automatically sets Windows system HTTP/HTTPS proxy to
    192.168.77.1:17890 after the VM starts.

.EXAMPLE
    # Download latest image, boot VM, auto-configure subscription and TUN mode
    .\Start-ClashForgeHyperV.ps1 -SubscriptionURL 'https://your-sub-url/...'

.EXAMPLE
    # Use a GitHub mirror when direct GitHub access is blocked
    .\Start-ClashForgeHyperV.ps1 -GithubMirror 'https://ghproxy.net/' -SubscriptionURL 'https://your-sub-url/...'

.EXAMPLE
    # Download from a CDN mirror (Azure Blob, R2, etc.) — no GitHub needed
    .\Start-ClashForgeHyperV.ps1 -VHDXUrl 'https://mycdn.example.com/releases/clashforge-openwrt-24.10.2-hyperv.vhdx' -SubscriptionURL 'https://your-sub-url/...'

.EXAMPLE
    # Use a VHDX you already downloaded
    .\Start-ClashForgeHyperV.ps1 -VHDXPath C:\VMs\clashforge-openwrt-24.10.2-hyperv.vhdx -SubscriptionURL 'https://your-sub-url/...'

.EXAMPLE
    # Boot only — configure subscription manually in Web UI
    .\Start-ClashForgeHyperV.ps1
#>

param(
    [string]$VMName          = 'ClashForge-Router',
    [string]$VHDXDir         = (Join-Path $env:USERPROFILE 'VMs\ClashForge'),
    [string]$VHDXPath        = '',
    [string]$LanSwitchName   = 'ClashForge-LAN',
    [string]$LanIP           = '192.168.77.1',
    [int]   $MemoryMB        = 512,
    [int]   $CPUCount        = 1,
    [string]$GitHubRepo      = 'wujun4code/clashforge',
    [string]$VHDXUrl         = '',
    [string]$GithubMirror    = '',
    [string]$SubscriptionURL = '',
    [string]$ApiSecret       = '',
    [switch]$SetSystemProxy
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ── helpers ──────────────────────────────────────────────────────────────────
function Write-Step([string]$msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }
function Write-OK  ([string]$msg) { Write-Host "  ✓ $msg"  -ForegroundColor Green }
function Write-Warn([string]$msg) { Write-Host "  ⚠ $msg"  -ForegroundColor Yellow }
function Write-Fail([string]$msg) { Write-Host "  ✗ $msg"  -ForegroundColor Red }

function Invoke-CF {
    param(
        [string]$Method,
        [string]$Path,
        [object]$Body = $null,
        [switch]$SSE
    )
    $uri     = "http://${LanIP}:7777${Path}"
    $headers = @{ 'Content-Type' = 'application/json' }
    if ($ApiSecret -ne '') { $headers['Authorization'] = "Bearer $ApiSecret" }

    if ($SSE) {
        # Read SSE stream line-by-line until "type":"done"
        $client  = [System.Net.Http.HttpClient]::new()
        $client.Timeout = [System.TimeSpan]::FromSeconds(120)
        foreach ($k in $headers.Keys) { $client.DefaultRequestHeaders.Add($k, $headers[$k]) }
        $json    = if ($Body) { [System.Net.Http.StringContent]::new(
                       ($Body | ConvertTo-Json -Depth 10 -Compress),
                       [System.Text.Encoding]::UTF8, 'application/json') } else { $null }
        $req     = [System.Net.Http.HttpRequestMessage]::new(
                       [System.Net.Http.HttpMethod]::new($Method), $uri)
        if ($json) { $req.Content = $json }
        $resp   = $client.SendAsync($req, [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead).Result
        $stream = $resp.Content.ReadAsStreamAsync().Result
        $reader = [System.IO.StreamReader]::new($stream)
        $result = $null
        while (-not $reader.EndOfStream) {
            $line = $reader.ReadLine()
            if ($line -match '^data:\s*(\{.+\})$') {
                $evt = $Matches[1] | ConvertFrom-Json
                if ($evt.type -eq 'step') {
                    $icon = if ($evt.status -eq 'ok') { '  ✓' } elseif ($evt.status -eq 'error') { '  ✗' } else { '  ·' }
                    Write-Host "$icon $($evt.step): $($evt.message)"
                }
                if ($evt.type -eq 'done') { $result = $evt; break }
            }
        }
        $client.Dispose()
        return $result
    } else {
        $bodyJson = if ($Body) { $Body | ConvertTo-Json -Depth 10 -Compress } else { $null }
        return Invoke-RestMethod -Uri $uri -Method $Method -Headers $headers `
               -Body $bodyJson -UseBasicParsing -ErrorAction Stop
    }
}

# ── 0. Pre-flight checks ──────────────────────────────────────────────────────
Write-Step "Pre-flight checks"

$hvFeature = Get-WindowsOptionalFeature -Online -FeatureName 'Microsoft-Hyper-V-All' -ErrorAction SilentlyContinue
if ($hvFeature.State -ne 'Enabled') {
    Write-Fail "Hyper-V is not enabled on this machine."
    Write-Host @"

  To enable Hyper-V, run this in an elevated PowerShell and reboot:
    Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V-All

  Or via Settings → Apps → Optional features → More Windows features → Hyper-V
"@
    exit 1
}
Write-OK "Hyper-V is enabled"

if (Get-VM -Name $VMName -ErrorAction SilentlyContinue) {
    Write-Warn "VM '$VMName' already exists."
    $choice = Read-Host "  Delete and recreate? [y/N]"
    if ($choice -match '^[Yy]') {
        $existing = Get-VM -Name $VMName
        if ($existing.State -ne 'Off') { Stop-VM -Name $VMName -Force }
        Remove-VM -Name $VMName -Force
        Write-OK "Removed existing VM"
    } else {
        Write-Host "Aborted." -ForegroundColor Yellow
        exit 0
    }
}

# ── 1. Download VHDX if needed ────────────────────────────────────────────────
function Invoke-DownloadFile([string]$Url, [string]$Dest) {
    Write-Host "  Downloading from: $Url"
    Write-Host "  Destination     : $Dest"
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing
    $sw.Stop()
    Write-OK "Downloaded in $($sw.Elapsed.TotalSeconds.ToString('F1'))s  ($('{0:N1}' -f ((Get-Item $Dest).Length / 1MB)) MB)"
}

if ($VHDXPath -ne '' -and (Test-Path $VHDXPath)) {
    Write-Step "Using provided VHDX: $VHDXPath"
    Write-OK "$('{0:N0}' -f ((Get-Item $VHDXPath).Length / 1MB)) MB"

} elseif ($VHDXUrl -ne '') {
    # Direct CDN / Azure Blob / any accessible URL — skip GitHub entirely
    Write-Step "Downloading VHDX from custom URL"
    New-Item -ItemType Directory -Path $VHDXDir -Force | Out-Null
    $vhdxName = [System.IO.Path]::GetFileName(([uri]$VHDXUrl).LocalPath)
    if (-not $vhdxName -or $vhdxName -notlike '*.vhdx') { $vhdxName = 'clashforge-hyperv.vhdx' }
    $VHDXPath = Join-Path $VHDXDir $vhdxName
    if (Test-Path $VHDXPath) {
        Write-Warn "File already exists at $VHDXPath, skipping download."
    } else {
        Invoke-DownloadFile -Url $VHDXUrl -Dest $VHDXPath
    }

} else {
    Write-Step "Downloading latest ClashForge Hyper-V image from GitHub"

    # Normalize mirror prefix (e.g. 'https://ghproxy.net/')
    $mirrorPrefix = if ($GithubMirror -ne '') { $GithubMirror.TrimEnd('/') + '/' } else { '' }
    if ($mirrorPrefix -ne '') { Write-Warn "GitHub download mirror: $mirrorPrefix" }

    $releaseApi = "https://api.github.com/repos/$GitHubRepo/releases/latest"
    Write-Host "  Querying $releaseApi ..."

    try {
        $release = Invoke-RestMethod -Uri $releaseApi -UseBasicParsing `
            -Headers @{ 'Accept' = 'application/vnd.github+json' }
    } catch {
        Write-Fail "Failed to query GitHub API: $_"
        Write-Host ""
        Write-Host "  If GitHub is unreachable, try one of these alternatives:" -ForegroundColor Yellow
        Write-Host "    1. Mirror  : .\Start-ClashForgeHyperV.ps1 -GithubMirror 'https://ghproxy.net/'" -ForegroundColor Yellow
        Write-Host "    2. CDN URL : .\Start-ClashForgeHyperV.ps1 -VHDXUrl 'https://your-cdn.example.com/clashforge.vhdx'" -ForegroundColor Yellow
        Write-Host "    3. Local   : .\Start-ClashForgeHyperV.ps1 -VHDXPath C:\path\to\clashforge.vhdx" -ForegroundColor Yellow
        exit 1
    }

    $vhdxAsset = $release.assets | Where-Object { $_.name -like '*.vhdx' } | Select-Object -First 1
    if (-not $vhdxAsset) {
        Write-Fail "No .vhdx asset found in release '$($release.tag_name)'."
        Write-Host "  Has the 'Build OpenWrt + ClashForge Hyper-V Image' workflow run for this release?"
        exit 1
    }

    New-Item -ItemType Directory -Path $VHDXDir -Force | Out-Null
    $VHDXPath = Join-Path $VHDXDir $vhdxAsset.name

    if (Test-Path $VHDXPath) {
        Write-Warn "File already exists at $VHDXPath, skipping download."
    } else {
        $dlUrl = if ($mirrorPrefix) { "${mirrorPrefix}$($vhdxAsset.browser_download_url)" } `
                 else               { $vhdxAsset.browser_download_url }
        Invoke-DownloadFile -Url $dlUrl -Dest $VHDXPath
    }
}

# ── 2. Create internal LAN switch ─────────────────────────────────────────────
Write-Step "Configuring Hyper-V internal switch: $LanSwitchName"

$lanSwitch = Get-VMSwitch -Name $LanSwitchName -ErrorAction SilentlyContinue
if (-not $lanSwitch) {
    New-VMSwitch -Name $LanSwitchName -SwitchType Internal | Out-Null
    Write-OK "Created internal switch '$LanSwitchName'"
} else {
    Write-OK "Switch '$LanSwitchName' already exists"
}

# Ensure the host-side vEthernet adapter is set to DHCP so OpenWrt dnsmasq assigns it an IP
$vEth = Get-NetAdapter | Where-Object { $_.Name -like "*$LanSwitchName*" } | Select-Object -First 1
if ($vEth) {
    # Remove any leftover static IP on this adapter so DHCP can take over
    Get-NetIPAddress -InterfaceIndex $vEth.InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.PrefixOrigin -ne 'WellKnown' } |
        Remove-NetIPAddress -Confirm:$false -ErrorAction SilentlyContinue
    Set-NetIPInterface -InterfaceIndex $vEth.InterfaceIndex -Dhcp Enabled -ErrorAction SilentlyContinue
    Write-OK "vEthernet ($LanSwitchName) set to DHCP — OpenWrt will assign an IP after VM boots"
} else {
    Write-Warn "Could not find vEthernet adapter for '$LanSwitchName'"
}

# ── 3. Check Default Switch exists (for WAN) ──────────────────────────────────
Write-Step "Verifying WAN switch"
$defaultSwitch = Get-VMSwitch | Where-Object { $_.Name -eq 'Default Switch' } | Select-Object -First 1
if (-not $defaultSwitch) {
    Write-Warn "'Default Switch' not found. Using first external/NAT switch as WAN."
    $defaultSwitch = Get-VMSwitch | Where-Object { $_.SwitchType -in 'External','Internal' } | Select-Object -First 1
    if (-not $defaultSwitch) {
        Write-Fail "No suitable WAN switch found. Connect the VM to a switch manually after creation."
    }
}
$wanSwitchName = if ($defaultSwitch) { $defaultSwitch.Name } else { '' }
Write-OK "WAN switch: $wanSwitchName"

# ── 4. Create VM ──────────────────────────────────────────────────────────────
Write-Step "Creating VM: $VMName"

$vm = New-VM `
    -Name $VMName `
    -Generation 1 `
    -MemoryStartupBytes ($MemoryMB * 1MB) `
    -NoVHD

Set-VMProcessor       -VMName $VMName -Count $CPUCount
Set-VMMemory          -VMName $VMName -DynamicMemoryEnabled $false -StartupBytes ($MemoryMB * 1MB)
Set-VMDvdDrive        -VMName $VMName -Path $null -ErrorAction SilentlyContinue

# Attach VHDX as primary disk
Add-VMHardDiskDrive   -VMName $VMName -Path $VHDXPath -ControllerType IDE -ControllerNumber 0 -ControllerLocation 0
Set-VMBios            -VMName $VMName -StartupOrder @('IDE', 'CD', 'LegacyNetworkAdapter', 'Floppy')

Write-OK "VM created (Gen1, ${MemoryMB}MB RAM, ${CPUCount} vCPU)"

# ── 5. Configure NICs ─────────────────────────────────────────────────────────
Write-Step "Configuring network adapters"

# Remove the default adapter created by New-VM
Get-VMNetworkAdapter -VMName $VMName | Remove-VMNetworkAdapter

# NIC 1: WAN — connected to Default Switch (internet via NAT)
if ($wanSwitchName) {
    Add-VMNetworkAdapter -VMName $VMName -SwitchName $wanSwitchName -Name 'WAN'
    Write-OK "WAN NIC → $wanSwitchName"
} else {
    Add-VMNetworkAdapter -VMName $VMName -Name 'WAN'
    Write-Warn "WAN NIC created but not connected (no switch found)"
}

# NIC 2: LAN — connected to internal ClashForge-LAN switch
Add-VMNetworkAdapter -VMName $VMName -SwitchName $LanSwitchName -Name 'LAN'
Write-OK "LAN NIC → $LanSwitchName (OpenWrt LAN: 192.168.77.1)"

# ── 6. Start VM ───────────────────────────────────────────────────────────────
Write-Step "Starting VM"
Start-VM -Name $VMName
Write-OK "VM started"

# ── 7. Wait for web UI ───────────────────────────────────────────────────────
Write-Step "Waiting for ClashForge to come online (up to 90s)..."
$deadline = (Get-Date).AddSeconds(90)
$online   = $false
while ((Get-Date) -lt $deadline) {
    try {
        $r = Invoke-WebRequest -Uri "http://${LanIP}:7777" -UseBasicParsing `
             -TimeoutSec 2 -ErrorAction Stop
        if ($r.StatusCode -lt 500) { $online = $true; break }
    } catch { }
    Write-Host '.' -NoNewline
    Start-Sleep 3
}
Write-Host ''

if ($online) {
    Write-OK "ClashForge is online!"
} else {
    Write-Warn "Could not reach http://${LanIP}:7777 within 90s."
    Write-Host "  The VM may still be booting. Try again in a moment."
    Write-Host "  You can open Hyper-V Manager to watch the console."
}

# ── 8. Configure subscription + apply TUN mode (if URL provided) ─────────────
if ($SubscriptionURL -ne '' -and $online) {
    Write-Step "Configuring subscription"

    # 8a. Add subscription
    $sub = Invoke-CF -Method POST -Path '/api/v1/subscriptions' -Body @{
        name    = 'auto'
        type    = 'url'
        url     = $SubscriptionURL
        enabled = $true
    }
    $subId = $sub.id
    Write-OK "Subscription added (id=$subId)"

    # 8b. Fetch nodes synchronously
    Write-Host "  Fetching nodes from subscription URL..."
    try {
        Invoke-CF -Method POST -Path "/api/v1/subscriptions/$subId/sync-update" | Out-Null
        Write-OK "Nodes fetched"
    } catch {
        Write-Warn "sync-update failed ($_) — will proceed anyway"
    }

    # 8c. Apply: TUN mode + sub as source, stream SSE progress
    Write-Step "Applying TUN mode (transparent proxy)"
    $result = Invoke-CF -Method POST -Path '/api/v1/core/apply' -SSE -Body @{
        source  = @{ type = 'sub_id'; sub_id = $subId; sync = $false }
        network = @{
            mode             = 'tun'
            firewall_backend = 'auto'
            bypass_lan       = $true
            bypass_china     = $false
            apply_on_start   = $true
            ipv6             = $false
        }
        dns     = @{
            enable         = $true
            mode           = 'fake-ip'
            dnsmasq_mode   = 'upstream'
            apply_on_start = $true
            nameservers    = @('119.29.29.29', '223.5.5.5')
            fallback       = @('8.8.8.8', '1.1.1.1')
        }
    }

    if ($result -and $result.success) {
        Write-OK "TUN mode active — proxy is running"
    } else {
        $errMsg = if ($result) { $result.error } else { 'no response' }
        Write-Warn "Apply finished with issues: $errMsg"
        Write-Host "  Open http://${LanIP}:7777 to check status and configure manually."
    }
} elseif ($SubscriptionURL -eq '') {
    Write-Warn "No -SubscriptionURL provided — proxy not started."
    Write-Host "  Open http://${LanIP}:7777 and configure a subscription manually."
}

# ── 9. Optionally set system proxy ───────────────────────────────────────────
if ($SetSystemProxy) {
    Write-Step "Configuring Windows system proxy"
    $proxyServer = "${LanIP}:17890"
    Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' `
        -Name 'ProxyServer'  -Value $proxyServer
    Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' `
        -Name 'ProxyEnable'  -Value 1
    Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' `
        -Name 'ProxyOverride' -Value 'localhost;127.*;192.168.77.*;10.*;172.16.*;<local>'
    Write-OK "System HTTP proxy set to $proxyServer"
    Write-Warn "Restart your browser for proxy changes to take effect."
}

# ── 9. Summary ────────────────────────────────────────────────────────────────
Write-Host ''
Write-Host '────────────────────────────────────────────────' -ForegroundColor DarkGray
Write-Host " ClashForge VM is running!"                       -ForegroundColor Green
Write-Host '────────────────────────────────────────────────' -ForegroundColor DarkGray
Write-Host "  VM Name   : $VMName"
Write-Host "  Web UI    : http://${LanIP}:7777"
Write-Host "  HTTP Proxy: ${LanIP}:17890"
Write-Host "  SOCKS5    : ${LanIP}:17891"
Write-Host "  Mixed     : ${LanIP}:17893"
Write-Host ''
Write-Host "  To configure your browser proxy manually:" -ForegroundColor DarkGray
Write-Host "    HTTP Proxy: $LanIP   Port: 17890"        -ForegroundColor DarkGray
Write-Host ''
Write-Host "  To stop the VM:"                           -ForegroundColor DarkGray
Write-Host "    Stop-VM -Name '$VMName'"                 -ForegroundColor DarkGray
Write-Host "  To remove the proxy setting:"             -ForegroundColor DarkGray
Write-Host "    Set-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' ProxyEnable 0" -ForegroundColor DarkGray
Write-Host '────────────────────────────────────────────────' -ForegroundColor DarkGray

# Open Web UI in browser
try {
    if ($online) { Start-Process "http://${LanIP}:7777" }
} catch { }
