# push-install.ps1 — Copy a local IPK to an OpenWrt router and install it
#
# Usage:
#   .\push-install.ps1 -Ipk <path-to-ipk> -Router <router-ip>
#   .\push-install.ps1 -Ipk .\dist\clashforge_0.3.0_x86_64.ipk -Router 192.168.1.1
#   .\push-install.ps1 -Ipk .\dist\clashforge_0.3.0_x86_64.ipk -Router 192.168.1.1 -User root -Port 22
#   .\push-install.ps1 -Ipk .\dist\clashforge_0.3.0_x86_64.ipk -Router 192.168.1.1 -Purge
#
# Requirements:
#   - ssh and scp must be available (OpenSSH, bundled with Windows 10/11)
#   - Password-less SSH key auth is recommended; otherwise you'll be prompted twice

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$Ipk,           # Local path to the .ipk file

    [Parameter(Mandatory)]
    [string]$Router,        # Router IP or hostname

    [string]$User   = "root",
    [int]$Port      = 22,
    [switch]$Purge          # Run --purge before installing (wipe old config)
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ── helpers ───────────────────────────────────────────────────────────────────

function Log  { param($msg) Write-Host "[clashforge] $msg" -ForegroundColor Cyan }
function Ok   { param($msg) Write-Host "[clashforge] $msg" -ForegroundColor Green }
function Warn { param($msg) Write-Host "[clashforge] WARN: $msg" -ForegroundColor Yellow }
function Die  { param($msg) Write-Host "[clashforge] ERROR: $msg" -ForegroundColor Red; exit 1 }

# ── validate inputs ───────────────────────────────────────────────────────────

if (-not (Test-Path $Ipk)) {
    Die "IPK file not found: $Ipk"
}

$IpkItem = Get-Item $Ipk
if ($IpkItem.Extension -ne ".ipk") {
    Warn "File does not have .ipk extension: $($IpkItem.Name)"
}

# Check ssh / scp are available
foreach ($tool in @("ssh", "scp")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        Die "$tool not found. Install OpenSSH: Settings → Apps → Optional Features → OpenSSH Client"
    }
}

$Target   = "${User}@${Router}"
$RemoteTmp = "/tmp/$($IpkItem.Name)"

Log "IPK     : $($IpkItem.FullName)  ($([math]::Round($IpkItem.Length/1KB, 1)) KB)"
Log "Router  : $Target  (port $Port)"
Log "Remote  : $RemoteTmp"

# ── copy IPK to router ────────────────────────────────────────────────────────

Log "Uploading IPK via scp..."

$scpArgs = @(
    "-P", $Port,
    "-o", "StrictHostKeyChecking=no",
    "-o", "ConnectTimeout=10",
    $IpkItem.FullName,
    "${Target}:${RemoteTmp}"
)

scp @scpArgs
if ($LASTEXITCODE -ne 0) {
    Die "scp failed (exit $LASTEXITCODE). Check router IP, SSH access, and disk space."
}
Ok "Upload complete."

# ── locate clashforgectl.sh ───────────────────────────────────────────────────
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ShScript  = Join-Path $ScriptDir "clashforgectl.sh"
if (-not (Test-Path $ShScript)) {
    $ShScript = Join-Path (Split-Path -Parent $ScriptDir) "scripts\clashforgectl.sh"
}
if (-not (Test-Path $ShScript)) {
    Die "clashforgectl.sh not found. Expected: $(Join-Path $ScriptDir 'clashforgectl.sh')"
}

$RemoteScript = "/tmp/clashforgectl.sh"

# ── upload clashforgectl.sh to router ─────────────────────────────────────────
Log "Uploading clashforgectl.sh to router..."
$scpCtlArgs = @(
    "-P", $Port,
    "-o", "StrictHostKeyChecking=no",
    "-o", "ConnectTimeout=10",
    $ShScript,
    "${Target}:${RemoteScript}"
)
scp @scpCtlArgs
if ($LASTEXITCODE -ne 0) {
    Die "scp upload of clashforgectl.sh failed (exit $LASTEXITCODE)."
}
Ok "Upload complete."

# ── run pre-install cleanup via clashforgectl ─────────────────────────────────
$SshBase = @(
    "-p", $Port,
    "-o", "StrictHostKeyChecking=no",
    "-o", "ConnectTimeout=10",
    $Target
)

# ── snapshot running state before we stop anything ───────────────────────────
$WasRunning = (ssh @SshBase @'
pgrep -f "/usr/bin/clashforge" >/dev/null 2>&1 && echo 1 && exit
pgrep -f "/usr/bin/mihomo-clashforge" >/dev/null 2>&1 && echo 1 && exit
nft list table inet metaclash >/dev/null 2>&1 && echo 1 && exit
[ -f /etc/init.d/clashforge ] && /etc/init.d/clashforge enabled 2>/dev/null && echo 1 && exit
echo 0
'@).Trim() -eq "1"

if ($Purge) {
    Log "Running reset --yes on router before install (--purge)..."
    ssh @SshBase "sh $RemoteScript reset --yes"
    $WasRunning = $false   # no config left to start from after full purge
} else {
    Log "Running stop on router before install..."
    ssh @SshBase "sh $RemoteScript stop"
}
if ($LASTEXITCODE -ne 0) {
    Warn "Pre-install cleanup returned non-zero (exit $LASTEXITCODE) — continuing with install."
}

# ── install IPK via opkg ──────────────────────────────────────────────────────
Log "Installing IPK via opkg..."
ssh @SshBase "opkg install --nodeps --force-downgrade '$RemoteTmp' && rm -f '$RemoteTmp'"
if ($LASTEXITCODE -ne 0) {
    Die "Remote install failed (exit $LASTEXITCODE)."
}

# ── restart service if it was running before the upgrade ─────────────────────
if ($WasRunning) {
    Log "Service was running before upgrade — restarting with new version..."
    ssh @SshBase "/etc/init.d/clashforge enable 2>/dev/null; /etc/init.d/clashforge start"
    if ($LASTEXITCODE -eq 0) {
        Ok "clashforge service restarted successfully."
    } else {
        Warn "Service start returned non-zero — check: /etc/init.d/clashforge status"
    }
} else {
    Log "Service was not running before upgrade — skipping auto-start."
}
