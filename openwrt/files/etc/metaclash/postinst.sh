#!/bin/sh
set -e
mkdir -p /etc/metaclash /var/run/metaclash

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
level = "warn"
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

/etc/init.d/clashforge enable 2>/dev/null || true
/etc/init.d/clashforge start 2>/dev/null || true

ROUTER_IP=$(ip -4 addr show br-lan 2>/dev/null | awk '/inet /{split($2,a,"/"); print a[1]; exit}')
[ -z "$ROUTER_IP" ] && ROUTER_IP=$(ip -4 addr show 2>/dev/null | awk '/inet /{split($2,a,"/"); if(a[1]!="127.0.0.1"){print a[1];exit}}')
[ -z "$ROUTER_IP" ] && ROUTER_IP="your-router-ip"
echo ""
echo "ClashForge installed! Web UI: http://${ROUTER_IP}:7777"
