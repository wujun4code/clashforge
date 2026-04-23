import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Upload, FileText, Globe, CheckCircle2, AlertCircle,
  ChevronRight, Play, Loader2, Wifi, XCircle, ArrowRight,
  Sparkles, RotateCw, Link2, Square, ShieldOff,
} from 'lucide-react'
import yaml from 'js-yaml'
import {
  updateOverrides, generateConfig, getConfig, updateConfig,
  startCore, stopCore, takeoverOverviewModule, releaseOverviewTakeover,
  getOverviewCore, getOverviewProbes, getLogs,
  addSubscription, triggerSubUpdate, enableService,
} from '../api/client'
import type { OverviewProbeData, OverviewModule, LogEntry } from '../api/client'

type InitStatus = 'checking' | 'running' | 'ready'

// ── Types ────────────────────────────────────────────────────────────────────

type Step = 'import' | 'dns' | 'network' | 'launch' | 'check'
const STEPS: { id: Step; label: string }[] = [
  { id: 'import',  label: '导入配置' },
  { id: 'dns',     label: 'DNS 设置' },
  { id: 'network', label: '网络设置' },
  { id: 'launch',  label: '启动服务' },
  { id: 'check',   label: '连通检测' },
]

interface ClashDNS {
  enable?: boolean
  ipv6?: boolean
  'enhanced-mode'?: string
  listen?: string
  nameserver?: string[]
  fallback?: string[]
  'default-nameserver'?: string[]
  'fake-ip-range'?: string
  'respect-rules'?: boolean
}

interface ClashParsed {
  mode?: string
  port?: number
  'socks-port'?: number
  'mixed-port'?: number
  'allow-lan'?: boolean
  dns?: ClashDNS
}

interface FormDNS {
  enable: boolean
  mode: string          // fake-ip | redir-host
  dnsmasq_mode: string  // none | upstream | replace
  apply_on_start: boolean
  listen: string
  ipv6: boolean
}

interface FormNetwork {
  mode: string           // none | tproxy | redir | tun
  firewall_backend: string
  bypass_lan: boolean
  bypass_china: boolean
  apply_on_start: boolean
}

// ── Small UI helpers ─────────────────────────────────────────────────────────

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="flex-shrink-0 w-44">
        <label className="text-sm text-slate-300">{label}</label>
        {hint && <p className="text-xs text-muted mt-0.5 leading-4">{hint}</p>}
      </div>
      <div className="flex-1">{children}</div>
    </div>
  )
}

function TextInput({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder?: string }) {
  return (
    <input
      className="w-full bg-surface-2 border border-white/10 rounded-xl px-3 py-2 text-sm text-white outline-none focus:border-brand transition-colors"
      value={value}
      onChange={e => onChange(e.target.value)}
      placeholder={placeholder}
    />
  )
}

function SelectInput({ value, onChange, options }: { value: string; onChange: (v: string) => void; options: { value: string; label: string }[] }) {
  return (
    <select
      className="w-full bg-surface-2 border border-white/10 rounded-xl px-3 py-2 text-sm text-white outline-none focus:border-brand transition-colors appearance-none"
      value={value}
      onChange={e => onChange(e.target.value)}
    >
      {options.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
    </select>
  )
}

function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label?: string }) {
  return (
    <div className="flex items-center gap-3">
      <button
        type="button"
        onClick={() => onChange(!checked)}
        className={`w-10 h-5 rounded-full transition-colors flex-shrink-0 relative ${checked ? 'bg-brand' : 'bg-surface-3'}`}
      >
        <span className={`absolute top-0.5 w-4 h-4 bg-white rounded-full shadow transition-all ${checked ? 'left-5' : 'left-0.5'}`} />
      </button>
      {label && <span className={`text-xs ${checked ? 'text-slate-200' : 'text-muted'}`}>{label}</span>}
    </div>
  )
}

function StepBar({ step }: { step: Step }) {
  const idx = STEPS.findIndex(s => s.id === step)
  return (
    <div className="flex items-center gap-0">
      {STEPS.map((s, i) => (
        <div key={s.id} className="flex items-center">
          <div className={`flex items-center gap-2 px-3 py-1.5 rounded-full text-xs font-medium transition-all ${
            i < idx ? 'text-success' :
            i === idx ? 'bg-brand/20 text-brand border border-brand/30' :
            'text-muted'
          }`}>
            {i < idx
              ? <CheckCircle2 size={12} className="text-success" />
              : <span className={`w-4 h-4 rounded-full flex items-center justify-center text-[10px] font-bold border ${i === idx ? 'border-brand bg-brand/30 text-brand' : 'border-white/15 text-muted'}`}>{i + 1}</span>
            }
            {s.label}
          </div>
          {i < STEPS.length - 1 && <ChevronRight size={14} className="text-white/15 mx-1 flex-shrink-0" />}
        </div>
      ))}
    </div>
  )
}

function InfoBadge({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between py-1.5 border-b border-white/5 last:border-0">
      <span className="text-xs text-muted">{label}</span>
      <span className="text-xs text-slate-200 font-mono">{value}</span>
    </div>
  )
}

// ── Browser probe helpers (mirrors Dashboard.tsx) ─────────────────────────────

async function fetchWithTimeout(input: RequestInfo | URL, init: RequestInit, ms: number) {
  const ctrl = new AbortController()
  const t = window.setTimeout(() => ctrl.abort(), ms)
  try { return await fetch(input, { ...init, signal: ctrl.signal }) }
  finally { window.clearTimeout(t) }
}

