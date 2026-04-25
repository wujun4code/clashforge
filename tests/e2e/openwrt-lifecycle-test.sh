#!/bin/sh
# tests/e2e/openwrt-lifecycle-test.sh
# ClashForge 完整生命周期 e2e 测试脚本
# 在 OpenWrt VM 内运行
#
# 测试流程：
#   1. 验证订阅 URL 可达
#   2. 记录启动前状态快照 (DNS/nft/routing)
#   3. 安装 clashforge（指定版本）
#   4. 启动服务 + 添加订阅 + 拉取节点
#   5. 触发 launch（DNS + 透明代理）
#   6. 验证 DNS 接管
#   7. 验证 nft 规则
#   8. 连通性测试（IP 变化 + 出口验证）
#   9. 停止服务
#  10. 验证 DNS/nft 还原 + 网络恢复
#
# 环境变量：
#   CLASHFORGE_VERSION   安装版本 (default: latest)
#   SUBSCRIPTION_URL     订阅 URL（必须）

set -e

# ── 颜色 ──────────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

FAILED=0

pass()    { printf "${GREEN}✅ PASS${RESET} %s\n" "$*"; }
fail()    { printf "${RED}❌ FAIL${RESET} %s\n" "$*"; FAILED=$((FAILED+1)); }
info()    { printf "${CYAN}ℹ️   ${RESET} %s\n" "$*"; }
section() { printf "\n${BOLD}${YELLOW}=== %s ===${RESET}\n" "$*"; }
warn()    { printf "${YELLOW}⚠️  WARN${RESET} %s\n" "$*"; }

# ── 配置 ──────────────────────────────────────────────────────────────────────
CLASHFORGE_VERSION="${CLASHFORGE_VERSION:-latest}"
SUBSCRIPTION_URL="${SUBSCRIPTION_URL:-}"
CLASHFORGE_API="http://127.0.0.1:7777/api/v1"
SNAPSHOT_DIR="/tmp/cf-test-snapshot"
SUB_ID=""

# ── 辅助函数 ──────────────────────────────────────────────────────────────────

wait_http() {
    url="$1"; max="$2"
    i=0
    while [ $i -lt "$max" ]; do
        if wget -q -O /dev/null --timeout=2 "$url" 2>/dev/null; then
            return 0
        fi
        sleep 1; i=$((i+1))
    done
    return 1
}

json_get() {
    echo "$1" | grep -o "\"$2\":[^,}]*" | head -1 | sed 's/.*: *"\{0,1\}\([^",}]*\).*/\1/'
}

# ── Step 0: 验证订阅 URL ──────────────────────────────────────────────────────
section "Step 0: 验证订阅 URL"

if [ -z "$SUBSCRIPTION_URL" ]; then
    fail "未提供订阅 URL：请设置 SUBSCRIPTION_URL 环境变量"
    exit 1
fi

info "订阅 URL: $SUBSCRIPTION_URL"

SUB_CHECK=$(wget -q -O - --timeout=15 "$SUBSCRIPTION_URL" 2>/dev/null | head -5)
if echo "$SUB_CHECK" | grep -qE "port|proxies|---"; then
    NODE_COUNT=$(wget -q -O - --timeout=15 "$SUBSCRIPTION_URL" 2>/dev/null | grep -c "^  - name:" || echo 0)
    pass "订阅 URL 可达，节点数: $NODE_COUNT"
else
    fail "订阅 URL 无效或无法访问"
    exit 1
fi

# ── Step 1: 记录启动前状态快照 ───────────────────────────────────────────────
section "Step 1: 记录启动前状态"

mkdir -p "$SNAPSHOT_DIR"

cp /etc/resolv.conf "$SNAPSHOT_DIR/resolv.conf.before" 2>/dev/null || true
uci export dhcp > "$SNAPSHOT_DIR/dhcp.uci.before" 2>/dev/null || true
nft list ruleset > "$SNAPSHOT_DIR/nft.before" 2>/dev/null || echo "" > "$SNAPSHOT_DIR/nft.before"
ip rule list > "$SNAPSHOT_DIR/ip-rule.before"
ip route list > "$SNAPSHOT_DIR/ip-route.before"
ls /etc/dnsmasq.d/ > "$SNAPSHOT_DIR/dnsmasq-d.before" 2>/dev/null || echo "" > "$SNAPSHOT_DIR/dnsmasq-d.before"

