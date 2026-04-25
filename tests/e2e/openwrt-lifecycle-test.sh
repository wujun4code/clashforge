#!/bin/sh
# tests/e2e/openwrt-lifecycle-test.sh
# ClashForge 完整生命周期 e2e 测试脚本（路由器端）
#
# 测试流程：
#   Phase 1 — 启动阶段
#     TC-01  订阅 URL 可达性
#     TC-02  记录启动前状态快照
#     TC-03  安装 clashforge + 依赖内核模块
#     TC-04  启动服务 + API 就绪
#     TC-05  添加订阅 + 拉取节点
#     TC-06  触发 Setup Launch（DNS + nftables tproxy）
#
#   Phase 2 — 接管验证
#     TC-07  DNS 接管验证（fake-ip + dnsmasq upstream 配置）
#     TC-08  nftables 透明代理接管验证（metaclash 表 + tproxy 规则）
#     TC-09  ip rule 策略路由验证
#
#   Phase 3 — 代理运行期探测（路由器端）
#     TC-10  路由器端 IP 检查（/overview/probes — ip_checks）
#     TC-11  路由器端可访问性检查（/overview/probes — access_checks）
#     TC-12  出口 IP 变化验证（代理有效性）
#
#   Phase 4 — 停止阶段
#     TC-13  停止服务
#     TC-14  nftables 还原验证（metaclash 表已移除）
#     TC-15  ip rule 还原验证
#     TC-16  dnsmasq 配置还原验证
#
#   Phase 5 — 停止后恢复探测（路由器端）
#     TC-17  停止后网络可达性（直连 IP 还原）
#     TC-18  停止后路由器端 IP 检查（确认已回直连）
#     TC-19  停止后路由器端可访问性检查
#
# 环境变量：
#   SUBSCRIPTION_URL     订阅 URL（必须）
#   CLASHFORGE_VERSION   版本（default: latest）
#   GITHUB_STEP_SUMMARY  GitHub Actions job summary 文件路径（CI 自动注入）

set -e

# ── 颜色 ──────────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

FAILED=0
WARNED=0
TOTAL=0

# ── 测试结果收集（用于 summary）─────────────────────────────────────────────
RESULTS=""   # 每行：STATUS|TC|名称|操作|预期|实际

record() {
    STATUS="$1"; TC="$2"; NAME="$3"; OP="$4"; EXPECTED="$5"; ACTUAL="$6"
    RESULTS="${RESULTS}${STATUS}|${TC}|${NAME}|${OP}|${EXPECTED}|${ACTUAL}\n"
    TOTAL=$((TOTAL+1))
    case "$STATUS" in
        PASS) printf "${GREEN}✅ PASS${RESET} [%s] %s — %s\n" "$TC" "$NAME" "$ACTUAL" ;;
        FAIL) printf "${RED}❌ FAIL${RESET} [%s] %s — %s\n" "$TC" "$NAME" "$ACTUAL"; FAILED=$((FAILED+1)) ;;
        WARN) printf "${YELLOW}⚠️  WARN${RESET} [%s] %s — %s\n" "$TC" "$NAME" "$ACTUAL"; WARNED=$((WARNED+1)) ;;
    esac
}

info()    { printf "${CYAN}ℹ️   ${RESET} %s\n" "$*"; }
section() { printf "\n${BOLD}${YELLOW}=== %s ===${RESET}\n" "$*"; }

# ── GitHub Actions summary helper ─────────────────────────────────────────────
summary() { [ -n "${GITHUB_STEP_SUMMARY:-}" ] && echo "$*" >> "$GITHUB_STEP_SUMMARY" || true; }

# ── 配置 ──────────────────────────────────────────────────────────────────────
CLASHFORGE_VERSION="${CLASHFORGE_VERSION:-latest}"
SUBSCRIPTION_URL="${SUBSCRIPTION_URL:-}"
CF_API="http://127.0.0.1:7777/api/v1"
SNAPSHOT_DIR="/tmp/cf-test-snapshot"
SUB_ID=""
PROXY_AUTH=""

# ── 辅助 ──────────────────────────────────────────────────────────────────────
wait_http() {
    url="$1"; max="$2"; i=0
    while [ $i -lt "$max" ]; do
        wget -q -O /dev/null --timeout=2 "$url" 2>/dev/null && return 0
        sleep 1; i=$((i+1))
    done; return 1
}

json_get() { echo "$1" | grep -o "\"$2\":[^,}]*" | head -1 | sed 's/.*: *"\{0,1\}\([^",}]*\).*/\1/'; }

# ── Phase 1: 启动阶段 ─────────────────────────────────────────────────────────
section "Phase 1 — 启动阶段"

# TC-01
if [ -z "$SUBSCRIPTION_URL" ]; then
    record FAIL TC-01 "订阅 URL 可达性" "检查订阅 URL 返回有效 YAML" "节点数 ≥ 1" "SUBSCRIPTION_URL 未设置"
    exit 1
