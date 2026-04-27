#!/bin/sh
# clashforgectl — ClashForge Unified Control Script
#
# ── OpenWrt local usage ──────────────────────────────────────────────────────
#   clashforgectl status
#   clashforgectl stop
#   clashforgectl reset [--start]
#   clashforgectl upgrade [--version <tag|latest>] [--mirror <url>] [--base-url <url>] [--purge]
#   clashforgectl uninstall [--keep-config]
#   clashforgectl diag [--output <path>] [--stdout] [--redact]
#
# ── Remote usage (via clashforgectl.ps1 / clashforgectl) ────────────────────
#   Invoked by the wrapper scripts; not meant to be called directly over SSH.
#
# Common options:
#   --yes          Skip confirmation prompts
#   --verbose      Extra detail
#   --dry-run      Print planned actions without mutating state
#   --help         Show help

set -e

REPO="wujun4code/clashforge"

# ── defaults ──────────────────────────────────────────────────────────────────
CLASHFORGE_VERSION="latest"
PURGE=0
MIRROR=""
BASE_URL=""
LOCAL_IPK=""
KEEP_CONFIG=0
AUTO_START=0
DIAG_OUTPUT="/tmp/cf-diag.txt"
DIAG_STDOUT=0
DIAG_REDACT=0
YES=0
VERBOSE=0
DRY_RUN=0
SUBCOMMAND=""

# ── helpers ───────────────────────────────────────────────────────────────────
log()  { printf "[clashforge] %s\n"      "$*" >&2; }
ok()   { printf "[clashforge] OK  %s\n"  "$*" >&2; }
warn() { printf "[clashforge] WARN %s\n" "$*" >&2; }
die()  { printf "[clashforge] ERROR %s\n" "$*" >&2; exit 1; }
step() { printf "\n[clashforge] ── %s\n" "$*" >&2; }

# ── usage ─────────────────────────────────────────────────────────────────────
usage() {
  cat <<'EOF'
clashforgectl — ClashForge Unified Control Script

Usage: clashforgectl <subcommand> [options]

Subcommands:
  status                     Show current ClashForge state (read-only)
  stop                       Stop all services and fully exit takeover mode
  reset [--start]            Reset to first-install state (keeps installed package)
  upgrade                    Upgrade to latest or specified version
  check                      Quick connectivity & egress IP check (lightweight)
  uninstall [--keep-config]  Completely remove ClashForge from the router
  diag                       Collect full diagnostic report

Common options:
  --yes, -y                  Skip confirmation prompts
  --verbose, -v              Extra detail
  --dry-run                  Print planned actions only (no mutations)
  --help, -h                 Show this help

Options for upgrade:
  --version <tag|latest>     Version to install (default: latest)
  --mirror <url>             Force a GitHub proxy mirror
  --base-url <url>           Custom release base URL (e.g. Cloudflare R2)
  --purge                    Full cleanup before install (wipes config)
  --local-ipk <path>         Use a pre-uploaded IPK on the router (skip download)

Options for reset:
  --start                    Start ClashForge after reset

Options for uninstall:
  --keep-config              Preserve /etc/metaclash (subscriptions, overrides)

Options for diag:
  --output <path>            Report output path (default: /tmp/cf-diag.txt)
  --stdout                   Also print report to stdout while writing to file
  --redact                   Best-effort masking of sensitive values in report
EOF
}

# ── argument parsing ──────────────────────────────────────────────────────────
[ $# -eq 0 ] && usage && exit 0

SUBCOMMAND="$1"; shift

while [ $# -gt 0 ]; do
  case "$1" in
    --version)     [ -n "${2:-}" ] || die "--version requires a value"; CLASHFORGE_VERSION="$2"; shift 2 ;;
    --mirror)      [ -n "${2:-}" ] || die "--mirror requires a value";  MIRROR="$2"; shift 2 ;;
    --base-url)    [ -n "${2:-}" ] || die "--base-url requires a value"; BASE_URL="${2%/}"; shift 2 ;;
    --output)      [ -n "${2:-}" ] || die "--output requires a value";  DIAG_OUTPUT="$2"; shift 2 ;;
    --purge)       PURGE=1; shift ;;
    --local-ipk)   [ -n "${2:-}" ] || die "--local-ipk requires a path"; LOCAL_IPK="$2"; shift 2 ;;
    --keep-config) KEEP_CONFIG=1; shift ;;
    --start)       AUTO_START=1; shift ;;
    --stdout)      DIAG_STDOUT=1; shift ;;
    --redact)      DIAG_REDACT=1; shift ;;
    --yes|-y)      YES=1; shift ;;
    --verbose|-v)  VERBOSE=1; shift ;;
    --dry-run)     DRY_RUN=1; shift ;;
    --help|-h)     usage; exit 0 ;;
    *) die "Unknown option: $1  (run with --help for usage)" ;;
  esac
done

# ── dry-run guard ─────────────────────────────────────────────────────────────
run_cmd() {
  if [ "$DRY_RUN" = "1" ]; then
    printf "[dry-run] %s\n" "$*" >&2
  else
    eval "$@"
  fi
}

# ── architecture detection ────────────────────────────────────────────────────
# Maps uname -m output to OpenWrt IPK architecture labels.
detect_ipk_arch() {
  _machine=$(uname -m 2>/dev/null || echo "unknown")
  case "$_machine" in
    x86_64|amd64)
      echo "x86_64" ;;
    aarch64|arm64)
      # Cortex-A53 CPU part 0xd03 is extremely common: RPi3, MT7981, MT7986...
      _cpu_part=$(grep -m1 "CPU part" /proc/cpuinfo 2>/dev/null | awk '{print tolower($NF)}')
      if [ "$_cpu_part" = "0xd03" ]; then
        echo "aarch64_cortex-a53"
      else
        echo "aarch64_generic"
      fi ;;
    armv7*|armhf)
      die "ARMv7 is not supported. Open an issue at https://github.com/${REPO}/issues" ;;
    *)
      die "Unsupported architecture: $_machine (supported: x86_64, aarch64)" ;;
  esac
}

