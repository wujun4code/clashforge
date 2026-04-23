#!/bin/sh
# stop-openclash.sh
# 完全停止 OpenClash：init.d 服务、watchdog、clash 内核进程、nftables 规则清理
# 适用于 OpenWrt + OpenClash (nftables 模式)
# 执行前建议先确认网络环境，停止后透明代理失效，流量将走正常路由。

set -e

log() { echo "[$(date '+%H:%M:%S')] $1"; }

# ── 1. 停止 init.d 服务 ────────────────────────────────────────────────────────
log "停止 openclash init.d 服务..."
if /etc/init.d/openclash stop 2>/dev/null; then
    log "  openclash stop 完成"
else
    log "  openclash stop 返回非零（可能已停止，继续）"
fi

# ── 2. 禁止 openclash 随路由器自启（可选，注释掉则只停止本次运行）─────────────
# /etc/init.d/openclash disable

# ── 3. 杀掉 watchdog（防止它重新拉起 clash 进程）──────────────────────────────
log "终止 openclash watchdog..."
WATCHDOG_PIDS=$(pgrep -f "openclash_watchdog" 2>/dev/null || true)
if [ -n "$WATCHDOG_PIDS" ]; then
    # shellcheck disable=SC2086
    kill $WATCHDOG_PIDS 2>/dev/null || true
    sleep 1
    # 确认已退出，必要时强制
    WATCHDOG_PIDS=$(pgrep -f "openclash_watchdog" 2>/dev/null || true)
    if [ -n "$WATCHDOG_PIDS" ]; then
        # shellcheck disable=SC2086
        kill -9 $WATCHDOG_PIDS 2>/dev/null || true
        log "  watchdog 已强制终止"
    else
        log "  watchdog 已正常退出"
    fi
else
    log "  watchdog 未运行，跳过"
fi

# ── 4. 杀掉 clash 内核进程 ─────────────────────────────────────────────────────
log "终止 clash 内核进程..."
CLASH_PIDS=$(pgrep -f "/etc/openclash/clash" 2>/dev/null || true)
if [ -n "$CLASH_PIDS" ]; then
    # shellcheck disable=SC2086
    kill $CLASH_PIDS 2>/dev/null || true
    sleep 1
    CLASH_PIDS=$(pgrep -f "/etc/openclash/clash" 2>/dev/null || true)
    if [ -n "$CLASH_PIDS" ]; then
        # shellcheck disable=SC2086
        kill -9 $CLASH_PIDS 2>/dev/null || true
        log "  clash 内核已强制终止"
    else
        log "  clash 内核已正常退出"
    fi
else
    log "  clash 内核未运行，跳过"
fi

# ── 5. 清理 nftables 中的 OpenClash 规则链 ────────────────────────────────────
# OpenClash 在 table inet fw4 中注入以下 chain / jump rule：
#   openclash, openclash_mangle, openclash_mangle_output
#   openclash_output, openclash_upnp, openclash_wan_input
# 先删除 fw4 中跳转到这些 chain 的 rule，再删除 chain 本身。
log "清理 nftables OpenClash 规则..."

OPENCLASH_CHAINS="openclash openclash_mangle openclash_mangle_output openclash_output openclash_upnp openclash_wan_input"

