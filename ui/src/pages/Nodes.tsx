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
  Key,
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

// ── Node Form Modal ─────────────────────────────────────────────────────────

function NodeFormModal({
  node,
  onClose,
  onSaved,
}: {
  node?: NodeListItem
  onClose: () => void
  onSaved: () => void
}) {
  const isEdit = !!node
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [sshPubKey, setSSHPubKey] = useState('')
  const [copiedKey, setCopiedKey] = useState(false)
  const [copiedCmd, setCopiedCmd] = useState(false)
  const [copiedLocal, setCopiedLocal] = useState(false)
  const [form, setForm] = useState<NodeCreateRequest>({
    name: node?.name ?? '',
    host: node?.host ?? '',
    port: node?.port ?? 22,
    username: node?.username ?? 'root',
    password: '',
    domain: node?.domain ?? '',
    email: '',
    cf_token: '',
    cf_account_id: '',
    cf_zone_id: '',
  })

  const update = <K extends keyof NodeCreateRequest>(k: K, v: NodeCreateRequest[K]) =>
    setForm(f => ({ ...f, [k]: v }))

  useEffect(() => {
    getNodeSSHPubKey().then(r => setSSHPubKey(r.public_key)).catch(() => {})
  }, [])

  const authorizeCmd = form.host && form.username
    ? `ssh -p ${form.port} ${form.username}@${form.host} "echo '${sshPubKey}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys"`
    : ''

  const localCmd = sshPubKey
    ? `mkdir -p ~/.ssh && echo '${sshPubKey}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && chmod 700 ~/.ssh`
    : ''

  const copyText = async (text: string, setCopied: (v: boolean) => void) => {
    await navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const handleSave = async () => {
    setSaving(true)
    setError('')
    try {
      if (isEdit && node) {
        // Only send changed fields
        await updateNode(node.id, form)
      } else {
        await createNode(form)
      }
      onSaved()
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <ModalShell
      title={isEdit ? '编辑节点' : '新增节点'}
      description="添加一台远程 Linux 服务器"
      onClose={onClose}
      size="lg"
      dismissible={false}
      icon={<Server size={18} />}
    >
      <div className="space-y-4 max-h-[70vh] overflow-y-auto pr-1">
        {/* Server Info */}
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
              <input className="glass-input" value={form.username} onChange={e => update('username', e.target.value)} placeholder="root" />
            </div>
          </div>
          <div>
            <label className="block text-xs text-slate-400 mb-1">{isEdit ? '新密码 (留空不修改)' : 'SSH 密码 (可选)'}</label>
            <input className="glass-input" type="password" value={form.password} onChange={e => update('password', e.target.value)} placeholder={isEdit ? '留空则不修改' : '已授权 SSH Key 可留空'} />
          </div>
        </fieldset>

        <div className="divider" />

        {/* Router SSH Key Authorization */}
        <fieldset className="space-y-3">
          <legend className="text-xs font-semibold uppercase tracking-[0.2em] text-muted mb-2 flex items-center gap-1.5">
            <Key size={12} />
            路由器 SSH 公钥授权
          </legend>
          <p className="text-xs text-slate-400 leading-relaxed">
            如果你的电脑已有该服务器的 SSH 访问权限，在本地终端执行下方命令，将路由器公钥加入服务器授权列表，之后无需密码即可连接。
          </p>
          {sshPubKey && (
            <div className="space-y-2">
              <div className="flex items-start gap-2">
                <code className="flex-1 block rounded-lg bg-surface-2 border border-white/5 px-3 py-2 font-mono text-[11px] text-slate-300 break-all leading-relaxed">
                  {sshPubKey}
                </code>
                <button
                  type="button"
                  className="btn-ghost shrink-0 px-2 py-2"
                  onClick={() => copyText(sshPubKey, setCopiedKey)}
                  title="复制公钥"
                >
                  {copiedKey ? <Check size={13} className="text-emerald-400" /> : <Copy size={13} />}
                </button>
              </div>
              {/* 方式一：从本地电脑远程执行 */}
              <p className="text-[11px] text-slate-500 mt-1">方式一：在你的电脑终端执行（需本地已有服务器 SSH 权限）</p>
              {authorizeCmd ? (
                <div className="flex items-start gap-2">
                  <code className="flex-1 block rounded-lg bg-surface-2 border border-white/5 px-3 py-2 font-mono text-[11px] text-slate-300 break-all leading-relaxed">
                    {authorizeCmd}
                  </code>
                  <button
                    type="button"
                    className="btn-ghost shrink-0 px-2 py-2"
                    onClick={() => copyText(authorizeCmd, setCopiedCmd)}
                    title="复制命令"
                  >
                    {copiedCmd ? <Check size={13} className="text-emerald-400" /> : <Copy size={13} />}
                  </button>
                </div>
              ) : (
                <p className="text-[11px] text-muted">填写主机地址和用户名后自动生成</p>
              )}

              {/* 方式二：登录服务器后直接执行 */}
              <p className="text-[11px] text-slate-500 mt-1">方式二：SSH 登录目标服务器后，在服务器上直接执行</p>
              <div className="flex items-start gap-2">
                <code className="flex-1 block rounded-lg bg-surface-2 border border-white/5 px-3 py-2 font-mono text-[11px] text-slate-300 break-all leading-relaxed">
                  {localCmd}
                </code>
                <button
                  type="button"
                  className="btn-ghost shrink-0 px-2 py-2"
                  onClick={() => copyText(localCmd, setCopiedLocal)}
                  title="复制命令"
                >
                  {copiedLocal ? <Check size={13} className="text-emerald-400" /> : <Copy size={13} />}
                </button>
              </div>
            </div>
          )}
        </fieldset>

        <div className="divider" />

        {/* Domain & TLS */}
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

        <div className="divider" />

        {/* Cloudflare */}
        <fieldset className="space-y-3">
          <legend className="text-xs font-semibold uppercase tracking-[0.2em] text-muted mb-2">Cloudflare DNS API</legend>
          <div>
            <label className="block text-xs text-slate-400 mb-1">CF API Token</label>
            <input className="glass-input" type="password" value={form.cf_token} onChange={e => update('cf_token', e.target.value)} placeholder="Cloudflare API Token" />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-slate-400 mb-1">Account ID</label>
              <input className="glass-input font-mono text-xs" value={form.cf_account_id} onChange={e => update('cf_account_id', e.target.value)} />
            </div>
            <div>
              <label className="block text-xs text-slate-400 mb-1">Zone ID</label>
              <input className="glass-input font-mono text-xs" value={form.cf_zone_id} onChange={e => update('cf_zone_id', e.target.value)} />
            </div>
          </div>
        </fieldset>

        {error && (
          <div className="rounded-xl border border-red-500/20 bg-red-500/5 px-4 py-3 text-sm text-red-400">
            <AlertCircle size={14} className="inline mr-1.5 -mt-0.5" />
            {error}
          </div>
        )}

        <div className="flex items-center justify-end gap-3 pt-2">
          <button className="btn-ghost" onClick={onClose} disabled={saving}>取消</button>
          <button className="btn-primary" onClick={handleSave} disabled={saving || !form.name || !form.host}>
            {saving ? <Loader2 size={14} className="animate-spin" /> : <CheckCircle2 size={14} />}
            {isEdit ? '保存修改' : '添加节点'}
          </button>
        </div>
      </div>
    </ModalShell>
  )
}

// ── Deploy Wizard Modal ─────────────────────────────────────────────────────

function DeployWizardModal({
  node,
  onClose,
  onReload,
  onOpenExport,
}: {
  node: NodeListItem
  onClose: () => void
  onReload: () => void
  onOpenExport: () => void
}) {
  const [step, setStep] = useState(1)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [events, setEvents] = useState<SSEEvent[]>([])
  const [probeResults, setProbeResults] = useState<NodeProbeResult[]>([])
  const [zones, setZones] = useState<CloudflareZone[]>([])
  const [domainSuggestions, setDomainSuggestions] = useState<string[]>([])
  const [cfToken, setCFToken] = useState('')
  const [cfAccountId, setCFAccountId] = useState('')
  const [cfZoneId, setCFZoneId] = useState('')
  const [domain, setDomain] = useState(node.domain || '')
  const [email, setEmail] = useState('')
  const [domainProbe, setDomainProbe] = useState<NodeProbeResult[]>([])
  const [sshPubKey, setSSHPubKey] = useState('')
  const authorizeCmd = sshPubKey
    ? `ssh -p ${node.port} ${node.username}@${node.host} "echo '${sshPubKey}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys"`
    : ''

  useEffect(() => {
    getNodeSSHPubKey().then(r => setSSHPubKey(r.public_key)).catch(() => {})
  }, [])

  useEffect(() => {
    const secret = localStorage.getItem('cf_secret') || ''
    const raw = localStorage.getItem(WIZARD_CF_STORAGE_KEY)
    if (!raw) return
    decryptFromLocalStorage(raw, secret).then((plain) => {
      if (!plain) return
      try {
        const data = JSON.parse(plain) as { cf_token?: string; cf_account_id?: string; acme_email?: string }
        if (data.cf_token) setCFToken(data.cf_token)
        if (data.cf_account_id) setCFAccountId(data.cf_account_id)
        if (data.acme_email) setEmail(data.acme_email)
      } catch {
        // ignore
      }
    }).catch(() => {})
  }, [])

  const persistCF = async () => {
    const secret = localStorage.getItem('cf_secret') || ''
    const raw = JSON.stringify({ cf_token: cfToken, cf_account_id: cfAccountId, acme_email: email })
    const encrypted = await encryptForLocalStorage(raw, secret)
    localStorage.setItem(WIZARD_CF_STORAGE_KEY, encrypted)
    setMessage(`Cloudflare 凭据已本地加密存储，Token: ${maskSecret(cfToken)}`)
  }

  const runDeploy = async (mode: 'bootstrap' | 'full') => {
    setBusy(true)
    setEvents([])
    setMessage('')
    const secret = localStorage.getItem('cf_secret') || ''
    const abort = new AbortController()
    try {
      await streamSSE(
        `${BASE}/nodes/${encodeURIComponent(node.id)}/deploy`,
        secret,
        (ev) => setEvents(prev => [...prev, ev]),
        (d) => {
          if (!d.success) {
            setMessage(d.error || '部署失败')
            return
          }
          if (d.probe_results) setProbeResults(d.probe_results)
          if (mode === 'bootstrap') {
            setStep(3)
            setMessage('GOST 部署完成，IP直连探测已生成。')
          } else {
            setStep(6)
            setMessage(d.cert_issued ? '域名绑定与证书签发完成。' : '部署完成，但证书未签发。')
          }
          onReload()
        },
        abort.signal,
        { mode },
      )
    } catch (e) {
      setMessage(e instanceof Error ? e.message : '部署请求失败')
    } finally {
      setBusy(false)
    }
  }

  const runDomainProbe = async () => {
    setBusy(true)
    setMessage('')
    try {
      const data = await probeNode(node.id, 'domain')
      setDomainProbe(data.probe_results)
      if (data.summary.success) {
        setStep(7)
      }
      setMessage(`域名探测完成：${data.summary.ok}/${data.summary.total}`)
    } catch (e) {
      setMessage(e instanceof Error ? e.message : '域名探测失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <ModalShell title={`节点分步部署 · ${node.name}`} onClose={onClose} size="lg" dismissible={!busy}>
      <div className="space-y-4">
        <div className="text-xs text-muted">步骤 {step}/7</div>

        {step === 1 && (
          <div className="space-y-3">
            <p className="text-sm text-slate-300">1) 先完成 SSH Key 配置并校验连通性。</p>
            {sshPubKey && (
              <>
                <p className="text-xs text-slate-400">请把以下公钥写入目标服务器 ~/.ssh/authorized_keys：</p>
                <code className="block rounded-lg bg-surface-2 border border-white/5 px-3 py-2 font-mono text-[11px] text-slate-300 break-all">{sshPubKey}</code>
                <code className="block rounded-lg bg-surface-2 border border-white/5 px-3 py-2 font-mono text-[11px] text-slate-300 break-all">{authorizeCmd}</code>
              </>
            )}
            <button className="btn-primary" disabled={busy} onClick={async () => {
              setBusy(true)
              setMessage('')
              try {
                const r = await testNodeConnection(node.id)
                setMessage(r.ok ? 'SSH 校验成功，可进入下一步。' : r.message)
                if (r.ok) setStep(2)
              } catch (e) {
                setMessage(e instanceof Error ? e.message : 'SSH 校验失败')
              } finally { setBusy(false) }
            }}>
              {busy ? <Loader2 size={14} className="animate-spin" /> : <CheckCircle2 size={14} />} 校验 SSH Key & 连接
            </button>
          </div>
        )}

        {step === 2 && (
          <div className="space-y-3">
            <p className="text-sm text-slate-300">2) 部署 GOST（IP直连模式）并执行 Google/YouTube/GitHub 探测。</p>
            <button className="btn-primary" disabled={busy} onClick={() => runDeploy('bootstrap')}>
              {busy ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />} 开始部署并探测
            </button>
            {probeResults.length > 0 && (
              <div className="rounded-xl border border-white/10 p-3 text-xs space-y-1">
                {probeResults.map((p, idx) => <p key={idx} className={p.ok ? 'text-emerald-300' : 'text-red-300'}>{p.name}: {p.ok ? `OK (${p.status_code}, ${p.latency_ms}ms)` : p.error}</p>)}
              </div>
            )}
          </div>
        )}

        {step === 3 && (
          <div className="space-y-3">
            <p className="text-sm text-slate-300">3) 配置 Cloudflare API Token（显示 Mask，且本地加密保存）。</p>
            <input className="glass-input" placeholder="CF API Token" value={cfToken} onChange={e => setCFToken(e.target.value)} />
            <input className="glass-input" placeholder="CF Account ID (可选)" value={cfAccountId} onChange={e => setCFAccountId(e.target.value)} />
            <button className="btn-primary" disabled={!cfToken || busy} onClick={persistCF}><Check size={14} /> 保存到本地加密存储</button>
            <button className="btn-ghost" onClick={() => setStep(4)}>下一步</button>
          </div>
        )}

        {step === 4 && (
          <div className="space-y-3">
            <p className="text-sm text-slate-300">4) 获取 Cloudflare 域名，推荐常见前缀二级域名，并填写 ACME 邮箱。</p>
            <button className="btn-primary" disabled={busy || !cfToken} onClick={async () => {
              setBusy(true)
              try {
                const r = await getCloudflareZones({ cf_token: cfToken, cf_account_id: cfAccountId })
                setZones(r.zones)
                if (r.zones[0]?.name) setDomainSuggestions(suggestSubdomains(r.zones[0].name))
                setMessage(`获取到 ${r.zones.length} 个域名`) 
              } catch (e) {
                setMessage(e instanceof Error ? e.message : '获取域名失败')
              } finally { setBusy(false) }
            }}><RotateCw size={14} /> 拉取域名列表</button>
            <select className="glass-input" value={cfZoneId} onChange={e => {
              setCFZoneId(e.target.value)
              const z = zones.find(v => v.id === e.target.value)
              if (z) setDomainSuggestions(suggestSubdomains(z.name))
            }}>
              <option value="">选择 Zone</option>
              {zones.map(z => <option key={z.id} value={z.id}>{z.name}</option>)}
            </select>
            <div className="flex flex-wrap gap-2">
              {domainSuggestions.map(s => <button key={s} className="btn-ghost text-xs" onClick={() => setDomain(s)}>{s}</button>)}
            </div>
            <input className="glass-input" placeholder="手动输入二级域名" value={domain} onChange={e => setDomain(e.target.value)} />
            <input className="glass-input" type="email" placeholder="ACME 邮箱（真实邮箱）" value={email} onChange={e => setEmail(e.target.value)} />
            <button className="btn-primary" disabled={!domain || !email || !cfToken || busy} onClick={async () => {
              setBusy(true)
              try {
                await updateNode(node.id, { domain, email, cf_token: cfToken, cf_account_id: cfAccountId, cf_zone_id: cfZoneId })
                await persistCF()
                setStep(5)
              } catch (e) {
                setMessage(e instanceof Error ? e.message : '保存节点配置失败')
              } finally { setBusy(false) }
            }}><Check size={14} /> 保存并进入绑定步骤</button>
          </div>
        )}

        {step === 5 && (
          <div className="space-y-3">
            <p className="text-sm text-slate-300">5) 开始绑定域名 + 申请证书 + 验证。</p>
            <button className="btn-primary" disabled={busy} onClick={() => runDeploy('full')}>
              {busy ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />} 开始绑定与签证
            </button>
          </div>
        )}

        {step === 6 && (
          <div className="space-y-3">
            <p className="text-sm text-slate-300">6) 使用账号密码+域名执行 probe，验证协议链路。</p>
            <button className="btn-primary" disabled={busy} onClick={runDomainProbe}>{busy ? <Loader2 size={14} className="animate-spin" /> : <Terminal size={14} />} 执行最终探测</button>
            {domainProbe.length > 0 && (
              <div className="rounded-xl border border-white/10 p-3 text-xs space-y-1">
                {domainProbe.map((p, idx) => <p key={idx} className={p.ok ? 'text-emerald-300' : 'text-red-300'}>{p.name}: {p.ok ? `OK (${p.status_code}, ${p.latency_ms}ms)` : p.error}</p>)}
              </div>
            )}
          </div>
        )}

        {step === 7 && (
          <div className="space-y-3">
            <p className="text-sm text-emerald-300">7) 验证成功。现在可导出 Clash 配置并直接复制/导入。</p>
            <button className="btn-primary" onClick={onOpenExport}><Download size={14} /> 打开 Clash 配置</button>
          </div>
        )}

        {events.length > 0 && (
          <div className="max-h-40 overflow-y-auto rounded-xl border border-white/10 p-3 text-xs font-mono space-y-1">
            {events.map((ev, i) => <p key={i}>[{ev.step}] {ev.message}</p>)}
          </div>
        )}
        {message && <div className="text-xs text-slate-300">{message}</div>}
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
    await navigator.clipboard.writeText(yaml)
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
  const [showForm, setShowForm] = useState(false)
  const [editNode, setEditNode] = useState<NodeListItem | undefined>()
  const [progressNodeId, setProgressNodeId] = useState<string | null>(null)
  const [progressAction, setProgressAction] = useState<'deploy' | 'destroy'>('deploy')
  const [exportNode, setExportNode] = useState<NodeListItem | null>(null)
  const [deployWizardNode, setDeployWizardNode] = useState<NodeListItem | null>(null)
  const [deployOpen, setDeployOpen] = useState(false)
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

  const handleDestroy = (id: string) => {
    if (!confirm('确定要远程销毁 GOST 部署吗？\n\n这将停止服务、删除配置和证书，恢复服务器至部署前状态。')) return
    setProgressNodeId(id)
    setProgressAction('destroy')
    setDeployOpen(true)
  }

  const handleDeploy = (node: NodeListItem) => {
    setDeployWizardNode(node)
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
          <button className="btn-primary" onClick={() => { setEditNode(undefined); setShowForm(true) }}>
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
            <button className="btn-primary" onClick={() => { setEditNode(undefined); setShowForm(true) }}>
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

                  {/* Deploy */}
                  {node.status !== 'deployed' && (
                    <button className="btn-icon-sm btn-ghost" title="部署 GOST" onClick={() => handleDeploy(node)}>
                      <Play size={12} className="text-brand" />
                    </button>
                  )}

                  {/* Redeploy */}
                  {node.status === 'deployed' && (
                    <button className="btn-icon-sm btn-ghost" title="重新部署" onClick={() => handleDeploy(node)}>
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
                  <button className="btn-icon-sm btn-ghost" title="编辑" onClick={() => { setEditNode(node); setShowForm(true) }}>
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
      {showForm && (
        <NodeFormModal
          node={editNode}
          onClose={() => { setShowForm(false); setEditNode(undefined) }}
          onSaved={loadNodes}
        />
      )}

      {deployWizardNode && (
        <DeployWizardModal
          node={deployWizardNode}
          onReload={loadNodes}
          onClose={() => setDeployWizardNode(null)}
          onOpenExport={() => {
            setExportNode(deployWizardNode)
            setDeployWizardNode(null)
          }}
        />
      )}

      {deployOpen && progressNodeId && (
        <ProgressModal
          title={progressAction === 'deploy' ? '部署 GOST' : '销毁 GOST 部署'}
          nodeId={progressNodeId}
          action={progressAction}
          onClose={() => { setDeployOpen(false); setProgressNodeId(null) }}
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
