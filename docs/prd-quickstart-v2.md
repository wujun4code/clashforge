# ClashForge 快速启动 v2 — PRD

> 状态：草稿 | 作者：Wu Jun | 日期：2026-05-28  
> 关联：[DNS 分流策略 PRD](prd-dns-strategy.md)

---

## 一、背景与动机

### 当前流程（v1）的痛点

用户从零开始使用 ClashForge，需要完成以下 **10+ 个手动步骤**，跨越多个工具和页面：

```
① 准备代理服务端（VPS 安装 xray 或 CF Worker 部署）
→ ② 手动配置协议 + 证书 → ③ 手动写 Clash YAML → ④ 生成订阅
→ ⑤ 打开 ClashForge → ⑥ Setup 向导：导入配置
→ ⑦ DNS 设置 → ⑧ 网络设置 → ⑨ 启动服务 → ⑩ 连通性测试
```

每个步骤都需要用户具备 Linux 运维、代理协议、Cloudflare 等专业知识。
**高学习曲线是最大的用户流失点。**

### v2 的目标

用户选择部署类型后只需填写最少凭据，点击"开始部署"，**全程自动化**，直到 ClashForge 在路由器上运行并通过所有连通性测试。

| 路径 | 用户需提供 | 优势 |
|------|-----------|------|
| **☁️ Cloudflare Workers**（推荐新用户）| CF API Token + Account ID | 无需 VPS，免费额度够用，5 分钟完成 |
| **🖥️ VPS + Cloudflare** | VPS SSH 凭据 + CF API 凭据 | 独立服务器，性能可控，支持多端口 |

---

## 二、目标用户

| 用户画像 | 描述 | 推荐路径 |
|---|---|---|
| **技术小白** | 有 CF 账号，没有或不会操作 VPS | CF Workers |
| **轻度用户** | 流量不大，不想额外付钱买 VPS | CF Workers |
| **有 VPS 的进阶用户** | 会 SSH，但不了解代理协议配置细节 | VPS + CF |
| **运维熟手** | 一切都会，只想要一键脚手架 | VPS + CF |
| **多路由部署用户** | 需要在多台路由器重复上述流程 | 任意（部署结果复用）|

---

## 三、产品原则

1. **用户只做决策，不做操作** — 填写凭据 + 点击确认 = 全部
2. **透明但不干扰** — 进度日志实时可见，但用户无需理解每行
3. **失败可恢复** — 任何步骤失败后提供明确原因 + 一键重试
4. **不破坏 v1** — 现有向导完整保留，v2 是独立入口
5. **可复用** — v2 产生的节点、订阅、配置与 v1 完全兼容

---

## 四、用户旅程（完整视角）

### 4.1 入口

用户打开 ClashForge UI，主页显示两个入口：

```
┌─────────────────────────────────────────────────────────┐
│  🚀  快速启动（推荐新用户）                               │
│      选择部署类型，10 分钟完成全部配置                    │
│                              [ 开始快速启动 → ]          │
├─────────────────────────────────────────────────────────┤
│  ⚙️  手动设置（v1，高级用户）                            │
│      自定义每个步骤，完整控制                             │
│                              [ 进入向导 → ]              │
└─────────────────────────────────────────────────────────┘
```

### 4.2 Step 0：选择部署类型（两条路径的分叉点）

进入快速启动后，第一步让用户选择部署类型：

```
┌─────────────────────────────────────────────────────────────────┐
│  选择您的部署方式                                                │
│                                                                  │
│  ┌────────────────────────────────────┐                         │
│  │  🖥️  VPS + Cloudflare    ★ 推荐   │                         │
│  │                                    │                         │
│  │  • 需要一台境外 VPS（Linux）        │                         │
│  │  • VLESS + TLS 直连，延迟更低      │                         │
│  │  • 兼容性最佳，绝大多数服务可用    │                         │
│  │  • 约 10 分钟完成部署              │                         │
│  └────────────────────────────────────┘                         │
│                                                                  │
│  ┌────────────────────────────────────┐                         │
│  │  ☁️  Cloudflare Workers            │                         │
│  │       （无 VPS 时的应急方案）       │                         │
│  │                                    │                         │
│  │  • 无需 VPS，只需 CF 账号和域名    │                         │
│  │  • Free 计划每天 10 万次请求免费   │                         │
│  │  • ⚠️ 部分服务封禁 CF 出口节点    │                         │
│  │  • 约 5 分钟完成部署               │                         │
│  └────────────────────────────────────┘                         │
│                                                                  │
│  ❓ 不知道选哪个？→ [查看对比表]                                 │
└─────────────────────────────────────────────────────────────────┘
```

**对比表（点击"查看对比表"展开）：**

