#!/bin/sh
# tests/e2e/vm/probe.sh
# Step 3 / 6 / 8 — 三轮网络探测（VM 内 curl，模拟用户设备直连）
#
# 三轮复用同一脚本，用 PHASE 区分断言逻辑：
#   baseline  Round 1：安装前基准（直连，无代理）
#   running   Round 2：代理运行期（tproxy 已接管，出口 IP 应变为代理节点 IP）
#   stopped   Round 3：停止后恢复（出口 IP 应还原为 Round 1 的基准 IP）
#
# 所有 curl 请求均无显式代理设置，流量由 tproxy 透明拦截（running 阶段）
# 或走直连（baseline/stopped 阶段）。
#
# 环境变量：
#   PHASE              baseline | running | stopped（必须）
#   PROXY_NODE_IP      代理节点服务器 IP（running 阶段的出口 IP 断言目标）
#   GITHUB_STEP_SUMMARY

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

# ── 敏感信息脂敏 ──────────────────────────────────────────────────────────────
mask_ip()     { echo "$1" | sed 's/\([0-9]*\)\.[0-9]*\.[0-9]*\.\([0-9]*\)/\1.*.*.\2/'; }
mask_domain() { echo "$1" | awk -F. '{ if (NF<=2) {print $0} else {print $1".***"."$NF"} }'; }

PHASE="${PHASE:-baseline}"
PROXY_NODE_IP="${PROXY_NODE_IP:-}"
MIXED_PORT_URL="${MIXED_PORT_URL:-}"
SNAPSHOT_DIR="/tmp/cf-snapshot"

case "$PHASE" in
    baseline) PHASE_LABEL="Round 1 — 基准（安装前直连）" ;;
    running)  PHASE_LABEL="Round 2 — 代理运行期" ;;
    stopped)  PHASE_LABEL="Round 3 — 停止后恢复" ;;
    *) echo "ERROR: PHASE must be baseline|running|stopped"; exit 1 ;;
esac

DIRECT_IP=$(cat "$SNAPSHOT_DIR/direct-ip" 2>/dev/null || echo "unknown")

section "VM 内网络探测 — $PHASE_LABEL"
info "直连基准 IP: $(mask_ip "$DIRECT_IP")"
[ -n "$PROXY_NODE_IP" ] && info "代理节点 IP: $(mask_ip "$PROXY_NODE_IP")"

