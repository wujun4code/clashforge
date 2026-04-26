#!/bin/bash
# tests/e2e/node/test-node-deploy.sh
# Node Server E2E — Deploy GOST + HTTPS Certificate
#
# 测试节点服务器的完整部署流程：
#   ND-01  创建节点（POST /api/v1/nodes）
#   ND-02  SSH 连通性检测（POST /api/v1/nodes/{id}/test）
#   ND-03  部署 GOST + 证书（POST /api/v1/nodes/{id}/deploy，SSE 流）
#   ND-04  验证部署状态 — API 查询节点 status === deployed
#   ND-05  验证 GOST 服务 — 远程 SSH 检查 systemctl is-active gost
#   ND-06  验证证书文件 — 远程 SSH 检查 /etc/gost/certs/{domain}/
#   ND-07  验证端口监听 — 远程 SSH 检查 ss -tlnp :443
#   ND-08  导出代理配置 — GET /api/v1/nodes/{id}/proxy-config 返回有效 YAML
#
# 环境变量（由 workflow 注入）：
#   NODE_TEST_HOST        测试服务器 IP/域名
#   NODE_TEST_PORT        SSH 端口（默认 22）
#   NODE_TEST_USER        SSH 用户名
#   NODE_TEST_PASSWORD    SSH 密码
#   NODE_TEST_DOMAIN      TLS 证书域名
#   NODE_TEST_EMAIL       acme.sh 注册邮箱
#   NODE_TEST_CF_TOKEN    Cloudflare API Token
#   NODE_TEST_CF_ACCOUNT_ID  Cloudflare Account ID
#   NODE_TEST_CF_ZONE_ID  Cloudflare Zone ID
#   GITHUB_STEP_SUMMARY

set -e

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

FAILED=0
RESULTS=""
TOTAL=0

CF_BINARY="/tmp/e2e-node/clashforge-linux-amd64"
CF_CONFIG="/tmp/e2e-node/config.toml"
CF_RUNTIME="/tmp/e2e-node/runtime"
CF_DATA="/tmp/e2e-node/data"
CF_PORT="18777"   # use non-standard port to avoid conflicts
CF_API="http://127.0.0.1:${CF_PORT}/api/v1"

# ── helpers ──────────────────────────────────────────────────────────────────
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

mask_ip()     { echo "$1" | sed 's/\([0-9]*\)\.[0-9]*\.[0-9]*\.\([0-9]*\)/\1.*.*.\2/'; }
mask_domain() { echo "$1" | awk -F. '{ if (NF<=2) {print $0} else {print $1".***."$NF} }'; }

# ── remote SSH helper (into the test target server) ──────────────────────────
remote_ssh() {
    PORT="${NODE_TEST_PORT:-22}"
    ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 \
        -p "$PORT" "${NODE_TEST_USER}@${NODE_TEST_HOST}" "$@" 2>&1
}

# ── validate required env vars ──────────────────────────────────────────────
validate_env() {
    local missing=""
    for var in NODE_TEST_HOST NODE_TEST_USER NODE_TEST_PASSWORD \
               NODE_TEST_DOMAIN NODE_TEST_EMAIL NODE_TEST_CF_TOKEN \
               NODE_TEST_CF_ACCOUNT_ID NODE_TEST_CF_ZONE_ID; do
        if [ -z "${!var:-}" ]; then
            missing="$missing $var"
        fi
    done

    if [ -n "$missing" ]; then
        echo "❌ ERROR: Missing required environment variables:$missing"
        echo "   Please configure them in GitHub Secrets."
        exit 1
    fi
    info "所有环境变量已就绪"
}

# ── cleanup on exit ─────────────────────────────────────────────────────────
cleanup() {
    info "清理中..."
    if [ -f /tmp/e2e-node/cf.pid ]; then
        PID=$(cat /tmp/e2e-node/cf.pid)
        kill "$PID" 2>/dev/null || true
        wait "$PID" 2>/dev/null || true
    fi
    pkill -f "clashforge-linux-amd64" 2>/dev/null || true
    rm -rf "$CF_RUNTIME" "$CF_DATA" /tmp/e2e-node/cf.pid 2>/dev/null || true
    info "清理完成"
}
trap cleanup EXIT

