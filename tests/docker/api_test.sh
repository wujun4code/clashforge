#!/usr/bin/env bash
# tests/docker/api_test.sh — ClashForge API smoke tests
# Run inside the clashforge container (no mihomo needed)
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'
PASS(){ echo -e "${GREEN}✅ PASS${RESET} $*"; }
FAIL(){ echo -e "${RED}❌ FAIL${RESET} $*"; FAILED=$((FAILED+1)); }
INFO(){ echo -e "${CYAN}ℹ️  ${RESET} $*"; }
SECTION(){ echo -e "\n${BOLD}${YELLOW}=== $* ===${RESET}"; }

FAILED=0
CF_URL="http://127.0.0.1:7777"
API="$CF_URL/api/v1"

# ── wait for clashforge to be up ──────────────────────────────────────────────
SECTION "Startup: waiting for clashforge"
for i in $(seq 1 20); do
    if curl -sf "$CF_URL/healthz" > /dev/null 2>&1; then
        PASS "clashforge up (waited ${i}s)"; break
    fi
    sleep 1
    [ $i -eq 20 ] && FAIL "clashforge did not start in 20s" && exit 1
done

# ── helper ────────────────────────────────────────────────────────────────────
check_json(){
    local desc="$1" url="$2" expected_field="$3" expected_val="$4"
    local body code
    body=$(curl -sf "$url" 2>/dev/null) || { FAIL "$desc — curl failed ($url)"; return; }
    if echo "$body" | python3 -c "import json,sys; d=json.load(sys.stdin); assert d.get('ok')==True" 2>/dev/null; then
        if [ -n "$expected_field" ]; then
            actual=$(echo "$body" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('data',{}).get('$expected_field','__missing__'))" 2>/dev/null || echo "__err__")
            if [ "$actual" = "$expected_val" ]; then
                PASS "$desc (${expected_field}=${actual})"
            else
                FAIL "$desc — expected ${expected_field}=${expected_val}, got ${actual}"
            fi
        else
            PASS "$desc"
        fi
    else
        FAIL "$desc — ok!=true: $body"
    fi
}

check_http(){
    local desc="$1" method="$2" url="$3" expected_code="$4" body_arg="${5:-}"
    local args=(-sf -o /dev/null -w '%{http_code}' -X "$method")
    [ -n "$body_arg" ] && args+=(-H 'Content-Type: application/json' -d "$body_arg")
    local code
    code=$(curl "${args[@]}" "$url" 2>/dev/null) || code="000"
    if [ "$code" = "$expected_code" ]; then
        PASS "$desc (HTTP $code)"
    else
        FAIL "$desc — expected HTTP $expected_code, got $code"
    fi
}

# ── healthz ───────────────────────────────────────────────────────────────────
SECTION "Test 1: Health check"
check_json "GET /healthz" "$CF_URL/healthz" "status" "ok"

# ── status ────────────────────────────────────────────────────────────────────
SECTION "Test 2: GET /api/v1/status"
check_json "status ok=true"        "$API/status"
STATUS=$(curl -sf "$API/status" 2>/dev/null)
CORE_STATE=$(echo "$STATUS" | python3 -c "import json,sys; print(json.load(sys.stdin)['data']['core']['state'])" 2>/dev/null || echo "unknown")
INFO "core.state = $CORE_STATE"
[ "$CORE_STATE" != "unknown" ] && PASS "status.core.state present ($CORE_STATE)" || FAIL "status.core.state missing"

META_VER=$(echo "$STATUS" | python3 -c "import json,sys; print(json.load(sys.stdin)['data']['metaclash']['version'])" 2>/dev/null || echo "")
[ -n "$META_VER" ] && PASS "metaclash.version = $META_VER" || FAIL "metaclash.version missing"

# ── config ────────────────────────────────────────────────────────────────────
SECTION "Test 3: GET /api/v1/config"
check_json "config ok=true" "$API/config"
CFG=$(curl -sf "$API/config")
HTTP_PORT=$(echo "$CFG" | python3 -c "import json,sys; print(json.load(sys.stdin)['data']['ports']['http'])" 2>/dev/null || echo "")
[ "$HTTP_PORT" = "7890" ] && PASS "config.ports.http=7890" || FAIL "config.ports.http expected 7890 got $HTTP_PORT"

# ── config PUT ────────────────────────────────────────────────────────────────
SECTION "Test 4: PUT /api/v1/config (partial update)"
check_http "PUT /config partial" PUT "$API/config" 200 '{"log":{"level":"debug"}}'

# ── subscriptions CRUD ────────────────────────────────────────────────────────
SECTION "Test 5: Subscriptions CRUD"
check_json "GET /subscriptions" "$API/subscriptions"

# Add
ADD_RESP=$(curl -sf -X POST "$API/subscriptions" \
    -H 'Content-Type: application/json' \
    -d '{"name":"test-sub","type":"url","url":"https://example.com/sub","enabled":true}' 2>/dev/null)
SUB_OK=$(echo "$ADD_RESP" | python3 -c "import json,sys; print(json.load(sys.stdin).get('ok'))" 2>/dev/null || echo "false")
SUB_ID=$(echo "$ADD_RESP" | python3 -c "import json,sys; print(json.load(sys.stdin).get('data',{}).get('id',''))" 2>/dev/null || echo "")
[ "$SUB_OK" = "True" ] && PASS "POST /subscriptions → id=$SUB_ID" || FAIL "POST /subscriptions failed: $ADD_RESP"

