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
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 deploy
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 deploy -Skip ui
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 deploy -Skip go
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 deploy -Purge
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
    [ValidateSet("status", "stop", "reset", "upgrade", "deploy", "uninstall", "diag", "help")]
    [string]$Action,

    # Connection parameters
    [Parameter(Mandatory)]
    [string]$Router,                      # Router IP or hostname

    [string]$User     = "root",
    [int]$Port        = 22,
    [string]$Identity = "",               # Path to SSH private key (optional)

    # deploy options (local build + package + push)
    [string]$Skip     = "",              # Comma-separated steps to skip: "ui", "go"
    [string]$RepoRoot = "",              # Repo root path (default: auto-detect from script location)

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

# ── resolve repo root (for deploy) ───────────────────────────────────────────
if ($RepoRoot -eq "") {
    # Walk up from the script directory to find the repo root (contains go.mod)
    $RepoRoot = $ScriptDir
    if (-not (Test-Path (Join-Path $RepoRoot "go.mod"))) {
        $RepoRoot = Split-Path -Parent $ScriptDir
    }
    if (-not (Test-Path (Join-Path $RepoRoot "go.mod"))) {
        Die "Cannot auto-detect repo root (no go.mod found). Pass -RepoRoot explicitly."
    }
}
$RepoRoot = (Resolve-Path $RepoRoot).Path

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

