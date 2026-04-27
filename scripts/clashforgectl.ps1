# clashforgectl.ps1 — ClashForge Remote Control Script (Windows)
#
# Uploads clashforgectl.sh to the target router and executes the requested
# subcommand over SSH, providing a unified control surface from Windows.
#
# Usage:
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 status
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 stop
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 reset
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 reset -Start
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade -Version v0.1.0
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade -Mirror https://ghproxy.com
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade -BaseUrl https://releases.example.com
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade -Purge
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 uninstall
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 uninstall -KeepConfig
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 diag
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 diag -Fetch
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 diag -Fetch -LocalPath .\cf-diag.txt -Redact
#
# Requirements:
#   - ssh and scp must be available (OpenSSH bundled with Windows 10/11)
#   - clashforgectl.sh must exist alongside this script (or in scripts/)
#   - Password-less SSH key auth is recommended

[CmdletBinding()]
param(
    # Subcommand — positional argument 0
    [Parameter(Mandatory, Position = 0)]
    [ValidateSet("status", "stop", "reset", "upgrade", "uninstall", "diag", "help")]
    [string]$Action,

    # Connection parameters
    [Parameter(Mandatory)]
    [string]$Router,                      # Router IP or hostname

    [string]$User     = "root",
    [int]$Port        = 22,
    [string]$Identity = "",               # Path to SSH private key (optional)

    # upgrade options
    [string]$Version  = "latest",
    [string]$Mirror   = "",
    [string]$BaseUrl  = "",
    [switch]$Purge,

    # reset options
    [switch]$Start,

    # uninstall options
    [switch]$KeepConfig,

    # diag options
    [switch]$Fetch,                       # Download report to local machine after collection
    [string]$RemoteOutput = "/tmp/cf-diag.txt",  # Remote report path
    [string]$LocalPath    = "",           # Local path for fetched report
    [switch]$Redact,                      # Best-effort masking of sensitive values

    # common options
    [switch]$Yes,
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ── helpers ───────────────────────────────────────────────────────────────────
function Log  { param($Msg) Write-Host "[clashforgectl] $Msg"       -ForegroundColor Cyan }
function Ok   { param($Msg) Write-Host "[clashforgectl] OK  $Msg"   -ForegroundColor Green }
function Warn { param($Msg) Write-Host "[clashforgectl] WARN $Msg"  -ForegroundColor Yellow }
function Die  { param($Msg) Write-Host "[clashforgectl] ERROR $Msg" -ForegroundColor Red; exit 1 }

# ── locate clashforgectl.sh ───────────────────────────────────────────────────
# Look next to this PS1 file first, then in the scripts/ directory.
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ShScript  = Join-Path $ScriptDir "clashforgectl.sh"

if (-not (Test-Path $ShScript)) {
    # Try workspace root scripts/ directory
    $ShScript = Join-Path (Split-Path -Parent $ScriptDir) "scripts\clashforgectl.sh"
}

if (-not (Test-Path $ShScript)) {
    Die "clashforgectl.sh not found next to this script. Expected: $(Join-Path $ScriptDir 'clashforgectl.sh')"
}

# ── validate tools ────────────────────────────────────────────────────────────
foreach ($Tool in @("ssh", "scp")) {
    if (-not (Get-Command $Tool -ErrorAction SilentlyContinue)) {
        Die "$Tool not found. Install OpenSSH: Settings → Apps → Optional Features → OpenSSH Client"
    }
}

# ── show help ─────────────────────────────────────────────────────────────────
if ($Action -eq "help") {
    Get-Help $MyInvocation.MyCommand.Path -Detailed
    exit 0
}

# ── build SSH / SCP base args ─────────────────────────────────────────────────
$Target  = "${User}@${Router}"
$SshBase = [System.Collections.Generic.List[string]]@(
    "-p", $Port,
    "-o", "StrictHostKeyChecking=no",
    "-o", "ConnectTimeout=15"
)
if ($Identity -ne "") {
    $SshBase.Add("-i")
    $SshBase.Add($Identity)
}

$ScpBase = [System.Collections.Generic.List[string]]@(
    "-P", $Port,
    "-o", "StrictHostKeyChecking=no",
    "-o", "ConnectTimeout=15"
)
if ($Identity -ne "") {
    $ScpBase.Add("-i")
    $ScpBase.Add($Identity)
}

$RemoteScript = "/tmp/clashforgectl.sh"

Log "Router  : $Target  (port $Port)"
Log "Script  : $ShScript"
Log "Action  : $Action"

# ── upload clashforgectl.sh to router ─────────────────────────────────────────
Log "Uploading clashforgectl.sh to router..."
$ScpUploadArgs = $ScpBase + @($ShScript, "${Target}:${RemoteScript}")
scp @ScpUploadArgs
if ($LASTEXITCODE -ne 0) {
    Die "scp upload failed (exit $LASTEXITCODE). Check router SSH access and disk space."
}
Ok "Upload complete."

# ── build remote command string ───────────────────────────────────────────────
# Values containing slashes or special characters are single-quoted for sh.
function ShQuote { param([string]$s) "'$($s -replace "'", "'\''")'" }

$RemoteCmd = "sh $RemoteScript $Action"

switch ($Action) {
    "upgrade" {
        if ($Version -ne "latest") { $RemoteCmd += " --version $(ShQuote $Version)" }
        if ($Mirror  -ne "")       { $RemoteCmd += " --mirror $(ShQuote $Mirror)" }
        if ($BaseUrl -ne "")       { $RemoteCmd += " --base-url $(ShQuote $BaseUrl)" }
        if ($Purge)                { $RemoteCmd += " --purge" }
    }
    "reset" {
        if ($Start) { $RemoteCmd += " --start" }
    }
    "uninstall" {
        if ($KeepConfig) { $RemoteCmd += " --keep-config" }
    }
    "diag" {
        $RemoteCmd += " --output $(ShQuote $RemoteOutput)"
        if ($Redact) { $RemoteCmd += " --redact" }
        # Always capture to file; --stdout only needed when not fetching
        if (-not $Fetch) { $RemoteCmd += " --stdout" }
    }
}

if ($Yes)    { $RemoteCmd += " --yes" }
if ($DryRun) { $RemoteCmd += " --dry-run" }

Log "Remote  : $RemoteCmd"
Log ""

# ── execute on router ─────────────────────────────────────────────────────────
$SshExecArgs = $SshBase + @($Target, $RemoteCmd)
ssh @SshExecArgs
if ($LASTEXITCODE -ne 0) {
    Die "Remote command failed (exit $LASTEXITCODE)."
}

# ── diag: fetch report to local machine ───────────────────────────────────────
if ($Action -eq "diag" -and $Fetch) {
    # Build local output path: default to cf-diag_<router>_<timestamp>.txt
    if ($LocalPath -eq "") {
        $Timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
        $LocalPath = "cf-diag_${Router}_${Timestamp}.txt"
    }

    Log ""
    Log "Fetching diagnostic report from router..."
    if ($Redact) {
        Warn "Fetching report with redaction applied on router side."
    } else {
        Warn "Report is UNREDACTED — handle with care (may contain subscription URLs, tokens)."
    }

    $ScpFetchArgs = $ScpBase + @("${Target}:${RemoteOutput}", $LocalPath)
    scp @ScpFetchArgs
    if ($LASTEXITCODE -ne 0) {
        Die "scp fetch failed (exit $LASTEXITCODE). Remote path: $RemoteOutput"
    }

    $FullLocalPath = (Resolve-Path $LocalPath).Path
    Ok ""
    Ok "Diagnostic report fetched:"
    Ok "  Remote : ${RemoteOutput}"
    Ok "  Local  : ${FullLocalPath}"
}

Ok ""
Ok "Done."
