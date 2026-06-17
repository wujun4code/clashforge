#!/bin/sh
# tests/e2e/vm/install-verify.sh
# Step 4 — 安装完备性检测
#
# 断言 clashforge 安装后的初始状态（还未执行任何 setup 操作）：
#   IV-01  clashforge 二进制可用，版本可读
#   IV-02  mihomo-clashforge 二进制可用
#   IV-03  init.d 服务文件存在
#   IV-04  Web UI 可访问（HTTP 200）
#   IV-05  mihomo 进程 此刻不应在运行（未做任何启动）
#   IV-06  nftables 中不应存在 metaclash 表（未接管）
#   IV-07  dnsmasq.d 中不应有 clashforge 相关配置文件（未接管）
#   IV-08  API /status 可响应
#
# 环境变量：
#   CLASHFORGE_VERSION   期望版本（可选，仅用于日志对比）
#   GITHUB_STEP_SUMMARY

set -e

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
CF_UI="http://127.0.0.1:7777"

section "Step 4 — 安装完备性检测"

# IV-01 clashforge 二进制
if command -v clashforge > /dev/null 2>&1; then
    CF_VER=$(clashforge --version 2>/dev/null | head -1 || echo "unknown")
    record PASS IV-01 "clashforge 二进制可用" "clashforge 在 PATH 中" "$CF_VER"
else
    record FAIL IV-01 "clashforge 二进制可用" "clashforge 在 PATH 中" "未找到 clashforge 命令"
fi

# IV-02 mihomo 二进制
if [ -x /usr/bin/mihomo-clashforge ]; then
    MIHOMO_VER=$(/usr/bin/mihomo-clashforge -v 2>/dev/null | head -1 || echo "unknown")
    record PASS IV-02 "mihomo-clashforge 二进制可用" "/usr/bin/mihomo-clashforge 存在且可执行" "$MIHOMO_VER"
else
    record FAIL IV-02 "mihomo-clashforge 二进制可用" "/usr/bin/mihomo-clashforge 存在且可执行" "文件不存在或不可执行"
fi

# IV-03 init.d 服务文件
if [ -x /etc/init.d/clashforge ]; then
    record PASS IV-03 "init.d 服务文件存在" "/etc/init.d/clashforge 存在且可执行" "存在"
else
    record FAIL IV-03 "init.d 服务文件存在" "/etc/init.d/clashforge 存在且可执行" "不存在"
fi

# IV-04 Web UI 可访问（clashforge 服务应已启动）
UI_CODE=$(curl -sf -o /dev/null -w "%{http_code}" --max-time 10 "$CF_UI/" 2>/dev/null || echo "0")
if [ "$UI_CODE" = "200" ]; then
    record PASS IV-04 "Web UI 可访问" "GET / → HTTP 200" "HTTP $UI_CODE"
else
    record FAIL IV-04 "Web UI 可访问" "GET / → HTTP 200" "HTTP $UI_CODE（服务未启动或端口未开）"
fi

# IV-05 mihomo 进程状态与 config.toml 中的 auto_start_core 一致
# 若 --keep-config 保留了 auto_start_core=true，clashforge 在 postinst 启动后
# 会自动拉起 mihomo，这是预期行为；若为 false/未设置则不应启动。
MIHOMO_PID=$(pgrep -f "mihomo-clashforge" 2>/dev/null || echo "")
_CORE_SECTION=$(awk '/^\[core\]/{f=1;next} /^\[/{f=0} f{print}' \
    /etc/metaclash/config.toml 2>/dev/null)
AUTO_START_CORE=$(echo "$_CORE_SECTION" | grep -c 'auto_start_core.*true' || true)
if [ "$AUTO_START_CORE" -gt 0 ]; then
    # auto_start_core=true: mihomo 应已自动启动
    if [ -n "$MIHOMO_PID" ]; then
        record PASS IV-05 "mihomo 自动启动（auto_start_core=true）" \
            "config.toml 中 auto_start_core=true → mihomo 应自动启动" \
            "PID=$MIHOMO_PID ✓"
    else
        record FAIL IV-05 "mihomo 自动启动（auto_start_core=true）" \
            "config.toml 中 auto_start_core=true → mihomo 应自动启动" \
            "mihomo 未运行（意外）"
    fi
else
    # auto_start_core=false/未设置: 未执行 setup，mihomo 不应自动启动
    if [ -z "$MIHOMO_PID" ]; then
        record PASS IV-05 "mihomo 进程未运行（auto_start_core=false）" \
            "pgrep mihomo-clashforge → 无进程" "mihomo 未运行 ✓"
    else
        record FAIL IV-05 "mihomo 进程未运行（auto_start_core=false）" \
            "auto_start_core=false → mihomo 不应自动启动" \
            "mihomo 正在运行 PID=$MIHOMO_PID（意外）"
    fi