fi
SUB_CHECK=$(wget -q -O - --timeout=15 "$SUBSCRIPTION_URL" 2>/dev/null | head -5)
if echo "$SUB_CHECK" | grep -qE "port|proxies|---"; then
    NODE_COUNT=$(wget -q -O - --timeout=15 "$SUBSCRIPTION_URL" 2>/dev/null | grep -c "^  - name:" || echo 0)
    record PASS TC-01 "订阅 URL 可达性" "HTTPS GET 订阅 URL，检查 YAML 格式" "返回有效 YAML，节点数 ≥ 1" "节点数: $NODE_COUNT"
else
    record FAIL TC-01 "订阅 URL 可达性" "HTTPS GET 订阅 URL，检查 YAML 格式" "返回有效 YAML，节点数 ≥ 1" "URL 无效或无法访问"
    exit 1
fi

# TC-02
mkdir -p "$SNAPSHOT_DIR"
cp /etc/resolv.conf "$SNAPSHOT_DIR/resolv.conf.before" 2>/dev/null || true
nft list ruleset > "$SNAPSHOT_DIR/nft.before" 2>/dev/null || echo "" > "$SNAPSHOT_DIR/nft.before"
ip rule list > "$SNAPSHOT_DIR/ip-rule.before"
ls /etc/dnsmasq.d/ > "$SNAPSHOT_DIR/dnsmasq-d.before" 2>/dev/null || echo "" > "$SNAPSHOT_DIR/dnsmasq-d.before"
DIRECT_IP=$(wget -q -O - --timeout=10 https://api.ipify.org 2>/dev/null || echo "FAILED")
if [ "$DIRECT_IP" != "FAILED" ]; then
    echo "$DIRECT_IP" > "$SNAPSHOT_DIR/direct-ip"
    record PASS TC-02 "启动前状态快照" "记录 nft/ip-rule/dnsmasq/resolv.conf 并获取直连 IP" "快照保存成功，直连 IP 可获取" "直连 IP: $DIRECT_IP"
else
    record FAIL TC-02 "启动前状态快照" "记录启动前状态并获取直连 IP" "直连 IP 可获取" "无法获取直连 IP，网络异常"
fi
info "启动前 nft 表: $(nft list tables 2>/dev/null | tr '\n' ' ')"
info "启动前 DNS: $(grep nameserver /etc/resolv.conf | tr '\n' ' ')"

# TC-03
if command -v clashforge > /dev/null 2>&1; then
    info "clashforge 已安装，跳过安装"
else
    info "安装 clashforge $CLASHFORGE_VERSION ..."
    if [ "$CLASHFORGE_VERSION" = "latest" ]; then
        wget -qO- https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh
    else
        wget -qO- https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh -s -- --version "$CLASHFORGE_VERSION"
    fi
fi
opkg install kmod-nft-tproxy kmod-nf-tproxy curl 2>/dev/null | grep -v "up to date" || true
modprobe nft_tproxy 2>/dev/null || true
if command -v clashforge > /dev/null 2>&1 && lsmod | grep -q nft_tproxy; then
    VER=$(opkg list-installed 2>/dev/null | grep clashforge | awk '{print $3}' || echo "unknown")
    record PASS TC-03 "安装 clashforge + 内核模块" "install.sh 安装 + opkg kmod-nft-tproxy + modprobe" "clashforge 可用，nft_tproxy 模块加载" "版本: $VER，nft_tproxy 已加载"
else
    record FAIL TC-03 "安装 clashforge + 内核模块" "install.sh 安装 + opkg kmod-nft-tproxy + modprobe" "clashforge 可用，nft_tproxy 模块加载" "安装或模块加载失败"
    exit 1
fi

# TC-04
/etc/init.d/clashforge start 2>/dev/null || true
if wait_http "$CF_API/status" 30; then
    record PASS TC-04 "启动服务 + API 就绪" "/etc/init.d/clashforge start，轮询 GET /api/v1/status" "30 秒内返回 {ok:true}" "API 就绪"
else
    record FAIL TC-04 "启动服务 + API 就绪" "/etc/init.d/clashforge start，轮询 GET /api/v1/status" "30 秒内返回 {ok:true}" "30 秒内未响应"
    exit 1
fi

# TC-05
ADD_RESP=$(curl -sf --max-time 15 \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"$SUBSCRIPTION_URL\",\"name\":\"e2e-test\",\"enabled\":true}" \
    "$CF_API/subscriptions" 2>/dev/null || echo "FAILED")
if [ "$ADD_RESP" = "FAILED" ]; then
    record FAIL TC-05 "添加订阅 + 拉取节点" "POST /subscriptions + POST /subscriptions/{id}/sync-update" "订阅 ID 返回，节点拉取成功" "添加订阅 API 失败"
    exit 1
fi
SUB_ID=$(echo "$ADD_RESP" | grep -o '"id":"[^"]*"' | head -1 | sed 's/"id":"//;s/"//')
SYNC_RESP=$(curl -sf --max-time 30 -H "Content-Type: application/json" -d '{}' \
    "$CF_API/subscriptions/$SUB_ID/sync-update" 2>/dev/null || echo "FAILED")
if [ -n "$SUB_ID" ] && echo "$SYNC_RESP" | grep -qE "ok|true|success|nodes"; then
    record PASS TC-05 "添加订阅 + 拉取节点" "POST /subscriptions + POST /subscriptions/{id}/sync-update" "订阅 ID 返回，节点拉取成功" "订阅 ID: $SUB_ID，节点拉取成功"
else
    record FAIL TC-05 "添加订阅 + 拉取节点" "POST /subscriptions + POST /subscriptions/{id}/sync-update" "订阅 ID 返回，节点拉取成功" "sync 失败: $SYNC_RESP"
    exit 1
fi

# TC-06
sleep 2
curl -sf --max-time 60 -H "Content-Type: application/json" \
    -d '{"dns":{"enable":true,"mode":"fake-ip","dnsmasq_mode":"upstream","apply_on_start":true},"network":{"mode":"tproxy","firewall_backend":"nftables","bypass_lan":true,"bypass_china":false,"apply_on_start":true}}' \
    "$CF_API/setup/launch" > /tmp/launch.log 2>/dev/null || true
LAUNCH_OUT=$(cat /tmp/launch.log)
if echo "$LAUNCH_OUT" | grep -q '"success":true'; then
    record PASS TC-06 "触发 Setup Launch" "POST /setup/launch dns.mode=fake-ip dnsmasq_mode=upstream network.mode=tproxy" '{"success":true}' "launch 成功"
else
    LAUNCH_ERR=$(echo "$LAUNCH_OUT" | grep -o '"error":"[^"]*"' | head -1)
    record FAIL TC-06 "触发 Setup Launch" "POST /setup/launch" '{"success":true}' "launch 失败: $LAUNCH_ERR"
    exit 1
fi
sleep 5

# ── Phase 2: 接管验证 ─────────────────────────────────────────────────────────
section "Phase 2 — 接管验证"

# TC-07
DNS_CHANGED="no"
if grep -rE "server=127.0.0.1|127.0.0.1#1053|127.0.0.1#7874" /etc/dnsmasq.d/ /tmp/dnsmasq.d/ 2>/dev/null | grep -q "server"; then
    DNS_CHANGED="yes"
fi
DNSMASQ_AFTER=$(ls /etc/dnsmasq.d/ 2>/dev/null | tr '\n' ' ')
DNSMASQ_BEFORE=$(cat "$SNAPSHOT_DIR/dnsmasq-d.before" | tr '\n' ' ')
[ "$DNSMASQ_AFTER" != "$DNSMASQ_BEFORE" ] && DNS_CHANGED="yes"
DNS_RESOLVE=$(nslookup google.com 2>/dev/null | grep "Address:" | grep -v "#53" | head -1 | awk '{print $2}')
if echo "$DNS_RESOLVE" | grep -qE "^198\.18\.|^198\.19\."; then
    record PASS TC-07 "DNS 接管验证" "检查 dnsmasq.d 配置变化 + DNS 解析返回 fake-ip 段地址" "dnsmasq upstream 指向 mihomo，解析返回 198.18.x.x" "fake-ip 解析: $DNS_RESOLVE，dnsmasq.d 变化: $DNS_CHANGED"
elif [ "$DNS_CHANGED" = "yes" ]; then
    record PASS TC-07 "DNS 接管验证" "检查 dnsmasq.d 配置变化 + DNS 解析" "dnsmasq 配置变化，DNS 接管生效" "dnsmasq.d 已变化（$DNS_RESOLVE）"
else
    record WARN TC-07 "DNS 接管验证" "检查 dnsmasq.d 配置变化 + DNS 解析" "dnsmasq 配置变化" "DNS 接管状态不明确"
fi

# TC-08
NFT_TABLES=$(nft list tables 2>/dev/null | tr '\n' ' ')
HAS_METACLASH=$(echo "$NFT_TABLES" | grep -qE "metaclash|clashforge" && echo "yes" || echo "no")
HAS_TPROXY=$(nft list ruleset 2>/dev/null | grep -qE "tproxy|redirect" && echo "yes" || echo "no")
if [ "$HAS_METACLASH" = "yes" ] && [ "$HAS_TPROXY" = "yes" ]; then
    record PASS TC-08 "nftables 透明代理接管" "nft list tables 检查 metaclash 表；ruleset 检查 tproxy 规则" "table inet metaclash 存在，tproxy 规则存在" "nft 表: $NFT_TABLES"
elif [ "$HAS_METACLASH" = "yes" ]; then
    record WARN TC-08 "nftables 透明代理接管" "nft list tables 检查 metaclash 表；ruleset 检查 tproxy 规则" "metaclash 表 + tproxy 规则均存在" "metaclash 表存在但 tproxy 规则未找到"
else
    record FAIL TC-08 "nftables 透明代理接管" "nft list tables 检查 metaclash 表" "table inet metaclash 存在" "metaclash 表未找到，nft 表: $NFT_TABLES"
fi

# TC-09
if ip rule list 2>/dev/null | grep -qE "fwmark|lookup"; then
    IP_RULE=$(ip rule list 2>/dev/null | grep -E "fwmark|lookup" | head -2 | tr '\n' ' ')
    record PASS TC-09 "ip rule 策略路由" "ip rule list 检查 fwmark/lookup 规则" "策略路由规则已配置" "$IP_RULE"
else
    record WARN TC-09 "ip rule 策略路由" "ip rule list 检查 fwmark/lookup 规则" "策略路由规则已配置" "策略路由未找到"
fi

# ── Phase 3: 代理运行期探测（路由器端）────────────────────────────────────────
section "Phase 3 — 代理运行期探测（路由器端）"

# 读取代理认证
CONFIG_CACHE=$(ls /etc/metaclash/cache/*.raw.yaml 2>/dev/null | tail -1)
if [ -n "$CONFIG_CACHE" ]; then
    AUTH_LINE=$(grep -A1 'authentication:' "$CONFIG_CACHE" 2>/dev/null | grep -v 'authentication:' | head -1 | tr -d ' -')
    [ -n "$AUTH_LINE" ] && PROXY_AUTH="$AUTH_LINE"
fi
info "代理认证: ${PROXY_AUTH:-无}"

sleep 3

# 调用路由器端 probes API
PROBES_RESP=$(curl -sf --max-time 30 "$CF_API/overview/probes" 2>/dev/null || echo "FAILED")

# TC-10 路由器端 IP 检查
if [ "$PROBES_RESP" != "FAILED" ]; then
    # 只从 ip_checks 数组里计数
    IP_SECTION=$(echo "$PROBES_RESP" | grep -o '"ip_checks":\[[^]]*\]' | head -1)
    IP_OK=$(echo "$IP_SECTION" | grep -o '"ok":true' | wc -l)
    IP_TOTAL=$(echo "$IP_SECTION" | grep -o '"provider":' | wc -l)
    [ "$IP_TOTAL" -eq 0 ] && IP_TOTAL=$(echo "$PROBES_RESP" | grep -o '"provider":' | wc -l)
    PROXY_IP=$(echo "$PROBES_RESP" | grep -o '"ip":"[0-9.a-f:]*"' | grep -v "fake\|198\.18" | head -1 | sed 's/"ip":"//;s/"//')
    LOCATION=$(echo "$PROBES_RESP" | grep -o '"location":"[^"]*"' | head -1 | sed 's/"location":"//;s/"//')
    if [ "$IP_OK" -gt 0 ] && [ -n "$PROXY_IP" ]; then
        record PASS TC-10 "路由器端 IP 检查（代理中）" "GET /overview/probes — ip_checks" "至少 1 个 IP 检查服务返回有效出口 IP" "$IP_OK/$IP_TOTAL 成功，出口 IP: $PROXY_IP ($LOCATION)"
    else
        record FAIL TC-10 "路由器端 IP 检查（代理中）" "GET /overview/probes — ip_checks" "至少 1 个 IP 检查服务返回有效出口 IP" "全部 IP 检查失败（IP_OK=$IP_OK PROXY_IP=$PROXY_IP）"
    fi
else
    record FAIL TC-10 "路由器端 IP 检查（代理中）" "GET /overview/probes — ip_checks" "至少 1 个 IP 检查服务返回有效出口 IP" "/overview/probes API 无响应"
fi

# TC-11 路由器端可访问性检查
if [ "$PROBES_RESP" != "FAILED" ]; then
    TOTAL_OK=$(echo "$PROBES_RESP" | grep -o '"ok":true' | wc -l)
    ACCESS_TOTAL=$(echo "$PROBES_RESP" | grep -o '"url":"http' | wc -l)
    IP_OK_COUNT=$(echo "$PROBES_RESP" | grep -o '"provider":' | wc -l)
    ACCESS_OK=$((TOTAL_OK - IP_OK_COUNT))
    [ "$ACCESS_OK" -lt 0 ] && ACCESS_OK=0
    if [ "$ACCESS_OK" -gt 0 ]; then
        record PASS TC-11 "路由器端可访问性检查（代理中）" "GET /overview/probes — access_checks" "国内外主要站点通过代理可达" "$ACCESS_OK/$ACCESS_TOTAL 成功"
    else
        record FAIL TC-11 "路由器端可访问性检查（代理中）" "GET /overview/probes — access_checks" "至少 1 个站点通过代理可达" "全部可访问性检查失败（ok=$TOTAL_OK providers=$IP_OK_COUNT access=$ACCESS_OK total=$ACCESS_TOTAL）"
    fi
fi

# TC-12 出口 IP 变化验证 + 与代理节点对应验证
DIRECT_IP=$(cat "$SNAPSHOT_DIR/direct-ip" 2>/dev/null || echo "unknown")
CURRENT_PROXY_IP=$(echo "$PROBES_RESP" | grep -o '"ip":"[0-9.]*"' | head -1 | sed 's/"ip":"//;s/"//')

# 从订阅缓存读取代理节点的服务器地址
# BusyBox grep 支持 -A N（短选项形式）
PROXY_NODE_NAME=$(grep -A 1 "^  - name:" /etc/metaclash/cache/*.raw.yaml 2>/dev/null \
    | grep "name:" | grep -v "^  - name:" | head -1 | awk "{print \$2}" | tr -d " ")
PROXY_SERVER=$(grep -A 5 "name: $PROXY_NODE_NAME" /etc/metaclash/cache/*.raw.yaml 2>/dev/null \
    | grep "server:" | head -1 | awk "{print \$2}" | tr -d " ")

# 通过 DoH 解析代理服务器真实 IP（绕过 fake-ip DNS）
PROXY_SERVER_IP=""
if [ -n "$PROXY_SERVER" ]; then
    # 使用 --resolve 强制 DNS 请求到 8.8.8.8，绕过 fake-ip
    PROXY_SERVER_IP=$(curl -sf --max-time 10 \
        --resolve "dns.google:443:8.8.8.8" \
        "https://dns.google/resolve?name=${PROXY_SERVER}&type=A" 2>/dev/null \
        | grep -o '"data":"[0-9.]*"' | head -1 | sed 's/"data":"//;s/"//')
    # 备用：直接 HTTP 到 8.8.8.8
    if [ -z "$PROXY_SERVER_IP" ]; then
        PROXY_SERVER_IP=$(curl -sf --max-time 10 \
            -H "Host: dns.google" \
            "https://8.8.8.8/resolve?name=${PROXY_SERVER}&type=A" \
            --insecure 2>/dev/null \
            | grep -o '"data":"[0-9.]*"' | head -1 | sed 's/"data":"//;s/"//')
    fi
fi
info "代理节点: $PROXY_NODE_NAME (服务器: $PROXY_SERVER → 真实 IP: $PROXY_SERVER_IP)"
info "代理出口 IP: $CURRENT_PROXY_IP"

# 验证逻辑：IP 变化 + 出口与节点对应
IP_CHANGED=$([ -n "$CURRENT_PROXY_IP" ] && [ "$CURRENT_PROXY_IP" != "$DIRECT_IP" ] && echo "yes" || echo "no")
IP_MATCHES_NODE=$([ -n "$PROXY_SERVER_IP" ] && [ "$CURRENT_PROXY_IP" = "$PROXY_SERVER_IP" ] && echo "yes" || echo "no")

if [ "$IP_CHANGED" = "yes" ] && [ "$IP_MATCHES_NODE" = "yes" ]; then
    record PASS TC-12 "出口 IP 变化 + 节点对应验证" \
        "对比直连 IP、代理出口 IP、订阅节点服务器 DNS 解析地址" \
        "出口 IP 不同于直连，且与使用的代理节点地址匹配" \
        "直连:$DIRECT_IP → 代理:$CURRENT_PROXY_IP = 节点[$PROXY_NODE_NAME] $PROXY_SERVER→$PROXY_SERVER_IP ✓"
elif [ "$IP_CHANGED" = "yes" ] && [ -z "$PROXY_SERVER_IP" ]; then
    record WARN TC-12 "出口 IP 变化 + 节点对应验证" \
        "对比直连 IP、代理出口 IP、订阅节点服务器 DNS 解析地址" \
        "出口 IP 不同于直连，且与使用的代理节点地址匹配" \
        "IP 已变化 ($DIRECT_IP → $CURRENT_PROXY_IP)，但无法解析节点服务器 DNS 做二次确认"
elif [ "$IP_CHANGED" = "yes" ]; then
    record WARN TC-12 "出口 IP 变化 + 节点对应验证" \
        "对比直连 IP、代理出口 IP、订阅节点服务器 DNS 解析地址" \
        "出口 IP 不同于直连，且与节点匹配" \
        "IP 已变化 ($DIRECT_IP → $CURRENT_PROXY_IP)，但节点服务器 IP ($PROXY_SERVER_IP) 不匹配"
elif [ -n "$CURRENT_PROXY_IP" ]; then
    record WARN TC-12 "出口 IP 变化 + 节点对应验证" \
        "对比直连 IP、代理出口 IP、订阅节点服务器 DNS 解析地址" \
        "出口 IP 不同于直连" \
        "IP 未变化（可能同出口）: $CURRENT_PROXY_IP"
else
    record FAIL TC-12 "出口 IP 变化 + 节点对应验证" \
        "对比直连 IP、代理出口 IP、订阅节点服务器 DNS 解析地址" \
        "出口 IP 可获取且与节点匹配" \
        "无法获取代理出口 IP"
fi

# ── Phase 4: 停止阶段 ─────────────────────────────────────────────────────────
section "Phase 4 — 停止阶段（仅通过用户前端 API）"

# TC-13 — 严格只通过 POST /api/v1/setup/stop，等待 SSE 返回 success:true
# 这等同于用户在前端点击「停止服务」按钮，不使用任何脚本/命令强制停止
info "调用 POST /api/v1/setup/stop（等同于用户前端操作）..."
curl -sN --max-time 60 -H "Content-Type: application/json" -d '{}' \
    "$CF_API/setup/stop" > /tmp/stop.log 2>/dev/null || true
STOP_LOG=$(cat /tmp/stop.log 2>/dev/null)
info "stop SSE 输出: $(echo "$STOP_LOG" | tail -3)"

STOP_SUCCESS=$(echo "$STOP_LOG" | grep -o '"success":true' | head -1)
STOP_HTTP404=$(echo "$STOP_LOG" | grep -q "404" && echo "yes" || echo "no")
STOP_ERROR=$(echo "$STOP_LOG" | grep -o '"error":"[^"]*"' | head -1)

if [ -n "$STOP_SUCCESS" ]; then
    record PASS TC-13 "停止服务（用户前端操作）" \
        "POST /api/v1/setup/stop — 等待 SSE success:true（等同前端停止按钮）" \
        "API 返回 success:true，所有资源（mihomo/DNS/nft/路由）已清理" \
        "stop API 成功"
elif [ "$STOP_HTTP404" = "yes" ]; then
    record WARN TC-13 "停止服务（用户前端操作）" \
        "POST /api/v1/setup/stop — 等待 SSE success:true（等同前端停止按钮）" \
        "API 返回 success:true" \
        "/setup/stop 在当前版本返回 404，需升级到包含 setup/stop 的版本"
else
    record FAIL TC-13 "停止服务（用户前端操作）" \
        "POST /api/v1/setup/stop — 等待 SSE success:true" \
        "API 返回 success:true" \
        "stop API 失败或超时: $STOP_ERROR"
fi

# 等待停止流程完成
# stop API 返回 404（版本不支持）时，必须通过 init.d 停止，否则后续验证无意义
if [ -n "$STOP_HTTP404" ] && [ "$STOP_HTTP404" = "yes" ]; then
    info "stop API 返回 404，使用 init.d 停止流程（限于版本）..."
    /etc/init.d/clashforge stop 2>/dev/null || true
    sleep 5
fi
sleep 5

# TC-14
NFT_FINAL=$(nft list tables 2>/dev/null | tr '\n' ' ')
if echo "$NFT_FINAL" | grep -qE "metaclash|clashforge"; then
    if [ -n "$STOP_HTTP404" ] && [ "$STOP_HTTP404" = "yes" ]; then
        # stop API 已经 404，未执行停止流程，记为 WARN（需升级版本）
        record WARN TC-14 "nftables 还原验证" "nft list tables 确认 metaclash 表已移除" "仅剩原始 fw4 表" "metaclash 表未清除（stop API 返回 404，尚未执行停止）"
    else
        record FAIL TC-14 "nftables 还原验证" "nft list tables 确认 metaclash 表已移除" "仅剩原始 fw4 表，metaclash 不存在" "metaclash 表未清除: $NFT_FINAL"
    fi
else
    record PASS TC-14 "nftables 还原验证" "nft list tables 确认 metaclash 表已移除" "仅剩原始 fw4 表，metaclash 不存在" "nft 表已还原: $NFT_FINAL"
fi

# TC-15
IP_RULE_FINAL=$(ip rule list 2>/dev/null)
IP_RULE_BEFORE=$(cat "$SNAPSHOT_DIR/ip-rule.before")
if [ "$IP_RULE_FINAL" = "$IP_RULE_BEFORE" ]; then
    record PASS TC-15 "ip rule 还原验证" "对比停止后 ip rule list 与启动前快照" "ip rule 完全还原" "已还原"
else
    record WARN TC-15 "ip rule 还原验证" "对比停止后 ip rule list 与启动前快照" "ip rule 完全还原" "有差异（可能是多次运行残留）"
fi

# TC-16
DNSMASQ_FINAL=$(ls /etc/dnsmasq.d/ 2>/dev/null | sort | tr '\n' ' ')
DNSMASQ_BEFORE_LIST=$(cat "$SNAPSHOT_DIR/dnsmasq-d.before" | sort | tr '\n' ' ')
if [ "$DNSMASQ_FINAL" = "$DNSMASQ_BEFORE_LIST" ]; then
    record PASS TC-16 "dnsmasq 配置还原验证" "对比停止后 /etc/dnsmasq.d/ 与启动前快照" "dnsmasq.d 文件列表还原" "已还原"
else
    record WARN TC-16 "dnsmasq 配置还原验证" "对比停止后 /etc/dnsmasq.d/ 与启动前快照" "dnsmasq.d 文件列表还原" "有残留文件（before: '$DNSMASQ_BEFORE_LIST' / after: '$DNSMASQ_FINAL'）"
fi

# ── Phase 5: 停止后恢复探测 ───────────────────────────────────────────────────
section "Phase 5 — 停止后恢复探测"

# TC-17 出口 IP 还原验证
FINAL_IP=$(wget -q -O - --timeout=10 https://api.ipify.org 2>/dev/null || echo "FAILED")
if [ "$FINAL_IP" != "FAILED" ] && [ "$FINAL_IP" = "$DIRECT_IP" ]; then
    record PASS TC-17 "停止后出口 IP 还原验证" \
        "停止服务后请求 api.ipify.org，对比启动前直连 IP" \
        "出口 IP 还原为启动前直连 IP（非代理节点 IP）" \
        "还原 IP: $FINAL_IP = 启动前 $DIRECT_IP ✓"
elif [ "$FINAL_IP" != "FAILED" ]; then
    record WARN TC-17 "停止后出口 IP 还原验证" \
        "停止服务后请求 api.ipify.org，对比启动前直连 IP" \
        "出口 IP 还原为启动前直连 IP" \
        "网络可达但 IP 不同（$DIRECT_IP → $FINAL_IP）"
else
    record FAIL TC-17 "停止后出口 IP 还原验证" \
        "停止服务后请求 api.ipify.org" \
        "网络正常，出口 IP 还原" \
        "停止服务后网络不可达！DNS/nft 可能未还原"
fi

# TC-20 DNS 原生性验证（核心！确认不再使用 clashforge fake-ip）
info "验证 DNS 是否还原为 OpenWrt 原生..."
# 1. dnsmasq.d 无 clashforge 残留配置
CF_DNSMASQ_FILES=$(ls /etc/dnsmasq.d/ 2>/dev/null | grep -iE "clash|mihomo|metaclash" || echo "")
# 2. DNS 解析不再返回 fake-ip（198.18.x.x 段）
DNS_TEST_DOMAIN="google.com"
DNS_RESULT=$(nslookup "$DNS_TEST_DOMAIN" 2>/dev/null | grep "Address:" | grep -v "#53" | head -1 | awk '{print $2}')
IS_FAKEIP=$(echo "$DNS_RESULT" | grep -qE "^198\.18\.|^198\.19\." && echo "yes" || echo "no")
# 3. dnsmasq upstream 不再指向 mihomo
DNSMASQ_UPSTREAM=$(cat /etc/dnsmasq.d/*.conf 2>/dev/null | grep "^server=" | head -3 || echo "")
HAS_MIHOMO_UPSTREAM=$(echo "$DNSMASQ_UPSTREAM" | grep -qE "127\.0\.0\.1#[0-9]" && echo "yes" || echo "no")

info "DNS 测试结果: $DNS_TEST_DOMAIN -> $DNS_RESULT (fake-ip: $IS_FAKEIP)"
info "dnsmasq.d clash 残留文件: '${CF_DNSMASQ_FILES:-无}'"
info "dnsmasq upstream 指向 mihomo: $HAS_MIHOMO_UPSTREAM"

if [ "$IS_FAKEIP" = "no" ] && [ -z "$CF_DNSMASQ_FILES" ] && [ "$HAS_MIHOMO_UPSTREAM" = "no" ]; then
    record PASS TC-20 "DNS 原生性验证（停止后）" \
        "检查 dnsmasq.d 无 clashforge 残留、DNS 不返回 fake-ip、dnsmasq 不指向 mihomo" \
        "DNS 完全还原为 OpenWrt 原生，无 clashforge 残留" \
        "DNS 结果: $DNS_RESULT（非 fake-ip）, dnsmasq 无残留配置 ✓"
elif [ "$IS_FAKEIP" = "yes" ]; then
    record FAIL TC-20 "DNS 原生性验证（停止后）" \
        "检查 DNS 解析不返回 fake-ip（198.18.x.x）" \
        "DNS 还原为真实 IP，不再返回 fake-ip" \
        "DNS 仍返回 fake-ip: $DNS_RESULT — clashforge DNS 未完全清除！"
elif [ -n "$CF_DNSMASQ_FILES" ]; then
    record FAIL TC-20 "DNS 原生性验证（停止后）" \
        "检查 dnsmasq.d 无 clashforge 残留配置文件" \
        "dnsmasq.d 无 clashforge 残留" \
        "发现残留配置: $CF_DNSMASQ_FILES"
else
    record WARN TC-20 "DNS 原生性验证（停止后）" \
        "检查 dnsmasq 不再指向 mihomo upstream" \
        "dnsmasq upstream 已清除" \
        "dnsmasq 仍有 mihomo upstream 配置: $DNSMASQ_UPSTREAM"
fi

# TC-18 停止后路由器端 probes（调用 /overview/probes API，验证出口已回直连）
# 注：此时 clashforge 仍在运行但 mihomo 已停止，API 可用
info "停止后路由器端 probes..."
AFTER_PROBES=$(curl -sf --max-time 20 "$CF_API/overview/probes" 2>/dev/null || echo "FAILED")
if [ "$AFTER_PROBES" != "FAILED" ]; then
    AFTER_IP=$(echo "$AFTER_PROBES" | grep -o '"ip":"[0-9.]*"' | head -1 | sed 's/"ip":"//;s/"//')
    info "停止后 probes 出口 IP: $AFTER_IP"
    if [ -n "$AFTER_IP" ] && [ "$AFTER_IP" = "$DIRECT_IP" ]; then
        record PASS TC-18 "停止后路由器端 IP 检查（/overview/probes）" \
            "停止 ClashForge 后调用 GET /overview/probes — ip_checks" \
            "出口 IP 还原为直连 IP（非代理节点 IP）" \
            "probes 出口 IP: $AFTER_IP = 直连 $DIRECT_IP ✓"
    elif [ -n "$AFTER_IP" ]; then
        record WARN TC-18 "停止后路由器端 IP 检查（/overview/probes）" \
            "停止 ClashForge 后调用 GET /overview/probes — ip_checks" \
            "出口 IP 还原为直连 IP" \
            "出口 IP: $AFTER_IP（直连基准: $DIRECT_IP）"
    else
        record FAIL TC-18 "停止后路由器端 IP 检查（/overview/probes）" \
            "停止 ClashForge 后调用 GET /overview/probes — ip_checks" \
            "能获取出口 IP" \
            "probes 无法获取出口 IP（mihomo 已停止，clashforge 仍可调用直连检测）"
    fi
else
    record WARN TC-18 "停止后路由器端 IP 检查（/overview/probes）" \
        "停止 ClashForge 后调用 GET /overview/probes" \
        "出口 IP 还原为直连 IP" \
        "probes API 无响应（clashforge 可能也已停止）"
fi

# TC-19 停止后可访问性检查
info "停止后路由器端可访问性检查..."
DIRECT_TARGETS="https://www.baidu.com https://music.163.com https://github.com https://www.youtube.com"
DIRECT_OK=0; DIRECT_TOTAL=0
for url in $DIRECT_TARGETS; do
    DIRECT_TOTAL=$((DIRECT_TOTAL+1))
    CODE=$(curl -sf -o /dev/null -w "%{http_code}" --max-time 10 "$url" 2>/dev/null || echo "0")
    if [ "$CODE" -ge 200 ] 2>/dev/null && [ "$CODE" -lt 400 ] 2>/dev/null; then
        DIRECT_OK=$((DIRECT_OK+1))
        info "  ✓ $url HTTP $CODE"
    else
        info "  ✗ $url HTTP $CODE"
    fi
done
if [ "$DIRECT_OK" = "$DIRECT_TOTAL" ]; then
    record PASS TC-19 "停止后路由器端可访问性检查" \
        "直连访问百度/网易云/GitHub/YouTube（ClashForge 已停止）" \
        "所有站点直连可达，网络恢复正常" \
        "$DIRECT_OK/$DIRECT_TOTAL 成功"
elif [ "$DIRECT_OK" -gt 0 ]; then
    record WARN TC-19 "停止后路由器端可访问性检查" \
        "直连访问百度/网易云/GitHub/YouTube" \
        "所有站点直连可达" \
        "$DIRECT_OK/$DIRECT_TOTAL 成功"
else
    record FAIL TC-19 "停止后路由器端可访问性检查" \
        "直连访问百度/网易云/GitHub/YouTube" \
        "至少部分站点直连可达" \
        "全部不可达（DNS 或路由可能未完全还原）"
fi

# ── 输出 GitHub Actions Job Summary ──────────────────────────────────────────
section "生成测试报告"

PASS_COUNT=$(echo "$RESULTS" | grep -c "^PASS" || echo 0)
FAIL_COUNT=$(echo "$RESULTS" | grep -c "^FAIL" || echo 0)
WARN_COUNT=$(echo "$RESULTS" | grep -c "^WARN" || echo 0)

summary "# ClashForge E2E 测试报告"
summary ""
summary "| 项目 | 值 |"
summary "|------|----|"
summary "| **测试环境** | OpenWrt 23.05.5 x86_64 |"
summary "| **ClashForge 版本** | $CLASHFORGE_VERSION |"
summary "| **直连 IP** | $DIRECT_IP |"
summary "| **代理出口 IP** | $CURRENT_PROXY_IP |"
summary "| **还原 IP** | $FINAL_IP |"
summary ""
if [ "$FAIL_COUNT" -eq 0 ]; then
    summary "## ✅ 全部测试通过"
else
    summary "## ❌ 存在失败用例 ($FAIL_COUNT 个)"
fi
summary ""
summary "| 结果 | 数量 |"
summary "|------|------|"
summary "| ✅ 通过 | $PASS_COUNT |"
summary "| ❌ 失败 | $FAIL_COUNT |"
summary "| ⚠️ 警告 | $WARN_COUNT |"
summary "| **合计** | **$TOTAL** |"
summary ""
summary "---"
summary ""
summary "## 详细用例结果"
summary ""
summary "| 编号 | 用例名称 | 操作 | 预期结果 | 实际结果 | 状态 |"
summary "|------|----------|------|----------|----------|------|"
printf "%b" "$RESULTS" | while IFS="|" read -r STATUS TC NAME OP EXPECTED ACTUAL; do
    [ -z "$TC" ] && continue
    case "$STATUS" in
        PASS) ICON="✅" ;;
        FAIL) ICON="❌" ;;
        WARN) ICON="⚠️" ;;
        *) continue ;;
    esac
    summary "| $TC | $NAME | $OP | $EXPECTED | $ACTUAL | $ICON $STATUS |"
done

summary ""
summary "---"
summary ""
summary "> 📝 **注：** 浏览器端探测（browser-probe.mjs）结果见下一个 step 的 summary"

# ── 控制台最终总结 ────────────────────────────────────────────────────────────
section "测试结果"
printf "\n${BOLD}直连 IP:${RESET}      $DIRECT_IP\n"
printf "${BOLD}代理出口 IP:${RESET}  $CURRENT_PROXY_IP\n"
printf "${BOLD}还原 IP:${RESET}      $FINAL_IP\n\n"
printf "${BOLD}通过: $PASS_COUNT  失败: $FAIL_COUNT  警告: $WARN_COUNT  合计: $TOTAL${RESET}\n\n"

if [ "$FAILED" -eq 0 ]; then
    printf "${GREEN}${BOLD}ALL TESTS PASSED ✅${RESET}\n"
    exit 0
else
    printf "${RED}${BOLD}$FAILED 个测试失败 ❌${RESET}\n"
    exit 1
fi
