import { useCallback, useEffect, useRef, useState, type CSSProperties } from 'react'
import {
  Server,
  Plus,
  CheckCircle2,
  AlertCircle,
  CloudCog,
  Loader2,
  Trash2,
  Pencil,
  Play,
  Download,
  ShieldOff,
  X,
  Terminal,
  RotateCw,
  Copy,
  Check,
  Upload,
  Cloud,
} from 'lucide-react'
import {
  getNodes,
  getNodeSSHPubKey,
  createNode,
  updateNode,
  deleteNode,
  testNodeConnection,
  getCloudflareZones,
  probeNode,
  getWorkerNodes,
  deleteWorkerNode,
  redeployWorkerNode,
  getWorkerNodeClashConfig,
  importSubscription,
  getNodeImports,
  deleteSubscription,
  getSubscriptionNodes,
  getAzureLocations,
  getAzureResourceGroups,
  getAzureVMSizes,
  validateAzureCredentials,
} from '../api/client'
import type { NodeListItem, NodeCreateRequest, NodeProbeResult, CloudflareZone, WorkerNodeListItem, Subscription, AzureLocation, AzureResourceGroup, AzureVMSize } from '../api/client'
import { PageHeader, SectionCard, ModalShell, EmptyState } from '../components/ui'
import {
  CFGate,
  CFPermissionTable,
  type CFConfig,
  CFConfigModal,
  maskSecret,
  useCFConfig,
} from '../components/CFConfig'
import {
  AzureConfigModal,
  AzureConfigBanner,
  useAzureConfig,
  type AzureConfig,
} from '../components/AzureConfig'
import { WorkerNodeWizard, WorkerNodeCard } from '../components/WorkerNodeWizard'

const BASE = '/api/v1'

// ── Status helpers ──────────────────────────────────────────────────────────

type NodeStatus = NodeListItem['status']

const STATUS_CONFIG: Record<NodeStatus, { label: string; color: string; dot: string }> = {
  pending:   { label: '待连接',   color: 'bg-surface-3 text-muted',              dot: 'bg-muted' },
  connected: { label: '已连接',   color: 'bg-sky-500/10 text-sky-400',           dot: 'bg-sky-400' },
  deploying: { label: '部署中',   color: 'bg-amber-500/10 text-amber-400',        dot: 'bg-amber-400 animate-pulse' },
  deployed:  { label: '已部署',   color: 'bg-emerald-500/10 text-emerald-400',    dot: 'bg-emerald-400' },
  error:     { label: '错误',     color: 'bg-red-500/10 text-red-400',            dot: 'bg-red-400' },
}

function StatusBadge({ status }: { status: NodeStatus }) {
  const c = STATUS_CONFIG[status] ?? STATUS_CONFIG.pending
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${c.color}`}>
      <span className={`inline-flex h-1.5 w-1.5 rounded-full ${c.dot}`} />
      {c.label}
    </span>
  )
}

function suggestSubdomains(zone: string) {
  const prefixes = ['market', 'sales', 'trials', 'blog', 'cdn', 'edge']
  return prefixes.map(p => `${p}-${Math.floor(Math.random() * 90 + 10)}.${zone}`)
}


// ── SSE helpers ──────────────────────────────────────────────────────────────

interface SSEEvent {
  step: string
  status: string
  message: string
  detail: string
}

interface SSEDone {
  type: 'done'
  success: boolean
  error?: string
  deploy_log?: string
  phase?: 'bootstrap' | 'full'
  cert_issued?: boolean
  probe_results?: NodeProbeResult[]
}

async function streamSSE(
  url: string,
  secret: string,
  onEvent: (ev: SSEEvent) => void,
  onDone: (done: SSEDone) => void,
  signal?: AbortSignal,
  payload?: unknown,
) {
  const resp = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${secret}`,
    },
    body: payload ? JSON.stringify(payload) : undefined,
    signal,
  })
  if (!resp.ok) {
    let detail = `请求失败 (${resp.status})`
    try {
      const contentType = resp.headers.get('content-type') ?? ''
      if (contentType.includes('application/json')) {
        const body = await resp.json()
        detail = body?.error?.message ?? body?.message ?? detail
      } else {
        const text = (await resp.text()).trim()
        if (text) detail = text
      }
    } catch {
      // ignore parse failures
    }
    throw new Error(detail)
  }
  const reader = resp.body?.getReader()
  if (!reader) return
  const decoder = new TextDecoder()
  let buffer = ''
  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop() ?? ''
    for (const line of lines) {
      if (!line.startsWith('data: ')) continue
      try {
        const data = JSON.parse(line.slice(6))
        if (data.type === 'done') {
          onDone(data as SSEDone)
        } else {
          onEvent(data as SSEEvent)
        }
      } catch { /* skip malformed */ }
    }
  }
}

const FULL_DEPLOY_STEPS: Array<{ id: string; label: string; optional?: boolean }> = [
  { id: 'connect', label: '连接节点' },
  { id: 'prereqs', label: '检查环境' },
  { id: 'gost-check', label: '检测 GOST' },
  { id: 'cf-zone', label: '识别 Zone', optional: true },
  { id: 'dns-bind', label: '绑定 DNS' },
  { id: 'acme-ready', label: '准备 ACME' },
  { id: 'cert-issue', label: '签发证书' },
  { id: 'cert-install', label: '部署证书' },
  { id: 'config-write', label: '更新配置' },
  { id: 'systemd-restart', label: '重启服务' },
  { id: 'probe-domain', label: '连通验证' },
]

const FIREWORK_PARTICLES = [
  { left: 12, color: '#f472b6', delay: 0, tx: -70, ty: -105 },
  { left: 18, color: '#fb7185', delay: 40, tx: -20, ty: -128 },
  { left: 24, color: '#f59e0b', delay: 90, tx: 32, ty: -112 },
  { left: 31, color: '#22d3ee', delay: 20, tx: -40, ty: -95 },
  { left: 38, color: '#34d399', delay: 120, tx: 24, ty: -122 },
  { left: 44, color: '#a78bfa', delay: 80, tx: 68, ty: -96 },
  { left: 53, color: '#38bdf8', delay: 60, tx: -66, ty: -118 },
  { left: 59, color: '#f43f5e', delay: 30, tx: -14, ty: -132 },
  { left: 66, color: '#fbbf24', delay: 140, tx: 40, ty: -109 },
  { left: 73, color: '#4ade80', delay: 70, tx: 84, ty: -103 },
  { left: 81, color: '#60a5fa', delay: 110, tx: -34, ty: -125 },
  { left: 88, color: '#e879f9', delay: 160, tx: 16, ty: -100 },
]

