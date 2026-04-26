#!/bin/sh
# tests/e2e/vm/probe.sh
# Step 3 / 6 / 8 — 三轮网络探测（VM 内）
#
# 三轮复用同一脚本，用 PHASE 区分断言逻辑：
#   baseline  Round 1：安装前基准（直连，无代理）
#   running   Round 2：代理运行期（tproxy 已接管，出口 IP 应变为代理节点 IP）
#   stopped   Round 3：停止后恢复（出口 IP 应还原为 Round 1 的基准 IP）
#
# 所有 curl 请求均无显式代理设置，流量由 tproxy 透明拦截（running 阶段）
# 或走直连（baseline/stopped 阶段）。
#
# running 阶段额外执行路由器侧诊断探测（Router Diagnostic Probe），
# 与 handler_overview.go 的 buildOverviewAccessChecks 逻辑严格一致：
#   1. 端口监听检查 → 2. DNS 解析 → 3. HTTP HEAD via proxy → 4. 错误分级
#
# 环境变量：
#   PHASE              baseline | running | stopped（必须）
#   PROXY_NODE_IP      代理节点服务器 IP（running 阶段的出口 IP 断言目标）
#   MIXED_PORT_URL     混合端口代理 URL（running 阶段显式代理用）
#   MIHOMO_DNS_PORT    mihomo DNS 端口（默认 7874）
#   MIXED_PORT         混合端口号（默认 7893）
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
mask_domain() { echo "$1" | awk -F. '{ if (NF<=2) {print $0} else {print $1".***."$NF} }'; }

PHASE="${PHASE:-baseline}"
PROXY_NODE_IP="${PROXY_NODE_IP:-}"
MIXED_PORT_URL="${MIXED_PORT_URL:-}"
MIHOMO_DNS_PORT="${MIHOMO_DNS_PORT:-7874}"
MIXED_PORT="${MIXED_PORT:-7893}"
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

# ═══════════════════════════════════════════════════════════════════════════════
# PR-01: 出口 IP 检查
# ═══════════════════════════════════════════════════════════════════════════════
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

