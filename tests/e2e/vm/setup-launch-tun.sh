#!/bin/sh
# tests/e2e/vm/setup-launch-tun.sh
# Step 5 (TUN variant) — 验证 TUN 模式下 setup/launch 正确启动
#
# 与 setup-launch.sh 的区别：
#   - network.mode = "tun"（不是 tproxy）
#   - 断言 /dev/net/tun 存在
#   - 断言 TUN 虚拟网卡已被 mihomo 创建（ip link show 出现 Meta/utun/tun 设备）
#   - 断言无传统 metaclash 表；检查 fw4 forward_lan 和 IPv4 补充路由
#   - 断言 mihomo 进程已启动
#   - 断言 DNS 解析可用（fake-ip 或真实 IP）
#
# 测试用例编号：
#   TUN-01  /dev/net/tun 设备存在
#   TUN-02  setup/launch (mode=tun) → success:true
#   TUN-03  mihomo 进程已启动
#   TUN-04  TUN 虚拟网卡已出现在 ip link 中
#   TUN-05  nftables metaclash 表【不存在】（TUN 模式不应有 tproxy 规则）
#   TUN-06  DNS 解析可用（通过 mihomo DNS 端口）
#
# 环境变量：
#   SUBSCRIPTION_URL   订阅地址（必须，除非 REUSE_SUBSCRIPTION=true）
#   REUSE_SUBSCRIPTION true/false（默认 false）：复用已有订阅和节点
#   GITHUB_STEP_SUMMARY

set -e

check_tun_dns() {
    local port="$1" name="$2" attempt=0 answer=""
    case "$port" in ''|*[!0-9]*) return 1 ;; esac
    [ "$port" -gt 0 ] && [ "$port" -le 65535 ] || return 1
    command -v dig >/dev/null 2>&1 || {
        echo "dig required: do not substitute an unrelated system resolver" >&2; return 1;
    }
    while [ "$attempt" -lt 5 ]; do
        attempt=$((attempt+1))
        answer=$(dig +short +time=3 +tries=1 @127.0.0.1 -p "$port" "$name" A 2>/dev/null) || answer=""
        answer=$(printf '%s\n' "$answer" | awk -F. '
            NF == 4 { ok=1; for(i=1;i<=4;i++) if($i !~ /^[0-9]+$/ || $i>255) ok=0; if(ok) {print; exit} }')
        if [ -n "$answer" ]; then printf '%s\n' "$answer"; return 0; fi
        [ "$attempt" -ge 5 ] || sleep 2
    done
    return 1
}

# Exercise the same DNS production function/argv safely in the host harness.
if [ "${1:-}" = --check-dns ]; then
    [ "$#" -eq 3 ] || exit 2
    check_tun_dns "$2" "$3"
    exit $?
fi

REUSE_SUBSCRIPTION="${REUSE_SUBSCRIPTION:-false}"

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
mkdir -p "$SNAPSHOT_DIR"

section "Step 5 (TUN) — TUN 模式启动验证"

# ── TUN-01: /dev/net/tun 设备存在 ─────────────────────────────────────────────
if [ -c /dev/net/tun ]; then
    record PASS TUN-01 "/dev/net/tun 设备存在" "/dev/net/tun 为字符设备" "$(ls -la /dev/net/tun)"
else
    record FAIL TUN-01 "/dev/net/tun 设备存在" "/dev/net/tun 为字符设备" "缺少设备：不由测试自修复/掩盖安装条件"
fi

# ── 订阅准备（复用或新建）────────────────────────────────────────────────────
if [ "$REUSE_SUBSCRIPTION" = "true" ]; then
    SUB_ID=$(cat "$SNAPSHOT_DIR/sub-id" 2>/dev/null || echo "")
    if [ -z "$SUB_ID" ]; then
        info "REUSE_SUBSCRIPTION=true 但 sub-id 文件不存在，尝试继续"
    fi
