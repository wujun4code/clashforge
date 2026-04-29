# Fork 后配置自己的 Cloudflare R2 下载加速

本文适用于 **fork 了 clashforge 仓库**、希望自己构建并通过 Cloudflare R2 分发 IPK 文件的用户。

整个流程不需要公网服务器，只需要一个免费的 Cloudflare 账号。

---

## 流程概览

```
Fork 仓库
  → 创建 Cloudflare R2 Bucket
  → 创建 R2 API Token
  → 在 GitHub 仓库填写 4 个 Secrets
  → 触发 workflow，文件自动同步到你的 R2
```

---

## 第一步：Fork 仓库

点击 GitHub 页面右上角的 **Fork** 按钮，将仓库 fork 到你自己的账号下。

---

## 第二步：创建 Cloudflare R2 Bucket

1. 登录 [Cloudflare Dashboard](https://dash.cloudflare.com/)
2. 左侧菜单选择 **R2 Object Storage**
3. 点击 **Create bucket**
4. Bucket 名称随意，例如：`clashforge-releases`
5. 区域选 **Automatic**，点击创建

> R2 每月有 10GB 免费存储 + 100 万次免费请求，对 IPK 文件的分发完全够用。

---

## 第三步：创建 R2 API Token

1. 在 R2 页面，点击右上角 **Manage R2 API tokens**
2. 点击 **Create API token**
3. 填写配置：
   - **Token name**：随意，例如 `clashforge-github-actions`
   - **Permissions**：选 `Object Read & Write`
   - **Specify bucket**：选你刚建的 bucket（不要选 All buckets）
4. 点击 **Create API Token**
5. 页面会显示以下信息，**只显示一次，请立刻保存**：

   | 字段 | 用途 |
   |------|------|
   | Access Key ID | 填入 GitHub Secret `R2_ACCESS_KEY_ID` |
   | Secret Access Key | 填入 GitHub Secret `R2_SECRET_ACCESS_KEY` |

---

## 第四步：获取 Account ID

1. 登录 Cloudflare Dashboard
2. 点击任意域名（或直接看右侧边栏）
3. 找到 **Account ID**，复制备用

---

## 第五步：在 GitHub 仓库配置 Secrets

进入你 fork 后的仓库页面：

**Settings → Secrets and variables → Actions → New repository secret**

依次添加以下 4 个 secret：

| Secret 名称 | 填写内容 |
|-------------|----------|
| `R2_ACCOUNT_ID` | 第四步获取的 Cloudflare Account ID |
| `R2_BUCKET_NAME` | 你的 R2 Bucket 名称，例如 `clashforge-releases` |
| `R2_ACCESS_KEY_ID` | 第三步的 Access Key ID |
| `R2_SECRET_ACCESS_KEY` | 第三步的 Secret Access Key |

---

## 第六步：手动触发同步

配置好 Secrets 后，就可以手动将任意版本的 IPK 同步到你的 R2 了。

1. 进入你的仓库 → **Actions** 标签
2. 左侧列表选择 **Sync Release Assets to Cloudflare R2**
3. 点击右侧 **Run workflow**
4. 在 `tag` 输入框填写你想同步的版本号，例如：

   ```
   v0.1.0-rc.1
   ```

5. 点击绿色 **Run workflow** 按钮

等待约 30-60 秒，workflow 跑完后文件会出现在你的 R2 bucket 中。

---

## 第七步：确认文件已上传

回到 Cloudflare R2，进入你的 bucket，应该看到如下目录结构：

```
releases/
├── v0.1.0-rc.1/
│   ├── clashforge_0.1.0-rc.1_x86_64.ipk
│   ├── clashforge_0.1.0-rc.1_aarch64_generic.ipk
│   ├── clashforge_0.1.0-rc.1_aarch64_cortex-a53.ipk
│   ├── install.sh
│   └── SHA256SUMS.txt
└── latest/
    └── （同上，始终指向最新同步的版本）
```

---

## 第八步（可选）：绑定自定义域名

如果你希望用自己的域名提供下载，可以在 R2 bucket 页面绑定一个域名：

1. 进入 bucket → **Settings** → **Custom Domains**
2. 点击 **Connect Domain**
3. 填入你的域名，例如 `releases.example.com`
4. 按提示在 DNS 添加 CNAME 记录

绑定后，用户可以通过如下链接下载：

```sh
wget https://releases.example.com/releases/v0.1.0-rc.1/clashforge_0.1.0-rc.1_x86_64.ipk
```

如果暂时不绑定自定义域名，也可以在 R2 bucket 的 **Settings → Public Access** 开启公开访问，使用 Cloudflare 自动分配的 `*.r2.dev` 域名。

> ⚠️ `r2.dev` 公共访问仅建议测试使用，生产建议绑定自己的域名。

---

## 自动触发（进阶）

如果你在自己的 fork 中发布了新的 release，workflow 会**自动触发**，无需手动操作。

触发条件：在 GitHub 上 **publish** 一个 release（draft 不触发，只有正式发布才会）。

---

## 常见问题

### workflow 报错 `NoSuchBucket`

Secret `R2_BUCKET_NAME` 填写的 bucket 名称与实际不一致，检查拼写。

### workflow 报错 `InvalidAccessKeyId`

`R2_ACCESS_KEY_ID` 填写有误，或者 API Token 已被删除，重新创建一个。

### workflow 报错 `SignatureDoesNotMatch`

`R2_SECRET_ACCESS_KEY` 填写有误，注意 Secret Access Key 只在创建时显示一次，需要重新创建 Token。

### workflow 报错 `AccessDenied`

API Token 权限不足，确保创建时选择了 `Object Read & Write` 且绑定了正确的 bucket。

### 找不到 `Sync Release Assets to Cloudflare R2` 这个 workflow

fork 后 GitHub Actions 默认是禁用的。进入你的仓库 → **Actions** 标签 → 点击 **I understand my workflows, go ahead and enable them**。

---

## 安全说明

- R2 API Token 只授权了指定 bucket 的读写权限，不会影响你 Cloudflare 账号下的其他资源
- GitHub Secrets 加密存储，不会出现在 workflow 日志中
- 建议为每个 fork 仓库单独创建一个 API Token，便于独立管理和撤销

---

*如有问题，欢迎在原仓库提交 Issue。*
