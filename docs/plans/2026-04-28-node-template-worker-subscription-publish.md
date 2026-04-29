# ClashForge 节点模板发布（CF Worker）落地方案

## 1. 目标与范围

目标：在 ClashForge 内实现一套“可发布订阅链接”的完整链路，能力来自三部分组合：

1. 选择 **节点服务器**（已在 ClashForge 的 `节点管理` 中部署成功的节点）。  
2. 使用 **Loyalsoldier/clash-rules** 规则模板生成规则化配置。  
3. 将最终 YAML 发布到 **Cloudflare Worker + KV**，产出可直接给客户端使用的订阅 URL。

本期只新增“发布能力”，不改现有 Setup/Core/接管业务逻辑。

---

## 2. 现状盘点（基于当前代码）

### 2.1 ClashForge 现有能力（可直接复用）

- 已有节点生命周期管理：`internal/nodes/*` + `ui/src/pages/Nodes.tsx`  
  （SSH 测试、部署 GOST、导出单节点 Clash 代理段）
- 已有订阅缓存与运行配置生成：`internal/subscription/*`、`internal/config/*`
- 已有设备路由增强（影子组 + RULE-SET + AND）：`internal/config/device_groups.go`
- 已有 Setup 最终配置预览与高亮解释：`ui/src/pages/Setup.tsx`

### 2.2 airport-tool 可借鉴能力

- 发布链路：节点选择 -> 模板合并 -> 预览 -> 上传到 Worker KV -> 版本化链接  
  参考：`backend/src/routes/publish.ts`
- CF Worker/KV 向导：权限校验、创建 namespace、部署 Worker、绑定域名、连通性验证  
  参考：`backend/src/routes/kv-setup.ts`、`backend/src/worker-template.ts`

### 2.3 Loyalsoldier 规则模板输入

- 规则集地址与典型 `rule-providers/rules` 结构可直接作为 ClashForge 内置模板来源。  
  参考仓库：`https://github.com/Loyalsoldier/clash-rules`

---

## 3. 产品形态（落地后的用户流程）

新增一个页面：`/publish`（建议中文名：**订阅发布**）。

用户流程：

1. 选择“配置来源”  
   - A. 使用 ClashForge 当前最终运行配置（含设备路由增强）  
   - B. 使用内置 Loyalsoldier 模板  
   - C. 粘贴/上传自定义模板
2. 选择节点（可多选，来源于 `status=deployed` 的节点）
3. 预览最终 YAML（高亮显示：节点注入块、规则模板块、ClashForge 管理块）
4. 选择/新建 Worker 托管环境（向导式）
5. 发布并生成版本化订阅链接
6. 在发布记录中复制/删除历史版本

---

## 4. 技术方案

### 4.1 后端模块拆分

新增 `internal/publish/`：

- `types.go`：数据结构（WorkerConfig、PublishRecord、PublishRequest、TemplatePreset）
- `store.go`：文件化持久化（沿用 ClashForge 现有 JSON/TOML 风格）
- `crypto.go`：敏感信息（token）AES-GCM 加密存储
- `template.go`：模板加载（内置 Loyalsoldier + 用户输入）
- `merge.go`：节点注入与分组算法（参考 airport-tool，但适配现有 NodeStore）
- `cloudflare.go`：CF API 调用封装（Workers scripts/domains/KV）
- `worker_script.go`：内置 worker 脚本字符串（TEXT2KV 兼容）

不引入数据库，保持当前“轻量文件存储”策略。

### 4.2 数据落盘设计（`Core.DataDir`）

- `publish-worker-configs.json`：托管环境列表（可多套）
- `publish-records.json`：发布历史（版本号、文件名、时间、所属托管环境）
- `publish.key`：发布模块加密密钥（仅本机 0600）

`WorkerConfig.token` 必须加密存储；API 返回时只给掩码与摘要。

### 4.3 配置合并策略

### 基础策略

- `proxies`：由选中节点生成（HTTP over TLS，复用节点的 `domain/proxy_user/proxy_password`）
- `proxy-groups`：
  - 自动生成主选择组：`🚀 节点选择`
  - 自动生成自动测速组：`♻️ 自动选择`
  - 按国家/地区生成 url-test 组（可选）
- 模板中旧代理名替换为 `🚀 节点选择` 引用，保留模板原有结构

### 规则策略

- 内置 Loyalsoldier 模板提供标准 `rule-providers + rules`
- 若用户使用“当前最终运行配置”为来源，则优先保留用户当前规则与设备路由增强块
- 输出前执行一次 YAML 结构校验与最小字段校验（proxies/proxy-groups/rules）

### 4.4 Cloudflare Worker/KV 托管流程

按向导分 5 步（与 airport-tool 一致）：

1. 校验 Token 权限（Workers Scripts + Workers KV + Zone）
2. 创建或复用 KV Namespace
3. 上传 Worker 脚本并绑定 `KV` + `TOKEN`
4. 绑定自定义域名到 Worker
5. 写入/读取/鉴权三段式验证，成功后保存托管配置

发布文件时使用 `b64` 参数写入，避免 YAML 特殊字符破坏 URL。

