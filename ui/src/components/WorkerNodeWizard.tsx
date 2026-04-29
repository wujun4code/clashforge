import { useState } from 'react'
import {
  AlertCircle,
  CheckCircle2,
  CloudCog,
  Copy,
  Download,
  Check,
  Loader2,
  RotateCw,
  Trash2,
} from 'lucide-react'
import { ModalShell } from './ui'
import {
  createWorkerNode,
  getCloudflareZones,
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
}: {
  node: WorkerNodeListItem
  onDelete: (id: string) => void
  onRedeploy: (id: string) => void
  onExport: (id: string) => void
}) {
  const statusColor = node.status === 'deployed'
    ? 'bg-emerald-500/10 text-emerald-400 ring-emerald-500/20'
    : node.status === 'error'
    ? 'bg-red-500/10 text-red-400 ring-red-500/20'
    : 'bg-amber-500/10 text-amber-400 ring-amber-500/20'
  const statusLabel = node.status === 'deployed' ? '已部署' : node.status === 'error' ? '错误' : '待部署'

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
      </div>
      <div className="col-span-4 flex items-center justify-end gap-2">
        <button className="btn-icon-sm btn-ghost" title="导出 Clash 配置" onClick={() => onExport(node.id)}>
          <Download size={14} className="text-emerald-400" />
        </button>
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
