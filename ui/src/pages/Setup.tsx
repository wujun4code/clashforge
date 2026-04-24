import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import {
  Upload, FileText, Globe, CheckCircle2, AlertCircle,
  ChevronRight, Play, Loader2, Wifi, XCircle, ArrowRight,
  Sparkles, RotateCw, Link2, Square, ShieldOff, Database, Radio,
} from 'lucide-react'
import yaml from 'js-yaml'
import {
  updateOverrides, generateConfig, getMihomoConfig, getConfig, updateConfig,
  startCore, stopCore, takeoverOverviewModule, releaseOverviewTakeover,
  getOverviewCore, getOverviewProbes, getLogs,
  addSubscription, getSubscriptions, syncSubUpdate, enableService,
  saveSource, setActiveSource, getSourceFile, getSources,
  detectConflicts, stopService,
} from '../api/client'
import type { OverviewProbeData, OverviewModule, LogEntry, ConflictService, SourceFile, Subscription } from '../api/client'

type InitStatus = 'checking' | 'running' | 'ready'

// ── Types ────────────────────────────────────────────────────────────────────

type Step = 'import' | 'dns' | 'network' | 'launch' | 'check'
const STEPS: { id: Step; label: string }[] = [
  { id: 'import',  label: 'IMPORT_CFG' },
  { id: 'dns',     label: 'DNS_SETUP' },
  { id: 'network', label: 'NET_CONFIG' },
  { id: 'launch',  label: 'LAUNCH_SVC' },
  { id: 'check',   label: 'CONN_CHECK' },
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

// ── Config preview helpers ──────────────────────────────────────────────────

type LineCat = 'dns' | 'geo' | 'port' | 'preserved'
interface AnnotatedLine { text: string; cat: LineCat; label?: string }

const BLOCK_INFO: Record<string, { cat: LineCat; label: string }> = {
  'dns':       { cat: 'dns',  label: 'DNS 配置 — 根据向导选择重写' },
  'geox-url':  { cat: 'geo',  label: 'GeoData 路径 — 使用 ClashForge 管理的本地文件' },
}
const PORT_INFO: Record<string, string> = {
  'port':                'HTTP 代理端口（ClashForge 管理）',
  'socks-port':          'SOCKS5 代理端口（ClashForge 管理）',
  'mixed-port':          '混合代理端口（ClashForge 管理）',
  'redir-port':          '透明代理（redir）端口（ClashForge 管理）',
  'tproxy-port':         'TProxy 代理端口（ClashForge 管理）',
  'external-controller': 'Mihomo API 地址（ClashForge 管理，仅本地访问）',
  'geodata-mode':        'GeoData 模式（ClashForge 管理）',
}

function annotateLines(content: string): AnnotatedLine[] {
  const lines = content.split('\n')
  const result: AnnotatedLine[] = []
  let blockCat: LineCat | '' = ''
  for (const text of lines) {
    const indent = text.search(/\S/)
    // leaving a top-level block
    if (blockCat && indent === 0 && text.trim() !== '') blockCat = ''
    if (indent === 0 || indent === -1) {
      const m = text.match(/^([a-z][a-z0-9-]*):/)
      const key = m?.[1] ?? ''
      if (BLOCK_INFO[key]) {
        blockCat = BLOCK_INFO[key].cat
        result.push({ text, cat: blockCat, label: BLOCK_INFO[key].label })
        continue
      }
      if (PORT_INFO[key]) {
        result.push({ text, cat: 'port', label: PORT_INFO[key] })
        continue
      }
      result.push({ text, cat: 'preserved' })
    } else {
      result.push({ text, cat: blockCat || 'preserved' })
    }
  }
  return result
}

const CAT_ROW: Record<LineCat, string> = {
  dns:       'border-l-2',
  geo:       'border-l-2',
  port:      'border-l-2',
  preserved: '',
}
const CAT_ROW_STYLE: Record<LineCat, React.CSSProperties> = {
  dns:       { background: 'rgba(0,245,255,0.06)', borderLeftColor: 'rgba(0,245,255,0.5)' },
  geo:       { background: 'rgba(255,0,170,0.06)', borderLeftColor: 'rgba(255,0,170,0.5)' },
  port:      { background: 'rgba(255,230,0,0.06)', borderLeftColor: 'rgba(255,230,0,0.5)' },
  preserved: {},
}
const CAT_LABEL_STYLE: Record<LineCat, React.CSSProperties> = {
  dns:       { color: 'rgba(0,245,255,0.6)' },
  geo:       { color: 'rgba(255,0,170,0.6)' },
  port:      { color: 'rgba(255,230,0,0.6)' },
  preserved: {},
}

function ConfigPreview({ content, onContinue }: { content: string; onContinue: () => void }) {
  const lines = annotateLines(content)
  const legend = [
    { style: { background: 'rgba(0,245,255,0.15)', color: '#00F5FF' },   label: 'DNS_CONFIG — rewritten per wizard' },
    { style: { background: 'rgba(255,230,0,0.15)', color: '#FFE600' },   label: 'PORT/API — managed by ClashForge' },
    { style: { background: 'rgba(255,0,170,0.15)', color: '#FF00AA' },   label: 'GEODATA — local files' },
  ]
  return (
    <div className="space-y-4">
      <div className="glass-card px-5 py-4 space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <FileText size={15} className="text-neon-cyan" />
            <h2 className="font-mono text-sm font-semibold uppercase tracking-[0.06em] text-white">CONFIG_PREVIEW</h2>
          </div>
          <span className="font-mono text-[10px] text-muted">{lines.length}_LINES</span>
        </div>
        <div className="flex flex-wrap gap-2 items-center">
          {legend.map(l => (
            <span key={l.label} className="font-mono inline-flex text-[10px] px-2 py-0.5" style={l.style}>{l.label}</span>
          ))}
          <span className="font-mono text-[10px] text-muted">NO_COLOR = PRESERVED</span>
        </div>
        <div className="overflow-auto max-h-96 text-xs font-mono select-text" style={{ background: 'rgba(2,4,8,0.6)', border: '1px solid rgba(0,245,255,0.08)' }}>
          {lines.map((ln, i) => (
            <div key={i} className={`flex items-start gap-2 px-2 py-px leading-5 ${CAT_ROW[ln.cat]}`} style={CAT_ROW_STYLE[ln.cat]}>
              <span className="select-none w-7 flex-shrink-0 text-right tabular-nums" style={{ color: 'rgba(0,245,255,0.2)' }}>{i + 1}</span>
              <span className="flex-1 text-slate-200 whitespace-pre">{ln.text || ' '}</span>
              {ln.label && (
                <span className="flex-shrink-0 text-[10px] pl-3 self-center" style={CAT_LABEL_STYLE[ln.cat]}>← {ln.label}</span>
              )}
            </div>
          ))}
        </div>
      </div>
      <button className="btn-primary w-full flex items-center justify-center gap-2" onClick={onContinue}>
        <ArrowRight size={14} />CONFIRM — CONTINUE_TO_DNS
      </button>
    </div>
  )
}

// ── Small UI helpers ─────────────────────────────────────────────────────────

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="flex-shrink-0 w-44">
        <label className="font-mono text-[10px] uppercase tracking-[0.15em] text-muted">{label}</label>
        {hint && <p className="font-mono text-[9px] text-muted mt-0.5 leading-4">{hint}</p>}
      </div>
      <div className="flex-1">{children}</div>
    </div>
  )
}

function TextInput({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder?: string }) {
  return (
    <input
      className="glass-input"
      value={value}
      onChange={e => onChange(e.target.value)}
      placeholder={placeholder}
    />
  )
}

