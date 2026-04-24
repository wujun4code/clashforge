#!/bin/sh
# install.sh — One-shot ClashForge installer / upgrader for OpenWrt
#
# Fixed URL (always installs the latest release, including pre-releases):
#   wget -qO- https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh
#
# Install a specific version:
#   wget -qO- https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh -s -- --version v0.1.0-alpha.48
#
# Full clean install (wipe old config and data first):
#   wget -qO- https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh -s -- --purge
#
# Options:
#   --version <tag>   Install a specific release tag (default: latest, including pre-releases)
#   --purge           Uninstall old version and wipe all config/data before installing
#   --help            Show this help

set -e

REPO="wujun4code/clashforge"
INSTALL_VERSION="latest"
PURGE=0

# ── helpers ───────────────────────────────────────────────────────────────────

log()  { echo "[clashforge] $*" 1>&2; }
ok()   { echo "[clashforge] $*" 1>&2; }
warn() { echo "[clashforge] WARN: $*" 1>&2; }
die()  { echo "[clashforge] ERROR: $*" 1>&2; exit 1; }

usage() {
  cat <<'EOF'
install.sh — ClashForge installer for OpenWrt

  wget -qO- https://github.com/wujun4code/clashforge/releases/latest/download/install.sh | sh

Options:
  --version <tag>   Install specific version, e.g. v1.2.0  (default: latest)
  --purge           Full clean install: uninstall old version and wipe all config/data
  --help            Show this help
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

# ── resolve version tag and pkg version ──────────────────────────────────────
# Release tag:  v1.2.3
# IPK version:  1.2.3  (no leading 'v')

# ── resolve version (called once, stored in TAG) ──────────────────────────────

if [ "$INSTALL_VERSION" != "latest" ]; then
  TAG="$INSTALL_VERSION"
else
  log "Resolving latest release from GitHub API..."
  API_URL="https://api.github.com/repos/${REPO}/releases"
  # busybox wget on OpenWrt doesn't send User-Agent; GitHub API blocks UA-less requests.
  # Try wget with explicit UA first, fall back to curl.
  TAG=$(wget -qO- --user-agent="clashforge-installer/1.0" "$API_URL" 2>/dev/null \
        | grep '"tag_name"' | head -1 \
        | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
  if [ -z "$TAG" ] && command -v curl >/dev/null 2>&1; then
    TAG=$(curl -fsSL -A "clashforge-installer/1.0" "$API_URL" 2>/dev/null \
          | grep '"tag_name"' | head -1 \
          | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
  fi
  [ -n "$TAG" ] || die "Could not resolve latest version. Specify explicitly: sh install.sh --version v0.1.0-alpha.53"
fi

PKG_VER="${TAG#v}"
IPK_NAME="clashforge_${PKG_VER}_${IPK_ARCH}.ipk"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${IPK_NAME}"

log "Version : $TAG"
log "Package : $IPK_NAME"

# ── purge old installation ────────────────────────────────────────────────────

do_purge() {
  log "Purging old installation..."

  # Stop service and kill any survivors
  /etc/init.d/clashforge stop 2>/dev/null || true
  /etc/init.d/clashforge disable 2>/dev/null || true
  for pid in $(ls /proc 2>/dev/null | grep '^[0-9]'); do
    cmdline=$(tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null) || continue
    case "$cmdline" in
      *"/usr/bin/clashforge"*|*"/usr/bin/mihomo-clashforge"*)
        kill -9 "$pid" 2>/dev/null || true ;;
    esac
  done

  # Firewall / routing cleanup
  nft delete table inet metaclash 2>/dev/null || true
  while ip rule del fwmark 0x1a3 table 100 2>/dev/null; do :; done
  ip route flush table 100 2>/dev/null || true

  # DNS cleanup
  rm -f /etc/dnsmasq.d/clashforge.conf /tmp/dnsmasq.d/clashforge.conf
  /etc/init.d/dnsmasq restart 2>/dev/null || /etc/init.d/dnsmasq reload 2>/dev/null || true

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
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$TMP_IPK" "$DOWNLOAD_URL" || die "Download failed: $DOWNLOAD_URL"
  elif command -v curl >/dev/null 2>&1; then
    curl -fsSL --retry 3 -o "$TMP_IPK" "$DOWNLOAD_URL" || die "Download failed: $DOWNLOAD_URL"
  else
    die "Neither wget nor curl is available."
  fi
  ok "Downloaded to $TMP_IPK"
  echo "$TMP_IPK"
}

# ── install IPK via opkg ──────────────────────────────────────────────────────

install_ipk() {
  ipk_path="$1"
  log "Installing via opkg..."
  # --nodeps: mihomo is bundled inside the IPK, skip dependency resolution
  opkg install --nodeps "$ipk_path" || die "opkg install failed"
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
  do_purge
fi

IPK_PATH=$(download_ipk)
install_ipk "$IPK_PATH"
print_success
