# uninstall.ps1 — Completely remove ClashForge from an OpenWrt router
#
# Restores everything clashforge touched back to its pre-install state:
#   - Stops clashforge service + kills all processes (clashforge, mihomo-clashforge)
#   - Removes nftables table inet metaclash  (tproxy + dns_redirect chains)
#   - Removes nftables table inet dnsmasq    (the hijack table dnsmasq re-injects)
#   - Cleans up ip rule fwmark 0x1a3 + route table 100 (IPv4 + IPv6)
#   - Restores dnsmasq via UCI (deletes port=0, server=, noresolv= overrides)
#   - Removes /etc/dnsmasq.d/clashforge.conf  (non-UCI fallback)
#   - Restarts dnsmasq so port 53 is listening again
#   - Removes the opkg package (clashforge)
#   - Wipes all data: /etc/metaclash  /usr/share/metaclash  /var/run/metaclash
#                     /var/log/clashforge.log
#
# Usage:
#   .\uninstall.ps1 -Router 192.168.10.1
#   .\uninstall.ps1 -Router 192.168.10.1 -User root -Port 22
#   .\uninstall.ps1 -Router 192.168.10.1 -KeepConfig   # skip wiping /etc/metaclash

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$Router,

    [string]$User       = "root",
    [int]$Port          = 22,
    [switch]$KeepConfig   # keep /etc/metaclash (subscription + override data)
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Log  { param($msg) Write-Host "[uninstall] $msg" -ForegroundColor Cyan }
function Ok   { param($msg) Write-Host "[uninstall] $msg" -ForegroundColor Green }
function Warn { param($msg) Write-Host "[uninstall] WARN: $msg" -ForegroundColor Yellow }
function Die  { param($msg) Write-Host "[uninstall] ERROR: $msg" -ForegroundColor Red; exit 1 }

foreach ($tool in @("ssh")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        Die "$tool not found. Install OpenSSH: Settings -> Apps -> Optional Features -> OpenSSH Client"
    }
}

$Target  = "${User}@${Router}"
$SshBase = @("-p", $Port, "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=15", $Target)


Log "Target  : $Target  (port $Port)"
Log "KeepConfig : $($KeepConfig.IsPresent)"
Log ""

# ── locate clashforgectl.sh ───────────────────────────────────────────────────
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ShScript  = Join-Path $ScriptDir "clashforgectl.sh"
if (-not (Test-Path $ShScript)) {
    $ShScript = Join-Path (Split-Path -Parent $ScriptDir) "scripts\clashforgectl.sh"
}
if (-not (Test-Path $ShScript)) {
    Die "clashforgectl.sh not found. Expected: $(Join-Path $ScriptDir 'clashforgectl.sh')"
}

# ── validate scp ──────────────────────────────────────────────────────────────
if (-not (Get-Command scp -ErrorAction SilentlyContinue)) {
    Die "scp not found. Install OpenSSH: Settings -> Apps -> Optional Features -> OpenSSH Client"
}

# ── upload clashforgectl.sh ───────────────────────────────────────────────────
$RemoteScript = "/tmp/clashforgectl.sh"
Log "Uploading clashforgectl.sh to router..."
$scpArgs = @(
    "-P", $Port,
    "-o", "StrictHostKeyChecking=no",
    "-o", "ConnectTimeout=15",
    $ShScript,
    "${Target}:${RemoteScript}"
)
scp @scpArgs
if ($LASTEXITCODE -ne 0) {
    Die "scp upload failed (exit $LASTEXITCODE). Check router SSH access and disk space."
}
Ok "Upload complete."

# ── run uninstall via clashforgectl ──────────────────────────────────────────
$RemoteCmd = "sh $RemoteScript uninstall --yes"
if ($KeepConfig) { $RemoteCmd += " --keep-config" }

$sshArgs = @(
    "-p", $Port,
    "-o", "StrictHostKeyChecking=no",
    "-o", "ConnectTimeout=15",
    $Target,
    $RemoteCmd
)

Log "Executing uninstall on router..."
Log ""
ssh @sshArgs
if ($LASTEXITCODE -ne 0) {
    Die "Remote uninstall failed (exit $LASTEXITCODE)."
}
