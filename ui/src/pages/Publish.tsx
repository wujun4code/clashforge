import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  AlertCircle,
  CheckCircle2,
  CloudCog,
  Copy,
  Database,
  ExternalLink,
  FileCode2,
  FileText,
  ListPlus,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Rocket,
  Server,
  Sparkles,
  Trash2,
  UploadCloud,
  Wand2,
} from 'lucide-react'

import {
  bindPublishWorkerDomain,
  createPublishWorkerNamespace,
  createRuleSet,
  deletePublishWorkerConfig,
  deletePublishRecord,
  deleteRuleSet,
  deployPublishWorkerScript,
  destroyPublishWorkerConfig,
  getCloudflareZones,
  getPublishNodes,
  getPublishRecords,
  getPublishTemplates,
  getPublishWorkerConfigs,
  getRuleSets,
  previewPublishConfig,
  updatePublishWorkerConfig,
  updateRuleSet,
  uploadPublishConfig,
  verifyAndSavePublishWorker,
  type CloudflareZone,
  type PublishNode,
  type PublishRecord,
  type PublishTemplateMode,
  type PublishTemplatePreset,
  type PublishWorkerConfig,
  type RuleSet,
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

// ─── WorkerConfigEditModal ─────────────────────────────────────────────────

type EditTab = 'rename' | 'rebind' | 'token'

function WorkerConfigEditModal({
  onClose,
  config,
  cfConfig,
  onSaved,
}: {
  onClose: () => void
  config: PublishWorkerConfig
  cfConfig: CFConfig | null
  onSaved: (updated: PublishWorkerConfig) => void
}) {
  const [tab, setTab] = useState<EditTab>('rename')
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState<NoticeState | null>(null)

  // rename
  const [name, setName] = useState(config.name)

  // rebind
  const [zones, setZones] = useState<CloudflareZone[]>([])
  const [zonesLoaded, setZonesLoaded] = useState(false)
  const [zoneID, setZoneID] = useState(config.zone_id)
  const [subdomain, setSubdomain] = useState('')
  const [rebindPhase, setRebindPhase] = useState<'' | 'binding' | 'saving' | 'done' | 'error'>('')

  // token
  const [newToken, setNewToken] = useState(() => randomToken('sub'))
  const [tokenPhase, setTokenPhase] = useState<'' | 'deploying' | 'saving' | 'done' | 'error'>('')

  const cfToken = cfConfig?.cf_token?.trim() ?? ''
  const accountID = config.account_id.trim()

  const selectedZone = useMemo(() => zones.find((z) => z.id === zoneID) ?? null, [zones, zoneID])
  const newHostname = useMemo(() => {
    const zone = selectedZone?.name?.trim().toLowerCase()
    const sub = normalizeSubdomain(subdomain)
    if (!zone || !sub) return ''
    return `${sub}.${zone}`
  }, [selectedZone?.name, subdomain])

  // Load zones when rebind tab is first opened
  useEffect(() => {
    if (tab !== 'rebind' || zonesLoaded || !cfToken || !accountID) return
    void (async () => {
      try {
        const data = await getCloudflareZones({ cf_token: cfToken, cf_account_id: accountID })
        setZones(data.zones ?? [])
        setZonesLoaded(true)
        const matched = (data.zones ?? []).find((z: CloudflareZone) => z.id === config.zone_id)
        if (matched) {
          setZoneID(matched.id)
          setSubdomain(parseSubdomain(config.hostname, matched.name))
        }
      } catch (error) {
        setNotice({ tone: 'danger', title: '加载域名失败', text: error instanceof Error ? error.message : '请求失败' })
      }
    })()
  }, [tab, zonesLoaded, cfToken, accountID, config.zone_id, config.hostname])

  const runRename = async () => {
    if (!name.trim()) return
    setBusy(true)
    setNotice(null)
    try {
      const data = await updatePublishWorkerConfig(config.id, {
        name: name.trim(),
        worker_name: config.worker_name,
        worker_url: config.worker_url,
        worker_dev_url: config.worker_dev_url,
        hostname: config.hostname,
        account_id: config.account_id,
        namespace_id: config.namespace_id,
        zone_id: config.zone_id,
      })
      onSaved(data.config)
      setNotice({ tone: 'success', title: '重命名成功', text: '' })
    } catch (error) {
      setNotice({ tone: 'danger', title: '重命名失败', text: error instanceof Error ? error.message : '请求失败' })
    } finally {
      setBusy(false)
    }
  }

  const runRebind = async () => {
    if (!newHostname || !cfToken) return
    setBusy(true)
    setRebindPhase('binding')
    setNotice(null)
    try {
      const bdData = await bindPublishWorkerDomain({
        token: cfToken,
        account_id: accountID,
        zone_id: zoneID,
        worker_name: config.worker_name,
        hostname: newHostname,
      })
      setRebindPhase('saving')
      const data = await updatePublishWorkerConfig(config.id, {
        name: config.name,
        worker_name: config.worker_name,
        worker_url: bdData.worker_url,
        worker_dev_url: config.worker_dev_url,
        hostname: bdData.hostname,
        account_id: accountID,
        namespace_id: config.namespace_id,
        zone_id: zoneID,
      })
      setRebindPhase('done')
      onSaved(data.config)
      setNotice({ tone: 'success', title: '域名换绑成功', text: `新地址：${bdData.hostname}` })
    } catch (error) {
      setRebindPhase('error')
      setNotice({ tone: 'danger', title: '换绑失败', text: error instanceof Error ? error.message : '请求失败' })
    } finally {
      setBusy(false)
    }
  }

  const runUpdateToken = async () => {
    if (!newToken.trim() || !cfToken) return
    setBusy(true)
    setTokenPhase('deploying')
    setNotice(null)
    try {
      const wkData = await deployPublishWorkerScript({
        token: cfToken,
        account_id: accountID,
        worker_name: config.worker_name,
        namespace_id: config.namespace_id,
        access_token: newToken,
      })
      setTokenPhase('saving')
      const data = await updatePublishWorkerConfig(config.id, {
        name: config.name,
        worker_name: config.worker_name,
        worker_url: config.worker_url,
        worker_dev_url: wkData.worker_dev_url,
        hostname: config.hostname,
        account_id: accountID,
        namespace_id: config.namespace_id,
        zone_id: config.zone_id,
        token: newToken,
      })
      setTokenPhase('done')
      onSaved(data.config)
      setNotice({ tone: 'success', title: 'Token 更新成功', text: '已重新部署 Worker 脚本，旧 Token 立即失效。' })
    } catch (error) {
      setTokenPhase('error')
      setNotice({ tone: 'danger', title: 'Token 更新失败', text: error instanceof Error ? error.message : '请求失败' })
    } finally {
      setBusy(false)
    }
  }

  return (
    <ModalShell
      title="编辑私有仓库"
      description={`${config.name} · ${config.hostname || config.worker_name}`}
      icon={<Database size={18} />}
      onClose={onClose}
      size="md"
    >
      <div className="space-y-4">
        <SegmentedTabs<EditTab>
          value={tab}
          onChange={(v) => { setTab(v); setNotice(null) }}
          items={[
            { value: 'rename', label: '重命名' },
            { value: 'rebind', label: '换绑域名' },
            { value: 'token', label: '更换 Token' },
          ]}
        />

        {/* Rename */}
        {tab === 'rename' && (
          <div className="space-y-3">
            <div>
              <label className="mb-1 block text-xs text-muted">备注名称</label>
              <input
                className="glass-input"
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoFocus
              />
            </div>
          </div>
        )}

        {/* Rebind domain */}
        {tab === 'rebind' && (
          <div className="space-y-3">
            <div className="rounded-lg border border-white/10 bg-white/[0.03] px-3 py-2.5">
              <p className="text-[10px] uppercase tracking-wider text-muted">当前服务地址</p>
              <p className="font-mono text-sm text-slate-300">{config.hostname || config.worker_url || '—'}</p>
            </div>
            {!cfToken ? (
              <InlineNotice tone="warning" title="需要 Cloudflare 凭据">请先在页面顶部完成 CF 凭据配置。</InlineNotice>
            ) : !zonesLoaded ? (
              <div className="flex items-center gap-2 text-xs text-muted">
                <Loader2 size={13} className="animate-spin" /> 正在加载域名列表…
              </div>
            ) : (
              <>
                <div className="grid gap-3 sm:grid-cols-2">
                  <div>
                    <label className="mb-1 block text-xs text-muted">顶级域名</label>
                    <select
                      className="theme-select glass-input"
                      value={zoneID}
                      onChange={(e) => setZoneID(e.target.value)}
                    >
                      {zones.map((z) => <option key={z.id} value={z.id}>{z.name}</option>)}
                    </select>
                  </div>
                  <div>
                    <label className="mb-1 block text-xs text-muted">二级域名前缀</label>
                    <input
                      className="glass-input font-mono"
                      value={subdomain}
                      onChange={(e) => setSubdomain(e.target.value)}
                      placeholder="如 sub / rules"
                    />
                  </div>
                </div>
                {newHostname && (
                  <div className="rounded-lg border border-brand/20 bg-brand/[0.05] px-3 py-2">
                    <p className="text-[10px] uppercase tracking-wider text-muted">新服务地址</p>
                    <p className="font-mono text-sm text-brand-light">{newHostname}</p>
                  </div>
                )}
                {rebindPhase !== '' && rebindPhase !== 'done' && rebindPhase !== 'error' && (
                  <div className="flex items-center gap-2 text-xs text-brand-light">
                    <Loader2 size={13} className="animate-spin" />
                    {rebindPhase === 'binding' ? '正在绑定新域名…' : '正在保存配置…'}
                  </div>
                )}
              </>
            )}
          </div>
        )}

        {/* Update token */}
        {tab === 'token' && (
          <div className="space-y-3">
            <div className="flex items-start gap-2 rounded-lg border border-amber-400/25 bg-amber-400/[0.06] px-3 py-2.5 text-xs text-amber-300">
              <AlertCircle size={13} className="mt-0.5 shrink-0" />
              <div>
                <p className="font-semibold">更换 Token 将重新部署 Worker 脚本</p>
                <p className="mt-0.5 text-amber-200/70">换后所有使用旧 Token 的订阅链接立即失效，请及时更新客户端配置。</p>
              </div>
            </div>
            {!cfToken ? (
              <InlineNotice tone="warning" title="需要 Cloudflare 凭据">请先在页面顶部完成 CF 凭据配置。</InlineNotice>
            ) : (
              <div>
                <label className="mb-1 block text-xs text-muted">新访问 Token</label>
                <div className="flex gap-2">
                  <input
                    className="glass-input font-mono"
                    value={newToken}
                    onChange={(e) => setNewToken(e.target.value.trim())}
                  />
                  <button
                    type="button"
                    className="btn-ghost h-9 shrink-0 px-3 text-xs"
                    onClick={() => setNewToken(randomToken('sub'))}
                  >
                    随机
                  </button>
                </div>
              </div>
            )}
            {tokenPhase !== '' && tokenPhase !== 'done' && tokenPhase !== 'error' && (
              <div className="flex items-center gap-2 text-xs text-brand-light">
                <Loader2 size={13} className="animate-spin" />
                {tokenPhase === 'deploying' ? '正在重新部署 Worker 脚本…' : '正在保存配置…'}
              </div>
            )}
          </div>
        )}

        {notice && (
          <InlineNotice tone={notice.tone} title={notice.title}>{notice.text}</InlineNotice>
        )}

        <div className="flex items-center justify-between gap-2">
          <button className="btn-ghost" onClick={onClose} disabled={busy}>
            {notice?.tone === 'success' ? '关闭' : '取消'}
          </button>
          <div className="flex gap-2">
            {tab === 'rename' && (
              <button
                className="btn-primary"
                onClick={() => { void runRename() }}
                disabled={busy || !name.trim()}
              >
                保存名称
              </button>
            )}
            {tab === 'rebind' && cfToken && zonesLoaded && (
              <button
                className="btn-primary flex items-center gap-2"
                onClick={() => { void runRebind() }}
                disabled={busy || !newHostname}
              >
                {busy && rebindPhase !== 'done' ? <Loader2 size={14} className="animate-spin" /> : null}
                重新绑定
              </button>
            )}
            {tab === 'token' && cfToken && (
              <button
                className="btn-primary flex items-center gap-2"
                onClick={() => { void runUpdateToken() }}
                disabled={busy || !newToken.trim()}
              >
                {busy && tokenPhase !== 'done' ? <Loader2 size={14} className="animate-spin" /> : null}
                更新 Token
              </button>
            )}
          </div>
        </div>
      </div>
    </ModalShell>
  )
}

// ─── WorkerConfigsModal ────────────────────────────────────────────────────

function WorkerConfigsModal({
  onClose,
  configs,
  cfConfig,
  onCreateNew,
  onRefresh,
}: {
  onClose: () => void
  configs: PublishWorkerConfig[]
  cfConfig: CFConfig | null
  onCreateNew: () => void
  onRefresh: () => void
}) {
  const [editConfig, setEditConfig] = useState<PublishWorkerConfig | null>(null)
  const [destroyingID, setDestroyingID] = useState<string | null>(null)
  const [notice, setNotice] = useState<NoticeState | null>(null)

  const handleDestroy = async (cfg: PublishWorkerConfig) => {
    if (!confirm(`确认彻底删除私有仓库「${cfg.name}」？\n\n将同时尝试删除 Cloudflare Worker 脚本和 KV Namespace，并移除所有关联的发布记录。`)) return
    setDestroyingID(cfg.id)
    setNotice(null)
    try {
      const data = await destroyPublishWorkerConfig(cfg.id)
      if (!data.deleted) {
        setNotice({ tone: 'danger', title: '删除失败', text: data.warnings?.join('\n') || '未知错误' })
      } else if (data.warnings?.length) {
        setNotice({
          tone: 'warning',
          title: '本地记录已删除，CF 云端资源清理失败',
          text: '以下 Cloudflare 资源可能需要手动删除：\n' + data.warnings.join('\n'),
        })
        onRefresh()
      } else {
        setNotice({ tone: 'success', title: '删除成功', text: '私有仓库已彻底删除。' })
        onRefresh()
      }
    } catch (error) {
      setNotice({ tone: 'danger', title: '删除失败', text: error instanceof Error ? error.message : '请求失败' })
    } finally {
      setDestroyingID(null)
    }
  }

  return (
    <>
      <ModalShell
        title="私有仓库管理"
        description="每个私有仓库对应一个 Cloudflare Worker + KV 存储，托管你的专属订阅和规则集"
        icon={<Database size={18} />}
        onClose={onClose}
        size="lg"
      >
        <div className="space-y-4">
          {notice && (
            <InlineNotice tone={notice.tone} title={notice.title}>{notice.text}</InlineNotice>
          )}

          {configs.length === 0 ? (
            <EmptyState
              title="暂无私有仓库"
              description="创建私有仓库后，即可发布订阅链接和托管自定义规则集。"
              icon={<Database size={18} />}
              action={(
                <button className="btn-primary flex items-center gap-2" onClick={onCreateNew}>
                  <Plus size={14} />
                  新建私有仓库
                </button>
              )}
            />
          ) : (
            <div className="table-shell overflow-hidden">
              <div className="grid grid-cols-12 gap-3 px-4 py-3 table-header-row">
                <span className="col-span-4">名称 / Worker</span>
                <span className="col-span-4">服务地址</span>
                <span className="col-span-2">Token</span>
                <span className="col-span-2 text-right">操作</span>
              </div>
              {configs.map((cfg) => (
                <div key={cfg.id} className="grid grid-cols-12 items-center gap-3 px-4 py-3 table-row">
                  <div className="col-span-4 min-w-0">
                    <p className="truncate text-sm font-semibold text-slate-100">{cfg.name}</p>
                    <p className="truncate font-mono text-xs text-muted">{cfg.worker_name}</p>
                  </div>
                  <div className="col-span-4 min-w-0">
                    <p className="truncate font-mono text-xs text-slate-300">{cfg.hostname || cfg.worker_url || '—'}</p>
                  </div>
                  <div className="col-span-2">
                    {cfg.has_token
                      ? <span className="text-xs text-emerald-400">Token ✓</span>
                      : <span className="text-xs text-amber-400">缺失</span>}
                  </div>
                  <div className="col-span-2 flex items-center justify-end gap-1.5">
                    <button
                      className="btn-icon-sm btn-ghost"
                      title="编辑"
                      onClick={() => setEditConfig(cfg)}
                    >
                      <Pencil size={14} />
                    </button>
                    <button
                      className="btn-icon-sm btn-ghost text-danger hover:bg-danger/10"
                      title="彻底删除（含 CF 资源）"
                      disabled={destroyingID === cfg.id}
                      onClick={() => { void handleDestroy(cfg) }}
                    >
                      {destroyingID === cfg.id
                        ? <Loader2 size={14} className="animate-spin" />
                        : <Trash2 size={14} />}
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}

          <div className="flex items-center justify-between gap-2">
            <button className="btn-ghost flex items-center gap-2" onClick={onCreateNew}>
              <Plus size={14} />
              新建私有仓库
            </button>
            <button className="btn-ghost" onClick={onClose}>关闭</button>
          </div>
        </div>
      </ModalShell>

      {editConfig && (
        <WorkerConfigEditModal
          onClose={() => setEditConfig(null)}
          config={editConfig}
          cfConfig={cfConfig}
          onSaved={(updated) => {
            setEditConfig(null)
            // Update the local entry without full reload for instant feedback
            onRefresh()
            setNotice({ tone: 'success', title: '已保存', text: `「${updated.name}」已更新。` })
          }}
        />
      )}
    </>
  )
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
  const [savedConfig, setSavedConfig] = useState<PublishWorkerConfig | null>(null)
  const [zones, setZones] = useState<CloudflareZone[]>([])
  const [domainSuggestions, setDomainSuggestions] = useState<string[]>([])
  const [subdomain, setSubdomain] = useState('')
  const [copiedWorkerURL, setCopiedWorkerURL] = useState(false)
  const [copiedPreviewURL, setCopiedPreviewURL] = useState(false)
  const [deployPhase, setDeployPhase] = useState<'' | 'namespace' | 'worker' | 'domain' | 'verify' | 'done' | 'error'>('')
  const [previewURL, setPreviewURL] = useState('')
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

  const runLoadZones = async () => {
    // Called when transitioning from Step 1 → Step 2
    if (!cfToken || !form.account_id) {
      setNotice({ tone: 'warning', title: 'Cloudflare 凭据不可用', text: '请先在页面顶部完成 Cloudflare 凭据配置后再继续。' })
      return
    }
    setBusy(true)
    setNotice(null)
    try {
      const data = await getCloudflareZones({ cf_token: cfToken, cf_account_id: form.account_id })
      const list = data.zones ?? []
      setZones(list)
      if (list.length === 0) {
        setNotice({ tone: 'warning', title: '未读取到域名', text: '当前账号下没有可用 Zone，请先在 Cloudflare 托管域名后再试。' })
        return
      }
      const nextZoneID = list.some((item) => item.id === form.zone_id) ? form.zone_id : list[0].id
      update('zone_id', nextZoneID)
      setStep(2)
    } catch (error) {
      const message = error instanceof Error ? error.message : '请求失败'
      setNotice({ tone: 'danger', title: '凭据验证失败', text: message })
    } finally {
      setBusy(false)
    }
  }

  // One-click full deploy: namespace → worker → bind domain → verify+save
  const runFullDeploy = async () => {
    if (!form.zone_id || !fullHostname) {
      setNotice({ tone: 'warning', title: '请先选择域名', text: '请选择顶级域名并填写二级域名前缀。' })
      return
    }
    if (!cfToken || !form.account_id) {
      setNotice({ tone: 'warning', title: 'Cloudflare 凭据不可用', text: '请先完成凭据配置。' })
      return
    }
    setBusy(true)
    setNotice(null)
    setLogs([])
    setDeployPhase('namespace')
    try {
      // 1. Create KV namespace
      addLog('创建 KV', 'info', `创建 KV Namespace（${form.worker_name}）`)
      const nsData = await createPublishWorkerNamespace({
        token: cfToken,
        account_id: form.account_id,
        worker_name: form.worker_name,
      })
      update('namespace_id', nsData.namespace_id)
      addLog('创建 KV', 'success', nsData.reused ? '复用已有 Namespace' : 'Namespace 创建成功', nsData.namespace_id)
      const namespace_id = nsData.namespace_id

      // 2. Deploy worker script
      setDeployPhase('worker')
      addLog('部署 Worker', 'info', '上传 Worker 脚本至 Cloudflare')
      const wkData = await deployPublishWorkerScript({
        token: cfToken,
        account_id: form.account_id,
        worker_name: form.worker_name,
        namespace_id,
        access_token: form.access_token,
      })
      update('worker_dev_url', wkData.worker_dev_url)
      addLog('部署 Worker', 'success', 'Worker 部署成功', wkData.worker_dev_url)

      // 3. Bind custom domain
      setDeployPhase('domain')
      addLog('绑定域名', 'info', `绑定 ${fullHostname}`)
      const bdData = await bindPublishWorkerDomain({
        token: cfToken,
        account_id: form.account_id,
        zone_id: form.zone_id,
        worker_name: form.worker_name,
        hostname: fullHostname,
      })
      update('worker_url', bdData.worker_url)
      update('hostname', bdData.hostname)
      addLog('绑定域名', 'success', '域名绑定成功', `${bdData.hostname} → ${bdData.worker_url}`)
      const worker_url = bdData.worker_url
      const hostname = bdData.hostname

      // 4. Verify + save
      setDeployPhase('verify')
      addLog('验证保存', 'info', '验证连通性并写入 ClashForge')
      const vsData = await verifyAndSavePublishWorker({
        name: form.name || form.worker_name,
        worker_name: form.worker_name,
        worker_url,
        worker_dev_url: wkData.worker_dev_url,
        hostname,
        account_id: form.account_id,
        namespace_id,
        zone_id: form.zone_id,
        access_token: form.access_token,
      })
      if (!vsData.result.ok || !vsData.config) {
        const failed = vsData.result.tests.filter((t) => !t.ok).map((t) => t.name).join('、')
        addLog('验证保存', 'error', `验证未通过：${failed}`)
        setNotice({ tone: 'warning', title: '部署完成但验证未通过', text: 'DNS 可能尚未生效，稍后关闭窗口重试即可。' })
        setDeployPhase('error')
        return
      }
      const passed = vsData.result.tests.filter((t) => t.ok).length
      addLog('验证保存', 'success', `验证通过（${passed}/${vsData.result.tests.length}）`, vsData.result.used_url || vsData.result.hello_url)

      // Build preview URL with token (strip any token already in the URL first)
      const helloBase = vsData.result.hello_url || vsData.result.used_url || worker_url
      const helloStripped = helloBase.replace(/([?&])token=[^&]*/g, '$1').replace(/[?&]$/, '')
      const sep = helloStripped.includes('?') ? '&' : '?'
      const preview = `${helloStripped}${sep}token=${encodeURIComponent(form.access_token)}`
      setPreviewURL(preview)
      setSavedConfig(vsData.config)
      setDeployPhase('done')
      setNotice(null)
      onSaved(vsData.config)
    } catch (error) {
      const message = error instanceof Error ? error.message : '请求失败'
      addLog('部署', 'error', '部署失败', message)
      setNotice({ tone: 'danger', title: '部署失败', text: message })
      setDeployPhase('error')
    } finally {
      setBusy(false)
    }
  }

  const copyWorkerEndpoint = async () => {
    const target = ensureHttps(savedConfig?.worker_url || form.worker_url)
    if (!target) return
    await copyText(target)
    setCopiedWorkerURL(true)
    setTimeout(() => setCopiedWorkerURL(false), 1500)
  }

  const copyPreview = async () => {
    if (!previewURL) return
    await copyText(previewURL)
    setCopiedPreviewURL(true)
    setTimeout(() => setCopiedPreviewURL(false), 1500)
  }

  const ensureHttps = (url: string) => {
    if (!url) return url
    if (url.startsWith('http://') || url.startsWith('https://')) return url
    return 'https://' + url
  }

  const stepLabels = ['了解私有仓库', '配置服务域名', '一键部署'] as const

  const isWizardDirty = form.zone_id !== '' || step > 1
  const handleWizardBeforeClose = () => {
    if (busy) return false
    if (savedConfig) return true
    if (!isWizardDirty) return true
    return window.confirm('确认放弃已输入的内容并关闭？')
  }

  const phaseLabel: Record<string, string> = {
    '': '准备就绪',
    namespace: '正在创建 KV 存储…',
    worker: '正在部署 Worker 脚本…',
    domain: '正在绑定域名…',
    verify: '正在验证连通性…',
    done: '部署完成',
    error: '部署遇到错误',
  }

  return (
    <ModalShell
      title="新建私有仓库"
      description="基于 Cloudflare Worker — 为订阅分发和规则集托管提供稳定的专属地址"
      icon={<CloudCog size={18} />}
      onClose={onClose}
      onBeforeClose={handleWizardBeforeClose}
      size="xl"
    >
      <div className="space-y-5">

        {/* Step indicator */}
        <div className="flex items-center gap-1">
          {stepLabels.map((label, idx) => {
            const index = idx + 1
            const done = step > index
            const active = step === index
            return (
              <div key={label} className="flex min-w-0 flex-1 flex-col items-center gap-1">
                <div className={[
                  'flex h-6 w-6 items-center justify-center rounded-full text-[11px] font-bold ring-2 ring-inset',
                  done ? 'bg-emerald-500/20 text-emerald-300 ring-emerald-500/40'
                    : active ? 'bg-brand/20 text-brand-light ring-brand/50'
                    : 'bg-white/5 text-muted ring-white/10',
                ].join(' ')}>
                  {done ? <CheckCircle2 size={12} /> : index}
                </div>
                <p className={[
                  'hidden truncate text-center text-[10px] sm:block',
                  done ? 'text-emerald-400/70' : active ? 'text-brand-light' : 'text-muted/50',
                ].join(' ')}>
                  {label}
                </p>
              </div>
            )
          })}
        </div>

        {/* Step content */}
        <div className="rounded-xl border border-white/10 bg-black/20 p-4 space-y-4">

          {/* ── Step 1: Explanation */}
          {step === 1 && (
            <div className="space-y-4">
              <p className="text-xs font-semibold text-muted uppercase tracking-widest">步骤 1 · 了解私有仓库</p>

              {/* Credential status */}
              <div className={[
                'flex items-center gap-2 rounded-lg border px-3 py-2 text-xs',
                cfToken && form.account_id
                  ? 'border-emerald-500/30 bg-emerald-500/[0.07] text-emerald-300'
                  : 'border-amber-400/30 bg-amber-400/[0.07] text-amber-300',
              ].join(' ')}>
                {cfToken && form.account_id ? <CheckCircle2 size={13} /> : <AlertCircle size={13} />}
                {cfToken && form.account_id
                  ? 'Cloudflare 凭据已配置，可以继续'
                  : '⚠️ 请先在页面顶部完成 Cloudflare 凭据配置'}
              </div>

              {/* Info cards */}
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="rounded-lg border border-brand/20 bg-brand/[0.05] p-3 space-y-1.5">
                  <div className="flex items-center gap-1.5 text-xs font-semibold text-brand-light">
                    <Server size={13} />
                    什么是私有仓库？
                  </div>
                  <p className="text-xs leading-relaxed text-muted">
                    私有仓库 = 一个绑定到你自有域名的
                    <span className="text-slate-200"> Cloudflare Worker + KV 存储</span>，
                    让你的订阅配置和规则集拥有
                    <strong className="text-slate-100">永久固定的专属 URL</strong>，且仅凭 Token 才可访问。
                  </p>
                </div>
                <div className="rounded-lg border border-cta/20 bg-cta/[0.05] p-3 space-y-1.5">
                  <div className="flex items-center gap-1.5 text-xs font-semibold text-cta-light">
                    <FileCode2 size={13} />
                    为什么发布订阅需要它？
                  </div>
                  <p className="text-xs leading-relaxed text-muted">
                    Mihomo / Clash 客户端需要一个<span className="text-slate-200">公网可访问的 URL</span>
                    才能拉取配置。私有仓库负责存储 ClashForge 推送的配置，并在客户端需要时安全返回。
                  </p>
                </div>
                <div className="rounded-lg border border-amber-400/20 bg-amber-400/[0.05] p-3 space-y-1.5">
                  <div className="flex items-center gap-1.5 text-xs font-semibold text-amber-300">
                    <ListPlus size={13} />
                    为什么自定义规则集需要它？
                  </div>
                  <p className="text-xs leading-relaxed text-muted">
                    自定义规则集（rule-provider）同样需要稳定的 URL 供 Mihomo 定期拉取更新。KV 存储让规则集 URL
                    <strong className="text-slate-100">写入即永久固定</strong>，修改规则内容不影响链接。
                  </p>
                </div>
                <div className="rounded-lg border border-white/10 bg-white/[0.04] p-3 space-y-1.5">
                  <div className="flex items-center gap-1.5 text-xs font-semibold text-slate-300">
                    <Wand2 size={13} />
                    创建后能做什么？
                  </div>
                  <p className="text-xs leading-relaxed text-muted">
                    创建完成后，即可使用「<span className="text-slate-200">发布新订阅</span>」生成订阅链接，
                    或使用「<span className="text-slate-200">新建规则集</span>」托管自定义规则，两者都会自动选用此服务。
                  </p>
                </div>
              </div>
            </div>
          )}

          {/* ── Step 2: Domain configuration */}
          {step === 2 && (
            <div className="space-y-4">
              <p className="text-xs font-semibold text-muted uppercase tracking-widest">步骤 2 · 配置服务域名</p>

              <div className="grid gap-3 sm:grid-cols-2">
                <div>
                  <label className="mb-1 block text-xs text-muted">顶级域名</label>
                  <select
                    className="theme-select glass-input"
                    value={form.zone_id}
                    onChange={(e) => update('zone_id', e.target.value)}
                  >
                    {zones.map((item) => (
                      <option key={item.id} value={item.id}>{item.name}</option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-xs text-muted">二级域名前缀</label>
                  <input
                    className="glass-input font-mono"
                    value={subdomain}
                    onChange={(e) => setSubdomain(e.target.value)}
                    placeholder="例如 sub / rules / proxy"
                  />
                </div>
              </div>

              {/* Quick-pick suggestions */}
              {domainSuggestions.length > 0 && (
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
              )}

              {/* Hostname preview */}
              <div className="flex items-center justify-between gap-3 rounded-lg border border-white/10 bg-white/[0.03] px-3 py-2.5">
                <div>
                  <p className="mb-0.5 text-[10px] uppercase tracking-wider text-muted">将绑定的服务地址</p>
                  <p className="font-mono text-sm text-slate-100">{fullHostname || '请填写上方域名'}</p>
                </div>
              </div>

              {/* Root domain warning */}
              <div className="flex items-start gap-2 rounded-lg border border-amber-400/25 bg-amber-400/[0.06] px-3 py-2.5 text-xs text-amber-300">
                <AlertCircle size={13} className="mt-0.5 shrink-0" />
                <div>
                  <p className="mb-0.5 font-semibold">不建议直接使用顶级域名</p>
                  <p className="text-amber-200/70">
                    直接绑定到{' '}
                    <code className="rounded bg-black/20 px-1 font-mono">{selectedZone?.name || 'example.com'}</code>
                    {' '}会把所有流量路由至 Worker，可能干扰主站、邮件等现有服务。建议使用二级域名（如{' '}
                    <code className="rounded bg-black/20 px-1 font-mono">sub.{selectedZone?.name || 'example.com'}</code>
                    ）以隔离影响。
                  </p>
                </div>
              </div>

              {/* Advanced: worker name + token */}
              <details className="rounded-lg border border-white/10 bg-white/[0.02]">
                <summary className="cursor-pointer select-none px-3 py-2.5 text-xs text-muted hover:text-slate-200">
                  高级配置（Worker 名称 / 访问 Token）
                </summary>
                <div className="grid gap-3 border-t border-white/10 p-3 sm:grid-cols-2">
                  <div>
                    <label className="mb-1 block text-xs text-muted">Worker 名称</label>
                    <input
                      className="glass-input font-mono"
                      value={form.worker_name}
                      onChange={(e) => update('worker_name', e.target.value.trim())}
                      placeholder="自动生成"
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs text-muted">服务备注名称</label>
                    <input
                      className="glass-input"
                      value={form.name}
                      onChange={(e) => update('name', e.target.value)}
                      placeholder="自动生成"
                    />
                  </div>
                  <div className="sm:col-span-2">
                    <label className="mb-1 block text-xs text-muted">订阅访问 Token</label>
                    <div className="flex gap-2">
                      <input
                        className="glass-input font-mono"
                        value={form.access_token}
                        onChange={(e) => update('access_token', e.target.value.trim())}
                        placeholder="自动生成"
                      />
                      <button
                        type="button"
                        className="btn-ghost h-9 shrink-0 px-3 text-xs"
                        onClick={() => update('access_token', randomToken('sub'))}
                      >
                        随机
                      </button>
                    </div>
                  </div>
                </div>
              </details>
            </div>
          )}

          {/* ── Step 3: One-click deploy */}
          {step === 3 && (
            <div className="space-y-4">
              <p className="text-xs font-semibold text-muted uppercase tracking-widest">步骤 3 · 一键部署</p>

              {/* Pre-deploy summary */}
              {deployPhase === '' && (
                <div className="grid gap-2 text-xs">
                  <div className="flex items-center justify-between rounded-lg border border-white/10 bg-white/[0.03] px-3 py-2">
                    <span className="text-muted">服务地址</span>
                    <span className="font-mono text-slate-200">{fullHostname}</span>
                  </div>
                  <div className="flex items-center justify-between rounded-lg border border-white/10 bg-white/[0.03] px-3 py-2">
                    <span className="text-muted">Worker 名称</span>
                    <span className="font-mono text-slate-200">{form.worker_name}</span>
                  </div>
                  <div className="flex items-center justify-between rounded-lg border border-white/10 bg-white/[0.03] px-3 py-2">
                    <span className="text-muted">访问 Token</span>
                    <span className="font-mono text-slate-200">{maskSecret(form.access_token)}</span>
                  </div>
                  <p className="px-1 text-xs text-muted">
                    点击「开始部署」将自动完成：创建 KV 存储 → 部署 Worker 脚本 → 绑定自定义域名 → 验证连通性。
                  </p>
                </div>
              )}

              {/* Phase indicator while deploying */}
              {deployPhase !== '' && deployPhase !== 'done' && (
                <div className={[
                  'flex items-center gap-2 rounded-lg border px-3 py-2.5 text-xs',
                  deployPhase === 'error'
                    ? 'border-danger/30 bg-danger/[0.07] text-danger'
                    : 'border-brand/20 bg-brand/[0.05] text-brand-light',
                ].join(' ')}>
                  {deployPhase === 'error'
                    ? <AlertCircle size={13} className="shrink-0" />
                    : <Loader2 size={13} className="shrink-0 animate-spin" />}
                  <span>{phaseLabel[deployPhase] ?? '部署中…'}</span>
                </div>
              )}

              {/* Progress log */}
              {logs.length > 0 && (
                <div className="max-h-[140px] space-y-1 overflow-y-auto rounded-lg border border-white/10 bg-black/30 p-2.5 font-mono text-[11px]">
                  {logs.map((item, idx) => (
                    <div
                      key={`${item.ts}-${idx}`}
                      className={[
                        'flex items-start gap-1.5',
                        item.status === 'error' ? 'text-red-300'
                          : item.status === 'success' ? 'text-emerald-300'
                          : 'text-slate-400',
                      ].join(' ')}
                    >
                      <span className="shrink-0 text-muted">[{item.ts}]</span>
                      <span>{item.message}</span>
                      {item.detail && <span className="truncate text-muted">{item.detail}</span>}
                    </div>
                  ))}
                </div>
              )}

              {/* Completion panel */}
              {deployPhase === 'done' && savedConfig && (
                <div className="space-y-3 rounded-xl border border-emerald-500/25 bg-emerald-500/[0.05] p-4">
                  <div className="flex items-center gap-2">
                    <CheckCircle2 size={16} className="text-emerald-400" />
                    <p className="text-sm font-semibold text-emerald-300">私有仓库已就绪！</p>
                  </div>
                  <p className="text-xs text-emerald-300/80">
                    现在可以使用「发布新订阅」和「新建规则集」，系统会自动选用此服务。
                  </p>

                  {/* Token security warning */}
                  <div className="flex items-start gap-2 rounded-lg border border-red-400/30 bg-red-400/[0.08] px-3 py-2.5 text-xs text-red-300">
                    <AlertCircle size={13} className="mt-0.5 shrink-0" />
                    <div>
                      <p className="mb-0.5 font-semibold">⚠️ 请妥善保管 Token，切勿泄漏给他人</p>
                      <p className="text-red-200/70">
                        Token 是访问订阅和规则集的唯一凭证。泄漏后他人可读取你的完整代理配置。
                        建议仅在可信设备上使用带 Token 的链接。如需更换 Token，须重新部署此服务。
                      </p>
                    </div>
                  </div>

                  {/* Service URL */}
                  <div className="rounded-lg border border-white/10 bg-black/20 p-2.5">
                    <p className="mb-1.5 text-[10px] uppercase tracking-wider text-muted">服务地址</p>
                    <div className="flex items-center gap-2">
                      <code className="min-w-0 flex-1 break-all font-mono text-xs text-emerald-200">
                        {ensureHttps(savedConfig.worker_url)}
                      </code>
                      <button className="btn-ghost h-7 shrink-0 px-2.5 text-xs" onClick={() => { void copyWorkerEndpoint() }}>
                        <Copy size={12} className={copiedWorkerURL ? 'text-success' : ''} />
                        {copiedWorkerURL ? '已复制' : '复制'}
                      </button>
                    </div>
                  </div>

                  {/* Preview URL with token */}
                  {previewURL && (
                    <div className="rounded-lg border border-white/10 bg-black/20 p-2.5">
                      <p className="mb-1.5 text-[10px] uppercase tracking-wider text-muted">预览地址（含 Token）</p>
                      <div className="flex items-center gap-2">
                        <code className="min-w-0 flex-1 break-all font-mono text-xs text-slate-300">{previewURL}</code>
                        <div className="flex shrink-0 gap-1.5">
                          <button className="btn-ghost h-7 px-2.5 text-xs" onClick={() => { void copyPreview() }}>
                            <Copy size={12} className={copiedPreviewURL ? 'text-success' : ''} />
                            {copiedPreviewURL ? '已复制' : '复制'}
                          </button>
                          <a
                            href={previewURL}
                            target="_blank"
                            rel="noreferrer"
                            className="btn-ghost flex h-7 items-center gap-1 px-2.5 text-xs"
                          >
                            <ExternalLink size={12} />
                            预览
                          </a>
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}

        </div>

        {/* Notice */}
        {notice && (
          <InlineNotice tone={notice.tone} title={notice.title}>
            {notice.text}
          </InlineNotice>
        )}

        {/* Action buttons */}
        <div className="flex items-center justify-between gap-2">
          <button className="btn-ghost" onClick={onClose} disabled={busy}>
            {savedConfig ? '关闭' : '取消'}
          </button>

          <div className="flex items-center gap-2">
            {step === 1 && (
              <button
                className="btn-primary flex items-center gap-2"
                onClick={runLoadZones}
                disabled={busy || !cfToken || !form.account_id}
              >
                {busy ? <Loader2 size={14} className="animate-spin" /> : null}
                我了解了，开始配置
              </button>
            )}
            {step === 2 && (
              <button
                className="btn-primary flex items-center gap-2"
                onClick={() => setStep(3)}
                disabled={!fullHostname}
              >
                <Rocket size={14} />
                下一步：确认部署
              </button>
            )}
            {step === 3 && deployPhase === '' && (
              <button className="btn-cta flex items-center gap-2" onClick={runFullDeploy} disabled={busy}>
                <Rocket size={14} />
                开始部署
              </button>
            )}
            {step === 3 && deployPhase === 'error' && (
              <button
                className="btn-ghost flex items-center gap-2"
                onClick={() => { setDeployPhase(''); setLogs([]); setNotice(null) }}
              >
                重置并重试
              </button>
            )}
            {deployPhase === 'done' && savedConfig && (
              <button className="btn-primary flex items-center gap-2" onClick={onClose}>
                <CheckCircle2 size={14} />
                完成
              </button>
            )}
          </div>
        </div>

      </div>
    </ModalShell>
  )
}


// ─── Annotated YAML preview helpers ───────────────────────────────────────

function sanitizeRuleSetKeyJS(name: string): string {
  let key = name.toLowerCase().replace(/[^a-z0-9]/g, '_')
  key = key.replace(/^_+|_+$/g, '')
  key = key.replace(/_+/g, '_')
  return key || 'ruleset'
}

type AnnotatedLine = { text: string; tag: 'rule' | 'provider' | null; ruleSetName?: string }

function annotateYamlLines(content: string, ruleSets: RuleSet[], selectedIDs: string[]): AnnotatedLine[] {
  if (!content) return []
  const selected = ruleSets.filter((rs) => selectedIDs.includes(rs.id))
  if (selected.length === 0) return content.split('\n').map((text) => ({ text, tag: null }))

  const keyToName: Record<string, string> = {}
  for (const rs of selected) keyToName[sanitizeRuleSetKeyJS(rs.name)] = rs.name

  const lines = content.split('\n')
  const result: AnnotatedLine[] = []
  let providerBlockLeft = 0
  let providerName = ''

  for (const line of lines) {
    const trimmed = line.trim()

    // Injected RULE-SET rules in the rules: section
    const ruleKey = Object.keys(keyToName).find((k) => trimmed.startsWith(`- RULE-SET,${k},`))
    if (ruleKey) {
      result.push({ text: line, tag: 'rule', ruleSetName: keyToName[ruleKey] })
      providerBlockLeft = 0
      continue
    }

    // Continuation of a provider block
    if (providerBlockLeft > 0) {
      result.push({ text: line, tag: 'provider', ruleSetName: providerName })
      providerBlockLeft--
      continue
    }

    // Provider key header: "    <key>:" in rule-providers section
    const providerKey = Object.keys(keyToName).find((k) => trimmed === `${k}:`)
    if (providerKey) {
      result.push({ text: line, tag: 'provider', ruleSetName: keyToName[providerKey] })
      providerBlockLeft = 5 // type, behavior, url, interval, format
      providerName = keyToName[providerKey]
      continue
    }

    result.push({ text: line, tag: null })
  }
  return result
}

function AnnotatedYamlView({
  lines,
  loading,
}: {
  lines: AnnotatedLine[]
  loading: boolean
}) {
  if (loading) return <div className="flex min-h-[160px] items-center justify-center"><Loader2 size={14} className="animate-spin text-brand" /></div>
  if (lines.length === 0) return <div className="flex min-h-[160px] items-center justify-center"><p className="text-xs text-muted">暂无预览</p></div>

  const labeled = new Set<string>()
  return (
    <div className="max-h-[420px] min-h-[200px] overflow-auto rounded-lg bg-black/40 p-2 font-mono text-[11px] leading-[1.65]">
      {lines.map((line, i) => {
        const showBadge = Boolean(line.ruleSetName && !labeled.has(line.ruleSetName))
        if (showBadge && line.ruleSetName) labeled.add(line.ruleSetName)
        return (
          <div
            key={i}
            className={[
              'flex items-baseline gap-1.5 rounded px-1',
              line.tag === 'rule' ? 'bg-emerald-500/15' : line.tag === 'provider' ? 'bg-amber-500/10' : '',
            ].join(' ')}
          >
            <span className="w-7 shrink-0 select-none text-right text-white/20">{i + 1}</span>
            <span className={[
              'flex-1 whitespace-pre',
              line.tag === 'rule' ? 'text-emerald-300' : line.tag === 'provider' ? 'text-amber-200/80' : 'text-slate-300',
            ].join(' ')}>
              {line.text || ' '}
            </span>
            {showBadge && (
              <span className={[
                'shrink-0 rounded px-1.5 py-px text-[9px] font-bold uppercase tracking-wide',
                line.tag === 'rule' ? 'bg-emerald-500/25 text-emerald-300' : 'bg-amber-500/20 text-amber-300',
              ].join(' ')}>
                {line.ruleSetName}
              </span>
            )}
          </div>
        )
      })}
    </div>
  )
}

// ─── PublishWizardModal ────────────────────────────────────────────────────

function PublishWizardModal({
  onClose,
  onPublished,
  nodes,
  templates,
  workerConfigs: initialWorkerConfigs,
  ruleSets,
  cfConfig,
}: {
  onClose: () => void
  onPublished: () => void
  nodes: PublishNode[]
  templates: PublishTemplatePreset[]
  workerConfigs: PublishWorkerConfig[]
  ruleSets: RuleSet[]
  cfConfig?: CFConfig | null
}) {
  const navigate = useNavigate()
  const [step, setStep] = useState<1 | 2 | 3>(1)
  const [notice, setNotice] = useState<NoticeState | null>(null)

  const [templateMode, setTemplateMode] = useState<PublishTemplateMode>('builtin')
  const [templateID, setTemplateID] = useState('loyalsoldier_standard')
  const [templateContent, setTemplateContent] = useState('')

  const [selectedNodes, setSelectedNodes] = useState<string[]>(() => nodes.slice(0, 3).map((n) => n.id))
  const [selectedRuleSetIDs, setSelectedRuleSetIDs] = useState<string[]>([])

  const [workerConfigs, setWorkerConfigs] = useState(initialWorkerConfigs)
  const [selectedWorkerConfigID, setSelectedWorkerConfigID] = useState(initialWorkerConfigs[0]?.id ?? '')
  const [publishing, setPublishing] = useState(false)
  const [latestPublish, setLatestPublish] = useState<{ access_url: string; file_name: string; version: number } | null>(null)
  const [copiedLink, setCopiedLink] = useState(false)
  const [showNestedWizard, setShowNestedWizard] = useState(false)

  const [previewContent, setPreviewContent] = useState('')
  const [previewLoading, setPreviewLoading] = useState(false)
  const previewTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const selectedWorkerConfig = useMemo(
    () => workerConfigs.find((c) => c.id === selectedWorkerConfigID) ?? null,
    [workerConfigs, selectedWorkerConfigID],
  )

  const toggleNode = (id: string) =>
    setSelectedNodes((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))

  const toggleRuleSet = (id: string) =>
    setSelectedRuleSetIDs((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))

  const refreshPreview = useCallback(async () => {
    setPreviewLoading(true)
    try {
      const data = await previewPublishConfig({
        node_ids: selectedNodes,
        template_mode: templateMode,
        template_id: templateMode === 'builtin' ? templateID : undefined,
        template_content: templateMode === 'custom' ? templateContent : undefined,
        rule_set_ids: selectedRuleSetIDs.length > 0 ? selectedRuleSetIDs : undefined,
      })
      setPreviewContent(data.content)
    } catch {
      // silent — preview is supplementary
    } finally {
      setPreviewLoading(false)
    }
  }, [selectedNodes, templateMode, templateID, templateContent, selectedRuleSetIDs])

  useEffect(() => {
    if (previewTimerRef.current) clearTimeout(previewTimerRef.current)
    previewTimerRef.current = setTimeout(() => { void refreshPreview() }, 700)
    return () => { if (previewTimerRef.current) clearTimeout(previewTimerRef.current) }
  }, [refreshPreview])

  const runPublish = async () => {
    if (!selectedWorkerConfigID) {
      setNotice({ tone: 'warning', title: '请选择私有仓库', text: '' })
      return
    }
    if (selectedNodes.length === 0) {
      setNotice({ tone: 'warning', title: '请先选择至少一个节点', text: '回到第二步勾选节点。' })
      return
    }
    setPublishing(true)
    setNotice(null)
    try {
      const data = await uploadPublishConfig({
        worker_config_id: selectedWorkerConfigID,
        base_name: 'clashforge',
        content: previewContent || undefined,
        node_ids: selectedNodes,
        template_mode: templateMode,
        template_id: templateMode === 'builtin' ? templateID : undefined,
        template_content: templateMode === 'custom' ? templateContent : undefined,
        rule_set_ids: selectedRuleSetIDs.length > 0 ? selectedRuleSetIDs : undefined,
      })
      setLatestPublish({ access_url: data.access_url, file_name: data.file_name, version: data.version })
      onPublished()
      try { await copyText(data.access_url) } catch { /* ignore */ }
      setNotice({ tone: 'success', title: '发布成功', text: `${data.file_name}（v${data.version}）已发布，链接已复制。` })
    } catch (error) {
      setNotice({ tone: 'danger', title: '发布失败', text: error instanceof Error ? error.message : '请求失败' })
    } finally {
      setPublishing(false)
    }
  }

  const removeCurrentWorker = async () => {
    if (!selectedWorkerConfigID) return
    if (!confirm('确认删除当前私有仓库？')) return
    try {
      await deletePublishWorkerConfig(selectedWorkerConfigID)
      const next = workerConfigs.filter((c) => c.id !== selectedWorkerConfigID)
      setWorkerConfigs(next)
      setSelectedWorkerConfigID(next[0]?.id ?? '')
    } catch (error) {
      setNotice({ tone: 'danger', title: '删除失败', text: error instanceof Error ? error.message : '请求失败' })
    }
  }

  const stepLabels = ['选择模板', '节点与规则', '发布'] as const

  return (
    <>
      <ModalShell
        title="发布新订阅"
        description="按步骤生成配置并推送到私有仓库。"
        icon={<UploadCloud size={18} />}
        onClose={onClose}
        onBeforeClose={() => {
          if (latestPublish) return true
          const dirty = selectedNodes.length > 0 || templateMode !== 'builtin' || templateContent !== '' || selectedRuleSetIDs.length > 0
          if (!dirty) return true
          return confirm('确认关闭？已填写的内容将会丢失。')
        }}
        size="2xl"
      >
        <div className="space-y-4 p-1">

          {/* Step indicator */}
          <div className="flex items-center gap-1">
            {stepLabels.map((label, idx) => {
              const index = idx + 1
              const done = step > index
              const active = step === index
              return (
                <div key={label} className="flex min-w-0 flex-1 flex-col items-center gap-1">
                  <div className={[
                    'flex h-6 w-6 items-center justify-center rounded-full text-[11px] font-bold ring-2 ring-inset',
                    done ? 'bg-emerald-500/20 text-emerald-300 ring-emerald-500/40'
                      : active ? 'bg-brand/20 text-brand-light ring-brand/50'
                      : 'bg-white/5 text-muted ring-white/10',
                  ].join(' ')}>
                    {done ? <CheckCircle2 size={12} /> : index}
                  </div>
                  <p className={[
                    'hidden truncate text-center text-[10px] sm:block',
                    done ? 'text-emerald-400/70' : active ? 'text-brand-light' : 'text-muted/50',
                  ].join(' ')}>
                    {label}
                  </p>
                </div>
              )
            })}
          </div>

          {/* Main content: step form */}
          <div className="space-y-4">

            {/* Left: step content */}
            <div className="min-w-0 space-y-4">

              {/* Step 1: Template */}
              {step === 1 && (
                <div className="space-y-3">
                  <p className="text-xs font-semibold uppercase tracking-widest text-muted">① 选择模板来源</p>
                  <SegmentedTabs<PublishTemplateMode>
                    value={templateMode}
                    onChange={setTemplateMode}
                    items={[
                      { value: 'builtin', label: '内置模板', icon: <Wand2 size={14} /> },
                      { value: 'runtime', label: '当前配置', icon: <Rocket size={14} /> },
                      { value: 'custom', label: '自定义', icon: <FileCode2 size={14} /> },
                    ]}
                  />
                  {templateMode === 'builtin' && (
                    <div>
                      <label className="mb-1.5 block text-xs text-muted">内置模板</label>
                      <select
                        className="theme-select glass-input"
                        value={templateID}
                        onChange={(e) => setTemplateID(e.target.value)}
                      >
                        {(templates.length > 0 ? templates : [{ id: 'loyalsoldier_standard', name: 'Loyalsoldier 标准规则', description: '' }]).map((t) => (
                          <option key={t.id} value={t.id}>{t.name}</option>
                        ))}
                      </select>
                      <p className="mt-1.5 text-xs text-muted">社区维护的规则集，覆盖常见国内外分流场景，开箱即用。</p>
                    </div>
                  )}
                  {templateMode === 'custom' && (
                    <div>
                      <label className="mb-1.5 block text-xs text-muted">自定义 YAML 模板</label>
                      <textarea
                        className="glass-textarea h-[200px] resize-y text-xs leading-5"
                        value={templateContent}
                        onChange={(e) => setTemplateContent(e.target.value)}
                        placeholder="粘贴完整 Clash YAML 模板，系统会自动注入所选节点。"
                      />
                    </div>
                  )}
                  {templateMode === 'runtime' && (
                    <p className="rounded-md border border-white/12 bg-white/5 px-3 py-2.5 text-xs text-muted">
                      将读取当前运行的 <code className="font-mono">mihomo-config.yaml</code> 作为模板基底，自动注入所选节点。
                    </p>
                  )}
                </div>
              )}

              {/* Step 2: Nodes + Rule Sets */}
              {step === 2 && (
                <div className="space-y-5">
                  <div>
                    <div className="mb-2 flex items-center justify-between">
                      <p className="text-xs font-semibold uppercase tracking-widest text-muted">② 选择代理节点</p>
                      <div className="flex gap-1">
                        <button className="btn-ghost h-6 px-2 text-[11px]" onClick={() => setSelectedNodes(nodes.map((n) => n.id))}>全选</button>
                        <button className="btn-ghost h-6 px-2 text-[11px]" onClick={() => setSelectedNodes([])}>清空</button>
                      </div>
                    </div>
                    {nodes.length === 0 ? (
                      <EmptyState title="暂无节点" description="请先在节点管理中添加节点。" icon={<Server size={18} />} />
                    ) : (
                      <div className="space-y-2">
                        {(() => {
                          const groups = [
                            { label: '托管节点', items: nodes.filter((n) => n.node_type === 'ssh') },
                            { label: '导入节点', items: nodes.filter((n) => n.node_type === 'imported') },
                            { label: 'Worker 节点', items: nodes.filter((n) => n.node_type === 'worker') },
                          ].filter((g) => g.items.length > 0)
                          return groups.map((group) => (
                            <div key={group.label}>
                              <p className="mb-1 text-[10px] font-semibold uppercase tracking-widest text-muted/60">{group.label}</p>
                              <div className="grid gap-1.5 sm:grid-cols-2">
                                {group.items.map((node) => {
                                  const checked = selectedNodes.includes(node.id)
                                  return (
                                    <label
                                      key={node.id}
                                      className={[
                                        'flex cursor-pointer items-start gap-2.5 rounded-lg border px-3 py-2 text-xs transition-all',
                                        checked ? 'border-brand/45 bg-brand/10' : 'border-white/10 bg-white/[0.03] hover:bg-white/[0.05]',
                                      ].join(' ')}
                                    >
                                      <input
                                        type="checkbox"
                                        className="mt-0.5 h-3.5 w-3.5 flex-shrink-0 rounded border-white/25 bg-transparent text-brand"
                                        checked={checked}
                                        onChange={() => toggleNode(node.id)}
                                      />
                                      <div className="min-w-0">
                                        <p className="truncate font-semibold text-slate-200">{node.name}</p>
                                        <p className="truncate font-mono text-[11px] text-muted">{node.domain || node.host}</p>
                                        {!node.has_credentials && node.node_type === 'ssh' && (
                                          <p className="mt-0.5 text-[11px] text-danger">缺少代理账号密码</p>
                                        )}
                                      </div>
                                    </label>
                                  )
                                })}
                              </div>
                            </div>
                          ))
                        })()}
                        <p className="text-xs text-muted">已选 {selectedNodes.length} / {nodes.length} 个节点</p>
                      </div>
                    )}
                  </div>

                  {ruleSets.length > 0 && (
                    <div>
                      <p className="mb-1 text-xs font-semibold uppercase tracking-widest text-muted">合并规则集（可选）</p>
                      <p className="mb-2 text-xs text-muted">
                        勾选的规则集将注入为 <code className="font-mono">rule-providers</code>，路由策略默认为 🚀 节点选择。
                      </p>
                      <div className="space-y-1">
                        {ruleSets.map((rs) => {
                          const checked = selectedRuleSetIDs.includes(rs.id)
                          return (
                            <label
                              key={rs.id}
                              className={[
                                'flex cursor-pointer items-center gap-2.5 rounded-lg border px-3 py-2 text-xs transition-all',
                                checked ? 'border-brand/45 bg-brand/10' : 'border-white/10 bg-white/[0.03] hover:bg-white/[0.05]',
                              ].join(' ')}
                            >
                              <input
                                type="checkbox"
                                className="h-3.5 w-3.5 flex-shrink-0 rounded border-white/25 bg-transparent text-brand"
                                checked={checked}
                                onChange={() => toggleRuleSet(rs.id)}
                              />
                              <div className="min-w-0 flex-1">
                                <p className="truncate font-semibold text-slate-200">{rs.name}</p>
                                <p className="truncate font-mono text-[11px] text-muted">{rs.rules.length} 条规则</p>
                              </div>
                              <span className="badge badge-muted shrink-0 font-mono text-[10px]">{rs.worker_name || rs.hostname || '—'}</span>
                            </label>
                          )
                        })}
                      </div>
                    </div>
                  )}
                </div>
              )}

              {/* Step 3: Publish */}
              {step === 3 && (
                <div className="space-y-4">
                  <p className="text-xs font-semibold uppercase tracking-widest text-muted">③ 选择私有仓库并发布</p>
                  {workerConfigs.length === 0 ? (
                    <div className="space-y-3 rounded-xl border border-white/10 bg-black/20 px-4 py-4">
                      <p className="text-sm text-muted">需要先创建一个私有仓库才能发布。</p>
                      <button className="btn-primary flex items-center gap-2" onClick={() => setShowNestedWizard(true)}>
                        <CloudCog size={14} />
                        新建私有仓库
                      </button>
                    </div>
                  ) : (
                    <div className="space-y-3">
                      <div>
                        <label className="mb-1.5 block text-xs text-muted">私有仓库</label>
                        <select
                          className="theme-select glass-input"
                          value={selectedWorkerConfigID}
                          onChange={(e) => setSelectedWorkerConfigID(e.target.value)}
                        >
                          {workerConfigs.map((cfg) => (
                            <option key={cfg.id} value={cfg.id}>{cfg.name} · {cfg.hostname || cfg.worker_name}</option>
                          ))}
                        </select>
                      </div>
                      {selectedWorkerConfig && (
                        <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-white/10 bg-black/20 px-3 py-2 text-xs text-muted">
                          <div className="flex flex-wrap items-center gap-3">
                            <span className="inline-flex items-center gap-1.5 text-slate-200">
                              <Server size={12} className="text-brand-light" />
                              {selectedWorkerConfig.worker_name}
                            </span>
                            <span className="font-mono">{selectedWorkerConfig.hostname || selectedWorkerConfig.worker_url}</span>
                            {selectedWorkerConfig.has_token
                              ? <span className="text-success">Token ✓</span>
                              : <span className="text-warning">Token 缺失</span>
                            }
                            {selectedWorkerConfig.worker_url && (
                              <a href={selectedWorkerConfig.worker_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-brand-light hover:underline">
                                打开 <ExternalLink size={11} />
                              </a>
                            )}
                          </div>
                          <button
                            className="text-danger transition-colors hover:text-red-300"
                            title="删除私有仓库"
                            onClick={() => { void removeCurrentWorker() }}
                          >
                            <Trash2 size={13} />
                          </button>
                        </div>
                      )}
                    </div>
                  )}

                  {latestPublish && (
                    <div className="space-y-2 rounded-xl border border-emerald-500/25 bg-emerald-500/[0.06] p-3">
                      <p className="text-sm font-semibold text-emerald-300">
                        ✓ 订阅已发布（v{latestPublish.version}）— 链接已复制
                      </p>
                      <code className="block break-all rounded bg-black/30 px-2 py-1 text-xs text-emerald-200">
                        {latestPublish.access_url}
                      </code>
                      <div className="flex flex-wrap items-center gap-2">
                        <button
                          className="btn-ghost h-7 px-2.5 text-xs"
                          onClick={async () => {
                            await copyText(latestPublish.access_url)
                            setCopiedLink(true)
                            setTimeout(() => setCopiedLink(false), 1600)
                          }}
                        >
                          <Copy size={12} className={copiedLink ? 'text-success' : ''} />
                          {copiedLink ? '已复制' : '复制链接'}
                        </button>
                        <a href={latestPublish.access_url} target="_blank" rel="noreferrer" className="btn-ghost h-7 px-2.5 text-xs">
                          <ExternalLink size={12} />
                          打开
                        </a>
                        <button
                          className="btn-ghost h-7 px-2.5 text-xs"
                          onClick={() => navigate(`/config?tab=subscriptions&addSub=${encodeURIComponent(latestPublish.access_url)}&subName=${encodeURIComponent(latestPublish.file_name.replace(/\.ya?ml$/i, ''))}`)}
                        >
                          <ListPlus size={12} />
                          添加到订阅
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>

            {/* Full-width YAML preview panel */}
            <div className="rounded-xl border border-white/10 bg-black/20 p-3">
              <div className="mb-2 flex items-center justify-between">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="text-xs font-semibold text-muted">配置预览</p>
                  {selectedRuleSetIDs.length > 0 && (
                    <span className="flex items-center gap-1 rounded-full bg-emerald-500/15 px-2 py-0.5 text-[10px] font-semibold text-emerald-300">
                      <Sparkles size={10} />
                      {selectedRuleSetIDs.length} 个规则集已注入 — 绿色=规则行，黄色=provider 配置
                    </span>
                  )}
                </div>
                <button
                  className="btn-icon-sm btn-ghost"
                  onClick={() => { void refreshPreview() }}
                  disabled={previewLoading}
                  title="刷新预览"
                >
                  <RefreshCw size={12} className={previewLoading ? 'animate-spin' : ''} />
                </button>
              </div>
              {selectedNodes.length === 0 && (
                <p className="mb-2 text-[10px] text-muted">
                  当前未选择节点，正在预览模板原文。
                </p>
              )}
              <AnnotatedYamlView
                lines={annotateYamlLines(previewContent, ruleSets, selectedRuleSetIDs)}
                loading={previewLoading}
              />
            </div>
          </div>

          {notice && (
            <InlineNotice tone={notice.tone} title={notice.title}>
              {notice.text}
            </InlineNotice>
          )}

          {/* Navigation */}
          <div className="flex items-center justify-between gap-2">
            <button className="btn-ghost" onClick={onClose}>
              {latestPublish ? '关闭' : '取消'}
            </button>
            <div className="flex items-center gap-2">
              {step > 1 && !latestPublish && (
                <button
                  className="btn-ghost flex items-center gap-2"
                  onClick={() => setStep((s) => (s > 1 ? (s - 1) as 1 | 2 | 3 : s))}
                >
                  上一步
                </button>
              )}
              {step < 3 && (
                <button
                  className="btn-primary flex items-center gap-2"
                  onClick={() => setStep((s) => (s < 3 ? (s + 1) as 1 | 2 | 3 : s))}
                  disabled={step === 2 && selectedNodes.length === 0}
                >
                  下一步
                  <span className="text-[11px] text-white/60">{(stepLabels as readonly string[])[step] ?? ''}</span>
                </button>
              )}
              {step === 3 && !latestPublish && (
                <button
                  className="btn-cta flex items-center gap-2"
                  onClick={() => { void runPublish() }}
                  disabled={publishing || workerConfigs.length === 0 || selectedNodes.length === 0}
                >
                  {publishing ? <Loader2 size={14} className="animate-spin" /> : <UploadCloud size={14} />}
                  发布订阅
                </button>
              )}
              {latestPublish && (
                <button className="btn-primary flex items-center gap-2" onClick={onClose}>
                  <CheckCircle2 size={14} />
                  完成
                </button>
              )}
            </div>
          </div>

        </div>
      </ModalShell>

      {showNestedWizard && (
        <WorkerWizardModal
          onClose={() => setShowNestedWizard(false)}
          defaultConfig={null}
          cfConfig={cfConfig}
          onSaved={(cfg) => {
            setWorkerConfigs((prev) => [...prev, cfg])
            setSelectedWorkerConfigID(cfg.id)
            setShowNestedWizard(false)
          }}
        />
      )}
    </>
  )
}

// ─── Publish page ──────────────────────────────────────────────────────────

export function Publish() {
  const navigate = useNavigate()
  const { config: cfGlobal, loading: cfLoading, save: saveCFGlobal, reload: reloadCF } = useCFConfig()
  const [showCFModal, setShowCFModal] = useState(false)

  const [nodes, setNodes] = useState<PublishNode[]>([])
  const [templates, setTemplates] = useState<PublishTemplatePreset[]>([])
  const [workerConfigs, setWorkerConfigs] = useState<PublishWorkerConfig[]>([])
  const [records, setRecords] = useState<PublishRecord[]>([])
  const [ruleSets, setRuleSets] = useState<RuleSet[]>([])
  const [loading, setLoading] = useState(true)
  const [busyRefresh, setBusyRefresh] = useState(false)
  const [notice, setNotice] = useState<NoticeState | null>(null)

  const [copiedRecordID, setCopiedRecordID] = useState('')
  const [copiedRuleSetID, setCopiedRuleSetID] = useState('')
  const [showWizard, setShowWizard] = useState(false)
  const [showManage, setShowManage] = useState(false)
  const [showPublishWizard, setShowPublishWizard] = useState(false)

  const refreshAll = useCallback(async (showRefreshing = false) => {
    if (showRefreshing) setBusyRefresh(true)
    try {
      const [nodesData, templatesData, configData, recordData, ruleSetsData] = await Promise.all([
        getPublishNodes(),
        getPublishTemplates(),
        getPublishWorkerConfigs(),
        getPublishRecords(),
        getRuleSets(),
      ])
      setNodes(nodesData.nodes ?? [])
      setTemplates(templatesData.templates ?? [])
      setWorkerConfigs(configData.configs ?? [])
      setRecords(recordData.records ?? [])
      setRuleSets(ruleSetsData.rule_sets ?? [])
    } catch (error) {
      setNotice({ tone: 'danger', title: '加载失败', text: error instanceof Error ? error.message : '请求失败' })
    } finally {
      setLoading(false)
      if (showRefreshing) setBusyRefresh(false)
    }
  }, [])

  useEffect(() => { void refreshAll() }, [refreshAll])

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

  const [showRuleSetModal, setShowRuleSetModal] = useState(false)
  const [editingRuleSet, setEditingRuleSet] = useState<RuleSet | null>(null)
  const [ruleSetName, setRuleSetName] = useState('')
  const [ruleSetWorkerID, setRuleSetWorkerID] = useState('')
  const [ruleSetText, setRuleSetText] = useState('')
  const [ruleSetBusy, setRuleSetBusy] = useState(false)

  const normalizeRuleInput = (raw: string): string => {
    const result: string[] = []
    for (const line of raw.split('\n')) {
      if (/^\s*payload\s*:\s*$/.test(line)) continue
      let s = line.replace(/^\s*-\s+/, '')
      s = s.replace(/^'(.*)'$/, '$1')
      s = s.trim()
      if (s) result.push(s)
    }
    return result.join('\n')
  }

  const handleRuleSetPaste = (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const pasted = e.clipboardData.getData('text')
    if (!/payload\s*:|^\s*-\s+/m.test(pasted)) return
    e.preventDefault()
    const normalized = normalizeRuleInput(pasted)
    const el = e.currentTarget
    const start = el.selectionStart
    const end = el.selectionEnd
    const next = ruleSetText.slice(0, start) + normalized + ruleSetText.slice(end)
    setRuleSetText(next)
    requestAnimationFrame(() => { el.selectionStart = el.selectionEnd = start + normalized.length })
  }

  const openNewRuleSetModal = () => {
    setEditingRuleSet(null)
    setRuleSetName('')
    setRuleSetWorkerID(workerConfigs[0]?.id ?? '')
    setRuleSetText('')
    setShowRuleSetModal(true)
  }

  const openEditRuleSetModal = (rs: RuleSet) => {
    setEditingRuleSet(rs)
    setRuleSetName(rs.name)
    setRuleSetWorkerID(rs.worker_config_id)
    setRuleSetText(rs.rules.join('\n'))
    setShowRuleSetModal(true)
  }

  const saveRuleSet = async () => {
    const rules = normalizeRuleInput(ruleSetText).split('\n').filter(Boolean)
    if (rules.length === 0) {
      setNotice({ tone: 'warning', title: '规则不能为空', text: '请至少输入一条规则。' })
      return
    }
    setRuleSetBusy(true)
    setNotice(null)
    try {
      if (editingRuleSet) {
        await updateRuleSet(editingRuleSet.id, rules)
        setNotice({ tone: 'success', title: '更新成功', text: '规则集已同步到远端 KV。' })
      } else {
        const name = ruleSetName.trim()
        if (!name) {
          setNotice({ tone: 'warning', title: '名称不能为空', text: '请填写规则集名称。' })
          return
        }
        if (!ruleSetWorkerID) {
          setNotice({ tone: 'warning', title: '请选择私有仓库', text: '需要选择一个私有仓库。' })
          return
        }
        await createRuleSet({ name, worker_config_id: ruleSetWorkerID, rules })
        setNotice({ tone: 'success', title: '创建成功', text: '规则集已发布到远端 KV，URL 永久有效。' })
      }
      setShowRuleSetModal(false)
      await refreshAll()
    } catch (error) {
      setNotice({ tone: 'danger', title: editingRuleSet ? '更新失败' : '创建失败', text: error instanceof Error ? error.message : '请求失败' })
    } finally {
      setRuleSetBusy(false)
    }
  }

  const removeRuleSet = async (id: string) => {
    if (!confirm('确认删除这个规则集吗？这会尝试同步删除远端 KV 文件。')) return
    setNotice(null)
    try {
      const data = await deleteRuleSet(id)
      await refreshAll()
      if (data.warning) {
        setNotice({ tone: 'warning', title: '记录已删除', text: data.warning })
      } else {
        setNotice({ tone: 'success', title: '删除成功', text: '规则集已删除。' })
      }
    } catch (error) {
      setNotice({ tone: 'danger', title: '删除失败', text: error instanceof Error ? error.message : '请求失败' })
    }
  }

  const handleCopyRuleSetLink = async (id: string, link: string) => {
    try {
      await copyText(link)
      setCopiedRuleSetID(id)
      setTimeout(() => setCopiedRuleSetID(''), 1600)
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
            正在加载订阅管理…
          </div>
        </div>
      ) : (
        <div className="space-y-6">
          <PageHeader
            eyebrow="代理资源 · 订阅分发"
            title="订阅分发"
            description="管理已发布的订阅链接和托管规则集，或发布新订阅。"
            metrics={[
              { label: '可用节点', value: String(nodes.length) },
              { label: '私有仓库', value: String(workerConfigs.length) },
              { label: '已发布', value: String(records.length) },
            ]}
            actions={(
              <>
                <button className="btn-icon-sm btn-ghost" onClick={() => { void refreshAll(true) }} disabled={busyRefresh} title="刷新">
                  <RefreshCw size={14} className={busyRefresh ? 'animate-spin' : ''} />
                </button>
                <button className="btn-ghost flex items-center gap-2" onClick={() => setShowManage(true)}>
                  <Database size={14} />
                  私有仓库
                </button>
                <button className="btn-cta flex items-center gap-2" onClick={() => setShowPublishWizard(true)}>
                  <UploadCloud size={14} />
                  发布新订阅
                </button>
              </>
            )}
          />

          <CFConfigBanner config={cfGlobal} loading={cfLoading} onConfigure={() => setShowCFModal(true)} />

          {notice && (
            <InlineNotice tone={notice.tone} title={notice.title}>
              {notice.text}
            </InlineNotice>
          )}

          {/* ── 已发布的订阅 */}
          <SectionCard
            title="已发布的订阅"
            description="通过 Cloudflare Worker 托管的订阅链接，粘贴到代理客户端即可使用。"
          >
            {records.length === 0 ? (
              <EmptyState
                title="暂无发布记录"
                description="点击「发布新订阅」，按步骤选择模板和节点，一键生成订阅链接。"
                icon={<UploadCloud size={18} />}
                action={(
                  <button className="btn-primary flex items-center gap-2" onClick={() => setShowPublishWizard(true)}>
                    <UploadCloud size={14} />
                    发布第一个订阅
                  </button>
                )}
              />
            ) : (
              <div className="table-shell overflow-hidden">
                <div className="grid grid-cols-12 gap-3 px-4 py-3 table-header-row">
                  <span className="col-span-3">文件</span>
                  <span className="col-span-2">私有仓库</span>
                  <span className="col-span-1">版本</span>
                  <span className="col-span-3">发布时间</span>
                  <span className="col-span-3 text-right">操作</span>
                </div>
                {records.map((record) => (
                  <div key={record.id} className="grid grid-cols-12 items-center gap-3 px-4 py-3 table-row">
                    <div className="col-span-3 min-w-0">
                      <p className="truncate text-sm font-semibold text-slate-100">{record.file_name}</p>
                      <p className="truncate font-mono text-xs text-muted">{record.base_name}</p>
                    </div>
                    <div className="col-span-2 min-w-0">
                      <p className="truncate text-xs text-slate-300">{record.worker_name}</p>
                    </div>
                    <div className="col-span-1">
                      <span className="badge badge-muted">v{record.version}</span>
                    </div>
                    <div className="col-span-3">
                      <p className="text-xs text-muted">
                        {new Date(record.published_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}
                      </p>
                    </div>
                    <div className="col-span-3 flex items-center justify-end gap-1.5">
                      <button
                        className="btn-icon-sm btn-ghost"
                        title="添加到订阅"
                        onClick={() => navigate(`/config?tab=subscriptions&addSub=${encodeURIComponent(record.access_url)}&subName=${encodeURIComponent(record.file_name.replace(/\.ya?ml$/i, ''))}`)}
                      >
                        <ListPlus size={14} />
                      </button>
                      <button className="btn-icon-sm btn-ghost" title="复制链接" onClick={() => { void handleCopyLink(record.id, record.access_url) }}>
                        <Copy size={14} className={copiedRecordID === record.id ? 'text-success' : ''} />
                      </button>
                      <a className="btn-icon-sm btn-ghost" title="打开链接" href={record.access_url} target="_blank" rel="noreferrer">
                        <ExternalLink size={14} />
                      </a>
                      <button className="btn-icon-sm btn-ghost text-danger hover:bg-danger/10" title="删除记录" onClick={() => { void removeRecord(record.id) }}>
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </SectionCard>

          {/* ── 规则集 */}
          <SectionCard
            title="规则集"
            description="托管到 Cloudflare KV 的 rule-provider 文件，URL 永久不变，可直接引用于订阅中。"
            actions={(
              workerConfigs.length > 0 ? (
                <button className="btn-ghost flex items-center gap-2" onClick={openNewRuleSetModal}>
                  <Plus size={14} />
                  新建规则集
                </button>
              ) : undefined
            )}
          >
            {ruleSets.length === 0 ? (
              <EmptyState
                title="暂无规则集"
                description="新建规则集后可在「发布新订阅」中选择合并，URL 永久固定可直接用于 rule-providers。"
                icon={<FileText size={18} />}
                action={
                  workerConfigs.length > 0 ? (
                    <button className="btn-primary flex items-center gap-2" onClick={openNewRuleSetModal}>
                      <Plus size={14} />
                      新建规则集
                    </button>
                  ) : (
                    <button className="btn-primary flex items-center gap-2" onClick={() => setShowWizard(true)}>
                      <CloudCog size={14} />
                      先创建私有仓库
                    </button>
                  )
                }
              />
            ) : (
              <div className="table-shell overflow-hidden">
                <div className="grid grid-cols-12 gap-3 px-4 py-3 table-header-row">
                  <span className="col-span-4">名称 / KV Key</span>
                  <span className="col-span-2">私有仓库</span>
                  <span className="col-span-2">规则条数</span>
                  <span className="col-span-4 text-right">操作</span>
                </div>
                {ruleSets.map((rs) => (
                  <div key={rs.id} className="grid grid-cols-12 items-center gap-3 px-4 py-3 table-row">
                    <div className="col-span-4 min-w-0">
                      <p className="truncate text-sm font-semibold text-slate-100">{rs.name}</p>
                      <p className="truncate font-mono text-xs text-muted">{rs.kv_key}</p>
                    </div>
                    <div className="col-span-2 min-w-0">
                      <p className="truncate text-xs text-slate-300">{rs.worker_name || rs.hostname || '—'}</p>
                    </div>
                    <div className="col-span-2">
                      <span className="badge badge-muted">{rs.rules.length} 条</span>
                    </div>
                    <div className="col-span-4 flex items-center justify-end gap-1.5">
                      <button className="btn-icon-sm btn-ghost" title="编辑规则" onClick={() => openEditRuleSetModal(rs)}>
                        <Pencil size={14} />
                      </button>
                      <button className="btn-icon-sm btn-ghost" title="复制链接" onClick={() => { void handleCopyRuleSetLink(rs.id, rs.access_url) }}>
                        <Copy size={14} className={copiedRuleSetID === rs.id ? 'text-success' : ''} />
                      </button>
                      <a className="btn-icon-sm btn-ghost" title="打开链接" href={rs.access_url} target="_blank" rel="noreferrer">
                        <ExternalLink size={14} />
                      </a>
                      <button className="btn-icon-sm btn-ghost text-danger hover:bg-danger/10" title="删除规则集" onClick={() => { void removeRuleSet(rs.id) }}>
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </SectionCard>

          {/* Modals */}
          {showPublishWizard && (
            <PublishWizardModal
              onClose={() => setShowPublishWizard(false)}
              onPublished={() => { void refreshAll() }}
              nodes={nodes}
              templates={templates}
              workerConfigs={workerConfigs}
              ruleSets={ruleSets}
              cfConfig={cfGlobal}
            />
          )}

          {showManage && (
            <WorkerConfigsModal
              onClose={() => setShowManage(false)}
              configs={workerConfigs}
              cfConfig={cfGlobal}
              onCreateNew={() => { setShowManage(false); setShowWizard(true) }}
              onRefresh={() => { void refreshAll() }}
            />
          )}

          {showWizard && (
            <WorkerWizardModal
              onClose={() => setShowWizard(false)}
              defaultConfig={null}
              cfConfig={cfGlobal}
              onSaved={() => { void refreshAll() }}
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

          {showRuleSetModal && (
            <ModalShell
              title={editingRuleSet ? '编辑规则集' : '新建规则集'}
              description={editingRuleSet ? `${editingRuleSet.name} · ${editingRuleSet.kv_key}` : '输入规则后发布到 Cloudflare KV，获得一条永久固定 URL'}
              onClose={() => setShowRuleSetModal(false)}
              size="lg"
            >
              <div className="flex flex-col gap-5 p-5">
                {!editingRuleSet && (
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label className="mb-1.5 block text-xs text-muted">规则集名称</label>
                      <input
                        className="glass-input w-full"
                        placeholder="例：JP 直连规则"
                        value={ruleSetName}
                        onChange={(e) => setRuleSetName(e.target.value)}
                        autoFocus
                      />
                    </div>
                    <div>
                      <label className="mb-1.5 block text-xs text-muted">私有仓库</label>
                      <select
                        className="glass-input theme-select w-full"
                        value={ruleSetWorkerID}
                        onChange={(e) => setRuleSetWorkerID(e.target.value)}
                      >
                        {workerConfigs.map((cfg) => (
                          <option key={cfg.id} value={cfg.id}>{cfg.name || cfg.worker_name}</option>
                        ))}
                      </select>
                    </div>
                  </div>
                )}
                <div>
                  <div className="mb-1.5 flex items-center justify-between">
                    <label className="text-xs text-muted">规则内容 · 每行一条</label>
                    <span className="tabular-nums text-xs text-muted">
                      {ruleSetText.split('\n').filter((l) => l.trim()).length} 条
                    </span>
                  </div>
                  <textarea
                    className="glass-textarea w-full resize-none text-[12px] leading-[1.65]"
                    rows={16}
                    placeholder={'# 每行一条 mihomo 规则，例：\nDOMAIN-SUFFIX,example.com\nDOMAIN-KEYWORD,google\n+.jp\nIP-CIDR,192.168.0.0/16,no-resolve\nGEOSITE,youtube'}
                    value={ruleSetText}
                    onChange={(e) => setRuleSetText(e.target.value)}
                    onPaste={handleRuleSetPaste}
                    spellCheck={false}
                  />
                </div>
                <div className="flex items-center justify-between gap-3">
                  <p className="text-xs text-muted">发布后 URL 永久不变，编辑内容不影响链接。</p>
                  <div className="flex flex-shrink-0 items-center gap-2">
                    <button className="btn-ghost" onClick={() => setShowRuleSetModal(false)}>取消</button>
                    <button
                      className="btn-brand flex items-center gap-1.5"
                      onClick={() => { void saveRuleSet() }}
                      disabled={ruleSetBusy}
                    >
                      {ruleSetBusy ? <Loader2 size={14} className="animate-spin" /> : <UploadCloud size={14} />}
                      {editingRuleSet ? '保存并同步' : '发布规则集'}
                    </button>
                  </div>
                </div>
              </div>
            </ModalShell>
          )}
        </div>
      )}
    </CFGate>
  )
}
