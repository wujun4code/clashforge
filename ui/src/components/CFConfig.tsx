import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { AlertCircle, CheckCircle2, CloudCog, Eye, EyeOff, Loader2 } from 'lucide-react'
import { ModalShell } from './ui'

// ── crypto helpers ────────────────────────────────────────────────────────────

export async function encryptForLocalStorage(raw: string, secret: string): Promise<string> {
  const eff = secret || 'clashforge-local-key'
  if (!window.crypto?.subtle) return raw
  const enc = new TextEncoder()
  const iv = window.crypto.getRandomValues(new Uint8Array(12))
  const km = await window.crypto.subtle.importKey('raw', enc.encode(eff), 'PBKDF2', false, ['deriveKey'])
  const key = await window.crypto.subtle.deriveKey(
    { name: 'PBKDF2', salt: enc.encode('cf-wizard-salt'), iterations: 100000, hash: 'SHA-256' },
    km, { name: 'AES-GCM', length: 256 }, false, ['encrypt'],
  )
  const cipher = await window.crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, enc.encode(raw))
  return `${btoa(String.fromCharCode(...iv))}.${btoa(String.fromCharCode(...new Uint8Array(cipher)))}`
}

export async function decryptFromLocalStorage(payload: string, secret: string): Promise<string> {
  const eff = secret || 'clashforge-local-key'
  if (!payload) return ''
  if (!payload.includes('.') || !window.crypto?.subtle) return payload
  const [ivB64, cipherB64] = payload.split('.')
  const iv = Uint8Array.from(atob(ivB64), c => c.charCodeAt(0))
  const cipher = Uint8Array.from(atob(cipherB64), c => c.charCodeAt(0))
  const enc = new TextEncoder()
  const dec = new TextDecoder()
  const km = await window.crypto.subtle.importKey('raw', enc.encode(eff), 'PBKDF2', false, ['deriveKey'])
  const key = await window.crypto.subtle.deriveKey(
    { name: 'PBKDF2', salt: enc.encode('cf-wizard-salt'), iterations: 100000, hash: 'SHA-256' },
    km, { name: 'AES-GCM', length: 256 }, false, ['decrypt'],
  )
  return dec.decode(await window.crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, cipher))
}

export function maskSecret(v: string) {
  if (!v) return ''
  if (v.length <= 8) return '*'.repeat(v.length)
  return `${v.slice(0, 4)}****${v.slice(-4)}`
}

// ── types & storage keys ──────────────────────────────────────────────────────

export interface CFConfig {
  cf_token: string
  cf_account_id: string
  acme_email: string
}

const CF_CONFIG_KEY = 'cf_global_config_v1'
const CF_LEGACY_KEY = 'cf_nodes_wizard_v1'

interface CFPermissionRow {
  scope: 'Zone' | 'Account'
  permission: string
  access: 'Read' | 'Edit'
  usedFor: string
}

const CF_PERMISSION_ROWS: CFPermissionRow[] = [
  { scope: 'Zone', permission: 'DNS', access: 'Edit', usedFor: '节点部署时写入 DNS 记录' },
  { scope: 'Zone', permission: 'Zone', access: 'Read', usedFor: '拉取 Zone 列表与域名校验' },
  { scope: 'Account', permission: 'Workers Scripts', access: 'Edit', usedFor: '发布订阅时部署/更新 Worker' },
  { scope: 'Account', permission: 'Workers KV Storage', access: 'Edit', usedFor: '创建 Namespace 与写入订阅内容' },
]

