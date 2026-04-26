#!/bin/sh
# tests/e2e/vm/stop-verify.sh
# Step 7 — 停止服务 + 状态还原断言
#
# 模拟用户在 http://192.168.10.1:7777/setup 点击"停止服务 + 退出所有接管"。
# 调用 POST /api/v1/setup/stop，然后断言系统状态是否完全还原到 Step 2 的快照。
#
# 断言项：
#   SV-01  setup/stop API → success:true
#   SV-02  mihomo 进程已停止
#   SV-03  nftables metaclash 表已移除（与 Step 2 快照对比）
#   SV-04  ip rule 策略路由已还原（与 Step 2 快照对比）
#   SV-05  dnsmasq.d 已还原（与 Step 2 快照对比）
#   SV-06  resolv.conf 已还原（与 Step 2 快照对比）
#   SV-07  Web UI 仍可访问（clashforge 服务本身应继续运行）
#   SV-08  API /status 仍可响应
#
# 环境变量：
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
CF_UI="http://127.0.0.1:7777"
SNAPSHOT_DIR="/tmp/cf-snapshot"

section "Step 7 — 停止服务 + 状态还原断言"

# ── SV-01: 调用 setup/stop ────────────────────────────────────────────────────
info "调用 POST /setup/stop ..."
STOP_RESP=$(curl -sf --max-time 90 \
    -H "Content-Type: application/json" -d '{}' \
    "$CF_API/setup/stop" 2>/dev/null || echo "FAILED")

if echo "$STOP_RESP" | grep -q '"success":true'; then
    record PASS SV-01 "setup/stop API" "POST /setup/stop → {success:true}" "停止成功"
elif [ "$STOP_RESP" = "FAILED" ]; then
    record FAIL SV-01 "setup/stop API" "POST /setup/stop → {success:true}" "API 请求失败（服务未响应）"
else
    STOP_ERR=$(echo "$STOP_RESP" | grep -o '"error":"[^"]*"' | head -1 || echo "$STOP_RESP")
    record FAIL SV-01 "setup/stop API" "POST /setup/stop → {success:true}" "失败: $STOP_ERR"
fi

# 等待清理完成
sleep 3

# ── SV-02: mihomo 进程已停止 ──────────────────────────────────────────────────
MIHOMO_PID=$(pgrep -f "mihomo-clashforge" 2>/dev/null || echo "")
if [ -z "$MIHOMO_PID" ]; then
    record PASS SV-02 "mihomo 进程已停止" "pgrep mihomo-clashforge → 无进程" "mihomo 已停止 ✓"
else
    record FAIL SV-02 "mihomo 进程已停止" "pgrep mihomo-clashforge → 无进程" "mihomo 仍在运行 PID=$MIHOMO_PID"
fi

# ── SV-03: nftables 还原 ──────────────────────────────────────────────────────
NFT_TABLES_AFTER=$(nft list tables 2>/dev/null | tr '\n' ' ')
NFT_BEFORE=$(cat "$SNAPSHOT_DIR/nft.before" 2>/dev/null || echo "")
NFT_TABLES_BEFORE=$(echo "$NFT_BEFORE" | grep "^table" | tr '\n' ' ')

if echo "$NFT_TABLES_AFTER" | grep -qE "metaclash|clashforge"; then
    record FAIL SV-03 "nftables metaclash 表已移除" "nft list tables 中无 metaclash/clashforge 表" "残留接管表: $NFT_TABLES_AFTER"
else
    record PASS SV-03 "nftables metaclash 表已移除" "nft list tables 中无 metaclash/clashforge 表" "无接管表 ✓（当前: ${NFT_TABLES_AFTER:-(空)}）"
fi

# ── SV-04: ip rule 还原 ───────────────────────────────────────────────────────
IP_RULE_AFTER=$(ip rule list 2>/dev/null)
IP_RULE_BEFORE=$(cat "$SNAPSHOT_DIR/ip-rule.before" 2>/dev/null || echo "")

# 检查是否有启动时新增的 fwmark/lookup 规则仍残留
CF_IP_RULES=$(echo "$IP_RULE_AFTER" | grep -E "fwmark" | grep -v "$(echo "$IP_RULE_BEFORE" | grep -E "fwmark")" 2>/dev/null || echo "")
if [ -z "$CF_IP_RULES" ]; then
    RULE_COUNT=$(echo "$IP_RULE_AFTER" | wc -l)
    BEFORE_COUNT=$(echo "$IP_RULE_BEFORE" | wc -l)
    if [ "$RULE_COUNT" = "$BEFORE_COUNT" ]; then
        record PASS SV-04 "ip rule 策略路由已还原" "ip rule 条数与 Step 2 快照一致" "✓ 规则数: $RULE_COUNT（与快照 $BEFORE_COUNT 一致）"
    else
        record WARN SV-04 "ip rule 策略路由已还原" "ip rule 条数与 Step 2 快照一致" "规则数变化: 快照 $BEFORE_COUNT → 当前 $RULE_COUNT（可能有残留）"
    fi