# ══════════════════════════════════════════════════════════════════════════════
# DEPLOY — local build + package + push to router
# ══════════════════════════════════════════════════════════════════════════════
if ($Action -eq "deploy") {
    Log "Repo root: $RepoRoot"
    Log ""

    # ── 1. Bump patch version ─────────────────────────────────────────────────
    Log "── Step 1: Bumping version"
    $ControlPath = Join-Path $RepoRoot "ipk\CONTROL\control"
    if (-not (Test-Path $ControlPath)) { Die "control file not found: $ControlPath" }
    $ctrl = Get-Content $ControlPath -Raw
    if ($ctrl -match 'Version:\s*([\d\.\-a-z]+)') {
        $OldVer = $Matches[1]
        if ($OldVer -match '^(.*\.)(\d+)$') {
            $NewVer = "$($Matches[1])$([int]$Matches[2] + 1)"
        } else {
            $NewVer = "$OldVer.1"
        }
        $ctrl = $ctrl -replace "Version: $([regex]::Escape($OldVer))", "Version: $NewVer"
        Set-Content $ControlPath $ctrl -NoNewline
        Ok "Version $OldVer  →  $NewVer"
    } else {
        Die "Cannot parse version from control file: $ControlPath"
    }
    $IpkName = "clashforge_${NewVer}_x86_64.ipk"

    # ── 2. Build UI ───────────────────────────────────────────────────────────
    if ($Skip -notmatch '\bui\b') {
        Log "── Step 2: Building React UI"
        Push-Location (Join-Path $RepoRoot "ui")
        try {
            $env:VITE_APP_VERSION = $NewVer
            npm run build | Select-Object -Last 10
            if ($LASTEXITCODE -ne 0) { Die "npm run build failed" }
        } finally {
            $env:VITE_APP_VERSION = ''
            Pop-Location
        }
        Log "Syncing UI dist → internal/api/ui_dist"
        $UiDist = Join-Path $RepoRoot "internal\api\ui_dist"
        Remove-Item -Recurse -Force "$UiDist\*" -ErrorAction SilentlyContinue
        Copy-Item -Recurse (Join-Path $RepoRoot "ui\dist\*") "$UiDist\"
        Ok "UI build complete"
    } else {
        Warn "Skipping UI build (-Skip contains 'ui')"
    }

    # ── 3. Cross-compile Go ───────────────────────────────────────────────────
    if ($Skip -notmatch '\bgo\b') {
        Log "── Step 3: Cross-compiling Go (linux/amd64)"
        $env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
        try {
            $BinOut = Join-Path $RepoRoot "ipk\usr\bin\clashforge"
            go build -trimpath -ldflags='-s -w' -o $BinOut (Join-Path $RepoRoot "cmd\clashforge")
            if ($LASTEXITCODE -ne 0) { Die "go build failed" }
        } finally {
            $env:GOOS = ''; $env:GOARCH = ''; $env:CGO_ENABLED = ''
        }
        Ok "Binary written to ipk/usr/bin/clashforge"
    } else {
        Warn "Skipping Go build (-Skip contains 'go')"
    }

    # ── 4. Sync openwrt/files helpers → ipk/ ─────────────────────────────────
    Log "── Step 4: Syncing openwrt/files helpers → ipk/"
    $HelperMap = @{
        "openwrt\files\usr\bin\clashforge-diag"                             = "ipk\usr\bin\clashforge-diag"
        "openwrt\files\usr\bin\uninstall-clashforge.sh"                     = "ipk\usr\bin\uninstall-clashforge"
        "openwrt\files\etc\init.d\metaclash"                                = "ipk\etc\init.d\clashforge"
        "openwrt\files\usr\share\luci\menu.d\luci-app-clashforge.json"      = "ipk\usr\share\luci\menu.d\luci-app-clashforge.json"
        "openwrt\files\www\luci-static\resources\view\clashforge\main.js"   = "ipk\www\luci-static\resources\view\clashforge\main.js"
        "openwrt\files\usr\share\rpcd\acl.d\luci-app-clashforge.json"       = "ipk\usr\share\rpcd\acl.d\luci-app-clashforge.json"
        "openwrt\files\etc\metaclash\postinst.sh"                           = "ipk\CONTROL\postinst"
        "openwrt\files\etc\metaclash\prerm.sh"                              = "ipk\CONTROL\prerm"
        "openwrt\files\etc\metaclash\postrm.sh"                             = "ipk\CONTROL\postrm"
    }
    foreach ($RelSrc in $HelperMap.Keys) {
        $Src = Join-Path $RepoRoot $RelSrc
        $Dst = Join-Path $RepoRoot $HelperMap[$RelSrc]
        if (Test-Path $Src) {
            $DstDir = Split-Path $Dst
            if (-not (Test-Path $DstDir)) { New-Item -ItemType Directory -Force $DstDir | Out-Null }
            Copy-Item -Force $Src $Dst
            Ok (Split-Path $Src -Leaf)
        } else {
            Warn "Not found (skipped): $RelSrc"
        }
    }

    # ── 5. Build IPK ─────────────────────────────────────────────────────────
    Log "── Step 5: Building IPK ($IpkName)"
    $BuildScript = Join-Path $RepoRoot "scripts\build_ipk.py"
    if (-not (Test-Path $BuildScript)) { Die "build_ipk.py not found: $BuildScript" }
    Push-Location $RepoRoot
    try {
        $env:PKG_NAME = $IpkName
        python $BuildScript
        if ($LASTEXITCODE -ne 0) { Die "build_ipk.py failed" }
        $env:PKG_NAME = ''
    } finally {
        Pop-Location
    }
    $LocalIpk = Join-Path $RepoRoot $IpkName
    Ok "IPK ready: $LocalIpk"

    # ── 6. Upload IPK to router ───────────────────────────────────────────────
    $RemoteIpk = "/tmp/cf_deploy.ipk"
    Log "── Step 6: Uploading $IpkName → ${Target}:${RemoteIpk}"
    $ScpIpkArgs = $ScpBase + @($LocalIpk, "${Target}:${RemoteIpk}")
    scp @ScpIpkArgs
    if ($LASTEXITCODE -ne 0) { Die "scp upload of IPK failed" }
    Ok "IPK uploaded"

    # ── 7. Upload clashforgectl.sh ────────────────────────────────────────────
    Log "── Step 7: Uploading clashforgectl.sh → ${Target}:${RemoteScript}"
    $ScpCtlArgs = $ScpBase + @($ShScript, "${Target}:${RemoteScript}")
    scp @ScpCtlArgs
    if ($LASTEXITCODE -ne 0) { Die "scp upload of clashforgectl.sh failed" }
    Ok "clashforgectl.sh uploaded"

    # ── 8. Install on router via clashforgectl upgrade --local-ipk ───────────
    $PurgeFlag = if ($Purge) { " --purge" } else { "" }
    $RemoteInstall = "sh $RemoteScript upgrade --local-ipk $RemoteIpk$PurgeFlag"
    Log "── Step 8: Installing on router"
    Log "Remote  : $RemoteInstall"
    Log ""
    $SshInstallArgs = $SshBase + @($Target, $RemoteInstall)
    ssh @SshInstallArgs
    if ($LASTEXITCODE -ne 0) { Die "Remote install failed (exit $LASTEXITCODE)" }

    Log ""
    Ok "Deploy complete: $NewVer  →  http://${Router}:7777"
    exit 0
}


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
