import { useEffect, useState } from 'react'
import {
  AlertCircle,
  CheckCircle2,
  CloudCog,
  Copy,
  Download,
  Check,
  Clock,
  Key,
  Loader2,
  RotateCw,
  Trash2,
} from 'lucide-react'
import { ModalShell } from './ui'
import {
  createWorkerNode,
  getCloudflareZones,
  getWorkerNodeFreeTierInfo,
  type WorkerNodeFreeTierInfo,
  type WorkerNodeListItem,
  type CloudflareZone,
} from '../api/client'
import type { CFConfig } from './CFConfig'

// ── helpers ───────────────────────────────────────────────────────────────────

function toWorkerName(name: string) {
  return name.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/-+/g, '-').replace(/^-|-$/g, '').slice(0, 63) || 'cf-proxy'
}

async function copyText(text: string) {
  if (navigator.clipboard?.writeText) { await navigator.clipboard.writeText(text); return }
  const el = document.createElement('textarea')
  el.value = text; el.style.cssText = 'position:fixed;opacity:0'; document.body.appendChild(el)
  el.select(); document.execCommand('copy'); document.body.removeChild(el)
}

// ── WorkerNodeWizard ──────────────────────────────────────────────────────────

const EXPIRY_OPTIONS = [
  { label: '不过期', value: 0 },
  { label: '30 天', value: 30 },
  { label: '60 天', value: 60 },
  { label: '90 天（推荐）', value: 90 },
  { label: '180 天', value: 180 },
  { label: '365 天', value: 365 },
]

