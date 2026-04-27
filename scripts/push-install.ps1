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

if ($Purge) {
    Log "Running reset --yes on router before install (--purge)..."
    ssh @SshBase "sh $RemoteScript reset --yes"
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
