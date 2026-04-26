#!/bin/sh
# tests/e2e/vm/snapshot.sh
# Step 2 — 基准状态快照
#
# 记录 clashforge 安装前的系统状态，供 stop-verify.sh 做还原对比。
# 同时获取当前直连出口 IP，作为三轮探测的 IP 基准。
#
# 输出：
#   /tmp/cf-snapshot/nft.before        nftables 规则
#   /tmp/cf-snapshot/ip-rule.before    ip rule 列表
#   /tmp/cf-snapshot/dnsmasq-d.before  /etc/dnsmasq.d/ 文件列表
#   /tmp/cf-snapshot/resolv.before     /etc/resolv.conf
#   /tmp/cf-snapshot/direct-ip         直连出口 IP
#
# 环境变量：
#   GITHUB_STEP_SUMMARY  GitHub Actions job summary 路径（CI 自动注入）

set -e

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

FAILED=0
RESULTS=""
TOTAL=0

record() {
    STATUS="$1"; TC="$2"; NAME="$3"; ACTUAL="$4"
    RESULTS="${RESULTS}${STATUS}|${TC}|${NAME}|${ACTUAL}\n"
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

SNAPSHOT_DIR="/tmp/cf-snapshot"
mkdir -p "$SNAPSHOT_DIR"

section "基准状态快照"

# SN-01 nftables 规则快照
nft list ruleset > "$SNAPSHOT_DIR/nft.before" 2>/dev/null || echo "" > "$SNAPSHOT_DIR/nft.before"
NFT_TABLES=$(nft list tables 2>/dev/null | tr '\n' ' ' || echo "(none)")
record PASS SN-01 "nftables 规则快照" "已记录: $NFT_TABLES"
info "当前 nft 表: $NFT_TABLES"

# SN-02 ip rule 快照
ip rule list > "$SNAPSHOT_DIR/ip-rule.before" 2>/dev/null || echo "" > "$SNAPSHOT_DIR/ip-rule.before"
IP_RULE_COUNT=$(wc -l < "$SNAPSHOT_DIR/ip-rule.before")
record PASS SN-02 "ip rule 策略路由快照" "已记录 $IP_RULE_COUNT 条规则"

# SN-03 dnsmasq.d 快照
ls /etc/dnsmasq.d/ > "$SNAPSHOT_DIR/dnsmasq-d.before" 2>/dev/null || echo "" > "$SNAPSHOT_DIR/dnsmasq-d.before"
DNSMASQ_FILES=$(cat "$SNAPSHOT_DIR/dnsmasq-d.before" | tr '\n' ' ')
record PASS SN-03 "dnsmasq.d 配置快照" "已记录: ${DNSMASQ_FILES:-(空)}"

# SN-04 resolv.conf 快照
cp /etc/resolv.conf "$SNAPSHOT_DIR/resolv.before" 2>/dev/null || echo "" > "$SNAPSHOT_DIR/resolv.before"
DNS_SERVERS=$(grep nameserver /etc/resolv.conf 2>/dev/null | tr '\n' ' ')
record PASS SN-04 "resolv.conf 快照" "当前 DNS: ${DNS_SERVERS:-(未配置)}"

# SN-05 直连出口 IP
DIRECT_IP=$(curl -sf --max-time 10 https://api.ipify.org 2>/dev/null || echo "FAILED")
if [ "$DIRECT_IP" != "FAILED" ] && [ -n "$DIRECT_IP" ]; then
    echo "$DIRECT_IP" > "$SNAPSHOT_DIR/direct-ip"
    record PASS SN-05 "直连出口 IP 基准" "直连 IP: $(mask_ip "$DIRECT_IP")"
else
    record FAIL SN-05 "直连出口 IP 基准" "无法获取直连 IP，网络异常"
fi

# ── Summary ────────────────────────────────────────────────────────────────────
PASS_COUNT=$(printf "%b" "$RESULTS" | grep -c "^PASS" || echo 0)
FAIL_COUNT=$(printf "%b" "$RESULTS" | grep -c "^FAIL" || echo 0)

summary "## 📸 Step 2 — 基准状态快照"
summary ""
summary "| 项目 | 值 |"
summary "|------|----|"
summary "| **直连 IP** | $(mask_ip "${DIRECT_IP:-未知}") |"
summary "| **nft 表** | $NFT_TABLES |"
summary "| **DNS** | ${DNS_SERVERS:-(未配置)} |"
summary ""
summary "| 编号 | 用例 | 实际结果 | 状态 |"
summary "|------|------|----------|------|"
printf "%b" "$RESULTS" | while IFS="|" read -r STATUS TC NAME ACTUAL; do
    [ -z "$TC" ] && continue
    case "$STATUS" in PASS) ICON="✅" ;; FAIL) ICON="❌" ;; WARN) ICON="⚠️" ;; *) continue ;; esac
    summary "| $TC | $NAME | $ACTUAL | $ICON |"
done

if [ "$FAILED" -gt 0 ]; then
    printf "${RED}${BOLD}快照失败 $FAILED 项 ❌${RESET}\n"
    exit 1
else
    printf "${GREEN}${BOLD}基准快照完成 ✅${RESET}\n"
fi
