#!/bin/sh
# tests/e2e/browser-probe-router.sh
# 浏览器端探测 - 在 OpenWrt VM 内运行版本
#
# 模拟「用户设备通过路由器上网」的场景：
#   - 在 OpenWrt VM 内直接发请求
#   - 流量被 ClashForge tproxy 透明拦截（不需要设置代理）
#   - 对比：ClashForge 运行中 vs 停止后的出口 IP 和可访问性
#
# 与宿主机版本的区别：
#   - 不依赖宿主机的网络能力，消除宿主机直连导致的误判
#   - 流量强制经过 tproxy，无法绕开
#   - 出口 IP 变化才能证明代理生效
#
# 环境变量：
#   PHASE              running（代理运行期）或 stopped（停止后）
#   DIRECT_IP          启动前的直连 IP（用于对比）
#   PROXY_SERVER_IP    代理节点服务器真实 IP（用于节点匹配验证）
#   PROXY_NODE_NAME    代理节点名称
#   GITHUB_STEP_SUMMARY  GitHub Actions job summary 路径

set -e

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

FAILED=0
RESULTS=""
TOTAL=0

record() {
    STATUS="$1"; TC="$2"; NAME="$3"; OP="$4"; EXPECTED="$5"; ACTUAL="$6"
    RESULTS="${RESULTS}${STATUS}|${TC}|${NAME}|${OP}|${EXPECTED}|${ACTUAL}\n"
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

PHASE="${PHASE:-running}"
DIRECT_IP="${DIRECT_IP:-unknown}"
PROXY_SERVER_IP="${PROXY_SERVER_IP:-}"
PROXY_NODE_NAME="${PROXY_NODE_NAME:-}"

LABEL="代理运行期"
[ "$PHASE" = "stopped" ] && LABEL="停止后恢复验证"

section "浏览器端探测（VM 内直连 · $LABEL）"
info "流量走 tproxy 透明拦截，无代理设置"

# ── IP 检查（不设置代理，流量被 tproxy 拦截）────────────────────────────────

# BV-01 IP 检查（直连 + tproxy）
CURRENT_IP=$(curl -sf --max-time 10 https://api.ipify.org 2>/dev/null || echo "FAILED")
info "当前出口 IP: $CURRENT_IP"

if [ "$PHASE" = "running" ]; then
    # 代理运行期：IP 应该变为代理节点出口
    if [ "$CURRENT_IP" = "FAILED" ]; then
        record FAIL BV-01 "IP 检查（代理运行期）" \
            "VM 内直连请求 api.ipify.org（流量被 tproxy 拦截）" \
            "出口 IP 为代理节点 IP，不同于直连 IP" \
            "请求失败"
    elif [ "$CURRENT_IP" != "$DIRECT_IP" ]; then
        # 进一步验证：是否等于代理节点 IP
        if [ -n "$PROXY_SERVER_IP" ] && [ "$CURRENT_IP" = "$PROXY_SERVER_IP" ]; then
            record PASS BV-01 "IP 检查（代理运行期）" \
                "VM 内直连请求 api.ipify.org（流量被 tproxy 拦截）" \
                "出口 IP = 代理节点 IP ≠ 直连 IP" \
                "直连:$DIRECT_IP → 当前:$CURRENT_IP = 节点[$PROXY_NODE_NAME] $PROXY_SERVER_IP ✓"
        elif [ -n "$PROXY_SERVER_IP" ]; then
            record WARN BV-01 "IP 检查（代理运行期）" \
                "VM 内直连请求 api.ipify.org（流量被 tproxy 拦截）" \
                "出口 IP = 代理节点 IP ≠ 直连 IP" \
                "IP 已变化($DIRECT_IP→$CURRENT_IP)，但与节点IP($PROXY_SERVER_IP)不匹配"
        else
            record PASS BV-01 "IP 检查（代理运行期）" \
                "VM 内直连请求 api.ipify.org（流量被 tproxy 拦截）" \
                "出口 IP ≠ 直连 IP（流量走代理）" \
                "直连:$DIRECT_IP → 当前:$CURRENT_IP（已变化）"
        fi
    else
        record FAIL BV-01 "IP 检查（代理运行期）" \
            "VM 内直连请求 api.ipify.org（流量被 tproxy 拦截）" \
            "出口 IP ≠ 直连 IP（代理应已生效）" \
            "IP 未变化: $CURRENT_IP = $DIRECT_IP （代理未生效或流量未被拦截）"
    fi
else
    # 停止后：IP 应该还原为直连 IP
    if [ "$CURRENT_IP" = "$DIRECT_IP" ]; then
        record PASS BV-01 "IP 检查（停止后恢复）" \
            "VM 内直连请求 api.ipify.org（已停止 ClashForge）" \
            "出口 IP 还原为直连 IP" \
            "IP 已还原: $CURRENT_IP = $DIRECT_IP ✓"
    elif [ "$CURRENT_IP" = "FAILED" ]; then
        record FAIL BV-01 "IP 检查（停止后恢复）" \
            "VM 内直连请求 api.ipify.org（已停止 ClashForge）" \
            "网络正常，出口 IP 还原" \
            "网络不可达！ClashForge 停止后网络未恢复"
    else
        record WARN BV-01 "IP 检查（停止后恢复）" \
            "VM 内直连请求 api.ipify.org（已停止 ClashForge）" \
            "出口 IP 还原为直连 IP" \
            "IP 未还原: $CURRENT_IP ≠ $DIRECT_IP"
    fi
fi

# ── BV-02 可访问性检查 ────────────────────────────────────────────────────────
section "可访问性检查（$LABEL）"

ACCESS_TARGETS="https://www.baidu.com https://music.163.com https://github.com https://www.youtube.com"
ACCESS_NAMES="百度搜索 网易云音乐 GitHub YouTube"
ACCESS_GROUPS="国内 国内 国际 国际"

OK_COUNT=0; TOTAL_ACCESS=0; DETAILS=""
i=1
for url in $ACCESS_TARGETS; do
    TOTAL_ACCESS=$((TOTAL_ACCESS+1))
    NAME=$(echo "$ACCESS_NAMES" | cut -d' ' -f$i)
    GROUP=$(echo "$ACCESS_GROUPS" | cut -d' ' -f$i)
    i=$((i+1))
    STARTED=$(date +%s)
    CODE=$(curl -sf -o /dev/null -w "%{http_code}" --max-time 10 "$url" 2>/dev/null || echo "0")
    ENDED=$(date +%s)
    LATENCY=$(( (ENDED-STARTED)*1000 ))
    if [ "$CODE" -ge 200 ] 2>/dev/null && [ "$CODE" -lt 400 ] 2>/dev/null; then
        OK_COUNT=$((OK_COUNT+1))
        DETAILS="${DETAILS}✓ $NAME HTTP $CODE ${LATENCY}ms | "
        info "  ✓ $NAME [$GROUP] HTTP $CODE (${LATENCY}ms)"
    else
        DETAILS="${DETAILS}✗ $NAME HTTP $CODE | "
        info "  ✗ $NAME [$GROUP] HTTP $CODE"
    fi
done

DETAILS=$(echo "$DETAILS" | sed 's/ | $//')

if [ "$PHASE" = "running" ]; then
    # 代理运行期：国际站点应通过代理可达，国内应正常
    if [ "$OK_COUNT" = "$TOTAL_ACCESS" ]; then
        record PASS BV-02 "可访问性检查（代理运行期）" \
            "VM 内直连访问百度/网易云/GitHub/YouTube（流量被 tproxy 拦截）" \
            "所有站点可达（国内直连 + 国际走代理）" \
            "$OK_COUNT/$TOTAL_ACCESS 成功: $DETAILS"
    elif [ "$OK_COUNT" -gt 0 ]; then
        record WARN BV-02 "可访问性检查（代理运行期）" \
            "VM 内直连访问百度/网易云/GitHub/YouTube" \
            "所有站点可达" \
            "$OK_COUNT/$TOTAL_ACCESS 成功: $DETAILS"
    else
        record FAIL BV-02 "可访问性检查（代理运行期）" \
            "VM 内直连访问百度/网易云/GitHub/YouTube" \
            "至少部分站点可达" \
            "全部失败: $DETAILS"
    fi
else
    # 停止后：至少国内站点应正常
    if [ "$OK_COUNT" -gt 0 ]; then
        record PASS BV-02 "可访问性检查（停止后恢复）" \
            "VM 内直连访问站点（ClashForge 已停止）" \
            "至少部分站点直连可达" \
            "$OK_COUNT/$TOTAL_ACCESS 成功: $DETAILS"
    else
        record FAIL BV-02 "可访问性检查（停止后恢复）" \
            "VM 内直连访问站点（ClashForge 已停止）" \
            "至少部分站点直连可达" \
            "全部失败（网络可能未恢复）"
    fi
fi

# ── Summary ────────────────────────────────────────────────────────────────────
PASS_COUNT=$(printf "%b" "$RESULTS" | grep -c "^PASS" || echo 0)
FAIL_COUNT=$(printf "%b" "$RESULTS" | grep -c "^FAIL" || echo 0)
WARN_COUNT=$(printf "%b" "$RESULTS" | grep -c "^WARN" || echo 0)

summary "## 🖥️ 浏览器端探测（VM 内 · $LABEL）"
summary ""
summary "| 项目 | 值 |"
summary "|------|----|"
summary "| **测试模式** | VM 内透明拦截（tproxy） |"
summary "| **Phase** | $LABEL |"
summary "| **直连 IP 基准** | $DIRECT_IP |"
summary "| **当前出口 IP** | $CURRENT_IP |"
[ -n "$PROXY_SERVER_IP" ] && summary "| **节点服务器 IP** | $PROXY_SERVER_IP |"
summary ""
summary "| 结果 | 数量 |"
summary "|------|------|"
summary "| ✅ 通过 | $PASS_COUNT |"
summary "| ❌ 失败 | $FAIL_COUNT |"
summary "| ⚠️ 警告 | $WARN_COUNT |"
summary ""
summary "| 编号 | 用例名称 | 操作 | 预期结果 | 实际结果 | 状态 |"
summary "|------|----------|------|----------|----------|------|"
printf "%b" "$RESULTS" | while IFS="|" read -r STATUS TC NAME OP EXPECTED ACTUAL; do
    [ -z "$TC" ] && continue
    case "$STATUS" in PASS) ICON="✅" ;; FAIL) ICON="❌" ;; WARN) ICON="⚠️" ;; *) continue ;; esac
    summary "| $TC | $NAME | $OP | $EXPECTED | $ACTUAL | $ICON $STATUS |"
done
summary ""

if [ "$FAILED" -eq 0 ]; then
    printf "${GREEN}${BOLD}BROWSER VM PROBE PASSED ✅${RESET}\n"
    exit 0
else
    printf "${RED}${BOLD}$FAILED 个测试失败 ❌${RESET}\n"
    exit 1
fi