else
    if [ -z "${SUBSCRIPTION_URL:-}" ]; then
        printf "${RED}ERROR: SUBSCRIPTION_URL is required${RESET}\n"
        exit 1
    fi
    info "添加订阅..."
    ADD_RESP=$(curl -sf --max-time 15 \
        -H "Content-Type: application/json" \
        -d "{\"url\":\"${SUBSCRIPTION_URL}\",\"name\":\"e2e-tun-test\",\"enabled\":true}" \
        "$CF_API/subscriptions" 2>/dev/null || echo "FAILED")
    SUB_ID=$(echo "$ADD_RESP" | grep -o '"id":"[^"]*"' | head -1 | sed 's/"id":"//;s/"//')
    [ -n "$SUB_ID" ] && echo "$SUB_ID" > "$SNAPSHOT_DIR/sub-id"

    if [ -n "$SUB_ID" ]; then
        info "同步订阅节点 ($SUB_ID)..."
        curl -sf --max-time 60 \
            -H "Content-Type: application/json" -d '{}' \
            "$CF_API/subscriptions/$SUB_ID/sync-update" >/dev/null 2>&1 || true
    fi
fi

# ── TUN-02: setup/launch with mode=tun ────────────────────────────────────────
LAUNCH_PAYLOAD='{"dns":{"enable":true,"mode":"fake-ip","dnsmasq_mode":"upstream","apply_on_start":true},"network":{"mode":"tun","firewall_backend":"nftables","ipv6":false,"bypass_lan":true,"bypass_china":false,"apply_on_start":true,"tun":{"device":"Meta","stack":"mixed","auto_route":true,"auto_detect_interface":true}}}'
info "触发 setup/launch (mode=tun)..."
LAUNCH_RESP=$(curl -sf --max-time 90 \
    -H "Content-Type: application/json" \
    -d "$LAUNCH_PAYLOAD" \
    "$CF_API/setup/launch" 2>/dev/null || echo "FAILED")

if echo "$LAUNCH_RESP" | grep -q '"success":true'; then
    record PASS TUN-02 "setup/launch (mode=tun)" "POST /setup/launch → {success:true}" "launch 成功"
else
    LAUNCH_ERR=$(echo "$LAUNCH_RESP" | grep -o '"error":"[^"]*"' | head -1 | sed 's/"error":"//;s/"$//' 2>/dev/null || echo "")
    [ -z "$LAUNCH_ERR" ] && LAUNCH_ERR=$(echo "$LAUNCH_RESP" | tail -3)
    printf "${RED}TUN-02 原始响应（最后5行）:${RESET}\n"
    echo "$LAUNCH_RESP" | tail -5
    record FAIL TUN-02 "setup/launch (mode=tun)" "POST /setup/launch → {success:true}" "失败: $LAUNCH_ERR"
    # Continue to collect diagnostics even after launch failure
fi

sleep 5

# ── TUN-03: mihomo 进程已启动 ──────────────────────────────────────────────────
MIHOMO_PID=$(pgrep -f "mihomo-clashforge" 2>/dev/null || echo "")
if [ -n "$MIHOMO_PID" ]; then
    record PASS TUN-03 "mihomo 进程已启动" "pgrep mihomo-clashforge → 有进程" "PID: $MIHOMO_PID"
else
    record FAIL TUN-03 "mihomo 进程已启动" "pgrep mihomo-clashforge → 有进程" "mihomo 未运行"
fi

# ── TUN-04: TUN 虚拟网卡已出现 ────────────────────────────────────────────────
# Require the configured device, not an unrelated VPN's tun interface.
sleep 3
TUN_IFACE=""
ip -4 link show dev Meta >/dev/null 2>&1 && TUN_IFACE=Meta
if [ -n "$TUN_IFACE" ]; then
    TUN_STATE=$(ip link show "$TUN_IFACE" 2>/dev/null | grep -oE 'state [A-Z]+' | head -1 || echo "")
    record PASS TUN-04 "TUN 虚拟网卡已创建" "ip link show 出现 Meta*/tun* 接口" "$TUN_IFACE ($TUN_STATE)"
else
    ALL_LINKS=$(ip link show 2>/dev/null | grep -oE '^[0-9]+: [^:]+' | awk '{print $2}' | tr '\n' ' ')
    record FAIL TUN-04 "TUN 虚拟网卡已创建" "ip link show 出现 Meta*/tun* 接口" "未找到 TUN 接口（当前: $ALL_LINKS）"
fi