| 特性 | VPS + CF ★ 推荐 | CF Workers（应急）|
|---|---|---|
| 需要 VPS | ✅ 需要 | ❌ 不需要 |
| 成本 | 💰 VPS 月费（$3–$10）| 🆓 CF Free tier |
| 协议 | VLESS + TLS | VLESS + WS + TLS |
| 流量上限 | 无限（VPS 带宽内）| 10 万次/天（免费）/ 无限（付费）|
| 服务兼容性 | ✅ 最佳，几乎无封禁 | ⚠️ 部分服务封禁 CF Workers 出口 |
| CF CDN 中转 | ❌ 否（直连 VPS）| ✅ 是（隐藏真实 IP）|
| 需要域名 | ✅ 需要（用于 TLS 证书）| ✅ 需要（自动从 CF 账号绑定）|
| 部署时间 | ~10 分钟 | ~5 分钟 |

---

### 4.3 路径 A：Cloudflare Workers 向导（4 步）

```
[0. 选择类型] → [A1. CF 配置] → [A2. 部署确认] → [A3. 自动部署] → [A4. 完成]
```

### 4.4 路径 B：VPS + Cloudflare 向导（6 步）

```
[0. 选择类型] → [B1. VPS 连接] → [B2. CF 配置] → [B3. 部署确认] → [B4. 自动部署] → [B5. 完成]
```

---

## 五、功能需求

### 路径 A：Cloudflare Workers

#### Step A1：Cloudflare 配置

**用户填写：**

| 字段 | 类型 | 说明 |
|------|------|------|
| API Token | 文本 | 需要权限：Workers:Edit, Zone:DNS:Edit |
| Account ID | 文本 | 从 CF Dashboard 复制 |
| 选择域名（Zone）| 下拉 | 填完 Token + Account ID 后自动拉取 |
| 节点子域名前缀 | 文本 | 默认 `node1`，最终地址 = `node1.yourdomain.com` |
| Workers 名称 | 文本 | 默认 `clashforge-node`，用户通常无需修改 |

**UI 行为：**
- Token + Account ID 填完后自动触发 CF API 验证 + 拉取 Zone 列表
- 已在 ClashForge 中配置过 CF 凭据 → 自动填入，用户只需确认
- 节点地址实时预览：`node1.yourdomain.com`
- **「验证 Cloudflare 配置」** 按钮 → 校验 Token 权限（Workers:Edit + Zone:DNS:Edit）、Zone 存在性

> **为什么需要域名？** 用户既然已经提供了 CF Token，我们可以直接通过 CF API 将自定义域绑定到 Worker，
> 域名绑定后节点地址更稳定（不依赖 workers.dev 子域名的可用性），且自带 TLS 证书由 CF 自动管理。

#### Step A2：部署预览与确认

```
即将执行以下操作：

Cloudflare（yourdomain.com）
  ✦ 创建 Cloudflare Worker：clashforge-node
  ✦ 部署 VLESS + WebSocket 代理脚本，生成 VLESS UUID
  ✦ 绑定自定义域名：node1.yourdomain.com → Worker
  ✦ TLS 证书由 Cloudflare 自动签发管理

ClashForge（本路由器）
  ✦ 生成并导入节点订阅
  ✦ 应用 DNS 分流策略（split 模式）
  ✦ 启动透明代理服务
  ✦ 执行全项连通性测试

[ 取消 ]                        [ 确认，开始部署 → ]
```

#### Step A3：自动部署（进度流）

```
🔄 阶段 1 / 3：创建 Cloudflare Worker
   ✅ API 认证成功
   ✅ 创建 Worker：clashforge-node
   ✅ 上传 VLESS+WS 代理脚本（v1.2.0）
   ✅ 生成 VLESS UUID：xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
   ✅ 绑定自定义域名：node1.yourdomain.com
   ✅ TLS 证书由 Cloudflare 自动签发（无需等待）
   ✅ Worker 节点已上线：node1.yourdomain.com

🔄 阶段 2 / 3：导入 ClashForge
   ✅ 生成订阅配置（VLESS+WS+TLS）
   ✅ 添加到 ClashForge 订阅列表
   ✅ 应用 DNS 分流策略（split）
   ✅ 配置透明代理（TProxy 模式）
   ✅ 重新生成 Mihomo 配置
   ✅ 启动 ClashForge 服务

🔄 阶段 3 / 3：连通性验证
   ✅ 路由器侧：出口 IP → Cloudflare IP（Workers 节点）
   ✅ 路由器侧：Google 可达（延迟 85ms）
   ✅ 路由器侧：YouTube 可达
   ✅ 路由器侧：百度可达（DIRECT）
   ✅ DNS 泄露检测：上游 = Google/Cloudflare DoH（洁净）
```

#### Step A4：完成页

```
🎉 ClashForge 已成功启动！

节点信息
  地址：node1.yourdomain.com:443
  协议：VLESS + WebSocket + TLS（Cloudflare Workers）
  状态：✅ 在线

连通性
  ✅ Google / YouTube  — 延迟 85ms（经 CF Workers）
  ✅ 百度 / 微信       — 延迟 12ms（直连）
  ✅ DNS 洁净          — 无泄露

配置摘要
  DNS 策略：分流优先（split）
  透明代理：TProxy
  节点已保存至 → 节点管理

[ 查看仪表板 ]    [ 管理节点 ]    [ 调整设置 ]
```