# ── main ────────────────────────────────────────────────────────────────────
section "Node Server E2E — Deploy GOST + HTTPS Certificate"

validate_env

# ────────────────────────────────────────────────────────────────────────────
# Step 2: Start clashforge
# ────────────────────────────────────────────────────────────────────────────
section "Step 2 — 启动 ClashForge 服务"

if [ ! -x "$CF_BINARY" ]; then
    echo "❌ Binary not found or not executable: $CF_BINARY"
    exit 1
fi

# Create minimal TOML config
mkdir -p "$CF_RUNTIME" "$CF_DATA"
cat > "$CF_CONFIG" << 'TOML'
[core]
binary = "/bin/echo"
runtime_dir = "/tmp/e2e-node/runtime"
data_dir = "/tmp/e2e-node/data"
auto_start_core = false

[ports]
http = 27890
socks = 27891
mixed = 27893
redir = 27892
tproxy = 27895
dns = 27874
mihomo_api = 29090
ui = 18777

[network]
mode = "none"

[dns]
enable = false

[security]
api_secret = ""

[log]
level = "debug"
TOML

info "配置文件已写入: $CF_CONFIG"
info "启动 clashforge (port $CF_PORT)..."

"$CF_BINARY" -config "$CF_CONFIG" > /tmp/e2e-node/clashforge.log 2>&1 &
CF_PID=$!
echo "$CF_PID" > /tmp/e2e-node/cf.pid
info "ClashForge PID: $CF_PID"

# Wait for API ready (up to 15s)
info "等待 API 就绪..."
for i in $(seq 1 30); do
    if curl -sf --max-time 2 -o /dev/null "$CF_API/status" 2>/dev/null; then
        info "API 就绪 (${i}s)"
        break
    fi
    sleep 1
done

# Final check
if ! curl -sf --max-time 3 -o /dev/null "$CF_API/status" 2>/dev/null; then
    echo "❌ ClashForge API 未能就绪"
    echo "--- clashforge log ---"
    cat /tmp/e2e-node/clashforge.log
    exit 1
fi

