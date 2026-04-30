#!/bin/sh
set -e
mkdir -p /etc/metaclash /var/run/metaclash

# Restore persistent data backed up by prerm (handles opkg removing /etc/metaclash on upgrade)
if [ -f /tmp/clashforge-config.bak ];      then mv /tmp/clashforge-config.bak    /etc/metaclash/config.toml;       fi
if [ -f /tmp/clashforge-ed25519.bak ];     then mv /tmp/clashforge-ed25519.bak   /etc/metaclash/clashforge_ed25519; chmod 600 /etc/metaclash/clashforge_ed25519; fi
if [ -f /tmp/clashforge-nodes.bak ];       then mv /tmp/clashforge-nodes.bak     /etc/metaclash/nodes.json;         fi
if [ -f /tmp/clashforge-nodes-key.bak ];   then mv /tmp/clashforge-nodes-key.bak /etc/metaclash/nodes.key;          fi

# Legacy SSH key (if restored above) is migrated to /root/.ssh by the Go binary
# on first start after upgrade.

if [ ! -f /etc/metaclash/config.toml ]; then
  cat > /etc/metaclash/config.toml << 'TOML'
[core]
binary = "/usr/bin/mihomo-clashforge"
runtime_dir = "/var/run/metaclash"
data_dir = "/etc/metaclash"
max_restarts = 3
[ports]
http = 17890
socks = 17891
mixed = 17893
redir = 17892
tproxy = 17895
dns = 17874
mihomo_api = 19090
ui = 7777
[network]
mode = "none"
firewall_backend = "none"
bypass_lan = true
bypass_china = false
ipv6 = false
bypass_cidr = []
[dns]
enable = false
dnsmasq_mode = "none"
[security]
api_secret = ""
allow_lan = true
[log]
level = "warning"
file = ""
max_size_mb = 10
[update]
auto_subscription = true
subscription_interval = "6h"
auto_geoip = false
auto_geosite = false
TOML
fi

if [ ! -f /etc/metaclash/overrides.yaml ]; then
  printf '# ClashForge overrides\n' > /etc/metaclash/overrides.yaml
fi

# Belt-and-suspenders: kill any surviving clashforge process before starting
# (handles the case where prerm couldn't reach a manually-started process)
for pid in $(ls /proc 2>/dev/null | grep '^[0-9]'); do
  cmdline=$(cat /proc/$pid/cmdline 2>/dev/null | tr '\0' ' ')
  case "$cmdline" in
    */usr/bin/clashforge*) kill -9 "$pid" 2>/dev/null || true ;;
  esac
done
rm -f /var/run/metaclash/metaclash.pid

# Warn if iproute2 full (ip rule/route with fwmark+table) is unavailable.
# ip-full is NOT declared as an opkg Depends because on most OpenWrt x86_64
# images it is baked into the base firmware and absent from the package index,
# which caused a spurious "cannot find dependency" warning on every install.
if ! ip rule add fwmark 0x1 table 1 2>/dev/null; then
  echo "⚠ ClashForge: iproute2 (ip-full) not available — transparent proxy (tproxy) will not work." >&2
else
  ip rule del fwmark 0x1 table 1 2>/dev/null || true
fi

/etc/init.d/clashforge enable 2>/dev/null || true
/etc/init.d/clashforge start 2>/dev/null || true

ROUTER_IP=$(ip -4 addr show br-lan 2>/dev/null | awk '/inet /{split($2,a,"/"); print a[1]; exit}')
[ -z "$ROUTER_IP" ] && ROUTER_IP=$(ip -4 addr show 2>/dev/null | awk '/inet /{split($2,a,"/"); if(a[1]!="127.0.0.1"){print a[1];exit}}')
[ -z "$ROUTER_IP" ] && ROUTER_IP="your-router-ip"
echo ""
echo "ClashForge installed! Web UI: http://${ROUTER_IP}:7777"