function SelectInput({ value, onChange, options }: { value: string; onChange: (v: string) => void; options: { value: string; label: string }[] }) {
  return (
    <select
      className="glass-input"
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
        className="w-10 h-5 flex-shrink-0 relative transition-all cursor-pointer"
        style={{
          border: `1px solid ${checked ? 'rgba(0,245,255,0.6)' : 'rgba(74,96,128,0.5)'}`,
          background: checked ? 'rgba(0,245,255,0.15)' : 'rgba(6,12,18,0.8)',
          boxShadow: checked ? '0 0 8px rgba(0,245,255,0.3)' : 'none',
        }}
      >
        <span
          className="absolute top-0.5 w-4 h-4 transition-all"
          style={{
            left: checked ? '1.25rem' : '0.125rem',
            background: checked ? '#00F5FF' : '#4A6080',
            boxShadow: checked ? '0 0 6px rgba(0,245,255,0.8)' : 'none',
          }}
        />
      </button>
      {label && (
        <span className="font-mono text-[10px] uppercase tracking-[0.12em]" style={{ color: checked ? '#00F5FF' : '#4A6080' }}>
          {label}
        </span>
      )}
    </div>
  )
}

function StepBar({ step }: { step: Step }) {
  const idx = STEPS.findIndex(s => s.id === step)
  return (
    <div className="flex items-center gap-0">
      {STEPS.map((s, i) => (
        <div key={s.id} className="flex items-center">
          <div
            className="flex items-center gap-2 px-3 py-1.5 font-mono text-[10px] uppercase tracking-[0.1em] transition-all"
            style={
              i < idx
                ? { color: '#00FF88' }
                : i === idx
                ? { color: '#00F5FF', border: '1px solid rgba(0,245,255,0.3)', background: 'rgba(0,245,255,0.08)', textShadow: '0 0 8px rgba(0,245,255,0.6)' }
                : { color: '#4A6080' }
            }
          >
            {i < idx
              ? <CheckCircle2 size={12} />
              : (
                <span
                  className="w-4 h-4 flex items-center justify-center text-[10px] font-bold"
                  style={i === idx
                    ? { border: '1px solid rgba(0,245,255,0.5)', background: 'rgba(0,245,255,0.2)', color: '#00F5FF' }
                    : { border: '1px solid rgba(74,96,128,0.4)', color: '#4A6080' }
                  }
                >{i + 1}</span>
              )
            }
            {s.label}
          </div>
          {i < STEPS.length - 1 && <ChevronRight size={14} className="mx-1 flex-shrink-0" style={{ color: 'rgba(74,96,128,0.4)' }} />}
        </div>
      ))}
    </div>
  )
}