HEALTH=$(curl -sf --max-time 3 "$CF_API/status" 2>/dev/null || echo '{}')
info "ClashForge 运行中: $(echo "$HEALTH" | grep -o '"status":"[^"]*"' | head -1 || echo 'ok')"

# ────────────────────────────────────────────────────────────────────────────
# Step 3: Create node + SSH test
# ────────────────────────────────────────────────────────────────────────────
section "Step 3 — 创建节点 + SSH 连通性检测"

# ND-01: Create node
info "创建节点..."
CREATE_RESP=$(curl -sf --max-time 10 \
    -H "Content-Type: application/json" \
    -d "{
        \"name\": \"e2e-test-node\",
        \"host\": \"${NODE_TEST_HOST}\",
        \"port\": ${NODE_TEST_PORT:-22},
        \"username\": \"${NODE_TEST_USER}\",
        \"password\": \"${NODE_TEST_PASSWORD}\",
        \"domain\": \"${NODE_TEST_DOMAIN}\",
        \"email\": \"${NODE_TEST_EMAIL}\",
        \"cf_token\": \"${NODE_TEST_CF_TOKEN}\",
        \"cf_account_id\": \"${NODE_TEST_CF_ACCOUNT_ID}\",
        \"cf_zone_id\": \"${NODE_TEST_CF_ZONE_ID}\"
    }" \
    "$CF_API/nodes" 2>/dev/null || echo '{"error":"CREATE_FAILED"}')

NODE_ID=$(echo "$CREATE_RESP" | grep -o '"id":"[^"]*"' | head -1 | sed 's/"id":"//;s/"//')
if [ -n "$NODE_ID" ]; then
    info "节点已创建: $NODE_ID"
    record PASS ND-01 "创建节点" "POST /nodes → 返回节点 ID" "ID: $NODE_ID"
else
    CREATE_ERR=$(echo "$CREATE_RESP" | grep -o '"error":"[^"]*"' | head -1 || echo "$CREATE_RESP")
    record FAIL ND-01 "创建节点" "POST /nodes → 返回节点 ID" "失败: $CREATE_ERR"
    exit 1
fi

# ND-02: Test SSH connectivity
info "测试 SSH 连通性..."
TEST_RESP=$(curl -sf --max-time 30 \
    -X POST \
    "$CF_API/nodes/$NODE_ID/test" 2>/dev/null || echo '{"ok":false,"message":"REQUEST_FAILED"}')

if echo "$TEST_RESP" | grep -q '"ok":true'; then
    MSG=$(echo "$TEST_RESP" | grep -o '"message":"[^"]*"' | head -1 | sed 's/"message":"//;s/"//')
    record PASS ND-02 "SSH 连通性检测" "POST /nodes/{id}/test → ok:true" "连接成功: ${MSG:-ok}"
else
    TEST_ERR=$(echo "$TEST_RESP" | grep -o '"message":"[^"]*"' | head -1 || echo "$TEST_RESP")
    record FAIL ND-02 "SSH 连通性检测" "POST /nodes/{id}/test → ok:true" "失败: $TEST_ERR"
    exit 1
fi

# ────────────────────────────────────────────────────────────────────────────
# Step 4: Deploy GOST + HTTPS Certificate (SSE streaming)
# ────────────────────────────────────────────────────────────────────────────
section "Step 4 — 部署 GOST 服务 + HTTPS 证书"

info "触发部署（SSE 流模式），超时 300s..."
DEPLOY_TMP="/tmp/e2e-node/deploy-output.txt"
DEPLOY_START=$(date +%s)

# Capture SSE stream; parse done event for success/failure
curl -sfN --max-time 300 \
    -X POST \
    "$CF_API/nodes/$NODE_ID/deploy" > "$DEPLOY_TMP" 2>/tmp/e2e-node/deploy-err.txt &
DEPLOY_PID=$!

# Stream progress to stdout while waiting
LAST_LINE=""
while kill -0 "$DEPLOY_PID" 2>/dev/null; do
    if [ -f "$DEPLOY_TMP" ]; then
        NEW_LINE=$(tail -1 "$DEPLOY_TMP" 2>/dev/null || echo "")
        if [ "$NEW_LINE" != "$LAST_LINE" ] && [ -n "$NEW_LINE" ]; then
            # Extract and display step info
            STEP_INFO=$(echo "$NEW_LINE" | grep -o '"step":"[^"]*"' | head -1 | sed 's/"step":"//;s/"//')
            STATUS_INFO=$(echo "$NEW_LINE" | grep -o '"status":"[^"]*"' | head -1 | sed 's/"status":"//;s/"//')
            MSG_INFO=$(echo "$NEW_LINE" | grep -o '"message":"[^"]*"' | head -1 | sed 's/"message":"//;s/"//')
            if [ -n "$STEP_INFO" ]; then
                case "$STATUS_INFO" in
                    ok)      printf "  ${GREEN}✓${RESET} %s: %s\n" "$STEP_INFO" "${MSG_INFO:-ok}" ;;
                    error)   printf "  ${RED}✗${RESET} %s: %s\n" "$STEP_INFO" "${MSG_INFO:-error}" ;;
                    running) printf "  ${CYAN}⟳${RESET} %s: %s\n" "$STEP_INFO" "${MSG_INFO:-running}" ;;
                    *)       printf "  %s: %s\n" "$STEP_INFO" "${MSG_INFO:-}" ;;
                esac
            fi
            LAST_LINE="$NEW_LINE"
        fi
    fi
    sleep 0.5
done

wait "$DEPLOY_PID" || true
DEPLOY_ELAPSED=$(($(date +%s) - DEPLOY_START))
info "部署完成（耗时 ${DEPLOY_ELAPSED}s）"

# Parse the final done event
DEPLOY_OUTPUT=$(cat "$DEPLOY_TMP" 2>/dev/null || echo "")
DEPLOY_DONE=$(echo "$DEPLOY_OUTPUT" | grep '"type":"done"' | tail -1)