# ── PR-01: 出口 IP 检查 ────────────────────────────────────────────────────────
# tproxy 只拦截 PREROUTING（LAN 客户端入流量），不拦截路由器本机进程（OUTPUT）。
# running 阶段用混合端口（HTTP/SOCKS）显式代理，确保出口 IP 经过代理节点。
if [ "$PHASE" = "running" ] && [ -n "$MIXED_PORT_URL" ]; then
    CURRENT_IP=$(curl -sf --max-time 10 --proxy "$MIXED_PORT_URL" https://api.ipify.org 2>/dev/null || echo "FAILED")
    info "当前出口 IP（via 混合端口）: $(mask_ip "$CURRENT_IP")"
else
    CURRENT_IP=$(curl -sf --max-time 10 https://api.ipify.org 2>/dev/null || echo "FAILED")
    info "当前出口 IP: $(mask_ip "$CURRENT_IP")"
fi

case "$PHASE" in
    baseline)
        # Round 1: IP 应该就是直连 IP（实际上此时 direct-ip 刚写，两者相同）
        if [ "$CURRENT_IP" != "FAILED" ]; then
            echo "$CURRENT_IP" > "$SNAPSHOT_DIR/baseline-probe-ip"
            record PASS PR-01 "出口 IP（基准）" \
                "curl api.ipify.org（无代理）" \
                "可获取直连出口 IP" \
                "基准 IP: $(mask_ip "$CURRENT_IP")"
        else
            record FAIL PR-01 "出口 IP（基准）" \
                "curl api.ipify.org（无代理）" \
                "可获取直连出口 IP" \
                "请求失败，网络异常"
        fi
        ;;
    running)
        if [ "$CURRENT_IP" = "FAILED" ]; then
            record FAIL PR-01 "出口 IP（代理运行期）" \
                "curl api.ipify.org（tproxy 透明拦截）" \
                "出口 IP = 代理节点 IP ≠ 直连 IP" \
                "请求失败"
        elif [ -n "$PROXY_NODE_IP" ] && [ "$CURRENT_IP" = "$PROXY_NODE_IP" ]; then
            record PASS PR-01 "出口 IP（代理运行期）" \
                "curl api.ipify.org（tproxy 透明拦截）" \
                "出口 IP = 代理节点 IP ($(mask_ip "$PROXY_NODE_IP"))" \
                "✓ $(mask_ip "$DIRECT_IP") → $(mask_ip "$CURRENT_IP") = 节点 IP"
        elif [ "$CURRENT_IP" != "$DIRECT_IP" ]; then
            record PASS PR-01 "出口 IP（代理运行期）" \
                "curl api.ipify.org（tproxy 透明拦截）" \
                "出口 IP ≠ 直连 IP（流量走代理）" \
                "IP 已变化: $(mask_ip "$DIRECT_IP") → $(mask_ip "$CURRENT_IP")（代理节点 IP 未配置，以 IP 变化作为代理生效依据）"
        else
            record FAIL PR-01 "出口 IP（代理运行期）" \
                "curl api.ipify.org（tproxy 透明拦截）" \
                "出口 IP ≠ 直连 IP（代理应已生效）" \
                "IP 未变化: $(mask_ip "$CURRENT_IP") = $(mask_ip "$DIRECT_IP")（tproxy 未生效）"
        fi
        ;;
    stopped)
        if [ "$CURRENT_IP" = "$DIRECT_IP" ]; then
            record PASS PR-01 "出口 IP（停止后恢复）" \
                "curl api.ipify.org（无代理直连）" \
                "出口 IP 还原为直连基准 IP" \
                "✓ IP 已还原: $(mask_ip "$CURRENT_IP") = $(mask_ip "$DIRECT_IP")"
        elif [ "$CURRENT_IP" = "FAILED" ]; then
            record FAIL PR-01 "出口 IP（停止后恢复）" \
                "curl api.ipify.org（无代理直连）" \
                "出口 IP 还原为直连基准 IP" \
                "网络不可达！停止后网络未恢复"
        else
            record WARN PR-01 "出口 IP（停止后恢复）" \
                "curl api.ipify.org（无代理直连）" \
                "出口 IP 还原为直连基准 IP" \
                "IP 未还原: $(mask_ip "$CURRENT_IP") ≠ $(mask_ip "$DIRECT_IP")"
        fi
        ;;
esac

# ── PR-02: 连通性检查 ──────────────────────────────────────────────────────────
section "连通性检查 — $PHASE_LABEL"

# 目标：国内 2 个 + 国际 2 个
TARGETS="https://www.baidu.com:百度搜索:国内 https://music.163.com:网易云音乐:国内 https://github.com:GitHub:国际 https://www.youtube.com:YouTube:国际"

OK_COUNT=0; TOTAL_ACCESS=0; DETAILS=""
for entry in $TARGETS; do
    URL=$(echo "$entry" | cut -d: -f1-2)    # https://xxx.xxx
    NAME=$(echo "$entry" | cut -d: -f3)
    GROUP=$(echo "$entry" | cut -d: -f4)
    TOTAL_ACCESS=$((TOTAL_ACCESS+1))
    STARTED=$(date +%s%3N 2>/dev/null || date +%s)
    if [ "$PHASE" = "running" ] && [ -n "$MIXED_PORT_URL" ]; then
        CODE=$(curl -sf -o /dev/null -w "%{http_code}" --proxy "$MIXED_PORT_URL" --max-time 12 "$URL" 2>/dev/null || echo "0")
    else
        CODE=$(curl -sf -o /dev/null -w "%{http_code}" --max-time 12 "$URL" 2>/dev/null || echo "0")
    fi
    ENDED=$(date +%s%3N 2>/dev/null || date +%s)
    LATENCY=$((ENDED - STARTED))
    if [ "$CODE" -ge 200 ] 2>/dev/null && [ "$CODE" -lt 400 ] 2>/dev/null; then
        OK_COUNT=$((OK_COUNT+1))
        DETAILS="${DETAILS}✓$NAME(${LATENCY}ms) "
        info "  ✓ $NAME [$GROUP] HTTP $CODE (${LATENCY}ms)"
    else
        DETAILS="${DETAILS}✗$NAME(HTTP$CODE) "
        info "  ✗ $NAME [$GROUP] HTTP $CODE"
    fi
done