function InfoBadge({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between py-1.5" style={{ borderBottom: '1px solid rgba(0,245,255,0.06)' }}>
      <span className="font-mono text-[10px] uppercase tracking-[0.12em] text-muted">{label}</span>
      <span className="font-mono text-[10px] text-neon-cyan">{value}</span>
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
    { name: 'Claude',    group: 'AI',   url: 'https://api.anthropic.com' },
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
  const location = useLocation()
  const activateSub = (location.state as { activateSub?: { id: string; name: string; url?: string }; activateFile?: { filename: string } } | null)?.activateSub
  const activateFile = (location.state as { activateSub?: { id: string; name: string; url?: string }; activateFile?: { filename: string } } | null)?.activateFile
  const fileRef = useRef<HTMLInputElement>(null)

  // ── init guard: check if core is already running ──
  const [initStatus, setInitStatus] = useState<InitStatus>('checking')
  const [stopping, setStopping] = useState(false)
  const [stopError, setStopError] = useState('')
  const [runningModules, setRunningModules] = useState<OverviewModule[]>([])

  useEffect(() => {
    getOverviewCore().then(async data => {
      if (data.core.state === 'running') {
        setRunningModules(data.modules ?? [])
        if (activateSub || activateFile) {
          // Auto-stop when switching config via activate
          setStopping(true)
          try {
            await stopCore()
            await releaseOverviewTakeover().catch(() => null)
            setInitStatus('ready')
          } catch (e) {
            setStopError(e instanceof Error ? e.message : '停止失败')
            setInitStatus('running')
          } finally {
            setStopping(false)
          }
        } else {
          setInitStatus('running')
        }
      } else {
        setInitStatus('ready')
      }
    }).catch(() => setInitStatus('ready'))
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

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
  type ImportMode = 'file' | 'paste' | 'url' | 'existing' | 'existing_file' | 'saved'
  const initMode = (): ImportMode => {
    if (activateSub) return 'existing'
    if (activateFile) return 'existing_file'
    return 'saved'
  }
  const [importMode, setImportMode] = useState<ImportMode>(initMode)

  // ── saved sources/subs list ──
  const [savedFiles, setSavedFiles] = useState<SourceFile[]>([])
  const [savedSubs, setSavedSubs] = useState<Subscription[]>([])
  const [savedLoading, setSavedLoading] = useState(false)
  const [selectedSaved, setSelectedSaved] = useState<{ kind: 'file'; filename: string } | { kind: 'sub'; sub: Subscription } | null>(null)
  const [subImportChoice, setSubImportChoice] = useState<'cache' | 'live' | null>(null)
  const [subLiveFailed, setSubLiveFailed] = useState(false)
  const [pasteContent, setPasteContent] = useState('')
  const [uploadedFileName, setUploadedFileName] = useState<string | null>(null)
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

  // ── import preview ──
  const [previewContent, setPreviewContent] = useState('')

  // ── launch step ──
  const [launching, setLaunching] = useState(false)
  const [launchDone, setLaunchDone] = useState(false)
  const [launchError, setLaunchError] = useState('')
  const [conflicts, setConflicts] = useState<ConflictService[]>([])
  const [stoppingConflicts, setStoppingConflicts] = useState(false)
  const [conflictStopped, setConflictStopped] = useState(false)

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

  // ── load saved sources when on saved tab ──
  useEffect(() => {
    if (importMode !== 'saved') return
    setSavedLoading(true)
    Promise.all([
      getSources().catch(() => ({ files: [] as SourceFile[], active_source: null })),
      getSubscriptions().catch(() => ({ subscriptions: [] as Subscription[] })),
    ]).then(([s, sub]) => {
      setSavedFiles(s.files ?? [])
      setSavedSubs(sub.subscriptions ?? [])
    }).finally(() => setSavedLoading(false))
  }, [importMode])

  // ── reset subscription import choice when selection changes ──
  useEffect(() => {
    setSubImportChoice(null)
    setSubLiveFailed(false)
  }, [selectedSaved])

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
      setUploadedFileName(file.name)
      setImportMode('file')
    }
    reader.readAsText(file)
  }, [])

  // ── import: save overrides + generate + parse ──
  const handleImport = useCallback(async () => {
    setImporting(true); setImportError('')
    try {
      const showPreview = async () => {
        const { content } = await getMihomoConfig().catch(() => ({ content: '' }))
        setPreviewContent(content)
      }

      // Saved config/sub selection mode
      if (importMode === 'saved') {
        if (!selectedSaved) { setImportError('请选择一个配置'); return }
        if (selectedSaved.kind === 'file') {
          const { content } = await getSourceFile(selectedSaved.filename)
          try {
            const parsed = yaml.load(content) as ClashParsed
            if (parsed && typeof parsed === 'object') applyClashParsed(parsed)
          } catch { /* ignore */ }
          await updateOverrides(content)
          await generateConfig().catch(() => null)
          await setActiveSource({ type: 'file', filename: selectedSaved.filename }).catch(() => null)
          setClashParsed({})
        } else {
          const sub = selectedSaved.sub
          // Default: use local cache if available (avoids network dead-loop).
          const wantLive = subImportChoice === 'live'
          const hasCache = !!sub.has_cache
          if (wantLive || !hasCache) {
            try {
              await syncSubUpdate(sub.id)
            } catch (e: unknown) {
              if (hasCache) {
                setSubLiveFailed(true)
                setImportError('在线更新失败，请选择"使用本地缓存"继续。（' + (e instanceof Error ? e.message : String(e)) + '）')
                return
              }
              throw e
            }
          } else {
            // Using cached version — no network fetch needed
          }
          await generateConfig().catch(() => null)
          await setActiveSource({ type: 'subscription', sub_id: sub.id, sub_name: sub.name }).catch(() => null)
          setClashParsed({})
        }
        await showPreview()
        return
      }

      // Existing subscription mode: download (sync) + generate
      if (importMode === 'existing' && activateSub) {
        await syncSubUpdate(activateSub.id)
        await generateConfig().catch(() => null)
        await setActiveSource({ type: 'subscription', sub_id: activateSub.id, sub_name: activateSub.name }).catch(() => null)
        setClashParsed({})
        await showPreview()
        return
      }
      // Existing source file mode: load content from disk
      if (importMode === 'existing_file' && activateFile) {
        const { content } = await getSourceFile(activateFile.filename)
        try {
          const parsed = yaml.load(content) as ClashParsed
          if (parsed && typeof parsed === 'object') applyClashParsed(parsed)
        } catch {
          // ignore YAML parse errors – backend validates
        }
        await updateOverrides(content)
        await generateConfig().catch(() => null)
        await setActiveSource({ type: 'file', filename: activateFile.filename }).catch(() => null)
        setClashParsed({})
        await showPreview()
        return
      }
      if (importMode === 'url') {
        // Reuse existing subscription with same URL instead of creating a duplicate.
        if (!remoteUrl.trim()) { setImportError('请输入订阅链接'); return }
        const subName = (() => { try { return new URL(remoteUrl).hostname } catch { return '远程订阅' } })()
        const existing = await getSubscriptions().catch(() => ({ subscriptions: [] }))
        const matched = existing.subscriptions.find(s => s.url === remoteUrl.trim())
        const subId = matched
          ? matched.id
          : (await addSubscription({ name: subName, url: remoteUrl, type: 'clash', enabled: true })).id
        await syncSubUpdate(subId)
        await generateConfig().catch(() => null)
        await setActiveSource({ type: 'subscription', sub_id: subId, sub_name: subName }).catch(() => null)
        setClashParsed({})
        await showPreview()
        return
      }
      const yamlContent = pasteContent
      if (!yamlContent.trim()) { setImportError('内容为空，请粘贴或上传配置文件'); return }

      // Parse client-side for form pre-fill
      try {
        const parsed = yaml.load(yamlContent) as ClashParsed
        if (parsed && typeof parsed === 'object') applyClashParsed(parsed)
      } catch {
        // ignore YAML parse errors – backend validates
      }

      // Save source file (paste → auto date+ver; upload → original filename)
      const suggestedName = (importMode === 'file' && uploadedFileName) ? uploadedFileName : undefined
      const { filename } = await saveSource(yamlContent, suggestedName).catch(() => ({ filename: '' }))
      if (filename) {
        await setActiveSource({ type: 'file', filename }).catch(() => null)
      }

      // Save as overrides and generate
      await updateOverrides(yamlContent)
      await generateConfig().catch(() => null)
      await showPreview()
    } catch (e: unknown) {
      setImportError(e instanceof Error ? e.message : String(e))
    } finally { setImporting(false) }
  }, [importMode, activateSub, activateFile, pasteContent, uploadedFileName, remoteUrl, applyClashParsed, selectedSaved])

  // ── launch ──
  const handleLaunch = useCallback(async () => {
    setLaunching(true); setLaunchError('')
    try {
      // Step 1: detect conflicts
      const { conflicts: found } = await detectConflicts().catch(() => ({ conflicts: [], has_conflict: false }))
      if (found.length > 0) {
        setConflicts(found)
        setLaunching(false)
        return
      }

      // Step 2: persist ClashForge config (DNS + network)
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

      // Step 3: start core
      await startCore()

      // Step 4: apply takeover if requested
      if (net.mode !== 'none' && net.apply_on_start) {
        await takeoverOverviewModule({ module: 'transparent_proxy', mode: net.mode })
      }
      if (dns.enable && dns.apply_on_start) {
        await takeoverOverviewModule({ module: 'dns_entry' })
      }

      setLaunchDone(true)
      setTimeout(() => setStep('check'), 800)
    } catch (e: unknown) {
      setLaunchError(e instanceof Error ? e.message : String(e))
    } finally { setLaunching(false) }
  }, [dns, net])

  // ── stop conflicts then re-launch ──
  const handleStopConflicts = useCallback(async () => {
    setStoppingConflicts(true)
    try {
      for (const svc of conflicts) {
        if (svc.name === 'openclash') {
          await stopService('openclash').catch(() => null)
        }
        // for other services (mihomo, clash) use generic kill via stop-service
        // currently those map to openclash stop script as best-effort
      }
      setConflictStopped(true)
      setConflicts([])
      // Re-run launch now that conflicts are cleared
      await handleLaunch()
    } finally {
      setStoppingConflicts(false)
    }
  }, [conflicts, handleLaunch])

  // ── auto-detect conflicts when entering launch step ──
  useEffect(() => {
    if (step !== 'launch' || launchDone) return
    detectConflicts()
      .then(({ conflicts: found }) => setConflicts(found))
      .catch(() => null)
  }, [step]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── auto-launch when switching configs via activate (skip DNS/network steps) ──
  useEffect(() => {
    if (step === 'launch' && (activateSub || activateFile) && !launching && !launchDone) {
      const t = setTimeout(() => { void handleLaunch() }, 0)
      return () => clearTimeout(t)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step, handleLaunch])

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
      <div className="min-h-full flex items-center justify-center">
        <div className="flex items-center gap-3 font-mono text-[10px] uppercase tracking-[0.2em] text-muted">
          <Loader2 size={18} className="animate-spin" style={{ color: '#00F5FF' }} />
          DETECTING_RUNTIME_STATE...
        </div>
      </div>
    )
  }

  // Guard: core is running — require stopping before re-running wizard
  if (initStatus === 'running') {
    const managed = runningModules.filter(m => m.managed_by_clashforge)
    return (
      <div className="min-h-full px-6 py-8">
        <div className="max-w-xl mx-auto space-y-6">
          <div className="flex items-center gap-3">
            <div className="p-2.5" style={{ border: '1px solid rgba(0,245,255,0.25)', background: 'rgba(0,245,255,0.08)' }}>
              <Sparkles size={18} style={{ color: '#00F5FF' }} />
            </div>
            <div>
              <p className="font-mono text-[10px] uppercase tracking-[0.3em] text-muted">SYS_PROXY</p>
              <h1 className="font-mono text-base font-bold uppercase tracking-[0.06em] text-white mt-1" style={{ textShadow: '0 0 12px rgba(0,245,255,0.3)' }}>PROXY_SERVICE</h1>
              <p className="font-mono text-[10px] text-muted mt-0.5">STOP_REQUIRED before reconfiguration</p>
            </div>
          </div>

          <div className="glass-card px-5 py-5 space-y-4">
            <div className="flex items-center gap-3">
              <span className="inline-flex h-2 w-2 animate-pulse" style={{ background: '#00FF88', boxShadow: '0 0 6px rgba(0,255,136,0.8)' }} />
              <p className="font-mono text-sm font-semibold uppercase tracking-[0.06em]" style={{ color: '#00FF88' }}>CORE_RUNNING</p>
            </div>
            <p className="font-mono text-[10px] text-muted leading-6">
              ClashForge core is ACTIVE and has taken over system services listed below.
              Stop the core and release all takeovers before reconfiguring.
            </p>

            {managed.length > 0 && (
              <div className="px-4 py-3 space-y-2" style={{ border: '1px solid rgba(0,245,255,0.1)', background: 'rgba(0,245,255,0.03)' }}>
                <p className="font-mono text-[10px] uppercase tracking-[0.2em] text-muted">ACTIVE_TAKEOVERS</p>
                {managed.map(m => (
                  <div key={m.id} className="flex items-center gap-2">
                    <ShieldOff size={12} style={{ color: '#FFE600' }} />
                    <span className="font-mono text-[10px] text-white">{m.title}</span>
                    <span className="font-mono text-[10px] text-muted">— {m.current_owner}</span>
                  </div>
                ))}
              </div>
            )}

            {stopError && (
              <div className="flex items-center gap-2 font-mono text-[10px]" style={{ color: '#FF2255' }}>
                <AlertCircle size={13} />{stopError}
              </div>
            )}

            <button
              className="btn-danger w-full flex items-center justify-center gap-2 py-3"
              onClick={handleStopAll}
              disabled={stopping}
            >
              {stopping
                ? <><Loader2 size={15} className="animate-spin" />STOPPING...</>
                : <><Square size={15} />STOP_CORE + RELEASE_ALL</>}
            </button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-full px-6 py-8">
      <div className="max-w-2xl mx-auto space-y-6">

        {/* Header */}
        <div className="flex items-center gap-3">
          <div className="p-2.5" style={{ border: '1px solid rgba(0,245,255,0.25)', background: 'rgba(0,245,255,0.08)' }}>
            <Sparkles size={18} style={{ color: '#00F5FF' }} />
          </div>
          <div>
            <p className="font-mono text-[10px] uppercase tracking-[0.3em] text-muted">SETUP_WIZARD</p>
            <h1 className="font-mono text-base font-bold uppercase tracking-[0.06em] text-white mt-1" style={{ textShadow: '0 0 12px rgba(0,245,255,0.3)' }}>PROXY_SETUP</h1>
            <p className="font-mono text-[10px] text-muted mt-0.5">IMPORT → DNS → NETWORK → LAUNCH → VERIFY</p>
          </div>
        </div>

        {/* Step bar */}
        <div className="overflow-x-auto">
          <StepBar step={step} />
        </div>

        {/* ─── Step 1: Import ─────────────────────────────────────────────── */}
        {step === 'import' && previewContent && (
          <ConfigPreview
            content={previewContent}
            onContinue={() => { setPreviewContent(''); setStep('dns') }}
          />
        )}
        {step === 'import' && !previewContent && (
          <div className="space-y-4">
            <div className="glass-card px-5 py-5 space-y-4">
              <div className="flex items-center gap-2 mb-1">
                <FileText size={16} style={{ color: '#00F5FF' }} />
                <h2 className="font-mono text-sm font-semibold uppercase tracking-[0.06em] text-white">SELECT_IMPORT_MODE</h2>
              </div>
              {importMode !== 'existing' && importMode !== 'existing_file' && (
                <div className="flex gap-2 flex-wrap">
                  {([
                    { id: 'saved', icon: <Database size={13} />, label: 'SAVED_CFG' },
                    { id: 'paste', icon: <FileText size={13} />, label: 'PASTE_YAML' },
                    { id: 'file',  icon: <Upload size={13} />,   label: 'UPLOAD_FILE' },
                    { id: 'url',   icon: <Link2 size={13} />,    label: 'SUB_URL' },
                  ] as const).map(m => (
                    <button
                      key={m.id}
                      onClick={() => setImportMode(m.id)}
                      className={`font-mono text-[10px] uppercase tracking-[0.1em] py-1.5 px-3 flex items-center gap-1.5 transition-all cursor-pointer ${importMode === m.id ? 'btn-primary' : 'btn-ghost'}`}
                    >
                      {m.icon}{m.label}
                    </button>
                  ))}
                </div>
              )}

              {importMode === 'saved' && (
                <div className="space-y-3">
                  {savedLoading && <p className="font-mono text-[10px] text-muted uppercase tracking-[0.15em]">LOADING...</p>}
                  {!savedLoading && savedFiles.length === 0 && savedSubs.length === 0 && (
                    <p className="font-mono text-[10px] text-muted">NO_SAVED_CONFIGS — use another import mode.</p>
                  )}
                  {savedFiles.length > 0 && (
                    <div className="space-y-1.5">
                      <p className="font-mono text-[10px] uppercase tracking-[0.2em] text-muted">CONFIG_FILES</p>
                      {savedFiles.map(f => {
                        const selected = selectedSaved?.kind === 'file' && selectedSaved.filename === f.filename
                        return (
                          <button
                            key={f.filename}
                            onClick={() => setSelectedSaved({ kind: 'file', filename: f.filename })}
                            className="w-full flex items-center gap-3 px-4 py-3 text-left transition-all cursor-pointer"
                            style={{
                              border: selected ? '1px solid rgba(0,245,255,0.5)' : '1px solid rgba(0,245,255,0.1)',
                              background: selected ? 'rgba(0,245,255,0.08)' : 'rgba(2,4,8,0.4)',
                            }}
                          >
                            <FileText size={14} className="flex-shrink-0" style={{ color: selected ? '#00F5FF' : '#4A6080' }} />
                            <div className="flex-1 min-w-0">
                              <p className="font-mono text-xs truncate" style={{ color: selected ? '#00F5FF' : '#CBD5E1' }}>{f.filename}</p>
                              <p className="font-mono text-[10px] text-muted mt-0.5">{(f.size_bytes / 1024).toFixed(1)} KB · {new Date(f.created_at).toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</p>
                            </div>
                            {selected && <CheckCircle2 size={14} className="flex-shrink-0" style={{ color: '#00F5FF' }} />}
                          </button>
                        )
                      })}
                    </div>
                  )}
                  {savedSubs.length > 0 && (
                    <div className="space-y-1.5">
                      <p className="font-mono text-[10px] uppercase tracking-[0.2em] text-muted">SUBSCRIPTIONS</p>
                      {savedSubs.map(sub => {
                        const selected = selectedSaved?.kind === 'sub' && selectedSaved.sub.id === sub.id
                        return (
                          <button
                            key={sub.id}
                            onClick={() => setSelectedSaved({ kind: 'sub', sub })}
                            className="w-full flex items-center gap-3 px-4 py-3 text-left transition-all cursor-pointer"
                            style={{
                              border: selected ? '1px solid rgba(0,245,255,0.5)' : '1px solid rgba(0,245,255,0.1)',
                              background: selected ? 'rgba(0,245,255,0.08)' : 'rgba(2,4,8,0.4)',
                            }}
                          >
                            <Radio size={14} className="flex-shrink-0" style={{ color: selected ? '#00F5FF' : '#4A6080' }} />
                            <div className="flex-1 min-w-0">
                              <p className="font-mono text-xs truncate" style={{ color: selected ? '#00F5FF' : '#CBD5E1' }}>{sub.name}</p>
                              <p className="font-mono text-[10px] text-muted mt-0.5">{sub.node_count ? `${sub.node_count}_NODES · ` : ''}{sub.url ? sub.url : 'NO_URL'}</p>
                            </div>
                            {selected && <CheckCircle2 size={14} className="flex-shrink-0" style={{ color: '#00F5FF' }} />}
                          </button>
                        )
                      })}
                      {selectedSaved?.kind === 'sub' && selectedSaved.sub.has_cache && (
                        <div className="mt-2 px-4 py-3 space-y-2" style={{ border: '1px solid rgba(0,245,255,0.1)', background: 'rgba(0,245,255,0.03)' }}>
                          <p className="font-mono text-[10px] uppercase tracking-[0.15em] text-white">SUB_UPDATE_MODE</p>
                          <div className="flex gap-2">
                            <button
                              onClick={() => { setSubImportChoice('cache'); setSubLiveFailed(false) }}
                              className="flex-1 flex items-center justify-center gap-1.5 py-2 font-mono text-[10px] uppercase tracking-[0.1em] transition-all cursor-pointer"
                              style={(subImportChoice === 'cache' || subImportChoice === null)
                                ? { border: '1px solid rgba(0,245,255,0.5)', background: 'rgba(0,245,255,0.1)', color: '#00F5FF' }
                                : { border: '1px solid rgba(0,245,255,0.1)', background: 'transparent', color: '#4A6080' }
                              }
                            >
                              <Database size={12} />USE_LOCAL_CACHE
                            </button>
                            <button
                              onClick={() => { setSubImportChoice('live'); setSubLiveFailed(false) }}
                              className="flex-1 flex items-center justify-center gap-1.5 py-2 font-mono text-[10px] uppercase tracking-[0.1em] transition-all cursor-pointer"
                              style={subImportChoice === 'live'
                                ? { border: '1px solid rgba(0,245,255,0.5)', background: 'rgba(0,245,255,0.1)', color: '#00F5FF' }
                                : { border: '1px solid rgba(0,245,255,0.1)', background: 'transparent', color: '#4A6080' }
                              }
                            >
                              <Link2 size={12} />LIVE_UPDATE
                            </button>
                          </div>
                          {(subImportChoice === 'cache' || subImportChoice === null) && selectedSaved.sub.last_updated && (
                            <p className="font-mono text-[10px] text-muted">
                              CACHED: {new Date(selectedSaved.sub.last_updated).toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}
                              {selectedSaved.sub.node_count ? `  ·  ${selectedSaved.sub.node_count}_NODES` : ''}
                            </p>
                          )}
                          {subLiveFailed && (
                            <div className="flex items-center gap-2 font-mono text-[10px]" style={{ color: '#FFE600' }}>
                              <AlertCircle size={12} />LIVE_UPDATE_FAILED — select USE_LOCAL_CACHE to continue.
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}

              {importMode === 'existing' && activateSub && (
                <div className="space-y-3">
                  <p className="font-mono text-[10px] text-muted leading-5">Will fetch latest nodes from subscription and regenerate config.</p>
                  <div className="px-4 py-3 space-y-1" style={{ background: 'rgba(0,245,255,0.06)', border: '1px solid rgba(0,245,255,0.3)' }}>
                    <p className="font-mono text-sm font-semibold text-white">{activateSub.name}</p>
                    {activateSub.url && <p className="font-mono text-[10px] text-muted truncate">{activateSub.url}</p>}
                  </div>
                  <button className="font-mono text-[10px] text-muted hover:text-neon-cyan underline underline-offset-2 transition-colors cursor-pointer" onClick={() => setImportMode('paste')}>
                    SWITCH_TO_MANUAL_IMPORT
                  </button>
                </div>
              )}

              {importMode === 'existing_file' && activateFile && (
                <div className="space-y-3">
                  <p className="font-mono text-[10px] text-muted leading-5">Will load saved config file and regenerate config.</p>
                  <div className="px-4 py-3 space-y-1" style={{ background: 'rgba(0,245,255,0.06)', border: '1px solid rgba(0,245,255,0.3)' }}>
                    <p className="font-mono text-sm font-semibold text-white">{activateFile.filename}</p>
                    <p className="font-mono text-[10px] text-muted">SOURCE: config file list</p>
                  </div>
                  <button className="font-mono text-[10px] text-muted hover:text-neon-cyan underline underline-offset-2 transition-colors cursor-pointer" onClick={() => setImportMode('paste')}>
                    SWITCH_TO_MANUAL_IMPORT
                  </button>
                </div>
              )}

              {importMode === 'paste' && (
                <div className="space-y-3">
                  <p className="font-mono text-[10px] text-muted leading-5">
                    Paste complete Clash / Mihomo YAML config (local config file or subscription content).
                  </p>
                  <textarea
                    className="w-full px-3 py-3 font-mono text-xs text-white outline-none resize-none"
                    style={{ background: 'rgba(2,4,8,0.8)', border: '1px solid rgba(0,245,255,0.15)' }}
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
                  <p className="font-mono text-[10px] text-muted leading-5">Upload .yaml / .yml config file.</p>
                  <div
                    className="px-6 py-12 flex flex-col items-center gap-3 transition-all cursor-pointer"
                    style={{ border: '2px dashed rgba(0,245,255,0.15)', background: 'rgba(2,4,8,0.4)' }}
                    onClick={() => fileRef.current?.click()}
                    onDragOver={e => e.preventDefault()}
                    onDrop={e => { e.preventDefault(); const f = e.dataTransfer.files[0]; if (f) handleFile(f) }}
                  >
                    <Upload size={28} className="text-muted" />
                    <div className="text-center">
                      <p className="font-mono text-sm font-medium text-white">CLICK_UPLOAD or DROP_FILE</p>
                      <p className="font-mono text-[10px] text-muted mt-1">.yaml / .yml format</p>
                    </div>
                    <input
                      ref={fileRef} type="file" accept=".yaml,.yml,.txt" className="hidden"
                      onChange={e => { const f = e.target.files?.[0]; if (f) handleFile(f) }}
                    />
                  </div>
                  {pasteContent && (
                    <div className="px-4 py-2 flex items-center gap-2" style={{ background: 'rgba(0,255,136,0.06)', border: '1px solid rgba(0,255,136,0.2)' }}>
                      <CheckCircle2 size={14} className="flex-shrink-0" style={{ color: '#00FF88' }} />
                      <span className="font-mono text-[10px]" style={{ color: '#00FF88' }}>FILE_LOADED — {pasteContent.split('\n').length}_LINES</span>
                    </div>
                  )}
                </div>
              )}

              {importMode === 'url' && (
                <div className="space-y-3">
                  <p className="font-mono text-[10px] text-muted leading-5">
                    Enter Clash subscription URL. Backend will fetch and parse nodes automatically.
                    A new subscription record will be created and can be managed in the Subscriptions page.
                  </p>
                  <TextInput value={remoteUrl} onChange={setRemoteUrl} placeholder="https://example.com/clash-subscribe?token=..." />
                </div>
              )}

              {importError && (
                <div className="flex items-center gap-2 font-mono text-[10px]" style={{ color: '#FF2255' }}>
                  <AlertCircle size={13} />{importError}
                </div>
              )}

              <button
                className="btn-primary w-full flex items-center justify-center gap-2"
                onClick={handleImport}
                disabled={
                  importing ||
                  (!selectedSaved && !pasteContent.trim() && !remoteUrl.trim())
                }
              >
                {importing
                  ? <><Loader2 size={14} className="animate-spin" />PARSING...</>
                  : <><ArrowRight size={14} />PARSE_AND_CONTINUE</>}
              </button>
            </div>

            <p className="font-mono text-[10px] text-muted text-center">
              No config file yet?{' '}
              <button className="text-neon-cyan hover:underline cursor-pointer" onClick={() => { setClashParsed({}); setStep('dns') }}>
                SKIP_IMPORT — MANUAL_SETUP
              </button>
            </p>
          </div>
        )}

        {/* ─── Step 2: DNS ────────────────────────────────────────────────── */}
        {step === 'dns' && (
          <div className="space-y-4">
            {clashParsed?.dns && (
              <div className="glass-card px-5 py-4 space-y-1" style={{ background: 'rgba(0,245,255,0.04)', borderColor: 'rgba(0,245,255,0.2)' }}>
                <p className="font-mono text-[10px] uppercase tracking-[0.2em] mb-2" style={{ color: '#00F5FF' }}>DNS_DETECTED_FROM_CONFIG</p>
                {clashParsed.dns.enable !== undefined && <InfoBadge label="DNS_ENABLE" value={String(clashParsed.dns.enable)} />}
                {clashParsed.dns['enhanced-mode'] && <InfoBadge label="DNS_MODE" value={clashParsed.dns['enhanced-mode']} />}
                {clashParsed.dns.listen && <InfoBadge label="LISTEN_ADDR" value={clashParsed.dns.listen} />}
                {clashParsed.dns['fake-ip-range'] && <InfoBadge label="FAKE_IP_RANGE" value={clashParsed.dns['fake-ip-range']} />}
                {(clashParsed.dns.nameserver ?? []).length > 0 && (
                  <InfoBadge label="UPSTREAM_DNS" value={(clashParsed.dns.nameserver ?? []).join(', ')} />
                )}
              </div>
            )}

            <div className="glass-card px-5 py-5 space-y-5">
              <h2 className="font-mono text-sm font-semibold uppercase tracking-[0.06em] text-white" style={{ borderBottom: '1px solid rgba(0,245,255,0.08)', paddingBottom: '0.75rem' }}>
                <span style={{ color: 'rgba(0,245,255,0.4)', marginRight: '0.25rem' }}>{'>'}</span>DNS_SETTINGS
              </h2>

              <Field label="MIHOMO_DNS" hint="When disabled, Mihomo uses system DNS">
                <Toggle checked={dns.enable} onChange={v => dnsSet('enable', v)} label={dns.enable ? 'ENABLED' : 'DISABLED'} />
              </Field>

              <Field label="DNS_MODE" hint="fake-ip mode uses virtual IPs for rule-based routing">
                <SelectInput
                  value={dns.mode} onChange={v => dnsSet('mode', v)}
                  options={[{ value: 'fake-ip', label: 'FAKE_IP (recommended)' }, { value: 'redir-host', label: 'REDIR_HOST' }]}
                />
              </Field>

              <Field label="LISTEN_ADDR">
                <TextInput value={dns.listen} onChange={v => dnsSet('listen', v)} placeholder="0.0.0.0:7874" />
              </Field>

              <Field label="IPV6_DNS" hint="Disabled by default to prevent IPv6 leaks">
                <Toggle checked={dns.ipv6} onChange={v => dnsSet('ipv6', v)} label={dns.ipv6 ? 'ENABLED' : 'DISABLED'} />
              </Field>

              <div className="space-y-4 pt-4" style={{ borderTop: '1px solid rgba(0,245,255,0.08)' }}>
                <h3 className="font-mono text-[10px] uppercase tracking-[0.2em] text-muted">CLASHFORGE_DNS_TAKEOVER</h3>

                <Field label="DNSMASQ_MODE" hint="none=no-op; upstream=set as upstream; replace=full replace">
                  <SelectInput
                    value={dns.dnsmasq_mode} onChange={v => dnsSet('dnsmasq_mode', v)}
                    options={[
                      { value: 'none',     label: 'NONE (default)' },
                      { value: 'upstream', label: 'MIHOMO_AS_UPSTREAM' },
                      { value: 'replace',  label: 'FULL_REPLACE' },
                    ]}
                  />
                </Field>

                <Field label="AUTO_TAKEOVER_ON_START" hint="ClashForge will route DNS to Mihomo on startup">
                  <Toggle checked={dns.apply_on_start} onChange={v => dnsSet('apply_on_start', v)} label={dns.apply_on_start ? 'YES' : 'NO'} />
                </Field>
              </div>
            </div>

            <div className="flex gap-3">
              <button className="btn-ghost flex-1" onClick={() => setStep('import')}>← BACK</button>
              <button className="btn-primary flex-1 flex items-center justify-center gap-2" onClick={() => setStep('network')}>
                NEXT: NET_CONFIG <ChevronRight size={14} />
              </button>
            </div>
          </div>
        )}

        {/* ─── Step 3: Network ────────────────────────────────────────────── */}
        {step === 'network' && (
          <div className="space-y-4">
            {clashParsed && (
              <div className="glass-card px-5 py-4 space-y-1" style={{ background: 'rgba(0,245,255,0.04)', borderColor: 'rgba(0,245,255,0.2)' }}>
                <p className="font-mono text-[10px] uppercase tracking-[0.2em] mb-2" style={{ color: '#00F5FF' }}>NET_INFO_FROM_CONFIG</p>
                {clashParsed.mode && <InfoBadge label="PROXY_MODE" value={clashParsed.mode} />}
                {clashParsed.port && <InfoBadge label="HTTP_PORT" value={String(clashParsed.port)} />}
                {clashParsed['mixed-port'] && <InfoBadge label="MIXED_PORT" value={String(clashParsed['mixed-port'])} />}
                {clashParsed['socks-port'] && <InfoBadge label="SOCKS_PORT" value={String(clashParsed['socks-port'])} />}
                {clashParsed['allow-lan'] !== undefined && <InfoBadge label="ALLOW_LAN" value={String(clashParsed['allow-lan'])} />}
              </div>
            )}

            <div className="glass-card px-5 py-5 space-y-5">
              <h2 className="font-mono text-sm font-semibold uppercase tracking-[0.06em] text-white" style={{ borderBottom: '1px solid rgba(0,245,255,0.08)', paddingBottom: '0.75rem' }}>
                <span style={{ color: 'rgba(0,245,255,0.4)', marginRight: '0.25rem' }}>{'>'}</span>TPROXY_NET_SETTINGS
              </h2>

              <Field label="TPROXY_MODE" hint="tproxy: OpenWrt recommended; redir: better compat; tun: kernel support required">
                <SelectInput
                  value={net.mode} onChange={v => netSet('mode', v)}
                  options={[
                    { value: 'tproxy', label: 'TPROXY (OpenWrt recommended)' },
                    { value: 'redir',  label: 'REDIR_TCP' },
                    { value: 'tun',    label: 'TUN' },
                    { value: 'none',   label: 'NONE (core only)' },
                  ]}
                />
              </Field>

              <Field label="FIREWALL_BACKEND" hint="auto detects nftables/iptables">
                <SelectInput
                  value={net.firewall_backend} onChange={v => netSet('firewall_backend', v)}
                  options={[
                    { value: 'auto',      label: 'AUTO_DETECT' },
                    { value: 'nftables',  label: 'NFTABLES' },
                    { value: 'iptables',  label: 'IPTABLES' },
                    { value: 'none',      label: 'NO_FIREWALL' },
                  ]}
                />
              </Field>

              <Field label="BYPASS_LAN" hint="LAN traffic bypasses transparent proxy">
                <Toggle checked={net.bypass_lan} onChange={v => netSet('bypass_lan', v)} label={net.bypass_lan ? 'YES' : 'NO'} />
              </Field>

              <Field label="BYPASS_CN_IPS" hint="Direct connection for mainland China IPs, reduces latency">
                <Toggle checked={net.bypass_china} onChange={v => netSet('bypass_china', v)} label={net.bypass_china ? 'YES' : 'NO'} />
              </Field>

              <Field label="AUTO_TAKEOVER_ON_START" hint="Apply transparent proxy rules immediately on startup">
                <Toggle checked={net.apply_on_start} onChange={v => netSet('apply_on_start', v)} label={net.apply_on_start ? 'YES' : 'NO'} />
              </Field>
            </div>

            <div className="flex gap-3">
              <button className="btn-ghost flex-1" onClick={() => setStep('dns')}>← BACK</button>
              <button className="btn-primary flex-1 flex items-center justify-center gap-2" onClick={() => setStep('launch')}>
                NEXT: LAUNCH_SVC <ChevronRight size={14} />
              </button>
            </div>
          </div>
        )}

        {/* ─── Step 4: Launch ─────────────────────────────────────────────── */}
        {step === 'launch' && (
          <div className="space-y-4">
            <div className="glass-card px-5 py-5 space-y-4">
              <h2 className="font-mono text-sm font-semibold uppercase tracking-[0.06em] text-white" style={{ borderBottom: '1px solid rgba(0,245,255,0.08)', paddingBottom: '0.75rem' }}>
                <span style={{ color: 'rgba(0,245,255,0.4)', marginRight: '0.25rem' }}>{'>'}</span>PRE_LAUNCH_SUMMARY
              </h2>

              <div className="space-y-2">
                <div className="flex items-center gap-2 font-mono text-[10px]">
                  <Wifi size={12} style={{ color: '#00F5FF' }} />
                  <span className="text-muted">DNS:</span>
                  <span className="text-white">{dns.enable ? `ENABLED · ${dns.mode.toUpperCase()} · ${dns.listen}` : 'DISABLED'}</span>
                  {dns.apply_on_start && dns.enable && <span style={{ color: '#00F5FF' }}>(AUTO_TAKEOVER)</span>}
                </div>
                <div className="flex items-center gap-2 font-mono text-[10px]">
                  <Globe size={12} style={{ color: '#00F5FF' }} />
                  <span className="text-muted">TPROXY:</span>
                  <span className="text-white">{net.mode === 'none' ? 'NONE' : `${net.mode.toUpperCase()} · ${net.firewall_backend.toUpperCase()}`}</span>
                  {net.apply_on_start && net.mode !== 'none' && <span style={{ color: '#00F5FF' }}>(AUTO_TAKEOVER)</span>}
                </div>
                <div className="flex items-center gap-2 font-mono text-[10px]">
                  <Sparkles size={12} style={{ color: '#00F5FF' }} />
                  <span className="text-muted">BYPASS_LAN:</span><span className="text-white">{net.bypass_lan ? 'YES' : 'NO'}</span>
                  <span className="text-muted ml-2">BYPASS_CN:</span><span className="text-white">{net.bypass_china ? 'YES' : 'NO'}</span>
                </div>
              </div>

              {conflicts.length > 0 && (
                <div className="px-4 py-4 space-y-3" style={{ background: 'rgba(255,230,0,0.06)', border: '1px solid rgba(255,230,0,0.3)' }}>
                  <div className="flex items-start gap-2">
                    <AlertCircle size={16} className="flex-shrink-0 mt-0.5" style={{ color: '#FFE600' }} />
                    <div>
                      <p className="font-mono text-sm font-semibold uppercase tracking-[0.06em]" style={{ color: '#FFE600' }}>CONFLICT_DETECTED</p>
                      <p className="font-mono text-[10px] text-muted mt-0.5 leading-5">
                        These services conflict with ClashForge ports or traffic takeover. Stop them before launch.
                      </p>
                    </div>
                  </div>
                  <div className="space-y-1.5">
                    {conflicts.map(svc => (
                      <div key={svc.name} className="flex items-center gap-2 px-3 py-2" style={{ background: 'rgba(255,230,0,0.06)', border: '1px solid rgba(255,230,0,0.2)' }}>
                        <XCircle size={13} className="flex-shrink-0" style={{ color: '#FFE600' }} />
                        <span className="font-mono text-[10px] font-medium text-white">{svc.label}</span>
                        {svc.pids && svc.pids.length > 0 && (
                          <span className="font-mono text-[10px] text-muted ml-auto">PID_{svc.pids.join('_')}</span>
                        )}
                      </div>
                    ))}
                  </div>
                  {stoppingConflicts ? (
                    <div className="flex items-center gap-2 font-mono text-[10px]" style={{ color: '#FFE600' }}>
                      <Loader2 size={13} className="animate-spin" />
                      STOPPING_CONFLICTS...
                    </div>
                  ) : (
                    <button
                      className="w-full flex items-center justify-center gap-2 py-2.5 font-mono text-[10px] uppercase tracking-[0.1em] transition-all cursor-pointer"
                      style={{ background: 'rgba(255,230,0,0.1)', border: '1px solid rgba(255,230,0,0.4)', color: '#FFE600' }}
                      onClick={handleStopConflicts}
                    >
                      <ShieldOff size={15} />
                      STOP_CONFLICTS + LAUNCH_CLASHFORGE
                    </button>
                  )}
                </div>
              )}

              {conflictStopped && conflicts.length === 0 && (
                <div className="px-4 py-2 flex items-center gap-2" style={{ background: 'rgba(0,255,136,0.06)', border: '1px solid rgba(0,255,136,0.2)' }}>
                  <CheckCircle2 size={13} className="flex-shrink-0" style={{ color: '#00FF88' }} />
                  <p className="font-mono text-[10px]" style={{ color: '#00FF88' }}>CONFLICTS_CLEARED</p>
                </div>
              )}

              {launchError && (
                <div className="px-4 py-3 flex items-start gap-2" style={{ background: 'rgba(255,34,85,0.06)', border: '1px solid rgba(255,34,85,0.2)' }}>
                  <XCircle size={15} className="flex-shrink-0 mt-0.5" style={{ color: '#FF2255' }} />
                  <div>
                    <p className="font-mono text-sm font-semibold uppercase" style={{ color: '#FF2255' }}>LAUNCH_FAILED</p>
                    <p className="font-mono text-[10px] text-muted mt-0.5">{launchError}</p>
                  </div>
                </div>
              )}

              {launchDone && (
                <div className="px-4 py-3 flex items-center gap-2" style={{ background: 'rgba(0,255,136,0.06)', border: '1px solid rgba(0,255,136,0.2)' }}>
                  <CheckCircle2 size={15} className="flex-shrink-0" style={{ color: '#00FF88' }} />
                  <p className="font-mono text-sm font-semibold" style={{ color: '#00FF88' }}>CORE_LAUNCHED — entering CONN_CHECK...</p>
                </div>
              )}

              {!launchDone && (
                <button
                  className="btn-primary w-full flex items-center justify-center gap-2 py-3"
                  onClick={handleLaunch}
                  disabled={launching || stoppingConflicts || conflicts.length > 0}
                >
                  {launching
                    ? <><Loader2 size={16} className="animate-spin" />LAUNCHING_CORE + TAKEOVER...</>
                    : <><Play size={16} />LAUNCH_CORE + APPLY_TAKEOVER</>}
                </button>
              )}
            </div>

            <div className="flex gap-3">
              <button className="btn-ghost flex-1" onClick={() => setStep('network')} disabled={launching}>← BACK</button>
              {launchDone && (
                <button className="btn-primary flex-1 flex items-center justify-center gap-2" onClick={() => setStep('check')}>
                  START_CONN_CHECK <ChevronRight size={14} />
                </button>
              )}
            </div>
          </div>
        )}

        {/* ─── Step 5: Check ──────────────────────────────────────────────── */}
        {step === 'check' && (
          <div className="space-y-4">
            <div className="glass-card px-5 py-5 space-y-4">
              <div className="flex items-center justify-between">
                <h2 className="font-mono text-sm font-semibold uppercase tracking-[0.06em] text-white">
                  <span style={{ color: 'rgba(0,245,255,0.4)', marginRight: '0.25rem' }}>{'>'}</span>EGRESS_IP / CONN_CHECK
                </h2>
                <button
                  className="btn-ghost font-mono text-[10px] uppercase tracking-[0.1em] flex items-center gap-1.5"
                  onClick={handleCheck}
                  disabled={checking}
                >
                  <RotateCw size={12} className={checking ? 'animate-spin' : ''} />
                  {checking ? 'CHECKING...' : 'RE_CHECK'}
                </button>
              </div>

              {!checkDone && !checking && (
                <button className="btn-primary w-full flex items-center justify-center gap-2" onClick={handleCheck}>
                  <Wifi size={14} />START_CHECK
                </button>
              )}

              {checking && (
                <div className="flex items-center gap-3 font-mono text-[10px] text-muted uppercase tracking-[0.12em]">
                  <Loader2 size={16} className="animate-spin" style={{ color: '#00F5FF' }} />
                  PROBING_FROM_ROUTER_AND_BROWSER...
                </div>
              )}

              {routerProbe && (
                <div className="space-y-2">
                  <p className="font-mono text-[10px] uppercase tracking-[0.2em] text-muted">ROUTER_SIDE (SERVER_PROBE)</p>
                  {routerProbe.ip_checks.reduce((acc, c, i) => {
                    const prev = i > 0 ? routerProbe.ip_checks[i - 1] : null
                    if (!prev || prev.group !== c.group) {
                      acc.push(<p key={`ipg-${c.group}`} className="font-mono text-[10px] uppercase tracking-[0.15em] text-muted mt-1">{c.group ?? 'OTHER'}_GROUP</p>)
                    }
                    acc.push(
                      <div key={i} className="flex items-start gap-2 font-mono text-xs px-3 py-2" style={c.ok
                        ? { background: 'rgba(0,255,136,0.06)', border: '1px solid rgba(0,255,136,0.2)' }
                        : { background: 'rgba(255,34,85,0.06)', border: '1px solid rgba(255,34,85,0.2)' }
                      }>
                        {c.ok
                          ? <CheckCircle2 size={13} className="flex-shrink-0 mt-0.5" style={{ color: '#00FF88' }} />
                          : <XCircle size={13} className="flex-shrink-0 mt-0.5" style={{ color: '#FF2255' }} />}
                        <div>
                          <span style={{ color: c.ok ? '#00FF88' : '#FF2255' }}>{c.provider}</span>
                          {c.ok && c.ip && <span className="ml-2 text-white">{c.ip}</span>}
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
                      acc.push(<p key={`acg-${c.group}`} className="font-mono text-[10px] uppercase tracking-[0.15em] text-muted mt-1">{c.group ?? 'OTHER'}_GROUP</p>)
                    }
                    acc.push(
                      <div key={i} className="flex items-start gap-2 font-mono text-xs px-3 py-2" style={c.ok
                        ? { background: 'rgba(0,255,136,0.06)', border: '1px solid rgba(0,255,136,0.2)' }
                        : { background: 'rgba(255,34,85,0.06)', border: '1px solid rgba(255,34,85,0.2)' }
                      }>
                        {c.ok
                          ? <CheckCircle2 size={13} className="flex-shrink-0 mt-0.5" style={{ color: '#00FF88' }} />
                          : <XCircle size={13} className="flex-shrink-0 mt-0.5" style={{ color: '#FF2255' }} />}
                        <div>
                          <span style={{ color: c.ok ? '#00FF88' : '#FF2255' }}>{c.name}</span>
                          {c.ok && c.latency_ms && <span className="ml-2 text-muted">{c.latency_ms}_ms</span>}
                          {!c.ok && c.error && <span className="ml-2 text-muted">{c.error}</span>}
                        </div>
                      </div>
                    )
                    return acc
                  }, [] as React.ReactNode[])}
                </div>
              )}

              {browserProbe && (
                <div className="space-y-2">
                  <p className="font-mono text-[10px] uppercase tracking-[0.2em] text-muted">BROWSER_SIDE (CLIENT_PROBE)</p>
                  <div className="flex items-start gap-2 font-mono text-xs px-3 py-2" style={browserProbe.ipOK
                    ? { background: 'rgba(0,255,136,0.06)', border: '1px solid rgba(0,255,136,0.2)' }
                    : { background: 'rgba(255,34,85,0.06)', border: '1px solid rgba(255,34,85,0.2)' }
                  }>
                    {browserProbe.ipOK
                      ? <CheckCircle2 size={13} className="flex-shrink-0 mt-0.5" style={{ color: '#00FF88' }} />
                      : <XCircle size={13} className="flex-shrink-0 mt-0.5" style={{ color: '#FF2255' }} />}
                    <div>
                      <span style={{ color: browserProbe.ipOK ? '#00FF88' : '#FF2255' }}>EGRESS_IP (IP.SB)</span>
                      {browserProbe.ipOK && browserProbe.ip && <span className="ml-2 text-white">{browserProbe.ip}</span>}
                      {!browserProbe.ipOK && browserProbe.ipError && <span className="ml-2 text-muted">{browserProbe.ipError}</span>}
                    </div>
                  </div>
                  {browserProbe.accessChecks.reduce((acc, c, i) => {
                    const prev = i > 0 ? browserProbe.accessChecks[i - 1] : null
                    if (!prev || prev.group !== c.group) {
                      acc.push(<p key={`bg-${c.group}`} className="font-mono text-[10px] uppercase tracking-[0.15em] text-muted mt-1">{c.group ?? 'OTHER'}_GROUP</p>)
                    }
                    acc.push(
                      <div key={i} className="flex items-start gap-2 font-mono text-xs px-3 py-2" style={c.ok
                        ? { background: 'rgba(0,255,136,0.06)', border: '1px solid rgba(0,255,136,0.2)' }
                        : { background: 'rgba(255,34,85,0.06)', border: '1px solid rgba(255,34,85,0.2)' }
                      }>
                        {c.ok
                          ? <CheckCircle2 size={13} className="flex-shrink-0 mt-0.5" style={{ color: '#00FF88' }} />
                          : <XCircle size={13} className="flex-shrink-0 mt-0.5" style={{ color: '#FF2255' }} />}
                        <div>
                          <span style={{ color: c.ok ? '#00FF88' : '#FF2255' }}>{c.name}</span>
                          {c.ok && c.latency_ms && <span className="ml-2 text-muted">{c.latency_ms}_ms</span>}
                          {!c.ok && c.error && <span className="ml-2 text-muted">{c.error}</span>}
                        </div>
                      </div>
                    )
                    return acc
                  }, [] as React.ReactNode[])}
                </div>
              )}

              {checkDone && overallOK && (
                <div className="space-y-3">
                  <div className="px-5 py-4 flex items-start gap-3" style={{ background: 'rgba(0,255,136,0.06)', border: '1px solid rgba(0,255,136,0.2)' }}>
                    <CheckCircle2 size={20} className="flex-shrink-0 mt-0.5" style={{ color: '#00FF88' }} />
                    <div>
                      <p className="font-mono text-sm font-bold uppercase tracking-[0.06em]" style={{ color: '#00FF88' }}>ALL_CHECKS_PASSED</p>
                      <p className="font-mono text-[10px] text-muted mt-1">Proxy operational — router and browser both reach external network.</p>
                    </div>
                  </div>
                  <div className="glass-card px-5 py-4 space-y-4">
                    <h3 className="font-mono text-sm font-semibold uppercase tracking-[0.06em] text-white">
                      <span style={{ color: 'rgba(0,245,255,0.4)', marginRight: '0.25rem' }}>{'>'}</span>COMPLETE_SETUP
                    </h3>
                    <Field label="AUTO_START_ON_BOOT" hint="Auto-start ClashForge and Mihomo core after router reboot">
                      <Toggle checked={autoStartCore} onChange={setAutoStartCore} label={autoStartCore ? 'ENABLED' : 'DISABLED'} />
                    </Field>
                    {saveError && (
                      <div className="flex items-center gap-2 font-mono text-[10px]" style={{ color: '#FF2255' }}>
                        <AlertCircle size={13} />{saveError}
                      </div>
                    )}
                    <button
                      className="btn-primary w-full flex items-center justify-center gap-2 py-3"
                      onClick={handleComplete}
                      disabled={saving}
                    >
                      {saving
                        ? <><Loader2 size={16} className="animate-spin" />SAVING...</>
                        : <><ArrowRight size={16} />COMPLETE — ENTER_OVERVIEW</>}
                    </button>
                  </div>
                </div>
              )}

              {checkDone && !overallOK && (
                <div className="space-y-3">
                  <div className="px-4 py-3 flex items-start gap-2" style={{ background: 'rgba(255,34,85,0.06)', border: '1px solid rgba(255,34,85,0.2)' }}>
                    <AlertCircle size={15} className="flex-shrink-0 mt-0.5" style={{ color: '#FF2255' }} />
                    <div>
                      <p className="font-mono text-sm font-semibold uppercase" style={{ color: '#FF2255' }}>CHECKS_FAILED</p>
                      <p className="font-mono text-[10px] text-muted mt-1">Review logs below or go back to reconfigure.</p>
                    </div>
                  </div>

                  {probeLogs.length > 0 && (
                    <div className="glass-card px-4 py-4 space-y-2">
                      <p className="font-mono text-[10px] uppercase tracking-[0.2em] text-muted">CLASHFORGE_RECENT_LOGS</p>
                      <div className="max-h-64 overflow-y-auto space-y-1">
                        {probeLogs.map((l, i) => {
                          const level = (l as unknown as Record<string,string>).level
                          const ts = (l as unknown as Record<string,string>).ts
                          const msg = (l as unknown as Record<string,string>).msg
                          const color = level === 'error' ? '#FF2255' : level === 'warn' ? '#FFE600' : '#4A6080'
                          return (
                            <div key={i} className="font-mono text-[10px] px-2 py-1" style={{ color, background: level === 'error' ? 'rgba(255,34,85,0.04)' : level === 'warn' ? 'rgba(255,230,0,0.04)' : 'transparent' }}>
                              {ts ? new Date(Number(ts) * 1000).toLocaleTimeString() : ''}
                              {' '}<span className="font-bold">[{level?.toUpperCase()}]</span>
                              {' '}{msg}
                            </div>
                          )
                        })}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>

            <div className="flex gap-3">
              <button className="btn-ghost flex-1" onClick={() => setStep('launch')}>← BACK</button>
              <button className="btn-ghost flex-1" onClick={() => navigate('/')}>
                SKIP — ENTER_OVERVIEW
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