if [ -z "$DEPLOY_DONE" ]; then
    record FAIL ND-03 "部署 GOST + 证书" "SSE done event → success:true" "未收到 done 事件"
    info "=== 部署原始输出 ==="
    cat "$DEPLOY_TMP" 2>/dev/null
    exit 1
fi

if echo "$DEPLOY_DONE" | grep -q '"success":true'; then
    DEPLOY_ELAPSED=$(($(date +%s) - DEPLOY_START))
    record PASS ND-03 "部署 GOST + 证书" "SSE done event → success:true" "部署成功（${DEPLOY_ELAPSED}s）"
else
    DEPLOY_ERR=$(echo "$DEPLOY_DONE" | grep -o '"error":"[^"]*"' | head -1 | sed 's/"error":"//;s/"//')
    record FAIL ND-03 "部署 GOST + 证书" "SSE done event → success:true" "部署失败: ${DEPLOY_ERR:-unknown}"
    info "=== 部署错误日志 ==="
    cat "$DEPLOY_TMP" 2>/dev/null
    exit 1
fi

# ────────────────────────────────────────────────────────────────────────────
# Step 5: Verify deployment
# ────────────────────────────────────────────────────────────────────────────
section "Step 5 — 验证部署结果"

# ND-04: API 查询节点状态
info "通过 API 查询节点状态..."
NODE_RESP=$(curl -sf --max-time 10 "$CF_API/nodes/$NODE_ID" 2>/dev/null || echo '{}')
NODE_STATUS=$(echo "$NODE_RESP" | grep -o '"status":"[^"]*"' | head -1 | sed 's/"status":"//;s/"//')

if [ "$NODE_STATUS" = "deployed" ]; then
    record PASS ND-04 "验证部署状态" "GET /nodes/{id} → status=deployed" "status: deployed ✓"
else
    ACTUAL_STATUS="${NODE_STATUS:-unknown}"
    record FAIL ND-04 "验证部署状态" "GET /nodes/{id} → status=deployed" "status: $ACTUAL_STATUS"
fi

# ND-05: 远程 SSH 检查 GOST 服务
info "远程检查 GOST 服务状态..."
GOST_STATUS=$(remote_ssh "systemctl is-active gost 2>&1" || echo "inactive")
GOST_STATUS=$(echo "$GOST_STATUS" | tr -d '\n\r' | xargs)

if [ "$GOST_STATUS" = "active" ]; then
    record PASS ND-05 "验证 GOST 服务" "remote: systemctl is-active gost → active" "GOST 服务运行中 ✓"
else
    record FAIL ND-05 "验证 GOST 服务" "remote: systemctl is-active gost → active" "GOST 状态: $GOST_STATUS"
fi

# ND-06: 验证证书文件
info "远程检查证书文件..."
CERT_CHECK=$(remote_ssh "ls -la /etc/gost/certs/${NODE_TEST_DOMAIN}/ 2>&1" || echo "MISSING")

if echo "$CERT_CHECK" | grep -qE "cert\.pem|key\.pem"; then
    CERT_FILES=$(echo "$CERT_CHECK" | grep -E "cert\.pem|key\.pem" | awk '{print $NF, $5}' | tr '\n' ' ')
    record PASS ND-06 "验证证书文件" "remote: /etc/gost/certs/{domain}/ 包含 cert.pem & key.pem" "文件: $CERT_FILES"
else
    record FAIL ND-06 "验证证书文件" "remote: /etc/gost/certs/{domain}/ 包含 cert.pem & key.pem" "未找到证书文件: $(echo "$CERT_CHECK" | head -3)"
fi

# ND-07: 验证端口监听
info "远程检查 GOST 端口监听..."
PORT_CHECK=$(remote_ssh "ss -tlnp 2>/dev/null | grep ':443 ' || netstat -tlnp 2>/dev/null | grep ':443 ' || echo 'NOT_LISTENING'")

if echo "$PORT_CHECK" | grep -qE ":443\b"; then
    PORT_INFO=$(echo "$PORT_CHECK" | awk '{print $1, $4, $NF}' | head -1)
    record PASS ND-07 "验证端口监听" "remote: ss -tlnp 显示 :443 监听" "监听: $PORT_INFO"