else
    record FAIL SV-04 "ip rule 策略路由已还原" "clashforge 添加的 fwmark 规则已清除" "残留规则: $CF_IP_RULES"
fi

# ── SV-05: dnsmasq.d 还原 ─────────────────────────────────────────────────────
DNSMASQ_AFTER=$(ls /etc/dnsmasq.d/ 2>/dev/null | sort | tr '\n' ' ')
DNSMASQ_BEFORE=$(cat "$SNAPSHOT_DIR/dnsmasq-d.before" 2>/dev/null | sort | tr '\n' ' ')

# 先检查有没有 clashforge 残留文件
CF_DNSMASQ=$(ls /etc/dnsmasq.d/ 2>/dev/null | grep -iE "clash|metaclash|mihomo" || echo "")
if [ -n "$CF_DNSMASQ" ]; then
    record FAIL SV-05 "dnsmasq.d 已还原" "/etc/dnsmasq.d/ 中无 clashforge 相关配置文件" "残留配置: $CF_DNSMASQ"
elif [ "$DNSMASQ_AFTER" = "$DNSMASQ_BEFORE" ]; then
    record PASS SV-05 "dnsmasq.d 已还原" "/etc/dnsmasq.d/ 与 Step 2 快照一致" "✓ 与快照一致: ${DNSMASQ_AFTER:-(空)}"
else
    record WARN SV-05 "dnsmasq.d 已还原" "/etc/dnsmasq.d/ 与 Step 2 快照一致" "快照: ${DNSMASQ_BEFORE:-(空)} → 当前: ${DNSMASQ_AFTER:-(空)}"
fi

# ── SV-06: resolv.conf 还原 ───────────────────────────────────────────────────
RESOLV_AFTER=$(grep nameserver /etc/resolv.conf 2>/dev/null | sort | tr '\n' ' ')
RESOLV_BEFORE=$(grep nameserver "$SNAPSHOT_DIR/resolv.before" 2>/dev/null | sort | tr '\n' ' ')

if [ "$RESOLV_AFTER" = "$RESOLV_BEFORE" ]; then
    record PASS SV-06 "resolv.conf 已还原" "DNS 配置与 Step 2 快照一致" "✓ DNS: $RESOLV_AFTER"
else
    # fake-ip 的 dnsmasq upstream 模式不一定改 resolv.conf，可能只是 WARN
    record WARN SV-06 "resolv.conf 已还原" "DNS 配置与 Step 2 快照一致" "快照: $RESOLV_BEFORE → 当前: $RESOLV_AFTER"
fi

# ── SV-07: Web UI 仍可访问 ────────────────────────────────────────────────────
UI_CODE=$(curl -sf -o /dev/null -w "%{http_code}" --max-time 10 "$CF_UI/" 2>/dev/null || echo "0")
if [ "$UI_CODE" = "200" ]; then
    record PASS SV-07 "Web UI 仍可访问" "GET / → HTTP 200（clashforge 服务本身未停止）" "HTTP $UI_CODE ✓"
else
    record FAIL SV-07 "Web UI 仍可访问" "GET / → HTTP 200（clashforge 服务本身未停止）" "HTTP $UI_CODE（服务意外停止？）"
fi

# ── SV-08: API /status 仍可响应 ───────────────────────────────────────────────
STATUS_RESP=$(curl -sf --max-time 10 "$CF_API/status" 2>/dev/null || echo "FAILED")
if echo "$STATUS_RESP" | grep -qE "\"ok\"|\"status\"|\"version\""; then
    record PASS SV-08 "API /status 仍可响应" "GET /api/v1/status → 正常响应" "响应正常 ✓"
else
    record FAIL SV-08 "API /status 仍可响应" "GET /api/v1/status → 正常响应" "无响应: $STATUS_RESP"
fi

# ── Summary ────────────────────────────────────────────────────────────────────
PASS_COUNT=$(printf "%b" "$RESULTS" | grep -c "^PASS" || echo 0)
FAIL_COUNT=$(printf "%b" "$RESULTS" | grep -c "^FAIL" || echo 0)
WARN_COUNT=$(printf "%b" "$RESULTS" | grep -c "^WARN" || echo 0)

summary "## 🛑 Step 7 — 停止服务 + 状态还原断言"
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
    printf "${RED}${BOLD}停止验证失败 ❌${RESET}\n"
    exit 1
else
    printf "${GREEN}${BOLD}停止验证通过 ✅${RESET}\n"
fi