export function WorkerNodeWizard({
  cfConfig,
  onClose,
  onCreated,
}: {
  cfConfig: CFConfig
  onClose: () => void
  onCreated: (node: WorkerNodeListItem) => void
}) {
  const [name, setName] = useState('')
  const [workerName, setWorkerName] = useState('')
  const [workerNameEdited, setWorkerNameEdited] = useState(false)
  const [hostnamePrefix, setHostnamePrefix] = useState('')
  const [zoneId, setZoneId] = useState('')
  const [zones, setZones] = useState<CloudflareZone[]>([])
  const [zonesLoading, setZonesLoading] = useState(false)
  const [zonesError, setZonesError] = useState('')
  const [expiresInDays, setExpiresInDays] = useState(90)

  const [deploying, setDeploying] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<{ node: WorkerNodeListItem; yaml: string } | null>(null)
  const [copied, setCopied] = useState(false)

  const loadZones = async () => {
    setZonesLoading(true); setZonesError('')
    try {
      const data = await getCloudflareZones({ cf_token: cfConfig.cf_token, cf_account_id: cfConfig.cf_account_id })
      setZones(data.zones ?? [])
      if (data.zones?.length === 1) setZoneId(data.zones[0].id)
    } catch (e) {
      setZonesError(e instanceof Error ? e.message : '加载失败')
    } finally { setZonesLoading(false) }
  }

  // Lazy-load zones on first render
  useState(() => { void loadZones() })

  const handleNameChange = (v: string) => {
    setName(v)
    if (!workerNameEdited) setWorkerName(toWorkerName(v))
  }

  const selectedZone = zones.find(z => z.id === zoneId)
  const hostname = hostnamePrefix && selectedZone ? `${hostnamePrefix.trim()}.${selectedZone.name}` : ''

  const handleDeploy = async () => {
    if (!name.trim() || !workerName.trim() || !hostnamePrefix.trim() || !zoneId) {
      setError('请填写全部字段并选择 Zone'); return
    }
    setDeploying(true); setError('')
    try {
      const data = await createWorkerNode({
        name: name.trim(),
        worker_name: workerName.trim(),
        cf_token: cfConfig.cf_token,
        cf_account_id: cfConfig.cf_account_id,
        cf_zone_id: zoneId,
        hostname,
        expires_in_days: expiresInDays > 0 ? expiresInDays : undefined,
      })
      setResult({ node: data.node, yaml: data.clash_config })
      onCreated(data.node)
    } catch (e) {
      setError(e instanceof Error ? e.message : '部署失败')
    } finally { setDeploying(false) }
  }

  const handleCopy = async () => {
    if (!result) return
    await copyText(result.yaml)
    setCopied(true)
    setTimeout(() => setCopied(false), 1800)
  }

  const isDirty = !result && !!(name || hostnamePrefix || (workerNameEdited && workerName) || zoneId)
  const handleBeforeClose = () => {
    if (!isDirty) return true
    return window.confirm('确认放弃已输入的内容并关闭？')
  }

  return (
    <ModalShell
      title="新建 Worker 代理节点"
      description="在 Cloudflare Workers 上部署 VLESS-WS 代理，无需 VPS"
      icon={<CloudCog size={18} />}
      onClose={onClose}
      onBeforeClose={handleBeforeClose}
      size="md"
    >
      {result ? (
        // ── Success state ───────────────────────────────────────────────────
        <div className="space-y-4">
          <div className="flex items-center gap-3 rounded-xl border border-emerald-500/20 bg-emerald-500/[0.05] px-4 py-3">
            <CheckCircle2 size={16} className="text-emerald-400 shrink-0" />
            <div>
              <p className="text-sm font-semibold text-emerald-300">Worker 节点部署成功</p>
              <p className="text-xs text-emerald-300/60 mt-0.5">{result.node.worker_url || result.node.worker_dev_url}</p>
            </div>
          </div>

          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-white/35 mb-2">Clash 代理配置</p>
            <p className="text-[11px] text-white/40 mb-2 leading-relaxed">
              这是完整的 Clash YAML（含 <code className="text-brand-light">proxies:</code> 根字段），可直接导入或合并到现有配置。
            </p>
            <div className="relative">
              <pre className="glass-textarea h-auto whitespace-pre text-xs leading-5 p-3 overflow-x-auto font-mono">
                {result.yaml}
              </pre>
              <button
                className="absolute right-2 top-2 btn-ghost h-7 px-2 text-xs flex items-center gap-1.5"
                onClick={handleCopy}
              >
                {copied ? <Check size={12} className="text-emerald-400" /> : <Copy size={12} />}
                {copied ? '已复制' : '复制'}
              </button>
            </div>
          </div>

          {result.node.expires_at && (
            <div className="flex items-center gap-2 rounded-lg border border-sky-500/15 bg-sky-500/[0.04] px-3 py-2">
              <Clock size={12} className="text-sky-400 shrink-0" />
              <p className="text-[11px] text-sky-300/80">
                节点有效期至 <span className="font-semibold">{new Date(result.node.expires_at).toLocaleDateString('zh-CN')}</span>，到期后 Worker 将拒绝所有连接
              </p>
            </div>
          )}

          <p className="text-[11px] text-amber-300/70 leading-relaxed rounded-lg border border-amber-500/15 bg-amber-500/[0.04] px-3 py-2">
            UUID 已写入 Worker 环境变量。请妥善保存以上配置，UUID 不会再次显示（可通过"重新部署"刷新）。
          </p>

          <div className="flex justify-end pt-1">
            <button className="btn-primary" onClick={onClose}>完成</button>
          </div>
        </div>
      ) : (
        // ── Form state ──────────────────────────────────────────────────────
        <div className="space-y-4">
          <div className="rounded-xl border border-white/10 bg-black/20 p-3 text-xs text-slate-300 space-y-1.5">
            <p className="font-semibold text-slate-200 mb-1">工作原理</p>
            <p className="text-white/45 leading-relaxed">
              ClashForge 将在你的 CF 账号下部署一个 VLESS-over-WebSocket Worker，绑定到你的自定义域名。
              Clash 客户端通过 TLS + WebSocket 连接该 Worker，Worker 使用 CF 的 TCP Sockets API 将流量转发到目标服务器。
            </p>
          </div>

          <div className="space-y-3">
            <div>
              <label className="block text-xs text-slate-400 mb-1">节点名称 <span className="text-red-400">*</span></label>
              <input
                className="glass-input"
                value={name}
                onChange={e => handleNameChange(e.target.value)}
                placeholder="例如：Tokyo Edge"
                autoFocus
              />
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">
                Worker 脚本名称 <span className="text-red-400">*</span>
                <span className="text-muted ml-1">（全 CF 账号唯一，只含小写字母、数字、连字符）</span>
              </label>
              <input
                className="glass-input font-mono text-xs"
                value={workerName}
                onChange={e => { setWorkerName(e.target.value); setWorkerNameEdited(true) }}
                placeholder="cf-proxy-tokyo"
              />
            </div>

            <div>
              <div className="flex items-center justify-between mb-1">
                <label className="text-xs text-slate-400">Zone <span className="text-red-400">*</span></label>
                <button className="text-[11px] text-brand-light hover:underline" onClick={loadZones} disabled={zonesLoading}>
                  {zonesLoading ? '加载中…' : '刷新'}
                </button>
              </div>
              {zonesError && <p className="text-xs text-red-400 mb-1">{zonesError}</p>}
              <select
                className="glass-input theme-select"
                value={zoneId}
                onChange={e => { setZoneId(e.target.value); if (!hostnamePrefix) setHostnamePrefix('proxy') }}
                disabled={zonesLoading}
              >
                <option value="">— 选择域名 Zone —</option>
                {zones.map(z => <option key={z.id} value={z.id}>{z.name}</option>)}
              </select>
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">
                绑定域名（Hostname）<span className="text-red-400">*</span>
              </label>
              <div className="flex items-stretch">
                <input
                  className="glass-input font-mono text-xs rounded-r-none flex-1 min-w-0"
                  value={hostnamePrefix}
                  onChange={e => setHostnamePrefix(e.target.value)}
                  placeholder="proxy"
                  disabled={!zoneId}
                />
                <span className="flex items-center px-3 font-mono text-xs text-white/40 bg-white/[0.03] border border-l-0 border-white/10 rounded-r-xl whitespace-nowrap select-none">
                  .{selectedZone?.name ?? 'yourdomain.com'}
                </span>
              </div>
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1 flex items-center gap-1.5">
                <Clock size={11} className="text-sky-400" />
                节点有效期
              </label>
              <select
                className="glass-input theme-select"
                value={expiresInDays}
                onChange={e => setExpiresInDays(Number(e.target.value))}
              >
                {EXPIRY_OPTIONS.map(o => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </select>
              {expiresInDays > 0 && (
                <p className="text-[11px] text-white/30 mt-1">
                  到期时间：{new Date(Date.now() + expiresInDays * 86400_000).toLocaleDateString('zh-CN')}
                </p>
              )}
            </div>
          </div>

          <div className="rounded-xl border border-white/[0.07] bg-white/[0.02] px-3 py-2.5 text-xs space-y-1">
            <p className="font-semibold text-white/50 uppercase tracking-[0.15em] text-[10px] mb-1.5">所需 CF Token 权限</p>
            {[
              ['Zone DNS Edit', 'Worker 域名绑定'],
              ['Workers Scripts Edit', '上传 VLESS Worker 脚本'],
              ['Zone Read', '查询 Zone ID'],
            ].map(([perm, use]) => (
              <div key={perm} className="flex items-center gap-2">
                <CheckCircle2 size={10} className="text-brand-light shrink-0" />
                <span className="text-slate-300 font-medium">{perm}</span>
                <span className="text-white/30">— {use}</span>
              </div>
            ))}
          </div>

          {error && (
            <div className="flex items-center gap-2 rounded-xl border border-red-500/20 bg-red-500/5 px-3 py-2 text-xs text-red-400">
              <AlertCircle size={12} className="shrink-0" />{error}
            </div>
          )}

          <div className="flex items-center justify-end gap-3 pt-1">
            <button className="btn-ghost" onClick={onClose} disabled={deploying}>取消</button>
            <button
              className="btn-primary"
              onClick={handleDeploy}
              disabled={deploying || !name.trim() || !workerName.trim() || !hostnamePrefix.trim() || !zoneId}
            >
              {deploying ? <Loader2 size={14} className="animate-spin" /> : <CloudCog size={14} />}
              {deploying ? '部署中…' : '一键部署'}
            </button>
          </div>
        </div>
      )}
    </ModalShell>
  )
}

// ── WorkerNodeCard (list item) ─────────────────────────────────────────────────

export function WorkerNodeCard({
  node,
  onDelete,
  onRedeploy,
  onExport,
  onRenew,
  onFreeTierInfo,
}: {
  node: WorkerNodeListItem
  onDelete: (id: string) => void
  onRedeploy: (id: string) => void
  onExport: (id: string) => void
  onRenew?: (id: string) => void
  onFreeTierInfo?: (id: string) => void
}) {
  const statusColor = node.status === 'deployed'
    ? 'bg-emerald-500/10 text-emerald-400 ring-emerald-500/20'
    : node.status === 'error'
    ? 'bg-red-500/10 text-red-400 ring-red-500/20'
    : 'bg-amber-500/10 text-amber-400 ring-amber-500/20'
  const statusLabel = node.status === 'deployed' ? '已部署' : node.status === 'error' ? '错误' : '待部署'

  const expiryInfo = (() => {
    if (!node.expires_at) return null
    const exp = new Date(node.expires_at)
    const daysLeft = Math.ceil((exp.getTime() - Date.now()) / 86400_000)
    const expired = daysLeft <= 0
    const soon = !expired && daysLeft <= 14
    return { label: exp.toLocaleDateString('zh-CN'), daysLeft, expired, soon }
  })()

  return (
    <div className="grid grid-cols-12 gap-3 px-4 py-3.5 table-row items-center">
      <div className="col-span-3">
        <p className="text-sm font-semibold text-white truncate">{node.name}</p>
        <p className="text-[11px] text-muted mt-0.5 truncate font-mono">{node.worker_name}</p>
      </div>
      <div className="col-span-3 min-w-0">
        <p className="text-xs text-slate-300 truncate">{node.hostname || '—'}</p>
        <p className="text-[10px] text-muted truncate">{node.worker_dev_url || '—'}</p>
      </div>
      <div className="col-span-2">
        <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ring-1 ring-inset ${statusColor}`}>
          <span className="h-1.5 w-1.5 rounded-full bg-current" />
          {statusLabel}
        </span>
        {node.error && <p className="text-[10px] text-red-400/70 mt-1 truncate max-w-[120px]" title={node.error}>{node.error}</p>}
        {expiryInfo && (
          <p className={`text-[10px] mt-1 flex items-center gap-1 ${expiryInfo.expired ? 'text-red-400' : expiryInfo.soon ? 'text-amber-400' : 'text-white/30'}`}>
            <Clock size={9} />
            {expiryInfo.expired
              ? '已过期'
              : expiryInfo.soon
              ? `${expiryInfo.daysLeft} 天后过期`
              : `至 ${expiryInfo.label}`}
          </p>
        )}
      </div>
      <div className="col-span-4 flex items-center justify-end gap-2">
        <button className="btn-icon-sm btn-ghost" title="导出 Clash 配置" onClick={() => onExport(node.id)}>
          <Download size={14} className="text-emerald-400" />
        </button>
        {onFreeTierInfo && (
          <button className="btn-icon-sm btn-ghost" title="CI / GitHub Secret 配置" onClick={() => onFreeTierInfo(node.id)}>
            <Key size={14} className="text-violet-400" />
          </button>
        )}
        {onRenew && (
          <button className="btn-icon-sm btn-ghost" title="续期" onClick={() => onRenew(node.id)}>
            <Clock size={14} className="text-sky-400" />
          </button>
        )}
        <button className="btn-icon-sm btn-ghost" title="重新部署" onClick={() => onRedeploy(node.id)}>
          <RotateCw size={14} className="text-amber-400" />
        </button>
        <button className="btn-icon-sm btn-ghost" title="删除节点" onClick={() => onDelete(node.id)}>
          <Trash2 size={14} className="text-muted hover:text-red-400" />
        </button>
      </div>
    </div>
  )
}

// ── FreeTierInfoModal ─────────────────────────────────────────────────────────

const GITHUB_SECRETS: { key: keyof WorkerNodeFreeTierInfo; secretName: string; label: string; sensitive: boolean }[] = [
  { key: 'sub_url',    secretName: 'FREE_NODE_URL',     label: '订阅 URL',  sensitive: false },
  { key: 'aes_key',    secretName: 'FREE_NODE_AES_KEY', label: 'AES 密钥',  sensitive: true  },
  { key: 'expires_at', secretName: '',                   label: '过期时间',  sensitive: false },
]

export function FreeTierInfoModal({
  nodeId,
  onClose,
}: {
  nodeId: string
  onClose: () => void
}) {
  const [info, setInfo]       = useState<WorkerNodeFreeTierInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError]     = useState('')
  const [copied, setCopied]   = useState<string | null>(null)

  useEffect(() => {
    getWorkerNodeFreeTierInfo(nodeId)
      .then(setInfo)
      .catch(e => setError(e instanceof Error ? e.message : '获取失败'))
      .finally(() => setLoading(false))
  }, [nodeId])

  const handleCopy = async (value: string, id: string) => {
    await copyText(value)
    setCopied(id)
    setTimeout(() => setCopied(null), 1800)
  }

  return (
    <ModalShell
      title="CI / GitHub Secret 配置"
      description="将以下值填入仓库 Secrets，GitHub Actions 打包时会自动将免费节点内置到 APK"
      icon={<Key size={18} />}
      onClose={onClose}
      size="md"
    >
      {loading ? (
        <div className="flex items-center justify-center py-10">
          <Loader2 size={20} className="animate-spin text-brand-light" />
        </div>
      ) : error ? (
        <div className="flex items-center gap-2 rounded-xl border border-red-500/20 bg-red-500/5 px-3 py-3 text-xs text-red-400">
          <AlertCircle size={13} className="shrink-0" />{error}
        </div>
      ) : info ? (
        <div className="space-y-4">
          {/* Secret 列表 */}
          <div className="space-y-2">
            {GITHUB_SECRETS.map(({ key, secretName, label, sensitive }) => {
              const value = info[key] || '—'
              const isEmpty = !info[key]
              return (
                <div key={key} className="rounded-xl border border-white/[0.08] bg-black/20 p-3 space-y-1.5">
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-slate-400">{label}</span>
                    {secretName && (
                      <span className="text-[10px] font-mono text-violet-400/80 bg-violet-500/10 px-2 py-0.5 rounded-full">
                        {secretName}
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <code className={`flex-1 text-[11px] font-mono break-all select-all rounded px-2 py-1.5 bg-black/30 border border-white/5 ${sensitive ? 'text-amber-200' : 'text-slate-200'} ${isEmpty ? 'text-white/25 italic' : ''}`}>
                      {sensitive && !isEmpty ? '•'.repeat(Math.min(value.length, 32)) + (value.length > 32 ? '…' : '') : value}
                    </code>
                    {!isEmpty && (
                      <button
                        className="btn-icon-sm btn-ghost shrink-0"
                        title="复制"
                        onClick={() => handleCopy(value, key)}
                      >
                        {copied === key
                          ? <Check size={13} className="text-emerald-400" />
                          : <Copy size={13} />}
                      </button>
                    )}
                  </div>
                </div>
              )
            })}
          </div>

          {/* 操作说明 */}
          <div className="rounded-xl border border-white/[0.07] bg-white/[0.02] px-3 py-3 text-xs space-y-2 text-white/50">
            <p className="font-semibold text-white/70 uppercase tracking-[0.15em] text-[10px]">配置步骤</p>
            <ol className="space-y-1.5 list-decimal list-inside leading-relaxed">
              <li>进入 GitHub 仓库 → <span className="text-white/70">Settings → Secrets and variables → Actions</span></li>
              <li>依次添加上方两个 Secret（名称须与紫色标签完全一致）</li>
              <li>下次触发 <span className="font-mono text-brand-light">android-release</span> workflow 时，免费节点将自动内置到 APK</li>
              <li>节点到期前，点击卡片的 <span className="inline-flex items-center gap-1"><Clock size={10} className="text-sky-400" />续期</span> 按钮更新 CF Worker，重新打包 APK 即可</li>
            </ol>
          </div>

          <div className="flex justify-end">
            <button className="btn-primary" onClick={onClose}>关闭</button>
          </div>
        </div>
      ) : null}
    </ModalShell>
  )
}