---

### 路径 B：VPS + Cloudflare

#### Step B1：VPS 连接信息

**用户填写：**

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| 服务器地址 | 文本 | — | IP 或域名，如 `203.0.113.10` |
| SSH 端口 | 数字 | 22 | |
| 用户名 | 文本 | root | |
| 认证方式 | 单选 | 密码 | 密码 / SSH 私钥 |
| 密码 / 私钥内容 | 文本（密码模式）/ 文本区（密钥模式）| — | 私钥内容粘贴，支持 PEM 格式 |

**UI 行为：**
- 填完后点击 **「测试连接」** → 后端尝试 SSH 握手 → 显示结果
  - ✅ 连接成功，检测到 OS：Ubuntu 24.04 / Debian 12 / CentOS 9
  - ❌ 连接失败（超时 / 密码错误 / 端口不通）+ 具体错误信息
- 连接成功后「下一步」按钮才可点击

**校验规则：**
- 地址不为空
- 端口 1–65535
- 用户名不为空
- 凭据不为空

#### Step B2：Cloudflare 配置

**用户填写：**

| 字段 | 类型 | 说明 |
|------|------|------|
| API Token | 文本 | 需要权限：Zone:DNS:Edit, Zone:Zone:Read |
| Account ID | 文本 | 从 CF Dashboard 复制 |
| 选择域名（Zone）| 下拉 | 填完 Token + Account ID 后自动拉取 |
| 节点子域名前缀 | 文本 | 默认 `node1`，最终地址 = `node1.yourdomain.com` |

**UI 行为：**
- Token + Account ID 填完后自动触发 CF API 拉取 Zone 列表
- 已在 ClashForge 中配置过 CF 凭据 → 自动填入，用户只需确认
- 节点地址实时预览：`node1.yourdomain.com`
- **「验证 Cloudflare 配置」** 按钮 → 校验 Token 权限、Zone 存在性

#### Step B3：部署预览与确认

```
即将执行以下操作：

VPS（203.0.113.10）
  ✦ 检测系统环境（OS / CPU 架构 / 防火墙）
  ✦ 下载并安装 gost（SOCKS5+TLS 代理服务端）
  ✦ 申请 TLS 证书（Let's Encrypt via Cloudflare DNS-01）
  ✦ 创建 systemd 服务，设置开机自启

Cloudflare（yourdomain.com）
  ✦ 创建 DNS 记录：node1.yourdomain.com → 203.0.113.10
  ✦ 申请 TLS 证书（ACME + DNS-01 验证，无需开放 80/443 入站）

ClashForge（本路由器）
  ✦ 生成并导入节点订阅
  ✦ 应用 DNS 分流策略（split 模式）
  ✦ 启动透明代理服务
  ✦ 执行全项连通性测试

⚠️  注意：VPS 上将安装代理软件并开放 443 端口。
    操作可在部署完成后通过「节点管理」页面撤销。

[ 取消 ]                        [ 确认，开始部署 → ]
```

#### Step B4：自动部署（进度流）

```
🔄 阶段 1 / 5：VPS 环境检测
   ✅ SSH 连接已建立
   ✅ OS：Ubuntu 24.04 LTS (aarch64)
   ✅ 防火墙：ufw 已检测
   ✅ 443 端口：未被占用

🔄 阶段 2 / 5：部署代理服务端
   ✅ 下载 gost v3.x (aarch64)
   ✅ 安装到 /usr/local/bin/gost
   ✅ 写入配置：/etc/gost/config.yaml（SOCKS5+TLS，监听 443）
   ✅ 创建 systemd 服务：gost.service

🔄 阶段 3 / 5：TLS 证书 + DNS
   ✅ 在 Cloudflare 创建 A 记录：node1.yourdomain.com → 203.0.113.10
   ✅ 申请 Let's Encrypt 证书（DNS-01 验证）
   ⏳ 等待 DNS 生效（通常 10–60 秒）...
   ✅ 证书签发成功，有效期至 2026-08-27
   ✅ 证书写入 VPS：/etc/gost/cert/
   ✅ 启动 gost.service ... 已运行

🔄 阶段 4 / 5：导入 ClashForge
   ✅ 生成订阅配置（SOCKS5+TLS，节点名：node1.yourdomain.com）
   ✅ 添加到 ClashForge 订阅列表
   ✅ 应用 DNS 分流策略（split）
   ✅ 配置透明代理（TProxy 模式）
   ✅ 重新生成 Mihomo 配置
   ✅ 启动 ClashForge 服务

🔄 阶段 5 / 5：连通性验证
   ✅ 路由器侧：出口 IP 检测 → 203.0.x.x（VPS IP）
   ✅ 路由器侧：Google 可达
   ✅ 路由器侧：YouTube 可达
   ✅ 路由器侧：百度可达（DIRECT）
   ✅ DNS 泄露检测：上游 = Google/Cloudflare DoH（洁净）
```