### 4.5 版本发布策略

- 文件名规则：`{baseName}.v{N}.{YYYYMMDD}.yaml`
- 每个 WorkerConfig + baseName 独立递增版本
- 记录中保留 `access_url`（含 token）用于快速复制
- 删除记录时：
  - 优先删除 Worker KV 上的 key
  - 本地记录无论 KV 删除是否成功都可删除（同时展示 warning）

---

## 5. API 设计（新增）

挂到 `/api/v1/publish/*`（并保留现有 `/api/v1/nodes/*`）：

- `GET /publish/nodes`：返回可发布节点（`status=deployed`）
- `GET /publish/templates`：返回模板预设列表
- `POST /publish/preview`：输入模板+节点，返回 merged YAML
- `GET /publish/worker-configs`：托管环境列表（脱敏）
- `POST /publish/worker/check-permissions`
- `POST /publish/worker/create-namespace`
- `POST /publish/worker/deploy-script`
- `POST /publish/worker/bind-domain`
- `POST /publish/worker/verify-save`
- `POST /publish/upload`：发布到 Worker KV 并返回订阅链接
- `GET /publish/records`：发布历史
- `DELETE /publish/records/{id}`：删除发布记录（含远端 KV best-effort 删除）

---

## 6. 前端方案（新增页面）

新增页面：`ui/src/pages/Publish.tsx`，路由 `/publish`，侧边栏新增入口“订阅发布”。

页面结构建议：

1. 模式选择卡：  
   - 当前运行配置 / 内置规则模板 / 自定义模板
2. 节点选择区：  
   - 按国家分组 + 全选/反选 + 节点状态提示
3. 配置预览区：  
   - YAML 代码窗 + 高亮标签（节点注入/规则模板/保留字段）
4. Worker 向导弹窗：  
   - 5 步状态条 + 错误细节
5. 发布结果区：  
   - 订阅 URL、版本号、文件名、复制按钮
6. 发布历史表：  
   - 复制链接、删除版本

API 客户端增补在 `ui/src/api/client.ts`。

---

## 7. 实施阶段（建议）

### Phase 1：发布后端骨架

- 新建 `internal/publish/{types,store,crypto}.go`
- 新建 `internal/api/handler_publish_*.go` 基础路由
- `internal/api/server.go` 注册新路由

验收：可创建/读取 WorkerConfig 与 PublishRecord（本地）。

### Phase 2：模板与合并引擎

- 新建 `internal/publish/template.go`（内置 Loyalsoldier 预设）
- 新建 `internal/publish/merge.go`（节点注入、group 修复、规则保留）

验收：`POST /publish/preview` 可输出合法 YAML。

### Phase 3：Cloudflare 向导

- 新建 `internal/publish/cloudflare.go` + `worker_script.go`
- 实现 check/create/deploy/bind/verify 五步 API

验收：可在真实 CF 账号下完成一套 Worker 环境初始化。

### Phase 4：发布与版本管理

- 实现 `POST /publish/upload`、`GET /publish/records`、`DELETE /publish/records/{id}`

验收：能生成可访问订阅 URL，历史可回看和删除。

### Phase 5：前端页面

- 新建 `ui/src/pages/Publish.tsx`
- `ui/src/App.tsx`、`ui/src/components/Sidebar.tsx` 增加入口
- `ui/src/api/client.ts` 增加 publish 相关 API

验收：完整 UI 闭环可操作。

### Phase 6：联调与回归

- 回归 Setup、Device Rules、Nodes、Subscriptions 页面
- 增加单测/集成测试（至少覆盖 merge 与版本递增）

验收：新增功能稳定，旧流程无回归。

---

## 8. 风险与规避

1. **CF API 频繁变更**  
   - 统一收敛到 `internal/publish/cloudflare.go`，避免散落调用
2. **模板不规范导致 YAML 失效**  
   - preview 与 upload 前都做结构校验
3. **token 泄露风险**  
   - token 仅后端持有，前端只见掩码
4. **OpenWrt 存储有限**  
   - 发布记录支持上限与清理策略（例如保留最近 N 条）

---

## 9. 验收标准（Definition of Done）

1. 用户可在 ClashForge 内选择节点 + 模板并预览最终 YAML。  
2. 用户可通过向导完成 Worker/KV 初始化（含连通性验证）。  
3. 用户可一键发布并拿到可用订阅链接（版本化文件名）。  
4. 用户可在历史记录中复制链接与删除发布版本。  
5. 现有 Setup / Device Rules / Nodes / Subscriptions 功能行为不变。  

---

## 10. 参考

- Loyalsoldier 规则集：`https://github.com/Loyalsoldier/clash-rules`  
- Cloudflare Workers multipart metadata：`https://developers.cloudflare.com/workers/configuration/multipart-upload-metadata/`  
- Cloudflare Workers Domains API：`https://developers.cloudflare.com/api/resources/workers/subresources/domains/`  
- Cloudflare KV Values API：`https://developers.cloudflare.com/api/resources/kv/subresources/namespaces/subresources/values/methods/update/`
