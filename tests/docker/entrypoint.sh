#!/usr/bin/env bash
set -euo pipefail

RUNTIME_DIR=/var/run/metaclash
CONFIG_FILE=$RUNTIME_DIR/mihomo-config.yaml
OVERRIDES=/testdata/sample-clash-config.yaml
MIHOMO_PID=""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

pass() { echo -e "${GREEN}✅ PASS${RESET} $*"; }
fail() { echo -e "${RED}❌ FAIL${RESET} $*"; FAILED=$((FAILED+1)); }
info() { echo -e "${CYAN}ℹ️  ${RESET} $*"; }
section() { echo -e "\n${BOLD}${YELLOW}=== $* ===${RESET}"; }

FAILED=0

cleanup() {
    if [ -n "$MIHOMO_PID" ] && kill -0 "$MIHOMO_PID" 2>/dev/null; then
        info "stopping mihomo (pid=$MIHOMO_PID)"
        kill "$MIHOMO_PID" 2>/dev/null || true
        wait "$MIHOMO_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# ----------------------------------------------------------------
section "Step 1: Check test prerequisites"
# ----------------------------------------------------------------
if [ ! -f "$OVERRIDES" ]; then
    echo -e "${RED}ERROR: $OVERRIDES not found. Mount it via docker volume.${RESET}"
    exit 1
fi
pass "sample config present: $OVERRIDES"

mihomo -v 2>&1 | head -1 | grep -q "Mihomo Meta" && pass "mihomo binary ok" || { fail "mihomo binary broken"; exit 1; }
genconfig --help 2>&1 | head -1; pass "genconfig binary ok"

# ----------------------------------------------------------------
section "Step 2: Generate mihomo config via clashforge"
# ----------------------------------------------------------------
mkdir -p "$RUNTIME_DIR"
genconfig \
    -overrides "$OVERRIDES" \
    -out "$CONFIG_FILE" \
    -strip

if [ ! -f "$CONFIG_FILE" ]; then
    fail "config not generated"
    exit 1
fi

SIZE=$(wc -c < "$CONFIG_FILE")
pass "config generated: $CONFIG_FILE ($SIZE bytes)"

# ----------------------------------------------------------------
section "Step 3: Validate config with mihomo -t"
# ----------------------------------------------------------------
VALIDATE_OUT=$(mihomo -t -d "$RUNTIME_DIR" -f "$CONFIG_FILE" 2>&1)
if echo "$VALIDATE_OUT" | grep -q "test is successful"; then
    pass "mihomo config validation passed"
else
    fail "mihomo config validation failed:"
    echo "$VALIDATE_OUT"
    exit 1
fi

# ----------------------------------------------------------------
section "Step 4: Parse nodes from config"
# ----------------------------------------------------------------
NODE_COUNT=$(grep -c "^  - name:" "$OVERRIDES" 2>/dev/null || echo 0)
info "nodes in sample config: $NODE_COUNT"
grep "name:" "$CONFIG_FILE" | head -5

# ----------------------------------------------------------------
section "Step 5: Start mihomo"
# ----------------------------------------------------------------
mihomo -d "$RUNTIME_DIR" -f "$CONFIG_FILE" > /tmp/mihomo.log 2>&1 &
MIHOMO_PID=$!
info "mihomo started (pid=$MIHOMO_PID)"

# Wait for API to be ready
for i in $(seq 1 15); do
    if curl -sf http://127.0.0.1:9090/version > /dev/null 2>&1; then
        pass "mihomo API ready (waited ${i}s)"
        break
    fi
    sleep 1
    if [ $i -eq 15 ]; then
        fail "mihomo API not ready after 15s"
        cat /tmp/mihomo.log
        exit 1
    fi
done

# ----------------------------------------------------------------
section "Step 6: Verify proxy groups via API"
# ----------------------------------------------------------------
API_OUT=$(curl -sf http://127.0.0.1:9090/proxies)
if echo "$API_OUT" | grep -q "wujun-sg"; then
    pass "node 'wujun-sg' registered in mihomo"
else
    fail "node 'wujun-sg' not found in mihomo API"
fi

CURRENT_GROUP=$(echo "$API_OUT" | python3 -c "
import json,sys
d=json.load(sys.stdin)
pg=d['proxies'].get('Proxy',{})
print(pg.get('now','unknown'))
" 2>/dev/null || echo "unknown")
info "Proxy group currently selected: $CURRENT_GROUP"

# ----------------------------------------------------------------
section "Step 7: Network baseline (direct, no proxy)"
# ----------------------------------------------------------------
DIRECT_IP=$(curl -sf --connect-timeout 8 --max-time 10 https://api.ipify.org || echo "FAILED")
if [ "$DIRECT_IP" != "FAILED" ]; then
    pass "direct outbound OK: $DIRECT_IP"
else
    fail "direct outbound failed"
fi

DIRECT_GEO=$(curl -sf --connect-timeout 8 --max-time 10 "https://ipinfo.io/${DIRECT_IP}/json" \
    | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('country','?'), d.get('city','?'))" \
    2>/dev/null || echo "unknown")
info "direct IP location: $DIRECT_GEO"

# ----------------------------------------------------------------
section "Step 8: Traffic via wujun-sg proxy"
# ----------------------------------------------------------------
PROXY_IP=$(curl -sf --proxy http://127.0.0.1:7890 \
    --connect-timeout 10 --max-time 15 \
    https://api.ipify.org || echo "FAILED")

if [ "$PROXY_IP" = "FAILED" ]; then
    fail "proxy request failed"
else
    pass "proxy outbound OK: $PROXY_IP"
fi

# ----------------------------------------------------------------
section "Step 9: Verify IP changed (traffic went through proxy)"
# ----------------------------------------------------------------
if [ "$DIRECT_IP" != "$PROXY_IP" ] && [ "$PROXY_IP" != "FAILED" ]; then
    pass "IP changed: direct=$DIRECT_IP → proxy=$PROXY_IP"
else
    fail "IP did NOT change — traffic may not be going through proxy"
fi

# ----------------------------------------------------------------
section "Step 10: Verify proxy exit location"
# ----------------------------------------------------------------
PROXY_GEO=$(curl -sf --connect-timeout 8 --max-time 10 "https://ipinfo.io/${PROXY_IP}/json" \
    | python3 -c "
import json,sys
d=json.load(sys.stdin)
country=d.get('country','?')
city=d.get('city','?')
org=d.get('org','?')
print(f'{country} / {city} — {org}')
" 2>/dev/null || echo "lookup failed")
info "proxy exit location: $PROXY_GEO"

if echo "$PROXY_GEO" | grep -qi "SG\|Singapore"; then
    pass "confirmed exit in Singapore ✈️"
else
    fail "unexpected exit location: $PROXY_GEO"
fi

# ----------------------------------------------------------------
section "Step 11: TLS certificate verification via proxy"
# ----------------------------------------------------------------
TLS_OUT=$(curl -sv --proxy http://127.0.0.1:7890 \
    --connect-timeout 10 --max-time 15 \
    https://www.google.com -o /dev/null 2>&1)

if echo "$TLS_OUT" | grep -q "HTTP/1.1 200 Connection established"; then
    pass "CONNECT tunnel established"
else
    fail "CONNECT tunnel not confirmed"
fi
if echo "$TLS_OUT" | grep -q "TLSv1"; then
    TLS_VER=$(echo "$TLS_OUT" | grep "SSL connection using" | head -1)
    pass "TLS handshake OK — $TLS_VER"
else
    fail "TLS not confirmed"
fi
if echo "$TLS_OUT" | grep -q "CN=www.google.com"; then
    pass "certificate subject verified: CN=www.google.com"
else
    fail "certificate not verified"
fi

# ----------------------------------------------------------------
section "Step 12: mihomo connection log"
# ----------------------------------------------------------------
info "recent mihomo connection log:"
grep "\[TCP\]" /tmp/mihomo.log | tail -10 | while read line; do
    echo "  $line"
done

# ----------------------------------------------------------------
section "Summary"
# ----------------------------------------------------------------
echo ""
echo -e "${BOLD}Direct IP:${RESET}   $DIRECT_IP ($DIRECT_GEO)"
echo -e "${BOLD}Proxy IP:${RESET}    $PROXY_IP ($PROXY_GEO)"
echo ""
if [ "$FAILED" -eq 0 ]; then
    echo -e "${GREEN}${BOLD}ALL TESTS PASSED${RESET}"
    exit 0
else
    echo -e "${RED}${BOLD}$FAILED TEST(S) FAILED${RESET}"
    exit 1
fi