**UI 交互细节：**
- 每个阶段独立展开/收起
- 当前阶段高亮 + 旋转加载图标
- 失败时：红色 ❌ + 错误详情 + **「查看完整日志」** + **「重试此阶段」**
- 不支持暂停（原子性）；可以中止（会弹出确认对话框，说明中止后的状态）

#### Step B5：完成页

```
🎉 ClashForge 已成功启动！

节点信息
  地址：node1.yourdomain.com:443
  协议：VLESS + TLS
  状态：✅ 在线

连通性
  ✅ Google / YouTube  — 延迟 65ms（经 node1）
  ✅ 百度 / 微信       — 延迟 12ms（直连）
  ✅ DNS 洁净          — 无泄露

配置摘要
  DNS 策略：分流优先（split）
  透明代理：TProxy
  节点已保存至 → 节点管理

[ 查看仪表板 ]    [ 管理节点 ]    [ 调整设置 ]
```

---

## 六、技术设计

### 6.1 新增后端包：`internal/quickstart`

```
internal/quickstart/
├── types.go          # 数据结构定义（含部署类型枚举）
├── pipeline.go       # 主部署流水线接口（工厂模式，按 DeployType 选择 pipeline）
├── pipeline_workers.go   # CF Workers 部署流水线
├── pipeline_vps.go       # VPS + CF 部署流水线
├── ssh.go            # SSH 连接、命令执行、文件上传封装
├── provision.go      # VPS 上的软件安装逻辑（仅 VPS 路径用）
├── acme.go           # Let's Encrypt ACME + CF DNS-01 证书申请（仅 VPS 路径用）
├── config.go         # gost config.yaml 生成（仅 VPS 路径用）
├── worker_script.go  # CF Worker VLESS 脚本生成（仅 Workers 路径用）
├── subscription.go   # Clash YAML 订阅生成（两条路径共用）
└── pipeline_test.go  # 单元测试（mock SSH / mock CF API）
```

#### `types.go` 核心结构

```go
// DeployType 部署类型
type DeployType string

const (
    DeployTypeCFWorkers DeployType = "cf_workers" // Cloudflare Workers（无 VPS）
    DeployTypeVPS       DeployType = "vps"         // VPS + Cloudflare
)

// QuickStartRequest 统一请求结构，按 DeployType 选填字段
type QuickStartRequest struct {
    DeployType DeployType       `json:"deploy_type"`
    Cloudflare CFCredentials    `json:"cloudflare"`
    // VPS 路径专用字段（DeployType == "vps" 时必填）
    VPS        *VPSCredentials  `json:"vps,omitempty"`
    NodePrefix string           `json:"node_prefix"` // "node1" 或 "clashforge-node"
    // Workers 路径专用字段
    WorkersDomain WorkersDomainConfig `json:"workers_domain,omitempty"`
}

type VPSCredentials struct {
    Host     string `json:"host"`
    Port     int    `json:"port"`   // default 22
    User     string `json:"user"`   // default "root"
    AuthType string `json:"auth_type"` // "password" | "key"
    Password string `json:"password,omitempty"`
    PrivKey  string `json:"priv_key,omitempty"` // PEM content
}

type CFCredentials struct {
    Token     string `json:"token"`
    AccountID string `json:"account_id"`
    ZoneID    string `json:"zone_id,omitempty"` // 仅 VPS 路径 / Workers 自定义域时用
    ZoneName  string `json:"zone_name,omitempty"`
}

// WorkersDomainConfig Workers 节点域名配置
// 用户已提供 CF Token，直接绑定自定义域名；不使用 workers.dev 子域
type WorkersDomainConfig struct {
    WorkerName   string `json:"worker_name"`   // CF Worker 名称，如 "clashforge-node"
    CustomDomain string `json:"custom_domain"` // 绑定的自定义域名，如 "node1.yourdomain.com"
    ZoneID       string `json:"zone_id"`       // 域名所属 CF Zone ID
}

// Phase 阶段枚举（两条路径共用前缀，各自有独立阶段）
type Phase string

const (
    // 通用阶段
    PhaseSSHTest    Phase = "ssh_test"    // VPS 连接测试
    PhaseCFTest     Phase = "cf_test"     // CF 凭据测试
    PhaseImport     Phase = "import"      // 导入 ClashForge
    PhaseVerify     Phase = "verify"      // 连通性验证
    // VPS 路径专用
    PhaseEnvDetect  Phase = "env_detect"  // VPS 环境检测
    PhaseProvision  Phase = "provision"   // 安装 xray
    PhaseCertDNS    Phase = "cert_dns"    // 证书 + DNS
    // Workers 路径专用
    PhaseWorkerDeploy Phase = "worker_deploy" // 创建并发布 Worker
)

// Event 流式事件（SSE）
type Event struct {
    Phase   Phase  `json:"phase"`
    Step    string `json:"step"`
    Status  string `json:"status"` // "running" | "ok" | "error" | "info"
    Message string `json:"message"`
    Detail  string `json:"detail,omitempty"`
}

// DeployState 持久化到磁盘，支持查询历史部署
type DeployState struct {
    ID          string            `json:"id"`
    DeployType  DeployType        `json:"deploy_type"`
    Request     QuickStartRequest `json:"request"` // 密码脱敏后
    Status      string            `json:"status"`  // "running" | "done" | "failed"
    NodeID      string            `json:"node_id,omitempty"`
    StartedAt   time.Time         `json:"started_at"`
    FinishedAt  *time.Time        `json:"finished_at,omitempty"`
    LastError   string            `json:"last_error,omitempty"`
}
```