function FullDeployProgress({ evs }: { evs: SSEEvent[] }) {
  const statusByStep = new Map<string, string>()
  const eventByStep = new Map<string, SSEEvent>()
  for (const ev of evs) {
    statusByStep.set(ev.step, ev.status)
    eventByStep.set(ev.step, ev)
  }
  const activeSteps = FULL_DEPLOY_STEPS.filter(({ id, optional }) => !optional || eventByStep.has(id))
  const doneCount = activeSteps.filter(({ id }) => {
    const s = statusByStep.get(id)
    return s === 'ok' || s === 'warning'
  }).length
  const pct = activeSteps.length > 0 ? Math.round((doneCount / activeSteps.length) * 100) : 0

  const statusIcon = (status?: string) => {
    if (status === 'ok') return <CheckCircle2 size={12} className="text-emerald-400 shrink-0" />
    if (status === 'warning') return <AlertCircle size={12} className="text-amber-400 shrink-0" />
    if (status === 'error') return <X size={12} className="text-red-400 shrink-0" />
    if (status === 'running') return <Loader2 size={12} className="text-brand shrink-0 animate-spin" />
    return <span className="inline-block h-2 w-2 rounded-full bg-white/20 shrink-0" />
  }

  return (
    <div className="rounded-xl border border-white/10 bg-black/20 p-3 space-y-3">
      <div className="space-y-1.5">
        <div className="flex items-center justify-between text-[11px]">
          <span className="text-slate-300">部署进度</span>
          <span className="text-slate-400 font-mono">{doneCount}/{activeSteps.length} · {pct}%</span>
        </div>
        <div className="h-1.5 rounded-full bg-white/10 overflow-hidden">
          <div className="h-full rounded-full bg-gradient-to-r from-cyan-400 to-violet-400 transition-all duration-300" style={{ width: `${pct}%` }} />
        </div>
      </div>
      <div className="grid gap-1.5">
        {FULL_DEPLOY_STEPS.map(({ id, label }) => {
          const ev = eventByStep.get(id)
          const status = statusByStep.get(id)
          return (
            <div key={id} className="rounded-lg border border-white/8 bg-white/[0.03] px-2.5 py-2">
              <div className="flex items-center gap-2 text-[11px]">
                {statusIcon(status)}
                <span className={status === 'error' ? 'text-red-300' : status === 'ok' ? 'text-emerald-300' : status === 'running' ? 'text-brand-light' : 'text-slate-300'}>
                  {label}
                </span>
              </div>
              {ev && (
                <p className="mt-1 pl-5 text-[10px] text-muted break-all">
                  {ev.message}{ev.detail ? ` · ${ev.detail}` : ''}
                </p>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function CelebrationFireworks({ show }: { show: boolean }) {
  if (!show) return null
  return (
    <div className="wizard-fireworks pointer-events-none">
      {FIREWORK_PARTICLES.map((p, idx) => (
        <span
          key={`${p.left}-${idx}`}
          className="wizard-firework-particle"
          style={
            {
              left: `${p.left}%`,
              color: p.color,
              background: p.color,
              animationDelay: `${p.delay}ms`,
              ['--fw-x' as string]: `${p.tx}px`,
              ['--fw-y' as string]: `${p.ty}px`,
            } as CSSProperties
          }
        />
      ))}
    </div>
  )
}

// ── Node Edit Modal (edit-only, existing nodes) ─────────────────────────────

function NodeEditModal({
  node,
  onClose,
  onSaved,
}: {
  node: NodeListItem
  onClose: () => void
  onSaved: () => void
}) {
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [form, setForm] = useState<NodeCreateRequest>({
    name: node.name,
    host: node.host,
    port: node.port,
    username: node.username,
    password: '',
    domain: node.domain ?? '',
    email: '',
    cf_token: '',
    cf_account_id: '',
    cf_zone_id: '',
  })

  const update = <K extends keyof NodeCreateRequest>(k: K, v: NodeCreateRequest[K]) =>
    setForm(f => ({ ...f, [k]: v }))

  const handleSave = async () => {
    setSaving(true)
    setError('')
    try {
      await updateNode(node.id, form)
      onSaved()
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <ModalShell title="编辑节点" onClose={onClose} size="lg" dismissible={false} icon={<Server size={18} />}>
      <div className="space-y-4 max-h-[70vh] overflow-y-auto pr-1">
        <fieldset className="space-y-3">
          <legend className="text-xs font-semibold uppercase tracking-[0.2em] text-muted mb-2">服务器信息</legend>
          <div className="grid grid-cols-3 gap-3">
            <div className="col-span-2">
              <label className="block text-xs text-slate-400 mb-1">名称</label>
              <input className="glass-input" value={form.name} onChange={e => update('name', e.target.value)} placeholder="如: 东京-AWS" />
            </div>
            <div>
              <label className="block text-xs text-slate-400 mb-1">端口</label>
              <input className="glass-input" type="number" value={form.port} onChange={e => update('port', Number(e.target.value))} />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-slate-400 mb-1">主机地址 (IP)</label>
              <input className="glass-input font-mono" value={form.host} onChange={e => update('host', e.target.value)} placeholder="1.2.3.4" />
            </div>
            <div>
              <label className="block text-xs text-slate-400 mb-1">用户名</label>
              <input className="glass-input" value={form.username} onChange={e => update('username', e.target.value)} />
            </div>
          </div>
          <div>
            <label className="block text-xs text-slate-400 mb-1">新密码 (留空不修改)</label>
            <input className="glass-input" type="password" value={form.password} onChange={e => update('password', e.target.value)} placeholder="留空则不修改" />
          </div>
        </fieldset>
        <div className="divider" />
        <fieldset className="space-y-3">
          <legend className="text-xs font-semibold uppercase tracking-[0.2em] text-muted mb-2">域名 & TLS</legend>
          <div>
            <label className="block text-xs text-slate-400 mb-1">域名</label>
            <input className="glass-input font-mono" value={form.domain} onChange={e => update('domain', e.target.value)} placeholder="vps.example.com" />
          </div>
          <div>
            <label className="block text-xs text-slate-400 mb-1">联系邮箱 (Let's Encrypt)</label>
            <input className="glass-input" type="email" value={form.email} onChange={e => update('email', e.target.value)} placeholder="admin@example.com" />
          </div>
        </fieldset>
        {error && (
          <div className="rounded-xl border border-red-500/20 bg-red-500/5 px-4 py-3 text-sm text-red-400">
            <AlertCircle size={14} className="inline mr-1.5 -mt-0.5" />{error}
          </div>
        )}
        <div className="flex items-center justify-end gap-3 pt-2">
          <button className="btn-ghost" onClick={onClose} disabled={saving}>取消</button>
          <button className="btn-primary" onClick={handleSave} disabled={saving || !form.name || !form.host}>
            {saving ? <Loader2 size={14} className="animate-spin" /> : <CheckCircle2 size={14} />} 保存修改
          </button>
        </div>
      </div>
    </ModalShell>
  )
}

// ── Node Wizard (新增节点 + 分步部署，一体化) ──────────────────────────────

const TOTAL_STEPS = 7

function StepIndicator({ current, total }: { current: number; total: number }) {
  return (
    <div className="flex items-center gap-1.5 mb-4">
      {Array.from({ length: total }, (_, i) => i + 1).map(s => (
        <div key={s} className={`h-1 flex-1 rounded-full transition-colors ${s < current ? 'bg-brand/60' : s === current ? 'bg-brand' : 'bg-white/10'}`} />
      ))}
      <span className="text-[10px] text-muted ml-1 shrink-0">{current}/{total}</span>
    </div>
  )
}

async function copyText(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    try { await navigator.clipboard.writeText(text); return } catch { /* fall through */ }
  }
  // Fallback for HTTP (non-secure) contexts
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.cssText = 'position:fixed;opacity:0;top:0;left:0'
  document.body.appendChild(ta)
  ta.focus(); ta.select()
  document.execCommand('copy')
  document.body.removeChild(ta)
}

function CopyableCode({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="flex items-start gap-2">
      <code className="flex-1 block rounded-lg bg-surface-2 border border-white/5 px-3 py-2 font-mono text-[11px] text-slate-300 break-all leading-relaxed">{text}</code>
      <button type="button" className="btn-ghost shrink-0 px-2 py-2" onClick={async () => {
        await copyText(text); setCopied(true); setTimeout(() => setCopied(false), 2000)
      }}>
        {copied ? <Check size={13} className="text-emerald-400" /> : <Copy size={13} />}
      </button>
    </div>
  )
}

function ProbeResultList({ results }: { results: NodeProbeResult[] }) {
  return (
    <div className="rounded-xl border border-white/10 bg-black/10 p-3 space-y-1.5">
      {results.map((p, i) => (
        <div key={i} className="flex items-center gap-2 text-xs">
          {p.ok
            ? <CheckCircle2 size={11} className="text-emerald-400 shrink-0" />
            : <AlertCircle size={11} className="text-red-400 shrink-0" />}
          <span className="text-slate-300 font-medium">{p.name}</span>
          {p.ok
            ? <span className="text-emerald-400/80 font-mono">{p.status_code} · {p.latency_ms}ms</span>
            : <span className="text-red-400/80 truncate">{p.error}</span>}
        </div>
      ))}
    </div>
  )
}

function summarizeProbe(results: NodeProbeResult[]) {
  const ok = results.filter(item => item.ok).length
  const total = results.length
  return { ok, total, success: total > 0 && ok === total }
}

function stringifyEventLog(evs: SSEEvent[]) {
  return evs.map((ev) => {
    const prefix = `[${ev.status}] ${ev.step}: ${ev.message}`
    return ev.detail ? `${prefix}\n  ${ev.detail}` : prefix
  }).join('\n')
}

function isNodeFullyDeployed(node: NodeListItem): boolean {
  return node.status === 'deployed' && node.domain.trim() !== ''
}

function resumeStep(node: NodeListItem): number {
  const hasBootstrapProgress = Boolean(node.deployed_at)
  const hasDomain = node.domain.trim() !== ''

  switch (node.status) {
    case 'connected':
      if (hasBootstrapProgress && hasDomain) return 6
      if (hasBootstrapProgress && !hasDomain) return 4
      return 3
    case 'deploying':
    case 'error':
      if (hasBootstrapProgress && hasDomain) return 6
      if (hasBootstrapProgress && !hasDomain) return 4
      return 3
    case 'deployed':
      return hasDomain ? 7 : 4
    default:
      return 2
  }
}

function NodeWizard({
  initialNode,
  cfConfig,
  onSaveCF,
  onClose,
  onDone,
  onOpenExport,
}: {
  initialNode?: NodeListItem
  cfConfig: CFConfig | null
  onSaveCF: (cfg: CFConfig) => Promise<void>
  onClose: () => void
  onDone: () => void
  onOpenExport: (node: NodeListItem) => void
}) {
  const [createdNode, setCreatedNode] = useState<NodeListItem | null>(initialNode ?? null)
  const [step, setStep] = useState(() => initialNode ? resumeStep(initialNode) : 1)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<{ text: string; ok?: boolean } | null>(null)
  const eventsRef = useRef<HTMLDivElement>(null)

  const minStep = initialNode ? 2 : 1
  const canGoBack = step > minStep && !busy
  const goBack = () => { setStep(s => s - 1); setMessage(null) }

  // Step 1: server info
  const [form, setForm] = useState<NodeCreateRequest>({
    name: initialNode?.name ?? '', host: initialNode?.host ?? '',
    port: initialNode?.port ?? 22, username: initialNode?.username ?? 'root',
    password: '', domain: initialNode?.domain ?? '',
    email: '', cf_token: '', cf_account_id: '', cf_zone_id: '',
  })
  const updateForm = <K extends keyof NodeCreateRequest>(k: K, v: NodeCreateRequest[K]) =>
    setForm(f => ({ ...f, [k]: v }))

  // Step 2: SSH pubkey
  const [sshPubKey, setSSHPubKey] = useState('')
  useEffect(() => { getNodeSSHPubKey().then(r => setSSHPubKey(r.public_key)).catch(() => {}) }, [])
  const sshHost = createdNode?.host ?? form.host
  const sshPort = createdNode?.port ?? form.port
  const sshUser = createdNode?.username ?? form.username
  const authorizeCmd = sshPubKey && sshHost && sshUser
    ? `ssh -p ${sshPort} ${sshUser}@${sshHost} "echo '${sshPubKey}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys"`
    : ''
  const localCmd = sshPubKey
    ? `mkdir -p ~/.ssh && echo '${sshPubKey}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && chmod 700 ~/.ssh`
    : ''

  // Step 3: bootstrap deploy
  const [events, setEvents] = useState<SSEEvent[]>([])
  const [bootstrapProbe, setBootstrapProbe] = useState<NodeProbeResult[]>([])
  const [bootstrapProbeSummary, setBootstrapProbeSummary] = useState<{ ok: number; total: number; success: boolean } | null>(null)

  // Step 4: CF credentials — pre-filled from global CF config
  const [cfToken, setCFToken] = useState(cfConfig?.cf_token ?? '')
  const [cfAccountId, setCFAccountId] = useState(cfConfig?.cf_account_id ?? '')

  // Step 5: domain selection
  const [zones, setZones] = useState<CloudflareZone[]>([])
  const [cfZoneId, setCFZoneId] = useState('')
  const [domain, setDomain] = useState(initialNode?.domain ?? '')
  const [email, setEmail] = useState(cfConfig?.acme_email ?? '')
  const [domainSuggestions, setDomainSuggestions] = useState<string[]>([])

  // Step 6: full deploy
  const [fullEvents, setFullEvents] = useState<SSEEvent[]>([])
  const [showCelebration, setShowCelebration] = useState(false)
  const [fullDeployCertIssued, setFullDeployCertIssued] = useState<boolean | null>(null)
  const [fullDeployLog, setFullDeployLog] = useState('')
  const [fullDeployLogCopied, setFullDeployLogCopied] = useState(false)

  // Step 7: domain probe
  const [domainProbe, setDomainProbe] = useState<NodeProbeResult[]>([])

  // Sync if cfConfig arrives after first render (async hook load)
  useEffect(() => {
    if (!cfConfig) return
    setCFToken(t => t || cfConfig.cf_token)
    setCFAccountId(id => id || cfConfig.cf_account_id)
    setEmail(e => e || cfConfig.acme_email)
  }, [cfConfig?.cf_token]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    eventsRef.current?.scrollTo({ top: eventsRef.current.scrollHeight, behavior: 'smooth' })
  }, [events, fullEvents])

  useEffect(() => {
    if (!showCelebration) return
    const timer = window.setTimeout(() => setShowCelebration(false), 2200)
    return () => window.clearTimeout(timer)
  }, [showCelebration])

  const node = createdNode
  const bootstrapCanContinue = (bootstrapProbeSummary?.success ?? false)

  // ── Step actions ──

  const handleCreateNode = async () => {
    setBusy(true); setMessage(null)
    try {
      const data = await createNode(form)
      setCreatedNode(data.node)
      onDone() // refresh list
      setStep(2)
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : '创建失败', ok: false })
    } finally { setBusy(false) }
  }

  const handleTestSSH = async () => {
    if (!node) return
    setBusy(true); setMessage(null)
    try {
      const r = await testNodeConnection(node.id)
      setMessage({ text: r.ok ? 'SSH 连接校验成功' : r.message, ok: r.ok })
      if (r.ok) setStep(3)
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : 'SSH 校验失败', ok: false })
    } finally { setBusy(false) }
  }

  const handleBootstrapDeploy = async () => {
    if (!node) return
    setBusy(true); setEvents([]); setBootstrapProbe([]); setBootstrapProbeSummary(null); setMessage(null)
    const secret = localStorage.getItem('cf_secret') || ''
    try {
      await streamSSE(
        `${BASE}/nodes/${encodeURIComponent(node.id)}/deploy`, secret,
        ev => setEvents(prev => [...prev, ev]),
        d => {
          if (!d.success) { setMessage({ text: d.error || '部署失败', ok: false }); return }
          if (d.probe_results && d.probe_results.length > 0) {
            setBootstrapProbe(d.probe_results)
            const summary = summarizeProbe(d.probe_results)
            setBootstrapProbeSummary(summary)
            setMessage({
              text: summary.success
                ? `GOST 部署完成，IP 连通探测 ${summary.ok}/${summary.total} 通过，可继续下一步`
                : `GOST 部署完成，但 IP 连通探测 ${summary.ok}/${summary.total} 通过，请手动测试后再继续`,
              ok: summary.success,
            })
          } else {
            setMessage({ text: 'GOST 部署完成，请点击“测试 GOST 连通”后继续下一步', ok: true })
          }
          onDone()
        },
        undefined,
        { mode: 'bootstrap' },
      )
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : '部署失败', ok: false })
    } finally { setBusy(false) }
  }

  const handleBootstrapProbe = async () => {
    if (!node) return
    setBusy(true); setMessage(null)
    try {
      const data = await probeNode(node.id, 'ip')
      setBootstrapProbe(data.probe_results)
      setBootstrapProbeSummary(data.summary)
      setMessage({
        text: data.summary.success
          ? `GOST 连通测试通过（${data.summary.ok}/${data.summary.total}）`
          : `GOST 连通测试未通过（${data.summary.ok}/${data.summary.total}），请检查节点网络后重试`,
        ok: data.summary.success,
      })
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : '测试失败', ok: false })
    } finally { setBusy(false) }
  }

  const handleSaveCF = async () => {
    setBusy(true); setMessage(null)
    try {
      const token = cfToken.trim()
      const account = cfAccountId.trim()
      const mail = email.trim()
      await onSaveCF({ cf_token: token, cf_account_id: account, acme_email: mail })
      setCFToken(token)
      setCFAccountId(account)
      setEmail(mail)
      setMessage({ text: `凭据已加密保存至全局配置，Token: ${maskSecret(token)}`, ok: true })
    } catch { setMessage({ text: '保存失败', ok: false }) }
    finally { setBusy(false) }
  }

  const handleFetchZones = async () => {
    setBusy(true); setMessage(null)
    try {
      const r = await getCloudflareZones({ cf_token: cfToken, cf_account_id: cfAccountId })
      setZones(r.zones)
      const normalizedDomain = domain.trim().toLowerCase()
      const matched = normalizedDomain
        ? r.zones.find((z) => normalizedDomain === z.name.toLowerCase() || normalizedDomain.endsWith(`.${z.name.toLowerCase()}`))
        : null
      if (matched) {
        setCFZoneId(matched.id)
        setDomainSuggestions(suggestSubdomains(matched.name))
      } else if (r.zones[0]?.name) {
        setDomainSuggestions(suggestSubdomains(r.zones[0].name))
      }
      setMessage({ text: `获取到 ${r.zones.length} 个 Zone`, ok: true })
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : '获取失败', ok: false })
    } finally { setBusy(false) }
  }

  const handleSaveDomain = async () => {
    if (!node) return
    setBusy(true); setMessage(null)
    try {
      const nextDomain = domain.trim()
      const nextEmail = email.trim()
      await updateNode(node.id, { domain: nextDomain, email: nextEmail, cf_token: cfToken.trim(), cf_account_id: cfAccountId.trim(), cf_zone_id: cfZoneId.trim() })
      await onSaveCF({ cf_token: cfToken, cf_account_id: cfAccountId, acme_email: email })
      setDomain(nextDomain)
      setEmail(nextEmail)
      setStep(6)
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : '保存失败', ok: false })
    } finally { setBusy(false) }
  }

  const handleFullDeploy = async () => {
    if (!node) return
    setBusy(true); setFullEvents([]); setShowCelebration(false); setFullDeployCertIssued(null); setFullDeployLog(''); setFullDeployLogCopied(false); setMessage(null)
    const secret = localStorage.getItem('cf_secret') || ''
    try {
      const patch: Partial<NodeCreateRequest> = {}
      if (domain.trim()) patch.domain = domain.trim()
      if (email.trim()) patch.email = email.trim()
      if (cfToken.trim()) patch.cf_token = cfToken.trim()
      if (cfAccountId.trim()) patch.cf_account_id = cfAccountId.trim()
      if (cfZoneId.trim()) patch.cf_zone_id = cfZoneId.trim()
      if (Object.keys(patch).length > 0) {
        const updated = await updateNode(node.id, patch)
        setCreatedNode(updated.node)
      }
      await streamSSE(
        `${BASE}/nodes/${encodeURIComponent(node.id)}/deploy`, secret,
        ev => setFullEvents(prev => [...prev, ev]),
        d => {
          if (!d.success) { setMessage({ text: d.error || '部署失败', ok: false }); return }
          const certIssued = Boolean(d.cert_issued)
          setFullDeployLog((d.deploy_log || '').trim())
          setFullDeployCertIssued(certIssued)
          if (d.phase === 'bootstrap') {
            setMessage({ text: '当前仅完成了 GOST 基础部署，未进入证书绑定阶段。请先在第5步确认域名/邮箱并保存。', ok: false })
            setStep(5)
            return
          }
          setMessage({
            text: certIssued
              ? '域名绑定、证书部署、服务重启与验证已完成'
              : '流程完成，但证书签发状态异常，请查看日志',
            ok: certIssued,
          })
          if (certIssued) {
            setShowCelebration(true)
          }
          setStep(7)
          onDone()
        },
        undefined,
        { mode: 'full' },
      )
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : '部署失败', ok: false })
    } finally { setBusy(false) }
  }

  const handleDomainProbe = async () => {
    if (!node) return
    setBusy(true); setMessage(null)
    try {
      const data = await probeNode(node.id, 'domain')
      setDomainProbe(data.probe_results)
      setMessage({ text: `域名探测 ${data.summary.ok}/${data.summary.total} 通过`, ok: data.summary.success })
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : '探测失败', ok: false })
    } finally { setBusy(false) }
  }

  const EventLog = ({ evs }: { evs: SSEEvent[] }) => (
    <div ref={eventsRef} className="max-h-36 overflow-y-auto rounded-xl border border-white/8 bg-black/20 p-3 font-mono text-[11px] space-y-1">
      {evs.length === 0 && <p className="text-muted flex items-center gap-2"><Loader2 size={10} className="animate-spin" />等待输出…</p>}
      {evs.map((ev, i) => (
        <div key={i} className={ev.status === 'error' ? 'text-red-300' : ev.status === 'ok' ? 'text-emerald-300' : ev.status === 'warning' ? 'text-amber-300' : 'text-slate-400'}>
          <p>[{ev.step}] {ev.message}</p>
          {ev.detail && <p className="pl-2 text-[10px] text-muted break-all">{ev.detail}</p>}
        </div>
      ))}
    </div>
  )

  const stepTitles = ['服务器信息', 'SSH 公钥授权', '部署 GOST', 'Cloudflare 凭据', '选择域名', '绑定 + 签证', '完成']

  const isWizardDirty = !!(form.name || form.host || step > 1)
  const handleWizardBeforeClose = () => {
    if (busy) return false
    if (!isWizardDirty) return true
    return window.confirm('确认放弃已输入的内容并关闭？')
  }

  return (
    <ModalShell
      title={`${initialNode ? '继续部署' : '新增节点'} · ${stepTitles[step - 1]}`}
      description={node ? `${node.username}@${node.host}` : '添加一台远程 Linux 服务器'}
      onClose={!busy ? onClose : undefined}
      onBeforeClose={handleWizardBeforeClose}
      size="lg"
      icon={<Server size={18} />}
    >
      <div className="space-y-4">
        <StepIndicator current={step} total={TOTAL_STEPS} />

        {/* ── Step 1: 服务器基本信息 ── */}
        {step === 1 && (
          <div className="space-y-3">
            <div className="grid grid-cols-3 gap-3">
              <div className="col-span-2">
                <label className="block text-xs text-slate-400 mb-1">名称</label>
                <input className="glass-input" value={form.name} onChange={e => updateForm('name', e.target.value)} placeholder="如: 东京-AWS" autoFocus />
              </div>
              <div>
                <label className="block text-xs text-slate-400 mb-1">SSH 端口</label>
                <input className="glass-input" type="number" value={form.port} onChange={e => updateForm('port', Number(e.target.value))} />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs text-slate-400 mb-1">主机地址 (IP)</label>
                <input className="glass-input font-mono" value={form.host} onChange={e => updateForm('host', e.target.value)} placeholder="1.2.3.4" />
              </div>
              <div>
                <label className="block text-xs text-slate-400 mb-1">用户名</label>
                <input className="glass-input" value={form.username} onChange={e => updateForm('username', e.target.value)} />
              </div>
            </div>
            <div>
              <label className="block text-xs text-slate-400 mb-1">SSH 密码 <span className="text-muted">(可选，授权 Key 后留空)</span></label>
              <input className="glass-input" type="password" value={form.password} onChange={e => updateForm('password', e.target.value)} placeholder="已授权 SSH Key 可留空" />
            </div>
            <div className="flex justify-end gap-3 pt-1">
              <button className="btn-ghost" onClick={onClose}>取消</button>
              {createdNode ? (
                <button className="btn-primary" onClick={() => setStep(2)}>
                  <CheckCircle2 size={14} /> 继续 →
                </button>
              ) : (
                <button className="btn-primary" disabled={busy || !form.name || !form.host} onClick={handleCreateNode}>
                  {busy ? <Loader2 size={14} className="animate-spin" /> : <CheckCircle2 size={14} />} 创建节点
                </button>
              )}
            </div>
          </div>
        )}

        {/* ── Step 2: SSH 公钥授权 ── */}
        {step === 2 && (
          <div className="space-y-3">
            <p className="text-xs text-slate-400 leading-relaxed">将路由器公钥加入服务器授权列表，后续无需密码即可连接。选择一种方式执行：</p>
            {sshPubKey && (
              <>
                <div>
                  <p className="text-[11px] text-muted mb-1">公钥</p>
                  <CopyableCode text={sshPubKey} />
                </div>
                <div>
                  <p className="text-[11px] text-muted mb-1">方式一：在本地终端执行（需本地已有服务器 SSH 权限）</p>
                  {authorizeCmd ? <CopyableCode text={authorizeCmd} /> : <p className="text-[11px] text-muted italic">填写主机地址后自动生成</p>}
                </div>
                <div>
                  <p className="text-[11px] text-muted mb-1">方式二：SSH 登录服务器后直接执行</p>
                  <CopyableCode text={localCmd} />
                </div>
              </>
            )}
            <div className="flex gap-3">
              {canGoBack && <button className="btn-ghost shrink-0" onClick={goBack} disabled={busy}>← 上一步</button>}
              <button className="btn-primary flex-1" disabled={busy} onClick={handleTestSSH}>
                {busy ? <Loader2 size={14} className="animate-spin" /> : <CheckCircle2 size={14} />} 已完成授权，校验 SSH 连接
              </button>
            </div>
          </div>
        )}

        {/* ── Step 3: Bootstrap 部署 ── */}
        {step === 3 && (
          <div className="space-y-3">
            <p className="text-xs text-slate-400 leading-relaxed">安装 GOST，以 IP 直连模式启动，并对 Google / YouTube / GitHub 发起连通性探测。</p>
            <div className="flex gap-3">
              {canGoBack && <button className="btn-ghost shrink-0" onClick={goBack} disabled={busy}>← 上一步</button>}
              <button className="btn-primary flex-1" disabled={busy} onClick={handleBootstrapDeploy}>
                {busy ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
                {busy ? '部署中，请等待…' : '开始部署并探测'}
              </button>
              <button className="btn-ghost" disabled={busy || !node} onClick={handleBootstrapProbe}>
                {busy ? <Loader2 size={14} className="animate-spin" /> : <Terminal size={14} />} 测试 GOST 连通
              </button>
              <button className="btn-primary" disabled={busy || !bootstrapCanContinue} onClick={() => setStep(4)}>
                下一步 →
              </button>
            </div>
            {(busy || events.length > 0) && <EventLog evs={events} />}
            {bootstrapProbeSummary && (
              <div className={`rounded-xl border px-3 py-2 text-xs ${
                bootstrapProbeSummary.success
                  ? 'border-emerald-500/20 bg-emerald-500/5 text-emerald-400'
                  : 'border-amber-500/20 bg-amber-500/5 text-amber-300'
              }`}>
                IP 连通性：{bootstrapProbeSummary.ok}/{bootstrapProbeSummary.total} 通过
              </div>
            )}
            {bootstrapProbe.length > 0 && <ProbeResultList results={bootstrapProbe} />}
          </div>
        )}

        {/* ── Step 4: Cloudflare 凭据 ── */}
        {step === 4 && (
          <div className="space-y-3">
            {cfConfig?.cf_token ? (
              <div className="flex items-center gap-2 rounded-xl border border-emerald-500/15 bg-emerald-500/[0.04] px-3 py-2 text-xs text-emerald-400">
                <CheckCircle2 size={12} className="shrink-0" />
                已从全局配置预加载 · 可直接进入下一步，或在下方修改后保存
              </div>
            ) : (
              <p className="text-xs text-slate-400 leading-relaxed">
                输入 Cloudflare API Token，凭据将加密保存至<strong className="text-slate-300">全局配置</strong>，节点部署与订阅分发共用。
              </p>
            )}
            <div className="rounded-xl border border-white/10 bg-black/20 p-3 text-xs text-slate-300 space-y-2">
              <p className="font-semibold text-slate-200">获取 Cloudflare API Token 和 Account ID</p>
              <ol className="list-decimal space-y-1.5 pl-4 text-[11px] leading-5 text-slate-300">
                <li>打开 API Tokens 页面，点击 <span className="font-medium text-slate-100">Create Token</span>。</li>
                <li>建议选择 <span className="font-medium text-slate-100">Create Custom Token</span>，并按下表添加权限。</li>
                <li>Zone Resources 选择需要部署的域名，Account Resources 选择当前账号。</li>
                <li>创建后复制 Token（只显示一次），并在控制台右侧复制 Account ID。</li>
              </ol>
              <CFPermissionTable compact />
              <div className="flex flex-wrap gap-2 pt-1">
                <a
                  href="https://dash.cloudflare.com/profile/api-tokens"
                  target="_blank"
                  rel="noreferrer"
                  className="btn-ghost h-7 px-2.5 text-xs"
                >
                  打开 API Tokens
                </a>
                <a
                  href="https://dash.cloudflare.com/"
                  target="_blank"
                  rel="noreferrer"
                  className="btn-ghost h-7 px-2.5 text-xs"
                >
                  打开 Cloudflare 控制台
                </a>
                <a
                  href="https://developers.cloudflare.com/fundamentals/api/reference/permissions/"
                  target="_blank"
                  rel="noreferrer"
                  className="btn-ghost h-7 px-2.5 text-xs"
                >
                  权限文档
                </a>
              </div>
            </div>
            <div>
              <label className="block text-xs text-slate-400 mb-1">CF API Token</label>
              <input className="glass-input" type="password" value={cfToken} onChange={e => setCFToken(e.target.value)} placeholder="Cloudflare API Token" autoFocus />
            </div>
            <div>
              <label className="block text-xs text-slate-400 mb-1">Account ID <span className="text-muted">(可选)</span></label>
              <input className="glass-input font-mono text-xs" value={cfAccountId} onChange={e => setCFAccountId(e.target.value)} />
            </div>
            <div className="flex gap-3">
              {canGoBack && <button className="btn-ghost shrink-0" onClick={goBack} disabled={busy}>← 上一步</button>}
              <button className="btn-ghost" disabled={!cfToken || busy} onClick={handleSaveCF}>
                {busy ? <Loader2 size={14} className="animate-spin" /> : <Check size={14} />} 保存凭据
              </button>
              <button className="btn-primary" disabled={!cfToken} onClick={() => setStep(5)}>
                下一步 →
              </button>
            </div>
          </div>
        )}

        {/* ── Step 5: 选择域名 ── */}
        {step === 5 && (
          <div className="space-y-3">
            <p className="text-xs text-slate-400 leading-relaxed">选择 Cloudflare 托管的域名，或手动输入二级域名，将用于 TLS 证书签发。</p>
            <div className="flex gap-2">
              <button className="btn-ghost flex-1" disabled={busy || !cfToken} onClick={handleFetchZones}>
                {busy ? <Loader2 size={14} className="animate-spin" /> : <RotateCw size={14} />} 拉取 Zone 列表
              </button>
            </div>
            {zones.length > 0 && (
              <select className="glass-input" value={cfZoneId} onChange={e => {
                setCFZoneId(e.target.value)
                const z = zones.find(v => v.id === e.target.value)
                if (z) setDomainSuggestions(suggestSubdomains(z.name))
              }}>
                <option value="">选择 Zone</option>
                {zones.map(z => <option key={z.id} value={z.id}>{z.name}</option>)}
              </select>
            )}
            {domainSuggestions.length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {domainSuggestions.map(s => (
                  <button key={s} className={`px-2.5 py-0.5 rounded-full border text-[10px] font-mono transition-colors ${domain === s ? 'border-brand/40 bg-brand/10 text-brand' : 'border-white/10 text-muted hover:text-slate-300 hover:border-white/20'}`} onClick={() => setDomain(s)}>{s}</button>
                ))}
              </div>
            )}
            <div>
              <label className="block text-xs text-slate-400 mb-1">二级域名</label>
              <input className="glass-input font-mono" value={domain} onChange={e => setDomain(e.target.value)} placeholder="如: edge-01.example.com" />
            </div>
            <div>
              <label className="block text-xs text-slate-400 mb-1">ACME 邮箱 (Let's Encrypt)</label>
              <input className="glass-input" type="email" value={email} onChange={e => setEmail(e.target.value)} placeholder="admin@example.com" />
            </div>
            <div className="flex gap-3">
              {canGoBack && <button className="btn-ghost shrink-0" onClick={goBack} disabled={busy}>← 上一步</button>}
              <button className="btn-primary flex-1" disabled={!domain || !email || !cfToken || busy} onClick={handleSaveDomain}>
                {busy ? <Loader2 size={14} className="animate-spin" /> : <Check size={14} />} 确认并进入下一步
              </button>
            </div>
          </div>
        )}

        {/* ── Step 6: 绑定域名 + 签发证书 ── */}
        {step === 6 && (
          <div className="space-y-3">
            <p className="text-xs text-slate-400 leading-relaxed">将为 <span className="text-slate-300 font-mono">{domain}</span> 绑定 DNS、签发并部署 TLS 证书，更新 GOST 配置并重启服务。</p>
            <div className="flex gap-3">
              {canGoBack && <button className="btn-ghost shrink-0" onClick={goBack} disabled={busy}>← 上一步</button>}
              <button className="btn-primary flex-1" disabled={busy} onClick={handleFullDeploy}>
                {busy ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
                {busy ? '绑定中，请等待…' : '开始绑定与签证'}
              </button>
            </div>
            {(busy || fullEvents.length > 0) && <FullDeployProgress evs={fullEvents} />}
            {(busy || fullEvents.length > 0) && <EventLog evs={fullEvents} />}
          </div>
        )}

        {/* ── Step 7: 最终探测 + 完成 ── */}
        {step === 7 && (
          <div
            className={[
              'relative overflow-hidden rounded-xl border p-3 space-y-3',
              fullDeployCertIssued
                ? 'border-emerald-500/15 bg-emerald-500/[0.03]'
                : 'border-amber-500/25 bg-amber-500/[0.06]',
            ].join(' ')}
          >
            <CelebrationFireworks show={showCelebration && !!fullDeployCertIssued} />
            <div className={`flex items-center gap-2 ${fullDeployCertIssued ? 'text-emerald-400' : 'text-amber-300'}`}>
              {fullDeployCertIssued ? <CheckCircle2 size={16} /> : <AlertCircle size={16} />}
              <p className="text-sm font-semibold">{fullDeployCertIssued ? '部署完成' : '部署存在异常'}</p>
            </div>
            <p className="text-xs text-slate-400">
              {fullDeployCertIssued
                ? '执行域名链路探测验证节点可用性，然后导出 Clash 配置。'
                : '证书签发未确认成功，请先查看部署日志并修复后重试。'}
            </p>
            {!fullDeployCertIssued && (
              <div className="rounded-xl border border-amber-500/20 bg-black/25 p-2 space-y-2">
                <p className="px-1 text-[11px] font-semibold uppercase tracking-[0.16em] text-amber-200/85">部署日志</p>
                {fullDeployLog ? (
                  <pre className="max-h-56 overflow-y-auto whitespace-pre-wrap rounded-lg border border-amber-500/15 bg-black/35 p-2.5 font-mono text-[11px] text-amber-100/90">
                    {fullDeployLog}
                  </pre>
                ) : fullEvents.length > 0 ? (
                  <EventLog evs={fullEvents} />
                ) : (
                  <p className="px-1 text-xs text-amber-200/70">暂无可显示的步骤日志</p>
                )}
                {(fullDeployLog || fullEvents.length > 0) && (
                  <button
                    className="btn-ghost h-7 px-2.5 text-xs"
                    onClick={async () => {
                      await copyText(fullDeployLog || stringifyEventLog(fullEvents))
                      setFullDeployLogCopied(true)
                      setTimeout(() => setFullDeployLogCopied(false), 1600)
                    }}
                  >
                    {fullDeployLogCopied ? <Check size={12} className="text-emerald-400" /> : <Copy size={12} />}
                    {fullDeployLogCopied ? '日志已复制' : '复制完整日志'}
                  </button>
                )}
              </div>
            )}
            {fullDeployCertIssued && (fullDeployLog || fullEvents.length > 0) && (
              <details className="rounded-xl border border-white/10 bg-black/20 p-2">
                <summary className="cursor-pointer text-xs text-slate-300">查看完整部署日志</summary>
                <pre className="mt-2 max-h-56 overflow-y-auto whitespace-pre-wrap rounded-lg border border-white/10 bg-black/35 p-2.5 font-mono text-[11px] text-slate-200/90">
                  {fullDeployLog || stringifyEventLog(fullEvents)}
                </pre>
              </details>
            )}
            {!fullDeployCertIssued && (
              <button
                className="btn-primary w-full"
                disabled={busy}
                onClick={() => { setMessage(null); setStep(6) }}
              >
                <RotateCw size={14} /> 返回上一步重试部署
              </button>
            )}
            <button className="btn-primary w-full" disabled={busy} onClick={handleDomainProbe}>
              {busy ? <Loader2 size={14} className="animate-spin" /> : <Terminal size={14} />} 执行域名探测
            </button>
            {domainProbe.length > 0 && <ProbeResultList results={domainProbe} />}
            {node && (
              <button className="btn-ghost w-full" onClick={() => onOpenExport(node)}>
                <Download size={14} /> 导出 Clash 配置
              </button>
            )}
            <button className="btn-ghost w-full" onClick={onClose}>关闭向导</button>
          </div>
        )}

        {/* Message bar */}
        {message && (
          <div className={`flex items-center gap-2 rounded-xl border px-3 py-2 text-xs ${message.ok ? 'border-emerald-500/20 bg-emerald-500/5 text-emerald-400' : 'border-red-500/20 bg-red-500/5 text-red-400'}`}>
            {message.ok ? <CheckCircle2 size={12} /> : <AlertCircle size={12} />}
            {message.text}
          </div>
        )}
      </div>
    </ModalShell>
  )
}

// ── Deploy / Destroy Progress Modal ──────────────────────────────────────────

function ProgressModal({
  title,
  nodeId,
  action,
  onClose,
  onDone,
}: {
  title: string
  nodeId: string
  action: 'deploy' | 'destroy'
  onClose: () => void
  onDone: () => void
}) {
  const [events, setEvents] = useState<SSEEvent[]>([])
  const [done, setDone] = useState<SSEDone | null>(null)
  const [deployLog, setDeployLog] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const secret = localStorage.getItem('cf_secret') || ''
    const url = `${BASE}/nodes/${encodeURIComponent(nodeId)}/${action}`
    const abort = new AbortController()
    setEvents([])
    setDone(null)
    setDeployLog('')

    streamSSE(
      url,
      secret,
      (ev) => setEvents(prev => [...prev, ev]),
      (d) => { setDone(d); if (d.deploy_log) setDeployLog(d.deploy_log) },
      abort.signal,
    ).catch(() => {})

    return () => abort.abort()
  }, [nodeId, action])

  useEffect(() => {
    containerRef.current?.scrollTo({ top: containerRef.current.scrollHeight, behavior: 'smooth' })
  }, [events])

  const isRun = !done
  const isSuccess = done?.success ?? false

  const statusIcon = (s: string) => {
    switch (s) {
      case 'ok': return <CheckCircle2 size={12} className="text-emerald-400 flex-shrink-0" />
      case 'error': return <X size={12} className="text-red-400 flex-shrink-0" />
      case 'warning': return <AlertCircle size={12} className="text-amber-400 flex-shrink-0" />
      default: return <Loader2 size={12} className="animate-spin text-brand flex-shrink-0" />
    }
  }

  return (
    <ModalShell
      title={title}
      onClose={isRun ? undefined : onClose}
      size="lg"
      icon={isRun ? <Loader2 size={18} className="animate-spin" /> : isSuccess ? <CheckCircle2 size={18} className="text-emerald-400" /> : <AlertCircle size={18} className="text-red-400" />}
      dismissible={!isRun}
    >
      <div ref={containerRef} className="space-y-2 max-h-64 overflow-y-auto rounded-xl bg-black/20 border border-white/5 p-3 font-mono text-xs">
        {events.length === 0 && !done && (
          <p className="text-muted flex items-center gap-2"><Loader2 size={12} className="animate-spin" /> 正在连接...</p>
        )}
        {events.map((ev, i) => (
          <div key={i} className="flex items-start gap-2 leading-5">
            {statusIcon(ev.status)}
            <span className={ev.status === 'error' ? 'text-red-300' : ev.status === 'warning' ? 'text-amber-300' : 'text-slate-300'}>
              [{ev.step}] {ev.message}
            </span>
            {ev.detail && <span className="text-muted/60 ml-1">{ev.detail.length > 80 ? ev.detail.slice(0, 80) + '...' : ev.detail}</span>}
          </div>
        ))}
        {done && (
          <div className={`pt-2 border-t border-white/5 flex items-center gap-2 leading-5 ${isSuccess ? 'text-emerald-400' : 'text-red-400'}`}>
            {isSuccess ? <CheckCircle2 size={12} /> : <X size={12} />}
            <span className="font-semibold">{isSuccess ? '操作完成 ✓' : `失败: ${done.error || '未知错误'}`}</span>
          </div>
        )}
      </div>

      {/* Error log */}
      {done && !isSuccess && deployLog && (
        <details className="mt-3">
          <summary className="text-xs text-red-400/80 cursor-pointer hover:text-red-300">查看详细日志</summary>
          <pre className="mt-2 rounded-xl bg-black/30 border border-red-500/15 p-3 text-xs font-mono text-red-300/80 max-h-48 overflow-y-auto whitespace-pre-wrap">
            {deployLog}
          </pre>
        </details>
      )}

      <div className="flex items-center justify-end gap-3 pt-4">
        {!isRun && !isSuccess && (
          <button className="btn-primary" onClick={() => {
            onClose()
            // Re-open will trigger re-deploy
            setTimeout(onDone, 100)
          }}>
            <RotateCw size={14} /> 重新部署
          </button>
        )}
        <button className="btn-ghost" onClick={onClose} disabled={isRun}>
          {isRun ? <Loader2 size={14} className="animate-spin" /> : null}
          {isRun ? '执行中...' : '关闭'}
        </button>
      </div>
    </ModalShell>
  )
}