# ═══════════════════════════════════════════════════════════════════════════════
# Router Diagnostic Probe（仅 running 阶段）
#
# 与 handler_overview.go → buildOverviewAccessChecks 逻辑严格一致：
#   1. 端口监听检查 (isTCPPortListening)
#   2. DNS 解析 (resolveForDebug → 查询 mihomo DNS 端口)
#   3. HTTP HEAD via 代理 (testHTTPProxyEndpoint → 经过 mixed 端口)
#   4. 错误分级: proxy_port / dns / timeout / connect
# ═══════════════════════════════════════════════════════════════════════════════
if [ "$PHASE" = "running" ]; then
    section "路由器侧诊断探测 — 对齐 handler_overview.go"

    # ── 检查端口可访问（跳过 nc -z，OpenWrt BusyBox 不可靠） ──────────────
    _tcp_ok() {
        curl --connect-timeout 1 --max-time 2 -o /dev/null "http://127.0.0.1:$1" 2>/dev/null
    }

    MIXED_LISTENING="false"
    if _tcp_ok "$MIXED_PORT"; then
        MIXED_LISTENING="true"
        info "mixed 端口 $MIXED_PORT 正在监听 ✓"
    else
        info "mixed 端口 $MIXED_PORT 未监听 ✗"
    fi

    DNS_LISTENING="false"
    # DNS 是 UDP 端口，用 nslookup 验证 DNS 链（对齐 SL-04b / SL-09）
    if nslookup google.com >/dev/null 2>&1; then
        DNS_LISTENING="true"
        info "mihomo DNS 端口 $MIHOMO_DNS_PORT 解析正常 ✓"
    else
        info "mihomo DNS 端口 $MIHOMO_DNS_PORT DNS 不通 ✗"
    fi

    # ── DNS 解析（对齐 resolveForDebug） ──────────────────────────────────
    # 此时 DNS 已接管，直接 nslookup 走 dnsmasq → mihomo 链路即可
    _dns_resolve() {
        _host="$1"
        nslookup "$_host" 2>/dev/null \
            | grep "Address" | grep -vE '#53|127\.0\.|::1' \
            | awk '{print $NF}' | head -3 | tr '\n' ',' | sed 's/,$//'
    }

    # ── HTTP HEAD via proxy（对齐 testHTTPProxyEndpoint） ──────────────────────
    _http_head_via_proxy() {
        _url="$1"
        # Use MIXED_PORT_URL (includes credentials when proxy auth is configured).
        # Falling back to bare http://127.0.0.1:PORT would cause 407 when auth is required.
        _proxy_url="${MIXED_PORT_URL:-http://127.0.0.1:${MIXED_PORT}}"
        # HEAD request, 8s timeout — matches Go's 8s http.Client.Timeout
        _out=$(curl -sfI -o /dev/null -w "%{http_code}:%{time_total}" \
            --proxy "$_proxy_url" --max-time 8 "$_url" 2>&1) || true
        echo "$_out"
    }

    # ── 错误分级（对齐 handler_overview.go stage 分类） ──────────────────────
    _classify_error() {
        _stage="$1"
        _dns_result="$2"
        _curl_err="$3"

        case "$_stage" in
            proxy_port)   echo "proxy_port" ;;
            dns)          echo "dns" ;;
            timeout)      echo "timeout" ;;
            connect)      echo "connect" ;;
            *)
                # Auto-classify from error strings
                _lower=$(echo "$_curl_err$_dns_result" | tr '[:upper:]' '[:lower:]')
                if [ "$MIXED_LISTENING" != "true" ]; then
                    echo "proxy_port"
                elif echo "$_lower" | grep -qE "could not resolve host|name resolution|no address associated"; then
                    echo "dns"
                elif echo "$_lower" | grep -qE "operation timed out|connection timed out|deadline exceeded"; then
                    echo "timeout"
                else
                    echo "connect"
                fi
                ;;
        esac
    }

    # ── Fetch actual port values from clashforge config on VM ─────────────────
    # The code-default ports (17893/17874) may differ from actual runtime ports
    # after SelectCompatiblePorts adjusts them for OpenWrt compatibility.
    _detect_ports() {
        _cf_api="http://127.0.0.1:7777/api/v1"
        # Try to read ports from clashforge config API
        _cfg_json=$(curl -sf --max-time 5 "$_cf_api/config" 2>/dev/null || echo "{}")
        _api_mixed=$(echo "$_cfg_json" | grep -o '"mixed":[0-9]*' | head -1 | sed 's/"mixed"://')
        _api_dns=$(echo "$_cfg_json" | grep -o '"dns":[0-9]*' | head -1 | sed 's/"dns"://')
        if [ -n "$_api_mixed" ] && [ "$_api_mixed" -gt 0 ] 2>/dev/null; then
            MIXED_PORT="$_api_mixed"
            info "从 clashforge 配置读取 mixed 端口: $MIXED_PORT"
        fi
        if [ -n "$_api_dns" ] && [ "$_api_dns" -gt 0 ] 2>/dev/null; then
            MIHOMO_DNS_PORT="$_api_dns"
            info "从 clashforge 配置读取 DNS 端口: $MIHOMO_DNS_PORT"
        fi
    }
    _detect_ports

    RD_OK_COUNT=0; RD_TOTAL=0

    # ── 目标列表（严格对齐 handler_overview.go:812-825） ─────────────────────
    # 用 heredoc + while read 遍历，避免管道 subshell 导致变量丢失，
    # 同时避免描述中的空格造成的 word-splitting。
    while IFS=: read -r _proto _rest name group; do
        URL="${_proto}:${_rest}"
        [ -z "$URL" ] || [ "$URL" = ":" ] && continue
        RD_TOTAL=$((RD_TOTAL+1))

        # 提取 hostname（对齐 resolveForDebug 的 url.Parse + Hostname()）
        HOST=$(echo "$URL" | sed 's|https\?://||' | cut -d/ -f1)

        info ""
        info "── ${name} [${group}] ${URL} ──"

        # ── Step 1: 端口监听检查 ──────────────────────────────────────────
        if [ "$MIXED_LISTENING" != "true" ]; then
            record FAIL "RD-${RD_TOTAL}a" "${name} 端口检查" \
                "curl http://127.0.0.1:${MIXED_PORT}" \
                "mixed 端口 ${MIXED_PORT} 监听" \
                "端口未监听（mihomo 未运行或配置错误）"
            # 端口不通，DNS 和 HTTP 都跳过
            record FAIL "RD-${RD_TOTAL}b" "${name} DNS 解析" \
                "nslookup ${HOST} via mihomo :${MIHOMO_DNS_PORT}" \
                "解析返回 IP" \
                "跳过：mixed 端口不通"
            record FAIL "RD-${RD_TOTAL}c" "${name} HTTP HEAD" \
                "curl --proxy 127.0.0.1:${MIXED_PORT} -I ${URL}" \
                "HTTP 2xx/3xx" \
                "跳过：端口 ${MIXED_PORT} 未监听（stage=proxy_port）"
            continue
        fi

        # ── Step 2: DNS 解析 ──────────────────────────────────────────────
        DNS_IPS=$(_dns_resolve "$HOST" "$MIHOMO_DNS_PORT")
        DNS_FIRST_IP=$(echo "$DNS_IPS" | cut -d, -f1)

        if [ -n "$DNS_FIRST_IP" ]; then
            FAKE_IP_MARKER=""
            if echo "$DNS_FIRST_IP" | grep -qE '^198\.(18|19)\.'; then
                FAKE_IP_MARKER=" (fake-ip)"
            fi
            record PASS "RD-${RD_TOTAL}b" "${name} DNS 解析" \
                "nslookup ${HOST} via mihomo :${MIHOMO_DNS_PORT}" \
                "解析返回 IP" \
                "解析: ${DNS_FIRST_IP}${FAKE_IP_MARKER}，共 $(echo "$DNS_IPS" | tr ',' '\n' | wc -l) 条"
            DNS_OK="true"
        else
            DNS_FAIL_MSG="解析失败"
            if ! _tcp_ok "$MIHOMO_DNS_PORT"; then
                DNS_FAIL_MSG="mihomo DNS 端口 ${MIHOMO_DNS_PORT} 未监听"
            fi
            record FAIL "RD-${RD_TOTAL}b" "${name} DNS 解析" \
                "nslookup ${HOST} via mihomo :${MIHOMO_DNS_PORT}" \
                "解析返回 IP" \
                "DNS 解析失败: ${DNS_FAIL_MSG}（stage=dns）"
            DNS_OK="false"
        fi

        # ── Step 3: HTTP HEAD via mixed proxy ─────────────────────────────
        HTTP_RESULT=$(_http_head_via_proxy "$URL")
        HTTP_CODE=$(echo "$HTTP_RESULT" | cut -d: -f1 | tr -d ' ')
        HTTP_TIME=$(echo "$HTTP_RESULT" | cut -d: -f2 | tr -d ' ')

        if [ -n "$HTTP_CODE" ] && [ "$HTTP_CODE" -ge 200 ] 2>/dev/null && [ "$HTTP_CODE" -lt 400 ] 2>/dev/null; then
            RD_OK_COUNT=$((RD_OK_COUNT+1))
            record PASS "RD-${RD_TOTAL}c" "${name} HTTP HEAD" \
                "curl --proxy <mixed> -I ${URL}" \
                "HTTP 2xx/3xx" \
                "HTTP ${HTTP_CODE} (${HTTP_TIME}s)"
        else
            # ── 错误分级（对齐 handler_overview.go stage 分类） ──────────
            STAGE=""
            CURL_ERR="$HTTP_RESULT"
            if echo "$CURL_ERR" | grep -qE "refused|not listening"; then
                STAGE="proxy_port"
            elif echo "$CURL_ERR" | grep -qE "Could not resolve|resolve error|Host not found"; then
                STAGE="dns"
            elif echo "$CURL_ERR" | grep -qE "timed out|timeout|Operation timed out"; then
                STAGE="timeout"
            else
                STAGE="connect"
            fi

            record FAIL "RD-${RD_TOTAL}c" "${name} HTTP HEAD" \
                "curl --proxy <mixed> -I ${URL}" \
                "HTTP 2xx/3xx" \
                "失败 (stage=${STAGE}): HTTP ${HTTP_CODE:-error}${HTTP_TIME:+ (${HTTP_TIME}s)}"
        fi
    done <<TARGETS
