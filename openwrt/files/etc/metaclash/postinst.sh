#!/bin/sh
set -e
mkdir -p /etc/metaclash

# Write default config.toml only if not already present
if [ ! -f /etc/metaclash/config.toml ]; then
  cat > /etc/metaclash/config.toml << 'EOF'
[core]
binary = "/usr/bin/mihomo-clashforge"
runtime_dir = "/var/run/metaclash"
data_dir = "/etc/metaclash"
max_restarts = 3

[ports]
http = 7890
socks = 7891
mixed = 7893
redir = 7892
tproxy = 7895
dns = 7874
mihomo_api = 9090
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
EOF
fi

# Write empty overrides.yaml only if not already present
if [ ! -f /etc/metaclash/overrides.yaml ]; then
  printf '# ClashForge overrides - paste your Clash YAML config here\n# or use the Web UI Settings tab to import\n' \
    > /etc/metaclash/overrides.yaml
fi

/etc/init.d/clashforge enable 2>/dev/null || true
/etc/init.d/clashforge start 2>/dev/null || true

ROUTER_IP=$(ip -4 addr show br-lan 2>/dev/null | awk '/inet /{split($2,a,"/"); print a[1]; exit}')
[ -z "$ROUTER_IP" ] && ROUTER_IP=$(ip -4 addr show 2>/dev/null | awk '/inet /{split($2,a,"/"); if(a[1]!="127.0.0.1") {print a[1]; exit}}')
[ -z "$ROUTER_IP" ] && ROUTER_IP="your-router-ip"

echo ""
echo "====================================================="
echo " ClashForge installed! Open the Web UI to configure:"
echo ""
echo "   http://${ROUTER_IP}:7777"
echo ""
echo " Steps:"
echo "   1. Open the URL above in your browser"
echo "   2. Go to [Subscriptions] tab, add your sub URL"
echo "      OR go to [Settings] tab, paste your YAML config"
echo "   3. Click [Start Core] button on the Dashboard"
echo "====================================================="
