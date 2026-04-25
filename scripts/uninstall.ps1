# uninstall.ps1 — Completely remove ClashForge from an OpenWrt router
#
# Restores everything clashforge touched back to its pre-install state:
#   - Stops clashforge service + kills all processes (clashforge, mihomo-clashforge)
#   - Removes nftables table inet metaclash  (tproxy + dns_redirect chains)
#   - Removes nftables table inet dnsmasq    (the hijack table dnsmasq re-injects)
#   - Cleans up ip rule fwmark 0x1a3 + route table 100 (IPv4 + IPv6)
#   - Restores dnsmasq via UCI (deletes port=0, server=, noresolv= overrides)
#   - Removes /etc/dnsmasq.d/clashforge.conf  (non-UCI fallback)
#   - Restarts dnsmasq so port 53 is listening again
#   - Removes the opkg package (clashforge)
#   - Wipes all data: /etc/metaclash  /usr/share/metaclash  /var/run/metaclash
#                     /var/log/clashforge.log
#
# Usage:
#   .\uninstall.ps1 -Router 192.168.10.1
#   .\uninstall.ps1 -Router 192.168.10.1 -User root -Port 22
#   .\uninstall.ps1 -Router 192.168.10.1 -KeepConfig   # skip wiping /etc/metaclash

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$Router,

    [string]$User       = "root",
    [int]$Port          = 22,
    [switch]$KeepConfig   # keep /etc/metaclash (subscription + override data)
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Log  { param($msg) Write-Host "[uninstall] $msg" -ForegroundColor Cyan }
function Ok   { param($msg) Write-Host "[uninstall] $msg" -ForegroundColor Green }
function Warn { param($msg) Write-Host "[uninstall] WARN: $msg" -ForegroundColor Yellow }
function Die  { param($msg) Write-Host "[uninstall] ERROR: $msg" -ForegroundColor Red; exit 1 }

foreach ($tool in @("ssh")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        Die "$tool not found. Install OpenSSH: Settings -> Apps -> Optional Features -> OpenSSH Client"
    }
}

$Target  = "${User}@${Router}"
$SshBase = @("-p", $Port, "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=15", $Target)


Log "Target  : $Target  (port $Port)"
Log "KeepConfig : $($KeepConfig.IsPresent)"
Log ""

# ── build the remote shell script ─────────────────────────────────────────────

$keepConfigLine = if ($KeepConfig) { "KEEP_CONFIG=1" } else { "KEEP_CONFIG=0" }