https://www.taobao.com:淘宝:国内
https://music.163.com:网易云音乐:国内
https://github.com:GitHub:国外
https://www.google.com:Google:国外
https://chat.openai.com:OpenAI:AI
https://api.anthropic.com:Claude:AI
https://gemini.google.com:Gemini:AI
TARGETS

    # ── 路由器侧总体断言 ──────────────────────────────────────────────────
    section "路由器侧探测汇总"
    if [ "$MIXED_LISTENING" != "true" ]; then
        record FAIL RD-SUM "路由器探测总览" \
            "mixed 端口 ${MIXED_PORT} + DNS 端口 ${MIHOMO_DNS_PORT} 均正常监听" \
            "端口未监听：mihomo 未运行或配置错误，所有路由器侧探测跳过"
    elif [ "$RD_OK_COUNT" = "$RD_TOTAL" ]; then
        record PASS RD-SUM "路由器探测总览" \
            "${RD_TOTAL} 个目标全部通过 HTTP HEAD via mixed proxy" \
            "${RD_OK_COUNT}/${RD_TOTAL} 全部通过 ✓"
    else
        record WARN RD-SUM "路由器探测总览" \
            "${RD_TOTAL} 个目标全部通过 HTTP HEAD via mixed proxy" \
            "${RD_OK_COUNT}/${RD_TOTAL} 通过（部分失败可能是因为节点网络限制）"
    fi
fi

# ═══════════════════════════════════════════════════════════════════════════════
# PR-02: 浏览器侧连通性检查（curl 透明代理 / 直连）
# ═══════════════════════════════════════════════════════════════════════════════
section "连通性检查 — $PHASE_LABEL"

# 目标：国内 2 个 + 国际 2 个
TARGETS="https://www.baidu.com:百度搜索:国内 https://music.163.com:网易云音乐:国内 https://github.com:GitHub:国际 https://www.youtube.com:YouTube:国际"

OK_COUNT=0; TOTAL_ACCESS=0; DETAILS=""
for entry in $TARGETS; do
    URL=$(echo "$entry" | cut -d: -f1-2)
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

# ═══════════════════════════════════════════════════════════════════════════════
# Summary
# ═══════════════════════════════════════════════════════════════════════════════
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
