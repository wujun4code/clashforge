#!/bin/sh
# stop-clashforge.sh
# 完全停止 ClashForge：init.d 服务、mihomo-clashforge 内核进程、
# nftables metaclash 表、ip rule/route、dnsmasq DNS 劫持恢复
# 适用于 OpenWrt + ClashForge (nftables tproxy 模式)
# 执行前确认网络环境，停止后透明代理失效，流量走正常路由。

set -e

log() { echo "[$(date '+%H:%M:%S')] $1"; }

# ── 1. 停止 init.d 服务 ────────────────────────────────────────────────────────
log "停止 clashforge init.d 服务..."
if /etc/init.d/clashforge stop 2>/dev/null; then
    log "  clashforge stop 完成"
else
    log "  clashforge stop 返回非零（可能已停止，继续）"
fi

# 等待服务自行清理
sleep 1

# ── 2. 杀掉 clashforge 主进程 ─────────────────────────────────────────────────
log "终止 clashforge 主进程..."
CF_PIDS=$(pgrep -f "/usr/bin/clashforge" 2>/dev/null || true)
if [ -n "$CF_PIDS" ]; then
    # shellcheck disable=SC2086
    kill $CF_PIDS 2>/dev/null || true
    sleep 1
    CF_PIDS=$(pgrep -f "/usr/bin/clashforge" 2>/dev/null || true)
    if [ -n "$CF_PIDS" ]; then
        # shellcheck disable=SC2086
        kill -9 $CF_PIDS 2>/dev/null || true
        log "  clashforge 主进程已强制终止"
    else
        log "  clashforge 主进程已正常退出"
    fi
else
    log "  clashforge 主进程未运行，跳过"
fi

# ── 3. 杀掉 mihomo-clashforge 内核进程 ────────────────────────────────────────
log "终止 mihomo-clashforge 内核进程..."
MH_PIDS=$(pgrep -f "/usr/bin/mihomo-clashforge" 2>/dev/null || true)
if [ -n "$MH_PIDS" ]; then
    # shellcheck disable=SC2086
    kill $MH_PIDS 2>/dev/null || true
    sleep 1
    MH_PIDS=$(pgrep -f "/usr/bin/mihomo-clashforge" 2>/dev/null || true)
    if [ -n "$MH_PIDS" ]; then
        # shellcheck disable=SC2086
        kill -9 $MH_PIDS 2>/dev/null || true
        log "  mihomo-clashforge 已强制终止"
    else
        log "  mihomo-clashforge 已正常退出"
    fi
else
    log "  mihomo-clashforge 未运行，跳过"
fi

# ── 4. 清理 nftables metaclash 表 ────────────────────────────────────────────
# ClashForge 使用独立的 table inet metaclash，包含：
#   chain dns_redirect  (nat hook prerouting — DNS 劫持到 :7874)
#   chain tproxy_prerouting  (mangle hook prerouting — TProxy 到 127.0.0.1:7895)
#   set bypass_ipv4
log "清理 nftables table inet metaclash..."
if nft list table inet metaclash >/dev/null 2>&1; then
    nft delete table inet metaclash 2>/dev/null \
        && log "  table inet metaclash 已删除" \
        || log "  删除失败，尝试逐 chain flush..."
    # 若整表删除失败，逐 chain 清理
    for CHAIN in dns_redirect tproxy_prerouting; do
        nft flush chain inet metaclash "$CHAIN" 2>/dev/null || true
        nft delete chain inet metaclash "$CHAIN" 2>/dev/null || true
    done
    nft delete table inet metaclash 2>/dev/null \
        && log "  table inet metaclash 已删除（二次尝试）" \
        || log "  table inet metaclash 清理失败，请手动执行: nft delete table inet metaclash"
else
    log "  table inet metaclash 不存在，跳过"
fi