// ── Proxy Probe Modal ───────────────────────────────────────────────────────

function NodeProbeModal({
  node,
  onClose,
}: {
  node: NodeListItem
  onClose: () => void
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [mode, setMode] = useState<'ip' | 'domain'>(node.domain.trim() ? 'domain' : 'ip')
  const [results, setResults] = useState<NodeProbeResult[]>([])
  const [summary, setSummary] = useState<{ ok: number; total: number; success: boolean } | null>(null)
  const [checkedAt, setCheckedAt] = useState('')

  const runProbe = useCallback(async (nextMode: 'ip' | 'domain') => {
    setMode(nextMode)
    setBusy(true)
    setError('')
    try {
      const data = await probeNode(node.id, nextMode)
      setMode(data.mode)
      setResults(data.probe_results ?? [])
      setSummary(data.summary ?? null)
      setCheckedAt(new Date().toLocaleString('zh-CN', { hour12: false }))
    } catch (e) {
      setError(e instanceof Error ? e.message : '代理探测失败')
      setResults([])
      setSummary(null)
    } finally {
      setBusy(false)
    }
  }, [node.id])

  useEffect(() => {
    const initialMode: 'ip' | 'domain' = node.domain.trim() ? 'domain' : 'ip'
    setMode(initialMode)
    void runProbe(initialMode)
  }, [node.domain, runProbe])

  return (
    <ModalShell
      title="代理可用性检测"
      onClose={onClose}
      size="lg"
      icon={<CheckCircle2 size={18} />}
    >
      <div className="space-y-4">
        <div className="rounded-xl border border-white/10 bg-black/15 p-3 text-xs text-slate-300">
          <p className="font-medium text-slate-200">{node.name}</p>
          <p className="mt-1 text-muted font-mono break-all">{node.username}@{node.host}:{node.port}</p>
          {checkedAt && <p className="mt-1 text-[11px] text-muted">最近检测: {checkedAt}</p>}
        </div>

        <div className="flex items-center gap-2">
          <button
            className="btn-ghost"
            disabled={busy}
            onClick={() => { void runProbe('domain') }}
          >
            {busy && mode === 'domain' ? <Loader2 size={14} className="animate-spin" /> : <CheckCircle2 size={14} />}
            域名探测
          </button>
          <button
            className="btn-ghost"
            disabled={busy}
            onClick={() => { void runProbe('ip') }}
          >
            {busy && mode === 'ip' ? <Loader2 size={14} className="animate-spin" /> : <Terminal size={14} />}
            IP 探测
          </button>
          {busy && <span className="text-xs text-muted">检测中...</span>}
        </div>

        {summary && (
          <div className={`rounded-xl border px-3 py-2 text-sm ${summary.success ? 'border-emerald-500/20 bg-emerald-500/5 text-emerald-300' : 'border-amber-500/20 bg-amber-500/5 text-amber-300'}`}>
            {summary.success
              ? `代理可用 · ${summary.ok}/${summary.total} 通过`
              : `代理部分可用 · ${summary.ok}/${summary.total} 通过`}
          </div>
        )}

        {error && (
          <div className="rounded-xl border border-red-500/20 bg-red-500/5 px-3 py-2 text-sm text-red-300">
            {error}
          </div>
        )}

        {results.length > 0 && <ProbeResultList results={results} />}

        <div className="flex items-center justify-end">
          <button className="btn-ghost" onClick={onClose}>关闭</button>
        </div>
      </div>
    </ModalShell>
  )
}

// ── Export Modal ─────────────────────────────────────────────────────────────

function ExportModal({ node, onClose }: { node: NodeListItem; onClose: () => void }) {
  const [yaml, setYaml] = useState('')
  const [loading, setLoading] = useState(true)
  const [exportError, setExportError] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    const secret = localStorage.getItem('cf_secret') || ''
    fetch(`${BASE}/nodes/${encodeURIComponent(node.id)}/proxy-config`, {
      headers: { Authorization: `Bearer ${secret}` },
    })
      .then(async (r) => {
        if (!r.ok) {
          const contentType = r.headers.get('content-type') ?? ''
          if (contentType.includes('application/json')) {
            const body = await r.json()
            throw new Error(body?.error?.message ?? '导出失败')
          }
          const text = await r.text()
          throw new Error(text || '导出失败')
        }
        return r.text()
      })
      .then(setYaml)
      .catch((e) => setExportError(e instanceof Error ? e.message : '导出失败'))
      .finally(() => setLoading(false))
  }, [node.id])

  const handleCopy = async () => {
    if (!yaml || exportError) return
    await copyText(yaml)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <ModalShell
      title="导出 Clash 代理配置"
      onClose={onClose}
      size="lg"
      icon={<Download size={18} />}
    >
      {loading ? (
        <div className="flex items-center gap-2 text-muted py-8 justify-center">
          <Loader2 size={16} className="animate-spin" /> 加载中...
        </div>
      ) : exportError ? (
        <div className="space-y-4">
          <div className="rounded-xl border border-red-500/20 bg-red-500/5 px-4 py-3 text-sm text-red-300">
            <AlertCircle size={14} className="inline mr-1.5 -mt-0.5" />
            {exportError}
          </div>
          <div className="flex items-center justify-end">
            <button className="btn-ghost" onClick={onClose}>关闭</button>
          </div>
        </div>
      ) : (
        <>
          <pre className="rounded-xl bg-black/30 border border-white/8 p-4 text-sm font-mono text-slate-200 overflow-x-auto max-h-80">
            {yaml}
          </pre>
          <div className="flex items-center justify-end gap-3 pt-4">
            <button className="btn-primary" onClick={handleCopy}>
              {copied ? <Check size={14} /> : <Copy size={14} />}
              {copied ? '已复制' : '复制到剪贴板'}
            </button>
            <button className="btn-ghost" onClick={onClose}>关闭</button>
          </div>
        </>
      )}
    </ModalShell>
  )
}

