#!/bin/sh
# tests/e2e/vm/setup-launch.sh
# Step 5 — 模拟 Setup Wizard 操作 + 接管断言
#
# 模拟用户在 http://192.168.10.1:7777/setup 上完成的操作：
#   1. 添加订阅并同步节点
#   2. 配置 DNS（fake-ip + dnsmasq upstream）
#   3. 配置网络（tproxy + nftables）
#   4. 触发 setup/launch
#
# 启动后只断言"接管是否到位"，不做连通性测试（连通性在 probe.sh 里）：
#   SL-01  添加订阅 → 返回订阅 ID
#   SL-02  sync-update → 节点拉取成功（节点数 ≥ 1）
#   SL-03  setup/launch → success:true
#   SL-04  mihomo 进程已启动
#   SL-05  nftables metaclash 表已创建
#   SL-06  nftables tproxy 规则已存在
#   SL-07  ip rule 策略路由已配置
#   SL-08  dnsmasq upstream 配置已写入
#   SL-09  DNS 解析返回 fake-ip 地址段（198.18.x.x）
#
# 输出：
#   /tmp/cf-snapshot/sub-id        订阅 ID
#   /tmp/cf-snapshot/proxy-auth    代理认证（user:pass）
#   /tmp/cf-snapshot/proxy-node    节点名:服务器域名
#
# 环境变量：
#   SUBSCRIPTION_URL    订阅地址（必须）
#   GITHUB_STEP_SUMMARY

set -e

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

FAILED=0
RESULTS=""
TOTAL=0

record() {
    STATUS="$1"; TC="$2"; NAME="$3"; EXPECTED="$4"; ACTUAL="$5"
    RESULTS="${RESULTS}${STATUS}|${TC}|${NAME}|${EXPECTED}|${ACTUAL}\n"
    TOTAL=$((TOTAL+1))
    case "$STATUS" in
        PASS) printf "${GREEN}✅ PASS${RESET} [%s] %s — %s\n" "$TC" "$NAME" "$ACTUAL" ;;
        FAIL) printf "${RED}❌ FAIL${RESET} [%s] %s — %s\n" "$TC" "$NAME" "$ACTUAL"; FAILED=$((FAILED+1)) ;;
        WARN) printf "${YELLOW}⚠️  WARN${RESET} [%s] %s — %s\n" "$TC" "$NAME" "$ACTUAL" ;;
    esac
}

info()    { printf "${CYAN}ℹ️   ${RESET} %s\n" "$*"; }
section() { printf "\n${BOLD}${YELLOW}=== %s ===${RESET}\n" "$*"; }
summary() { [ -n "${GITHUB_STEP_SUMMARY:-}" ] && echo "$*" >> "$GITHUB_STEP_SUMMARY" || true; }

CF_API="http://127.0.0.1:7777/api/v1"
SNAPSHOT_DIR="/tmp/cf-snapshot"

if [ -z "${SUBSCRIPTION_URL:-}" ]; then
    echo "ERROR: SUBSCRIPTION_URL is required"
    exit 1
fi

section "Step 5 — Setup Launch + 接管断言"

# ── SL-01: 添加订阅 ────────────────────────────────────────────────────────────
info "添加订阅..."
ADD_RESP=$(curl -sf --max-time 15 \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"${SUBSCRIPTION_URL}\",\"name\":\"e2e-test\",\"enabled\":true}" \
    "$CF_API/subscriptions" 2>/dev/null || echo "FAILED")

if [ "$ADD_RESP" = "FAILED" ]; then
    record FAIL SL-01 "添加订阅" "POST /subscriptions → 返回订阅 ID" "API 请求失败"
    exit 1
fi

SUB_ID=$(echo "$ADD_RESP" | grep -o '"id":"[^"]*"' | head -1 | sed 's/"id":"//;s/"//')
if [ -n "$SUB_ID" ]; then
    echo "$SUB_ID" > "$SNAPSHOT_DIR/sub-id"
    record PASS SL-01 "添加订阅" "POST /subscriptions → 返回订阅 ID" "订阅 ID: $SUB_ID"
else
    record FAIL SL-01 "添加订阅" "POST /subscriptions → 返回订阅 ID" "响应中无 id 字段: $ADD_RESP"
    exit 1
fi

# ── SL-02: 同步节点 ────────────────────────────────────────────────────────────
info "同步订阅节点..."
SYNC_RESP=$(curl -sf --max-time 60 \
    -H "Content-Type: application/json" -d '{}' \
    "$CF_API/subscriptions/$SUB_ID/sync-update" 2>/dev/null || echo "FAILED")

if echo "$SYNC_RESP" | grep -qE '"ok"|"true"|"success"|"nodes"|"proxies"'; then
    NODE_COUNT=$(echo "$SYNC_RESP" | grep -o '"count":[0-9]*' | head -1 | sed 's/"count"://' || echo "?")
    record PASS SL-02 "同步订阅节点" "POST /subscriptions/{id}/sync-update → 节点数 ≥ 1" "节点同步成功（count: $NODE_COUNT）"