### 6.2 新增 API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/v1/quickstart/validate-vps` | SSH 连接测试（仅 VPS 路径）|
| `POST` | `/api/v1/quickstart/validate-cf` | CF Token + 账号校验（两条路径）|
| `GET`  | `/api/v1/quickstart/cf-zones` | 拉取 CF Zone 列表（两条路径）|
| `POST` | `/api/v1/quickstart/deploy` | 启动部署（SSE 流式响应）|
| `GET`  | `/api/v1/quickstart/deploy/:id` | 查询历史部署状态 |
| `POST` | `/api/v1/quickstart/deploy/:id/abort` | 中止部署 |

#### `POST /quickstart/deploy` 请求体

```json
{
  "deploy_type": "cf_workers",
  "cloudflare": {
    "token": "...",
    "account_id": "..."
  },
  "node_prefix": "clashforge-node",
  "workers_domain": {
    "use_workers_dev": true,
    "worker_name": "clashforge-node"
  }
}
```

或：

```json
{
  "deploy_type": "vps",
  "vps": {
    "host": "203.0.113.10",
    "port": 22,
    "user": "root",
    "auth_type": "password",
    "password": "..."
  },
  "cloudflare": {
    "token": "...",
    "account_id": "...",
    "zone_id": "...",
    "zone_name": "yourdomain.com"
  },
  "node_prefix": "node1"
}
```

#### SSE 响应格式

```
Content-Type: text/event-stream

data: {"phase":"worker_deploy","step":"create_worker","status":"ok","message":"Worker clashforge-node 已创建"}
data: {"phase":"worker_deploy","step":"upload_script","status":"running","message":"上传 VLESS+WS 脚本..."}
...
data: {"phase":"verify","step":"done","status":"ok","message":"所有测试通过"}
```

### 6.3 路径 A 技术实现：Cloudflare Workers

#### 6.3.1 Worker 脚本复用现有架构

`internal/workernode/` 包已经实现了 CF Worker VLESS+WS 代理逻辑。
`pipeline_workers.go` 直接调用现有逻辑：

```go
// pipeline_workers.go

func (p *WorkersPipeline) Run(ctx context.Context, req *QuickStartRequest, events chan<- Event) error {
    // Phase 1: 创建 CF Worker
    workerID, uuid, err := p.workerClient.CreateWorker(ctx, req)
    //  └─ 复用 internal/workernode 的 Worker 部署逻辑
    //  └─ 生成 VLESS UUID，嵌入 Worker 脚本
    //  └─ 调用 CF API 发布 Worker

    // Phase 2: 导入 ClashForge
    node := buildWorkerNode(req, workerID, uuid)
    err = p.importNode(ctx, node)

    // Phase 3: 验证
    return p.verify(ctx, events)
}
```

**Workers 节点地址：** 直接使用 `WorkersDomainConfig.CustomDomain`，由 CF API 自动绑定到 Worker 并签发 TLS 证书。无需推导 workers.dev 子域。

#### 6.3.2 CF API 新增方法

扩展现有 `internal/cloudflare/` 客户端，新增：

```go
// DeployWorker 创建/更新 Worker 脚本（支持 module 格式）
func (c *Client) DeployWorker(ctx context.Context, accountID, workerName, scriptContent string) error

// AddWorkerCustomDomain 将自定义域名绑定到 Worker（CF 自动签发 TLS 证书）
// 对应 CF API: PUT /accounts/{account_id}/workers/domains
func (c *Client) AddWorkerCustomDomain(ctx context.Context, accountID, workerName, zoneID, domain string) error
```

> **注意：** `internal/workernode/` 目前是否已封装上述 CF API 操作，实现前需确认，
> 若已有则直接复用，避免重复实现。

### 6.4 路径 B 技术实现：VPS + Cloudflare

#### 6.4.1 环境检测（`provision.go: DetectEnv`）

