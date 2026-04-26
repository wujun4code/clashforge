#!/bin/sh
# install.sh — One-shot ClashForge installer / upgrader for OpenWrt
#
# ── Standard (direct GitHub) ────────────────────────────────────────────────
#   wget -qO- https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh
#
# ── 国内加速 / China mirror (via ghproxy) ───────────────────────────────────
#   wget -qO- https://ghproxy.com/https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh
#   wget -qO- https://mirror.ghproxy.com/https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh
#
# Options:
#   --version <tag>   Install a specific release tag (default: latest, including pre-releases)
#   --purge           Uninstall old version and wipe all config/data before installing
#   --mirror <url>    Force a specific GitHub mirror prefix (e.g. https://ghproxy.com)
#                     When set, only that mirror is tried (no auto-fallback).
#   --help            Show this help

set -e

REPO="wujun4code/clashforge"
INSTALL_VERSION="latest"
PURGE=0
MIRROR=""    # empty = auto-detect (try direct then mirrors)
BASE_URL=""  # custom base URL, e.g. https://releases.example.com
             # files expected at: <BASE_URL>/releases/<tag>/<ipk>

# ── helpers ───────────────────────────────────────────────────────────────────

log()  { echo "[clashforge] $*" 1>&2; }
ok()   { echo "[clashforge] $*" 1>&2; }
warn() { echo "[clashforge] WARN: $*" 1>&2; }
die()  { echo "[clashforge] ERROR: $*" 1>&2; exit 1; }

usage() {
  cat <<'EOF'
install.sh — ClashForge installer for OpenWrt

  wget -qO- https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh

  国内加速 (China mirror):
  wget -qO- https://ghproxy.com/https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh

Options:
  --version <tag>     Install specific version, e.g. v1.2.0  (default: latest)
  --purge             Full clean install: uninstall old version and wipe all config/data
  --mirror <url>      Force a GitHub proxy prefix, e.g. --mirror https://ghproxy.com
  --base-url <url>    Download from a custom base URL (e.g. Cloudflare R2 custom domain)
                      Files must be at: <url>/releases/<tag>/<ipk>
                      Example: --base-url https://releases.example.com
  --help              Show this help
EOF
}

# ── argument parsing ──────────────────────────────────────────────────────────

while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      [ -n "$2" ] || die "--version requires a value"
      INSTALL_VERSION="$2"; shift 2 ;;
    --purge)
      PURGE=1; shift ;;
    --mirror)
      [ -n "$2" ] || die "--mirror requires a value"
      MIRROR="$2"; shift 2 ;;
    --base-url)
      [ -n "$2" ] || die "--base-url requires a value"
      BASE_URL="${2%/}"; shift 2 ;;
    --help|-h)
      usage; exit 0 ;;
    *)
      die "unknown option: $1" ;;
  esac
done

# ── detect OpenWrt IPK architecture label ─────────────────────────────────────
# IPK filenames use OpenWrt arch labels, not uname -m strings.
# Supported: x86_64, aarch64_generic, aarch64_cortex-a53

detect_ipk_arch() {
  machine=$(uname -m 2>/dev/null || echo "unknown")
  case "$machine" in
    x86_64|amd64)
      echo "x86_64" ;;
    aarch64|arm64)
      # Cortex-A53 is extremely common (RPi3, MT7981, MT7986, MT7622...)
      # Check /proc/cpuinfo for the CPU part number: 0xd03 = Cortex-A53
      cpu_part=$(grep -m1 "CPU part" /proc/cpuinfo 2>/dev/null | awk '{print tolower($NF)}')
      if [ "$cpu_part" = "0xd03" ]; then
        echo "aarch64_cortex-a53"
      else
        echo "aarch64_generic"
      fi ;;
    armv7*|armhf)
      die "ARMv7 is not supported. Open an issue at https://github.com/${REPO}/issues" ;;
    *)
      die "Unsupported architecture: $machine (supported: x86_64, aarch64)" ;;
  esac
}

IPK_ARCH=$(detect_ipk_arch)
log "Detected architecture: $IPK_ARCH"