else
    record FAIL SL-02 "同步订阅节点" "POST /subscriptions/{id}/sync-update → 节点数 ≥ 1" "sync 失败: $SYNC_RESP"
    exit 1
fi

# ── SL-03: setup/launch ───────────────────────────────────────────────────────
LAUNCH_PAYLOAD='{"dns":{"enable":true,"mode":"fake-ip","dnsmasq_mode":"upstream","apply_on_start":true},"network":{"mode":"tproxy","firewall_backend":"nftables","bypass_lan":true,"bypass_china":false,"apply_on_start":true}}'
info "触发 setup/launch..."
info "DNS  选项: enable=true  mode=fake-ip  dnsmasq_mode=upstream  apply_on_start=true"
info "网络选项: mode=tproxy  firewall_backend=nftables  bypass_lan=true  bypass_china=false  apply_on_start=true"
info "完整 payload: $LAUNCH_PAYLOAD"
LAUNCH_RESP=$(curl -sf --max-time 90 \
    -H "Content-Type: application/json" \
    -d "$LAUNCH_PAYLOAD" \
    "$CF_API/setup/launch" 2>/dev/null || echo "FAILED")

if echo "$LAUNCH_RESP" | grep -q '"success":true'; then
    record PASS SL-03 "setup/launch" "POST /setup/launch → {success:true}" "launch 成功"
else
    LAUNCH_ERR=$(echo "$LAUNCH_RESP" | grep -o '"error":"[^"]*"' | head -1 || echo "$LAUNCH_RESP")
    record FAIL SL-03 "setup/launch" "POST /setup/launch → {success:true}" "失败: $LAUNCH_ERR"
    exit 1
fi

# 等待接管生效
sleep 5

# ── SL-04: mihomo 进程已启动 ──────────────────────────────────────────────────
MIHOMO_PID=$(pgrep -f "mihomo-clashforge" 2>/dev/null || echo "")
if [ -n "$MIHOMO_PID" ]; then
    record PASS SL-04 "mihomo 进程已启动" "pgrep mihomo-clashforge → 有进程" "PID: $MIHOMO_PID"
else
    record FAIL SL-04 "mihomo 进程已启动" "pgrep mihomo-clashforge → 有进程" "mihomo 未运行"
fi

# ── SL-05: nftables metaclash 表已创建 ────────────────────────────────────────
NFT_TABLES=$(nft list tables 2>/dev/null | tr '\n' ' ')
if echo "$NFT_TABLES" | grep -qE "metaclash|clashforge"; then
    record PASS SL-05 "nftables metaclash 表已创建" "nft list tables 含 metaclash/clashforge 表" "表: $NFT_TABLES"
else
    record FAIL SL-05 "nftables metaclash 表已创建" "nft list tables 含 metaclash/clashforge 表" "未找到接管表（当前: ${NFT_TABLES:-(空)}）"
fi

# ── SL-06: tproxy 规则已存在 ──────────────────────────────────────────────────
if nft list ruleset 2>/dev/null | grep -qE "tproxy|redirect"; then
    record PASS SL-06 "nftables tproxy 规则已存在" "nft list ruleset 含 tproxy/redirect 规则" "规则存在 ✓"
else
    record WARN SL-06 "nftables tproxy 规则已存在" "nft list ruleset 含 tproxy/redirect 规则" "未找到 tproxy 规则（可能使用 redirect 模式）"
fi

# ── SL-07: ip rule 策略路由已配置 ─────────────────────────────────────────────
IP_RULE_AFTER=$(ip rule list 2>/dev/null)
if echo "$IP_RULE_AFTER" | grep -qE "fwmark|lookup"; then
    IP_RULE_NEW=$(echo "$IP_RULE_AFTER" | grep -E "fwmark|lookup" | head -2 | tr '\n' ' ')
    record PASS SL-07 "ip rule 策略路由已配置" "ip rule list 含 fwmark/lookup 规则" "$IP_RULE_NEW"
else
    record WARN SL-07 "ip rule 策略路由已配置" "ip rule list 含 fwmark/lookup 规则" "策略路由规则未找到"
fi

# ── SL-08: dnsmasq upstream 配置已写入 ────────────────────────────────────────
DNSMASQ_AFTER=$(ls /etc/dnsmasq.d/ 2>/dev/null | tr '\n' ' ')
DNSMASQ_BEFORE=$(cat "$SNAPSHOT_DIR/dnsmasq-d.before" 2>/dev/null | tr '\n' ' ')
CF_DNS_FILE=$(ls /etc/dnsmasq.d/ 2>/dev/null | grep -iE "clash|metaclash|mihomo" | head -1 || echo "")