export function CFPermissionTable({ compact = false, className = '' }: { compact?: boolean; className?: string }) {
  return (
    <div className={`rounded-xl border border-white/10 bg-black/20 p-3 ${className}`}>
      {!compact && (
        <div className="mb-2.5">
          <p className="text-xs font-semibold text-slate-200">推荐权限示例（Custom Token）</p>
          <p className="mt-0.5 text-[11px] text-slate-400">按下表配置后，可同时覆盖节点部署与订阅发布两条流程。</p>
        </div>
      )}
      <div className="overflow-x-auto">
        <table className={`w-full min-w-[560px] border-separate border-spacing-0 ${compact ? 'text-[11px]' : 'text-xs'}`}>
          <thead>
            <tr>
              <th className="rounded-tl-lg border border-white/10 bg-white/5 px-2.5 py-2 text-left font-semibold text-slate-200">类型</th>
              <th className="border border-white/10 bg-white/5 px-2.5 py-2 text-left font-semibold text-slate-200">权限</th>
              <th className="border border-white/10 bg-white/5 px-2.5 py-2 text-left font-semibold text-slate-200">访问级别</th>
              <th className="rounded-tr-lg border border-white/10 bg-white/5 px-2.5 py-2 text-left font-semibold text-slate-200">用途</th>
            </tr>
          </thead>
          <tbody>
            {CF_PERMISSION_ROWS.map((row, idx) => (
              <tr key={`${row.scope}-${row.permission}`}>
                <td className={`border border-white/10 px-2.5 py-2 text-slate-300 ${idx === CF_PERMISSION_ROWS.length - 1 ? 'rounded-bl-lg' : ''}`}>
                  {row.scope}
                </td>
                <td className="border border-white/10 px-2.5 py-2 text-slate-100">{row.permission}</td>
                <td className="border border-white/10 px-2.5 py-2">
                  <span className="inline-flex rounded-md border border-brand/35 bg-brand/10 px-1.5 py-0.5 font-medium text-brand-light">
                    {row.access}
                  </span>
                </td>
                <td className={`border border-white/10 px-2.5 py-2 text-slate-300 ${idx === CF_PERMISSION_ROWS.length - 1 ? 'rounded-br-lg' : ''}`}>
                  {row.usedFor}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ── hook ──────────────────────────────────────────────────────────────────────

export function useCFConfig() {
  const [config, setConfig] = useState<CFConfig | null>(null)
  const [loading, setLoading] = useState(true)

  const reload = useCallback(async () => {
    setLoading(true)
    const secret = localStorage.getItem('cf_secret') || ''
    let raw = localStorage.getItem(CF_CONFIG_KEY)

    // One-time migration from the old per-wizard key
    if (!raw) {
      const legacy = localStorage.getItem(CF_LEGACY_KEY)
      if (legacy) {
        try {
          const plain = await decryptFromLocalStorage(legacy, secret)
          const d = JSON.parse(plain) as Partial<CFConfig>
          if (d.cf_token) {
            const enc = await encryptForLocalStorage(
              JSON.stringify({ cf_token: d.cf_token, cf_account_id: d.cf_account_id ?? '', acme_email: d.acme_email ?? '' }),
              secret,
            )
            localStorage.setItem(CF_CONFIG_KEY, enc)
            localStorage.removeItem(CF_LEGACY_KEY)
            raw = enc
          }
        } catch { /* ignore migration failure */ }
      }
    }

    if (!raw) { setConfig(null); setLoading(false); return }

    try {
      const plain = await decryptFromLocalStorage(raw, secret)
      const d = JSON.parse(plain) as Partial<CFConfig>
      setConfig({ cf_token: d.cf_token ?? '', cf_account_id: d.cf_account_id ?? '', acme_email: d.acme_email ?? '' })
    } catch { setConfig(null) }
    setLoading(false)
  }, [])

  useEffect(() => { void reload() }, [reload])

  const save = useCallback(async (cfg: CFConfig) => {
    const secret = localStorage.getItem('cf_secret') || ''
    const enc = await encryptForLocalStorage(JSON.stringify(cfg), secret)
    localStorage.setItem(CF_CONFIG_KEY, enc)
    setConfig(cfg)
  }, [])

  const clear = useCallback(() => {
    localStorage.removeItem(CF_CONFIG_KEY)
    setConfig(null)
  }, [])

  return { config, loading, isConfigured: Boolean(config?.cf_token && config?.cf_account_id), save, clear, reload }
}

// ── CFConfigModal ─────────────────────────────────────────────────────────────

export function CFConfigModal({
  initial,
  save,
  onClose,
  onSaved,
}: {
  initial?: CFConfig | null
  save: (cfg: CFConfig) => Promise<void>
  onClose: () => void
  onSaved?: () => void
}) {
  const [form, setForm] = useState<CFConfig>({
    cf_token: initial?.cf_token ?? '',
    cf_account_id: initial?.cf_account_id ?? '',
    acme_email: initial?.acme_email ?? '',
  })
  const [busy, setBusy] = useState(false)
  const [showToken, setShowToken] = useState(false)
  const [error, setError] = useState('')
  const initialToken = initial?.cf_token ?? ''
  const initialAccountID = initial?.cf_account_id ?? ''
  const initialAcmeEmail = initial?.acme_email ?? ''
  const isDirty =
    form.cf_token !== initialToken
    || form.cf_account_id !== initialAccountID
    || form.acme_email !== initialAcmeEmail

  const update = <K extends keyof CFConfig>(k: K, v: CFConfig[K]) =>
    setForm(f => ({ ...f, [k]: v }))

  const requestClose = () => {
    if (busy) return
    if (isDirty) {
      const confirmed = window.confirm('你有未保存的 Cloudflare 凭据修改，确定放弃并关闭吗？')
      if (!confirmed) return
    }
    onClose()
  }

  const handleSave = async () => {
    if (!form.cf_token.trim()) { setError('API Token 不能为空'); return }
    if (!form.cf_account_id.trim()) { setError('Account ID 不能为空'); return }
    setBusy(true); setError('')
    try {
      await save(form)
      onSaved?.()
      onClose()
    } catch { setError('保存失败') }
    finally { setBusy(false) }
  }

  return (
    <ModalShell
      title="Cloudflare 凭据配置"
      description="一次配置，中继节点部署与订阅分发均可共用"
      icon={<CloudCog size={18} />}
      onClose={requestClose}
      size="xl"
      dismissible={!busy}
    >
      <div className="space-y-4">
        <div className="rounded-xl border border-white/10 bg-black/20 p-3 text-xs text-slate-300 space-y-2">
          <p className="font-semibold text-slate-200">获取 Cloudflare API Token 和 Account ID</p>
          <ol className="list-decimal space-y-1 pl-4 text-[11px] leading-5">
            <li>前往 <a href="https://dash.cloudflare.com/profile/api-tokens" target="_blank" rel="noreferrer" className="text-brand-light hover:underline">API Tokens</a>，点击 <strong>Create Token</strong>。</li>
            <li>选择 <strong>Create Custom Token</strong>，按下方权限表示例添加 4 项权限。</li>
            <li>Zone Resources 选择要管理的域名，Account Resources 选择当前账号。</li>
            <li>创建后复制 Token（仅显示一次），并在控制台右侧复制 Account ID。</li>
          </ol>
          <CFPermissionTable compact />
          <div className="flex gap-2 pt-0.5">
            <a href="https://dash.cloudflare.com/profile/api-tokens" target="_blank" rel="noreferrer" className="btn-ghost h-7 px-2.5 text-xs">API Tokens</a>
            <a href="https://dash.cloudflare.com/" target="_blank" rel="noreferrer" className="btn-ghost h-7 px-2.5 text-xs">CF 控制台</a>
            <a href="https://developers.cloudflare.com/fundamentals/api/reference/permissions/" target="_blank" rel="noreferrer" className="btn-ghost h-7 px-2.5 text-xs">权限文档</a>
          </div>
        </div>

        <div className="space-y-3">
          <div>
            <label className="block text-xs text-slate-400 mb-1">CF API Token <span className="text-red-400">*</span></label>
            <div className="flex gap-2">
              <input
                className="glass-input flex-1"
                type={showToken ? 'text' : 'password'}
                value={form.cf_token}
                onChange={e => update('cf_token', e.target.value)}
                placeholder="Cloudflare API Token"
                autoFocus
              />
              <button type="button" className="btn-ghost px-2.5" onClick={() => setShowToken(v => !v)}>
                {showToken ? <EyeOff size={14} /> : <Eye size={14} />}
              </button>
            </div>
          </div>
          <div>
            <label className="block text-xs text-slate-400 mb-1">
              Account ID <span className="text-red-400">*</span>
            </label>
            <input
              className="glass-input font-mono text-xs"
              value={form.cf_account_id}
              onChange={e => update('cf_account_id', e.target.value)}
              placeholder="从 CF 控制台复制"
            />
          </div>
          <div>
            <label className="block text-xs text-slate-400 mb-1">
              ACME 邮箱 <span className="text-muted">（Let's Encrypt，节点证书签发需要）</span>
            </label>
            <input
              className="glass-input"
              type="email"
              value={form.acme_email}
              onChange={e => update('acme_email', e.target.value)}
              placeholder="admin@example.com"
            />
          </div>
        </div>

        {error && (
          <div className="flex items-center gap-2 rounded-xl border border-red-500/20 bg-red-500/5 px-3 py-2 text-xs text-red-400">
            <AlertCircle size={12} className="shrink-0" />{error}
          </div>
        )}

        <div className="flex items-center justify-end gap-3 pt-1">
          <button className="btn-ghost" onClick={requestClose} disabled={busy}>取消</button>
          <button className="btn-primary" onClick={handleSave} disabled={busy || !form.cf_token.trim() || !form.cf_account_id.trim()}>
            {busy ? <Loader2 size={14} className="animate-spin" /> : <CheckCircle2 size={14} />}
            保存凭据
          </button>
        </div>
      </div>
    </ModalShell>
  )
}

// ── CFGate ────────────────────────────────────────────────────────────────────

export function CFGate({
  config,
  loading,
  save,
  children,
}: {
  config: CFConfig | null
  loading: boolean
  save: (cfg: CFConfig) => Promise<void>
  children: ReactNode
}) {
  const [showSetup, setShowSetup] = useState(false)

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20">
        <Loader2 size={20} className="animate-spin text-brand" />
      </div>
    )
  }

  if (!config?.cf_token || !config?.cf_account_id) {
    return (
      <>
        <div className="flex flex-col items-center justify-center gap-6 rounded-2xl border border-white/[0.07] bg-white/[0.02] px-8 py-16 text-center">
          <div
            className="flex h-16 w-16 items-center justify-center rounded-2xl border border-brand/20 bg-brand/[0.08]"
            style={{ boxShadow: 'var(--shadow-glow-brand-sm)' }}
          >
            <CloudCog size={28} className="text-brand-light" />
          </div>
          <div className="space-y-2">
            <h2 className="text-xl font-semibold text-white">需要配置 Cloudflare 凭据</h2>
            <p className="max-w-md text-sm text-white/45 leading-relaxed">
              中继节点部署（DNS 绑定 · TLS 证书签发）和订阅分发（Worker · KV 托管）均通过 Cloudflare API 完成，请先完成一次性配置。
            </p>
          </div>
          <button className="btn-primary px-6 py-2.5 text-sm flex items-center gap-2" onClick={() => setShowSetup(true)}>
            <CloudCog size={15} /> 配置 Cloudflare 凭据
          </button>
          <div className="w-full max-w-3xl text-left">
            <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-white/30">所需权限</p>
            <CFPermissionTable compact />
          </div>
        </div>
        {showSetup && (
          <CFConfigModal save={save} onClose={() => setShowSetup(false)} onSaved={() => setShowSetup(false)} />
        )}
      </>
    )
  }

  return <>{children}</>
}

// ── CFConfigBanner ─────────────────────────────────────────────────────────────

export function CFConfigBanner({
  config,
  loading,
  onConfigure,
}: {
  config: CFConfig | null
  loading: boolean
  onConfigure: () => void
}) {
  if (loading) return null

  if (config?.cf_token && config?.cf_account_id) {
    return (
      <div className="flex items-center gap-3 rounded-xl border border-emerald-500/15 bg-emerald-500/[0.04] px-4 py-2.5 text-xs">
        <CheckCircle2 size={13} className="text-emerald-400 shrink-0" />
        <span className="flex-1 text-emerald-300/80">Cloudflare 凭据已配置，节点部署与订阅分发共用</span>
        {config.cf_account_id && (
          <span className="font-mono text-[10px] text-muted">
            Account: {maskSecret(config.cf_account_id)}
          </span>
        )}
        <button className="btn-ghost h-6 px-2.5 text-[11px] shrink-0 ml-2" onClick={onConfigure}>
          修改
        </button>
      </div>
    )
  }

  return (
    <div className="flex items-start gap-3 rounded-xl border border-amber-500/20 bg-amber-500/[0.04] px-4 py-3">
      <AlertCircle size={14} className="text-amber-400 shrink-0 mt-0.5" />
      <div className="flex-1 min-w-0">
        <p className="text-sm font-semibold text-amber-300">Cloudflare 凭据未配置</p>
        <p className="text-xs text-amber-300/55 mt-0.5 leading-relaxed">
          DNS 绑定、TLS 证书签发及订阅分发功能需要 Cloudflare API Token 和 Account ID。
        </p>
      </div>
      <button className="btn-primary h-8 px-3 text-xs shrink-0 self-center" onClick={onConfigure}>
        立即配置
      </button>
    </div>
  )
}