# ── mirror / download helpers ─────────────────────────────────────────────────
GH_PROXIES="https://ghproxy.com https://mirror.ghproxy.com https://ghfast.top https://github.moeyy.xyz"

_fetch_text() {
  _url="$1"
  if command -v wget >/dev/null 2>&1; then
    wget --timeout=10 -qO- --user-agent="clashforgectl/1.0" "$_url" 2>/dev/null
  elif command -v curl >/dev/null 2>&1; then
    curl -fsSL --connect-timeout 10 --max-time 15 -A "clashforgectl/1.0" "$_url" 2>/dev/null
  fi
}

_fetch_file() {
  _url="$1"; _dest="$2"
  if command -v wget >/dev/null 2>&1; then
    wget --timeout=30 -qO "$_dest" --user-agent="clashforgectl/1.0" "$_url" 2>/dev/null
  elif command -v curl >/dev/null 2>&1; then
    curl -fsSL --connect-timeout 15 --max-time 300 -A "clashforgectl/1.0" -o "$_dest" "$_url" 2>/dev/null
  else
    return 1
  fi
}

_resolve_tag_from_json() {
  awk -F'"tag_name":"' 'NF>1{split($2,a,"\""); print a[1]; exit}'
}

resolve_version() {
  if [ "$CLASHFORGE_VERSION" != "latest" ]; then
    TAG="$CLASHFORGE_VERSION"
    return 0
  fi

  log "Resolving latest release from GitHub API..."
  _api_path="repos/${REPO}/releases?per_page=1"
  TAG=""

  if [ -n "$MIRROR" ]; then
    TAG=$(_fetch_text "${MIRROR}/https://api.github.com/${_api_path}" | _resolve_tag_from_json)
  else
    TAG=$(_fetch_text "https://api.github.com/${_api_path}" | _resolve_tag_from_json)
    if [ -z "$TAG" ]; then
      for _proxy in $GH_PROXIES; do
        TAG=$(_fetch_text "${_proxy}/https://api.github.com/${_api_path}" | _resolve_tag_from_json)
        if [ -n "$TAG" ]; then
          log "Version resolved via mirror: $_proxy"
          break
        fi
      done
    fi
  fi

  [ -n "$TAG" ] || die "Could not resolve latest version. Use: clashforgectl upgrade --version v0.1.0"
}

download_ipk() {
  resolve_version
  IPK_ARCH=$(detect_ipk_arch)
  PKG_VER="${TAG#v}"
  IPK_NAME="clashforge_${PKG_VER}_${IPK_ARCH}.ipk"
  GH_URL="https://github.com/${REPO}/releases/download/${TAG}/${IPK_NAME}"
  TMP_IPK="/tmp/${IPK_NAME}"

  log "Version : $TAG"
  log "Package : $IPK_NAME"

  # Custom base URL (Cloudflare R2, self-hosted CDN)
  if [ -n "$BASE_URL" ]; then
    _url="${BASE_URL}/releases/${TAG}/${IPK_NAME}"
    log "Downloading from custom base URL: $BASE_URL"
    _fetch_file "$_url" "$TMP_IPK" && [ -s "$TMP_IPK" ] \
      || die "Download failed from base URL: $_url"
    ok "Downloaded to $TMP_IPK"
    return 0
  fi

  # Forced mirror
  if [ -n "$MIRROR" ]; then
    _url="${MIRROR}/${GH_URL}"
    log "Downloading via mirror: $MIRROR"
    _fetch_file "$_url" "$TMP_IPK" && [ -s "$TMP_IPK" ] \
      || die "Download failed from mirror: $MIRROR"
    ok "Downloaded to $TMP_IPK"
    return 0
  fi

  # Direct GitHub, then auto-fallback through proxies
  if _fetch_file "$GH_URL" "$TMP_IPK" && [ -s "$TMP_IPK" ]; then
    ok "Downloaded from GitHub"
    return 0
  fi
  rm -f "$TMP_IPK"

  for _proxy in $GH_PROXIES; do
    log "Trying mirror: $_proxy"
    if _fetch_file "${_proxy}/${GH_URL}" "$TMP_IPK" && [ -s "$TMP_IPK" ]; then
      ok "Downloaded via $_proxy"
      return 0
    fi
    rm -f "$TMP_IPK"
  done

  die "Download failed from all sources. Try: clashforgectl upgrade --mirror https://ghproxy.com"
}