# ── 5. 清理策略路由规则与路由表 ───────────────────────────────────────────────
# ClashForge 添加：ip rule fwmark 0x1a3 lookup 100
# 以及 ip route add local default dev lo table 100
MARK_HEX="0x1a3"
ROUTE_TABLE=100

log "清理策略路由规则 fwmark $MARK_HEX..."
while ip rule show 2>/dev/null | grep -q "fwmark $MARK_HEX"; do
    ip rule del fwmark "$MARK_HEX" lookup "$ROUTE_TABLE" 2>/dev/null \
        && log "  ip rule fwmark $MARK_HEX lookup $ROUTE_TABLE 已删除" \
        || { log "  ip rule 删除失败，停止重试"; break; }
done

log "清理路由表 $ROUTE_TABLE 中的本地路由..."
if ip route show table "$ROUTE_TABLE" 2>/dev/null | grep -q .; then
    ip route flush table "$ROUTE_TABLE" 2>/dev/null \
        && log "  路由表 $ROUTE_TABLE 已清空" \
        || log "  路由表 $ROUTE_TABLE 清空失败，忽略"
else
    log "  路由表 $ROUTE_TABLE 为空，跳过"
fi

# ── 6. 恢复 dnsmasq DNS 端口 ──────────────────────────────────────────────────
# ClashForge 写入 /etc/dnsmasq.d/clashforge.conf 内容为 "port=0"
# 使 dnsmasq 不监听 DNS，由 mihomo 接管 53 端口
# 删除该文件后重启 dnsmasq 即可恢复
DNSMASQ_CONF="/etc/dnsmasq.d/clashforge.conf"
log "恢复 dnsmasq DNS 端口..."
if [ -f "$DNSMASQ_CONF" ]; then
    rm -f "$DNSMASQ_CONF" \
        && log "  已删除 $DNSMASQ_CONF" \
        || log "  删除 $DNSMASQ_CONF 失败，请手动删除"
else
    log "  $DNSMASQ_CONF 不存在，跳过"
fi

log "重启 dnsmasq 恢复正常 DNS 监听..."
/etc/init.d/dnsmasq restart 2>/dev/null \
    && log "  dnsmasq 已重启" \
    || log "  dnsmasq 重启失败，请手动执行: /etc/init.d/dnsmasq restart"

# ── 7. 清理 pid 文件 ──────────────────────────────────────────────────────────
PID_FILE="/var/run/metaclash/metaclash.pid"
if [ -f "$PID_FILE" ]; then
    rm -f "$PID_FILE" 2>/dev/null \
        && log "  pid 文件已清理" \
        || true
fi

# ── 8. 验证结果 ───────────────────────────────────────────────────────────────
log "--- 验证结果 ---"

REMAINING=$(pgrep -f "clashforge\|mihomo-clashforge" 2>/dev/null || true)
if [ -n "$REMAINING" ]; then
    log "  警告：仍有残留进程 PID: $REMAINING"
else
    log "  clashforge / mihomo-clashforge 进程已全部清除"
fi

if nft list table inet metaclash >/dev/null 2>&1; then
    log "  警告：table inet metaclash 仍然存在，请手动执行: nft delete table inet metaclash"
else
    log "  nftables metaclash 表已清除"
fi

if ip rule show 2>/dev/null | grep -q "fwmark $MARK_HEX"; then
    log "  警告：策略路由规则 fwmark $MARK_HEX 仍然存在"
else
    log "  策略路由规则已清除"
fi

DNS_PORT=$(netstat -lnup 2>/dev/null | grep ':53 ' | head -1 || true)
if [ -n "$DNS_PORT" ]; then
    log "  DNS 端口 53 已恢复监听: $DNS_PORT"
else
    log "  警告：DNS 端口 53 暂未监听，dnsmasq 可能仍在重启中"
fi

log "完成。ClashForge 透明代理已停止，流量走正常路由，DNS 已恢复由 dnsmasq 处理。"