# ── mirror helpers ────────────────────────────────────────────────────────────
# Proxy mirrors: prepend to full github.com URL.
# e.g. https://ghproxy.com/https://github.com/owner/repo/...
GH_PROXIES="https://ghproxy.com https://mirror.ghproxy.com https://ghfast.top https://github.moeyy.xyz"

# Try fetching a URL to stdout (version probe). Short timeout to fail fast.
_fetch_text() {
  url="$1"
  if command -v wget >/dev/null 2>&1; then
    wget --timeout=10 -qO- --user-agent="clashforge-installer/1.0" "$url" 2>/dev/null
  elif command -v curl >/dev/null 2>&1; then
    curl -fsSL --connect-timeout 10 --max-time 15 -A "clashforge-installer/1.0" "$url" 2>/dev/null
  fi
}

# Try downloading a file to $dest. Returns 0 on success.
_fetch_file() {
  url="$1"
  dest="$2"
  if command -v wget >/dev/null 2>&1; then
    wget --timeout=30 -qO "$dest" --user-agent="clashforge-installer/1.0" "$url" 2>/dev/null
  elif command -v curl >/dev/null 2>&1; then
    curl -fsSL --connect-timeout 15 --max-time 300 -A "clashforge-installer/1.0" -o "$dest" "$url" 2>/dev/null
  else
    return 1
  fi
}

# ── resolve version (called once, stored in TAG) ──────────────────────────────

_resolve_tag() {
  awk -F'"tag_name":"' 'NF>1{split($2,a,"\""); print a[1]; exit}'
}

if [ "$INSTALL_VERSION" != "latest" ]; then
  TAG="$INSTALL_VERSION"
else
  log "Resolving latest release from GitHub API..."
  API_PATH="repos/${REPO}/releases?per_page=1"
  TAG=""

  if [ -n "$MIRROR" ]; then
    # User forced a specific mirror
    TAG=$(_fetch_text "${MIRROR}/https://api.github.com/${API_PATH}" | _resolve_tag)
  else
    # Try direct GitHub API first, then fall through proxies
    TAG=$(_fetch_text "https://api.github.com/${API_PATH}" | _resolve_tag)
    if [ -z "$TAG" ]; then
      for _proxy in $GH_PROXIES; do
        TAG=$(_fetch_text "${_proxy}/https://api.github.com/${API_PATH}" | _resolve_tag)
        if [ -n "$TAG" ]; then
          log "Version resolved via mirror: $_proxy"
          break
        fi
      done
    fi
  fi

  [ -n "$TAG" ] || die "Could not resolve latest version. Specify explicitly: sh install.sh --version v0.1.0"
fi

PKG_VER="${TAG#v}"
IPK_NAME="clashforge_${PKG_VER}_${IPK_ARCH}.ipk"
GH_DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${IPK_NAME}"

log "Version : $TAG"
log "Package : $IPK_NAME"

# ── pre-upgrade cleanup ───────────────────────────────────────────────────────
# Run before every install (fresh or upgrade).
# Restores the system to a clean pre-clashforge state without touching user
# config/data (/etc/metaclash is left intact).
#
# ORDER matters for zero-DNS-blackout teardown:
#   1. DNS restore FIRST — UCI deletes port=0 override, dnsmasq restarts on :53.
#      While dnsmasq is restarting the metaclash dns_redirect chain (:53→mihomo)
#      is still active, so LAN clients never lose DNS resolution.
#   2. nftables cleanup SECOND — by the time we delete table inet metaclash,
#      dnsmasq is already on :53 and takes over seamlessly.
#   3. Kill processes LAST — avoids racing with the service's own SIGTERM handler.