interface BrowserProbeResult {
  ipOK: boolean; ip?: string; ipError?: string
  accessOK: boolean; accessChecks: { name: string; group?: string; ok: boolean; latency_ms?: number; error?: string }[]
}

async function runBrowserProbe(): Promise<BrowserProbeResult> {
  let ipOK = false, ip: string | undefined, ipError: string | undefined
  try {
    const r = await fetchWithTimeout('https://api.ip.sb/geoip', { cache: 'no-store' }, 7000)
    const d = await r.json() as { ip?: string }
    ipOK = !!d.ip; ip = d.ip
  } catch (e) {
    ipError = e instanceof Error ? e.message : '获取失败'
  }

  const targets = [
    { name: '淠宝',      group: '国内', url: 'https://www.taobao.com' },
    { name: '网易云音乐',  group: '国内', url: 'https://music.163.com' },
    { name: 'Google',    group: '国外', url: 'https://www.google.com' },
    { name: 'GitHub',    group: '国外', url: 'https://github.com' },
    { name: 'OpenAI',    group: 'AI',   url: 'https://chat.openai.com' },
    { name: 'Claude',    group: 'AI',   url: 'https://claude.ai' },
    { name: 'Gemini',    group: 'AI',   url: 'https://gemini.google.com' },
  ]
  const accessChecks = await Promise.all(targets.map(async t => {
    const start = performance.now()
    try {
      await fetchWithTimeout(t.url, { mode: 'no-cors', cache: 'no-store' }, 8000)
      return { name: t.name, group: t.group, ok: true, latency_ms: Math.round(performance.now() - start) }
    } catch (e) {
      return { name: t.name, group: t.group, ok: false, error: e instanceof Error ? e.message : '访问失败' }
    }
  }))
  return { ipOK, ip, ipError, accessOK: accessChecks.every(c => c.ok), accessChecks }
}

// ── Main component ────────────────────────────────────────────────────────────