pass "启动前状态已记录到 $SNAPSHOT_DIR"
info "启动前 nft 表: $(nft list tables 2>/dev/null | tr '\n' ' ' || echo 'none')"
info "启动前 DNS: $(grep nameserver /etc/resolv.conf | tr '\n' ' ')"

DIRECT_IP=$(wget -q -O - --timeout=10 https://api.ipify.org 2>/dev/null || echo "FAILED")
if [ "$DIRECT_IP" != "FAILED" ]; then
    echo "$DIRECT_IP" > "$SNAPSHOT_DIR/direct-ip"
    pass "直连 IP: $DIRECT_IP"
else
    fail "无法获取直连 IP，网络异常"
fi

# ── Step 2: 安装 clashforge ───────────────────────────────────────────────────
section "Step 2: 安装 clashforge ($CLASHFORGE_VERSION)"

if command -v clashforge > /dev/null 2>&1; then
    CURRENT_VER=$(clashforge --version 2>/dev/null | head -1 || echo "unknown")
    info "已安装版本: $CURRENT_VER，跳过安装"
else
    info "开始安装..."
    if [ "$CLASHFORGE_VERSION" = "latest" ]; then
        wget -qO- https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh
    else
        wget -qO- https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh -s -- --version "$CLASHFORGE_VERSION"
    fi
fi

# 确保 tproxy 内核模块和 curl 已安装
info "检查依赖模块..."
opkg install kmod-nft-tproxy kmod-nf-tproxy curl 2>/dev/null | grep -v "up to date" || true
modprobe nft_tproxy 2>/dev/null || true
if lsmod | grep -q nft_tproxy; then
    pass "nft_tproxy 内核模块已加载"
else
    warn "nft_tproxy 模块未加载，tproxy 模式可能失败"
fi

if command -v clashforge > /dev/null 2>&1; then
    pass "clashforge 安装成功"
else
    fail "clashforge 安装失败"
    exit 1
fi

# ── Step 3: 启动服务 + 添加订阅 ──────────────────────────────────────────────
section "Step 3: 启动 clashforge + 加载订阅"

/etc/init.d/clashforge start 2>/dev/null || true

info "等待 clashforge API 就绪..."
if wait_http "$CLASHFORGE_API/status" 30; then
    pass "clashforge API 就绪"
else
    fail "clashforge API 30 秒内未响应"
    exit 1
fi

# 添加订阅
info "添加订阅..."
ADD_RESP=$(curl -sf --max-time 15 \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"$SUBSCRIPTION_URL\",\"name\":\"e2e-test\",\"enabled\":true}" \
    "$CLASHFORGE_API/subscriptions" 2>/dev/null || echo "FAILED")

if [ "$ADD_RESP" = "FAILED" ]; then
    fail "添加订阅失败"
    exit 1
fi

# 提取订阅 ID
SUB_ID=$(echo "$ADD_RESP" | grep -o '"id":"[^"]*"' | head -1 | sed 's/"id":"//;s/"//')
info "订阅 ID: $SUB_ID"

if [ -z "$SUB_ID" ]; then
    fail "无法获取订阅 ID，响应: $ADD_RESP"
    exit 1
fi

# 同步拉取订阅
info "拉取订阅节点..."
SYNC_RESP=$(curl -sf --max-time 30 \
    -H "Content-Type: application/json" \
    -d '{}' \
    "$CLASHFORGE_API/subscriptions/$SUB_ID/sync-update" 2>/dev/null || echo "FAILED")

if echo "$SYNC_RESP" | grep -qE "ok|true|success|nodes"; then
    pass "订阅节点拉取成功"
else
    warn "订阅同步响应: $SYNC_RESP"
fi

sleep 3

# 触发 launch（DNS + 透明代理）
info "触发 setup launch（DNS + nftables tproxy）..."
curl -sf --max-time 60 \
    -H "Content-Type: application/json" \
    -d '{"dns":{"enable":true,"mode":"fake-ip","dnsmasq_mode":"upstream","apply_on_start":true},"network":{"mode":"tproxy","firewall_backend":"nftables","bypass_lan":true,"bypass_china":false,"apply_on_start":true}}' \
    "$CLASHFORGE_API/setup/launch" > /tmp/launch.log 2>/dev/null || true

info "launch 输出: $(cat /tmp/launch.log | tail -3)"
sleep 5

# ── Step 4: 验证 DNS 接管 ─────────────────────────────────────────────────────
section "Step 4: 验证 DNS 接管"

CURRENT_DNS=$(grep nameserver /etc/resolv.conf | head -1 | awk '{print $2}')
info "当前 DNS: $CURRENT_DNS"

if grep -rE "server=127.0.0.1|127.0.0.1#1053|127.0.0.1#7874" /etc/dnsmasq.d/ /tmp/dnsmasq.d/ 2>/dev/null | grep -qE "server"; then
    pass "dnsmasq 已配置指向 mihomo DNS"
else
    DNS_TEST=$(nslookup google.com 2>/dev/null | grep "Address" | tail -1 || echo "")
    if [ -n "$DNS_TEST" ]; then
        pass "DNS 解析正常: $DNS_TEST"
    else
        warn "dnsmasq → mihomo 路径不确定"
    fi
fi

DNSMASQ_AFTER=$(ls /etc/dnsmasq.d/ 2>/dev/null | tr '\n' ' ')
DNSMASQ_BEFORE=$(cat "$SNAPSHOT_DIR/dnsmasq-d.before" | tr '\n' ' ')
if [ "$DNSMASQ_AFTER" != "$DNSMASQ_BEFORE" ]; then
    pass "dnsmasq.d 配置已变化（接管生效）"
else
    info "dnsmasq.d 无变化，DNS 可能通过其他方式接管"
fi

# ── Step 5: 验证 nft 规则 ─────────────────────────────────────────────────────
section "Step 5: 验证 nftables 接管"

NFT_TABLES=$(nft list tables 2>/dev/null | tr '\n' ' ')
info "当前 nft 表: $NFT_TABLES"

if echo "$NFT_TABLES" | grep -qE "metaclash|clashforge|tproxy"; then
    pass "nftables 透明代理表已建立"
else
    fail "nftables 透明代理表未找到"
fi

if nft list ruleset 2>/dev/null | grep -qE "tproxy|redirect|mark"; then
    pass "nftables tproxy 规则存在"
else
    warn "tproxy 规则未确认"
fi

if ip rule list 2>/dev/null | grep -qE "fwmark|lookup"; then
    pass "ip rule 策略路由已配置"
else
    warn "ip rule 策略路由未找到"
fi

# ── Step 6: 连通性测试 ────────────────────────────────────────────────────────
section "Step 6: 连通性测试"

sleep 3

# 从 mihomo 生成的配置中读取代理认证信息
PROXY_AUTH=""
CONFIG_CACHE=$(ls /etc/metaclash/cache/*.raw.yaml 2>/dev/null | tail -1)
if [ -n "$CONFIG_CACHE" ]; then
    AUTH_LINE=$(grep -A1 'authentication:' "$CONFIG_CACHE" 2>/dev/null | grep -v 'authentication:' | head -1 | tr -d ' -')
    if [ -n "$AUTH_LINE" ]; then
        PROXY_AUTH="$AUTH_LINE"
        info "代理认证: $PROXY_AUTH"
    fi
fi

if [ -n "$PROXY_AUTH" ]; then
    PROXY_IP=$(curl -sf --max-time 15 \
        --proxy "http://${PROXY_AUTH}@127.0.0.1:7890" \
        https://api.ipify.org 2>/dev/null || echo "FAILED")
else
    PROXY_IP=$(curl -sf --max-time 15 \
        --proxy http://127.0.0.1:7890 \
        https://api.ipify.org 2>/dev/null || echo "FAILED")
fi

DIRECT_IP=$(cat "$SNAPSHOT_DIR/direct-ip" 2>/dev/null || echo "unknown")

if [ "$PROXY_IP" = "FAILED" ]; then
    fail "代理连通性测试失败（无法通过 :7890 访问外网）"
else
    pass "代理出口 IP: $PROXY_IP"

    if [ "$PROXY_IP" != "$DIRECT_IP" ] && [ "$DIRECT_IP" != "unknown" ]; then
        pass "IP 已变化: 直连=$DIRECT_IP → 代理=$PROXY_IP ✓"
    else
        warn "IP 未变化（节点可能和本机同出口）"
    fi

    if [ -n "$PROXY_AUTH" ]; then
        GEO=$(curl -sf --max-time 10 \
            --proxy "http://${PROXY_AUTH}@127.0.0.1:7890" \
            "https://ipinfo.io/$PROXY_IP/json" 2>/dev/null || echo "")
    else
        GEO=$(curl -sf --max-time 10 \
            --proxy http://127.0.0.1:7890 \
            "https://ipinfo.io/$PROXY_IP/json" 2>/dev/null || echo "")
    fi
    COUNTRY=$(json_get "$GEO" "country")
    CITY=$(json_get "$GEO" "city")
    info "代理出口位置: $COUNTRY / $CITY"
fi

# mihomo API 验证
MIHOMO_VER=$(wget -q -O - --timeout=5 "http://127.0.0.1:9090/version" 2>/dev/null || echo "FAILED")
if [ "$MIHOMO_VER" != "FAILED" ]; then
    pass "mihomo API 响应正常"
else
    fail "mihomo API 无响应"
fi

# ── Step 7: 停止服务 ──────────────────────────────────────────────────────────
section "Step 7: 停止 clashforge 服务"

curl -sf --max-time 30 \
    -H "Content-Type: application/json" \
    -d '{}' \
    "$CLASHFORGE_API/setup/stop" > /dev/null 2>/dev/null || true

/etc/init.d/clashforge stop 2>/dev/null || true
sleep 5

if pgrep -f "clashforge\|mihomo" > /dev/null 2>&1; then
    warn "进程仍在运行，强制终止..."
    pkill -f "clashforge" 2>/dev/null || true
    pkill -f "mihomo-clashforge" 2>/dev/null || true
    sleep 2
fi

if ! pgrep -f "clashforge\|mihomo-clashforge" > /dev/null 2>&1; then
    pass "clashforge / mihomo 进程已退出"
else
    fail "进程未完全退出"
fi

# ── Step 8: 验证 DNS/nft 还原 ────────────────────────────────────────────────
section "Step 8: 验证 DNS/nft 还原"

sleep 3

NFT_TABLES_FINAL=$(nft list tables 2>/dev/null | tr '\n' ' ')
info "停止后 nft 表: $NFT_TABLES_FINAL"

if echo "$NFT_TABLES_FINAL" | grep -qE "metaclash|clashforge"; then
    fail "nftables metaclash 表未清除，状态未还原"
else
    pass "nftables metaclash 表已移除"
fi

IP_RULE_FINAL=$(ip rule list 2>/dev/null)
IP_RULE_BEFORE=$(cat "$SNAPSHOT_DIR/ip-rule.before")
if [ "$IP_RULE_FINAL" = "$IP_RULE_BEFORE" ]; then
    pass "ip rule 已还原到启动前状态"
else
    warn "ip rule 有变化（可能未完全还原）"
fi

DNSMASQ_FINAL=$(ls /etc/dnsmasq.d/ 2>/dev/null | tr '\n' ' ')
if [ "$DNSMASQ_FINAL" = "$(cat "$SNAPSHOT_DIR/dnsmasq-d.before" | tr '\n' ' ')" ]; then
    pass "dnsmasq.d 已还原"
else
    warn "dnsmasq.d 有残留: $DNSMASQ_FINAL"
fi

FINAL_IP=$(wget -q -O - --timeout=10 https://api.ipify.org 2>/dev/null || echo "FAILED")
if [ "$FINAL_IP" != "FAILED" ]; then
    pass "停止服务后网络正常: $FINAL_IP"
    if [ "$FINAL_IP" = "$DIRECT_IP" ]; then
        pass "IP 还原: $FINAL_IP = 启动前 $DIRECT_IP ✓"
    else
        warn "IP 与启动前不同（$DIRECT_IP → $FINAL_IP），但网络可用"
    fi
else
    fail "停止服务后网络不可达！DNS/nft 可能未完全还原"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
section "测试结果"

printf "\n${BOLD}直连 IP:${RESET}  $DIRECT_IP\n"
printf "${BOLD}代理 IP:${RESET}  $PROXY_IP\n"
printf "${BOLD}还原 IP:${RESET}  $FINAL_IP\n\n"

if [ "$FAILED" -eq 0 ]; then
    printf "${GREEN}${BOLD}ALL TESTS PASSED ✅${RESET}\n"
    exit 0
else
    printf "${RED}${BOLD}$FAILED 个测试失败 ❌${RESET}\n"
    exit 1
fi