function WorkerExportModal({ node, onClose }: { node: WorkerNodeListItem; onClose: () => void }) {
  const [yaml, setYaml] = useState('')
  const [loading, setLoading] = useState(true)
  const [exportError, setExportError] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    setLoading(true)
    setExportError('')
    void getWorkerNodeClashConfig(node.id)
      .then((data) => setYaml(data.yaml ?? ''))
      .catch((e) => setExportError(e instanceof Error ? e.message : '导出失败'))
      .finally(() => setLoading(false))
  }, [node.id])

  const handleCopy = async () => {
    if (!yaml || exportError) return
    await copyText(yaml)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <ModalShell
      title="导出 Clash 代理配置"
      onClose={onClose}
      size="lg"
      icon={<Download size={18} />}
    >
      {loading ? (
        <div className="flex items-center gap-2 text-muted py-8 justify-center">
          <Loader2 size={16} className="animate-spin" /> 加载中...
        </div>
      ) : exportError ? (
        <div className="space-y-4">
          <div className="rounded-xl border border-red-500/20 bg-red-500/5 px-4 py-3 text-sm text-red-300">
            <AlertCircle size={14} className="inline mr-1.5 -mt-0.5" />
            {exportError}
          </div>
          <div className="flex items-center justify-end">
            <button className="btn-ghost" onClick={onClose}>关闭</button>
          </div>
        </div>
      ) : (
        <>
          <pre className="rounded-xl bg-black/30 border border-white/8 p-4 text-sm font-mono text-slate-200 overflow-x-auto max-h-80">
            {yaml}
          </pre>
          <div className="flex items-center justify-end gap-3 pt-4">
            <button className="btn-primary" onClick={handleCopy}>
              {copied ? <Check size={14} /> : <Copy size={14} />}
              {copied ? '已复制' : '复制到剪贴板'}
            </button>
            <button className="btn-ghost" onClick={onClose}>关闭</button>
          </div>
        </>
      )}
    </ModalShell>
  )
}