# ══════════════════════════════════════════════════════════════════════════════
# restore_system_state — canonical network/service restore
#
# ORDER is deliberate (see design doc):
#   1. DNS restore FIRST  — dnsmasq back on :53 before redirect tables removed
#   2. nftables metaclash — remove tproxy + dns_redirect chains
#   3. nftables dnsmasq   — remove HIJACK table injected by dnsmasq on restart
#   4. Policy routing     — flush fwmark rules and route tables
#   5. Kill processes LAST — avoids racing the service's own SIGTERM handler
# ══════════════════════════════════════════════════════════════════════════════
restore_system_state() {
  # Quick-exit if there is nothing to restore
  _cf_running=0
  pgrep -f "/usr/bin/clashforge" >/dev/null 2>&1        && _cf_running=1
  pgrep -f "/usr/bin/mihomo-clashforge" >/dev/null 2>&1 && _cf_running=1
  nft list table inet metaclash >/dev/null 2>&1          && _cf_running=1

  if [ "$_cf_running" = "0" ]; then
    log "Nothing running — skipping system restore"
    return 0
  fi

  log "Restoring system state..."

  # ── 1. Restore dnsmasq DNS config ──────────────────────────────────────────
  step "[1/5] Restoring dnsmasq via UCI..."
  if command -v uci >/dev/null 2>&1; then
    uci -q delete dhcp.@dnsmasq[0].port     || true   # replace mode: port=0 override
    uci -q delete dhcp.@dnsmasq[0].server   || true   # upstream mode: server= override
    uci -q delete dhcp.@dnsmasq[0].noresolv || true   # upstream mode: noresolv= flag
    uci commit dhcp 2>/dev/null             || true
    ok "UCI dhcp.@dnsmasq[0] overrides removed"
  fi
  rm -f /etc/dnsmasq.d/clashforge.conf     2>/dev/null || true
  rm -f /var/etc/dnsmasq.d/clashforge.conf 2>/dev/null || true
  /etc/init.d/dnsmasq restart 2>/dev/null  || true
  ok "dnsmasq restarted (port 53 restored)"

  # ── 2. Remove nftables table inet metaclash ────────────────────────────────
  step "[2/5] Removing nftables table inet metaclash..."
  if nft list table inet metaclash >/dev/null 2>&1; then
    if nft delete table inet metaclash 2>/dev/null; then
      ok "table inet metaclash deleted"
    else
      warn "Whole-table delete failed — trying chain-by-chain cleanup..."
      for _chain in dns_redirect tproxy_prerouting; do
        nft flush  chain inet metaclash "$_chain" 2>/dev/null || true
        nft delete chain inet metaclash "$_chain" 2>/dev/null || true
      done
      nft delete table inet metaclash 2>/dev/null \
        && ok "table inet metaclash deleted (second attempt)" \
        || warn "table inet metaclash still present — manual cleanup may be needed"
    fi
  else
    ok "table inet metaclash not present, skipping"
  fi

  # ── 3. Remove nftables table inet dnsmasq (HIJACK) ────────────────────────
  # dnsmasq re-injects this table on every restart (priority dstnat-5).
  # While port=0, the :53 redirect hits nothing and breaks DNS.
  step "[3/5] Removing nftables table inet dnsmasq (HIJACK)..."
  if nft list table inet dnsmasq >/dev/null 2>&1; then
    nft delete table inet dnsmasq 2>/dev/null \
      && ok "table inet dnsmasq (HIJACK) deleted" \
      || warn "Failed to delete table inet dnsmasq"
  else
    ok "table inet dnsmasq not present, skipping"
  fi

  # ── 4. Clean up policy routing rules and route tables ─────────────────────
  step "[4/5] Cleaning policy routing (fwmark 0x1a3/0x1a4)..."

  _removed=0
  while ip rule del fwmark 0x1a3 table 100 2>/dev/null; do _removed=$((_removed+1)); done
  [ "$_removed" -gt 0 ] && ok "Removed $_removed IPv4 ip rule(s) fwmark 0x1a3" \
                         || ok "No IPv4 ip rules fwmark 0x1a3 to remove"
  ip route flush table 100 2>/dev/null || true

  _removed6=0
  while ip -6 rule del fwmark 0x1a3 table 100 2>/dev/null; do _removed6=$((_removed6+1)); done
  [ "$_removed6" -gt 0 ] && ok "Removed $_removed6 IPv6 ip rule(s) fwmark 0x1a3" \
                           || ok "No IPv6 ip rules fwmark 0x1a3 to remove"
  ip -6 route flush table 100 2>/dev/null || true

  _removed_out=0
  while ip rule del fwmark 0x1a4 table 101 2>/dev/null; do _removed_out=$((_removed_out+1)); done
  [ "$_removed_out" -gt 0 ] && ok "Removed $_removed_out output tproxy rule(s) fwmark 0x1a4" \
                             || ok "No output tproxy rules fwmark 0x1a4 to remove"
  ip route flush table 101 2>/dev/null || true

  # ── 5. Stop service and kill remaining processes ───────────────────────────
  step "[5/5] Stopping clashforge service and processes..."
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
      ok "$_name stopped"
    fi
  done

  ok "System state restored"
}

# ── final verification summary ────────────────────────────────────────────────
print_state_summary() {
  echo "" >&2
  log "── Verification Summary ──"

  # Processes
  _procs=$(pgrep -f "clashforge\|mihomo-clashforge" 2>/dev/null || true)
  if [ -n "$_procs" ]; then
    warn "Remaining processes: $_procs"
  else
    ok "Processes: none running"
  fi

  # nftables
  if nft list table inet metaclash >/dev/null 2>&1; then
    warn "nftables table inet metaclash: still present"
  else
    ok "nftables table inet metaclash: cleared"
  fi

  # Policy routing
  if ip rule show 2>/dev/null | grep -q "fwmark 0x1a3"; then
    warn "ip rule fwmark 0x1a3: still present"
  else
    ok "ip rule fwmark 0x1a3: cleared"
  fi

  # DNS port 53
  if netstat -lnup 2>/dev/null | grep -q ':53 ' || \
     ss    -lnup 2>/dev/null | grep -q ':53 '; then
    ok "DNS port 53: listening"
  else
    warn "DNS port 53: not detected (dnsmasq may still be starting)"
  fi
}

