import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Server,
  Plus,
  CheckCircle2,
  AlertCircle,
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
} from '../api/client'
import type { NodeListItem, NodeCreateRequest, NodeProbeResult, CloudflareZone } from '../api/client'
import { PageHeader, SectionCard, ModalShell, EmptyState } from '../components/ui'

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

const WIZARD_CF_STORAGE_KEY = 'cf_nodes_wizard_v1'

function maskSecret(v: string) {
  if (!v) return ''
  if (v.length <= 8) return '*'.repeat(v.length)
  return `${v.slice(0, 4)}****${v.slice(-4)}`
}

function suggestSubdomains(zone: string) {
  const prefixes = ['market', 'sales', 'trials', 'blog', 'cdn', 'edge']
  return prefixes.map(p => `${p}-${Math.floor(Math.random() * 90 + 10)}.${zone}`)
}

async function encryptForLocalStorage(raw: string, secret: string) {
  const effectiveSecret = secret || 'clashforge-local-key'
  if (!window.crypto?.subtle) return raw
  const enc = new TextEncoder()
  const iv = window.crypto.getRandomValues(new Uint8Array(12))
  const keyMaterial = await window.crypto.subtle.importKey('raw', enc.encode(effectiveSecret), 'PBKDF2', false, ['deriveKey'])
  const key = await window.crypto.subtle.deriveKey(
    { name: 'PBKDF2', salt: enc.encode('cf-wizard-salt'), iterations: 100000, hash: 'SHA-256' },
    keyMaterial,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt'],
  )
  const cipher = await window.crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, enc.encode(raw))
  return `${btoa(String.fromCharCode(...iv))}.${btoa(String.fromCharCode(...new Uint8Array(cipher)))}`
}

async function decryptFromLocalStorage(payload: string, secret: string) {
  const effectiveSecret = secret || 'clashforge-local-key'
  if (!payload) return ''
  if (!payload.includes('.') || !window.crypto?.subtle) return payload
  const [ivB64, cipherB64] = payload.split('.')
  const iv = Uint8Array.from(atob(ivB64), c => c.charCodeAt(0))
  const cipher = Uint8Array.from(atob(cipherB64), c => c.charCodeAt(0))
  const enc = new TextEncoder()
  const dec = new TextDecoder()
  const keyMaterial = await window.crypto.subtle.importKey('raw', enc.encode(effectiveSecret), 'PBKDF2', false, ['deriveKey'])
  const key = await window.crypto.subtle.deriveKey(
    { name: 'PBKDF2', salt: enc.encode('cf-wizard-salt'), iterations: 100000, hash: 'SHA-256' },
    keyMaterial,
    { name: 'AES-GCM', length: 256 },
    false,
    ['decrypt'],
  )
  const plain = await window.crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, cipher)
  return dec.decode(plain)
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

function resumeStep(status: NodeListItem['status']): number {
  switch (status) {
    case 'connected': return 3
    case 'deploying':
    case 'error': return 3
    case 'deployed': return 7
    default: return 2
  }
}