// ── Import Clash Proxy Modal ──────────────────────────────────────────────────

function ImportClashModal({ onClose, onImported }: { onClose: () => void; onImported: () => void }) {
  const [content, setContent] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [imported, setImported] = useState<{ id: string; node_count: number; nodes: { name: string; type: string; server: string; port: number }[] } | null>(null)
  const dragRef = useRef<HTMLDivElement>(null)

  const handleFileDrop = (e: React.DragEvent) => {
    e.preventDefault()
    const file = e.dataTransfer.files[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (ev) => { setContent(ev.target?.result as string ?? ''); setError('') }
    reader.readAsText(file)
  }

  const handleFileInput = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (ev) => { setContent(ev.target?.result as string ?? ''); setError('') }
    reader.readAsText(file)
  }

  const handleImport = async () => {
    if (!content.trim()) return
    setSaving(true); setError('')
    try {
      const res = await importSubscription(content)
      setImported({
        id: res.id,
        node_count: res.node_count,
        nodes: (res.nodes as { name: string; type: string; server: string; port: number }[]) ?? [],
      })
      onImported()
    } catch (e) {
      setError(e instanceof Error ? e.message : '导入失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <ModalShell
      title="导入 Clash 代理配置"
      onClose={onClose}
      size="lg"
      icon={<Upload size={18} />}
    >
      {imported ? (
        <div className="space-y-4">
          <div className="rounded-xl border border-emerald-500/20 bg-emerald-500/5 px-4 py-4 space-y-3">
            <div className="flex items-center gap-2">
              <CheckCircle2 size={18} className="text-emerald-400 shrink-0" />
              <p className="text-sm font-semibold text-emerald-300">导入成功，共 {imported.node_count} 个节点</p>
            </div>
            {imported.nodes.length > 0 && (
              <div className="rounded-lg border border-white/8 bg-black/20 max-h-52 overflow-y-auto divide-y divide-white/5">
                {imported.nodes.slice(0, 100).map((n, i) => (
                  <div key={i} className="flex items-center gap-3 px-3 py-1.5 text-xs">
                    <span className="shrink-0 rounded px-1.5 py-0.5 text-[10px] bg-brand/10 text-brand font-mono uppercase">{n.type || '?'}</span>
                    <span className="flex-1 truncate text-slate-300">{n.name}</span>
                    <span className="shrink-0 font-mono text-[11px] text-muted">{n.server}:{n.port}</span>
                  </div>
                ))}
                {imported.nodes.length > 100 && (
                  <div className="px-3 py-1.5 text-xs text-muted">…还有 {imported.nodes.length - 100} 个节点</div>
                )}
              </div>
            )}
          </div>
          <div className="flex justify-end">
            <button className="btn-ghost" onClick={onClose}>关闭</button>
          </div>
        </div>
      ) : (
        <div className="space-y-4">
          {/* Drop zone / textarea */}
          <div>
            <label className="block text-xs text-slate-400 mb-1">Clash 代理配置 (YAML)</label>
            <div
              ref={dragRef}
              onDragOver={e => e.preventDefault()}
              onDrop={handleFileDrop}
              className="relative rounded-xl border border-white/10 bg-black/20 transition-colors hover:border-brand/30"
            >
              <textarea
                className="w-full bg-transparent rounded-xl px-3 py-3 font-mono text-xs text-slate-300 placeholder:text-muted resize-none outline-none min-h-[180px]"
                value={content}
                onChange={e => { setContent(e.target.value); setError('') }}
                placeholder={"粘贴 Clash YAML 配置，或拖拽 .yaml 文件到此处\n\n支持完整 Clash 配置（含 proxies: 块）或纯代理列表"}
                spellCheck={false}
                autoFocus
              />
              <label className="absolute bottom-2 right-2 btn-ghost h-7 px-2.5 text-xs cursor-pointer">
                <Upload size={11} /> 选择文件
                <input type="file" accept=".yaml,.yml,.txt" className="hidden" onChange={handleFileInput} />
              </label>
            </div>
          </div>

          {error && (
            <div className="rounded-xl border border-red-500/20 bg-red-500/5 px-3 py-2 text-xs text-red-400 flex items-center gap-2">
              <AlertCircle size={12} /> {error}
            </div>
          )}

          <div className="flex items-center justify-end gap-3 pt-1">
            <button className="btn-ghost" onClick={onClose} disabled={saving}>取消</button>
            <button
              className="btn-primary"
              onClick={handleImport}
              disabled={saving || !content.trim()}
            >
              {saving ? <Loader2 size={14} className="animate-spin" /> : <Upload size={14} />}
              {saving ? '导入中…' : '确认导入'}
            </button>
          </div>
        </div>
      )}
    </ModalShell>
  )
}

// ── Azure VM Wizard ──────────────────────────────────────────────────────────

const AZURE_PROVISION_STEPS: Array<{ id: string; label: string }> = [
  { id: 'rg',       label: '准备资源组' },
  { id: 'vnet',     label: '创建虚拟网络' },
  { id: 'pip',      label: '分配公网 IP' },
  { id: 'nsg',      label: '配置安全组' },
  { id: 'nic',      label: '创建网络接口' },
  { id: 'vm',       label: '创建虚拟机' },
  { id: 'ip',       label: '获取 IP 地址' },
  { id: 'register', label: '注册托管节点' },
]

interface AzureProvisionDone {
  type: 'done'
  success: boolean
  error?: string
  node_id?: string
  public_ip?: string
  vm_id?: string
}

function AzureProvisionProgress({ evs }: { evs: SSEEvent[] }) {
  const statusByStep = new Map<string, string>()
  const eventByStep = new Map<string, SSEEvent>()
  for (const ev of evs) {
    statusByStep.set(ev.step, ev.status)
    eventByStep.set(ev.step, ev)
  }
  const doneCount = AZURE_PROVISION_STEPS.filter(({ id }) => {
    const s = statusByStep.get(id)
    return s === 'ok'
  }).length
  const pct = Math.round((doneCount / AZURE_PROVISION_STEPS.length) * 100)

  const statusIcon = (status?: string) => {
    if (status === 'ok') return <CheckCircle2 size={12} className="text-emerald-400 shrink-0" />
    if (status === 'error') return <X size={12} className="text-red-400 shrink-0" />
    if (status === 'running') return <Loader2 size={12} className="text-sky-400 shrink-0 animate-spin" />
    return <span className="inline-block h-2 w-2 rounded-full bg-white/20 shrink-0" />
  }

  return (
    <div className="rounded-xl border border-white/10 bg-black/20 p-3 space-y-3">
      <div className="space-y-1.5">
        <div className="flex items-center justify-between text-[11px]">
          <span className="text-slate-300">创建进度</span>
          <span className="text-slate-400 font-mono">{doneCount}/{AZURE_PROVISION_STEPS.length} · {pct}%</span>
        </div>
        <div className="h-1.5 rounded-full bg-white/10 overflow-hidden">
          <div className="h-full rounded-full bg-gradient-to-r from-sky-400 to-blue-500 transition-all duration-300" style={{ width: `${pct}%` }} />
        </div>
      </div>
      <div className="grid gap-1.5">
        {AZURE_PROVISION_STEPS.map(({ id, label }) => {
          const ev = eventByStep.get(id)
          const status = statusByStep.get(id)
          return (
            <div key={id} className="rounded-lg border border-white/8 bg-white/[0.03] px-2.5 py-2">
              <div className="flex items-center gap-2 text-[11px]">
                {statusIcon(status)}
                <span className={status === 'error' ? 'text-red-300' : status === 'ok' ? 'text-emerald-300' : status === 'running' ? 'text-sky-300' : 'text-slate-300'}>
                  {label}
                </span>
              </div>
              {ev && (
                <p className="mt-1 pl-5 text-[10px] text-muted break-all">
                  {ev.message}{ev.detail ? ` · ${ev.detail}` : ''}
                </p>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function AzureVMWizard({
  azureConfig,
  onSaveAzure,
  onClose,
  onDone,
}: {
  azureConfig: Omit<AzureConfig, 'client_secret'> | null
  onSaveAzure: (cfg: AzureConfig) => Promise<void>
  onClose: () => void
  onDone: () => void
}) {
  const [step, setStep] = useState<'credentials' | 'region' | 'config' | 'creating' | 'done'>('credentials')
  const [showAzureModal, setShowAzureModal] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  // Step: region
  const [locations, setLocations] = useState<AzureLocation[]>([])
  const [resourceGroups, setResourceGroups] = useState<AzureResourceGroup[]>([])
  const [selectedLocation, setSelectedLocation] = useState('')
  const [resourceGroupMode, setResourceGroupMode] = useState<'new' | 'existing'>('new')
  const [newResourceGroup, setNewResourceGroup] = useState('')
  const [selectedResourceGroup, setSelectedResourceGroup] = useState('')

  // Step: config
  const [vmSizes, setVMSizes] = useState<AzureVMSize[]>([])
  const [vmName, setVmName] = useState('')
  const [vmSize, setVmSize] = useState('Standard_B1s')
  const [adminUsername, setAdminUsername] = useState('clashforge')
  const [nodeName, setNodeName] = useState('')

  // Step: creating
  const [sseEvents, setSseEvents] = useState<SSEEvent[]>([])
  const [provisionResult, setProvisionResult] = useState<AzureProvisionDone | null>(null)

  const isConfigured = Boolean(azureConfig?.tenant_id && azureConfig?.subscription_id)

  // Load locations + RGs on credentials step completion
  const loadRegionData = useCallback(async () => {
    setBusy(true); setError('')
    try {
      const [locsData, rgsData] = await Promise.all([
        getAzureLocations(),
        getAzureResourceGroups(),
      ])
      setLocations(locsData.locations)
      setResourceGroups(rgsData.resource_groups)
      if (locsData.locations.length > 0 && !selectedLocation) {
        // Default to East Asia
        const eastAsia = locsData.locations.find(l => l.name === 'eastasia')
        setSelectedLocation(eastAsia?.name ?? locsData.locations[0].name)
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : '加载失败')
    } finally {
      setBusy(false)
    }
  }, [selectedLocation])

  // Load VM sizes when location changes
  const loadVMSizes = useCallback(async (location: string) => {
    if (!location) return
    setBusy(true)
    try {
      const data = await getAzureVMSizes(location)
      setVMSizes(data.vm_sizes)
      if (data.vm_sizes.length > 0 && !data.vm_sizes.find(s => s.name === vmSize)) {
        setVmSize(data.vm_sizes[0].name)
      }
    } catch {
      // Ignore, use fallback list
    } finally {
      setBusy(false)
    }
  }, [vmSize])

  const handleGoToRegion = async () => {
    if (!isConfigured) { setError('请先配置 Azure 凭据'); return }
    // Validate credentials first
    setBusy(true); setError('')
    try {
      await validateAzureCredentials()
      await loadRegionData()
      setStep('region')
    } catch (e) {
      setError(e instanceof Error ? e.message : '凭据验证失败，请检查配置')
    } finally {
      setBusy(false)
    }
  }

  const handleGoToConfig = async () => {
    const rg = resourceGroupMode === 'new' ? newResourceGroup.trim() : selectedResourceGroup
    if (!selectedLocation) { setError('请选择区域'); return }
    if (!rg) { setError('请输入或选择资源组名称'); return }
    setError('')
    await loadVMSizes(selectedLocation)
    setStep('config')
  }

  const handleStartProvision = async () => {
    const rg = resourceGroupMode === 'new' ? newResourceGroup.trim() : selectedResourceGroup
    if (!vmName.trim()) { setError('请输入虚拟机名称'); return }
    if (!adminUsername.trim()) { setError('请输入管理员用户名'); return }
    setError(''); setSseEvents([]); setProvisionResult(null)
    setStep('creating')

    const secret = localStorage.getItem('cf_secret') || ''
    const payload = {
      location: selectedLocation,
      resource_group: rg,
      vm_name: vmName.trim().toLowerCase(),
      vm_size: vmSize,
      admin_username: adminUsername.trim().toLowerCase(),
      node_name: nodeName.trim() || vmName.trim(),
    }

    try {
      const resp = await fetch('/api/v1/azure/vms', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(secret ? { Authorization: `Bearer ${secret}` } : {}),
        },
        body: JSON.stringify(payload),
      })
      if (!resp.ok) {
        const json = await resp.json().catch(() => ({}))
        throw new Error(json?.error?.message ?? `HTTP ${resp.status}`)
      }
      const reader = resp.body?.getReader()
      if (!reader) throw new Error('无法读取响应流')
      const decoder = new TextDecoder()
      let buffer = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() ?? ''
        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          try {
            const data = JSON.parse(line.slice(6))
            if (data.type === 'done') {
              setProvisionResult(data as AzureProvisionDone)
              if (data.success) { onDone(); setStep('done') }
            } else {
              setSseEvents(prev => [...prev, data as SSEEvent])
            }
          } catch { /* skip malformed */ }
        }
      }
    } catch (e) {
      setProvisionResult({ type: 'done', success: false, error: e instanceof Error ? e.message : '创建失败' })
    }
  }

  return (
    <ModalShell
      title="Azure 云主机"
      description="通过 Azure API 开机一台 Linux VM，自动配置 SSH 并注册为托管节点"
      icon={<Cloud size={18} />}
      onClose={onClose}
      size="xl"
      dismissible={step !== 'creating'}
    >
      {/* ── Step: Credentials ── */}
      {step === 'credentials' && (
        <div className="space-y-4">
          <AzureConfigBanner
            config={azureConfig}
            loading={false}
            onConfigure={() => setShowAzureModal(true)}
          />

          {!isConfigured && (
            <div className="rounded-xl border border-white/10 bg-black/20 p-4 space-y-3">
              <div className="flex items-center gap-3">
                <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-sky-500/20 bg-sky-500/10">
                  <Cloud size={18} className="text-sky-400" />
                </div>
                <div>
                  <p className="text-sm font-semibold text-white">通过 Azure 创建代理服务器</p>
                  <p className="text-[11px] text-muted mt-0.5">配置 Service Principal 后，ClashForge 将全程引导你在 Azure 开机云主机并注册为节点</p>
                </div>
              </div>
              <div className="grid grid-cols-3 gap-2">
                {['配置 Azure 凭据', '选择区域与规格', '自动开机注册'].map((s, i) => (
                  <div key={s} className="rounded-lg border border-white/8 bg-white/[0.03] px-3 py-2 text-center">
                    <div className="text-lg font-bold text-sky-400/60 mb-0.5">{i + 1}</div>
                    <p className="text-[11px] text-slate-300">{s}</p>
                  </div>
                ))}
              </div>
            </div>
          )}

          {error && (
            <div className="flex items-center gap-2 rounded-xl border border-red-500/20 bg-red-500/5 px-3 py-2 text-xs text-red-400">
              <AlertCircle size={12} className="shrink-0" />{error}
            </div>
          )}

          <div className="flex items-center justify-end gap-3 pt-1">
            <button className="btn-ghost" onClick={onClose}>取消</button>
            {!isConfigured ? (
              <button className="btn-primary" onClick={() => setShowAzureModal(true)}>
                <Cloud size={14} /> 配置 Azure 凭据
              </button>
            ) : (
              <button className="btn-primary" onClick={handleGoToRegion} disabled={busy}>
                {busy ? <Loader2 size={14} className="animate-spin" /> : <CheckCircle2 size={14} />}
                下一步：选择区域
              </button>
            )}
          </div>
        </div>
      )}

      {/* ── Step: Region + Resource Group ── */}
      {step === 'region' && (
        <div className="space-y-4">
          <div>
            <label className="block text-xs text-slate-400 mb-1">选择 Azure 区域 <span className="text-red-400">*</span></label>
            <select
              className="glass-input"
              value={selectedLocation}
              onChange={e => setSelectedLocation(e.target.value)}
              disabled={busy}
            >
              {locations.length === 0 && <option value="">加载中…</option>}
              {locations.map(l => (
                <option key={l.name} value={l.name}>{l.display_name} ({l.name})</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs text-slate-400 mb-1">资源组</label>
            <div className="flex gap-2 mb-2">
              <button
                className={`flex-1 rounded-lg border px-3 py-2 text-xs transition-colors ${resourceGroupMode === 'new' ? 'border-sky-500/40 bg-sky-500/10 text-sky-300' : 'border-white/10 text-muted hover:text-slate-300'}`}
                onClick={() => setResourceGroupMode('new')}
              >
                新建资源组
              </button>
              <button
                className={`flex-1 rounded-lg border px-3 py-2 text-xs transition-colors ${resourceGroupMode === 'existing' ? 'border-sky-500/40 bg-sky-500/10 text-sky-300' : 'border-white/10 text-muted hover:text-slate-300'}`}
                onClick={() => setResourceGroupMode('existing')}
                disabled={resourceGroups.length === 0}
              >
                选择已有资源组 {resourceGroups.length > 0 ? `(${resourceGroups.length})` : ''}
              </button>
            </div>
            {resourceGroupMode === 'new' ? (
              <input
                className="glass-input"
                value={newResourceGroup}
                onChange={e => setNewResourceGroup(e.target.value)}
                placeholder="例如: clashforge-rg"
              />
            ) : (
              <select
                className="glass-input"
                value={selectedResourceGroup}
                onChange={e => setSelectedResourceGroup(e.target.value)}
              >
                <option value="">— 选择资源组 —</option>
                {resourceGroups.map(rg => (
                  <option key={rg.name} value={rg.name}>{rg.name} ({rg.location})</option>
                ))}
              </select>
            )}
          </div>

          {error && (
            <div className="flex items-center gap-2 rounded-xl border border-red-500/20 bg-red-500/5 px-3 py-2 text-xs text-red-400">
              <AlertCircle size={12} className="shrink-0" />{error}
            </div>
          )}

          <div className="flex items-center justify-between pt-1">
            <button className="btn-ghost" onClick={() => { setStep('credentials'); setError('') }}>← 上一步</button>
            <button className="btn-primary" onClick={handleGoToConfig} disabled={busy}>
              {busy ? <Loader2 size={14} className="animate-spin" /> : null}
              下一步：配置虚拟机
            </button>
          </div>
        </div>
      )}

      {/* ── Step: VM Config ── */}
      {step === 'config' && (
        <div className="space-y-4">
          <div className="rounded-xl border border-white/8 bg-white/[0.02] px-4 py-3 text-xs text-slate-300 space-y-1">
            <div className="flex items-center gap-2">
              <span className="text-muted">区域:</span>
              <span className="font-semibold text-white">{locations.find(l => l.name === selectedLocation)?.display_name ?? selectedLocation}</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-muted">资源组:</span>
              <span className="font-semibold text-white">{resourceGroupMode === 'new' ? newResourceGroup : selectedResourceGroup}</span>
              {resourceGroupMode === 'new' && <span className="text-[10px] text-sky-400/70">（将自动创建）</span>}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-slate-400 mb-1">虚拟机名称 <span className="text-red-400">*</span></label>
              <input
                className="glass-input font-mono text-xs"
                value={vmName}
                onChange={e => setVmName(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))}
                placeholder="my-proxy-vm"
                maxLength={15}
              />
              <p className="text-[10px] text-muted mt-0.5">仅限小写字母、数字、连字符，最多 15 字符</p>
            </div>
            <div>
              <label className="block text-xs text-slate-400 mb-1">节点显示名称</label>
              <input
                className="glass-input text-xs"
                value={nodeName}
                onChange={e => setNodeName(e.target.value)}
                placeholder={vmName || '默认同虚拟机名称'}
              />
            </div>
          </div>

          <div>
            <label className="block text-xs text-slate-400 mb-1">虚拟机规格 <span className="text-red-400">*</span></label>
            <select
              className="glass-input"
              value={vmSize}
              onChange={e => setVmSize(e.target.value)}
            >
              {vmSizes.length === 0 ? (
                // Fallback hardcoded list
                [
                  { name: 'Standard_B1s', cores: 1, memory_gb: '1.0' },
                  { name: 'Standard_B1ms', cores: 1, memory_gb: '2.0' },
                  { name: 'Standard_B2s', cores: 2, memory_gb: '4.0' },
                  { name: 'Standard_B2ms', cores: 2, memory_gb: '8.0' },
                  { name: 'Standard_D2s_v3', cores: 2, memory_gb: '8.0' },
                ].map(s => (
                  <option key={s.name} value={s.name}>{s.name} · {s.cores} vCPU · {s.memory_gb} GB RAM</option>
                ))
              ) : vmSizes.map(s => (
                <option key={s.name} value={s.name}>{s.name} · {s.cores} vCPU · {s.memory_gb} GB RAM</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs text-slate-400 mb-1">管理员用户名 <span className="text-red-400">*</span></label>
            <input
              className="glass-input font-mono text-xs"
              value={adminUsername}
              onChange={e => setAdminUsername(e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, ''))}
              placeholder="clashforge"
            />
            <p className="text-[10px] text-muted mt-0.5">ClashForge 将通过内置 SSH 公钥认证访问此用户</p>
          </div>

          <div className="rounded-xl border border-sky-500/15 bg-sky-500/[0.04] px-4 py-3 space-y-1.5 text-xs">
            <p className="font-semibold text-sky-300 flex items-center gap-1.5"><CheckCircle2 size={12} /> 自动配置的内容</p>
            <ul className="space-y-0.5 text-sky-300/70 pl-4 list-disc">
              <li>系统镜像: Ubuntu 24.04 LTS</li>
              <li>SSH 公钥: ClashForge 内置密钥对（无需密码）</li>
              <li>安全组: 开放 SSH 22 端口</li>
              <li>公网 IP: 静态 Standard SKU</li>
              <li>磁盘: Premium SSD</li>
              <li>注册为托管节点（不自动部署 GOST）</li>
            </ul>
          </div>

          {error && (
            <div className="flex items-center gap-2 rounded-xl border border-red-500/20 bg-red-500/5 px-3 py-2 text-xs text-red-400">
              <AlertCircle size={12} className="shrink-0" />{error}
            </div>
          )}

          <div className="flex items-center justify-between pt-1">
            <button className="btn-ghost" onClick={() => { setStep('region'); setError('') }}>← 上一步</button>
            <button
              className="btn-primary"
              onClick={handleStartProvision}
              disabled={!vmName.trim() || !adminUsername.trim()}
            >
              <Cloud size={14} /> 开始创建虚拟机
            </button>
          </div>
        </div>
      )}

      {/* ── Step: Creating (SSE progress) ── */}
      {step === 'creating' && (
        <div className="space-y-4">
          <div className="flex items-center gap-3 rounded-xl border border-sky-500/15 bg-sky-500/[0.04] px-4 py-3">
            {provisionResult ? (
              provisionResult.success
                ? <CheckCircle2 size={16} className="text-emerald-400 shrink-0" />
                : <AlertCircle size={16} className="text-red-400 shrink-0" />
            ) : (
              <Loader2 size={16} className="text-sky-400 shrink-0 animate-spin" />
            )}
            <div>
              <p className="text-sm font-semibold text-white">
                {provisionResult
                  ? (provisionResult.success ? '虚拟机创建完成！' : '创建失败')
                  : '正在 Azure 创建虚拟机…'}
              </p>
              <p className="text-[11px] text-muted mt-0.5">
                {provisionResult?.success
                  ? `公网 IP: ${provisionResult.public_ip}`
                  : provisionResult?.error
                    ? provisionResult.error
                    : '整个过程通常需要 3-8 分钟，请耐心等待'}
              </p>
            </div>
          </div>

          <AzureProvisionProgress evs={sseEvents} />

          {provisionResult && !provisionResult.success && (
            <div className="flex items-center justify-end gap-3">
              <button className="btn-ghost" onClick={() => { setStep('config'); setError('') }}>← 返回修改</button>
              <button className="btn-ghost" onClick={onClose}>关闭</button>
            </div>
          )}
        </div>
      )}

      {/* ── Step: Done ── */}
      {step === 'done' && provisionResult?.success && (
        <div className="space-y-4">
          <div className="rounded-xl border border-emerald-500/20 bg-emerald-500/5 px-4 py-5 space-y-3">
            <div className="flex items-center gap-3">
              <CheckCircle2 size={20} className="text-emerald-400 shrink-0" />
              <div>
                <p className="text-sm font-semibold text-emerald-300">Azure 虚拟机已创建并注册为托管节点</p>
                <p className="text-xs text-emerald-300/60 mt-0.5">节点已以「已连接」状态加入节点列表，可随时部署 GOST 代理服务</p>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-2 text-xs">
              <div className="rounded-lg border border-white/8 bg-black/20 px-3 py-2">
                <p className="text-muted mb-0.5">公网 IP</p>
                <p className="font-mono font-semibold text-white">{provisionResult.public_ip}</p>
              </div>
              <div className="rounded-lg border border-white/8 bg-black/20 px-3 py-2">
                <p className="text-muted mb-0.5">管理员用户</p>
                <p className="font-mono font-semibold text-white">{adminUsername}</p>
              </div>
            </div>
          </div>

          <div className="rounded-xl border border-sky-500/15 bg-sky-500/[0.04] px-4 py-3 text-xs text-sky-300/80 space-y-1">
            <p className="font-semibold text-sky-300">接下来可以做什么</p>
            <ul className="space-y-0.5 list-disc pl-4">
              <li>在「节点列表」找到刚创建的节点，点击 <strong>测试连接</strong> 验证 SSH 访问</li>
              <li>点击「部署向导」完成 GOST + TLS 证书部署，使其成为代理中继节点</li>
              <li>或直接使用此服务器的 IP 配置手动代理规则</li>
            </ul>
          </div>

          <div className="flex items-center justify-end">
            <button className="btn-primary" onClick={onClose}>完成</button>
          </div>
        </div>
      )}

      {/* Azure credentials modal */}
      {showAzureModal && (
        <AzureConfigModal
          initial={azureConfig}
          save={onSaveAzure}
          onClose={() => setShowAzureModal(false)}
          onSaved={() => { setShowAzureModal(false) }}
        />
      )}
    </ModalShell>
  )
}

// ── Main Page ────────────────────────────────────────────────────────────────

export function Nodes() {
  const { config: cfGlobal, loading: cfLoading, save: saveCFGlobal, reload: reloadCF } = useCFConfig()
  const { config: azureGlobal, save: saveAzureGlobal } = useAzureConfig()
  const [showCFModal, setShowCFModal] = useState(false)
  const [showAzureConfigModal, setShowAzureConfigModal] = useState(false)
  const [showAzureWizard, setShowAzureWizard] = useState(false)

  const [nodes, setNodes] = useState<NodeListItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // Modal state
  const [showWizard, setShowWizard] = useState(false)
  const [wizardInitialNode, setWizardInitialNode] = useState<NodeListItem | undefined>()
  const [showEditModal, setShowEditModal] = useState(false)
  const [editNode, setEditNode] = useState<NodeListItem | undefined>()

  const openWizard = (node?: NodeListItem) => { setWizardInitialNode(node); setShowWizard(true) }
  const closeWizard = () => { setShowWizard(false); setWizardInitialNode(undefined) }
  const [progressNodeId, setProgressNodeId] = useState<string | null>(null)
  const [progressAction, setProgressAction] = useState<'deploy' | 'destroy'>('deploy')
  const [exportNode, setExportNode] = useState<NodeListItem | null>(null)
  const [workerExportNode, setWorkerExportNode] = useState<WorkerNodeListItem | null>(null)
  const [probeTargetNode, setProbeTargetNode] = useState<NodeListItem | null>(null)
  const [testLoading, setTestLoading] = useState<Record<string, boolean>>({})

  // ── Worker nodes state ───────────────────────────────────────────────────
  const [workerNodes, setWorkerNodes] = useState<WorkerNodeListItem[]>([])
  const [showWorkerWizard, setShowWorkerWizard] = useState(false)
  const [showImportModal, setShowImportModal] = useState(false)

  // ── Static (imported) subscriptions ─────────────────────────────────────
  type StaticNodeRow = { subId: string; name: string; type: string; server: string; port: number }
  const [importedNodes, setImportedNodes] = useState<StaticNodeRow[]>([])

  const loadStaticSubs = useCallback(async () => {
    try {
      const data = await getNodeImports()
      const rows: StaticNodeRow[] = []
      await Promise.all((data.subscriptions ?? []).map(async (sub: Subscription) => {
        try {
          const res = await getSubscriptionNodes(sub.id)
          for (const n of res.nodes) {
            rows.push({ subId: sub.id, name: n.name, type: n.type, server: n.server, port: n.port })
          }
        } catch { /* non-fatal */ }
      }))
      setImportedNodes(rows)
    } catch { /* non-fatal */ }
  }, [])

  useEffect(() => { void loadStaticSubs() }, [loadStaticSubs])

  const handleStaticSubDelete = async (subId: string) => {
    const count = importedNodes.filter(n => n.subId === subId).length
    const label = count > 1 ? `这组导入节点（${count} 个）` : '这个导入节点'
    if (!confirm(`确定删除${label}？`)) return
    try { await deleteSubscription(subId); void loadStaticSubs() }
    catch (e) { alert(e instanceof Error ? e.message : '删除失败') }
  }

  const loadWorkerNodes = useCallback(async () => {
    try {
      const data = await getWorkerNodes()
      setWorkerNodes(data.nodes ?? [])
    } catch { /* non-fatal */ }
  }, [])

  useEffect(() => { void loadWorkerNodes() }, [loadWorkerNodes])

  const handleWorkerDelete = async (id: string) => {
    if (!confirm('确定删除此 Worker 节点？（CF 上的 Worker 脚本也将被删除）')) return
    try { await deleteWorkerNode(id); void loadWorkerNodes() }
    catch (e) { alert(e instanceof Error ? e.message : '删除失败') }
  }

  const handleWorkerRedeploy = async (id: string) => {
    try { await redeployWorkerNode(id); void loadWorkerNodes() }
    catch (e) { alert(e instanceof Error ? e.message : '重新部署失败') }
  }

  const handleWorkerExport = (id: string) => {
    const target = workerNodes.find((item) => item.id === id)
    if (!target) return
    setWorkerExportNode(target)
  }

  const loadNodes = useCallback(async () => {
    try {
      setError('')
      const data = await getNodes()
      setNodes(data.nodes ?? [])
    } catch (e) {
      setError(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { loadNodes() }, [loadNodes])

  const handleTest = async (id: string) => {
    setTestLoading(prev => ({ ...prev, [id]: true }))
    try {
      await testNodeConnection(id)
    } catch { /* error shown in button */ }
    finally {
      setTestLoading(prev => ({ ...prev, [id]: false }))
      loadNodes()
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm('确定删除此节点？')) return
    try {
      await deleteNode(id)
      loadNodes()
    } catch (e) {
      alert(e instanceof Error ? e.message : '删除失败')
    }
  }

  const handleDeploy = (id: string) => {
    setProgressNodeId(id)
    setProgressAction('deploy')
  }

  const handleDestroy = (id: string) => {
    if (!confirm('确定要远程销毁 GOST 部署吗？\n\n这将停止服务、删除配置和证书，恢复服务器至部署前状态。')) return
    setProgressNodeId(id)
    setProgressAction('destroy')
  }

  const handleProgressDone = () => {
    loadNodes()
  }

  const totalNodes = nodes.length
  const connectedNodes = nodes.filter(n => n.status === 'connected' || n.status === 'deployed').length
  const deployedNodes = nodes.filter(isNodeFullyDeployed).length

  return (
    <div className="space-y-6 animate-fade-in">
      <PageHeader
        eyebrow="代理资源 · 中继节点"
        title="节点服务器"
        description="托管节点（VPS + GOST）· 导入节点（Clash YAML）· Worker 节点（CF Workers）· Azure 云主机"
        metrics={[
          { label: '节点总数', value: String(totalNodes) },
          { label: '已连接', value: String(connectedNodes) },
          { label: '已部署', value: String(deployedNodes) },
        ]}
        actions={cfGlobal?.cf_token ? (
          <div className="flex items-center gap-2">
            <button className="btn-ghost flex items-center gap-2" onClick={() => setShowImportModal(true)}>
              <Upload size={14} /> 导入节点
            </button>
            <button className="btn-ghost flex items-center gap-2" onClick={() => setShowWorkerWizard(true)}>
              <CloudCog size={14} /> Worker 节点
            </button>
            <button className="btn-ghost flex items-center gap-2" onClick={() => setShowAzureWizard(true)}>
              <Cloud size={14} /> Azure 云主机
            </button>
            <button className="btn-primary" onClick={() => openWizard()}>
              <Plus size={14} /> 新增节点
            </button>
          </div>
        ) : null}
      />

      <CFGate config={cfGlobal} loading={cfLoading} save={saveCFGlobal}>

      {error && (
        <div className="rounded-2xl border border-red-500/20 bg-red-500/5 px-4 py-3 text-sm text-red-400 flex items-center gap-2">
          <AlertCircle size={14} /> {error}
        </div>
      )}

      {loading ? (
        <SectionCard title="托管节点" description="远程 Linux 服务器，通过 GOST + TLS 提供代理转发">
          <div className="flex items-center justify-center py-12">
            <Loader2 size={24} className="animate-spin text-brand" />
          </div>
        </SectionCard>
      ) : nodes.length === 0 ? (
        <SectionCard title="托管节点" description="远程 Linux 服务器，通过 GOST + TLS 提供代理转发">
          <EmptyState
            title="暂无托管节点"
            description="添加一台远程 Linux 服务器，ClashForge 将自动部署 GOST 代理并签发 TLS 证书。"
            action={
              <button className="btn-primary" onClick={() => openWizard()}>
                <Plus size={14} /> 添加第一台服务器
              </button>
            }
            icon={<Server size={18} />}
          />
        </SectionCard>
      ) : (
        <SectionCard title="托管节点" description="远程 Linux 服务器，通过 GOST + TLS 提供代理转发">
          <div className="table-shell">
            {/* Table header */}
            <div className="grid grid-cols-12 gap-3 px-4 py-3 table-header-row">
              <span className="col-span-3">名称</span>
              <span className="col-span-2">主机</span>
              <span className="col-span-2">域名</span>
              <span className="col-span-1">状态</span>
              <span className="col-span-4 text-right">操作</span>
            </div>
            {/* Rows */}
            {nodes.map(node => {
              const fullyDeployed = isNodeFullyDeployed(node)
              return (
              <div key={node.id} className="grid grid-cols-12 gap-3 px-4 py-3.5 table-row items-center">
                <div className="col-span-3">
                  <p className="text-sm font-semibold text-white truncate">{node.name}</p>
                  <p className="text-[11px] text-muted mt-0.5 truncate">{node.username}@{node.host}:{node.port}</p>
                </div>
                <div className="col-span-2">
                  <span className="text-sm text-slate-300 font-mono text-xs truncate block">{node.host}</span>
                </div>
                <div className="col-span-2">
                  <span className="text-sm text-slate-300 truncate block">{node.domain || '—'}</span>
                </div>
                <div className="col-span-1">
                  <StatusBadge status={node.status} />
                  {node.error && node.status === 'error' && (
                    <p className="text-[10px] text-red-400/70 mt-1 truncate max-w-[120px]" title={node.error}>{node.error}</p>
                  )}
                </div>
                <div className="col-span-4 flex items-center justify-end gap-2">
                  {/* Test connection */}
                  <button
                    className="btn-icon-sm btn-ghost"
                    title="测试连接"
                    onClick={() => handleTest(node.id)}
                    disabled={testLoading[node.id]}
                  >
                    {testLoading[node.id] ? <Loader2 size={14} className="animate-spin" /> : <Terminal size={14} />}
                  </button>

                  {/* Deploy / resume wizard for non-fully-deployed nodes */}
                  {!fullyDeployed && (
                    <button className="btn-icon-sm btn-ghost" title="继续部署向导" onClick={() => openWizard(node)}>
                      <Play size={14} className="text-brand" />
                    </button>
                  )}

                  {/* Proxy probe for fully deployed nodes */}
                  {fullyDeployed && (
                    <button className="btn-icon-sm btn-ghost" title="代理探测" onClick={() => setProbeTargetNode(node)}>
                      <CheckCircle2 size={14} className="text-cyan-400" />
                    </button>
                  )}

                  {/* Redeploy for fully deployed nodes */}
                  {fullyDeployed && (
                    <button className="btn-icon-sm btn-ghost" title="重新部署" onClick={() => handleDeploy(node.id)}>
                      <RotateCw size={14} className="text-amber-400" />
                    </button>
                  )}

                  {/* Destroy */}
                  {fullyDeployed && (
                    <button className="btn-icon-sm btn-ghost" title="销毁部署" onClick={() => handleDestroy(node.id)}>
                      <ShieldOff size={14} className="text-red-400" />
                    </button>
                  )}

                  {/* Export */}
                  {fullyDeployed && (
                    <button className="btn-icon-sm btn-ghost" title="导出配置" onClick={() => setExportNode(node)}>
                      <Download size={14} className="text-emerald-400" />
                    </button>
                  )}

                  {/* Edit */}
                  <button className="btn-icon-sm btn-ghost" title="编辑" onClick={() => {
                    if (!fullyDeployed) { openWizard(node) }
                    else { setEditNode(node); setShowEditModal(true) }
                  }}>
                    <Pencil size={14} />
                  </button>

                  {/* Delete */}
                  <button className="btn-icon-sm btn-ghost" title="删除" onClick={() => handleDelete(node.id)}>
                    <Trash2 size={14} className="text-muted hover:text-red-400" />
                  </button>
                </div>
              </div>
              )
            })}
          </div>
        </SectionCard>
      )}

      {/* ── Imported (static) Nodes ──────────────────────────────────── */}
      <SectionCard
        title="导入节点"
        description="通过粘贴 Clash YAML 配置导入的代理节点组，参与 mihomo 配置生成"
        actions={
          <button className="btn-ghost h-7 px-2.5 text-xs flex items-center gap-1.5" onClick={() => setShowImportModal(true)}>
            <Upload size={12} /> 导入配置
          </button>
        }
      >
        {importedNodes.length === 0 ? (
          <EmptyState
            title="暂无导入节点"
            description="点击「导入配置」粘贴 Clash YAML，将代理节点加入 mihomo 路由配置。"
            action={
              <button className="btn-primary" onClick={() => setShowImportModal(true)}>
                <Upload size={14} /> 导入 Clash 配置
              </button>
            }
            icon={<Upload size={18} />}
          />
        ) : (
          <div className="table-shell">
            <div className="grid grid-cols-12 gap-3 px-4 py-3 table-header-row">
              <span className="col-span-5">节点名称</span>
              <span className="col-span-2">类型</span>
              <span className="col-span-3">服务器</span>
              <span className="col-span-2 text-right">操作</span>
            </div>
            {importedNodes.map((node, i) => (
              <div key={i} className="grid grid-cols-12 gap-3 px-4 py-3 table-row items-center">
                <div className="col-span-5">
                  <p className="text-sm font-semibold text-white truncate">{node.name}</p>
                </div>
                <div className="col-span-2">
                  <span className="rounded px-1.5 py-0.5 text-[10px] bg-brand/10 text-brand font-mono uppercase">{node.type || '?'}</span>
                </div>
                <div className="col-span-3">
                  <span className="text-xs font-mono text-muted truncate">{node.server}:{node.port}</span>
                </div>
                <div className="col-span-2 flex items-center justify-end">
                  <button className="btn-icon-sm btn-ghost" title="删除此导入组" onClick={() => handleStaticSubDelete(node.subId)}>
                    <Trash2 size={14} className="text-muted hover:text-red-400" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </SectionCard>

      {/* ── Worker Nodes ─────────────────────────────────────────────── */}
      <SectionCard
        title="Worker 代理节点"
        description="运行于 Cloudflare Workers 的 VLESS-WS 节点，无需 VPS，CF 账号即可使用"
        actions={
          <button className="btn-ghost h-7 px-2.5 text-xs flex items-center gap-1.5" onClick={() => setShowWorkerWizard(true)}>
            <Plus size={12} /> 新建
          </button>
        }
      >
        {workerNodes.length === 0 ? (
          <EmptyState
            title="暂无 Worker 节点"
            description="点击「新建」按钮部署一个基于 Cloudflare Workers 的 VLESS 代理节点，无需 VPS。"
            action={
              <button className="btn-primary" onClick={() => setShowWorkerWizard(true)}>
                <CloudCog size={14} /> 新建 Worker 节点
              </button>
            }
            icon={<CloudCog size={18} />}
          />
        ) : (
          <div className="table-shell">
            <div className="grid grid-cols-12 gap-3 px-4 py-3 table-header-row">
              <span className="col-span-3">名称</span>
              <span className="col-span-3">域名 / Dev URL</span>
              <span className="col-span-2">状态</span>
              <span className="col-span-4 text-right">操作</span>
            </div>
            {workerNodes.map(node => (
              <WorkerNodeCard
                key={node.id}
                node={node}
                onDelete={handleWorkerDelete}
                onRedeploy={handleWorkerRedeploy}
                onExport={handleWorkerExport}
              />
            ))}
          </div>
        )}
      </SectionCard>

      {/* Modals */}
      {showWorkerWizard && cfGlobal && (
        <WorkerNodeWizard
          cfConfig={cfGlobal}
          onClose={() => setShowWorkerWizard(false)}
          onCreated={() => { void loadWorkerNodes() }}
        />
      )}

      {showAzureWizard && (
        <AzureVMWizard
          azureConfig={azureGlobal}
          onSaveAzure={saveAzureGlobal}
          onClose={() => setShowAzureWizard(false)}
          onDone={loadNodes}
        />
      )}

      {showAzureConfigModal && (
        <AzureConfigModal
          initial={azureGlobal}
          save={saveAzureGlobal}
          onClose={() => setShowAzureConfigModal(false)}
          onSaved={() => setShowAzureConfigModal(false)}
        />
      )}

      {showWizard && (
        <NodeWizard
          initialNode={wizardInitialNode}
          cfConfig={cfGlobal}
          onSaveCF={saveCFGlobal}
          onClose={closeWizard}
          onDone={loadNodes}
          onOpenExport={node => { setExportNode(node); closeWizard() }}
        />
      )}

      {showCFModal && (
        <CFConfigModal
          initial={cfGlobal}
          save={saveCFGlobal}
          onClose={() => setShowCFModal(false)}
          onSaved={() => { setShowCFModal(false); void reloadCF() }}
        />
      )}

      {showEditModal && editNode && (
        <NodeEditModal
          node={editNode}
          onClose={() => { setShowEditModal(false); setEditNode(undefined) }}
          onSaved={loadNodes}
        />
      )}

      {progressNodeId && (
        <ProgressModal
          title={progressAction === 'deploy' ? '部署 GOST' : '销毁 GOST 部署'}
          nodeId={progressNodeId}
          action={progressAction}
          onClose={() => setProgressNodeId(null)}
          onDone={handleProgressDone}
        />
      )}

      {probeTargetNode && (
        <NodeProbeModal
          node={probeTargetNode}
          onClose={() => setProbeTargetNode(null)}
        />
      )}

      {exportNode && (
        <ExportModal
          node={exportNode}
          onClose={() => setExportNode(null)}
        />
      )}

      {workerExportNode && (
        <WorkerExportModal
          node={workerExportNode}
          onClose={() => setWorkerExportNode(null)}
        />
      )}

      {showImportModal && (
        <ImportClashModal
          onClose={() => setShowImportModal(false)}
          onImported={() => { void loadStaticSubs() }}
        />
      )}
    </CFGate>
  </div>
  )
}