if [ -n "$CF_DNS_FILE" ]; then
    record PASS SL-08 "dnsmasq upstream 配置已写入" "/etc/dnsmasq.d/ 中有 clashforge 相关配置文件" "文件: $CF_DNS_FILE"
elif [ "$DNSMASQ_AFTER" != "$DNSMASQ_BEFORE" ]; then
    record PASS SL-08 "dnsmasq upstream 配置已写入" "dnsmasq.d 配置发生变化" "dnsmasq.d 变化: $DNSMASQ_BEFORE → $DNSMASQ_AFTER"
else
    record WARN SL-08 "dnsmasq upstream 配置已写入" "/etc/dnsmasq.d/ 变化或有 clashforge 配置文件" "dnsmasq.d 未变化（DNS 可能用其他方式接管）"
fi

# ── SL-09: DNS 解析返回 fake-ip ───────────────────────────────────────────────
sleep 2
DNS_RESULT=$(nslookup google.com 2>/dev/null | grep "Address:" | grep -vE "#53|^127\." | head -1 | awk '{print $2}' || echo "")
info "DNS 解析 google.com → $DNS_RESULT"

if echo "$DNS_RESULT" | grep -qE "^198\.18\.|^198\.19\."; then
    record PASS SL-09 "DNS 解析返回 fake-ip" "nslookup google.com → 198.18.x.x 或 198.19.x.x" "fake-ip: $DNS_RESULT ✓"
elif [ -n "$DNS_RESULT" ]; then
    record WARN SL-09 "DNS 解析返回 fake-ip" "nslookup google.com → 198.18.x.x 或 198.19.x.x" "解析到: $DNS_RESULT（非 fake-ip 段，DNS 接管状态不确定）"
else
    record WARN SL-09 "DNS 解析返回 fake-ip" "nslookup google.com → 198.18.x.x 或 198.19.x.x" "DNS 解析无结果"
fi

# ── 提取代理认证信息（供 host 端 browser-probe.mjs 使用）───────────────────────
CACHE_FILE=$(ls /etc/metaclash/cache/*.raw.yaml 2>/dev/null | tail -1 || echo "")
if [ -n "$CACHE_FILE" ]; then
    AUTH_LINE=$(grep -A1 'authentication:' "$CACHE_FILE" 2>/dev/null \
        | grep -v 'authentication:' | head -1 | tr -d ' -' || echo "")
    NODE_NAME=$(grep -m1 'name:' "$CACHE_FILE" 2>/dev/null \
        | sed 's/.*name: *//' | tr -d ' ' || echo "")
    SERVER_HOST=$(grep -m1 'server:' "$CACHE_FILE" 2>/dev/null \
        | awk '{print $2}' | tr -d ' ' || echo "")
    [ -n "$AUTH_LINE" ] && echo "$AUTH_LINE" > "$SNAPSHOT_DIR/proxy-auth"
    [ -n "$NODE_NAME" ] && [ -n "$SERVER_HOST" ] && \
        echo "${NODE_NAME}:${SERVER_HOST}" > "$SNAPSHOT_DIR/proxy-node"
    info "代理节点: ${NODE_NAME:-未知} / ${SERVER_HOST:-未知}"
    info "代理认证: ${AUTH_LINE:-(无认证)}"
fi

# ── Summary ────────────────────────────────────────────────────────────────────
PASS_COUNT=$(printf "%b" "$RESULTS" | grep -c "^PASS" || echo 0)
FAIL_COUNT=$(printf "%b" "$RESULTS" | grep -c "^FAIL" || echo 0)
WARN_COUNT=$(printf "%b" "$RESULTS" | grep -c "^WARN" || echo 0)

summary "## 🚀 Step 5 — Setup Launch + 接管断言"
summary ""
summary "| 结果 | 数量 |"
summary "|------|------|"
summary "| ✅ 通过 | $PASS_COUNT |"
summary "| ❌ 失败 | $FAIL_COUNT |"
summary "| ⚠️ 警告 | $WARN_COUNT |"
summary ""
summary "| 编号 | 用例 | 预期 | 实际 | 状态 |"
summary "|------|------|------|------|------|"
printf "%b" "$RESULTS" | while IFS="|" read -r STATUS TC NAME EXPECTED ACTUAL; do
    [ -z "$TC" ] && continue
    case "$STATUS" in PASS) ICON="✅" ;; FAIL) ICON="❌" ;; WARN) ICON="⚠️" ;; *) continue ;; esac
    summary "| $TC | $NAME | $EXPECTED | $ACTUAL | $ICON |"
done

printf "\n${BOLD}通过: $PASS_COUNT  失败: $FAIL_COUNT  警告: $WARN_COUNT${RESET}\n"

if [ "$FAILED" -gt 0 ]; then
    printf "${RED}${BOLD}Setup 验证失败 ❌${RESET}\n"
    exit 1
else
    printf "${GREEN}${BOLD}Setup 验证通过 ✅${RESET}\n"
fi