function NodeWizard({
  initialNode,
  onClose,
  onDone,
  onOpenExport,
}: {
  initialNode?: NodeListItem
  onClose: () => void
  onDone: () => void
  onOpenExport: (node: NodeListItem) => void
}) {
  const [createdNode, setCreatedNode] = useState<NodeListItem | null>(initialNode ?? null)
  const [step, setStep] = useState(() => initialNode ? resumeStep(initialNode.status) : 1)
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

  // Step 4: CF credentials
  const [cfToken, setCFToken] = useState('')
  const [cfAccountId, setCFAccountId] = useState('')

  // Step 5: domain selection
  const [zones, setZones] = useState<CloudflareZone[]>([])
  const [cfZoneId, setCFZoneId] = useState('')
  const [domain, setDomain] = useState('')
  const [email, setEmail] = useState('')
  const [domainSuggestions, setDomainSuggestions] = useState<string[]>([])

  // Step 6: full deploy
  const [fullEvents, setFullEvents] = useState<SSEEvent[]>([])

  // Step 7: domain probe
  const [domainProbe, setDomainProbe] = useState<NodeProbeResult[]>([])

  useEffect(() => {
    const secret = localStorage.getItem('cf_secret') || ''
    const raw = localStorage.getItem(WIZARD_CF_STORAGE_KEY)
    if (!raw) return
    decryptFromLocalStorage(raw, secret).then(plain => {
      if (!plain) return
      try {
        const data = JSON.parse(plain) as { cf_token?: string; cf_account_id?: string; acme_email?: string }
        if (data.cf_token) setCFToken(data.cf_token)
        if (data.cf_account_id) setCFAccountId(data.cf_account_id)
        if (data.acme_email) setEmail(data.acme_email)
      } catch { /* ignore */ }
    }).catch(() => {})
  }, [])

  useEffect(() => {
    eventsRef.current?.scrollTo({ top: eventsRef.current.scrollHeight, behavior: 'smooth' })
  }, [events, fullEvents])

  const persistCF = async () => {
    const secret = localStorage.getItem('cf_secret') || ''
    const plain = JSON.stringify({ cf_token: cfToken, cf_account_id: cfAccountId, acme_email: email })
    const enc = await encryptForLocalStorage(plain, secret)
    localStorage.setItem(WIZARD_CF_STORAGE_KEY, enc)
  }

  const node = createdNode

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
    setBusy(true); setEvents([]); setMessage(null)
    const secret = localStorage.getItem('cf_secret') || ''
    try {
      await streamSSE(
        `${BASE}/nodes/${encodeURIComponent(node.id)}/deploy`, secret,
        ev => setEvents(prev => [...prev, ev]),
        d => {
          if (!d.success) { setMessage({ text: d.error || '部署失败', ok: false }); return }
          if (d.probe_results) setBootstrapProbe(d.probe_results)
          setMessage({ text: 'GOST 部署完成，IP 直连探测通过', ok: true })
          setStep(4)
          onDone()
        },
        undefined,
        { mode: 'bootstrap' },
      )
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : '部署失败', ok: false })
    } finally { setBusy(false) }
  }

  const handleSaveCF = async () => {
    setBusy(true); setMessage(null)
    try {
      await persistCF()
      setMessage({ text: `凭据已加密保存，Token: ${maskSecret(cfToken)}`, ok: true })
    } catch { setMessage({ text: '保存失败', ok: false }) }
    finally { setBusy(false) }
  }

  const handleFetchZones = async () => {
    setBusy(true); setMessage(null)
    try {
      const r = await getCloudflareZones({ cf_token: cfToken, cf_account_id: cfAccountId })
      setZones(r.zones)
      if (r.zones[0]?.name) setDomainSuggestions(suggestSubdomains(r.zones[0].name))
      setMessage({ text: `获取到 ${r.zones.length} 个 Zone`, ok: true })
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : '获取失败', ok: false })
    } finally { setBusy(false) }
  }

  const handleSaveDomain = async () => {
    if (!node) return
    setBusy(true); setMessage(null)
    try {
      await updateNode(node.id, { domain, email, cf_token: cfToken, cf_account_id: cfAccountId, cf_zone_id: cfZoneId })
      await persistCF()
      setStep(6)
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : '保存失败', ok: false })
    } finally { setBusy(false) }
  }

  const handleFullDeploy = async () => {
    if (!node) return
    setBusy(true); setFullEvents([]); setMessage(null)
    const secret = localStorage.getItem('cf_secret') || ''
    try {
      await streamSSE(
        `${BASE}/nodes/${encodeURIComponent(node.id)}/deploy`, secret,
        ev => setFullEvents(prev => [...prev, ev]),
        d => {
          if (!d.success) { setMessage({ text: d.error || '部署失败', ok: false }); return }
          setMessage({ text: d.cert_issued ? '域名绑定与证书签发完成' : '部署完成（证书未签发）', ok: !!d.cert_issued })
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
        <p key={i} className={ev.status === 'error' ? 'text-red-300' : ev.status === 'ok' ? 'text-emerald-300' : 'text-slate-400'}>
          [{ev.step}] {ev.message}
        </p>
      ))}
    </div>
  )

  const stepTitles = ['服务器信息', 'SSH 公钥授权', '部署 GOST', 'Cloudflare 凭据', '选择域名', '绑定 + 签证', '完成']

  return (
    <ModalShell
      title={`${initialNode ? '继续部署' : '新增节点'} · ${stepTitles[step - 1]}`}
      description={node ? `${node.username}@${node.host}` : '添加一台远程 Linux 服务器'}
      onClose={!busy ? onClose : undefined}
      size="lg"
      dismissible={false}
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
            </div>
            {(busy || events.length > 0) && <EventLog evs={events} />}
            {bootstrapProbe.length > 0 && <ProbeResultList results={bootstrapProbe} />}
          </div>
        )}

        {/* ── Step 4: Cloudflare 凭据 ── */}
        {step === 4 && (
          <div className="space-y-3">
            <p className="text-xs text-slate-400 leading-relaxed">输入 Cloudflare API Token，凭据将本地加密保存，用于后续自动申请 TLS 证书。</p>
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
            <p className="text-xs text-slate-400 leading-relaxed">将为 <span className="text-slate-300 font-mono">{domain}</span> 创建 DNS A 记录，并通过 ACME 签发 TLS 证书，然后重启 GOST 以域名模式运行。</p>
            <div className="flex gap-3">
              {canGoBack && <button className="btn-ghost shrink-0" onClick={goBack} disabled={busy}>← 上一步</button>}
              <button className="btn-primary flex-1" disabled={busy} onClick={handleFullDeploy}>
                {busy ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
                {busy ? '绑定中，请等待…' : '开始绑定与签证'}
              </button>
            </div>
            {(busy || fullEvents.length > 0) && <EventLog evs={fullEvents} />}
          </div>
        )}

        {/* ── Step 7: 最终探测 + 完成 ── */}
        {step === 7 && (
          <div className="space-y-3">
            <div className="flex items-center gap-2 text-emerald-400">
              <CheckCircle2 size={16} />
              <p className="text-sm font-semibold">部署完成</p>
            </div>
            <p className="text-xs text-slate-400">执行域名链路探测验证节点可用性，然后导出 Clash 配置。</p>
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

// ── Export Modal ─────────────────────────────────────────────────────────────

function ExportModal({ node, onClose }: { node: NodeListItem; onClose: () => void }) {
  const [yaml, setYaml] = useState('')
  const [loading, setLoading] = useState(true)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    const secret = localStorage.getItem('cf_secret') || ''
    fetch(`${BASE}/nodes/${encodeURIComponent(node.id)}/proxy-config`, {
      headers: { Authorization: `Bearer ${secret}` },
    })
      .then(r => r.text())
      .then(setYaml)
      .finally(() => setLoading(false))
  }, [node.id])

  const handleCopy = async () => {
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

// ── Main Page ────────────────────────────────────────────────────────────────

export function Nodes() {
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
  const [testLoading, setTestLoading] = useState<Record<string, boolean>>({})

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
  const deployedNodes = nodes.filter(n => n.status === 'deployed').length

  return (
    <div className="space-y-6 animate-fade-in">
      <PageHeader
        eyebrow="节点管理"
        title="节点服务器"
        description="管理远程 Linux 服务器，部署 GOST 代理 + TLS 证书，导出 Clash 配置文件"
        metrics={[
          { label: '节点总数', value: String(totalNodes) },
          { label: '已连接', value: String(connectedNodes) },
          { label: '已部署', value: String(deployedNodes) },
        ]}
        actions={
          <button className="btn-primary" onClick={() => openWizard()}>
            <Plus size={14} /> 新增节点
          </button>
        }
      />

      {error && (
        <div className="rounded-2xl border border-red-500/20 bg-red-500/5 px-4 py-3 text-sm text-red-400 flex items-center gap-2">
          <AlertCircle size={14} /> {error}
        </div>
      )}

      {loading ? (
        <div className="flex items-center justify-center py-20">
          <Loader2 size={24} className="animate-spin text-brand" />
        </div>
      ) : nodes.length === 0 ? (
        <EmptyState
          title="暂无节点"
          description="添加一台远程 Linux 服务器，ClashForge 将自动部署 GOST 代理并签发 TLS 证书。"
          action={
            <button className="btn-primary" onClick={() => openWizard()}>
              <Plus size={14} /> 添加第一台服务器
            </button>
          }
          icon={<Server size={18} />}
        />
      ) : (
        <SectionCard>
          <div className="table-shell">
            {/* Table header */}
            <div className="grid grid-cols-12 gap-3 px-4 py-3 table-header-row">
              <span className="col-span-3">名称</span>
              <span className="col-span-2">主机</span>
              <span className="col-span-2">域名</span>
              <span className="col-span-2">状态</span>
              <span className="col-span-3 text-right">操作</span>
            </div>
            {/* Rows */}
            {nodes.map(node => (
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
                <div className="col-span-2">
                  <StatusBadge status={node.status} />
                  {node.error && node.status === 'error' && (
                    <p className="text-[10px] text-red-400/70 mt-1 truncate max-w-[160px]" title={node.error}>{node.error}</p>
                  )}
                </div>
                <div className="col-span-3 flex items-center justify-end gap-1.5">
                  {/* Test connection */}
                  <button
                    className="btn-icon-sm btn-ghost"
                    title="测试连接"
                    onClick={() => handleTest(node.id)}
                    disabled={testLoading[node.id]}
                  >
                    {testLoading[node.id] ? <Loader2 size={12} className="animate-spin" /> : <Terminal size={12} />}
                  </button>

                  {/* Deploy / resume wizard for non-deployed nodes */}
                  {node.status !== 'deployed' && (
                    <button className="btn-icon-sm btn-ghost" title="继续部署向导" onClick={() => openWizard(node)}>
                      <Play size={12} className="text-brand" />
                    </button>
                  )}

                  {/* Redeploy for deployed nodes */}
                  {node.status === 'deployed' && (
                    <button className="btn-icon-sm btn-ghost" title="重新部署" onClick={() => handleDeploy(node.id)}>
                      <RotateCw size={12} className="text-amber-400" />
                    </button>
                  )}

                  {/* Destroy */}
                  {node.status === 'deployed' && (
                    <button className="btn-icon-sm btn-ghost" title="销毁部署" onClick={() => handleDestroy(node.id)}>
                      <ShieldOff size={12} className="text-red-400" />
                    </button>
                  )}

                  {/* Export */}
                  {node.status === 'deployed' && (
                    <button className="btn-icon-sm btn-ghost" title="导出配置" onClick={() => setExportNode(node)}>
                      <Download size={12} className="text-emerald-400" />
                    </button>
                  )}

                  {/* Edit */}
                  <button className="btn-icon-sm btn-ghost" title="编辑" onClick={() => {
                    if (node.status !== 'deployed') { openWizard(node) }
                    else { setEditNode(node); setShowEditModal(true) }
                  }}>
                    <Pencil size={12} />
                  </button>

                  {/* Delete */}
                  <button className="btn-icon-sm btn-ghost" title="删除" onClick={() => handleDelete(node.id)}>
                    <Trash2 size={12} className="text-muted hover:text-red-400" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        </SectionCard>
      )}

      {/* Modals */}
      {showWizard && (
        <NodeWizard
          initialNode={wizardInitialNode}
          onClose={closeWizard}
          onDone={loadNodes}
          onOpenExport={node => { setExportNode(node); closeWizard() }}
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

      {exportNode && (
        <ExportModal
          node={exportNode}
          onClose={() => setExportNode(null)}
        />
      )}
    </div>
  )
}