```go
type EnvInfo struct {
    OS        string // "ubuntu" | "debian" | "centos" | "rhel" | "alpine"
    OSVersion string // "24.04"
    Arch      string // "amd64" | "arm64"
    Firewall  string // "ufw" | "firewalld" | "iptables" | "none"
    Port443   bool   // 443 是否已被占用
    HasSystemd bool
}
```

#### 6.4.2 安装 gost

```bash
GOST_VERSION="v3.0.0"
ARCH="$(uname -m | sed 's/x86_64/amd64/ ; s/aarch64/arm64/')"
curl -fsSL "https://github.com/go-gost/gost/releases/download/${GOST_VERSION}/gost_${GOST_VERSION#v}_linux_${ARCH}.tar.gz" \
    -o /tmp/gost.tar.gz
tar -xzf /tmp/gost.tar.gz -C /usr/local/bin/ gost
chmod +x /usr/local/bin/gost
mkdir -p /etc/gost/cert
```

> **版本管理：** gost 版本号从 ClashForge 发布版本中读取（与 ClashForge 版本保持一致，Q3 决策）。

#### 6.4.3 gost config.yaml 模板

```yaml
# /etc/gost/config.yaml — 由 ClashForge QuickStart 自动生成
services:
  - name: clashforge-node
    addr: ":443"
    handler:
      type: socks5
    listener:
      type: tls
      tls:
        certFile: /etc/gost/cert/fullchain.pem
        keyFile: /etc/gost/cert/privkey.pem
```

对应的 Clash/Mihomo 订阅节点格式：

```yaml
proxies:
  - name: "node1.yourdomain.com"
    type: socks5
    server: node1.yourdomain.com
    port: 443
    tls: true
    skip-cert-verify: false
```

#### 6.4.4 systemd 服务

```ini
[Unit]
Description=Gost Proxy Service (ClashForge)
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/gost -C /etc/gost/config.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

#### 6.4.5 防火墙开放 443

```bash
# ufw
ufw allow 443/tcp
# firewalld
firewall-cmd --permanent --add-port=443/tcp && firewall-cmd --reload
# iptables（fallback）
iptables -I INPUT -p tcp --dport 443 -j ACCEPT
```

#### 6.4.6 TLS 证书（`acme.go`）

**ACME v2 + Cloudflare DNS-01 验证**，无需 VPS 开放 80/443 入站（申请阶段）。

```go
func IssueCert(ctx context.Context, domain string, cfToken string, zoneID string) (*CertPair, error) {
    // 1. 用 golang.org/x/crypto/acme 创建 ACME 账户
    // 2. 请求 DNS-01 challenge
    // 3. 调用 CF API 创建 _acme-challenge TXT 记录
    // 4. 等待 DNS 生效（最多 120s）
    // 5. 通知 ACME 完成 challenge
    // 6. 下载证书链 + 私钥
    // 7. 删除 TXT 记录
    // 8. 通过 SSH 将证书写入 VPS /etc/gost/cert/
    return certPair, nil
}
```

> **CF DNS 代理状态：** A 记录使用**灰云（直连）**，不开 CF CDN 代理。
> 原因：gost SOCKS5+TLS 是原始 TCP 连接，CF CDN 只代理 HTTP(S)，强制开橙云会断连。
> 若需 CF CDN 中转 → 改用 VLESS+WS+TLS（即路径 A），留 v2.1 作为路径 B 的协议选项。

### 6.5 两条路径的公共模块

#### 6.5.1 ClashForge 自动配置

```go
func AutoConfigure(ctx context.Context, deps Dependencies) error {
    cfg := deps.Config
    cfg.DNS.Strategy      = config.DNSStrategysplit  // 分流优先（与 DNS PRD 一致）
    cfg.Network.Mode      = "tproxy"
    cfg.Network.BypassChina = true
    cfg.DNS.ApplyOnStart  = true
    cfg.Network.ApplyOnStart = true
    // 保存配置 → 生成 Mihomo config → 启动 core
    return nil
}
```

#### 6.5.2 连通性验证

复用现有 `internal/api/handler_dns_leak.go` 和 Dashboard 连通性测试逻辑，
在 QuickStart pipeline 里**服务端**发起，不依赖浏览器端探测：

```go
type VerifyResult struct {
    OutboundIP string // 应为 VPS IP 或 CF Worker IP
    Google     bool
    YouTube    bool
    Baidu      bool  // 应走 DIRECT
    DNSClean   bool  // 上游 resolver 不是 ISP DNS
}
```

---

## 七、与现有流程的关系

```
用户入口
  ├─ 快速启动 v2（本文档）← 新增，独立路由 /quickstart
  │    ├─ 路径 A：CF Workers（复用 internal/workernode/）
  │    └─ 路径 B：VPS + CF（新增 internal/quickstart/ 包）
  │
  └─ 手动设置 v1（现有 Setup 向导）← 完整保留，不改动
       ├─ 导入配置
       ├─ DNS 设置
       ├─ 网络设置
       ├─ 启动服务
       └─ 连通性检测