export function Setup() {
  const navigate = useNavigate()
  const fileRef = useRef<HTMLInputElement>(null)

  // ── init guard: check if core is already running ──
  const [initStatus, setInitStatus] = useState<InitStatus>('checking')
  const [stopping, setStopping] = useState(false)
  const [stopError, setStopError] = useState('')
  const [runningModules, setRunningModules] = useState<OverviewModule[]>([])

  useEffect(() => {
    getOverviewCore().then(data => {
      if (data.core.state === 'running') {
        setRunningModules(data.modules ?? [])
        setInitStatus('running')
      } else {
        setInitStatus('ready')
      }
    }).catch(() => setInitStatus('ready'))
  }, [])

  const handleStopAll = async () => {
    setStopping(true); setStopError('')
    try {
      await stopCore()
      await releaseOverviewTakeover().catch(() => null)
      setInitStatus('ready')
    } catch (e) {
      setStopError(e instanceof Error ? e.message : '操作失败')
    } finally { setStopping(false) }
  }

  // ── step state ──
  const [step, setStep] = useState<Step>('import')

  // ── import step ──
  const [importMode, setImportMode] = useState<'file' | 'paste' | 'url'>('paste')
  const [pasteContent, setPasteContent] = useState('')
  const [remoteUrl, setRemoteUrl] = useState('')
  const [importing, setImporting] = useState(false)
  const [importError, setImportError] = useState('')
  const [clashParsed, setClashParsed] = useState<ClashParsed | null>(null)

  // ── dns form ──
  const [dns, setDns] = useState<FormDNS>({
    enable: true, mode: 'fake-ip', dnsmasq_mode: 'none',
    apply_on_start: true, listen: '0.0.0.0:7874', ipv6: false,
  })

  // ── network form ──
  const [net, setNet] = useState<FormNetwork>({
    mode: 'tproxy', firewall_backend: 'auto',
    bypass_lan: true, bypass_china: true, apply_on_start: true,
  })

  // ── launch step ──
  const [launching, setLaunching] = useState(false)
  const [launchDone, setLaunchDone] = useState(false)
  const [launchError, setLaunchError] = useState('')

  // ── check step ──
  const [checking, setChecking] = useState(false)
  const [routerProbe, setRouterProbe] = useState<OverviewProbeData | null>(null)
  const [browserProbe, setBrowserProbe] = useState<BrowserProbeResult | null>(null)
  const [probeLogs, setProbeLogs] = useState<LogEntry[]>([])
  const [checkDone, setCheckDone] = useState(false)

  // ── completion ──
  const [autoStartCore, setAutoStartCore] = useState(true)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')

  // ── Load existing ClashForge config to pre-fill forms ──
  useEffect(() => {
    getConfig().then(cfg => {
      const c = cfg as Record<string, Record<string, unknown>>
      if (c.dns) {
        setDns(prev => ({
          ...prev,
          enable: c.dns?.enable !== undefined ? Boolean(c.dns.enable) : prev.enable,
          mode: String(c.dns?.mode || prev.mode),
          dnsmasq_mode: String(c.dns?.dnsmasq_mode || prev.dnsmasq_mode),
          apply_on_start: c.dns?.apply_on_start !== undefined ? Boolean(c.dns.apply_on_start) : prev.apply_on_start,
        }))
      }
      if (c.network) {
        setNet(prev => ({
          ...prev,
          mode: String(c.network?.mode || prev.mode),
          firewall_backend: String(c.network?.firewall_backend || prev.firewall_backend),
          bypass_lan: c.network?.bypass_lan !== undefined ? Boolean(c.network.bypass_lan) : prev.bypass_lan,
          bypass_china: c.network?.bypass_china !== undefined ? Boolean(c.network.bypass_china) : prev.bypass_china,
          apply_on_start: c.network?.apply_on_start !== undefined ? Boolean(c.network.apply_on_start) : prev.apply_on_start,
        }))
      }
    }).catch(() => null)
  }, [])

  // ── helpers ──
  const dnsSet = useCallback(<K extends keyof FormDNS>(k: K, v: FormDNS[K]) =>
    setDns(prev => ({ ...prev, [k]: v })), [])
  const netSet = useCallback(<K extends keyof FormNetwork>(k: K, v: FormNetwork[K]) =>
    setNet(prev => ({ ...prev, [k]: v })), [])

  // ── fill forms from parsed Clash YAML ──
  const applyClashParsed = useCallback((parsed: ClashParsed) => {
    setClashParsed(parsed)
    if (parsed.dns) {
      setDns(prev => ({
        ...prev,
        enable: parsed.dns?.enable !== undefined ? parsed.dns.enable : prev.enable,
        mode: parsed.dns?.['enhanced-mode'] === 'redir-host' ? 'redir-host' : 'fake-ip',
        ipv6: parsed.dns?.ipv6 !== undefined ? parsed.dns.ipv6 : prev.ipv6,
        listen: parsed.dns?.listen || prev.listen,
      }))
    }
  }, [])

  // ── import: file upload ──
  const handleFile = useCallback((file: File) => {
    const reader = new FileReader()
    reader.onload = ev => {
      const content = (ev.target?.result as string) || ''
      setPasteContent(content)
      setImportMode('paste')
    }
    reader.readAsText(file)
  }, [])

  // ── import: save overrides + generate + parse ──
  const handleImport = useCallback(async () => {
    setImporting(true); setImportError('')
    try {
      let yamlContent = pasteContent
      if (importMode === 'url') {
        // Add as subscription and trigger update
        if (!remoteUrl.trim()) { setImportError('请输入订阅链接'); return }
        const { id } = await addSubscription({ name: '向导导入', url: remoteUrl, type: 'clash', enabled: true })
        await triggerSubUpdate(id)
        await generateConfig().catch(() => null)
        // No YAML to pre-parse, just move forward
        setClashParsed({})
        setStep('dns')
        return
      }
      if (!yamlContent.trim()) { setImportError('内容为空，请粘贴或上传配置文件'); return }

      // Parse client-side for form pre-fill
      try {
        const parsed = yaml.load(yamlContent) as ClashParsed
        if (parsed && typeof parsed === 'object') applyClashParsed(parsed)
      } catch (_) {
        // Ignore parse errors here – backend will validate
      }

      // Save as overrides and generate
      await updateOverrides(yamlContent)
      await generateConfig().catch(() => null)
      setStep('dns')
    } catch (e: unknown) {
      setImportError(e instanceof Error ? e.message : String(e))
    } finally { setImporting(false) }
  }, [importMode, pasteContent, remoteUrl, applyClashParsed])

  // ── launch ──
  const handleLaunch = useCallback(async () => {
    setLaunching(true); setLaunchError('')
    try {
      // Persist ClashForge config (DNS + network)
      const cfg = await getConfig()
      const updated = {
        ...cfg,
        dns: {
          ...(cfg as Record<string, unknown>).dns as Record<string, unknown>,
          enable: dns.enable,
          mode: dns.mode,
          dnsmasq_mode: dns.dnsmasq_mode,
          apply_on_start: dns.apply_on_start,
        },
        network: {
          ...(cfg as Record<string, unknown>).network as Record<string, unknown>,
          mode: net.mode,
          firewall_backend: net.firewall_backend,
          bypass_lan: net.bypass_lan,
          bypass_china: net.bypass_china,
          apply_on_start: net.apply_on_start,
        },
      }
      await updateConfig(updated as Record<string, unknown>)

      // Start core
      await startCore()

      // Apply takeover if requested
      if (net.mode !== 'none' && net.apply_on_start) {
        await takeoverOverviewModule({ module: 'transparent_proxy', mode: net.mode })
      }
      if (dns.enable && dns.apply_on_start) {
        await takeoverOverviewModule({ module: 'dns_entry' })
      }

      setLaunchDone(true)
      // Auto-advance to check after short pause
      setTimeout(() => setStep('check'), 800)
    } catch (e: unknown) {
      setLaunchError(e instanceof Error ? e.message : String(e))
    } finally { setLaunching(false) }
  }, [dns, net])

  // ── complete (save autostart + navigate) ──
  const handleComplete = useCallback(async () => {
    setSaving(true); setSaveError('')
    try {
      const cfg = await getConfig()
      const updated = {
        ...cfg,
        core: {
          ...(cfg as Record<string, unknown>).core as Record<string, unknown>,
          auto_start_core: autoStartCore,
        },
      }
      await updateConfig(updated as Record<string, unknown>)
      if (autoStartCore) {
        await enableService().catch(() => null)
      }
      navigate('/')
    } catch (e: unknown) {
      setSaveError(e instanceof Error ? e.message : '保存失败')
    } finally { setSaving(false) }
  }, [autoStartCore, navigate])

  // ── connectivity check ──
  const handleCheck = useCallback(async () => {
    setChecking(true); setRouterProbe(null); setBrowserProbe(null); setProbeLogs([])
    try {
      const [rp, bp] = await Promise.all([
        getOverviewProbes().catch(() => null),
        runBrowserProbe(),
      ])
      setRouterProbe(rp)
      setBrowserProbe(bp)

      const routerOK = rp ? rp.ip_checks.some(c => c.ok) : false
      const browserOK = bp.ipOK

      if (!routerOK || !browserOK) {
        const { logs } = await getLogs('info', 50)
        setProbeLogs(logs)
      }
      setCheckDone(true)
    } finally { setChecking(false) }
  }, [])

  const overallOK = routerProbe
    ? routerProbe.ip_checks.some(c => c.ok) && (browserProbe?.ipOK ?? false)
    : false

  // ── Render steps ──────────────────────────────────────────────────────────

  // Guard: checking
  if (initStatus === 'checking') {
    return (
      <div className="min-h-full bg-gradient-to-b from-surface-0 to-surface-1 flex items-center justify-center">
        <div className="flex items-center gap-3 text-muted text-sm">
          <Loader2 size={18} className="animate-spin text-brand" />
          正在检测当前运行状态…
        </div>
      </div>
    )
  }

  // Guard: core is running — require stopping before re-running wizard
  if (initStatus === 'running') {
    const managed = runningModules.filter(m => m.managed_by_clashforge)
    return (
      <div className="min-h-full bg-gradient-to-b from-surface-0 to-surface-1 px-6 py-8">
        <div className="max-w-xl mx-auto space-y-6">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-xl bg-brand/20 flex items-center justify-center">
              <Sparkles size={18} className="text-brand" />
            </div>
            <div>
              <h1 className="text-lg font-bold text-white">配置向导</h1>
              <p className="text-xs text-muted">重新运行向导前需要先停止当前服务</p>
            </div>
          </div>

          <div className="card px-5 py-5 space-y-4">
            <div className="flex items-center gap-3">
              <span className="inline-flex h-2.5 w-2.5 rounded-full bg-success animate-pulse" />
              <p className="text-sm font-semibold text-white">内核正在运行</p>
            </div>
            <p className="text-sm text-muted leading-6">
              ClashForge 内核当前处于运行状态，并已接管以下系统服务。
              要重新运行配置向导，请先停止内核并退出所有接管，然后再继续。
            </p>

            {managed.length > 0 && (
              <div className="rounded-xl border border-white/8 bg-black/10 px-4 py-3 space-y-2">
                <p className="text-xs text-muted uppercase tracking-wider font-semibold">当前已接管</p>
                {managed.map(m => (
                  <div key={m.id} className="flex items-center gap-2 text-xs">
                    <ShieldOff size={12} className="text-warning" />
                    <span className="text-slate-300">{m.title}</span>
                    <span className="text-muted">— {m.current_owner}</span>
                  </div>
                ))}
              </div>
            )}

            {stopError && (
              <div className="flex items-center gap-2 text-xs text-danger">
                <AlertCircle size={13} />{stopError}
              </div>
            )}

            <button
              className="btn-danger w-full flex items-center justify-center gap-2 py-3 text-sm font-semibold"
              onClick={handleStopAll}
              disabled={stopping}
            >
              {stopping
                ? <><Loader2 size={15} className="animate-spin" />停止中…</>
                : <><Square size={15} />停止内核 + 退出所有接管</>}
            </button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-full bg-gradient-to-b from-surface-0 to-surface-1 px-6 py-8">
      <div className="max-w-2xl mx-auto space-y-6">

        {/* Header */}
        <div className="flex items-center gap-3">
          <div className="w-9 h-9 rounded-xl bg-brand/20 flex items-center justify-center">
            <Sparkles size={18} className="text-brand" />
          </div>
          <div>
            <h1 className="text-lg font-bold text-white">快速配置向导</h1>
            <p className="text-xs text-muted">导入配置 → 调整参数 → 一键启动 → 验证连通</p>
          </div>
        </div>

        {/* Step bar */}
        <div className="overflow-x-auto">
          <StepBar step={step} />
        </div>

        {/* ─── Step 1: Import ─────────────────────────────────────────────── */}
        {step === 'import' && (
          <div className="space-y-4">
            {/* Mode tabs */}
            <div className="card px-5 py-5 space-y-4">
              <div className="flex items-center gap-2 mb-1">
                <FileText size={16} className="text-brand" />
                <h2 className="text-sm font-semibold text-slate-200">选择导入方式</h2>
              </div>
              <div className="flex gap-2">
                {([
                  { id: 'paste', icon: <FileText size={13} />, label: '粘贴 YAML' },
                  { id: 'file',  icon: <Upload size={13} />,    label: '上传文件' },
                  { id: 'url',   icon: <Link2 size={13} />,     label: '订阅链接' },
                ] as const).map(m => (
                  <button
                    key={m.id}
                    onClick={() => setImportMode(m.id)}
                    className={`btn text-xs py-1.5 flex items-center gap-1.5 ${importMode === m.id ? 'btn-primary' : 'btn-ghost'}`}
                  >
                    {m.icon}{m.label}
                  </button>
                ))}
              </div>

              {importMode === 'paste' && (
                <div className="space-y-3">
                  <p className="text-xs text-muted leading-5">
                    粘贴完整的 Clash / Mihomo YAML 配置（本地配置文件或订阅下载内容）。
                  </p>
                  <textarea
                    className="w-full bg-surface-2 border border-white/10 rounded-xl px-3 py-3 text-xs text-white font-mono outline-none focus:border-brand transition-colors resize-none"
                    rows={16}
                    placeholder={'port: 7890\nsocks-port: 7891\ndns:\n  enable: true\n  enhanced-mode: fake-ip\n  listen: 0.0.0.0:7874\n  ...'}
                    value={pasteContent}
                    onChange={e => setPasteContent(e.target.value)}
                    spellCheck={false}
                  />
                </div>
              )}

              {importMode === 'file' && (
                <div className="space-y-3">
                  <p className="text-xs text-muted leading-5">上传 .yaml / .yml 格式的配置文件。</p>
                  <div
                    className="border-2 border-dashed border-white/15 rounded-2xl px-6 py-12 flex flex-col items-center gap-3 hover:border-brand/40 hover:bg-brand/5 transition-all cursor-pointer"
                    onClick={() => fileRef.current?.click()}
                    onDragOver={e => e.preventDefault()}
                    onDrop={e => { e.preventDefault(); const f = e.dataTransfer.files[0]; if (f) handleFile(f) }}
                  >
                    <Upload size={28} className="text-muted" />
                    <div className="text-center">
                      <p className="text-sm text-slate-300 font-medium">点击上传或拖放文件</p>
                      <p className="text-xs text-muted mt-1">.yaml / .yml 格式</p>
                    </div>
                    <input
                      ref={fileRef} type="file" accept=".yaml,.yml,.txt" className="hidden"
                      onChange={e => { const f = e.target.files?.[0]; if (f) handleFile(f) }}
                    />
                  </div>
                  {pasteContent && (
                    <div className="rounded-xl bg-success/10 border border-success/20 px-4 py-2 flex items-center gap-2">
                      <CheckCircle2 size={14} className="text-success flex-shrink-0" />
                      <span className="text-xs text-success">文件已加载，共 {pasteContent.split('\n').length} 行</span>
                    </div>
                  )}
                </div>
              )}

              {importMode === 'url' && (
                <div className="space-y-3">
                  <p className="text-xs text-muted leading-5">
                    输入 Clash 订阅链接，后端将自动拉取并解析节点。
                    此方式会创建一条新的订阅记录，后续可在「订阅」页管理。
                  </p>
                  <TextInput value={remoteUrl} onChange={setRemoteUrl} placeholder="https://example.com/clash-subscribe?token=..." />
                </div>
              )}

              {importError && (
                <div className="flex items-center gap-2 text-xs text-danger">
                  <AlertCircle size={13} />{importError}
                </div>
              )}

              <button
                className="btn-primary w-full flex items-center justify-center gap-2"
                onClick={handleImport}
                disabled={importing || (importMode !== 'url' && !pasteContent.trim()) || (importMode === 'url' && !remoteUrl.trim())}
              >
                {importing
                  ? <><Loader2 size={14} className="animate-spin" />解析中…</>
                  : <><ArrowRight size={14} />解析并继续</>}
              </button>
            </div>

            <p className="text-xs text-muted text-center">如果还没有配置文件，可以直接跳过 →
              <button className="ml-1 text-brand hover:underline" onClick={() => { setClashParsed({}); setStep('dns') }}>
                跳过导入，手动设置
              </button>
            </p>
          </div>
        )}

        {/* ─── Step 2: DNS ────────────────────────────────────────────────── */}
        {step === 'dns' && (
          <div className="space-y-4">
            {clashParsed?.dns && (
              <div className="card px-5 py-4 bg-brand/5 border-brand/20 space-y-1">
                <p className="text-xs font-semibold text-brand mb-2">从配置文件中读取到 DNS 设置</p>
                {clashParsed.dns.enable !== undefined && <InfoBadge label="DNS 启用" value={String(clashParsed.dns.enable)} />}
                {clashParsed.dns['enhanced-mode'] && <InfoBadge label="DNS 模式" value={clashParsed.dns['enhanced-mode']} />}
                {clashParsed.dns.listen && <InfoBadge label="监听地址" value={clashParsed.dns.listen} />}
                {clashParsed.dns['fake-ip-range'] && <InfoBadge label="fake-ip 段" value={clashParsed.dns['fake-ip-range']} />}
                {(clashParsed.dns.nameserver ?? []).length > 0 && (
                  <InfoBadge label="上游 DNS" value={(clashParsed.dns.nameserver ?? []).join(', ')} />
                )}
              </div>
            )}

            <div className="card px-5 py-5 space-y-5">
              <h2 className="text-sm font-semibold text-slate-200 border-b border-white/5 pb-3">DNS 设置</h2>

              <Field label="启用 Mihomo DNS" hint="关闭时 Mihomo 使用系统 DNS，不接管查询">
                <Toggle checked={dns.enable} onChange={v => dnsSet('enable', v)} label={dns.enable ? '已启用' : '已禁用'} />
              </Field>

              <Field label="DNS 解析模式" hint="fake-ip 模式下虚构 IP 实现规则分流">
                <SelectInput
                  value={dns.mode} onChange={v => dnsSet('mode', v)}
                  options={[{ value: 'fake-ip', label: 'Fake-IP（推荐）' }, { value: 'redir-host', label: 'Redir-Host' }]}
                />
              </Field>

              <Field label="DNS 监听地址">
                <TextInput value={dns.listen} onChange={v => dnsSet('listen', v)} placeholder="0.0.0.0:7874" />
              </Field>

              <Field label="IPv6 DNS" hint="默认关闭，避免 IPv6 泄露">
                <Toggle checked={dns.ipv6} onChange={v => dnsSet('ipv6', v)} label={dns.ipv6 ? '启用' : '禁用'} />
              </Field>

              <div className="border-t border-white/5 pt-4 space-y-4">
                <h3 className="text-xs font-semibold text-muted uppercase tracking-wider">ClashForge DNS 接管</h3>

                <Field label="dnsmasq 共存模式" hint="none=不干预 dnsmasq；upstream=设为上游；replace=完全替换">
                  <SelectInput
                    value={dns.dnsmasq_mode} onChange={v => dnsSet('dnsmasq_mode', v)}
                    options={[
                      { value: 'none',     label: '不干预 dnsmasq（默认）' },
                      { value: 'upstream', label: 'Mihomo 作为 dnsmasq 上游' },
                      { value: 'replace',  label: '完全替换 dnsmasq' },
                    ]}
                  />
                </Field>

                <Field label="启动时接管 DNS" hint="开启后 ClashForge 启动时自动将 DNS 查询引向 Mihomo">
                  <Toggle checked={dns.apply_on_start} onChange={v => dnsSet('apply_on_start', v)} label={dns.apply_on_start ? '是' : '否'} />
                </Field>
              </div>
            </div>

            <div className="flex gap-3">
              <button className="btn btn-ghost flex-1" onClick={() => setStep('import')}>← 返回</button>
              <button className="btn-primary flex-1 flex items-center justify-center gap-2" onClick={() => setStep('network')}>
                下一步：网络设置 <ChevronRight size={14} />
              </button>
            </div>
          </div>
        )}

        {/* ─── Step 3: Network ────────────────────────────────────────────── */}
        {step === 'network' && (
          <div className="space-y-4">
            {clashParsed && (
              <div className="card px-5 py-4 bg-brand/5 border-brand/20 space-y-1">
                <p className="text-xs font-semibold text-brand mb-2">从配置文件中读取到基本信息</p>
                {clashParsed.mode && <InfoBadge label="代理模式" value={clashParsed.mode} />}
                {clashParsed.port && <InfoBadge label="HTTP 端口" value={String(clashParsed.port)} />}
                {clashParsed['mixed-port'] && <InfoBadge label="混合端口" value={String(clashParsed['mixed-port'])} />}
                {clashParsed['socks-port'] && <InfoBadge label="SOCKS 端口" value={String(clashParsed['socks-port'])} />}
                {clashParsed['allow-lan'] !== undefined && <InfoBadge label="允许局域网" value={String(clashParsed['allow-lan'])} />}
              </div>
            )}

            <div className="card px-5 py-5 space-y-5">
              <h2 className="text-sm font-semibold text-slate-200 border-b border-white/5 pb-3">透明代理 / 网络设置</h2>

              <Field label="透明代理模式" hint="tproxy 适用于 OpenWrt；redir 兼容性更好；tun 模式需内核支持">
                <SelectInput
                  value={net.mode} onChange={v => netSet('mode', v)}
                  options={[
                    { value: 'tproxy', label: 'TProxy（OpenWrt 推荐）' },
                    { value: 'redir',  label: 'Redir-TCP' },
                    { value: 'tun',    label: 'TUN' },
                    { value: 'none',   label: '不接管（仅启动内核）' },
                  ]}
                />
              </Field>

              <Field label="防火墙后端" hint="auto 会自动探测 nftables/iptables">
                <SelectInput
                  value={net.firewall_backend} onChange={v => netSet('firewall_backend', v)}
                  options={[
                    { value: 'auto',      label: '自动探测' },
                    { value: 'nftables',  label: 'nftables' },
                    { value: 'iptables',  label: 'iptables' },
                    { value: 'none',      label: '不配置防火墙' },
                  ]}
                />
              </Field>

              <Field label="绕过局域网" hint="局域网流量不走透明代理">
                <Toggle checked={net.bypass_lan} onChange={v => netSet('bypass_lan', v)} label={net.bypass_lan ? '是' : '否'} />
              </Field>

              <Field label="绕过中国大陆 IP" hint="国内 IP 直连，减少延迟">
                <Toggle checked={net.bypass_china} onChange={v => netSet('bypass_china', v)} label={net.bypass_china ? '是' : '否'} />
              </Field>

              <Field label="启动时自动接管流量" hint="开启后 ClashForge 启动时立即应用透明代理规则">
                <Toggle checked={net.apply_on_start} onChange={v => netSet('apply_on_start', v)} label={net.apply_on_start ? '是' : '否'} />
              </Field>
            </div>

            <div className="flex gap-3">
              <button className="btn btn-ghost flex-1" onClick={() => setStep('dns')}>← 返回</button>
              <button className="btn-primary flex-1 flex items-center justify-center gap-2" onClick={() => setStep('launch')}>
                下一步：启动服务 <ChevronRight size={14} />
              </button>
            </div>
          </div>
        )}

        {/* ─── Step 4: Launch ─────────────────────────────────────────────── */}
        {step === 'launch' && (
          <div className="space-y-4">
            <div className="card px-5 py-5 space-y-4">
              <h2 className="text-sm font-semibold text-slate-200 border-b border-white/5 pb-3">启动前确认</h2>

              {/* Summary */}
              <div className="space-y-2 text-xs">
                <div className="flex items-center gap-2 text-muted">
                  <Wifi size={12} className="text-brand" />
                  <span className="text-slate-300">DNS：</span>
                  {dns.enable ? `启用 · ${dns.mode} · ${dns.listen}` : '禁用'}
                  {dns.apply_on_start && dns.enable && <span className="text-brand">（启动时接管）</span>}
                </div>
                <div className="flex items-center gap-2 text-muted">
                  <Globe size={12} className="text-brand" />
                  <span className="text-slate-300">透明代理：</span>
                  {net.mode === 'none' ? '不接管' : `${net.mode.toUpperCase()} · ${net.firewall_backend}`}
                  {net.apply_on_start && net.mode !== 'none' && <span className="text-brand">（启动时接管）</span>}
                </div>
                <div className="flex items-center gap-2 text-muted">
                  <Sparkles size={12} className="text-brand" />
                  <span className="text-slate-300">绕过局域网：</span>{net.bypass_lan ? '是' : '否'}
                  <span className="text-slate-300 ml-2">绕过国内 IP：</span>{net.bypass_china ? '是' : '否'}
                </div>
              </div>

              {launchError && (
                <div className="rounded-xl bg-danger/10 border border-danger/20 px-4 py-3 flex items-start gap-2">
                  <XCircle size={15} className="text-danger flex-shrink-0 mt-0.5" />
                  <div>
                    <p className="text-sm font-semibold text-danger">启动失败</p>
                    <p className="text-xs text-muted mt-0.5">{launchError}</p>
                  </div>
                </div>
              )}

              {launchDone && (
                <div className="rounded-xl bg-success/10 border border-success/20 px-4 py-3 flex items-center gap-2">
                  <CheckCircle2 size={15} className="text-success flex-shrink-0" />
                  <p className="text-sm font-semibold text-success">内核已启动，即将进入连通检测…</p>
                </div>
              )}

              {!launchDone && (
                <button
                  className="btn-primary w-full flex items-center justify-center gap-2 py-3 text-sm font-semibold"
                  onClick={handleLaunch}
                  disabled={launching}
                >
                  {launching
                    ? <><Loader2 size={16} className="animate-spin" />正在启动内核 + 接管服务…</>
                    : <><Play size={16} />一键启动内核 + 应用接管</>}
                </button>
              )}
            </div>

            <div className="flex gap-3">
              <button className="btn btn-ghost flex-1" onClick={() => setStep('network')} disabled={launching}>← 返回</button>
              {launchDone && (
                <button className="btn-primary flex-1 flex items-center justify-center gap-2" onClick={() => setStep('check')}>
                  开始连通检测 <ChevronRight size={14} />
                </button>
              )}
            </div>
          </div>
        )}

        {/* ─── Step 5: Check ──────────────────────────────────────────────── */}
        {step === 'check' && (
          <div className="space-y-4">
            <div className="card px-5 py-5 space-y-4">
              <div className="flex items-center justify-between">
                <h2 className="text-sm font-semibold text-slate-200">出口 IP / 连通检测</h2>
                <button
                  className="btn btn-ghost text-xs flex items-center gap-1.5"
                  onClick={handleCheck}
                  disabled={checking}
                >
                  <RotateCw size={12} className={checking ? 'animate-spin' : ''} />
                  {checking ? '检测中…' : '重新检测'}
                </button>
              </div>

              {!checkDone && !checking && (
                <button
                  className="btn-primary w-full flex items-center justify-center gap-2"
                  onClick={handleCheck}
                >
                  <Wifi size={14} />开始检测
                </button>
              )}

              {checking && (
                <div className="flex items-center gap-3 text-sm text-muted">
                  <Loader2 size={16} className="animate-spin text-brand" />
                  正在从路由器和浏览器两侧发起检测…
                </div>
              )}

              {/* Router probe results */}
              {routerProbe && (
                <div className="space-y-2">
                  <p className="text-xs font-semibold text-muted uppercase tracking-wider">路由器侧（服务端检测）</p>
                  {routerProbe.ip_checks.reduce((acc, c, i) => {
                    const prev = i > 0 ? routerProbe.ip_checks[i - 1] : null
                    if (!prev || prev.group !== c.group) {
                      acc.push(<p key={`ipg-${c.group}`} className="text-[10px] uppercase tracking-wider text-muted mt-1">{c.group ?? '其他'}组</p>)
                    }
                    acc.push(
                      <div key={i} className={`flex items-start gap-2 text-xs rounded-xl px-3 py-2 ${c.ok ? 'bg-success/8 border border-success/20' : 'bg-danger/8 border border-danger/20'}`}>
                        {c.ok
                          ? <CheckCircle2 size={13} className="text-success flex-shrink-0 mt-0.5" />
                          : <XCircle size={13} className="text-danger flex-shrink-0 mt-0.5" />}
                        <div>
                          <span className={c.ok ? 'text-success' : 'text-danger'}>{c.provider}</span>
                          {c.ok && c.ip && <span className="ml-2 text-slate-300 font-mono">{c.ip}</span>}
                          {c.ok && c.location && <span className="ml-1 text-muted">({c.location})</span>}
                          {!c.ok && c.error && <span className="ml-2 text-muted">{c.error}</span>}
                        </div>
                      </div>
                    )
                    return acc
                  }, [] as React.ReactNode[])}
                  {routerProbe.access_checks.reduce((acc, c, i) => {
                    const prev = i > 0 ? routerProbe.access_checks[i - 1] : null
                    if (!prev || prev.group !== c.group) {
                      acc.push(<p key={`acg-${c.group}`} className="text-[10px] uppercase tracking-wider text-muted mt-1">{c.group ?? '其他'}组</p>)
                    }
                    acc.push(
                      <div key={i} className={`flex items-start gap-2 text-xs rounded-xl px-3 py-2 ${c.ok ? 'bg-success/8 border border-success/20' : 'bg-danger/8 border border-danger/20'}`}>
                        {c.ok
                          ? <CheckCircle2 size={13} className="text-success flex-shrink-0 mt-0.5" />
                          : <XCircle size={13} className="text-danger flex-shrink-0 mt-0.5" />}
                        <div>
                          <span className={c.ok ? 'text-success' : 'text-danger'}>{c.name}</span>
                          {c.ok && c.latency_ms && <span className="ml-2 text-muted">{c.latency_ms} ms</span>}
                          {!c.ok && c.error && <span className="ml-2 text-muted">{c.error}</span>}
                        </div>
                      </div>
                    )
                    return acc
                  }, [] as React.ReactNode[])}
                </div>
              )}

              {/* Browser probe results */}
              {browserProbe && (
                <div className="space-y-2">
                  <p className="text-xs font-semibold text-muted uppercase tracking-wider">浏览器侧（前端直连检测）</p>
                  <div className={`flex items-start gap-2 text-xs rounded-xl px-3 py-2 ${browserProbe.ipOK ? 'bg-success/8 border border-success/20' : 'bg-danger/8 border border-danger/20'}`}>
                    {browserProbe.ipOK
                      ? <CheckCircle2 size={13} className="text-success flex-shrink-0 mt-0.5" />
                      : <XCircle size={13} className="text-danger flex-shrink-0 mt-0.5" />}
                    <div>
                      <span className={browserProbe.ipOK ? 'text-success' : 'text-danger'}>出口 IP 检测 (IP.SB)</span>
                      {browserProbe.ipOK && browserProbe.ip && <span className="ml-2 text-slate-300 font-mono">{browserProbe.ip}</span>}
                      {!browserProbe.ipOK && browserProbe.ipError && <span className="ml-2 text-muted">{browserProbe.ipError}</span>}
                    </div>
                  </div>
                  {browserProbe.accessChecks.reduce((acc, c, i) => {
                    const prev = i > 0 ? browserProbe.accessChecks[i - 1] : null
                    if (!prev || prev.group !== c.group) {
                      acc.push(<p key={`bg-${c.group}`} className="text-[10px] uppercase tracking-wider text-muted mt-1">{c.group ?? '其他'}组</p>)
                    }
                    acc.push(
                      <div key={i} className={`flex items-start gap-2 text-xs rounded-xl px-3 py-2 ${c.ok ? 'bg-success/8 border border-success/20' : 'bg-danger/8 border border-danger/20'}`}>
                        {c.ok
                          ? <CheckCircle2 size={13} className="text-success flex-shrink-0 mt-0.5" />
                          : <XCircle size={13} className="text-danger flex-shrink-0 mt-0.5" />}
                        <div>
                          <span className={c.ok ? 'text-success' : 'text-danger'}>{c.name}</span>
                          {c.ok && c.latency_ms && <span className="ml-2 text-muted">{c.latency_ms} ms</span>}
                          {!c.ok && c.error && <span className="ml-2 text-muted">{c.error}</span>}
                        </div>
                      </div>
                    )
                    return acc
                  }, [] as React.ReactNode[])}
                </div>
              )}

              {/* Overall result */}
              {checkDone && overallOK && (
                <div className="space-y-3">
                  <div className="rounded-xl bg-success/10 border border-success/20 px-5 py-4 flex items-start gap-3">
                    <CheckCircle2 size={20} className="text-success flex-shrink-0 mt-0.5" />
                    <div>
                      <p className="text-sm font-bold text-success">全部检测通过！</p>
                      <p className="text-xs text-muted mt-1">代理工作正常，路由器和浏览器均可正常访问外网。</p>
                    </div>
                  </div>
                  <div className="card px-5 py-4 space-y-4">
                    <h3 className="text-sm font-semibold text-slate-200">完成设置</h3>
                    <Field label="开机自动启动内核" hint="路由器重启后自动启动 ClashForge 并自动启动 Mihomo 内核">
                      <Toggle checked={autoStartCore} onChange={setAutoStartCore} label={autoStartCore ? '启用' : '禁用'} />
                    </Field>
                    {saveError && (
                      <div className="flex items-center gap-2 text-xs text-danger">
                        <AlertCircle size={13} />{saveError}
                      </div>
                    )}
                    <button
                      className="btn-primary w-full flex items-center justify-center gap-2 py-3 text-sm font-semibold"
                      onClick={handleComplete}
                      disabled={saving}
                    >
                      {saving
                        ? <><Loader2 size={16} className="animate-spin" />保存中…</>
                        : <><ArrowRight size={16} />完成配置，进入概览</>}
                    </button>
                  </div>
                </div>
              )}

              {checkDone && !overallOK && (
                <div className="space-y-3">
                  <div className="rounded-xl bg-danger/10 border border-danger/20 px-4 py-3 flex items-start gap-2">
                    <AlertCircle size={15} className="text-danger flex-shrink-0 mt-0.5" />
                    <div>
                      <p className="text-sm font-semibold text-danger">部分检测未通过</p>
                      <p className="text-xs text-muted mt-1">请检查以下日志排查问题，或返回上一步重新配置。</p>
                    </div>
                  </div>

                  {probeLogs.length > 0 && (
                    <div className="card px-4 py-4 space-y-2">
                      <p className="text-xs font-semibold text-muted uppercase tracking-wider">ClashForge 最近日志</p>
                      <div className="max-h-64 overflow-y-auto space-y-1">
                        {probeLogs.map((l, i) => (
                          <div key={i} className={`text-xs font-mono px-2 py-1 rounded ${
                            (l as unknown as Record<string,string>).level === 'error' ? 'text-danger bg-danger/5' :
                            (l as unknown as Record<string,string>).level === 'warn'  ? 'text-warning bg-warning/5' :
                            'text-slate-400'
                          }`}>
                            {(l as unknown as Record<string,string>).ts ? new Date(Number((l as unknown as Record<string,string>).ts) * 1000).toLocaleTimeString() : ''}
                            {' '}
                            <span className="font-semibold uppercase">[{(l as unknown as Record<string,string>).level}]</span>
                            {' '}{(l as unknown as Record<string,string>).msg}
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>

            <div className="flex gap-3">
              <button className="btn btn-ghost flex-1" onClick={() => setStep('launch')}>← 返回</button>
              <button className="btn btn-ghost flex-1" onClick={() => navigate('/')}>
                跳过，直接进入概览
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