# ══════════════════════════════════════════════════════════════════════════════
# SUBCOMMAND: status
# ══════════════════════════════════════════════════════════════════════════════
cmd_status() {
  log "ClashForge Status"
  echo "" >&2

  # Installed package
  if command -v opkg >/dev/null 2>&1 && opkg status clashforge 2>/dev/null | grep -q "^Package:"; then
    _ver=$(opkg status clashforge 2>/dev/null | awk '/^Version:/{print $2}')
    ok "Package: installed ($_ver)"
  else
    warn "Package: not installed"
  fi

  # init.d
  if [ -f /etc/init.d/clashforge ]; then
    _enabled=$(/etc/init.d/clashforge enabled 2>/dev/null && echo "enabled" || echo "disabled")
    ok "Init script: present ($_enabled)"
  else
    warn "Init script: /etc/init.d/clashforge not found"
  fi

  # Processes
  for _name in clashforge mihomo-clashforge; do
    _pids=$(pgrep -f "/usr/bin/$_name" 2>/dev/null || true)
    if [ -n "$_pids" ]; then
      ok "Process $_name: running (PID $_pids)"
    else
      warn "Process $_name: not running"
    fi
  done

  # nftables takeover state
  if nft list table inet metaclash >/dev/null 2>&1; then
    ok "nftables table inet metaclash: present (takeover active)"
  else
    warn "nftables table inet metaclash: not present (takeover inactive)"
  fi

  if nft list table inet dnsmasq >/dev/null 2>&1; then
    warn "nftables table inet dnsmasq: present (HIJACK table visible)"
  else
    ok "nftables table inet dnsmasq: not present"
  fi

  # Policy routing
  if ip rule show 2>/dev/null | grep -q "fwmark 0x1a3"; then
    ok "Policy routing fwmark 0x1a3: present"
  else
    warn "Policy routing fwmark 0x1a3: not present"
  fi

  # DNS port 53
  if netstat -lnup 2>/dev/null | grep -q ':53 ' || \
     ss    -lnup 2>/dev/null | grep -q ':53 '; then
    ok "DNS port 53: listening"
  else
    warn "DNS port 53: not detected"
  fi

  # Web UI port 7777
  if netstat -lntp 2>/dev/null | grep -q ':7777 ' || \
     ss    -lntp 2>/dev/null | grep -q ':7777 '; then
    ok "Web UI port 7777: listening"
  else
    warn "Web UI port 7777: not listening"
  fi

  echo "" >&2
}

# ══════════════════════════════════════════════════════════════════════════════
# SUBCOMMAND: stop
# ══════════════════════════════════════════════════════════════════════════════
cmd_stop() {
  log "Stopping ClashForge and exiting takeover mode..."
  restore_system_state
  print_state_summary
  ok "Done."
}

# ══════════════════════════════════════════════════════════════════════════════
# SUBCOMMAND: reset
# ══════════════════════════════════════════════════════════════════════════════
cmd_reset() {
  log "Resetting ClashForge to first-install state (package kept, all data wiped)..."

  if [ "$YES" != "1" ]; then
    printf "[clashforge] This will wipe all subscriptions, rules, overrides, and caches. Continue? [y/N] "
    read -r _reply
    case "$_reply" in
      y|Y|yes|YES) ;;
      *) log "Aborted."; exit 0 ;;
    esac
  fi

  # 1. Stop and restore network state first
  restore_system_state

  # 2. Clear all runtime state
  step "Clearing runtime state (/var/run/metaclash)..."
  rm -rf /var/run/metaclash && ok "Cleared /var/run/metaclash" || true

  # 3. Clear all user config and data (subscriptions, overrides, rules, caches)
  step "Clearing config and data (/etc/metaclash)..."
  rm -rf /etc/metaclash && ok "Cleared /etc/metaclash" || true

  # 4. Clear geodata and bundled support files
  step "Clearing geodata (/usr/share/metaclash)..."
  rm -rf /usr/share/metaclash && ok "Cleared /usr/share/metaclash" || true

  # 5. Clear logs
  step "Clearing logs..."
  rm -f /var/log/clashforge.log  && ok "Cleared /var/log/clashforge.log"  || true
  rm -f /var/log/metaclash.log   && ok "Cleared /var/log/metaclash.log"   || true
  rm -f /tmp/clashforge.log      && ok "Cleared /tmp/clashforge.log"      || true
  rm -f /tmp/metaclash.log       && ok "Cleared /tmp/metaclash.log"       || true

  # 6. Recreate required directories (package may expect them to exist at startup)
  step "Recreating required directories..."
  mkdir -p /etc/metaclash /var/run/metaclash /usr/share/metaclash
  ok "Directories recreated"

  if [ "$AUTO_START" = "1" ]; then
    step "Starting ClashForge..."
    /etc/init.d/clashforge start 2>/dev/null \
      && ok "ClashForge started. Open http://$(ip -4 addr show br-lan 2>/dev/null | awk '/inet /{split($2,a,"/"); print a[1]; exit}'):7777" \
      || warn "Start failed — check /etc/init.d/clashforge status"
  else
    echo "" >&2
    ok "Reset complete. Run '/etc/init.d/clashforge start' to start, or use --start flag."
  fi
}