v2 产生的节点、订阅、配置 ──► 完全兼容 v1 的管理页面
                               （节点管理、订阅管理、Settings 等）
```

---

## 八、错误处理与恢复策略

### 路径 A（CF Workers）

| 阶段 | 常见错误 | 处理策略 |
|------|----------|----------|
| CF 验证 | Token 无 Workers:Edit 权限 | 明确指出缺失权限 + 链接 CF Token 配置文档 |
| Worker 创建 | Account 未开通 Workers | 提示开通方式（CF Dashboard → Workers & Pages）|
| Worker 发布 | 脚本语法错误（代码 bug）| 上报错误 + 建议更新 ClashForge |
| 连通性验证 | Workers 节点延迟 > 3000ms | 警告但不阻断（CF 边缘节点可能距离远）|

### 路径 B（VPS + CF）

| 阶段 | 常见错误 | 处理策略 |
|------|----------|----------|
| SSH 连接 | 密码错误 / 端口不通 / 超时 | 明确错误提示 + 返回 Step B1 修改 |
| OS 检测 | 不支持的系统（FreeBSD 等）| 提示支持范围 + 建议 Ubuntu/Debian |
| 下载 xray | GitHub 无法访问 | 自动尝试 ghproxy 镜像（与 clashforgectl 一致）|
| 443 端口占用 | nginx / caddy 已在运行 | 提示冲突进程 + 建议指定其他端口 |
| ACME 证书 | DNS 未生效（超时）| 等待 + 自动重试，最多 5 分钟 |
| CF API | Token 无 DNS Edit 权限 | 明确指出缺失权限 + 链接 CF Token 配置文档 |
| 启动 xray | systemd 启动失败 | 显示 `journalctl` 最后 20 行 |
| 连通性验证 | 节点完全不可达 | 提供诊断建议（防火墙 / 证书 / 路由）|

### 通用保证

**幂等性：**
- 每次部署生成唯一 `deployID`
- 阶段状态持久化到磁盘（`/etc/metaclash/quickstart/`）
- 中途失败后可从失败阶段重试（而非从头）
- 重复运行同一 Worker Name / VPS + 域名：检测到已有配置，提示"覆盖 / 跳过"

---

## 九、UI 设计规范

### 9.1 路由

新增独立路由 `/quickstart`，在 React Router 中注册。
导航栏不显示此路由（专用向导页面，完成后跳转到 Dashboard）。

### 9.2 整体布局

与现有 Setup 向导一致：
- 左侧：步骤进度条（固定，根据所选路径动态显示步骤）
- 右侧：当前步骤内容（滚动）
- 底部：「上一步 / 下一步 / 开始部署」按钮

**步骤进度条对比：**

```
路径 A（CF Workers）          路径 B（VPS + CF）
① 选择类型  ✅                ① 选择类型  ✅
② CF 配置   🔄                ② VPS 连接  ✅
③ 部署确认  ○                 ③ CF 配置   🔄
④ 自动部署  ○                 ④ 部署确认  ○
⑤ 完成      ○                 ⑤ 自动部署  ○
                               ⑥ 完成      ○
```

### 9.3 Step 0 — 选择部署类型 UX

- 两个大卡片，左右并排（桌面）/ 上下排列（移动）
- 卡片支持键盘导航（Tab / Space 选择）
- 选中卡片：紫色描边 + 左上角 ✅ 徽章
- "查看对比表"：点击展开内联对比表，不跳转新页面
- 选择后不立即跳转，等用户点「下一步」确认

### 9.4 Step 部署进度 — 特殊设计

```
阶段进度条（路径 A 示例）
┌──────────────────────────────────────────────┐
│ ① CF认证 ─── ② 创建Worker ─── ③ 导入 ─── ④ 验证 │
│      ✅              🔄          ○          ○ │
└──────────────────────────────────────────────┘

当前阶段日志（实时滚动）
┌──────────────────────────────────────────────┐
│ [22:31:05] ✅ 上传 VLESS+WS 脚本（v1.2.0）  │
│ [22:31:06] ⏳ 等待 Worker 发布...            │
│ [22:31:08] ✅ Worker 已上线                  │
│ ▌                                            │
└──────────────────────────────────────────────┘