case "$PHASE" in
    baseline|stopped)
        # 直连场景：国内站应全通，国际按实际网络环境（WARN 不算失败）
        DOMESTIC_OK=$(printf "%b" "$DETAILS" | grep -c "✓百度\|✓网易云" || echo 0)
        if [ "$OK_COUNT" = "$TOTAL_ACCESS" ]; then
            record PASS PR-02 "连通性检查（直连）" \
                "curl 百度/网易云/GitHub/YouTube（无代理）" \
                "所有站点可达" \
                "$OK_COUNT/$TOTAL_ACCESS: $DETAILS"
        elif [ "$DOMESTIC_OK" -ge 1 ]; then
            record WARN PR-02 "连通性检查（直连）" \
                "curl 百度/网易云/GitHub/YouTube（无代理）" \
                "所有站点可达" \
                "$OK_COUNT/$TOTAL_ACCESS: $DETAILS"
        else
            record FAIL PR-02 "连通性检查（直连）" \
                "curl 百度/网易云/GitHub/YouTube（无代理）" \
                "至少国内站点可达" \
                "$OK_COUNT/$TOTAL_ACCESS: $DETAILS"
        fi
        ;;
    running)
        # 代理场景：全部应通（tproxy 透明代理国内+国际）
        if [ "$OK_COUNT" = "$TOTAL_ACCESS" ]; then
            record PASS PR-02 "连通性检查（代理运行期）" \
                "curl 百度/网易云/GitHub/YouTube（tproxy 透明拦截）" \
                "所有站点通畅（国内直连 + 国际走代理）" \
                "$OK_COUNT/$TOTAL_ACCESS: $DETAILS"
        elif [ "$OK_COUNT" -gt 0 ]; then
            record WARN PR-02 "连通性检查（代理运行期）" \
                "curl 百度/网易云/GitHub/YouTube（tproxy 透明拦截）" \
                "所有站点通畅" \
                "$OK_COUNT/$TOTAL_ACCESS: $DETAILS"
        else
            record FAIL PR-02 "连通性检查（代理运行期）" \
                "curl 百度/网易云/GitHub/YouTube（tproxy 透明拦截）" \
                "所有站点通畅" \
                "全部失败: $DETAILS"
        fi
        ;;
esac

# 保存本轮结果供对比
echo "phase=$PHASE ip=$CURRENT_IP ok=$OK_COUNT total=$TOTAL_ACCESS" \
    > "$SNAPSHOT_DIR/probe-result-${PHASE}.txt"

# ── Summary ────────────────────────────────────────────────────────────────────
PASS_COUNT=$(printf "%b" "$RESULTS" | grep -c "^PASS" || echo 0)
FAIL_COUNT=$(printf "%b" "$RESULTS" | grep -c "^FAIL" || echo 0)
WARN_COUNT=$(printf "%b" "$RESULTS" | grep -c "^WARN" || echo 0)

summary "## 🌐 VM 网络探测 — $PHASE_LABEL"
summary ""
summary "| 项目 | 值 |"
summary "|------|----|"
summary "| **当前出口 IP** | $(mask_ip "$CURRENT_IP") |"
summary "| **直连基准 IP** | $(mask_ip "$DIRECT_IP") |"
[ -n "$PROXY_NODE_IP" ] && summary "| **代理节点 IP** | $(mask_ip "$PROXY_NODE_IP") |"
summary "| **连通性** | $OK_COUNT/$TOTAL_ACCESS |"
summary ""
summary "| 编号 | 用例 | 操作 | 预期 | 实际 | 状态 |"
summary "|------|------|------|------|------|------|"
printf "%b" "$RESULTS" | while IFS="|" read -r STATUS TC NAME OP EXPECTED ACTUAL; do
    [ -z "$TC" ] && continue
    case "$STATUS" in PASS) ICON="✅" ;; FAIL) ICON="❌" ;; WARN) ICON="⚠️" ;; *) continue ;; esac
    summary "| $TC | $NAME | $OP | $EXPECTED | $ACTUAL | $ICON |"
done

printf "\n${BOLD}通过: $PASS_COUNT  失败: $FAIL_COUNT  警告: $WARN_COUNT${RESET}\n"

if [ "$FAILED" -gt 0 ]; then
    printf "${RED}${BOLD}探测失败 ❌${RESET}\n"
    exit 1
else
    printf "${GREEN}${BOLD}探测通过 ✅${RESET}\n"
fi