$remoteScript = (@'
#!/bin/sh
set -e
ts() { date '+%H:%M:%S'; }
log()  { echo "[$(ts)] $1"; }
ok()   { echo "[$(ts)] OK  $1"; }
warn() { echo "[$(ts)] WARN $1"; }

MARK_HEX="0x1a3"
ROUTE_TABLE=100
KEEP_CONFIG_PLACEHOLDER

log "=========================================="
log " ClashForge 完全卸载 / 环境还原"
log "=========================================="

# ── 1. 停止 clashforge init.d 服务 ────────────────────────────────────────────
log "[1/9] 停止 clashforge init.d 服务..."
if /etc/init.d/clashforge stop 2>/dev/null; then
    ok "  clashforge init.d 服务已停止"
else
    warn "  init.d stop 返回非零（可能已停止），继续"
fi
/etc/init.d/clashforge disable 2>/dev/null || true
sleep 1

# ── 2. 终止 clashforge 主进程 ────────────────────────────────────────────────
log "[2/9] 终止 clashforge / mihomo-clashforge 进程..."
for name in clashforge mihomo-clashforge; do
    PIDS=$(pgrep -f "/usr/bin/$name" 2>/dev/null || true)
    if [ -n "$PIDS" ]; then
        kill $PIDS 2>/dev/null || true
        sleep 1
        PIDS=$(pgrep -f "/usr/bin/$name" 2>/dev/null || true)
        if [ -n "$PIDS" ]; then
            kill -9 $PIDS 2>/dev/null || true
            warn "  $name PID $PIDS 已强制 SIGKILL"
        else
            ok "  $name 进程已正常退出"
        fi
    else
        ok "  $name 进程未运行，跳过"
    fi
done

# ── 3. 清理 nftables table inet metaclash ────────────────────────────────────
log "[3/9] 清理 nftables table inet metaclash..."
if nft list table inet metaclash >/dev/null 2>&1; then
    if nft delete table inet metaclash 2>/dev/null; then
        ok "  table inet metaclash 已删除"
    else
        warn "  整表删除失败，逐 chain 清理..."
        for CHAIN in dns_redirect tproxy_prerouting; do
            nft flush chain inet metaclash "$CHAIN" 2>/dev/null || true
            nft delete chain inet metaclash "$CHAIN" 2>/dev/null || true
        done
        nft delete table inet metaclash 2>/dev/null && ok "  table inet metaclash 删除成功（二次）" \
            || warn "  table inet metaclash 仍残留，请手动: nft delete table inet metaclash"
    fi
else
    ok "  table inet metaclash 不存在，跳过"
fi

# ── 4. 清理 nftables table inet dnsmasq (HIJACK 表) ──────────────────────────
# dnsmasq 在 replace 模式下重启时会自动注入这张表 (priority dstnat-5)，
# 若 port=0 时残留会导致 DNS 全挂（53 → redirect to :53 → 没人监听）
log "[4/9] 清理 nftables table inet dnsmasq (HIJACK)..."
if nft list table inet dnsmasq >/dev/null 2>&1; then
    if nft delete table inet dnsmasq 2>/dev/null; then
        ok "  table inet dnsmasq 已删除"
    else
        warn "  table inet dnsmasq 删除失败，将在 dnsmasq 重启后重新注入（正常）"
    fi
else
    ok "  table inet dnsmasq 不存在，跳过"
fi

# ── 5. 清理策略路由规则与路由表 ──────────────────────────────────────────────
log "[5/9] 清理策略路由 fwmark $MARK_HEX / table $ROUTE_TABLE..."

# IPv4 ip rule
REMOVED_RULES=0
while ip rule show 2>/dev/null | grep -q "fwmark $MARK_HEX"; do
    ip rule del fwmark "$MARK_HEX" lookup "$ROUTE_TABLE" 2>/dev/null || break
    REMOVED_RULES=$((REMOVED_RULES + 1))
done
if [ "$REMOVED_RULES" -gt 0 ]; then
    ok "  已删除 $REMOVED_RULES 条 IPv4 ip rule fwmark $MARK_HEX"
else
    ok "  IPv4 ip rule fwmark $MARK_HEX 不存在，跳过"
fi

# IPv4 route table
if ip route show table "$ROUTE_TABLE" 2>/dev/null | grep -q .; then
    ip route flush table "$ROUTE_TABLE" 2>/dev/null \
        && ok "  IPv4 路由表 $ROUTE_TABLE 已清空" \
        || warn "  IPv4 路由表 $ROUTE_TABLE 清空失败"
else
    ok "  IPv4 路由表 $ROUTE_TABLE 为空，跳过"
fi

# IPv6 ip rule
REMOVED_RULES6=0
while ip -6 rule show 2>/dev/null | grep -q "fwmark $MARK_HEX"; do
    ip -6 rule del fwmark "$MARK_HEX" lookup "$ROUTE_TABLE" 2>/dev/null || break
    REMOVED_RULES6=$((REMOVED_RULES6 + 1))
done
if [ "$REMOVED_RULES6" -gt 0 ]; then
    ok "  已删除 $REMOVED_RULES6 条 IPv6 ip rule fwmark $MARK_HEX"
else
    ok "  IPv6 ip rule fwmark $MARK_HEX 不存在，跳过"
fi

# IPv6 route table
if ip -6 route show table "$ROUTE_TABLE" 2>/dev/null | grep -q .; then
    ip -6 route flush table "$ROUTE_TABLE" 2>/dev/null \
        && ok "  IPv6 路由表 $ROUTE_TABLE 已清空" \
        || warn "  IPv6 路由表 $ROUTE_TABLE 清空失败"
else
    ok "  IPv6 路由表 $ROUTE_TABLE 为空，跳过"
fi

# ── 6. 恢复 dnsmasq DNS 配置 ─────────────────────────────────────────────────
log "[6/9] 恢复 dnsmasq 配置..."

# 6a. UCI 方式（OpenWrt 主路径）— 删除 clashforge 写入的三个覆盖项
if command -v uci >/dev/null 2>&1; then
    # replace 模式：删除 port=0 覆盖
    if uci get dhcp.@dnsmasq[0].port >/dev/null 2>&1; then
        uci delete dhcp.@dnsmasq[0].port 2>/dev/null \
            && ok "  UCI: dhcp.@dnsmasq[0].port 已删除（恢复默认 53）" \
            || warn "  UCI: 删除 port 失败"
    else
        ok "  UCI: dhcp.@dnsmasq[0].port 无覆盖，跳过"
    fi
    # upstream 模式：删除 server 和 noresolv 覆盖
    if uci get dhcp.@dnsmasq[0].server >/dev/null 2>&1; then
        uci delete dhcp.@dnsmasq[0].server 2>/dev/null \
            && ok "  UCI: dhcp.@dnsmasq[0].server 已删除" \
            || warn "  UCI: 删除 server 失败"
    fi
    if uci get dhcp.@dnsmasq[0].noresolv >/dev/null 2>&1; then
        uci delete dhcp.@dnsmasq[0].noresolv 2>/dev/null \
            && ok "  UCI: dhcp.@dnsmasq[0].noresolv 已删除" \
            || warn "  UCI: 删除 noresolv 失败"
    fi
    uci commit dhcp 2>/dev/null && ok "  UCI: dhcp commit 完成" || warn "  UCI: dhcp commit 失败"
fi

# 6b. conf-dir 方式（非 UCI 或兜底）— 删除 clashforge 写的 conf 文件
for CONF in /etc/dnsmasq.d/clashforge.conf /var/etc/dnsmasq.d/clashforge.conf; do
    if [ -f "$CONF" ]; then
        rm -f "$CONF" && ok "  已删除 $CONF" || warn "  删除 $CONF 失败"
    fi
done

# ── 7. 重启 dnsmasq ───────────────────────────────────────────────────────────
log "[7/9] 重启 dnsmasq 恢复正常 DNS 监听..."
if /etc/init.d/dnsmasq restart 2>/dev/null; then
    sleep 2
    # 验证 53 端口
    if netstat -lnup 2>/dev/null | grep -q ':53 '; then
        ok "  dnsmasq 已重启，端口 53 正在监听"
    else
        warn "  dnsmasq 已重启，但端口 53 暂未监听（可能仍在启动中）"
    fi
else
    warn "  dnsmasq restart 失败，请手动执行: /etc/init.d/dnsmasq restart"
fi

# ── 8. opkg 卸载 clashforge 包 ───────────────────────────────────────────────
log "[8/9] 卸载 clashforge opkg 包..."
if command -v opkg >/dev/null 2>&1 && opkg status clashforge 2>/dev/null | grep -q "^Package:"; then
    opkg remove clashforge 2>/dev/null \
        && ok "  opkg: clashforge 包已卸载" \
        || warn "  opkg remove 返回非零，手动确认: opkg status clashforge"
else
    ok "  opkg: clashforge 包未安装或已卸载，跳过"
fi

# ── 9. 清除所有数据文件 ───────────────────────────────────────────────────────
log "[9/9] 清除 clashforge 数据文件..."

# 运行时目录（PID、生成的 mihomo-config.yaml、cache.db、rule_provider 等）
if [ -d /var/run/metaclash ]; then
    rm -rf /var/run/metaclash && ok "  已删除 /var/run/metaclash" || warn "  删除 /var/run/metaclash 失败"
else
    ok "  /var/run/metaclash 不存在，跳过"
fi

# geodata 目录（Country.mmdb、geosite.dat）
if [ -d /usr/share/metaclash ]; then
    rm -rf /usr/share/metaclash && ok "  已删除 /usr/share/metaclash" || warn "  删除 /usr/share/metaclash 失败"
else
    ok "  /usr/share/metaclash 不存在，跳过"
fi

# 配置目录（订阅缓存、overrides.yaml、config.toml 等）
if [ "$KEEP_CONFIG" = "1" ]; then
    ok "  --KeepConfig: /etc/metaclash 保留"
else
    if [ -d /etc/metaclash ]; then
        rm -rf /etc/metaclash && ok "  已删除 /etc/metaclash" || warn "  删除 /etc/metaclash 失败"
    else
        ok "  /etc/metaclash 不存在，跳过"
    fi
fi

# 日志文件
if [ -f /var/log/clashforge.log ]; then
    rm -f /var/log/clashforge.log && ok "  已删除 /var/log/clashforge.log" || warn "  删除日志失败"
fi

# ── 最终验证 ──────────────────────────────────────────────────────────────────
echo ""
log "=========================================="
log " 验证结果"
log "=========================================="

# 进程
REMAINING=$(pgrep -f "clashforge\|mihomo-clashforge" 2>/dev/null || true)
if [ -n "$REMAINING" ]; then
    warn "仍有残留进程 PID: $REMAINING"
else
    ok "clashforge / mihomo-clashforge 进程：已全部清除"
fi

# nftables
if nft list table inet metaclash >/dev/null 2>&1; then
    warn "table inet metaclash 仍存在"
else
    ok "nftables table inet metaclash：已清除"
fi

# 策略路由
if ip rule show 2>/dev/null | grep -q "fwmark $MARK_HEX"; then
    warn "ip rule fwmark $MARK_HEX 仍存在"
else
    ok "策略路由规则 fwmark $MARK_HEX：已清除"
fi

# DNS
if netstat -lnup 2>/dev/null | grep -q ':53 '; then
    ok "DNS 端口 53：正在监听（dnsmasq 恢复正常）"
else
    warn "DNS 端口 53：暂未监听，dnsmasq 可能仍在启动中"
fi

echo ""
log "完成。ClashForge 已完全卸载，路由器已恢复至安装前状态。"
'@) -replace 'KEEP_CONFIG_PLACEHOLDER', $keepConfigLine

# ── execute on router ─────────────────────────────────────────────────────────

Log "连接路由器并执行卸载脚本..."
Log ""

$sshArgs = @(
    "-p", $Port,
    "-o", "StrictHostKeyChecking=no",
    "-o", "ConnectTimeout=15",
    $Target,
    "sh -s"
)

$remoteScript | & ssh @sshArgs
if ($LASTEXITCODE -ne 0) {
    Die "远程脚本执行失败 (exit $LASTEXITCODE)"
}
