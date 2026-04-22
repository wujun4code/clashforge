#!/bin/sh
# Entrypoint for API test container (sh-compatible)
set -eu

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

printf "${BOLD}ClashForge API Integration Tests${RESET}\n"
printf "==================================\n"

mkdir -p /etc/metaclash /var/run/metaclash
rm -f /var/run/metaclash/metaclash.pid

# Minimal test config — disable everything that needs root or external binaries
cat > /etc/metaclash/config.toml << 'EOF'
[core]
binary = "/usr/bin/mihomo-notexist"
runtime_dir = "/var/run/metaclash"
data_dir = "/etc/metaclash"
max_restarts = 0

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

[dns]
enable = false
dnsmasq_mode = "none"

[security]
api_secret = ""
allow_lan = true

[log]
level = "info"
EOF

printf "${CYAN}Starting clashforge...${RESET}\n"
clashforge -config /etc/metaclash/config.toml &
CF_PID=$!
printf "clashforge PID=%d\n" "$CF_PID"

cleanup(){
    printf "${CYAN}Stopping clashforge (pid=%d)${RESET}\n" "$CF_PID"
    kill "$CF_PID" 2>/dev/null || true
    wait "$CF_PID" 2>/dev/null || true
}
trap cleanup EXIT

sh /api_test.sh
exit $?