# ══════════════════════════════════════════════════════════════════════════════
# SUBCOMMAND: upgrade
# ══════════════════════════════════════════════════════════════════════════════
cmd_upgrade() {
  log "Upgrading ClashForge..."

  # ── Snapshot running state BEFORE we stop anything ─────────────────────────
  # If clashforge was running (processes alive or takeover tables present or
  # init.d service is enabled), we must restart it after the new IPK is installed
  # so the user does not lose network connectivity.
  _was_running=0
  pgrep -f "/usr/bin/clashforge"        >/dev/null 2>&1 && _was_running=1
  pgrep -f "/usr/bin/mihomo-clashforge"  >/dev/null 2>&1 && _was_running=1
  nft list table inet metaclash          >/dev/null 2>&1 && _was_running=1
  # Also treat init.d-enabled-but-not-yet-started as "should start after upgrade"
  if [ "$_was_running" = "0" ] && [ -f /etc/init.d/clashforge ]; then
    /etc/init.d/clashforge enabled 2>/dev/null && _was_running=1 || true
  fi

  if [ "$PURGE" = "1" ]; then
    log "Purge mode: wiping existing installation and data before upgrade..."
    restore_system_state
    if command -v opkg >/dev/null 2>&1 && opkg status clashforge 2>/dev/null | grep -q "^Package:"; then
      opkg remove clashforge 2>/dev/null \
        || warn "opkg remove returned non-zero, continuing"
    fi
    rm -rf /etc/metaclash /usr/share/metaclash /var/run/metaclash
    rm -f  /var/log/clashforge.log
    ok "Purge complete"
    # After a full purge there is no configuration to start from; skip auto-restart.
    _was_running=0
  else
    # Normal upgrade: stop takeover, preserve /etc/metaclash config
    restore_system_state
  fi

  if [ -n "$LOCAL_IPK" ]; then
    log "Using pre-staged local IPK: $LOCAL_IPK"
    [ -f "$LOCAL_IPK" ] || die "--local-ipk path not found on router: $LOCAL_IPK"
    TMP_IPK="$LOCAL_IPK"
    _cleanup_ipk=0
  else
    download_ipk
    _cleanup_ipk=1
  fi

  log "Installing via opkg (--nodeps --force-downgrade)..."
  opkg install --nodeps --force-downgrade "$TMP_IPK" \
    || die "opkg install failed"
  [ "${_cleanup_ipk:-1}" = "1" ] && rm -f "$TMP_IPK" || true
  ok "opkg install complete"

  # ── Re-enable and start the service if it was running before upgrade ────────
  if [ "$_was_running" = "1" ]; then
    log "Service was running before upgrade — restarting with new version..."
    /etc/init.d/clashforge enable 2>/dev/null || true
    if /etc/init.d/clashforge start 2>/dev/null; then
      ok "clashforge service restarted successfully"
    else
      warn "Service start returned non-zero — check: /etc/init.d/clashforge status"
    fi
  else
    log "Service was not running before upgrade — skipping auto-start"
    log "To start: /etc/init.d/clashforge enable && /etc/init.d/clashforge start"
  fi

  _router_ip=$(ip -4 addr show br-lan 2>/dev/null | awk '/inet /{split($2,a,"/"); print a[1]; exit}')
  [ -z "$_router_ip" ] && _router_ip=$(ip -4 addr show 2>/dev/null \
    | awk '/inet /{split($2,a,"/"); if(a[1]!="127.0.0.1"){print a[1];exit}}')
  [ -z "$_router_ip" ] && _router_ip="<router-ip>"

  echo "" >&2
  ok "ClashForge $TAG installed!"
  ok "Web UI → http://${_router_ip}:7777"
  echo "" >&2
}

# ══════════════════════════════════════════════════════════════════════════════
# SUBCOMMAND: uninstall
# ══════════════════════════════════════════════════════════════════════════════
cmd_uninstall() {
  log "Uninstalling ClashForge..."

  if [ "$YES" != "1" ]; then
    printf "[clashforge] This will completely remove ClashForge and all its data. Continue? [y/N] "
    read -r _reply
    case "$_reply" in
      y|Y|yes|YES) ;;
      *) log "Aborted."; exit 0 ;;
    esac
  fi

  # 1. Stop all services and restore network state
  restore_system_state

  # 2. Disable init script to prevent auto-start on reboot
  /etc/init.d/clashforge disable 2>/dev/null || true

  # 3. Remove opkg package (runs prerm/postrm scripts)
  step "Removing opkg package..."
  if command -v opkg >/dev/null 2>&1 && opkg status clashforge 2>/dev/null | grep -q "^Package:"; then
    opkg remove clashforge 2>/dev/null \
      && ok "opkg package removed" \
      || warn "opkg remove returned non-zero — check: opkg status clashforge"
  else
    ok "opkg package not installed, skipping"
  fi

  # 4. Wipe all data directories
  step "Removing runtime and geodata..."
  rm -rf /usr/share/metaclash && ok "Removed /usr/share/metaclash" || true
  rm -rf /var/run/metaclash   && ok "Removed /var/run/metaclash"   || true
  rm -f  /var/log/clashforge.log && ok "Removed clashforge.log"    || true

  if [ "$KEEP_CONFIG" = "1" ]; then
    ok "/etc/metaclash preserved (--keep-config)"
  else
    rm -rf /etc/metaclash && ok "Removed /etc/metaclash" || true
  fi

  # 5. Final verification
  print_state_summary
  echo "" >&2
  ok "ClashForge uninstalled. Router restored to pre-install state."
}