[ 查看完整原始日志 ↓ ]
```

### 9.5 颜色语义

与现有 Setup 向导一致：
- 绿色 `text-success` → 成功
- 黄色 `text-warning` → 警告 / 等待
- 红色 `text-danger` → 错误
- `text-brand`（紫色）→ 进行中

---

## 十、数据模型变更

| 层 | 位置 | 变更 |
|---|---|---|
| 新增包 | `internal/quickstart/` | 全新（VPS 路径 pipeline）|
| CF API 扩展 | `internal/cloudflare/` | 新增 DNS Record CRUD + Worker Deploy API |
| workernode 复用 | `internal/workernode/` | 路径 A 直接调用，可能需要导出部分内部方法 |
| 存储 | `/etc/metaclash/quickstart/` | 部署历史 JSON |
| API 路由 | `internal/api/router.go` | 注册 `/quickstart/*` |
| 前端路由 | `ui/src/App.tsx` | 注册 `/quickstart` |
| 新增页面 | `ui/src/pages/QuickStart.tsx` | 全新（含类型选择 + 两套向导步骤）|
| 主页入口 | `ui/src/pages/Dashboard.tsx` 或 Overview | 增加快速启动入口卡片 |

---

## 十一、依赖

### Go 新增依赖

| 包 | 用途 | 路径 |
|---|---|---|
| `golang.org/x/crypto/ssh` | SSH 客户端 | 路径 B |
| `golang.org/x/crypto/acme` | ACME v2 协议 | 路径 B |

> `golang.org/x/crypto` 已在项目中使用，只需确认版本包含 `acme` 子包。

### 无新增前端依赖

使用现有 UI 组件库（glass-card, Toggle, SelectInput 等）。

---

## 十二、不在本期范围（v2.1+）

| 功能 | 理由 |
|------|------|
| VLESS+WS+TLS via CF CDN 中转（路径 B 协议升级）| 路径 A 已覆盖此场景 |
| Reality 协议 | 配置复杂，非新手友好 |
| 多节点同时部署 | v2 只做单节点快速启动 |
| 节点健康监控 / 自动切换 | 在现有 Dashboard 层面已有 |
| 一键扩容（第 N 台 VPS / 第 N 个 Worker）| v2 完成后评估 |
| Windows 路由器（WSL 环境）| 当前目标是 OpenWrt |
| 自定义端口（路径 B 非 443）| 界面简化，高级模式留后 |

---

## 十三、里程碑

| 阶段 | 内容 | 估时 |
|---|---|---|
| **M0** | 依赖确认 + `internal/quickstart` 包骨架 + 确认 `internal/workernode` 可复用范围 | 0.5 天 |
| **M1（路径 A）** | CF Workers 部署 pipeline：账号子域名获取、Worker 创建/发布、节点导入 | 1 天 |
| **M2（路径 B）** | SSH 连接 + VPS 环境检测 + gost 安装 + systemd 服务 | 1.5 天 |
| **M3（路径 B）** | ACME 证书 + CF DNS A 记录 + 证书上传到 VPS | 1 天 |
| **M4（共用）** | 订阅生成 + 自动导入 + ClashForge 配置 + 启动 + 连通性验证（服务端）| 0.5 天 |
| **M5（共用）** | 部署状态持久化 + SSE 流式 API + 幂等性 / 重试 | 0.5 天 |
| **M6（前端）** | `/quickstart` 页面：类型选择 + 路径 A 向导 + 路径 B 向导 + 进度流 + 完成页 | 2 天 |
| **M7** | 错误处理完善 + 端到端测试（mock + 真实 CF 沙盒）| 1 天 |
| **合计** | | **~8 天** |

---

## 十四、开放问题（需决策后才能动手）

| # | 问题 | 选项 | 建议 |
|---|------|------|------|
| Q1 | ~~VPS 侧代理软件选型（路径 B）~~ | ~~xray-core / sing-box / gost~~ | ✅ **已决策：gost。** 目前仅支持 gost，其他软件日后再说。|
| Q2 | ~~证书方案（路径 B）~~ | ~~Let's Encrypt / 自签名~~ | ✅ **已决策：Let's Encrypt。** 现有默认方案，无需变更。|
| Q3 | ~~Workers 脚本版本管理~~ | ~~固定版本号 / 跟随 ClashForge 版本~~ | ✅ **已决策：跟随 ClashForge 版本。** 保持一致，随产品发布更新。|
| Q4 | ~~失败后 VPS 清理（路径 B）~~ | ~~自动回滚 / 保留 + 提示~~ | ✅ **已决策：保留 + 提示手动清理。** 回滚脚本容易出错，且用户 VPS 上可能有其他服务。|
| Q5 | ~~GitHub 无法访问时 gost 下载回退（路径 B）~~ | ~~自动切 ghproxy / 用户提供 URL~~ | ✅ **已决策：暂不处理。** 当前不使用 xray，gost 下载回退策略日后与 gost 支持一起完善。|
| Q6 | ~~路径 A 默认用 workers.dev 还是强制要求域名？~~ | ~~workers.dev / 必须有域名~~ | ✅ **已决策：强制自定义域名。** 用户已提供 CF Token，系统可直接通过 API 完成域名绑定，节点地址更稳定，且 CF 自动管理 TLS 证书，零额外操作。|
| Q7 | ~~两条路径的入口优先级~~ | ~~CF Workers 推荐 / 并列~~ | ✅ **已决策：VPS + CF 标注"推荐"。** CF Workers 出口节点被部分服务封禁，仅作为没有 VPS 时的应急方案。有 VPS 的用户应首选路径 B。|
