#!/bin/sh
# tests/docker/api_test.sh — ClashForge API smoke tests (sh-compatible)
set -eu

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'
PASS(){ printf "${GREEN}✅ PASS${RESET} %s\n" "$*"; }
FAIL(){ printf "${RED}❌ FAIL${RESET} %s\n" "$*"; FAILED=$((FAILED+1)); }
INFO(){ printf "${CYAN}ℹ️   ${RESET} %s\n" "$*"; }
SECTION(){ printf "\n${BOLD}${YELLOW}=== %s ===${RESET}\n" "$*"; }

FAILED=0
CF_URL="http://127.0.0.1:7777"
API="$CF_URL/api/v1"

# ── wait for clashforge ───────────────────────────────────────────────────────
SECTION "Startup: waiting for clashforge"
i=0
while [ $i -lt 20 ]; do
    if curl -sf "$CF_URL/healthz" > /dev/null 2>&1; then
        PASS "clashforge up (waited ${i}s)"; break
    fi
    sleep 1; i=$((i+1))
    if [ $i -eq 20 ]; then FAIL "clashforge did not start in 20s"; exit 1; fi
done

# ── helpers ───────────────────────────────────────────────────────────────────
check_json_ok(){
    desc="$1"; url="$2"
    body=$(curl -sf "$url" 2>/dev/null) || { FAIL "$desc — curl failed"; return; }
    echo "$body" | python3 -c "import json,sys; d=json.load(sys.stdin); assert d.get('ok')==True" 2>/dev/null \
        && PASS "$desc" || FAIL "$desc — ok!=true: $body"
}

check_http(){
    desc="$1"; method="$2"; url="$3"; expected="$4"; body_data="${5:-}"
    if [ -n "$body_data" ]; then
        code=$(curl -s -o /dev/null -w '%{http_code}' -X "$method" -H 'Content-Type: application/json' -d "$body_data" "$url" 2>/dev/null)
    else
        code=$(curl -s -o /dev/null -w '%{http_code}' -X "$method" "$url" 2>/dev/null)
    fi
    [ "$code" = "$expected" ] && PASS "$desc (HTTP $code)" || FAIL "$desc — expected HTTP $expected, got $code"
}

json_get(){
    url="$1"; field="$2"
    curl -sf "$url" 2>/dev/null | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('data',{}).get('$field',''))" 2>/dev/null || echo ""
}

# ── test 1: healthz ───────────────────────────────────────────────────────────
SECTION "Test 1: Health check"
check_json_ok "GET /healthz" "$CF_URL/healthz"

# ── test 2: status ────────────────────────────────────────────────────────────
SECTION "Test 2: GET /api/v1/status"
check_json_ok "GET /api/v1/status" "$API/status"

STATUS=$(curl -sf "$API/status" 2>/dev/null || echo "{}")
CORE_STATE=$(echo "$STATUS" | python3 -c "import json,sys; print(json.load(sys.stdin)['data']['core']['state'])" 2>/dev/null || echo "")
[ -n "$CORE_STATE" ] && PASS "core.state present: $CORE_STATE" || FAIL "core.state missing"

META_VER=$(echo "$STATUS" | python3 -c "import json,sys; print(json.load(sys.stdin)['data']['metaclash']['version'])" 2>/dev/null || echo "")
[ -n "$META_VER" ] && PASS "metaclash.version = $META_VER" || FAIL "metaclash.version missing"

# ── test 3: config ────────────────────────────────────────────────────────────
SECTION "Test 3: GET /api/v1/config"
check_json_ok "GET /api/v1/config" "$API/config"
HTTP_PORT=$(curl -sf "$API/config" 2>/dev/null | python3 -c "import json,sys; print(json.load(sys.stdin)['data']['ports']['http'])" 2>/dev/null || echo "")
[ "$HTTP_PORT" = "7890" ] && PASS "config.ports.http = $HTTP_PORT" || FAIL "config.ports.http expected 7890 got $HTTP_PORT"

# ── test 4: config partial update ────────────────────────────────────────────
SECTION "Test 4: PUT /api/v1/config (partial update)"
check_http "PUT /config (log.level=debug)" PUT "$API/config" 200 '{"log":{"level":"debug"}}'

# ── test 5: subscriptions CRUD ───────────────────────────────────────────────
SECTION "Test 5: Subscriptions CRUD"
check_json_ok "GET /subscriptions (empty)" "$API/subscriptions"

# Add subscription
ADD_RESP=$(curl -sf -X POST "$API/subscriptions" \
    -H 'Content-Type: application/json' \
    -d '{"name":"test-sub","type":"url","url":"https://example.com/sub","enabled":true}' 2>/dev/null || echo "{}")
SUB_ID=$(echo "$ADD_RESP" | python3 -c "import json,sys; print(json.load(sys.stdin).get('data',{}).get('id',''))" 2>/dev/null || echo "")
OK=$(echo "$ADD_RESP" | python3 -c "import json,sys; print(json.load(sys.stdin).get('ok',''))" 2>/dev/null || echo "")
[ "$OK" = "True" ] && PASS "POST /subscriptions → id=$SUB_ID" || FAIL "POST /subscriptions failed: $ADD_RESP"

