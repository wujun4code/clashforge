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

# IV-05 mihomo 进程不应在运行
MIHOMO_PID=$(pgrep -f "mihomo-clashforge" 2>/dev/null || echo "")
if [ -z "$MIHOMO_PID" ]; then
    record PASS IV-05 "mihomo 进程未运行" "pgrep mihomo-clashforge → 无进程（未做任何启动）" "mihomo 未运行 ✓"
else
    record FAIL IV-05 "mihomo 进程未运行" "安装后未执行 setup，mihomo 不应自动启动" "mihomo 正在运行 PID=$MIHOMO_PID（意外）"
fi

# IV-06 nftables 中不应存在 metaclash 表
NFT_TABLES=$(nft list tables 2>/dev/null | tr '\n' ' ')
if echo "$NFT_TABLES" | grep -qE "metaclash|clashforge"; then
    record FAIL IV-06 "nftables 无接管规则" "安装后未执行 setup，不应有 metaclash 表" "发现意外 nft 表: $NFT_TABLES"
else
    record PASS IV-06 "nftables 无接管规则" "nft list tables 中无 metaclash/clashforge 表" "无接管表 ✓（当前: ${NFT_TABLES:-(空)}）"
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
