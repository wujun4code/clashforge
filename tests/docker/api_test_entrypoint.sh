#!/usr/bin/env bash
# Entrypoint for API test container
# Starts clashforge with a minimal config, then runs api_test.sh
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

echo -e "${BOLD}ClashForge API Integration Tests${RESET}"
echo "=================================="

# Write a minimal test config
mkdir -p /etc/metaclash /var/run/metaclash
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

[security]
api_secret = ""
allow_lan = true

[log]
level = "info"
EOF

echo -e "${CYAN}Starting clashforge...${RESET}"
clashforge -config /etc/metaclash/config.toml &
CF_PID=$!
echo "clashforge PID=$CF_PID"

cleanup(){
    echo -e "${CYAN}Stopping clashforge (pid=$CF_PID)${RESET}"
    kill $CF_PID 2>/dev/null || true
    wait $CF_PID 2>/dev/null || true
}
trap cleanup EXIT

# Run the API tests
bash /api_test.sh
EXIT_CODE=$?
exit $EXIT_CODE
