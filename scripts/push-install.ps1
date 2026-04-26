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

# ── build remote install commands ─────────────────────────────────────────────
# Mirrors install.sh exactly:
#   non-purge → pre_upgrade_cleanup then opkg install
#   -Purge    → do_purge (calls pre_upgrade_cleanup + opkg remove + wipe) then opkg install

$purgeAction = if ($Purge) { "do_purge" } else { "pre_upgrade_cleanup" }

$remoteScript = (@'
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
opkg install --nodeps --force-downgrade IPK_PATH_PLACEHOLDER
rm -f IPK_PATH_PLACEHOLDER
'@) -replace 'PURGE_ACTION_PLACEHOLDER', $purgeAction `
   -replace 'IPK_PATH_PLACEHOLDER',   $RemoteTmp

# ── execute on router ─────────────────────────────────────────────────────────

Log "Running installer on router..."

$sshArgs = @(
    "-p", $Port,
    "-o", "StrictHostKeyChecking=no",
    "-o", "ConnectTimeout=10",
    $Target,
    "sh -s"
)

$remoteScript | ssh @sshArgs
if ($LASTEXITCODE -ne 0) {
    Die "Remote install failed (exit $LASTEXITCODE)."
}
