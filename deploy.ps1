#!/usr/bin/env pwsh
# deploy.ps1 — Build and deploy ClashForge to OpenWrt router
# Usage: .\deploy.ps1 [-Router 192.168.20.1] [-Skip ui|go] [-Purge]
#   -Purge  : fully wipe /etc/metaclash and /usr/share/metaclash (fresh install)
#   default : keep user config/subscriptions between deploys
param(
    [string]$Router = "192.168.20.1",
    [string]$Skip   = "",  # comma-separated: "ui", "go"
    [switch]$Purge  = $false
)
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$Root = Split-Path $MyInvocation.MyCommand.Path

function Step($msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }
function OK($msg)   { Write-Host "    OK: $msg" -ForegroundColor Green }
function Die($msg)  { Write-Host "    ERROR: $msg" -ForegroundColor Red; exit 1 }

# ── 1. Bump patch version ─────────────────────────────────────────────────────
Step "Bumping version"
$controlPath = "$Root\ipk\CONTROL\control"
$ctrl = Get-Content $controlPath -Raw
if ($ctrl -match 'Version:\s*([\d\.\-a-z]+)') {
    $old = $Matches[1]
    if ($old -match '^(.*\.)(\d+)$') {
        $new = "$($Matches[1])$([int]$Matches[2] + 1)"
    } else {
        $new = "$old.1"
    }
    $ctrl = $ctrl -replace "Version: $([regex]::Escape($old))", "Version: $new"
    Set-Content $controlPath $ctrl -NoNewline
    OK "$old  →  $new"
} else {
    Die "Cannot parse version from control file"
}

# ── 2. Build UI ───────────────────────────────────────────────────────────────
if ($Skip -notmatch '\bui\b') {
    Step "Building React UI"
    Push-Location "$Root\ui"
    $env:VITE_APP_VERSION = $new
    npm run build | Select-Object -Last 8
    $env:VITE_APP_VERSION = ''
    if ($LASTEXITCODE -ne 0) { Pop-Location; Die "npm build failed" }
    Pop-Location

    Step "Syncing UI dist → internal/api/ui_dist"
    Remove-Item -Recurse -Force "$Root\internal\api\ui_dist\*" -ErrorAction SilentlyContinue
    Copy-Item -Recurse "$Root\ui\dist\*" "$Root\internal\api\ui_dist\"
    OK "Synced"
} else {
    Write-Host "    Skipped UI build" -ForegroundColor Yellow
}

# ── 3. Cross-compile Go ───────────────────────────────────────────────────────
if ($Skip -notmatch '\bgo\b') {
    Step "Cross-compiling Go (linux/amd64)"
    $env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
    go build -trimpath -ldflags='-s -w' -o "$Root\ipk\usr\bin\clashforge" "$Root\cmd\clashforge"
    if ($LASTEXITCODE -ne 0) { Die "go build failed" }
    $env:GOOS = ''; $env:GOARCH = ''; $env:CGO_ENABLED = ''
    OK "Binary written to ipk/usr/bin/clashforge"
} else {
    Write-Host "    Skipped Go build" -ForegroundColor Yellow
}

# ── 4. Build IPK ─────────────────────────────────────────────────────────────
Step "Building IPK ($new)"
$ipkName = "clashforge_${new}_x86_64.ipk"
$env:PKG_NAME = $ipkName
python "$Root\scripts\build_ipk.py"
if ($LASTEXITCODE -ne 0) { Die "build_ipk.py failed" }
OK $ipkName

# ── 5. Upload to router ───────────────────────────────────────────────────────
$remote = "/tmp/cf_deploy.ipk"
Step "Uploading $ipkName → root@${Router}:$remote"
scp "$Root\$ipkName" "root@${Router}:$remote"
if ($LASTEXITCODE -ne 0) { Die "scp failed" }
OK "Upload complete"

# ── 6. Uninstall old + install new ───────────────────────────────────────────
if ($Purge) {
    Step "Installing on router (FULL PURGE — wiping all config)"
    $uninstallCmd = "uninstall-clashforge"
    Write-Host "    WARN: -Purge is set. All user config will be erased." -ForegroundColor Yellow
} else {
    Step "Installing on router (keep-config)"
    $uninstallCmd = "uninstall-clashforge --keep-config"
}
$cmds = "$uninstallCmd; opkg install $remote; sleep 3; pgrep -af clashforge > /dev/null || { echo '[WARN] clashforge process not found after install'; pgrep -af clashforge; }; /etc/init.d/clashforge enabled 2>/dev/null || echo '[WARN] clashforge service not enabled'"
ssh "root@$Router" $cmds
if ($LASTEXITCODE -ne 0) { Die "SSH install failed" }

Write-Host "`nDeploy complete: $new  →  http://${Router}:7777" -ForegroundColor Green