# ══════════════════════════════════════════════════════════════════════════════
# SUBCOMMAND: check
#
# Lightweight connectivity & IP probe — prints results directly to stdout.
# Mirrors the "IP检查 + 连通性检测" panels shown in the ClashForge web UI.
# ══════════════════════════════════════════════════════════════════════════════
cmd_check() {
  _MIXED_PORT="7893"
  _API_BASE="http://127.0.0.1:7777/api/v1"

  # ── tiny HTTP helper (wget or curl) ────────────────────────────────────────
  _get() {
    url="$1"; proxy="$2"; timeout="${3:-8}"
    if [ -n "$proxy" ]; then
      if command -v curl >/dev/null 2>&1; then
        curl -fsSL --max-time "$timeout" --proxy "$proxy" \
             -A "clashforgectl-check/1.0" "$url" 2>/dev/null
      else
        http_proxy="$proxy" wget -qO- --timeout="$timeout" \
             --user-agent="clashforgectl-check/1.0" "$url" 2>/dev/null
      fi
    else
      if command -v curl >/dev/null 2>&1; then
        curl -fsSL --max-time "$timeout" \
             -A "clashforgectl-check/1.0" "$url" 2>/dev/null
      else
        wget -qO- --timeout="$timeout" \
             --user-agent="clashforgectl-check/1.0" "$url" 2>/dev/null
      fi
    fi
  }

  # ── check if ClashForge API + mixed proxy are up ───────────────────────────
  _cf_api_ok=0
  _cf_proxy_ok=0
  # Use -sSL (no -f) so 401/403 from auth middleware still counts as "reachable"
  if command -v curl >/dev/null 2>&1; then
    _api_resp="$(curl -sSL --max-time 4 -A "clashforgectl-check/1.0" \
                      "$_API_BASE/health/check" 2>/dev/null || true)"
  else
    _api_resp="$(wget -qO- --timeout=4 --user-agent="clashforgectl-check/1.0" \
                      "$_API_BASE/health/check" 2>/dev/null || true)"
  fi
  [ -n "$_api_resp" ] && _cf_api_ok=1
  # Test mixed proxy: netstat is always present on OpenWrt/BusyBox and shows all
  # interfaces. BusyBox nc has no -z flag, and mihomo binds :::PORT (IPv6 dual-stack)
  # so connecting to 127.0.0.1 fails even when the port is open.
  netstat -tlnp 2>/dev/null | grep -q ":${_MIXED_PORT} " && _cf_proxy_ok=1 || true

  _PROXY=""
  [ "$_cf_proxy_ok" = "1" ] && _PROXY="http://127.0.0.1:$_MIXED_PORT"

  printf "\n"
  printf "═══════════════════════════════════════════════════════\n"
  printf "  ClashForge Connectivity Check\n"
  printf "═══════════════════════════════════════════════════════\n"

  # ── Service status ─────────────────────────────────────────────────────────
  printf "\n[Service]\n"
  if [ "$_cf_api_ok" = "1" ]; then
    printf "  ClashForge API  : ✓ running (http://127.0.0.1:7777)\n"
  else
    printf "  ClashForge API  : ✗ not reachable\n"
  fi
  if [ "$_cf_proxy_ok" = "1" ]; then
    printf "  Mixed proxy     : ✓ port %s active\n" "$_MIXED_PORT"
  else
    printf "  Mixed proxy     : ✗ port %s not active\n" "$_MIXED_PORT"
  fi

  # ── IP check providers ─────────────────────────────────────────────────────
  printf "\n[Egress IP]\n"

  _check_ip() {
    label="$1"; url="$2"; jq_ip="$3"; jq_loc="$4"
    _body="$(_get "$url" "$_PROXY" 8)"
    if [ -z "$_body" ]; then
      printf "  %-10s : ✗ (request failed)\n" "$label"
      return
    fi
    # Poor-man's JSON field extraction (no jq required on router)
    _ip="$(printf '%s' "$_body" | grep -o "\"${jq_ip}\":\"[^\"]*\"" | head -1 | cut -d'"' -f4)"
    if [ -n "$jq_loc" ]; then
      _loc="$(printf '%s' "$_body" | grep -o "\"${jq_loc}\":\"[^\"]*\"" | head -1 | cut -d'"' -f4)"
    else
      _loc=""
    fi
    if [ -n "$_ip" ]; then
      if [ -n "$_loc" ]; then
        printf "  %-10s : ✓ %-42s  %s\n" "$label" "$_ip" "$_loc"
      else
        printf "  %-10s : ✓ %s\n" "$label" "$_ip"
      fi
    else
      printf "  %-10s : ✗ (unexpected response)\n" "$label"
    fi
  }

  _check_ip "IP.SB"  "https://api.ip.sb/geoip"               "ip"      "country"
  _check_ip "ipify"  "https://api.ipify.org?format=json"      "ip"      ""
  _check_ip "ip-api" "http://ip-api.com/json?fields=query,isp,country" "query" "country"

  # ── Access checks ──────────────────────────────────────────────────────────
  printf "\n[Access Check]  (via ClashForge mixed proxy port %s)\n" "$_MIXED_PORT"

  _check_url() {
    label="$1"; url="$2"
    _ts_start="$(date +%s)"
    if command -v curl >/dev/null 2>&1; then
      _code="$(curl -fsSL --max-time 12 --proxy "$_PROXY" \
                    -A "clashforgectl-check/1.0" \
                    -o /dev/null -w "%{http_code}" "$url" 2>/dev/null || echo "000")"
    else
      _code="$(http_proxy="$_PROXY" wget -qO/dev/null --timeout=12 \
                    --server-response "$url" 2>&1 | awk '/HTTP\//{code=$2} END{print code}' || echo "000")"
    fi
    _ts_end="$(date +%s)"
    _ms=$(( (_ts_end - _ts_start) * 1000 ))
    if printf '%s' "$_code" | grep -qE '^[23][0-9][0-9]$'; then
      printf "  %-20s : ✓ HTTP %s  (%s ms)\n" "$label" "$_code" "$_ms"
    elif [ "$_code" = "000" ]; then
      if [ -z "$_PROXY" ]; then
        printf "  %-20s : ✗ (proxy not running)\n" "$label"
      else
        printf "  %-20s : ✗ (timeout or blocked)\n" "$label"
      fi
    else
      printf "  %-20s : ✗ HTTP %s\n" "$label" "$_code"
    fi
  }

  _check_url "baidu.com"         "https://www.baidu.com"
  _check_url "github.com"        "https://github.com"
  _check_url "youtube.com"       "https://www.youtube.com"
  _check_url "google.com"        "https://www.google.com"

  # ── nftables takeover ──────────────────────────────────────────────────────
  printf "\n[Takeover]\n"
  if nft list table inet metaclash >/dev/null 2>&1; then
    printf "  nft metaclash   : ✓ active\n"
  else
    printf "  nft metaclash   : ✗ not loaded\n"
  fi
  if ip rule list 2>/dev/null | grep -q "0x1a3"; then
    printf "  policy routing  : ✓ rule 0x1a3 present\n"
  else
    printf "  policy routing  : ✗ rule 0x1a3 absent\n"
  fi

  printf "\n"
}

