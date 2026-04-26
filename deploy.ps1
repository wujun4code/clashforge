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

# ── 3b. Sync openwrt/files helper scripts → ipk/ ─────────────────────────────
Step "Syncing openwrt/files helpers → ipk/"
$helperMap = @{
    "$Root\openwrt\files\usr\bin\clashforge-diag"                                      = "$Root\ipk\usr\bin\clashforge-diag"
    "$Root\openwrt\files\usr\bin\uninstall-clashforge.sh"                              = "$Root\ipk\usr\bin\uninstall-clashforge"
    "$Root\openwrt\files\etc\init.d\metaclash"                                         = "$Root\ipk\etc\init.d\clashforge"
    "$Root\openwrt\files\usr\share\luci\menu.d\luci-app-clashforge.json"               = "$Root\ipk\usr\share\luci\menu.d\luci-app-clashforge.json"
    "$Root\openwrt\files\www\luci-static\resources\view\clashforge\main.js"            = "$Root\ipk\www\luci-static\resources\view\clashforge\main.js"
    "$Root\openwrt\files\usr\share\rpcd\acl.d\luci-app-clashforge.json"                = "$Root\ipk\usr\share\rpcd\acl.d\luci-app-clashforge.json"
    "$Root\openwrt\files\etc\metaclash\postinst.sh"                                    = "$Root\ipk\CONTROL\postinst"
    "$Root\openwrt\files\etc\metaclash\prerm.sh"                                       = "$Root\ipk\CONTROL\prerm"
    "$Root\openwrt\files\etc\metaclash\postrm.sh"                                      = "$Root\ipk\CONTROL\postrm"
}
foreach ($src in $helperMap.Keys) {
    $dst = $helperMap[$src]
    if (Test-Path $src) {
        $dstDir = Split-Path $dst
        if (-not (Test-Path $dstDir)) { New-Item -ItemType Directory -Force $dstDir | Out-Null }
        Copy-Item -Force $src $dst
        OK "$(Split-Path $src -Leaf)"
    } else {
        Write-Host "    SKIP (not found): $src" -ForegroundColor Yellow
    }
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

# ── 6. Pre-upgrade cleanup + install ─────────────────────────────────────────
if ($Purge) {
    Step "Installing on router (FULL PURGE — wiping all config)"
    Write-Host "    WARN: -Purge is set. All user config will be erased." -ForegroundColor Yellow
    $purgeAction = "do_purge"
} else {
    Step "Installing on router (keep-config)"
    $purgeAction = "pre_upgrade_cleanup"
}

$deployScript = ((@'
pre_upgrade_cleanup() {
  _cf_running=0
  pgrep -f "/usr/bin/clashforge" >/dev/null 2>&1 && _cf_running=1
  pgrep -f "/usr/bin/mihomo-clashforge" >/dev/null 2>&1 && _cf_running=1
  nft list table inet metaclash >/dev/null 2>&1 && _cf_running=1
  if [ "$_cf_running" = "0" ]; then
    echo "[clashforge] pre-upgrade: nothing running, skipping cleanup"
    return 0
  fi
  echo "[clashforge] pre-upgrade: restoring system state..."
  if command -v uci >/dev/null 2>&1; then
    uci -q delete dhcp.@dnsmasq[0].port     || true
    uci -q delete dhcp.@dnsmasq[0].server   || true
    uci -q delete dhcp.@dnsmasq[0].noresolv || true
    uci commit dhcp 2>/dev/null             || true
  fi
  rm -f /etc/dnsmasq.d/clashforge.conf 2>/dev/null || true
  /etc/init.d/dnsmasq restart 2>/dev/null || true
  nft delete table inet metaclash 2>/dev/null || true
  nft delete table inet dnsmasq   2>/dev/null || true
  while ip rule del fwmark 0x1a3 table 100 2>/dev/null; do :; done
  ip route flush table 100 2>/dev/null || true
  while ip -6 rule del fwmark 0x1a3 table 100 2>/dev/null; do :; done
  ip -6 route flush table 100 2>/dev/null || true
  while ip rule del fwmark 0x1a4 table 101 2>/dev/null; do :; done
  ip route flush table 101 2>/dev/null || true
  /etc/init.d/clashforge stop 2>/dev/null || true
  sleep 1
  for _name in clashforge mihomo-clashforge; do
    _pids=$(pgrep -f "/usr/bin/$_name" 2>/dev/null || true)
    if [ -n "$_pids" ]; then
      kill $_pids 2>/dev/null || true
      sleep 1
      _pids=$(pgrep -f "/usr/bin/$_name" 2>/dev/null || true)
      [ -n "$_pids" ] && kill -9 $_pids 2>/dev/null || true
    fi
  done
  echo "[clashforge] pre-upgrade cleanup complete"
}

do_purge() {
  pre_upgrade_cleanup
  if command -v opkg >/dev/null 2>&1 && opkg status clashforge 2>/dev/null | grep -q "^Package:"; then
    opkg remove clashforge 2>/dev/null || true
  fi
  rm -rf /etc/metaclash /usr/share/metaclash /var/run/metaclash
  rm -f /var/log/clashforge.log
  echo "[clashforge] Purge complete."
}

PURGE_ACTION_PLACEHOLDER
opkg install --nodeps --force-downgrade REMOTE_PATH_PLACEHOLDER
rm -f REMOTE_PATH_PLACEHOLDER
'@) -replace '\r\n', "`n") -replace 'PURGE_ACTION_PLACEHOLDER', $purgeAction `
                             -replace 'REMOTE_PATH_PLACEHOLDER',   $remote

$deployScript | ssh "root@$Router" "sh -s"
if ($LASTEXITCODE -ne 0) { Die "SSH install failed" }

Write-Host "`nDeploy complete: $new  →  http://${Router}:7777" -ForegroundColor Green
