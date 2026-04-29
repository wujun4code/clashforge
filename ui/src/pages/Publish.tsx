import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  AlertCircle,
  CheckCircle2,
  CloudCog,
  Copy,
  Eye,
  ExternalLink,
  FileCode2,
  Loader2,
  Network,
  Plus,
  RefreshCw,
  Rocket,
  Save,
  Server,
  Trash2,
  UploadCloud,
  Wand2,
} from 'lucide-react'

import {
  bindPublishWorkerDomain,
  createPublishWorkerNamespace,
  deletePublishWorkerConfig,
  deletePublishRecord,
  deployPublishWorkerScript,
  getCloudflareZones,
  getPublishNodes,
  getPublishRecords,
  getPublishTemplates,
  getPublishWorkerConfigs,
  previewPublishConfig,
  uploadPublishConfig,
  verifyAndSavePublishWorker,
  type CloudflareZone,
  type PublishNode,
  type PublishRecord,
  type PublishTemplateMode,
  type PublishTemplatePreset,
  type PublishWorkerConfig,
  type PublishWorkerVerifyResult,
} from '../api/client'
import { EmptyState, InlineNotice, ModalShell, PageHeader, SectionCard, SegmentedTabs } from '../components/ui'
import {
  CFGate,
  type CFConfig,
  CFConfigBanner,
  CFConfigModal,
  maskSecret,
  useCFConfig,
} from '../components/CFConfig'

type NoticeTone = 'info' | 'success' | 'warning' | 'danger'

interface NoticeState {
  tone: NoticeTone
  title: string
  text: string
}

function randomToken(prefix = 'cf') {
  const suffix = Math.random().toString(36).slice(2, 12)
  return `${prefix}-${suffix}`
}

function randomConfigName() {
  const date = new Date()
  const stamp = `${date.getFullYear()}${String(date.getMonth() + 1).padStart(2, '0')}${String(date.getDate()).padStart(2, '0')}`
  return `订阅发布-${stamp}-${Math.random().toString(36).slice(2, 6)}`
}

function parseSubdomain(hostname: string, zone: string): string {
  const host = hostname.trim().toLowerCase()
  const z = zone.trim().toLowerCase()
  if (!host || !z) return ''
  const suffix = `.${z}`
  if (!host.endsWith(suffix)) return ''
  return host.slice(0, host.length - suffix.length)
}

function normalizeSubdomain(raw: string): string {
  return raw
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9-.]/g, '-')
    .replace(/-+/g, '-')
    .replace(/^\.+|\.+$/g, '')
    .replace(/^-+|-+$/g, '')
}

function suggestHostnames(zone: string): string[] {
  return ['blog', 'store', 'market', 'sales', 'news'].map((prefix) => `${prefix}.${zone}`)
}

async function copyText(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text)
    return
  }
  const input = document.createElement('textarea')
  input.value = text
  input.style.cssText = 'position:fixed;opacity:0;top:0;left:0'
  document.body.appendChild(input)
  input.focus()
  input.select()
  document.execCommand('copy')
  document.body.removeChild(input)
}