fi

# IV-06 nftables metaclash 表存在状态与 config 一致
# auto_start_core=true 且 network.apply_on_start=true 且 mode != "none" 时，
# postinst 启动后会应用 nftables 规则，metaclash 表出现是预期行为。
# 使用 awk 提取 [network] 段，避免 [dns] 段同名字段干扰。
NFT_TABLES=$(nft list tables 2>/dev/null | tr '\n' ' ')
_NET_SECTION=$(awk '/^\[network\]/{f=1;next} /^\[/{f=0} f{print}' \
    /etc/metaclash/config.toml 2>/dev/null)
NET_APPLY=$(echo "$_NET_SECTION" | grep -c 'apply_on_start.*true' || true)
NET_MODE=$(echo "$_NET_SECTION" | awk -F'"' '/^mode =/{print $2; exit}')
echo "[IV-06 debug] AUTO_START_CORE=$AUTO_START_CORE NET_APPLY=$NET_APPLY NET_MODE=$NET_MODE NFT_TABLES=$NFT_TABLES"
NFT_EXPECTED=0
[ "$AUTO_START_CORE" -gt 0 ] && [ "$NET_APPLY" -gt 0 ] \
    && [ "$NET_MODE" != "none" ] && [ "$NET_MODE" != "" ] && NFT_EXPECTED=1
if echo "$NFT_TABLES" | grep -qE "metaclash|clashforge"; then
    if [ "$NFT_EXPECTED" -gt 0 ]; then
        record PASS IV-06 "nftables metaclash 表（apply_on_start=true）" \
            "config 要求自动接管 → metaclash 表应存在" \
            "metaclash 表已创建 ✓"
    else
        record FAIL IV-06 "nftables 无接管规则" \
            "auto_start_core/apply_on_start 未启用 → 不应有 metaclash 表" \
            "发现意外 nft 表: $NFT_TABLES"
    fi
else
    if [ "$NFT_EXPECTED" -gt 0 ]; then
        record FAIL IV-06 "nftables metaclash 表（apply_on_start=true）" \
            "config 要求自动接管 → metaclash 表应存在" \
            "metaclash 表未创建（意外）"
    else
        record PASS IV-06 "nftables 无接管规则（auto_start_core=false）" \
            "nft list tables 中无 metaclash/clashforge 表" \
            "无接管表 ✓（当前: ${NFT_TABLES:-(空)}）"
    fi
fi

# IV-07 dnsmasq.d 中不应有 clashforge 相关配置
CF_DNSMASQ=$(ls /etc/dnsmasq.d/ 2>/dev/null | grep -iE "clash|metaclash|mihomo" || echo "")
if [ -z "$CF_DNSMASQ" ]; then
    record PASS IV-07 "dnsmasq.d 无接管配置" "安装后 /etc/dnsmasq.d/ 中无 clashforge 相关配置文件" "无接管配置 ✓"
else
    record FAIL IV-07 "dnsmasq.d 无接管配置" "安装后 /etc/dnsmasq.d/ 中无 clashforge 相关配置文件" "发现意外配置: $CF_DNSMASQ"
fi

# IV-08 API /status 可响应
STATUS_RESP=$(curl -sf --max-time 10 "$CF_API/status" 2>/dev/null || echo "FAILED")
if echo "$STATUS_RESP" | grep -qE "\"ok\"|\"status\"|\"version\""; then
    record PASS IV-08 "API /status 可响应" "GET /api/v1/status → 含 ok/status/version 字段" "响应正常"
else
    record FAIL IV-08 "API /status 可响应" "GET /api/v1/status → 含 ok/status/version 字段" "无响应或格式异常: $STATUS_RESP"
fi

# ── Summary ────────────────────────────────────────────────────────────────────
PASS_COUNT=$(printf "%b" "$RESULTS" | grep -c "^PASS" || echo 0)
FAIL_COUNT=$(printf "%b" "$RESULTS" | grep -c "^FAIL" || echo 0)
WARN_COUNT=$(printf "%b" "$RESULTS" | grep -c "^WARN" || echo 0)

summary "## 📦 Step 4 — 安装完备性检测"
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
    printf "${RED}${BOLD}安装验证失败 ❌${RESET}\n"
    exit 1
else
    printf "${GREEN}${BOLD}安装验证通过 ✅${RESET}\n"
fi