for CHAIN in $OPENCLASH_CHAINS; do
    # 删除所有 jump 到该 chain 的规则（跨 chain 引用）
    HANDLES=$(nft -a list table inet fw4 2>/dev/null \
        | grep "jump ${CHAIN}" \
        | grep -oE 'handle [0-9]+' \
        | awk '{print $2}' || true)
    for H in $HANDLES; do
        # 找到规则所在的 chain 名称，需要从上下文反查
        PARENT_CHAIN=$(nft -a list table inet fw4 2>/dev/null \
            | awk "/chain /{cur=\$2} /jump ${CHAIN}.*handle ${H}/{print cur}" || true)
        if [ -n "$PARENT_CHAIN" ]; then
            nft delete rule inet fw4 "$PARENT_CHAIN" handle "$H" 2>/dev/null \
                && log "  删除 inet fw4 $PARENT_CHAIN 中 jump $CHAIN (handle $H)" \
                || true
        fi
    done
    # 删除 chain 本身（先 flush 清空规则再删除）
    if nft list chain inet fw4 "$CHAIN" >/dev/null 2>&1; then
        nft flush chain inet fw4 "$CHAIN" 2>/dev/null || true
        nft delete chain inet fw4 "$CHAIN" 2>/dev/null \
            && log "  删除 chain inet fw4 $CHAIN" \
            || log "  chain inet fw4 $CHAIN 删除失败（可能仍被引用，忽略）"
    fi
done

# 清理 OpenClash 在 fw4 中插入的 ICMP / QUIC reject 规则（通过 comment 识别）
log "清理 fw4 中 OpenClash comment 标记的规则..."
HANDLES=$(nft -a list table inet fw4 2>/dev/null \
    | grep 'comment "OpenClash' \
    | grep -oE 'handle [0-9]+' \
    | awk '{print $2}' || true)
for H in $HANDLES; do
    PARENT_CHAIN=$(nft -a list table inet fw4 2>/dev/null \
        | awk "/chain /{cur=\$2} /comment \"OpenClash.*handle ${H}/{print cur}" || true)
    if [ -n "$PARENT_CHAIN" ]; then
        nft delete rule inet fw4 "$PARENT_CHAIN" handle "$H" 2>/dev/null \
            && log "  删除 OpenClash 标记规则 $PARENT_CHAIN handle $H" \
            || true
    fi
done

# ── 6. 清理 TUN / 虚拟网卡（如果存在）───────────────────────────────────────
log "检查并清理 TUN 网卡..."
for IFACE in utun0 utun tun0; do
    if ip link show "$IFACE" >/dev/null 2>&1; then
        ip link set "$IFACE" down 2>/dev/null || true
        ip link delete "$IFACE" 2>/dev/null \
            && log "  删除网卡 $IFACE" \
            || log "  $IFACE 删除失败（可能由内核管理，忽略）"
    fi
done

# ── 7. 清理 fake-ip 路由（198.18.0.0/16）─────────────────────────────────────
log "清理 fake-ip 路由..."
if ip route show 198.18.0.0/16 2>/dev/null | grep -q .; then
    ip route del 198.18.0.0/16 2>/dev/null \
        && log "  删除 198.18.0.0/16 路由" \
        || log "  fake-ip 路由删除失败，忽略"
else
    log "  无 fake-ip 路由，跳过"
fi

# ── 8. 重载 dnsmasq（恢复正常 DNS）───────────────────────────────────────────
log "重载 dnsmasq..."
/etc/init.d/dnsmasq reload 2>/dev/null \
    && log "  dnsmasq 已重载" \
    || log "  dnsmasq reload 失败，尝试 restart..."
# 如果 reload 失败则 restart
/etc/init.d/dnsmasq status 2>/dev/null | grep -q running \
    || /etc/init.d/dnsmasq restart 2>/dev/null || true

# ── 9. 验证结果 ───────────────────────────────────────────────────────────────
log "--- 验证结果 ---"
REMAINING=$(pgrep -f "openclash\|/etc/openclash/clash" 2>/dev/null || true)
if [ -n "$REMAINING" ]; then
    log "  警告：仍有残留进程: $REMAINING"
else
    log "  clash / openclash 进程已全部清除"
fi

NFT_REMAINING=$(nft list table inet fw4 2>/dev/null | grep -c "openclash" || true)
if [ "$NFT_REMAINING" -gt 0 ]; then
    log "  警告：fw4 中仍有 $NFT_REMAINING 条 openclash 相关规则（可能为正常残留引用）"
else
    log "  nftables fw4 中 OpenClash 规则已清除"
fi

log "完成。透明代理已停止，流量现在走正常路由。"