# ── TUN-05: nftables metaclash 表【不存在】───────────────────────────────────
NFT_TABLES=$(nft list tables 2>/dev/null | tr '\n' ' ')
if echo "$NFT_TABLES" | grep -qE "metaclash|clashforge"; then
    record FAIL TUN-05 "nftables metaclash 表不存在" "TUN 模式不应创建 tproxy 规则表" "表意外存在: $NFT_TABLES"
else
    record PASS TUN-05 "nftables metaclash 表不存在" "TUN 模式不应创建 tproxy 规则表" "表未创建 ✓（当前: ${NFT_TABLES:-空}）"
fi

# ── TUN-06: DNS 解析可用 ──────────────────────────────────────────────────────
CFG_JSON=$(curl -sf --max-time 5 "$CF_API/config" 2>/dev/null || echo "{}")
DNS_PORT=$(echo "$CFG_JSON" | grep -o '"dns":[0-9]*' | head -1 | sed 's/"dns"://')
DNS_RESULT=$(check_tun_dns "${DNS_PORT:-}" "${DNS_TEST_NAME:-example.com}") || DNS_RESULT=""

if [ -n "$DNS_RESULT" ]; then
    IS_FAKEIP=""
    echo "$DNS_RESULT" | grep -qE "^198\.1[89]\." && IS_FAKEIP=" (fake-ip ✓)"
    record PASS TUN-06 "核心 DNS 端口响应" "配置的 Mihomo DNS 端口返回 A 记录" "${DNS_RESULT}${IS_FAKEIP}"
else
    record FAIL TUN-06 "核心 DNS 端口响应" "配置的 Mihomo DNS 端口返回 A 记录" "有限重试后仍无结果/缺少 dig"
fi

# These are control-plane assertions, NOT a LAN TCP/UDP forwarding test.
RULES=$(ip -4 rule show 2>/dev/null || true)
if printf '%s\n' "$RULES" | grep -qE '^9020:[[:space:]]+from all lookup main .*suppress_prefixlength 0' &&
   printf '%s\n' "$RULES" | grep -qE '^9020:.*(proto|protocol) 242' &&
   printf '%s\n' "$RULES" | grep -qE '^9021:[[:space:]]+from all lookup 2022 (proto|protocol) 242'; then
    record PASS TUN-07 "TUN IPv4 补充路由" "9020/9021，protocol 242" "已发现带所有权标记的规则"
else
    record FAIL TUN-07 "TUN IPv4 补充路由" "9020/9021，protocol 242" "$RULES"
fi
ROUTES=$(ip -4 route show table 2022 2>/dev/null || true)
if printf '%s\n' "$ROUTES" | grep -qE '^default .*dev Meta([[:space:]]|$)'; then
    record PASS TUN-08 "TUN IPv4 默认路由" "table 2022 default dev Meta" "$ROUTES"
else
    record FAIL TUN-08 "TUN IPv4 默认路由" "table 2022 default dev Meta" "$ROUTES"
fi
FORWARD=$(nft -a list chain inet fw4 forward_lan 2>/dev/null || true)
if printf '%s\n' "$FORWARD" | grep -qE 'meta nfproto ipv4 oifname "Meta" .*accept comment "clashforge: allow forwarding into TUN device"'; then
    record PASS TUN-09 "fw4 LAN 补充规则" "IPv4 Meta accept，项目 comment" "规则存在（未证明 LAN 数据面）"
else
    record FAIL TUN-09 "fw4 LAN 补充规则" "IPv4 Meta accept，项目 comment" "$FORWARD"
fi

# ── Summary ────────────────────────────────────────────────────────────────────
PASS_COUNT=$(printf "%b" "$RESULTS" | grep -c "^PASS" || true)
FAIL_COUNT=$(printf "%b" "$RESULTS" | grep -c "^FAIL" || true)
WARN_COUNT=$(printf "%b" "$RESULTS" | grep -c "^WARN" || true)

summary "## 🌐 Step 5 (TUN) — TUN 模式启动验证"
summary "范围：IPv4/fw4 forward_lan 控制面与核心 DNS 端口；不覆盖 LAN 无代理 TCP/UDP、IPv6 防绕行、guest zone、fw4 reload。"
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
    printf "${RED}${BOLD}TUN 模式验证失败 ❌${RESET}\n"
    exit 1
else
    printf "${GREEN}${BOLD}TUN 模式验证通过 ✅${RESET}\n"
fi