else
    record FAIL ND-07 "验证端口监听" "remote: ss -tlnp 显示 :443 监听" "端口 443 未监听: $PORT_CHECK"
fi

# ND-08: 导出代理配置
info "导出代理配置 YAML..."
PROXY_YAML=$(curl -sf --max-time 10 "$CF_API/nodes/$NODE_ID/proxy-config" 2>/dev/null || echo "")

if echo "$PROXY_YAML" | grep -qE "^proxies:"; then
    # Verify it's valid YAML with expected fields
    YAML_CHECKS=""
    echo "$PROXY_YAML" | grep -q "name:" && YAML_CHECKS="$YAML_CHECKS name"
    echo "$PROXY_YAML" | grep -q "type:" && YAML_CHECKS="$YAML_CHECKS type"
    echo "$PROXY_YAML" | grep -q "server:" && YAML_CHECKS="$YAML_CHECKS server"
    echo "$PROXY_YAML" | grep -q "port:" && YAML_CHECKS="$YAML_CHECKS port"
    echo "$PROXY_YAML" | grep -q "tls:" && YAML_CHECKS="$YAML_CHECKS tls"
    echo "$PROXY_YAML" | grep -q "username:" && YAML_CHECKS="$YAML_CHECKS username"
    echo "$PROXY_YAML" | grep -q "password:" && YAML_CHECKS="$YAML_CHECKS password"
    record PASS ND-08 "导出代理配置" "GET /nodes/{id}/proxy-config → 有效 Clash proxy YAML" "字段:$YAML_CHECKS ✓"
    info "代理配置预览:"
    echo "$PROXY_YAML" | head -10 | sed 's/^/  /'
else
    record FAIL ND-08 "导出代理配置" "GET /nodes/{id}/proxy-config → 有效 Clash proxy YAML" "无效 YAML: $(echo "$PROXY_YAML" | head -3)"
fi

# ────────────────────────────────────────────────────────────────────────────
# Step 7: Cleanup — destroy node
# ────────────────────────────────────────────────────────────────────────────
section "Step 7 — 清理：销毁节点部署"

info "触发销毁（SSE 流模式）..."
DESTROY_TMP="/tmp/e2e-node/destroy-output.txt"

curl -sfN --max-time 120 \
    -X POST \
    "$CF_API/nodes/$NODE_ID/destroy" > "$DESTROY_TMP" 2>/dev/null || true

DESTROY_DONE=$(cat "$DESTROY_TMP" 2>/dev/null | grep '"type":"done"' | tail -1 || echo "")

if echo "$DESTROY_DONE" | grep -q '"success":true'; then
    info "节点销毁成功"
else
    DESTROY_ERR=$(echo "$DESTROY_DONE" | grep -o '"error":"[^"]*"' | head -1 || echo "unknown")
    info "节点销毁结果: ${DESTROY_ERR:-部分成功}（继续）"
fi

# Verify remote cleanup
info "远程验证 GOST 已停止..."
GOST_AFTER=$(remote_ssh "systemctl is-active gost 2>&1 || echo 'inactive'" | tr -d '\n\r' | xargs)
if [ "$GOST_AFTER" != "active" ]; then
    info "GOST 服务已停止: $GOST_AFTER"
else
    info "警告: GOST 可能仍在运行 (${GOST_AFTER})"
fi

# ────────────────────────────────────────────────────────────────────────────
# Summary
# ────────────────────────────────────────────────────────────────────────────
PASS_COUNT=$(printf "%b" "$RESULTS" | grep -c "^PASS" || echo 0)
FAIL_COUNT=$(printf "%b" "$RESULTS" | grep -c "^FAIL" || echo 0)
WARN_COUNT=$(printf "%b" "$RESULTS" | grep -c "^WARN" || echo 0)

section "测试结果汇总"

summary "## 🖥️ Node Server E2E — Deploy GOST + Certificate"
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
    printf "${RED}${BOLD}Node Server E2E 测试失败 ❌${RESET}\n"
    exit 1
else
    printf "${GREEN}${BOLD}Node Server E2E 测试通过 ✅${RESET}\n"
fi
