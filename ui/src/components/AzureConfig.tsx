import { useCallback, useEffect, useState } from 'react'
import { AlertCircle, CheckCircle2, Cloud, Eye, EyeOff, Loader2 } from 'lucide-react'
import { ModalShell } from './ui'

export interface AzureConfig {
  tenant_id: string
  client_id: string
  client_secret: string
  subscription_id: string
}

const BASE = '/api/v1'

async function apiFetch<T>(method: string, body?: unknown): Promise<T> {
  const secret = localStorage.getItem('cf_secret') || ''
  const res = await fetch(`${BASE}/azure-config`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(secret ? { Authorization: `Bearer ${secret}` } : {}),
    },
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
  })
  if (res.status === 204) return undefined as T
  const json = await res.json()
  if (!json.ok) throw new Error(json.error?.message ?? `HTTP ${res.status}`)
  return json.data as T
}

// ── hook ──────────────────────────────────────────────────────────────────────

export function useAzureConfig() {
  const [config, setConfig] = useState<Omit<AzureConfig, 'client_secret'> | null>(null)
  const [loading, setLoading] = useState(true)

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const data = await apiFetch<AzureConfig>('GET')
      setConfig(data.tenant_id ? data : null)
    } catch {
      setConfig(null)
    }
    setLoading(false)
  }, [])

  useEffect(() => { void reload() }, [reload])

  const save = useCallback(async (cfg: AzureConfig) => {
    const saved = await apiFetch<AzureConfig>('PUT', cfg)
    setConfig(saved.tenant_id ? saved : null)
  }, [])

  const clear = useCallback(async () => {
    await apiFetch<void>('DELETE')
    setConfig(null)
  }, [])

  return {
    config,
    loading,
    isConfigured: Boolean(config?.tenant_id && config?.client_id && config?.subscription_id),
    save,
    clear,
    reload,
  }
}

// ── AzureConfigModal ──────────────────────────────────────────────────────────