pre_upgrade_cleanup() {
  # Quick check: skip if nothing is running and no nft metaclash table exists.
  _cf_running=0
  pgrep -f "/usr/bin/clashforge" >/dev/null 2>&1 && _cf_running=1
  pgrep -f "/usr/bin/mihomo-clashforge" >/dev/null 2>&1 && _cf_running=1
  nft list table inet metaclash >/dev/null 2>&1 && _cf_running=1

  if [ "$_cf_running" = "0" ]; then
    log "pre-upgrade: no running clashforge state detected, skipping cleanup"
    return 0
  fi

  log "pre-upgrade: restoring system state before install..."

  # 1. Restore dnsmasq DNS config (UCI)
  log "  [1/5] restoring dnsmasq via UCI..."
  if command -v uci >/dev/null 2>&1; then
    uci -q delete dhcp.@dnsmasq[0].port    || true   # replace mode: port=0 override
    uci -q delete dhcp.@dnsmasq[0].server  || true   # upstream mode: server= override
    uci -q delete dhcp.@dnsmasq[0].noresolv || true  # upstream mode: noresolv= override
    uci commit dhcp 2>/dev/null            || true
    log "  UCI dhcp.@dnsmasq[0] overrides deleted and committed"
  fi
  # conf-dir fallback (non-UCI systems)
  rm -f /etc/dnsmasq.d/clashforge.conf 2>/dev/null || true
  /etc/init.d/dnsmasq restart 2>/dev/null || true
  log "  dnsmasq restarted (full restart, not SIGHUP)"

  # 2. Remove table inet metaclash (tproxy_prerouting + dns_redirect chains)
  log "  [2/5] removing nftables table inet metaclash..."
  if nft list table inet metaclash >/dev/null 2>&1; then
    nft delete table inet metaclash 2>/dev/null \
      && log "  table inet metaclash deleted" \
      || warn "  failed to delete table inet metaclash (will retry after kill)"
  else
    log "  table inet metaclash not present, skipping"
  fi

  # 3. Remove table inet dnsmasq HIJACK table
  # dnsmasq injects this on every restart (priority dstnat-5).  When port=0 the
  # redirect :53→:53 hits nothing and breaks DNS for all LAN clients.
  log "  [3/5] removing nftables table inet dnsmasq (HIJACK)..."
  if nft list table inet dnsmasq >/dev/null 2>&1; then
    nft delete table inet dnsmasq 2>/dev/null \
      && log "  table inet dnsmasq (HIJACK) deleted" \
      || warn "  failed to delete table inet dnsmasq"
  else
    log "  table inet dnsmasq not present, skipping"
  fi

  # 4. Clean up policy routing rules and route tables (IPv4 + IPv6)
  log "  [4/5] cleaning up policy routing fwmark 0x1a3 / table 100 and fwmark 0x1a4 / table 101..."
  _removed=0
  while ip rule del fwmark 0x1a3 table 100 2>/dev/null; do _removed=$((_removed+1)); done
  [ "$_removed" -gt 0 ] && log "  removed $_removed IPv4 ip rule(s) (0x1a3)" || log "  no IPv4 ip rules to remove (0x1a3)"
  ip route flush table 100 2>/dev/null || true
  _removed6=0
  while ip -6 rule del fwmark 0x1a3 table 100 2>/dev/null; do _removed6=$((_removed6+1)); done
  [ "$_removed6" -gt 0 ] && log "  removed $_removed6 IPv6 ip rule(s) (0x1a3)" || log "  no IPv6 ip rules to remove (0x1a3)"
  ip -6 route flush table 100 2>/dev/null || true
  _removed_out=0
  while ip rule del fwmark 0x1a4 table 101 2>/dev/null; do _removed_out=$((_removed_out+1)); done
  [ "$_removed_out" -gt 0 ] && log "  removed $_removed_out output tproxy ip rule(s) (0x1a4)" || log "  no output tproxy ip rules to remove (0x1a4)"
  ip route flush table 101 2>/dev/null || true

  # 5. Stop clashforge service and kill any remaining processes
  log "  [5/5] stopping clashforge service and processes..."
  /etc/init.d/clashforge stop 2>/dev/null || true
  sleep 1
  for _name in clashforge mihomo-clashforge; do
    _pids=$(pgrep -f "/usr/bin/$_name" 2>/dev/null || true)
    if [ -n "$_pids" ]; then
      # shellcheck disable=SC2086
      kill $_pids 2>/dev/null || true
      sleep 1
      _pids=$(pgrep -f "/usr/bin/$_name" 2>/dev/null || true)
      # shellcheck disable=SC2086
      [ -n "$_pids" ] && kill -9 $_pids 2>/dev/null || true
      log "  $_name processes stopped"
    fi
  done

  ok "pre-upgrade cleanup complete"
}

# ── purge old installation ────────────────────────────────────────────────────