if [ -n "$SUB_ID" ]; then
    # List
    COUNT=$(curl -sf "$API/subscriptions" 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)['data']['subscriptions']))" 2>/dev/null || echo 0)
    [ "$COUNT" -ge 1 ] && PASS "subscriptions list count=$COUNT" || FAIL "subscription not in list"

    # Update
    check_http "PUT /subscriptions/:id" PUT "$API/subscriptions/$SUB_ID" 200 '{"enabled":false}'

    # Verify enabled=false
    ENABLED=$(curl -sf "$API/subscriptions" 2>/dev/null | python3 -c "
import json,sys
subs=json.load(sys.stdin)['data']['subscriptions']
for s in subs:
    if s['id']=='$SUB_ID':
        print(s['enabled'])
        break
" 2>/dev/null || echo "unknown")
    [ "$ENABLED" = "False" ] && PASS "subscription disabled after PUT" || FAIL "enabled=$ENABLED (expected False)"

    # Delete
    check_http "DELETE /subscriptions/:id" DELETE "$API/subscriptions/$SUB_ID" 200

    # Verify gone
    COUNT2=$(curl -sf "$API/subscriptions" 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)['data']['subscriptions']))" 2>/dev/null || echo 0)
    [ "$COUNT2" -eq 0 ] && PASS "subscription deleted (count=$COUNT2)" || FAIL "still present (count=$COUNT2)"
fi

# ── test 6: overrides ────────────────────────────────────────────────────────
SECTION "Test 6: Config overrides"
check_json_ok "GET /config/overrides" "$API/config/overrides"
check_http "PUT /config/overrides valid YAML" PUT "$API/config/overrides" 200 '{"content":"# test\nrules:\n  - DOMAIN,test.example.com,DIRECT\n"}'
check_http "PUT /config/overrides invalid YAML" PUT "$API/config/overrides" 400 '{"content":"key: [bad { yaml"}'

# ── test 7: core endpoints ────────────────────────────────────────────────────
SECTION "Test 7: Core management"
START_CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/core/start" 2>/dev/null)
case "$START_CODE" in
    200|409|500) PASS "POST /core/start returns structured HTTP $START_CODE" ;;
    *) FAIL "POST /core/start unexpected code $START_CODE" ;;
esac
check_json_ok "GET /core/version" "$API/core/version"

# ── test 8: UI is served ──────────────────────────────────────────────────────
SECTION "Test 8: UI served from embed"
HTML_CODE=$(curl -s -o /dev/null -w '%{http_code}' "$CF_URL/" 2>/dev/null)
[ "$HTML_CODE" = "200" ] && PASS "GET / → 200" || FAIL "GET / returned $HTML_CODE"

CTYPE=$(curl -s -o /dev/null -w '%{content_type}' "$CF_URL/" 2>/dev/null)
echo "$CTYPE" | grep -qi "text/html" && PASS "Content-Type: text/html ($CTYPE)" || FAIL "wrong Content-Type: $CTYPE"

# SPA fallback
SPA=$(curl -s -o /dev/null -w '%{http_code}' "$CF_URL/proxies" 2>/dev/null)
[ "$SPA" = "200" ] && PASS "SPA fallback /proxies → 200" || FAIL "SPA fallback /proxies → $SPA"

SPA2=$(curl -s -o /dev/null -w '%{http_code}' "$CF_URL/settings" 2>/dev/null)
[ "$SPA2" = "200" ] && PASS "SPA fallback /settings → 200" || FAIL "SPA fallback /settings → $SPA2"

# ── test 9: CORS ──────────────────────────────────────────────────────────────
SECTION "Test 9: CORS headers"
CORS=$(curl -sI -H "Origin: http://192.168.1.100" "$API/status" 2>/dev/null | tr -d '\r' | grep -i "access-control-allow-origin" | head -1)
[ -n "$CORS" ] && PASS "CORS header present: $CORS" || FAIL "CORS header missing"

# ── test 10: edge cases ───────────────────────────────────────────────────────
SECTION "Test 10: 404 / edge cases"
check_http "DELETE non-existent sub" DELETE "$API/subscriptions/sub_notexist" 404
check_http "PUT non-existent sub"    PUT    "$API/subscriptions/sub_notexist" 404 '{"name":"x"}'
check_http "GET unknown API path"    GET    "$API/does_not_exist" 404

# ── summary ───────────────────────────────────────────────────────────────────
SECTION "Summary"
if [ "$FAILED" -eq 0 ]; then
    printf "\n${GREEN}${BOLD}ALL API TESTS PASSED${RESET}\n"
    exit 0
else
    printf "\n${RED}${BOLD}%d TEST(S) FAILED${RESET}\n" "$FAILED"
    exit 1
fi