# List (should have 1)
if [ -n "$SUB_ID" ]; then
    COUNT=$(curl -sf "$API/subscriptions" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['data']['subscriptions']))" 2>/dev/null || echo 0)
    [ "$COUNT" -ge 1 ] && PASS "subscriptions list count=$COUNT" || FAIL "subscription not in list"

    # Update
    check_http "PUT /subscriptions/:id" PUT "$API/subscriptions/$SUB_ID" 200 '{"enabled":false}'

    # Verify update
    ENABLED=$(curl -sf "$API/subscriptions" | python3 -c "
import json,sys
subs=json.load(sys.stdin)['data']['subscriptions']
for s in subs:
    if s['id']=='$SUB_ID':
        print(s['enabled'])
        break
" 2>/dev/null || echo "unknown")
    [ "$ENABLED" = "False" ] && PASS "subscription disabled after PUT" || FAIL "subscription enabled=$ENABLED (expected False)"

    # Delete
    check_http "DELETE /subscriptions/:id" DELETE "$API/subscriptions/$SUB_ID" 200

    # Verify deleted
    COUNT2=$(curl -sf "$API/subscriptions" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['data']['subscriptions']))" 2>/dev/null || echo 0)
    [ "$COUNT2" -eq 0 ] && PASS "subscription deleted (count=$COUNT2)" || FAIL "subscription still present (count=$COUNT2)"
fi

# ── overrides ─────────────────────────────────────────────────────────────────
SECTION "Test 6: Config overrides"
check_json "GET /config/overrides" "$API/config/overrides"
check_http "PUT /config/overrides valid YAML" PUT "$API/config/overrides" 200 '{"content":"# test\nrules:\n  - DOMAIN,test.example.com,DIRECT\n"}'
check_http "PUT /config/overrides invalid YAML" PUT "$API/config/overrides" 400 '{"content":"key: [invalid yaml {"}'

# ── core ──────────────────────────────────────────────────────────────────────
SECTION "Test 7: Core management API"
# mihomo binary won't exist in this container, so start will fail with a proper error
START_RESP=$(curl -sf -X POST "$API/core/start" 2>/dev/null || echo "{}")
START_CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/core/start" 2>/dev/null)
# Either 500 (binary not found) or 409 (already running) are valid structured responses
if [ "$START_CODE" = "500" ] || [ "$START_CODE" = "409" ] || [ "$START_CODE" = "200" ]; then
    PASS "POST /core/start returns structured response (HTTP $START_CODE)"
else
    FAIL "POST /core/start unexpected code $START_CODE"
fi

check_json "GET /core/version" "$API/core/version"

# ── UI is served ──────────────────────────────────────────────────────────────
SECTION "Test 8: UI is served"
HTML_CODE=$(curl -s -o /dev/null -w '%{http_code}' "$CF_URL/" 2>/dev/null)
[ "$HTML_CODE" = "200" ] && PASS "GET / returns HTML (HTTP $HTML_CODE)" || FAIL "GET / returned HTTP $HTML_CODE"

CONTENT_TYPE=$(curl -sI "$CF_URL/" 2>/dev/null | grep -i content-type | head -1)
echo "$CONTENT_TYPE" | grep -qi "text/html" && PASS "Content-Type is text/html" || FAIL "Content-Type not html: $CONTENT_TYPE"

# Assets
ASSET_CODE=$(curl -s -o /dev/null -w '%{http_code}' "$CF_URL/assets/" 2>/dev/null)
INFO "Assets path HTTP $ASSET_CODE"

# SPA fallback: unknown path should return index.html
SPA_CODE=$(curl -s -o /dev/null -w '%{http_code}' "$CF_URL/proxies" 2>/dev/null)
[ "$SPA_CODE" = "200" ] && PASS "SPA fallback /proxies → 200" || FAIL "SPA fallback returned HTTP $SPA_CODE"

# ── CORS ──────────────────────────────────────────────────────────────────────
SECTION "Test 9: CORS headers"
CORS=$(curl -sI -H "Origin: http://192.168.1.100" "$API/status" 2>/dev/null | grep -i "Access-Control" | head -3)
echo "$CORS" | grep -qi "Access-Control-Allow-Origin" && PASS "CORS headers present" || FAIL "CORS headers missing"

# ── rate / edge cases ─────────────────────────────────────────────────────────
SECTION "Test 10: Edge cases"
# Non-existent subscription
check_http "DELETE non-existent sub" DELETE "$API/subscriptions/sub_nonexistent" 404
check_http "PUT non-existent sub"    PUT    "$API/subscriptions/sub_nonexistent" 404 '{"name":"x"}'

# Summary
SECTION "Summary"
if [ "$FAILED" -eq 0 ]; then
    echo -e "\n${GREEN}${BOLD}ALL API TESTS PASSED${RESET}"
    exit 0
else
    echo -e "\n${RED}${BOLD}$FAILED TEST(S) FAILED${RESET}"
    exit 1
fi