do_purge() {
  log "Purging old installation..."

  # Use pre_upgrade_cleanup for the network/process teardown (correct order).
  pre_upgrade_cleanup

  # Remove via opkg (runs prerm/postrm scripts)
  if command -v opkg >/dev/null 2>&1 && opkg status clashforge 2>/dev/null | grep -q "^Package:"; then
    opkg remove clashforge 2>/dev/null || warn "opkg remove returned non-zero, continuing"
  fi

  # Wipe config and data
  rm -rf /etc/metaclash /usr/share/metaclash /var/run/metaclash
  rm -f /var/log/clashforge.log

  ok "Purge complete."
}

# ── download IPK ──────────────────────────────────────────────────────────────

download_ipk() {
  TMP_IPK="/tmp/${IPK_NAME}"
  log "Downloading ${IPK_NAME}..."

  # ── 自定义 base URL（R2 / 自建 CDN）─────────────────────────────────
  if [ -n "$BASE_URL" ]; then
    _url="${BASE_URL}/releases/${TAG}/${IPK_NAME}"
    log "Using custom base URL: $BASE_URL"
    if _fetch_file "$_url" "$TMP_IPK" && [ -s "$TMP_IPK" ]; then
      ok "Downloaded to $TMP_IPK"; echo "$TMP_IPK"; return 0
    fi
    die "Download failed from base URL: $_url"
  fi

  if [ -n "$MIRROR" ]; then
    # User forced a specific mirror — try it exclusively
    _url="${MIRROR}/${GH_DOWNLOAD_URL}"
    log "Using mirror: $MIRROR"
    if _fetch_file "$_url" "$TMP_IPK" && [ -s "$TMP_IPK" ]; then
      ok "Downloaded to $TMP_IPK"; echo "$TMP_IPK"; return 0
    fi
    die "Download failed from mirror $MIRROR"
  fi

  # Try direct GitHub first, then fall through proxy mirrors
  if _fetch_file "$GH_DOWNLOAD_URL" "$TMP_IPK" && [ -s "$TMP_IPK" ]; then
    ok "Downloaded to $TMP_IPK"; echo "$TMP_IPK"; return 0
  fi
  rm -f "$TMP_IPK"

  for _proxy in $GH_PROXIES; do
    log "Trying mirror: $_proxy"
    _url="${_proxy}/${GH_DOWNLOAD_URL}"
    if _fetch_file "$_url" "$TMP_IPK" && [ -s "$TMP_IPK" ]; then
      ok "Downloaded via $_proxy"
      echo "$TMP_IPK"; return 0
    fi
    rm -f "$TMP_IPK"
  done

  die "Download failed from all sources. Try: sh install.sh --mirror https://ghproxy.com"
}

# ── install IPK via opkg ──────────────────────────────────────────────────────

install_ipk() {
  ipk_path="$1"
  log "Installing via opkg..."
  # --nodeps: mihomo is bundled inside the IPK, skip dependency resolution
  opkg install --nodeps --force-downgrade "$ipk_path" || die "opkg install failed"
  rm -f "$ipk_path"
  ok "opkg install complete."
}

# ── print result ──────────────────────────────────────────────────────────────

print_success() {
  ROUTER_IP=$(ip -4 addr show br-lan 2>/dev/null | awk '/inet /{split($2,a,"/"); print a[1]; exit}')
  [ -z "$ROUTER_IP" ] && ROUTER_IP=$(ip -4 addr show 2>/dev/null \
    | awk '/inet /{split($2,a,"/"); if(a[1]!="127.0.0.1"){print a[1];exit}}')
  [ -z "$ROUTER_IP" ] && ROUTER_IP="<router-ip>"
  echo ""
  ok "ClashForge $TAG installed successfully!"
  ok "Web UI → http://${ROUTER_IP}:7777"
  echo ""
}

# ── main ──────────────────────────────────────────────────────────────────────

if [ "$PURGE" = "1" ]; then
  # --purge: full wipe (network cleanup + opkg remove + data wipe)
  do_purge
else
  # Normal install / upgrade: clean up running state first, keep config/data.
  pre_upgrade_cleanup
fi

IPK_PATH=$(download_ipk)
install_ipk "$IPK_PATH"
print_success