export function AzureConfigModal({
  initial,
  save,
  onClose,
  onSaved,
}: {
  initial?: Omit<AzureConfig, 'client_secret'> | null
  save: (cfg: AzureConfig) => Promise<void>
  onClose: () => void
  onSaved?: () => void
}) {
  const [form, setForm] = useState<AzureConfig>({
    tenant_id: initial?.tenant_id ?? '',
    client_id: initial?.client_id ?? '',
    client_secret: '',
    subscription_id: initial?.subscription_id ?? '',
  })
  const [busy, setBusy] = useState(false)
  const [showSecret, setShowSecret] = useState(false)
  const [error, setError] = useState('')

  const isEditing = Boolean(initial?.tenant_id)

  const update = <K extends keyof AzureConfig>(k: K, v: AzureConfig[K]) =>
    setForm(f => ({ ...f, [k]: v }))

  const handleSave = async () => {
    if (!form.tenant_id.trim()) { setError('Tenant ID 不能为空'); return }
    if (!form.client_id.trim()) { setError('Client ID (Application ID) 不能为空'); return }
    if (!isEditing && !form.client_secret.trim()) { setError('Client Secret 不能为空'); return }
    if (!form.subscription_id.trim()) { setError('Subscription ID 不能为空'); return }
    setBusy(true); setError('')
    try {
      await save(form)
      onSaved?.()
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <ModalShell
      title="Azure 凭据配置"
      description="Service Principal 凭据，用于通过 Azure API 开机云主机"
      icon={<Cloud size={18} />}
      onClose={onClose}
      size="xl"
      dismissible={!busy}
    >
      <div className="space-y-4">
        {/* Guide */}
        <div className="rounded-xl border border-white/10 bg-black/20 p-3 text-xs text-slate-300 space-y-2">
          <p className="font-semibold text-slate-200">如何获取 Azure Service Principal 凭据</p>
          <ol className="list-decimal space-y-1 pl-4 text-[11px] leading-5">
            <li>
              在 Azure Portal →{' '}
              <a href="https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade" target="_blank" rel="noreferrer" className="text-sky-400 hover:underline">
                App registrations
              </a>{' '}
              → 点击 <strong>New registration</strong>，填写名称（如 <code>clashforge</code>），注册。
            </li>
            <li>注册完成后复制页面顶部的 <strong>Application (client) ID</strong> 和 <strong>Directory (tenant) ID</strong>。</li>
            <li>进入 <strong>Certificates &amp; secrets</strong> → <strong>New client secret</strong>，复制生成的 Secret Value（仅显示一次）。</li>
            <li>
              前往{' '}
              <a href="https://portal.azure.com/#view/Microsoft_Azure_Billing/SubscriptionsBlade" target="_blank" rel="noreferrer" className="text-sky-400 hover:underline">
                Subscriptions
              </a>
              {' '}复制要使用的订阅 ID。
            </li>
            <li>
              在订阅的 <strong>Access control (IAM)</strong> → <strong>Add role assignment</strong> → 为应用分配 <strong>Contributor</strong> 角色（或 Virtual Machine Contributor + Network Contributor）。
            </li>
          </ol>
          <div className="flex gap-2 pt-0.5">
            <a href="https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade" target="_blank" rel="noreferrer" className="btn-ghost h-7 px-2.5 text-xs">App Registrations</a>
            <a href="https://portal.azure.com/#view/Microsoft_Azure_Billing/SubscriptionsBlade" target="_blank" rel="noreferrer" className="btn-ghost h-7 px-2.5 text-xs">订阅列表</a>
            <a href="https://learn.microsoft.com/azure/active-directory/develop/howto-create-service-principal-portal" target="_blank" rel="noreferrer" className="btn-ghost h-7 px-2.5 text-xs">官方文档</a>
          </div>
        </div>

        {/* Permission table */}
        <div className="rounded-xl border border-white/10 bg-black/20 p-3">
          <p className="text-xs font-semibold text-slate-200 mb-2">所需 IAM 角色权限</p>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[480px] border-separate border-spacing-0 text-[11px]">
              <thead>
                <tr>
                  <th className="rounded-tl-lg border border-white/10 bg-white/5 px-2.5 py-2 text-left font-semibold text-slate-200">角色</th>
                  <th className="border border-white/10 bg-white/5 px-2.5 py-2 text-left font-semibold text-slate-200">作用范围</th>
                  <th className="rounded-tr-lg border border-white/10 bg-white/5 px-2.5 py-2 text-left font-semibold text-slate-200">用途</th>
                </tr>
              </thead>
              <tbody>
                {[
                  ['Contributor', '订阅或资源组', '创建 VM、网络、公网 IP 等全部资源'],
                  ['Virtual Machine Contributor', '订阅或资源组', '仅允许管理 VM（最小权限方案）'],
                  ['Network Contributor', '订阅或资源组', '创建 VNet / NSG / Public IP / NIC'],
                ].map(([role, scope, desc], idx) => (
                  <tr key={role}>
                    <td className={`border border-white/10 px-2.5 py-2 text-slate-100 font-medium ${idx === 2 ? 'rounded-bl-lg' : ''}`}>{role}</td>
                    <td className="border border-white/10 px-2.5 py-2 text-slate-300">{scope}</td>
                    <td className={`border border-white/10 px-2.5 py-2 text-slate-300 ${idx === 2 ? 'rounded-br-lg' : ''}`}>{desc}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* Form */}
        <div className="space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-slate-400 mb-1">Tenant ID <span className="text-red-400">*</span></label>
              <input
                className="glass-input font-mono text-xs"
                value={form.tenant_id}
                onChange={e => update('tenant_id', e.target.value)}
                placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              />
            </div>
            <div>
              <label className="block text-xs text-slate-400 mb-1">Subscription ID <span className="text-red-400">*</span></label>
              <input
                className="glass-input font-mono text-xs"
                value={form.subscription_id}
                onChange={e => update('subscription_id', e.target.value)}
                placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              />
            </div>
          </div>
          <div>
            <label className="block text-xs text-slate-400 mb-1">Client ID (Application ID) <span className="text-red-400">*</span></label>
            <input
              className="glass-input font-mono text-xs"
              value={form.client_id}
              onChange={e => update('client_id', e.target.value)}
              placeholder="App registrations 页面的 Application (client) ID"
            />
          </div>
          <div>
            <label className="block text-xs text-slate-400 mb-1">
              Client Secret <span className="text-red-400">*</span>
              {isEditing && <span className="text-muted ml-1">（留空则保留已有密钥）</span>}
            </label>
            <div className="flex gap-2">
              <input
                className="glass-input flex-1 font-mono text-xs"
                type={showSecret ? 'text' : 'password'}
                value={form.client_secret}
                onChange={e => update('client_secret', e.target.value)}
                placeholder={isEditing ? '不修改请留空' : 'Certificates & secrets 中生成的 Secret Value'}
              />
              <button type="button" className="btn-ghost px-2.5" onClick={() => setShowSecret(v => !v)}>
                {showSecret ? <EyeOff size={14} /> : <Eye size={14} />}
              </button>
            </div>
          </div>
        </div>

        {error && (
          <div className="flex items-center gap-2 rounded-xl border border-red-500/20 bg-red-500/5 px-3 py-2 text-xs text-red-400">
            <AlertCircle size={12} className="shrink-0" />{error}
          </div>
        )}

        <div className="flex items-center justify-end gap-3 pt-1">
          <button className="btn-ghost" onClick={onClose} disabled={busy}>取消</button>
          <button
            className="btn-primary"
            onClick={handleSave}
            disabled={busy || !form.tenant_id.trim() || !form.client_id.trim() || !form.subscription_id.trim()}
          >
            {busy ? <Loader2 size={14} className="animate-spin" /> : <CheckCircle2 size={14} />}
            保存凭据
          </button>
        </div>
      </div>
    </ModalShell>
  )
}

// ── AzureConfigBanner ─────────────────────────────────────────────────────────

export function AzureConfigBanner({
  config,
  loading,
  onConfigure,
}: {
  config: Omit<AzureConfig, 'client_secret'> | null
  loading: boolean
  onConfigure: () => void
}) {
  if (loading) return null

  if (config?.tenant_id && config?.subscription_id) {
    return (
      <div className="flex items-center gap-3 rounded-xl border border-sky-500/15 bg-sky-500/[0.04] px-4 py-2.5 text-xs">
        <CheckCircle2 size={13} className="text-sky-400 shrink-0" />
        <span className="flex-1 text-sky-300/80">Azure 凭据已配置，可通过 Azure API 创建虚拟机</span>
        <span className="font-mono text-[10px] text-muted truncate max-w-[200px]">
          Sub: {config.subscription_id.slice(0, 8)}…
        </span>
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
        <p className="text-sm font-semibold text-amber-300">Azure 凭据未配置</p>
        <p className="text-xs text-amber-300/55 mt-0.5 leading-relaxed">
          配置 Service Principal 凭据后，可直接在此创建 Azure 虚拟机并注册为托管节点。
        </p>
      </div>
      <button className="btn-primary h-8 px-3 text-xs shrink-0 self-center" onClick={onConfigure}>
        立即配置
      </button>
    </div>
  )
}