function WorkerWizardModal({
  onClose,
  onSaved,
  defaultConfig,
  cfConfig,
}: {
  onClose: () => void
  onSaved: (config: PublishWorkerConfig) => void
  defaultConfig: PublishWorkerConfig | null
  cfConfig?: CFConfig | null
}) {
  const [step, setStep] = useState(1)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState<NoticeState | null>(null)
  const [verify, setVerify] = useState<PublishWorkerVerifyResult | null>(null)
  const [savedConfig, setSavedConfig] = useState<PublishWorkerConfig | null>(null)
  const [zones, setZones] = useState<CloudflareZone[]>([])
  const [domainSuggestions, setDomainSuggestions] = useState<string[]>([])
  const [subdomain, setSubdomain] = useState('')
  const [copiedWorkerURL, setCopiedWorkerURL] = useState(false)
  const [logs, setLogs] = useState<Array<{
    ts: string
    step: string
    status: 'info' | 'success' | 'error'
    message: string
    detail?: string
  }>>([])

  const cfToken = cfConfig?.cf_token?.trim() ?? ''
  const accountID = (cfConfig?.cf_account_id?.trim() ?? defaultConfig?.account_id ?? '').trim()

  const [form, setForm] = useState({
    name: defaultConfig?.name || randomConfigName(),
    account_id: accountID,
    zone_id: defaultConfig?.zone_id ?? '',
    worker_name: defaultConfig?.worker_name || randomToken('cf-sub'),
    hostname: defaultConfig?.hostname ?? '',
    namespace_id: defaultConfig?.namespace_id ?? '',
    worker_dev_url: defaultConfig?.worker_dev_url ?? '',
    worker_url: defaultConfig?.worker_url ?? '',
    access_token: randomToken('sub'),
  })

  const update = <K extends keyof typeof form>(key: K, value: (typeof form)[K]) => {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  const selectedZone = useMemo(
    () => zones.find((item) => item.id === form.zone_id) ?? null,
    [zones, form.zone_id],
  )

  const fullHostname = useMemo(() => {
    const zone = selectedZone?.name?.trim().toLowerCase()
    const sub = normalizeSubdomain(subdomain)
    if (!zone || !sub) return ''
    return `${sub}.${zone}`
  }, [selectedZone?.name, subdomain])

  const addLog = useCallback((stage: string, status: 'info' | 'success' | 'error', message: string, detail?: string) => {
    const ts = new Date().toLocaleTimeString('zh-CN', { hour12: false })
    setLogs((prev) => [...prev, { ts, step: stage, status, message, detail }])
  }, [])

  useEffect(() => {
    if (!selectedZone?.name) {
      setDomainSuggestions([])
      return
    }
    setDomainSuggestions(suggestHostnames(selectedZone.name))
    setSubdomain((prev) => {
      if (prev) return prev
      const parsed = parseSubdomain(form.hostname, selectedZone.name)
      return parsed || 'blog'
    })
  }, [selectedZone?.name, form.hostname])

  useEffect(() => {
    if (!cfToken || !form.account_id) return
    void runLoadZones()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const runLoadZones = async () => {
    if (!cfToken || !form.account_id) {
      setNotice({ tone: 'warning', title: 'Cloudflare 凭据不可用', text: '请先在页面顶部完成 Cloudflare 凭据配置。' })
      addLog('步骤 1', 'error', 'Cloudflare 凭据不可用，无法加载域名')
      return
    }
    setBusy(true)
    setNotice(null)
    addLog('步骤 1', 'info', '开始检测凭据并加载 Cloudflare 域名列表')
    try {
      const data = await getCloudflareZones({
        cf_token: cfToken,
        cf_account_id: form.account_id,
      })
      const list = data.zones ?? []
      setZones(list)
      if (list.length === 0) {
        setNotice({ tone: 'warning', title: '未读取到域名', text: '当前账号下没有可用 Zone，请先在 Cloudflare 托管域名。' })
        addLog('步骤 1', 'error', '凭据可用，但未读取到任何可用域名')
        return
      }
      const nextZoneID = list.some((item) => item.id === form.zone_id) ? form.zone_id : list[0].id
      update('zone_id', nextZoneID)
      setStep((prev) => Math.max(prev, 2))
      const zoneName = list.find((item) => item.id === nextZoneID)?.name ?? '未知域名'
      setNotice({ tone: 'success', title: '域名加载完成', text: `已读取 ${list.length} 个域名，默认选择 ${zoneName}。` })
      addLog('步骤 1', 'success', `已读取 ${list.length} 个域名`, `默认域名：${zoneName}`)
    } catch (error) {
      const message = error instanceof Error ? error.message : '请求失败'
      setNotice({ tone: 'danger', title: '加载失败', text: message })
      addLog('步骤 1', 'error', '加载域名失败', message)
    } finally {
      setBusy(false)
    }
  }

  const runCreateNamespace = async () => {
    if (!form.worker_name) {
      setNotice({ tone: 'warning', title: 'Worker 名称缺失', text: '请确认 Worker 名称后再继续。' })
      return
    }
    if (!cfToken || !form.account_id) {
      setNotice({ tone: 'warning', title: 'Cloudflare 凭据不可用', text: '请先完成凭据配置。' })
      return
    }
    setBusy(true)
    setNotice(null)
    addLog('步骤 2', 'info', '开始创建（或复用）KV Namespace')
    try {
      const data = await createPublishWorkerNamespace({
        token: cfToken,
        account_id: form.account_id,
        worker_name: form.worker_name,
      })
      update('namespace_id', data.namespace_id)
      setStep((prev) => Math.max(prev, 3))
      setNotice({
        tone: 'success',
        title: data.reused ? '已复用 Namespace' : 'Namespace 创建成功',
        text: `${data.title} · ${data.namespace_id}`,
      })
      addLog('步骤 2', 'success', data.reused ? '复用已有 Namespace' : '创建 Namespace 成功', data.namespace_id)
    } catch (error) {
      const message = error instanceof Error ? error.message : '请求失败'
      setNotice({ tone: 'danger', title: '创建失败', text: message })
      addLog('步骤 2', 'error', '创建 Namespace 失败', message)
    } finally {
      setBusy(false)
    }
  }

  const runDeployWorker = async () => {
    if (!form.namespace_id || !form.access_token) {
      setNotice({ tone: 'warning', title: '参数不足', text: '请先完成 Namespace 创建，并确认访问 Token。' })
      return
    }
    if (!cfToken || !form.account_id) {
      setNotice({ tone: 'warning', title: 'Cloudflare 凭据不可用', text: '请先完成凭据配置。' })
      return
    }
    setBusy(true)
    setNotice(null)
    addLog('步骤 3', 'info', '开始部署 Worker 脚本')
    try {
      const data = await deployPublishWorkerScript({
        token: cfToken,
        account_id: form.account_id,
        worker_name: form.worker_name,
        namespace_id: form.namespace_id,
        access_token: form.access_token,
      })
      update('worker_dev_url', data.worker_dev_url)
      setStep((prev) => Math.max(prev, 4))
      setNotice({ tone: 'success', title: 'Worker 已部署', text: '请继续绑定自定义域名。' })
      addLog('步骤 3', 'success', 'Worker 部署成功', data.worker_dev_url)
    } catch (error) {
      const message = error instanceof Error ? error.message : '请求失败'
      setNotice({ tone: 'danger', title: '部署失败', text: message })
      addLog('步骤 3', 'error', 'Worker 部署失败', message)
    } finally {
      setBusy(false)
    }
  }

  const runBindDomain = async () => {
    if (!form.zone_id || !fullHostname) {
      setNotice({ tone: 'warning', title: '请先选择域名', text: '请先选择顶级域名并填写二级域名。' })
      return
    }
    if (!cfToken || !form.account_id) {
      setNotice({ tone: 'warning', title: 'Cloudflare 凭据不可用', text: '请先完成凭据配置。' })
      return
    }
    setBusy(true)
    setNotice(null)
    addLog('步骤 4', 'info', `开始绑定域名 ${fullHostname}`)
    try {
      const data = await bindPublishWorkerDomain({
        token: cfToken,
        account_id: form.account_id,
        zone_id: form.zone_id,
        worker_name: form.worker_name,
        hostname: fullHostname,
      })
      update('worker_url', data.worker_url)
      update('hostname', data.hostname)
      setStep((prev) => Math.max(prev, 5))
      setNotice({ tone: 'success', title: '域名绑定完成', text: '最后执行连通性验证并保存。' })
      addLog('步骤 4', 'success', '域名绑定成功', `${data.hostname} -> ${data.worker_url}`)
    } catch (error) {
      const message = error instanceof Error ? error.message : '请求失败'
      setNotice({ tone: 'danger', title: '绑定失败', text: message })
      addLog('步骤 4', 'error', '域名绑定失败', message)
    } finally {
      setBusy(false)
    }
  }

  const runVerifyAndSave = async () => {
    if (!form.zone_id || !form.worker_url || !form.hostname) {
      setNotice({ tone: 'warning', title: '请先完成前置步骤', text: '请先完成 Worker 部署与域名绑定。' })
      return
    }
    setBusy(true)
    setNotice(null)
    setVerify(null)
    addLog('步骤 5', 'info', '开始验证托管环境并写入 ClashForge')
    try {
      const data = await verifyAndSavePublishWorker({
        name: form.name || form.worker_name,
        worker_name: form.worker_name,
        worker_url: form.worker_url,
        worker_dev_url: form.worker_dev_url,
        hostname: form.hostname,
        account_id: form.account_id,
        namespace_id: form.namespace_id,
        zone_id: form.zone_id,
        access_token: form.access_token,
      })
      setVerify(data.result)
      if (!data.result.ok || !data.config) {
        setNotice({ tone: 'warning', title: '验证未通过', text: '请检查上面的失败项后重试。' })
        addLog('步骤 5', 'error', '验证未通过', '请根据测试结果修复后重试')
        return
      }
      setSavedConfig(data.config)
      setStep(6)
      setNotice({ tone: 'success', title: '保存成功', text: '托管环境已写入 ClashForge，可直接创建订阅链接。' })
      const passed = data.result.tests.filter((item) => item.ok).length
      addLog('步骤 5', 'success', `验证通过（${passed}/${data.result.tests.length}）`, data.result.used_url || data.result.hello_url)
      onSaved(data.config)
    } catch (error) {
      const message = error instanceof Error ? error.message : '请求失败'
      setNotice({ tone: 'danger', title: '验证失败', text: message })
      addLog('步骤 5', 'error', '验证或保存失败', message)
    } finally {
      setBusy(false)
    }
  }

  const copyWorkerEndpoint = async () => {
    const target = savedConfig?.worker_url || form.worker_url
    if (!target) return
    await copyText(target)
    setCopiedWorkerURL(true)
    setTimeout(() => setCopiedWorkerURL(false), 1500)
  }

  const steps = ['加载域名', '创建 Namespace', '部署 Worker', '绑定域名', '验证并保存']

  const isWizardDirty = form.zone_id !== '' || step > 1
  const handleWizardBeforeClose = () => {
    if (busy) return false
    if (!isWizardDirty) return true
    return window.confirm('确认放弃已输入的内容并关闭？')
  }

  return (
    <ModalShell
      title="创建订阅托管环境"
      description="最少输入，按步骤点击执行。只需选择域名和二级域名前缀。"
      icon={<CloudCog size={18} />}
      onClose={onClose}
      onBeforeClose={handleWizardBeforeClose}
      size="xl"
    >
      <div className="space-y-4">
        <div className="rounded-xl border border-white/10 bg-black/20 px-3 py-2.5 text-xs">
          <div className="flex flex-wrap items-center gap-2">
            <span className={cfToken && form.account_id ? 'text-emerald-300' : 'text-amber-300'}>
              {cfToken && form.account_id ? 'Cloudflare 凭据已配置' : 'Cloudflare 凭据缺失'}
            </span>
            {form.account_id ? (
              <span className="font-mono text-muted">Account: {maskSecret(form.account_id)}</span>
            ) : null}
            <button className="btn-ghost ml-auto h-7 px-2.5 text-xs" onClick={runLoadZones} disabled={busy || !cfToken || !form.account_id}>
              {busy ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
              加载域名
            </button>
          </div>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <label className="mb-1 block text-xs text-muted">配置名称</label>
            <input
              className="glass-input"
              value={form.name}
              onChange={(event) => update('name', event.target.value)}
              placeholder="自动生成，可改"
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-muted">Worker 名称</label>
            <input
              className="glass-input font-mono"
              value={form.worker_name}
              onChange={(event) => update('worker_name', event.target.value.trim())}
              placeholder="自动生成，可改"
            />
          </div>
          <div className="sm:col-span-2">
            <label className="mb-1 block text-xs text-muted">订阅访问 Token</label>
            <div className="flex gap-2">
              <input
                className="glass-input font-mono"
                value={form.access_token}
                onChange={(event) => update('access_token', event.target.value.trim())}
                placeholder="自动生成，可改"
              />
              <button
                type="button"
                className="btn-ghost h-10 px-3"
                onClick={() => update('access_token', randomToken('sub'))}
              >
                随机
              </button>
            </div>
          </div>
        </div>

        <div className="rounded-xl border border-white/10 bg-black/20 p-3 space-y-3">
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <label className="mb-1 block text-xs text-muted">顶级域名</label>
              <select
                className="theme-select glass-input"
                value={form.zone_id}
                onChange={(event) => update('zone_id', event.target.value)}
                disabled={zones.length === 0}
              >
                {zones.length === 0 ? (
                  <option value="">先点击“加载域名”</option>
                ) : (
                  zones.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name}
                    </option>
                  ))
                )}
              </select>
            </div>
            <div>
              <label className="mb-1 block text-xs text-muted">二级域名前缀</label>
              <input
                className="glass-input font-mono"
                value={subdomain}
                onChange={(event) => setSubdomain(event.target.value)}
                placeholder="例如 blog / market / sales"
              />
            </div>
          </div>
          {domainSuggestions.length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {domainSuggestions.map((host) => (
                <button
                  key={host}
                  className={[
                    'rounded-full border px-2.5 py-1 text-[11px] font-mono transition-colors',
                    fullHostname === host
                      ? 'border-brand/50 bg-brand/12 text-brand-light'
                      : 'border-white/10 bg-white/[0.04] text-muted hover:border-white/20 hover:text-slate-200',
                  ].join(' ')}
                  onClick={() => setSubdomain(host.split('.')[0])}
                >
                  {host}
                </button>
              ))}
            </div>
          ) : null}
          <div className="rounded-lg border border-white/10 bg-white/[0.03] px-3 py-2">
            <p className="text-[11px] text-muted">将绑定域名</p>
            <p className="text-sm font-mono text-slate-100">{fullHostname || '未选择'}</p>
          </div>
        </div>

        <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
          {steps.map((label, idx) => {
            const index = idx + 1
            const done = step > index
            const active = step === index
            return (
              <div
                key={label}
                className={[
                  'rounded-md border px-2 py-1.5 text-center text-xs',
                  done
                    ? 'border-emerald-400/30 bg-emerald-400/10 text-emerald-300'
                    : active
                      ? 'border-brand/50 bg-brand/12 text-brand-light'
                      : 'border-white/10 bg-white/5 text-muted',
                ].join(' ')}
              >
                {index}. {label}
              </div>
            )
          })}
        </div>

        <div className="flex flex-wrap gap-2">
          <button className="btn-primary" onClick={runLoadZones} disabled={busy}>
            {busy ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
            1. 检测并加载域名
          </button>
          <button className="btn-ghost" onClick={runCreateNamespace} disabled={busy || step < 2}>
            {busy ? <Loader2 size={14} className="animate-spin" /> : <Plus size={14} />}
            2. 创建 Namespace
          </button>
          <button className="btn-ghost" onClick={runDeployWorker} disabled={busy || step < 3}>
            {busy ? <Loader2 size={14} className="animate-spin" /> : <Rocket size={14} />}
            3. 部署 Worker
          </button>
          <button className="btn-ghost" onClick={runBindDomain} disabled={busy || step < 4 || !fullHostname}>
            {busy ? <Loader2 size={14} className="animate-spin" /> : <Network size={14} />}
            4. 绑定域名
          </button>
          <button className="btn-cta" onClick={runVerifyAndSave} disabled={busy || step < 5}>
            {busy ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
            5. 验证并保存
          </button>
        </div>

        <div className="rounded-lg border border-white/10 bg-black/20 p-3">
          <div className="mb-2 flex items-center justify-between">
            <p className="text-xs font-semibold uppercase tracking-[0.16em] text-muted">操作日志</p>
            <span className="text-[11px] text-muted">{logs.length} 条</span>
          </div>
          <div className="max-h-[180px] space-y-1 overflow-y-auto rounded-md border border-white/10 bg-black/30 p-2 font-mono text-[11px]">
            {logs.length === 0 ? (
              <p className="text-muted">等待操作...</p>
            ) : (
              logs.map((item, idx) => (
                <div key={`${item.ts}-${idx}`} className="rounded border border-white/5 bg-white/[0.02] px-2 py-1.5">
                  <p className={item.status === 'error' ? 'text-red-300' : item.status === 'success' ? 'text-emerald-300' : 'text-slate-300'}>
                    [{item.ts}] [{item.step}] {item.message}
                  </p>
                  {item.detail ? <p className="mt-0.5 break-all text-[10px] text-muted">{item.detail}</p> : null}
                </div>
              ))
            )}
          </div>
        </div>

        {verify ? (
          <div className="rounded-lg border border-white/10 bg-black/20 p-3">
            {verify.tests.map((item) => (
              <div key={item.name} className="mb-1 flex items-start gap-2 text-xs last:mb-0">
                {item.ok ? (
                  <CheckCircle2 size={13} className="mt-0.5 text-success" />
                ) : (
                  <AlertCircle size={13} className="mt-0.5 text-danger" />
                )}
                <div className="min-w-0">
                  <p className={item.ok ? 'text-slate-200' : 'text-danger'}>{item.name}</p>
                  {item.detail ? <p className="mt-0.5 break-all text-muted">{item.detail}</p> : null}
                </div>
              </div>
            ))}
          </div>
        ) : null}

        {savedConfig ? (
          <div className="rounded-xl border border-emerald-500/25 bg-emerald-500/[0.05] p-3 space-y-2">
            <p className="text-sm font-semibold text-emerald-300">托管环境已就绪</p>
            <p className="text-xs text-emerald-300/80">下一步回到页面点击“创建订阅”，系统会自动生成并复制订阅链接。</p>
            <div className="flex flex-wrap items-center gap-2">
              <code className="max-w-full break-all rounded bg-black/30 px-2 py-1 text-xs text-emerald-200">{savedConfig.worker_url}</code>
              <button className="btn-ghost h-7 px-2.5 text-xs" onClick={() => { void copyWorkerEndpoint() }}>
                <Copy size={12} className={copiedWorkerURL ? 'text-success' : ''} />
                {copiedWorkerURL ? '已复制' : '复制 Worker 地址'}
              </button>
              <a href={savedConfig.worker_url} target="_blank" rel="noreferrer" className="btn-ghost h-7 px-2.5 text-xs">
                <ExternalLink size={12} />
                打开
              </a>
            </div>
          </div>
        ) : null}

        {notice ? (
          <InlineNotice tone={notice.tone} title={notice.title}>
            {notice.text}
          </InlineNotice>
        ) : null}

        <div className="flex justify-end">
          <button className="btn-ghost" onClick={onClose} disabled={busy}>
            关闭
          </button>
        </div>
      </div>
    </ModalShell>
  )
}

export function Publish() {
  const { config: cfGlobal, loading: cfLoading, save: saveCFGlobal, reload: reloadCF } = useCFConfig()
  const [showCFModal, setShowCFModal] = useState(false)

  const [nodes, setNodes] = useState<PublishNode[]>([])
  const [templates, setTemplates] = useState<PublishTemplatePreset[]>([])
  const [workerConfigs, setWorkerConfigs] = useState<PublishWorkerConfig[]>([])
  const [records, setRecords] = useState<PublishRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [busyRefresh, setBusyRefresh] = useState(false)
  const [notice, setNotice] = useState<NoticeState | null>(null)

  const [templateMode, setTemplateMode] = useState<PublishTemplateMode>('builtin')
  const [templateID, setTemplateID] = useState('loyalsoldier_standard')
  const [templateContent, setTemplateContent] = useState('')
  const [selectedNodes, setSelectedNodes] = useState<string[]>([])
  const [previewContent, setPreviewContent] = useState('')
  const [previewing, setPreviewing] = useState(false)

  const [selectedWorkerConfigID, setSelectedWorkerConfigID] = useState('')
  const [publishing, setPublishing] = useState(false)
  const [copiedRecordID, setCopiedRecordID] = useState('')
  const [latestPublish, setLatestPublish] = useState<{ access_url: string; file_name: string; version: number } | null>(null)
  const [showWizard, setShowWizard] = useState(false)

  const refreshAll = useCallback(async (showRefreshing = false) => {
    if (showRefreshing) setBusyRefresh(true)
    try {
      const [nodesData, templatesData, configData, recordData] = await Promise.all([
        getPublishNodes(),
        getPublishTemplates(),
        getPublishWorkerConfigs(),
        getPublishRecords(),
      ])
      setNodes(nodesData.nodes ?? [])
      setTemplates(templatesData.templates ?? [])
      setWorkerConfigs(configData.configs ?? [])
      setRecords(recordData.records ?? [])

      setSelectedNodes((prev) => {
        const allowed = new Set((nodesData.nodes ?? []).map((item) => item.id))
        const next = prev.filter((id) => allowed.has(id))
        if (next.length > 0) return next
        return (nodesData.nodes ?? []).slice(0, 3).map((item) => item.id)
      })
      setSelectedWorkerConfigID((prev) => {
        const configs = configData.configs ?? []
        if (configs.length === 0) return ''
        if (configs.some((item) => item.id === prev)) return prev
        return configs[0].id
      })
    } catch (error) {
      setNotice({ tone: 'danger', title: '加载失败', text: error instanceof Error ? error.message : '请求失败' })
    } finally {
      setLoading(false)
      if (showRefreshing) setBusyRefresh(false)
    }
  }, [])

  useEffect(() => {
    void refreshAll()
  }, [refreshAll])

  const selectedWorkerConfig = useMemo(
    () => workerConfigs.find((item) => item.id === selectedWorkerConfigID) ?? null,
    [workerConfigs, selectedWorkerConfigID],
  )

  const toggleNode = (id: string) => {
    setSelectedNodes((prev) => {
      if (prev.includes(id)) return prev.filter((item) => item !== id)
      return [...prev, id]
    })
  }

  const runPreview = async () => {
    if (selectedNodes.length === 0) {
      setNotice({ tone: 'warning', title: '请先选择节点', text: '至少选择一个已部署节点后再预览。' })
      return
    }
    setPreviewing(true)
    setNotice(null)
    try {
      const data = await previewPublishConfig({
        node_ids: selectedNodes,
        template_mode: templateMode,
        template_id: templateID,
        template_content: templateMode === 'custom' ? templateContent : undefined,
      })
      setPreviewContent(data.content)
      setNotice({
        tone: 'success',
        title: '预览完成',
        text: `已生成 ${data.node_count} 个节点的合并配置。`,
      })
    } catch (error) {
      setNotice({ tone: 'danger', title: '预览失败', text: error instanceof Error ? error.message : '请求失败' })
    } finally {
      setPreviewing(false)
    }
  }

  const runPublish = async () => {
    if (!selectedWorkerConfigID) {
      setNotice({ tone: 'warning', title: '请选择托管环境', text: '先绑定一个 Worker 配置，再执行发布。' })
      return
    }
    if (selectedNodes.length === 0) {
      setNotice({ tone: 'warning', title: '请先选择节点', text: '至少选择一个已部署节点。' })
      return
    }
    setPublishing(true)
    setNotice(null)
    setLatestPublish(null)
    try {
      const data = await uploadPublishConfig({
        worker_config_id: selectedWorkerConfigID,
        base_name: 'clashforge',
        content: previewContent || undefined,
        node_ids: selectedNodes,
        template_mode: templateMode,
        template_id: templateID,
        template_content: templateMode === 'custom' ? templateContent : undefined,
      })
      await refreshAll()
      setLatestPublish({ access_url: data.access_url, file_name: data.file_name, version: data.version })
      try {
        await copyText(data.access_url)
        setNotice({
          tone: 'success',
          title: '订阅创建成功',
          text: `已生成 ${data.file_name}（v${data.version}），链接已自动复制。`,
        })
      } catch {
        setNotice({
          tone: 'success',
          title: '订阅创建成功',
          text: `已生成 ${data.file_name}（v${data.version}），请手动复制链接。`,
        })
      }
    } catch (error) {
      setNotice({ tone: 'danger', title: '发布失败', text: error instanceof Error ? error.message : '请求失败' })
    } finally {
      setPublishing(false)
    }
  }

  const removeWorkerConfig = async () => {
    if (!selectedWorkerConfigID) return
    if (!confirm('确认删除当前托管环境？关联发布记录也会在本地清理。')) return
    setNotice(null)
    try {
      await deletePublishWorkerConfig(selectedWorkerConfigID)
      await refreshAll()
      setNotice({ tone: 'success', title: '删除成功', text: '托管环境已删除。' })
    } catch (error) {
      setNotice({ tone: 'danger', title: '删除失败', text: error instanceof Error ? error.message : '请求失败' })
    }
  }

  const removeRecord = async (id: string) => {
    if (!confirm('确认删除这条发布记录吗？这会尝试同步删除远端 KV 文件。')) return
    setNotice(null)
    try {
      const data = await deletePublishRecord(id)
      await refreshAll()
      if (data.warning) {
        setNotice({ tone: 'warning', title: '记录已删除', text: data.warning })
      } else {
        setNotice({ tone: 'success', title: '删除成功', text: '发布记录已删除。' })
      }
    } catch (error) {
      setNotice({ tone: 'danger', title: '删除失败', text: error instanceof Error ? error.message : '请求失败' })
    }
  }

  const handleCopyLink = async (id: string, link: string) => {
    try {
      await copyText(link)
      setCopiedRecordID(id)
      setTimeout(() => setCopiedRecordID(''), 1600)
    } catch {
      setNotice({ tone: 'warning', title: '复制失败', text: '当前浏览器环境不支持自动复制。' })
    }
  }

  return (
    <CFGate config={cfGlobal} loading={cfLoading} save={saveCFGlobal}>
      {loading ? (
        <div className="flex min-h-[42vh] items-center justify-center">
          <div className="flex items-center gap-2 text-sm text-muted">
            <Loader2 size={16} className="animate-spin text-brand" />
            正在加载发布工作台…
          </div>
        </div>
      ) : (
        <div className="space-y-6">
      <PageHeader
        eyebrow="代理资源 · 订阅分发"
        title="订阅分发"
        description="选择已部署节点，合并规则模板，发布到 Cloudflare Worker + KV，生成可直接使用的订阅链接。"
        metrics={[
          { label: '可用节点', value: String(nodes.length) },
          { label: '托管环境', value: String(workerConfigs.length) },
          { label: '发布记录', value: String(records.length) },
          { label: '已选节点', value: String(selectedNodes.length) },
        ]}
        actions={(
          <>
            <button className="btn-ghost flex items-center gap-2" onClick={() => { void refreshAll(true) }} disabled={busyRefresh}>
              <RefreshCw size={14} className={busyRefresh ? 'animate-spin' : ''} />
              刷新
            </button>
            <button className="btn-primary flex items-center gap-2" onClick={() => setShowWizard(true)}>
              <CloudCog size={14} />
              新建托管环境
            </button>
          </>
        )}
      />

      <CFConfigBanner config={cfGlobal} loading={cfLoading} onConfigure={() => setShowCFModal(true)} />

      {notice ? (
        <InlineNotice tone={notice.tone} title={notice.title}>
          {notice.text}
        </InlineNotice>
      ) : null}

      <SectionCard
        title="模板与节点"
        description="先确定模板来源，再勾选要注入的节点。"
      >
        <div className="grid gap-4 lg:grid-cols-[1.3fr_1fr]">
          <div className="space-y-3">
            <SegmentedTabs<PublishTemplateMode>
              value={templateMode}
              onChange={setTemplateMode}
              items={[
                { value: 'builtin', label: '内置模板', icon: <Wand2 size={14} /> },
                { value: 'runtime', label: '当前运行配置', icon: <Rocket size={14} /> },
                { value: 'custom', label: '自定义模板', icon: <FileCode2 size={14} /> },
              ]}
            />

            {templateMode === 'builtin' ? (
              <select
                className="theme-select glass-input"
                value={templateID}
                onChange={(event) => setTemplateID(event.target.value)}
              >
                {(templates.length > 0 ? templates : [{ id: 'loyalsoldier_standard', name: 'Loyalsoldier 标准规则', description: '' }]).map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </select>
            ) : null}

            {templateMode === 'custom' ? (
              <textarea
                className="glass-textarea h-[240px] resize-y text-xs leading-5"
                value={templateContent}
                onChange={(event) => setTemplateContent(event.target.value)}
                placeholder="粘贴完整 Clash YAML 模板"
              />
            ) : null}

            {templateMode === 'runtime' ? (
              <p className="rounded-md border border-white/12 bg-white/5 px-3 py-2 text-xs text-muted">
                将读取当前运行的 `mihomo-config.yaml` 作为模板基底。
              </p>
            ) : null}
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <p className="text-xs font-semibold uppercase tracking-[0.18em] text-muted">节点选择</p>
              <div className="flex gap-2">
                <button
                  className="btn-ghost h-7 px-2.5 text-xs"
                  onClick={() => setSelectedNodes(nodes.map((item) => item.id))}
                >
                  全选
                </button>
                <button className="btn-ghost h-7 px-2.5 text-xs" onClick={() => setSelectedNodes([])}>
                  清空
                </button>
              </div>
            </div>
            <div className="max-h-[260px] space-y-2 overflow-y-auto rounded-lg border border-white/10 bg-black/20 p-2.5">
              {nodes.length === 0 ? (
                <p className="px-2 py-3 text-xs text-muted">暂无可发布节点，请先到“节点管理”部署。</p>
              ) : (
                nodes.map((node) => {
                  const checked = selectedNodes.includes(node.id)
                  return (
                    <label
                      key={node.id}
                      className={[
                        'flex cursor-pointer items-start gap-2 rounded-md border px-2.5 py-2 text-xs transition-all',
                        checked ? 'border-brand/45 bg-brand/10' : 'border-white/10 bg-white/[0.03] hover:bg-white/[0.05]',
                      ].join(' ')}
                    >
                      <input
                        type="checkbox"
                        className="mt-0.5 h-3.5 w-3.5 rounded border-white/25 bg-transparent text-brand"
                        checked={checked}
                        onChange={() => toggleNode(node.id)}
                      />
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5">
                          <p className="truncate font-semibold text-slate-200">{node.name}</p>
                          {node.node_type === 'worker' && (
                            <span className="shrink-0 rounded px-1 py-0.5 text-[9px] font-semibold uppercase tracking-wide bg-brand/15 text-brand-light ring-1 ring-inset ring-brand/25">
                              Worker
                            </span>
                          )}
                        </div>
                        <p className="truncate font-mono text-muted">{node.domain || node.host}</p>
                        {!node.has_credentials && node.node_type !== 'worker' ? (
                          <p className="mt-0.5 text-danger">缺少代理账号密码</p>
                        ) : null}
                      </div>
                    </label>
                  )
                })
              )}
            </div>
          </div>
        </div>

        <div className="mt-4 flex flex-wrap gap-2">
          <button className="btn-primary flex items-center gap-2" onClick={runPreview} disabled={previewing}>
            {previewing ? <Loader2 size={14} className="animate-spin" /> : <Eye size={14} />}
            预览最终 YAML
          </button>
          <button className="btn-ghost flex items-center gap-2" onClick={() => setPreviewContent('')}>
            清空预览
          </button>
        </div>
      </SectionCard>

      <SectionCard
        title="最终配置预览"
        description="这里展示最终发布内容。ClashForge 注入代理节点与受管策略组（🚀 节点选择、♻️ 自动选择）。"
      >
        {previewContent ? (
          <textarea
            className="glass-textarea h-[420px] resize-y text-xs leading-5"
            value={previewContent}
            readOnly
            spellCheck={false}
          />
        ) : (
          <EmptyState
            title="还没有预览内容"
            description="点击“预览最终 YAML”后，这里会显示完整配置。"
            icon={<FileCode2 size={18} />}
          />
        )}
      </SectionCard>

      <SectionCard
        title="创建订阅"
        description="确认托管环境后，一键生成订阅链接。生成成功后会自动复制。"
      >
        {workerConfigs.length === 0 ? (
          <EmptyState
            title="暂无托管环境"
            description="先通过 Cloudflare Worker 向导创建一个可用环境。"
            action={(
              <button className="btn-primary flex items-center gap-2" onClick={() => setShowWizard(true)}>
                <CloudCog size={14} />
                新建托管环境
              </button>
            )}
            icon={<CloudCog size={18} />}
          />
        ) : (
          <div className="space-y-4">
            <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
              <select
                className="theme-select glass-input"
                value={selectedWorkerConfigID}
                onChange={(event) => setSelectedWorkerConfigID(event.target.value)}
              >
                {workerConfigs.map((cfg) => (
                  <option key={cfg.id} value={cfg.id}>
                    {cfg.name} · {cfg.hostname || cfg.worker_name}
                  </option>
                ))}
              </select>
              <button className="btn-cta flex items-center gap-2" onClick={runPublish} disabled={publishing}>
                {publishing ? <Loader2 size={14} className="animate-spin" /> : <UploadCloud size={14} />}
                创建订阅链接
              </button>
            </div>

            <div className="flex justify-end">
              <button
                className="btn-ghost flex items-center gap-2 text-danger hover:bg-danger/10"
                onClick={() => { void removeWorkerConfig() }}
                disabled={!selectedWorkerConfigID}
              >
                <Trash2 size={13} />
                删除当前托管环境
              </button>
            </div>

            {selectedWorkerConfig ? (
              <div className="rounded-lg border border-white/10 bg-black/20 px-3 py-2 text-xs text-muted">
                <div className="flex flex-wrap items-center gap-3">
                  <span className="inline-flex items-center gap-1.5 text-slate-200">
                    <Server size={12} className="text-brand-light" />
                    {selectedWorkerConfig.worker_name}
                  </span>
                  <span className="font-mono">{selectedWorkerConfig.hostname || selectedWorkerConfig.worker_url}</span>
                  {selectedWorkerConfig.has_token ? (
                    <span className="text-success">Token 已保存</span>
                  ) : (
                    <span className="text-warning">Token 缺失</span>
                  )}
                  {selectedWorkerConfig.worker_url ? (
                    <a
                      href={selectedWorkerConfig.worker_url}
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex items-center gap-1 text-brand-light hover:underline"
                    >
                      打开
                      <ExternalLink size={11} />
                    </a>
                  ) : null}
                </div>
              </div>
            ) : null}

            {latestPublish ? (
              <div className="rounded-xl border border-emerald-500/25 bg-emerald-500/[0.06] p-3 space-y-2">
                <p className="text-sm font-semibold text-emerald-300">
                  订阅已生成（v{latestPublish.version}）
                </p>
                <code className="block break-all rounded bg-black/30 px-2 py-1 text-xs text-emerald-200">
                  {latestPublish.access_url}
                </code>
                <div className="flex flex-wrap items-center gap-2">
                  <button
                    className="btn-ghost h-7 px-2.5 text-xs"
                    onClick={() => { void handleCopyLink('latest', latestPublish.access_url) }}
                  >
                    <Copy size={12} className={copiedRecordID === 'latest' ? 'text-success' : ''} />
                    {copiedRecordID === 'latest' ? '已复制' : '复制链接'}
                  </button>
                  <a href={latestPublish.access_url} target="_blank" rel="noreferrer" className="btn-ghost h-7 px-2.5 text-xs">
                    <ExternalLink size={12} />
                    打开链接
                  </a>
                  <span className="text-[11px] text-emerald-200/80">{latestPublish.file_name}</span>
                </div>
              </div>
            ) : null}
          </div>
        )}
      </SectionCard>

      <SectionCard
        title="发布记录"
        description="支持复制链接与删除历史版本。删除会尝试同步删除远端 KV 文件。"
      >
        {records.length === 0 ? (
          <EmptyState
            title="暂无发布记录"
            description="完成第一次发布后，这里会显示版本历史。"
            icon={<UploadCloud size={18} />}
          />
        ) : (
          <div className="table-shell overflow-hidden">
            <div className="grid grid-cols-12 gap-3 px-4 py-3 table-header-row">
              <span className="col-span-3">文件</span>
              <span className="col-span-2">托管环境</span>
              <span className="col-span-2">版本</span>
              <span className="col-span-3">发布时间</span>
              <span className="col-span-2 text-right">操作</span>
            </div>
            {records.map((record) => (
              <div key={record.id} className="grid grid-cols-12 gap-3 items-center px-4 py-3 table-row">
                <div className="col-span-3 min-w-0">
                  <p className="truncate text-sm font-semibold text-slate-100">{record.file_name}</p>
                  <p className="truncate text-xs font-mono text-muted">{record.base_name}</p>
                </div>
                <div className="col-span-2 min-w-0">
                  <p className="truncate text-xs text-slate-300">{record.worker_name}</p>
                </div>
                <div className="col-span-2">
                  <span className="badge badge-muted">v{record.version}</span>
                </div>
                <div className="col-span-3">
                  <p className="text-xs text-muted">
                    {new Date(record.published_at).toLocaleString('zh-CN', {
                      month: '2-digit',
                      day: '2-digit',
                      hour: '2-digit',
                      minute: '2-digit',
                    })}
                  </p>
                </div>
                <div className="col-span-2 flex items-center justify-end gap-2">
                  <button
                    className="btn-icon-sm btn-ghost"
                    title="复制链接"
                    onClick={() => { void handleCopyLink(record.id, record.access_url) }}
                  >
                    <Copy size={14} className={copiedRecordID === record.id ? 'text-success' : ''} />
                  </button>
                  <a
                    className="btn-icon-sm btn-ghost"
                    title="打开链接"
                    href={record.access_url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    <ExternalLink size={14} />
                  </a>
                  <button
                    className="btn-icon-sm btn-ghost text-danger hover:bg-danger/10"
                    title="删除记录"
                    onClick={() => { void removeRecord(record.id) }}
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </SectionCard>

      {showWizard ? (
        <WorkerWizardModal
          onClose={() => setShowWizard(false)}
          defaultConfig={selectedWorkerConfig}
          cfConfig={cfGlobal}
          onSaved={(config) => {
            setSelectedWorkerConfigID(config.id)
            void refreshAll()
          }}
        />
      ) : null}

      {showCFModal && (
        <CFConfigModal
          initial={cfGlobal}
          save={saveCFGlobal}
          onClose={() => setShowCFModal(false)}
          onSaved={() => { setShowCFModal(false); void reloadCF() }}
        />
      )}
        </div>
      )}
    </CFGate>
  )
}
