#!/bin/sh
# install.sh — ClashForge bootstrap installer for OpenWrt
#
# This script's only job is to fetch clashforgectl.sh from GitHub and delegate
# to its `upgrade` subcommand.  All real logic (arch detection, version
# resolution, mirror fallback, opkg install, auto-restart) lives in
# clashforgectl.sh so there is a single canonical implementation.
#
# ── Standard (direct GitHub) ────────────────────────────────────────────────
#   wget -qO- https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh
#
# ── 国内加速 / China mirror (via ghproxy) ───────────────────────────────────
#   wget -qO- https://ghproxy.com/https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh
#
# Options (forwarded to clashforgectl upgrade):
#   --version <tag>   Install a specific release tag (default: latest)
#   --purge           Full wipe before install
#   --mirror <url>    Force a GitHub proxy mirror
#   --base-url <url>  Custom release base URL
#   --help            Show this help

set -e

REPO="wujun4code/clashforge"
RAW_BASE="https://raw.githubusercontent.com/${REPO}/main/scripts"
GH_PROXIES="https://ghproxy.com https://mirror.ghproxy.com https://ghfast.top https://github.moeyy.xyz"
CTL_SCRIPT="/tmp/clashforgectl.sh"

log()  { echo "[clashforge] $*" >&2; }
warn() { echo "[clashforge] WARN: $*" >&2; }
die()  { echo "[clashforge] ERROR: $*" >&2; exit 1; }

# ── forward all arguments directly to clashforgectl upgrade ──────────────────
FORWARD_ARGS=""
while [ $# -gt 0 ]; do
  case "$1" in
    --version)  FORWARD_ARGS="$FORWARD_ARGS --version $2"; shift 2 ;;
    --purge)    FORWARD_ARGS="$FORWARD_ARGS --purge";      shift ;;
    --mirror)   FORWARD_ARGS="$FORWARD_ARGS --mirror $2";  shift 2 ;;
    --base-url) FORWARD_ARGS="$FORWARD_ARGS --base-url $2"; shift 2 ;;
    --help|-h)
      cat <<'EOF'
install.sh — ClashForge bootstrap installer for OpenWrt

  wget -qO- https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh

  国内加速 (China mirror):
  wget -qO- https://ghproxy.com/https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh

Options:
  --version <tag>     Install specific version, e.g. v0.1.0  (default: latest)
  --purge             Full clean install: wipe all config/data before installing
  --mirror <url>      Force a GitHub proxy prefix, e.g. --mirror https://ghproxy.com
  --base-url <url>    Custom release base URL (files at <url>/releases/<tag>/<ipk>)
  --help              Show this help

After installation, manage ClashForge with:
  clashforgectl status
  clashforgectl upgrade
  clashforgectl stop
  clashforgectl reset
  clashforgectl diag
  clashforgectl uninstall
EOF
      exit 0 ;;
    *)
      die "unknown option: $1 — run with --help for usage" ;;
  esac
done

# ── fetch helpers ─────────────────────────────────────────────────────────────
_fetch() {
  url="$1"; dest="$2"
  if command -v wget >/dev/null 2>&1; then
    wget --timeout=30 -qO "$dest" --user-agent="clashforge-installer/1.0" "$url" 2>/dev/null
  elif command -v curl >/dev/null 2>&1; then
    curl -fsSL --connect-timeout 15 --max-time 120 \
         -A "clashforge-installer/1.0" -o "$dest" "$url" 2>/dev/null
  else
    die "Neither wget nor curl is available."
  fi
}

# ── fetch clashforgectl.sh from GitHub (direct then mirrors) ─────────────────
log "Fetching clashforgectl.sh..."

_downloaded=0
_ctl_url="${RAW_BASE}/clashforgectl.sh"

if _fetch "$_ctl_url" "$CTL_SCRIPT" && [ -s "$CTL_SCRIPT" ]; then
  _downloaded=1
else
  rm -f "$CTL_SCRIPT"
  for _proxy in $GH_PROXIES; do
    log "Trying mirror: $_proxy"
    if _fetch "${_proxy}/${_ctl_url}" "$CTL_SCRIPT" && [ -s "$CTL_SCRIPT" ]; then
      _downloaded=1
      break
    fi
    rm -f "$CTL_SCRIPT"
  done
fi

[ "$_downloaded" = "1" ] || \
  die "Could not fetch clashforgectl.sh from GitHub or any mirror. Check network access."

chmod +x "$CTL_SCRIPT"
log "clashforgectl.sh downloaded. Delegating to: upgrade${FORWARD_ARGS}"
echo ""

# ── delegate to clashforgectl upgrade ────────────────────────────────────────
# shellcheck disable=SC2086
exec sh "$CTL_SCRIPT" upgrade $FORWARD_ARGS