# ══════════════════════════════════════════════════════════════════════════════
# SUBCOMMAND: diag
#
# Full diagnostic report covering all 23 sections.
# Originally standalone as openwrt/files/usr/bin/clashforge-diag — now
# integrated here as the single canonical diagnostic entry point.
# ══════════════════════════════════════════════════════════════════════════════
cmd_diag() {
  _API="http://127.0.0.1:7777/api/v1"
  _SEP="════════════════════════════════════════"

  _diag_section() { printf "\n%s\n%s\n" "$_SEP" "$*"; }
  _diag_run()     { printf "\n▶ %s\n" "$*"; eval "$*" 2>&1 || true; }

  # _do_diag writes everything to stdout; the caller decides where to route it.
  _do_diag() {
    set +e   # Individual probe failures must not abort the report

    _diag_section "【0】Collection time / System info"
    date
    uname -a
    cat /etc/openwrt_release 2>/dev/null || true

    _diag_section "【1】Process state"
    _diag_run "ps w 2>/dev/null | grep -E 'mihomo|clashforge|dnsmasq|PID' | grep -v grep"

    _mihomo_pid=$(ps 2>/dev/null | grep mihomo | grep -v grep | awk '{print $1}' | head -1)
    if [ -n "$_mihomo_pid" ]; then
      echo "mihomo PID: $_mihomo_pid"
      cat /proc/"$_mihomo_pid"/status 2>/dev/null || true
      printf "cmdline: "
      cat /proc/"$_mihomo_pid"/cmdline 2>/dev/null | tr '\0' ' '
      echo ""
      cat /proc/"$_mihomo_pid"/stat 2>/dev/null | awk '{print "starttime(ticks):", $22}'
    else
      echo "⚠ mihomo process not found"
    fi

    _diag_section "【2】Port bindings — /proc/net/tcp (LISTEN state=0A)"
    echo "Common ports hex: 7777=1E61  7874=1EC2  7893=1EE5  53=0035"
    awk 'NR>1 && $4=="0A" {print}' /proc/net/tcp  2>/dev/null | head -40
    awk 'NR>1 && $4=="0A" {print}' /proc/net/tcp6 2>/dev/null | head -40

    _diag_section "【2b】Port bindings — /proc/net/udp (all bound)"
    awk 'NR>1 {print}' /proc/net/udp  2>/dev/null | head -40
    awk 'NR>1 {print}' /proc/net/udp6 2>/dev/null | head -40

    _diag_section "【2c】ss / netstat snapshot"
    ss -tlnpu 2>/dev/null || netstat -tlnpu 2>/dev/null || echo "ss/netstat not available"

    _diag_section "【3】ClashForge API — status (127.0.0.1:7777)"
    _diag_run "curl -sf --max-time 5 '${_API}/status'"
    echo ""

    _diag_section "【3b】ClashForge API — running config ports"
    _diag_run "curl -sf --max-time 5 '${_API}/config'"
    echo ""

    _diag_section "【3c】ClashForge API — port check (/setup/port-check)"
    _diag_run "curl -sf --max-time 10 '${_API}/setup/port-check'"
    echo ""

    _diag_section "【3d】ClashForge API — core version"
    _diag_run "curl -sf --max-time 5 '${_API}/core/version'"
    echo ""

    _diag_section "【4】ClashForge config files (config.toml)"
    for _p in /etc/metaclash/config.toml /etc/clashforge/config.toml; do
      if [ -f "$_p" ]; then
        echo "=== $_p ==="
        cat "$_p"
        echo ""
      fi
    done

    _diag_section "【5】Mihomo generated config (config.yaml)"
    for _p in /etc/metaclash/config.yaml \
               /etc/clashforge/config.yaml \
               /tmp/metaclash/config.yaml \
               /var/run/metaclash/config.yaml; do
      if [ -f "$_p" ]; then
        echo "=== $_p ==="
        cat "$_p"
        echo ""
      fi
    done
    echo "--- via API ---"
    _diag_run "curl -sf --max-time 5 '${_API}/config/mihomo'"
    echo ""

    _diag_section "【6】DNS state"
    _diag_run "cat /etc/resolv.conf"

    echo "--- dnsmasq main config (non-comment lines) ---"
    grep -vE "^#|^$" /etc/dnsmasq.conf 2>/dev/null || true

    echo "--- dnsmasq.d/ ---"
    ls -la /etc/dnsmasq.d/ 2>/dev/null || echo "directory does not exist"
    for _f in /etc/dnsmasq.d/*; do
      [ -f "$_f" ] && echo "=== $_f ===" && cat "$_f" && echo "" || true
    done

    echo "--- nslookup google.com (system DNS chain) ---"
    nslookup google.com 2>&1 || true

    echo "--- nslookup google.com via 127.0.0.1 (mihomo DNS) ---"
    nslookup google.com 127.0.0.1 2>&1 || true

    _diag_section "【7】nftables rules"
    _diag_run "nft list ruleset"
    _diag_run "nft list table inet metaclash 2>&1"

    _diag_section "【8】IP policy routing"
    _diag_run "ip rule list"
    _diag_run "ip route show table 100 2>/dev/null || ip route show table all | grep 'table 100'"
    _diag_run "ip route show"

    _diag_section "【9】Network interfaces"
    _diag_run "ip addr show"
    _diag_run "ip link show"

    _diag_section "【10】System resources"
    _diag_run "free"
    _diag_run "df -h"
    _diag_run "uptime"

    _diag_section "【11】OpenWrt service registration"
    ls -la /etc/init.d/ 2>/dev/null | grep -iE "clash|mihomo|metaclash" || echo "no related init.d entries"
    for _svc in clashforge metaclash mihomo; do
      if [ -f "/etc/init.d/$_svc" ]; then
        echo "=== /etc/init.d/$_svc ==="
        /etc/init.d/"$_svc" enabled 2>/dev/null && echo "enabled" || echo "disabled/unknown"
      fi
    done

    _diag_section "【12】System logs (last 150 lines)"
    logread 2>/dev/null | tail -150 || \
      cat /var/log/messages 2>/dev/null | tail -150 || \
      dmesg | tail -50 || \
      echo "Cannot retrieve system logs"

    _diag_section "【13】ClashForge log files"
    for _p in /var/log/clashforge.log /var/log/metaclash.log \
               /tmp/clashforge.log    /tmp/metaclash.log; do
      if [ -f "$_p" ]; then
        echo "=== $_p (last 100 lines) ==="
        tail -100 "$_p"
        echo ""
      fi
    done

    _diag_section "【14】Mihomo binary and version"
    for _p in /usr/bin/mihomo /usr/bin/mihomo-clashforge \
               /usr/local/bin/mihomo /opt/mihomo/mihomo; do
      if [ -f "$_p" ]; then
        echo "=== $_p ==="
        ls -lh "$_p"
        "$_p" -v 2>&1 | head -3 || true
      fi
    done

    _diag_section "【15】ClashForge binary and version"
    for _p in /usr/bin/clashforge /usr/local/bin/clashforge; do
      if [ -f "$_p" ]; then
        echo "=== $_p ==="
        ls -lh "$_p"
        "$_p" version 2>&1 | head -3 || true
      fi
    done

    _diag_section "【16】/etc/metaclash directory structure"
    find /etc/metaclash /etc/clashforge 2>/dev/null -maxdepth 4 | head -60 || true
    ls -laR /etc/metaclash 2>/dev/null | head -80 || true

    _diag_section "【17】Connectivity probes"
    echo "--- Direct ipify (no proxy) ---"
    curl -sf --max-time 8 https://api.ipify.org 2>&1 || echo "failed"
    echo ""

    echo "--- Via mixed port proxy (127.0.0.1:7893) ---"
    curl -sf --max-time 8 --proxy http://127.0.0.1:7893 https://api.ipify.org 2>&1 || echo "failed"
    echo ""

    echo "--- TCP port checks ---"
    for _port in 7777 7874 7893 53; do
      _r=$(curl --connect-timeout 1 --max-time 2 -o /dev/null \
            -w "HTTP %{http_code}" "http://127.0.0.1:$_port" 2>&1 || echo "refused/timeout")
      echo "  :$_port → $_r"
    done

    _diag_section "【18】OOM / Crash records"
    dmesg 2>/dev/null | grep -iE "kill|oom|segfault|mihomo|clashforge" | tail -20 || true
    cat /proc/last_kmsg 2>/dev/null | grep -iE "kill|oom|segfault" | tail -10 || true

    _diag_section "【19】ESTABLISHED TCP connections (Mihomo active connections)"
    awk 'NR>1 && $4=="01" {count++} END {print "ESTABLISHED:", count+0}' \
      /proc/net/tcp 2>/dev/null || true

    _diag_section "【20】Collection complete"
    date
    echo "Report: $DIAG_OUTPUT"
  }

  # Apply optional redaction filter (best-effort, not a security guarantee)
  _apply_redact() {
    sed \
      -e 's|https\?://[^ ]*@[^ "]*|<REDACTED_URL>|g' \
      -e 's|[Aa]uthorization:[[:space:]]*[^ ]*|Authorization: <REDACTED>|g' \
      -e 's|[Tt]oken=[^&" ]*|token=<REDACTED>|g' \
      -e 's|[Pp]assword=[^&" ]*|password=<REDACTED>|g' \
      -e 's|[Ss]ecret=[^&" ]*|secret=<REDACTED>|g' \
      -e 's|"Authorization":"[^"]*"|"Authorization":"<REDACTED>"|g'
  }

  log "Collecting diagnostic report..."
  log "Output: $DIAG_OUTPUT"

  if [ "$DIAG_REDACT" = "1" ]; then
    warn "Redaction enabled (best-effort — treat report as sensitive data)"
    if [ "$DIAG_STDOUT" = "1" ]; then
      _do_diag 2>&1 | _apply_redact | tee "$DIAG_OUTPUT"
    else
      _do_diag 2>&1 | _apply_redact > "$DIAG_OUTPUT"
    fi
  else
    if [ "$DIAG_STDOUT" = "1" ]; then
      _do_diag 2>&1 | tee "$DIAG_OUTPUT"
    else
      _do_diag > "$DIAG_OUTPUT" 2>&1
    fi
  fi

  echo "" >&2
  ok "Diagnostic report saved to: $DIAG_OUTPUT"
  printf "[clashforge]    scp root@<router-ip>:%s ./\n" "$DIAG_OUTPUT" >&2
}

# ══════════════════════════════════════════════════════════════════════════════
# MAIN DISPATCHER
# ══════════════════════════════════════════════════════════════════════════════
case "$SUBCOMMAND" in
  status)         cmd_status    ;;
  stop)           cmd_stop      ;;
  reset)          cmd_reset     ;;
  upgrade)        cmd_upgrade   ;;
  check)          cmd_check     ;;
  uninstall)      cmd_uninstall ;;
  diag)           cmd_diag      ;;
  help|--help|-h) usage; exit 0 ;;
  *) die "Unknown subcommand: '$SUBCOMMAND'  (run clashforgectl --help for usage)" ;;
esac
