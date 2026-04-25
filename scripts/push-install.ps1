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

$purgeBlock = @'
echo "[clashforge] Purging old installation..."
/etc/init.d/clashforge stop 2>/dev/null || true
/etc/init.d/clashforge disable 2>/dev/null || true
for pid in $(ls /proc 2>/dev/null | grep '^[0-9]'); do
  cmdline=$(tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null) || continue
  case "$cmdline" in
    *"/usr/bin/clashforge"*|*"/usr/bin/mihomo-clashforge"*)
      kill -9 "$pid" 2>/dev/null || true ;;
  esac
done
nft delete table inet metaclash 2>/dev/null || true
while ip rule del fwmark 0x1a3 table 100 2>/dev/null; do :; done
ip route flush table 100 2>/dev/null || true
rm -f /etc/dnsmasq.d/clashforge.conf /tmp/dnsmasq.d/clashforge.conf
/etc/init.d/dnsmasq restart 2>/dev/null || /etc/init.d/dnsmasq reload 2>/dev/null || true
if command -v opkg >/dev/null 2>&1 && opkg status clashforge 2>/dev/null | grep -q "^Package:"; then
  opkg remove clashforge 2>/dev/null || true
fi
rm -rf /etc/metaclash /usr/share/metaclash /var/run/metaclash
rm -f /var/log/clashforge.log
echo "[clashforge] Purge complete."
'@

$installBlock = (@'
set -e
echo '[clashforge] Installing via opkg...'
opkg install --nodeps --force-downgrade 'REMOTE_TMP_PLACEHOLDER'
rm -f 'REMOTE_TMP_PLACEHOLDER'
ROUTER_IP=$(ip -4 addr show br-lan 2>/dev/null | awk '/inet /{split($2,a,"/"); print a[1]; exit}')
[ -z "$ROUTER_IP" ] && ROUTER_IP=$(ip -4 addr show 2>/dev/null | awk '/inet /{split($2,a,"/"); if(a[1]!="127.0.0.1"){print a[1];exit}}')
[ -z "$ROUTER_IP" ] && ROUTER_IP='<router-ip>'
echo ''
echo '[clashforge] Installed successfully!'
echo "[clashforge] Web UI -> http://${ROUTER_IP}:7777"
echo ''
'@) -replace 'REMOTE_TMP_PLACEHOLDER', $RemoteTmp

if ($Purge) {
    $remoteScript = $purgeBlock + "`n" + $installBlock
} else {
    $remoteScript = $installBlock
}

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
