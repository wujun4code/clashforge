#!/bin/sh

set +e

SCRIPT_NAME="uninstall-clashforge"
PURGE_CONFIG=1
SKIP_OPKG=0

usage() {
  cat << 'EOF'
Usage: uninstall-clashforge [options]

Options:
  --keep-config   Keep /etc/metaclash and /usr/share/metaclash
  --skip-opkg     Do not call opkg remove; only perform cleanup
  -h, --help      Show this help
EOF
}

log() {
  echo "[$SCRIPT_NAME] $*"
}

dns_port_from_config() {
  if [ -f /etc/metaclash/config.toml ]; then
    port=$(grep -E '^[[:space:]]*dns[[:space:]]*=' /etc/metaclash/config.toml | head -n 1 | sed -E 's/.*=[[:space:]]*([0-9]+).*/\1/')
    [ -n "$port" ] && echo "$port" && return 0
  fi
  echo "17874"
}

kill_leftovers() {
  /etc/init.d/clashforge disable 2>/dev/null || true
  /etc/init.d/clashforge stop 2>/dev/null || true

  for pidfile in /var/run/metaclash/metaclash.pid /var/run/metaclash/mihomo.pid; do
    if [ -f "$pidfile" ]; then
      pid=$(cat "$pidfile" 2>/dev/null)
      [ -n "$pid" ] && kill -9 "$pid" 2>/dev/null || true
    fi
  done

  for pid in $(ls /proc 2>/dev/null | grep '^[0-9]'); do
    cmdline=$(tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null)
    case "$cmdline" in
      *"/usr/bin/clashforge"*|*"/usr/bin/mihomo-clashforge"*)
        kill -9 "$pid" 2>/dev/null || true
        ;;
    esac
  done

  ubus call service delete '{"name":"clashforge"}' 2>/dev/null || true
}

cleanup_firewall() {
  dns_port="$1"

  nft delete table inet metaclash 2>/dev/null || true
  while ip rule del fwmark 0x1a3 table 100 2>/dev/null; do :; done
  ip route flush table 100 2>/dev/null || true

  iptables -t mangle -D PREROUTING -j METACLASH 2>/dev/null || true
  iptables -t mangle -F METACLASH 2>/dev/null || true
  iptables -t mangle -X METACLASH 2>/dev/null || true

  for p in "$dns_port" 17874 7874; do
    [ -n "$p" ] || continue
    iptables -t nat -D PREROUTING -p udp --dport 53 -j REDIRECT --to-port "$p" 2>/dev/null || true
    iptables -t nat -D PREROUTING -p tcp --dport 53 -j REDIRECT --to-port "$p" 2>/dev/null || true
  done
}

cleanup_dnsmasq_takeover() {
  rm -f /etc/dnsmasq.d/clashforge.conf
  rm -f /tmp/dnsmasq.d/clashforge.conf

  for f in /tmp/dnsmasq.cfg*.d/clashforge.conf /var/etc/dnsmasq.conf*/clashforge.conf; do
    [ -e "$f" ] && rm -f "$f"
  done

  /etc/init.d/dnsmasq restart 2>/dev/null || /etc/init.d/dnsmasq reload 2>/dev/null || true
}

cleanup_runtime() {
  rm -f /var/log/clashforge.log
  rm -f /var/run/metaclash/metaclash.pid /var/run/metaclash/mihomo.pid
  rm -rf /var/run/metaclash

  for p in /tmp/metaclash* /tmp/clashforge*; do
    [ -e "$p" ] && rm -rf "$p"
  done
}

cleanup_files() {
  rm -f /usr/bin/clashforge
  rm -f /usr/bin/mihomo-clashforge
  rm -f /etc/init.d/clashforge

  rm -f /usr/share/luci/menu.d/luci-app-clashforge.json
  rm -f /usr/share/rpcd/acl.d/luci-app-clashforge.json
  rm -f /www/luci-static/resources/view/clashforge/main.js

  rmdir /www/luci-static/resources/view/clashforge 2>/dev/null || true

  if [ "$PURGE_CONFIG" = "1" ]; then
    rm -rf /etc/metaclash
    rm -rf /usr/share/metaclash
  fi
}

while [ $# -gt 0 ]; do
  case "$1" in
    --keep-config)
      PURGE_CONFIG=0
      ;;
    --skip-opkg)
      SKIP_OPKG=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      log "unknown option: $1"
      usage
      exit 1
      ;;
  esac
  shift
done

dns_port=$(dns_port_from_config)

log "stopping clashforge and cleaning takeover state"
kill_leftovers
cleanup_firewall "$dns_port"
cleanup_dnsmasq_takeover

if [ "$SKIP_OPKG" = "0" ] && command -v opkg >/dev/null 2>&1; then
  if opkg status clashforge >/dev/null 2>&1; then
    log "removing opkg package: clashforge"
    CLASHFORGE_PURGE_CONFIG="$PURGE_CONFIG" opkg remove clashforge >/dev/null 2>&1 || \
      log "opkg remove returned non-zero, continuing with manual cleanup"
  else
    log "package clashforge not found in opkg status"
  fi
fi

dns_port=$(dns_port_from_config)
kill_leftovers
cleanup_firewall "$dns_port"
cleanup_dnsmasq_takeover
cleanup_runtime
cleanup_files

log "done"
if [ "$PURGE_CONFIG" = "1" ]; then
  log "all config and runtime data removed"
else
  log "config and data kept under /etc/metaclash and /usr/share/metaclash"
fi

exit 0