import { useCallback, useEffect, useRef, useState } from 'react'
import {
  AlertCircle,
  CheckCircle2,
  ChevronRight,
  Cloud,
  Globe,
  Key,
  Loader2,
  Lock,
  RefreshCw,
  Rocket,
  Server,
  ShieldCheck,
  Sparkles,
  Terminal,
  Wifi,
  XCircle,
} from 'lucide-react'
import {
  getConfig,
  getActiveSource,
  getNodeProxyConfig,
  getNodes,
  probeNode,
  quickStartCheckNode,
  quickStartValidateCF,
  quickStartValidateVPS,
  QUICKSTART_DEPLOY_URL,
  type ActiveSource,
  type NodeListItem,
  type NodeProbeResult,
  type QSCheckNodeResult,
  type QSDeployRequest,
  type QSDeployType,
  type QSEvent,
  type QSCFZone,
  type QSValidateCFResult,
  type QSValidateVPSResult,
} from '../api/client'
import { WorkflowDiagram, DNSModeDiagram } from '../components/ModeDiagrams'

interface LaunchEvent {
  type: 'step' | 'info' | 'done'
  step?: string
  status?: 'running' | 'ok' | 'error' | 'skip' | 'info'
  message: string
  detail?: string
  success?: boolean
  error?: string
}
import { CFConfigBanner, CFConfigModal, useCFConfig } from '../components/CFConfig'

// ── types ──────────────────────────────────────────────────────────────────────

type WizardStep =
  | 'select'      // Step 0: choose VPS or CF Workers
  | 'vps_connect' // Path B Step 1: SSH credentials
  | 'cf_creds'    // Step: CF token + zone select (both paths)
  | 'vps_config'  // Path B only: node prefix / node name
  | 'confirm'     // Both: review & deploy
  | 'deploying'   // SSE stream
  | 'done'        // Success or failure

interface SSHForm {
  host: string
  port: string
  user: string
  auth_type: 'password' | 'key'
  password: string
  priv_key: string
}

interface CFForm {
  token: string
  account_id: string
}

interface VPSConfigForm {
  node_prefix: string
  node_name: string
}

// ── helpers ───────────────────────────────────────────────────────────────────

function phaseLabel(phase: string): string {
  const labels: Record<string, string> = {
    cf_validate:     'CF 验证',
    worker_deploy:   'Worker 部署',
    publish:         '订阅发布',
    existing_check:  '既有检测',
    ssh_test:        'SSH 连接',
    env_detect:      '环境检测',
    provision:       '服务部署',
    cert_dns:        'DNS & 证书',
    import:          '订阅导入',
    configure:       '自动配置',
    verify:          '连通验证',
  }
  return labels[phase] ?? phase
}

// ── sub-components ─────────────────────────────────────────────────────────────

function FieldRow({
  label, children, hint,
}: {
  label: string
  children: React.ReactNode
  hint?: string
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <label className="text-[12px] font-semibold uppercase tracking-[0.12em] text-white/45">
        {label}
      </label>
      {children}
      {hint && <p className="text-[11px] text-white/30">{hint}</p>}
    </div>
  )
}

function Input({
  value, onChange, placeholder, type = 'text', disabled, mono,
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  type?: string
  disabled?: boolean
  mono?: boolean
}) {
  return (
    <input
      type={type}
      value={value}
      onChange={e => onChange(e.target.value)}
      placeholder={placeholder}
      disabled={disabled}
      className={[
        'w-full rounded-lg border border-white/[0.08] bg-white/[0.04] px-3 py-2.5 text-sm text-white',
        'placeholder:text-white/20 focus:border-brand/40 focus:outline-none focus:ring-1 focus:ring-brand/25',
        'disabled:opacity-40',
        mono ? 'font-mono' : '',
      ].join(' ')}
    />
  )
}

function Textarea({
  value, onChange, placeholder, rows = 5, mono,
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  rows?: number
  mono?: boolean
}) {
  return (
    <textarea
      value={value}
      onChange={e => onChange(e.target.value)}
      placeholder={placeholder}
      rows={rows}
      className={[
        'w-full resize-none rounded-lg border border-white/[0.08] bg-white/[0.04] px-3 py-2.5 text-sm text-white',
        'placeholder:text-white/20 focus:border-brand/40 focus:outline-none focus:ring-1 focus:ring-brand/25',
        mono ? 'font-mono text-xs leading-relaxed' : '',
      ].join(' ')}
    />
  )
}

function StepDot({ active, done }: { active: boolean; done: boolean }) {
  if (done) return <span className="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full bg-emerald-500/20 text-emerald-400"><CheckCircle2 size={12} /></span>
  if (active) return <span className="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full bg-brand/20 text-brand-light"><span className="h-1.5 w-1.5 rounded-full bg-brand-light" /></span>
  return <span className="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full bg-white/[0.06]"><span className="h-1.5 w-1.5 rounded-full bg-white/20" /></span>
}

// ── event log ─────────────────────────────────────────────────────────────────

interface EventRowProps { event: QSEvent; isLast: boolean }
function EventRow({ event, isLast }: EventRowProps) {
  const icons: Record<string, React.ReactNode> = {
    running: <Loader2 size={13} className="animate-spin text-brand-light" />,
    ok:      <CheckCircle2 size={13} className="text-emerald-400" />,
    error:   <XCircle size={13} className="text-rose-400" />,
    warning: <AlertCircle size={13} className="text-amber-400" />,
    info:    <span className="h-1.5 w-1.5 rounded-full bg-white/30 mx-0.5" />,
  }
  const textColor: Record<string, string> = {
    running: 'text-white/80',
    ok:      'text-white/70',
    error:   'text-rose-300',
    warning: 'text-amber-300',
    info:    'text-white/40',
  }

  return (
    <div className={['flex gap-2.5 py-1.5', isLast ? '' : 'border-b border-white/[0.04]'].join(' ')}>
      <div className="mt-px flex-shrink-0">{icons[event.status] ?? icons.info}</div>
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2">
          <span className="text-[10px] font-semibold uppercase tracking-wider text-white/25">
            {phaseLabel(event.phase)}
          </span>
          <span className={['text-[13px]', textColor[event.status] ?? 'text-white/60'].join(' ')}>
            {event.message}
          </span>
        </div>
        {event.detail && (
          <p className="mt-0.5 font-mono text-[11px] text-white/30">{event.detail}</p>
        )}
      </div>
    </div>
  )
}

// ── main component ─────────────────────────────────────────────────────────────

export function QuickStart() {
  const [step, setStep] = useState<WizardStep>('select')
  const [deployType, setDeployType] = useState<QSDeployType>('vps')

  // Shared CF config (persisted on the router at /etc/metaclash/cf-config.json)
  const { config: cfGlobal, loading: cfGlobalLoading, save: saveCFGlobal } = useCFConfig()
  const [showCFModal, setShowCFModal] = useState(false)

  // SSH form (VPS only)
  const [ssh, setSSH] = useState<SSHForm>({
    host: '', port: '22', user: 'root', auth_type: 'password', password: '', priv_key: '',
  })
  const [sshValidating, setSSHValidating] = useState(false)
  const [sshResult, setSSHResult] = useState<QSValidateVPSResult | null>(null)
  const [sshError, setSSHError] = useState('')
  const [managedNodes, setManagedNodes] = useState<NodeListItem[]>([])
  const [managedNodesLoading, setManagedNodesLoading] = useState(false)
  const [selectedNodeId, setSelectedNodeId] = useState('')
  const [selectedNodeProxyYAML, setSelectedNodeProxyYAML] = useState('')
  const [selectedNodeProxyLoading, setSelectedNodeProxyLoading] = useState(false)
  const [selectedNodeProxyError, setSelectedNodeProxyError] = useState('')
  const [probeResults, setProbeResults] = useState<NodeProbeResult[] | null>(null)
  const [probeLoading, setProbeLoading] = useState(false)

  // CF form — pre-filled from stored global config
  const [cf, setCF] = useState<CFForm>({ token: '', account_id: '' })
  const [cfValidating, setCFValidating] = useState(false)
  const [cfResult, setCFResult] = useState<QSValidateCFResult | null>(null)
  const [cfError, setCFError] = useState('')
  const [selectedZoneId, setSelectedZoneId] = useState('')

  // Pre-fill CF form when global config loads (async)
  useEffect(() => {
    if (!cfGlobal?.cf_token) return
    setCF(prev => ({
      token: prev.token || cfGlobal.cf_token,
      account_id: prev.account_id || cfGlobal.cf_account_id,
    }))
  }, [cfGlobal?.cf_token]) // eslint-disable-line react-hooks/exhaustive-deps

  // Load managed nodes so QuickStart can reuse saved SSH credentials.
  const loadManagedNodes = useCallback(() => {
    setManagedNodesLoading(true)
    getNodes()
      .then((res) => {
        const items = (res.nodes ?? []).filter(n => n.kind !== 'external')
        setManagedNodes(items)
      })
      .catch(() => setManagedNodes([]))
      .finally(() => setManagedNodesLoading(false))
  }, [])

  useEffect(() => { loadManagedNodes() }, [loadManagedNodes])

  useEffect(() => {
    setProbeResults(null); setProbeLoading(false)
    if (!selectedNodeId) return
    const node = managedNodes.find(n => n.id === selectedNodeId)
    if (!node) return
    setSSH({
      host: node.host,
      port: String(node.port || 22),
      user: node.username || 'root',
      auth_type: 'password',
      password: '',
      priv_key: '',
    })
    setSSHResult(null)
    setSSHError('')
  }, [selectedNodeId, managedNodes])

  useEffect(() => {
    let cancelled = false
    setSelectedNodeProxyYAML('')
    setSelectedNodeProxyError('')
    if (!selectedNodeId) return () => { cancelled = true }
    setSelectedNodeProxyLoading(true)
    getNodeProxyConfig(selectedNodeId)
      .then((yaml) => {
        if (!cancelled) setSelectedNodeProxyYAML(yaml)
      })
      .catch((e) => {
        if (!cancelled) setSelectedNodeProxyError(e instanceof Error ? e.message : '读取代理配置失败')
      })
      .finally(() => {
        if (!cancelled) setSelectedNodeProxyLoading(false)
      })
    return () => { cancelled = true }
  }, [selectedNodeId])

  // VPS config
  const [vpsConfig, setVPSConfig] = useState<VPSConfigForm>({ node_prefix: 'node', node_name: '' })
  const [vpsNameEdited, setVPSNameEdited] = useState(false)

  // Node existence check (vps_config step)
  const [nodeCheck, setNodeCheck] = useState<QSCheckNodeResult | null>(null)
  const [nodeCheckLoading, setNodeCheckLoading] = useState(false)
  const [nodeCheckError, setNodeCheckError] = useState('')

  // Deploying state
  const [events, setEvents] = useState<QSEvent[]>([])
  const [deployError, setDeployError] = useState('')
  const [deployDone, setDeployDone] = useState(false)
  const [, setDeployId] = useState('')
  const eventsEndRef = useRef<HTMLDivElement>(null)

  // Auto-scroll event log
  useEffect(() => {
    if (eventsEndRef.current) {
      eventsEndRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [events])

  // ── quick-launch: one-click restart with last saved config ─────────────────
  const [qlCfg, setQlCfg] = useState<Record<string, unknown> | null>(null)
  const [qlActive, setQlActive] = useState<ActiveSource | null>(null)
  const [qlLoading, setQlLoading] = useState(true)
  const [qlState, setQlState] = useState<'idle' | 'launching' | 'done' | 'error'>('idle')
  const [qlEvents, setQlEvents] = useState<LaunchEvent[]>([])
  const [qlError, setQlError] = useState('')
  const qlEndRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    Promise.all([getConfig(), getActiveSource()])
      .then(([cfg, as_]) => {
        setQlCfg(cfg as Record<string, unknown>)
        setQlActive(as_.active_source)
      })
      .catch(() => {})
      .finally(() => setQlLoading(false))
  }, [])

  useEffect(() => {
    if (qlEndRef.current) qlEndRef.current.scrollIntoView({ behavior: 'smooth' })
  }, [qlEvents])

  const buildQlPayload = useCallback(() => {
    if (!qlCfg) return null
    const dns = (qlCfg.dns ?? {}) as Record<string, unknown>
    const network = (qlCfg.network ?? {}) as Record<string, unknown>
    const ports = (qlCfg.ports ?? {}) as Record<string, unknown>
    return {
      dns: {
        enable: dns.enable ?? true,
        mode: dns.mode ?? 'fake-ip',
        dnsmasq_mode: dns.dnsmasq_mode ?? 'none',
        apply_on_start: dns.apply_on_start ?? true,
        listen: `0.0.0.0:${(ports.dns as number | undefined) ?? 17874}`,
        ipv6: dns.ipv6 ?? false,
        strategy: dns.strategy ?? 'split',
        nameservers: dns.nameservers ?? ['223.5.5.5', '119.29.29.29'],
        fallback: dns.fallback ?? ['tls://8.8.4.4', 'tls://1.1.1.1', 'https://dns.google/dns-query', 'https://cloudflare-dns.com/dns-query'],
        doh: dns.doh ?? [],
        fake_ip_filter: dns.fake_ip_filter ?? [],
      },
      network: {
        mode: network.mode ?? 'tproxy',
        firewall_backend: network.firewall_backend ?? 'auto',
        bypass_lan: network.bypass_lan ?? true,
        bypass_china: network.bypass_china ?? true,
        apply_on_start: network.apply_on_start ?? true,
        ipv6: network.ipv6 ?? false,
      },
    }
  }, [qlCfg])

  const handleQuickLaunch = useCallback(async () => {
    const payload = buildQlPayload()
    if (!payload) return
    setQlState('launching')
    setQlEvents([])
    setQlError('')
    const secret = localStorage.getItem('cf_secret') || ''
    try {
      const res = await fetch('/api/v1/setup/launch', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(secret ? { Authorization: `Bearer ${secret}` } : {}),
        },
        body: JSON.stringify(payload),
      })
      if (!res.ok) {
        const text = await res.text()
        setQlError(`HTTP ${res.status}: ${text}`)
        setQlState('error')
        return
      }
      const reader = res.body?.getReader()
      if (!reader) { setQlError('SSE 流不可用'); setQlState('error'); return }
      const decoder = new TextDecoder()
      let buf = ''
      let streamDone = false
      while (!streamDone) {
        const chunk = await reader.read()
        streamDone = chunk.done
        if (chunk.value) buf += decoder.decode(chunk.value, { stream: true })
        const lines = buf.split('\n')
        buf = lines.pop() ?? ''
        for (const raw of lines) {
          const line = raw.trim()
          if (!line.startsWith('data:')) continue
          const dataStr = line.slice(5).trim()
          if (!dataStr || dataStr === '[DONE]') continue
          try {
            const evt = JSON.parse(dataStr) as LaunchEvent
            if (evt.type === 'done') {
              setQlState(evt.success ? 'done' : 'error')
              if (!evt.success) setQlError(evt.error ?? '启动失败')
            } else {
              setQlEvents(prev => [...prev, evt])
            }
          } catch { /* ignore parse errors */ }
        }
      }
      setQlState(prev => prev === 'launching' ? 'done' : prev)
    } catch (e) {
      setQlError(e instanceof Error ? e.message : '请求失败')
      setQlState('error')
    }
  }, [buildQlPayload])

  const resetQl = useCallback(() => {
    setQlState('idle')
    setQlEvents([])
    setQlError('')
  }, [])

  // ── CF validation ──────────────────────────────────────────────────────────
  const validateCF = useCallback(async () => {
    if (!cf.token.trim()) { setCFError('请输入 CF API Token'); return }
    setCFValidating(true); setCFError(''); setCFResult(null)
    try {
      const res = await quickStartValidateCF({ token: cf.token.trim(), account_id: cf.account_id.trim() || undefined })
      setCFResult(res)
      if (!res.valid) setCFError(res.error ?? '验证失败')
      else if (res.zones?.length === 1) {
        setSelectedZoneId(res.zones[0].id)
      }
    } catch (e) {
      setCFError(e instanceof Error ? e.message : '请求失败')
    } finally { setCFValidating(false) }
  }, [cf.token, cf.account_id])

  // ── SSH validation ──────────────────────────────────────────────────────────
  const validateSSH = useCallback(async () => {
    if (!selectedNodeId && !ssh.host.trim()) { setSSHError('请输入服务器地址'); return }
    setSSHValidating(true); setSSHError(''); setSSHResult(null)
    try {
      const req = selectedNodeId
        ? { node_id: selectedNodeId, auth_type: 'password' as const }
        : {
            host: ssh.host.trim(),
            port: parseInt(ssh.port) || 22,
            user: ssh.user.trim() || 'root',
            auth_type: ssh.auth_type,
            password: ssh.auth_type === 'password' ? ssh.password : undefined,
            priv_key: ssh.auth_type === 'key' ? ssh.priv_key.trim() : undefined,
          }
      const res = await quickStartValidateVPS(req)
      setSSHResult(res)
      if (!res.valid) {
        setSSHError(res.error ?? '连接失败')
      } else if (res.node_id && !selectedNodeId) {
        // Backend saved the credentials to /nodes — switch to the saved node so
        // future deploy requests reference it by ID instead of raw credentials.
        setSelectedNodeId(res.node_id)
        loadManagedNodes()
      }
    } catch (e) {
      setSSHError(e instanceof Error ? e.message : '请求失败')
    } finally { setSSHValidating(false) }
  }, [ssh, selectedNodeId, loadManagedNodes])

  // ── deploy ──────────────────────────────────────────────────────────────────
  const startDeploy = useCallback(async (overrideReq?: QSDeployRequest) => {
    setStep('deploying')
    setEvents([])
    setDeployError('')
    setDeployDone(false)
    setDeployId('')

    let req: QSDeployRequest
    if (overrideReq) {
      req = overrideReq
    } else {
    const selectedZone = cfResult?.zones?.find(z => z.id === selectedZoneId)

    req = {
      deploy_type: deployType,
      cloudflare: {
        token: cf.token.trim(),
        account_id: cf.account_id.trim(),
        zone_id: selectedZoneId,
        zone_name: selectedZone?.name ?? '',
      },
    }

    if (deployType === 'cf_workers') {
      // node_name left unset — backend auto-generates readable random subdomain
    } else {
      req.node_prefix = vpsConfig.node_prefix.trim()
      req.node_name = vpsConfig.node_name.trim() || undefined
      if (selectedNodeId) {
        req.vps_node_id = selectedNodeId
      } else {
        req.vps = {
          host: ssh.host.trim(),
          port: parseInt(ssh.port) || 22,
          user: ssh.user.trim() || 'root',
          auth_type: ssh.auth_type,
          password: ssh.auth_type === 'password' ? ssh.password : undefined,
          priv_key: ssh.auth_type === 'key' ? ssh.priv_key.trim() : undefined,
        }
      }
    }
    } // end overrideReq else

    const secret = localStorage.getItem('cf_secret') || ''
    try {
      const response = await fetch(QUICKSTART_DEPLOY_URL(), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(secret ? { Authorization: `Bearer ${secret}` } : {}),
        },
        body: JSON.stringify(req),
      })

      if (!response.ok) {
        const text = await response.text()
        setDeployError(`HTTP ${response.status}: ${text}`)
        setDeployDone(true)
        return
      }

      const reader = response.body?.getReader()
      if (!reader) { setDeployError('SSE 流不可用'); setDeployDone(true); return }

      const decoder = new TextDecoder()
      let buf = ''
      let currentEventType = ''

      while (true) {
        const { value, done } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        const parts = buf.split('\n\n')
        buf = parts.pop() ?? ''

        for (const part of parts) {
          const lines = part.split('\n')
          let eventType = currentEventType
          let dataLine = ''
          for (const line of lines) {
            if (line.startsWith('event:')) eventType = line.slice(6).trim()
            else if (line.startsWith('data:')) dataLine = line.slice(5).trim()
          }
          currentEventType = eventType

          if (!dataLine) continue
          try {
            const parsed = JSON.parse(dataLine)
            if (eventType === 'deploy_id') {
              setDeployId(parsed.id ?? '')
            } else if (eventType === 'event') {
              setEvents(prev => [...prev, parsed as QSEvent])
            } else if (eventType === 'done') {
              setDeployDone(true)
              if (parsed.status === 'failed') setDeployError(parsed.error ?? '部署失败')
            }
          } catch { /* ignore parse errors */ }
        }
      }
    } catch (e) {
      setDeployError(e instanceof Error ? e.message : '部署请求失败')
    } finally {
      setDeployDone(true)
    }
  }, [deployType, cf, selectedZoneId, cfResult, vpsConfig, ssh, selectedNodeId])

  // ── zone auto-select on CF validate ───────────────────────────────────────
  const selectedZone: QSCFZone | undefined = cfResult?.zones?.find(z => z.id === selectedZoneId)
  const selectedManagedNode = managedNodes.find(n => n.id === selectedNodeId)
  const usingManagedNode = selectedNodeId !== ''
  const selectedNodeHasAuth = /(^|\n)\s*username:\s*.+/m.test(selectedNodeProxyYAML)
    && /(^|\n)\s*password:\s*.+/m.test(selectedNodeProxyYAML)

  const handleZoneSelect = (id: string) => {
    setSelectedZoneId(id)
  }

  // ── auto-validate on cf_creds step whenever a token is available ─────────
  // Fires when: (a) user navigates to cf_creds with token already pre-filled,
  //             (b) cfGlobal loads AFTER user arrived at cf_creds (token changes from '' to value).
  useEffect(() => {
    if (step !== 'cf_creds') return
    if (cfResult?.valid) return     // already validated
    if (!cf.token.trim()) return    // no token yet
    void validateCF()
  }, [step, cf.token]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── step navigation helpers ────────────────────────────────────────────────
  const stepOrder: WizardStep[] =
    deployType === 'cf_workers'
      ? ['select', 'cf_creds', 'confirm', 'deploying', 'done']
      : ['select', 'vps_connect', 'cf_creds', 'vps_config', 'confirm', 'deploying', 'done']

  const stepIdx = stepOrder.indexOf(step)
  const prevStep = () => { if (stepIdx > 0) setStep(stepOrder[stepIdx - 1]) }
  const nextStep = () => { if (stepIdx < stepOrder.length - 1) setStep(stepOrder[stepIdx + 1]) }

  // ── reset ──────────────────────────────────────────────────────────────────
  const reset = () => {
    setStep('select')
    setSSH({ host: '', port: '22', user: 'root', auth_type: 'password', password: '', priv_key: '' })
    setSSHResult(null); setSSHError('')
    setSelectedNodeId(''); setProbeResults(null); setProbeLoading(false)
    setSelectedNodeProxyYAML(''); setSelectedNodeProxyError(''); setSelectedNodeProxyLoading(false)
    setCF({ token: '', account_id: '' }); setCFResult(null); setCFError(''); setSelectedZoneId('')
    setVPSConfig({ node_prefix: 'node', node_name: '' }); setVPSNameEdited(false)
    setNodeCheck(null); setNodeCheckLoading(false); setNodeCheckError('')
    setEvents([]); setDeployError(''); setDeployDone(false); setDeployId('')
    setShowCFModal(false)
  }

  // ── render ─────────────────────────────────────────────────────────────────
  return (
    <div className="mx-auto max-w-2xl space-y-6 pb-12">
      {/* Page header */}
      <div>
        <div className="flex items-center gap-2 mb-1">
          <Sparkles size={16} className="text-brand-light" />
          <span className="text-[11px] font-semibold uppercase tracking-[0.18em] text-brand-light/70">
            Quick Start
          </span>
        </div>
        <h1 className="font-heading text-2xl font-bold text-white">快速启动向导</h1>
        <p className="mt-1.5 text-sm text-white/40">
          一键重启上次运行配置，或按步骤部署新的出口节点。
        </p>
      </div>

      {/* Quick-launch: restart last saved config */}
      <QLSection
        qlCfg={qlCfg}
        qlActive={qlActive}
        qlLoading={qlLoading}
        qlState={qlState}
        qlEvents={qlEvents}
        qlError={qlError}
        qlEndRef={qlEndRef}
        onLaunch={() => void handleQuickLaunch()}
        onReset={resetQl}
      />

      {/* Divider before node deployment section */}
      <div className="flex items-center gap-3">
        <div className="h-px flex-1 bg-white/[0.06]" />
        <span className="text-[11px] font-semibold uppercase tracking-[0.18em] text-white/25">部署出口节点</span>
        <div className="h-px flex-1 bg-white/[0.06]" />
      </div>

      {/* Progress indicator */}
      {step !== 'select' && (
        <div className="flex items-center gap-2 overflow-x-auto pb-1">
          {stepOrder.slice(1, -2).map((s, i) => {
            const idx = stepOrder.indexOf(s)
            return (
              <div key={s} className="flex items-center gap-2">
                {i > 0 && <div className="h-px w-6 flex-shrink-0 bg-white/[0.08]" />}
                <div className="flex flex-shrink-0 items-center gap-1.5">
                  <StepDot active={step === s} done={stepIdx > idx} />
                  <span className={[
                    'text-[11px] font-medium',
                    step === s ? 'text-white/80' : stepIdx > idx ? 'text-white/35' : 'text-white/22',
                  ].join(' ')}>
                    {stepLabel(s)}
                  </span>
                </div>
              </div>
            )
          })}
        </div>
      )}

      {/* ── Step: Select deploy type ── */}
      {step === 'select' && (
        <div className="space-y-4">
          <p className="text-sm text-white/50">选择你的出口节点类型：</p>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {/* VPS option */}
            <button
              onClick={() => { setDeployType('vps'); setStep('vps_connect') }}
              className="group relative flex flex-col gap-3 rounded-xl border border-white/[0.08] bg-white/[0.03] p-5 text-left transition-all hover:border-brand/30 hover:bg-brand/[0.04]"
            >
              <div className="flex items-center justify-between">
                <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand/10 text-brand-light">
                  <Server size={18} />
                </div>
                <span className="rounded-full bg-brand/15 px-2.5 py-1 text-[10px] font-bold uppercase tracking-wider text-brand-light">
                  推荐
                </span>
              </div>
              <div>
                <p className="text-[15px] font-semibold text-white">VPS + gost</p>
                <p className="mt-1 text-[12px] text-white/40">
                  自建 HTTPS+TLS 节点，兼容全部服务，适合长期使用。
                </p>
              </div>
              <ul className="space-y-1">
                {['全协议兼容，不受 CF 封锁影响', 'Let\'s Encrypt 证书自动申请', 'gost v3 服务自动安装启动'].map(t => (
                  <li key={t} className="flex items-start gap-1.5 text-[11.5px] text-white/45">
                    <CheckCircle2 size={11} className="mt-0.5 flex-shrink-0 text-emerald-400/70" />
                    {t}
                  </li>
                ))}
              </ul>
              <div className="flex items-center gap-1 text-[11px] font-medium text-white/30">
                需要: VPS + Cloudflare 账号 <ChevronRight size={11} className="group-hover:translate-x-0.5 transition-transform" />
              </div>
            </button>

            {/* CF Workers option */}
            <button
              onClick={() => { setDeployType('cf_workers'); setStep('cf_creds') }}
              className="group relative flex flex-col gap-3 rounded-xl border border-white/[0.08] bg-white/[0.03] p-5 text-left transition-all hover:border-amber-500/25 hover:bg-amber-500/[0.03]"
            >
              <div className="flex items-center justify-between">
                <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-amber-500/10 text-amber-300">
                  <Cloud size={18} />
                </div>
                <span className="rounded-full bg-amber-500/12 px-2.5 py-1 text-[10px] font-bold uppercase tracking-wider text-amber-400/80">
                  应急
                </span>
              </div>
              <div>
                <p className="text-[15px] font-semibold text-white">Cloudflare Workers</p>
                <p className="mt-1 text-[12px] text-white/40">
                  无需 VPS，快速部署。许多服务拒绝 CF 出口，仅建议临时使用。
                </p>
              </div>
              <ul className="space-y-1">
                {['免费额度，无需 VPS', '自动绑定自定义域名', '适合访问 Google 等基本需求'].map(t => (
                  <li key={t} className="flex items-start gap-1.5 text-[11.5px] text-white/45">
                    <CheckCircle2 size={11} className="mt-0.5 flex-shrink-0 text-emerald-400/70" />
                    {t}
                  </li>
                ))}
                <li className="flex items-start gap-1.5 text-[11.5px] text-amber-400/60">
                  <AlertCircle size={11} className="mt-0.5 flex-shrink-0" />
                  Netflix/Spotify 等平台封锁 CF 出口
                </li>
              </ul>
              <div className="flex items-center gap-1 text-[11px] font-medium text-white/30">
                需要: Cloudflare 账号 + 自有域名 <ChevronRight size={11} className="group-hover:translate-x-0.5 transition-transform" />
              </div>
            </button>
          </div>
        </div>
      )}

      {/* ── Step: VPS SSH connect ── */}
      {step === 'vps_connect' && (
        <Card title="连接 VPS" icon={<Terminal size={16} />}>
          <div className="space-y-4">
            {managedNodes.length > 0 && (
              <FieldRow label="托管节点（可选）" hint="选择后复用托管节点已保存的用户名/密码；留空则手动输入新凭据。">
                <select
                  value={selectedNodeId}
                  disabled={managedNodesLoading}
                  onChange={e => setSelectedNodeId(e.target.value)}
                  className="w-full rounded-lg border border-white/[0.08] bg-white/[0.04] px-3 py-2.5 text-sm text-white focus:border-brand/40 focus:outline-none focus:ring-1 focus:ring-brand/25"
                >
                  <option value="" className="bg-slate-900 text-white">
                    {managedNodesLoading ? '读取托管节点中...' : '手动输入新的 VPS 凭据'}
                  </option>
                  {managedNodes.map((node) => (
                    <option key={node.id} value={node.id} className="bg-slate-900 text-white">
                      {node.name} · {node.username}@{node.host}:{node.port}
                    </option>
                  ))}
                </select>
              </FieldRow>
            )}

            {!usingManagedNode ? (
              <>
                <div className="grid grid-cols-3 gap-3">
                  <div className="col-span-2">
                    <FieldRow label="服务器 IP / 域名">
                      <Input value={ssh.host} onChange={v => setSSH(s => ({ ...s, host: v }))} placeholder="1.2.3.4" />
                    </FieldRow>
                  </div>
                  <FieldRow label="SSH 端口">
                    <Input value={ssh.port} onChange={v => setSSH(s => ({ ...s, port: v }))} placeholder="22" />
                  </FieldRow>
                </div>

                <FieldRow label="用户名">
                  <Input value={ssh.user} onChange={v => setSSH(s => ({ ...s, user: v }))} placeholder="root" />
                </FieldRow>

                {/* Auth type toggle */}
                <div>
                  <label className="text-[12px] font-semibold uppercase tracking-[0.12em] text-white/45">认证方式</label>
                  <div className="mt-1.5 flex rounded-lg border border-white/[0.08] bg-white/[0.03] p-0.5">
                    {(['password', 'key'] as const).map(t => (
                      <button
                        key={t}
                        onClick={() => setSSH(s => ({ ...s, auth_type: t }))}
                        className={[
                          'flex flex-1 items-center justify-center gap-1.5 rounded-md py-1.5 text-[12px] font-medium transition-colors',
                          ssh.auth_type === t
                            ? 'bg-white/[0.07] text-white'
                            : 'text-white/35 hover:text-white/55',
                        ].join(' ')}
                      >
                        {t === 'password' ? <Lock size={11} /> : <Key size={11} />}
                        {t === 'password' ? '密码' : '私钥'}
                      </button>
                    ))}
                  </div>
                </div>

                {ssh.auth_type === 'password' ? (
                  <FieldRow label="密码">
                    <Input type="password" value={ssh.password} onChange={v => setSSH(s => ({ ...s, password: v }))} placeholder="••••••••" />
                  </FieldRow>
                ) : (
                  <FieldRow label="SSH 私钥 (PEM)" hint="粘贴 ~/.ssh/id_rsa 或 id_ed25519 内容">
                    <Textarea
                      value={ssh.priv_key}
                      onChange={v => setSSH(s => ({ ...s, priv_key: v }))}
                      placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                      rows={6}
                      mono
                    />
                  </FieldRow>
                )}
              </>
            ) : (
              <div className="space-y-2">
                <div className="rounded-lg border border-brand/20 bg-brand/[0.05] px-3 py-2.5">
                  <p className="text-[12px] text-brand-light/90">
                    已选择托管节点：
                    <span className="ml-1 font-mono text-white/90">
                      {selectedManagedNode?.username}@{selectedManagedNode?.host}:{selectedManagedNode?.port}
                    </span>
                  </p>
                  <p className="mt-1 text-[11px] text-white/45">
                    将复用托管节点已保存的 SSH 密码进行连接与部署，无需再次输入 credentials。
                  </p>
                </div>
                {/* Clash proxy config preview (reuses /nodes export endpoint) */}
                {selectedManagedNode && (
                  <div>
                    <p className="text-[10px] font-semibold uppercase tracking-[0.18em] text-white/30 mb-1">Clash 代理配置</p>
                    {selectedNodeProxyLoading ? (
                      <div className="rounded-lg border border-white/[0.07] bg-black/25 px-3 py-2.5 text-[12px] text-white/50">
                        读取节点代理配置中...
                      </div>
                    ) : (
                      <pre className="rounded-lg border border-white/[0.07] bg-black/25 px-3 py-2.5 font-mono text-[11px] text-white/55 leading-relaxed overflow-x-auto">
                        {selectedNodeProxyYAML || `proxies:\n  - name: ${selectedManagedNode.name}\n    type: http\n    server: ${selectedManagedNode.domain || selectedManagedNode.host}\n    port: 443\n    tls: true\n    skip-cert-verify: false`}
                      </pre>
                    )}
                    {selectedNodeProxyError && (
                      <p className="mt-1 text-[11px] text-rose-300/80">读取代理配置失败：{selectedNodeProxyError}</p>
                    )}
                    {!selectedNodeProxyLoading && !selectedNodeHasAuth && (
                      <p className="mt-1 text-[11px] text-amber-300/80">
                        当前托管节点尚未保存代理用户名/密码。可点击下方“补齐认证并导入”，无需完整重部署。
                      </p>
                    )}
                  </div>
                )}
              </div>
            )}

            {/* Validate button */}
            <div className="flex items-center gap-3">
              <button
                onClick={validateSSH}
                disabled={sshValidating}
                className="inline-flex items-center gap-2 rounded-lg bg-white/[0.07] px-4 py-2 text-sm font-medium text-white/80 hover:bg-white/[0.10] disabled:opacity-50"
              >
                {sshValidating ? <Loader2 size={13} className="animate-spin" /> : <Wifi size={13} />}
                测试连接
              </button>
              {sshResult?.valid && (
                <span className="flex items-center gap-1.5 text-[12px] text-emerald-400">
                  <CheckCircle2 size={13} />
                  已连接 · {sshResult.os} {sshResult.os_version} ({sshResult.arch})
                </span>
              )}
              {sshError && <span className="text-[12px] text-rose-400">{sshError}</span>}
            </div>

            {/* Proxy connectivity probe — shown after SSH test passes on a managed node */}
            {usingManagedNode && sshResult?.valid && (
              <div className="space-y-2.5">
                <div className="flex items-center gap-3">
                  <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-white/30">代理连通性</p>
                  <button
                    onClick={async () => {
                      setProbeResults(null); setProbeLoading(true)
                      try {
                        const data = await probeNode(selectedNodeId, 'domain')
                        setProbeResults(data.probe_results ?? [])
                      } catch { setProbeResults([]) }
                      finally { setProbeLoading(false) }
                    }}
                    disabled={probeLoading}
                    className="inline-flex items-center gap-1.5 rounded-md bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-white/55 hover:bg-white/[0.10] hover:text-white/80 transition-colors disabled:opacity-40"
                  >
                    {probeLoading
                      ? <><Loader2 size={11} className="animate-spin" />探测中</>
                      : <><Wifi size={11} />{probeResults ? '重新探测' : '探测代理'}</>}
                  </button>
                </div>

                {probeLoading && (
                  <div className="flex items-center gap-2 text-[12px] text-white/35 py-1">
                    <Loader2 size={13} className="animate-spin" /> 正在通过节点发起连通性测试…
                  </div>
                )}

                {probeResults !== null && probeResults.length > 0 && (
                  <div className="rounded-xl border border-white/[0.08] bg-black/10 p-3 space-y-1.5">
                    {probeResults.map((p, i) => (
                      <div key={i} className="flex items-center gap-2 text-xs">
                        {p.ok
                          ? <CheckCircle2 size={11} className="text-emerald-400 shrink-0" />
                          : <AlertCircle size={11} className="text-rose-400 shrink-0" />}
                        <span className="text-white/70 font-medium">{p.name}</span>
                        {p.ok
                          ? <span className="ml-auto font-mono text-emerald-400/80">{p.status_code} · {p.latency_ms}ms</span>
                          : <span className="ml-auto text-rose-400/80 truncate max-w-[180px]">{p.error}</span>}
                      </div>
                    ))}
                  </div>
                )}

                {probeResults !== null && probeResults.length === 0 && !probeLoading && (
                  <p className="text-[12px] text-white/35">探测未返回结果，节点可能尚未部署代理服务。</p>
                )}
              </div>
            )}
          </div>

          {(() => {
            const probeOk = probeResults !== null && probeResults.length > 0 && probeResults.every(r => r.ok)
            const canQuickImport = usingManagedNode && probeOk && selectedNodeHasAuth && !!(selectedManagedNode?.domain || selectedManagedNode?.name)
            if (canQuickImport) {
              const domainStr = selectedManagedNode!.domain || selectedManagedNode!.name || ''
              const parts = domainStr.split('.')
              const prefix = parts.length >= 3 ? parts[0] : ''
              const zoneName = parts.length >= 3 ? parts.slice(1).join('.') : domainStr
              const quickReq: QSDeployRequest = {
                deploy_type: 'vps',
                vps_node_id: selectedNodeId,
                node_prefix: prefix,
                node_name: selectedManagedNode!.name || domainStr,
                force_import: true,
                cloudflare: {
                  token: cfGlobal?.cf_token ?? '',
                  account_id: cfGlobal?.cf_account_id ?? '',
                  zone_id: '',
                  zone_name: zoneName,
                },
              }
              return (
                <div className="flex items-center justify-between border-t border-white/[0.05] pt-4">
                  <button onClick={prevStep} className="text-[13px] font-medium text-white/40 hover:text-white/70 transition-colors">← 返回</button>
                  <button
                    onClick={() => void startDeploy(quickReq)}
                    className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-semibold bg-emerald-500/20 text-emerald-300 hover:bg-emerald-500/30 transition-colors border border-emerald-500/30"
                  >
                    <Rocket size={14} /> 直接导入订阅
                  </button>
                </div>
              )
            }
            if (usingManagedNode && probeOk && !selectedNodeHasAuth) {
              const domainStr = selectedManagedNode?.domain || selectedManagedNode?.name || ''
              const parts = domainStr.split('.')
              const prefix = parts.length >= 3 ? parts[0] : ''
              const zoneName = parts.length >= 3 ? parts.slice(1).join('.') : domainStr
              const quickReq: QSDeployRequest = {
                deploy_type: 'vps',
                vps_node_id: selectedNodeId,
                node_prefix: prefix,
                node_name: selectedManagedNode?.name || domainStr,
                ensure_proxy_auth: true,
                cloudflare: {
                  token: cfGlobal?.cf_token ?? '',
                  account_id: cfGlobal?.cf_account_id ?? '',
                  zone_id: '',
                  zone_name: zoneName,
                },
              }
              return (
                <div className="flex items-center justify-between border-t border-white/[0.05] pt-4">
                  <button onClick={prevStep} className="text-[13px] font-medium text-white/40 hover:text-white/70 transition-colors">← 返回</button>
                  <button
                    onClick={() => void startDeploy(quickReq)}
                    className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-semibold bg-amber-500/20 text-amber-300 hover:bg-amber-500/30 transition-colors border border-amber-500/30"
                  >
                    <Rocket size={14} /> 补齐认证并导入
                  </button>
                </div>
              )
            }
            return (
              <NavButtons
                onBack={prevStep}
                onNext={nextStep}
                nextLabel="下一步"
                nextDisabled={!sshResult?.valid}
              />
            )
          })()}
        </Card>
      )}

      {/* ── Step: CF credentials ── */}
      {step === 'cf_creds' && (
        <Card title="Cloudflare 凭据" icon={<Globe size={16} />}>
          <div className="space-y-4">
            {/* Shared CF config banner — shows configured status and modify button */}
            <CFConfigBanner
              config={cfGlobal}
              loading={cfGlobalLoading}
              onConfigure={() => setShowCFModal(true)}
            />

            {/* Only show manual token entry if not pre-filled from global config */}
            {!cfGlobal?.cf_token && (
              <>
                <FieldRow label="API Token" hint="需要 Zone:Read + DNS:Edit 权限">
                  <Input
                    type="password"
                    value={cf.token}
                    onChange={v => { setCF(c => ({ ...c, token: v })); setCFResult(null) }}
                    placeholder="Cloudflare API Token"
                    mono
                  />
                </FieldRow>
                <FieldRow label="Account ID (可选)" hint="留空则自动检测">
                  <Input
                    value={cf.account_id}
                    onChange={v => setCF(c => ({ ...c, account_id: v }))}
                    placeholder="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
                    mono
                  />
                </FieldRow>
              </>
            )}

            <div className="flex items-center gap-3">
              <button
                onClick={validateCF}
                disabled={cfValidating || !cf.token.trim()}
                className="inline-flex items-center gap-2 rounded-lg bg-white/[0.07] px-4 py-2 text-sm font-medium text-white/80 hover:bg-white/[0.10] disabled:opacity-50"
              >
                {cfValidating ? <Loader2 size={13} className="animate-spin" /> : <ShieldCheck size={13} />}
                {cfResult?.valid ? '重新验证' : '验证 Token'}
              </button>
              {cfResult?.valid && (
                <span className="flex items-center gap-1.5 text-[12px] text-emerald-400">
                  <CheckCircle2 size={13} /> Token 有效，找到 {cfResult.zones?.length ?? 0} 个域名
                </span>
              )}
              {cfError && <span className="text-[12px] text-rose-400">{cfError}</span>}
            </div>

            {/* Zone selector */}
            {cfResult?.valid && cfResult.zones && cfResult.zones.length > 0 && (
              <FieldRow label="选择域名">
                <div className="space-y-1.5">
                  {cfResult.zones.map(z => (
                    <button
                      key={z.id}
                      onClick={() => handleZoneSelect(z.id)}
                      className={[
                        'flex w-full items-center gap-2.5 rounded-lg border px-3 py-2.5 text-left transition-colors',
                        selectedZoneId === z.id
                          ? 'border-brand/35 bg-brand/[0.06] text-white'
                          : 'border-white/[0.06] bg-white/[0.02] text-white/60 hover:border-white/[0.12] hover:text-white/80',
                      ].join(' ')}
                    >
                      <Globe size={13} className={selectedZoneId === z.id ? 'text-brand-light' : 'text-white/30'} />
                      <span className="font-mono text-[13px]">{z.name}</span>
                      {selectedZoneId === z.id && <CheckCircle2 size={13} className="ml-auto text-brand-light" />}
                    </button>
                  ))}
                </div>
              </FieldRow>
            )}
          </div>

          <NavButtons
            onBack={prevStep}
            onNext={nextStep}
            nextLabel="下一步"
            nextDisabled={!cfResult?.valid || !selectedZoneId}
          />
        </Card>
      )}

      {/* ── Step: VPS config ── */}
      {step === 'vps_config' && (() => {
        const vpsIP = selectedNodeId
          ? (managedNodes.find(n => n.id === selectedNodeId)?.host ?? ssh.host)
          : ssh.host
        const nodeDomain = vpsConfig.node_prefix && selectedZone
          ? `${vpsConfig.node_prefix}.${selectedZone.name}`
          : selectedZone?.name ?? ''

        const runNodeCheck = async () => {
          if (!nodeDomain) return
          setNodeCheck(null); setNodeCheckError(''); setNodeCheckLoading(true)
          try {
            const res = await quickStartCheckNode({ domain: nodeDomain, vps_ip: vpsIP || undefined })
            setNodeCheck(res)
          } catch (e) {
            setNodeCheckError(e instanceof Error ? e.message : '检测失败')
          } finally {
            setNodeCheckLoading(false)
          }
        }

        return (
          <Card title="节点配置" icon={<Server size={16} />}>
            <div className="space-y-4">
              <FieldRow label="DNS 记录前缀" hint={`节点域名: <前缀>.${selectedZone?.name ?? 'yourdomain.com'}`}>
                <Input
                  value={vpsConfig.node_prefix}
                  onChange={v => {
                    setVPSConfig(c => ({ ...c, node_prefix: v }))
                    setNodeCheck(null); setNodeCheckError('')
                    if (!vpsNameEdited) {
                      const host = v && selectedZone ? `${v}.${selectedZone.name}` : selectedZone?.name ?? ''
                      setVPSConfig(c => ({ ...c, node_name: host }))
                    }
                  }}
                  placeholder="node"
                />
              </FieldRow>

              {selectedZone && (
                <div className="flex items-center gap-2 rounded-lg border border-emerald-500/15 bg-emerald-500/[0.04] px-3 py-2">
                  <Globe size={12} className="text-emerald-400" />
                  <span className="font-mono text-[12px] text-emerald-300 flex-1">
                    {nodeDomain}
                    {vpsIP && <span className="ml-1 text-emerald-400/50">→ {vpsIP}</span>}
                  </span>
                  <button
                    onClick={runNodeCheck}
                    disabled={nodeCheckLoading || !nodeDomain}
                    className="flex items-center gap-1.5 rounded-md bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-white/60 hover:bg-white/[0.10] hover:text-white/80 transition-colors disabled:opacity-40"
                  >
                    {nodeCheckLoading
                      ? <><Loader2 size={11} className="animate-spin" />检测中</>
                      : <><ShieldCheck size={11} />检测节点</>}
                  </button>
                </div>
              )}

              {/* Check result banner */}
              {nodeCheck?.reusable && (
                <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/[0.07] px-3.5 py-3 flex items-start gap-3">
                  <CheckCircle2 size={15} className="mt-0.5 flex-shrink-0 text-emerald-400" />
                  <div className="min-w-0 flex-1">
                    <p className="text-[13px] font-semibold text-emerald-300">节点已部署且可用</p>
                    <p className="mt-0.5 text-[11.5px] text-emerald-400/70">
                      TLS 证书剩余 {nodeCheck.days_left} 天，无需重新部署，可直接导入订阅。
                    </p>
                  </div>
                </div>
              )}
              {nodeCheck && !nodeCheck.reusable && (
                <div className="rounded-lg border border-amber-500/25 bg-amber-500/[0.05] px-3.5 py-3 flex items-start gap-3">
                  <AlertCircle size={15} className="mt-0.5 flex-shrink-0 text-amber-400" />
                  <div className="min-w-0 flex-1">
                    <p className="text-[13px] font-semibold text-amber-300">节点不可复用</p>
                    <p className="mt-0.5 text-[11.5px] text-amber-400/70">{nodeCheck.skip_reason}</p>
                  </div>
                </div>
              )}
              {nodeCheckError && (
                <div className="rounded-lg border border-rose-500/20 bg-rose-500/[0.05] px-3.5 py-2 text-[12px] text-rose-300">
                  {nodeCheckError}
                </div>
              )}

              <FieldRow label="节点显示名称 (可选)" hint="在订阅列表中显示的名称">
                <Input
                  value={vpsConfig.node_name}
                  onChange={v => { setVPSConfig(c => ({ ...c, node_name: v })); setVPSNameEdited(true) }}
                  placeholder="自动生成"
                />
              </FieldRow>
            </div>

            {nodeCheck?.reusable
              ? (
                <div className="flex items-center justify-between border-t border-white/[0.05] pt-4">
                  <button onClick={prevStep} className="text-[13px] font-medium text-white/40 hover:text-white/70 transition-colors">← 返回</button>
                  <button
                    onClick={() => void startDeploy()}
                    className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-semibold bg-emerald-500/20 text-emerald-300 hover:bg-emerald-500/30 transition-colors"
                  >
                    <Rocket size={14} /> 直接导入订阅
                  </button>
                </div>
              )
              : <NavButtons onBack={prevStep} onNext={nextStep} nextLabel="预览部署" />
            }
          </Card>
        )
      })()}

      {/* ── Step: Confirm ── */}
      {step === 'confirm' && (
        <Card title="确认部署" icon={<Rocket size={16} />}>
          <div className="space-y-3">
            <p className="text-sm text-white/50">即将执行以下操作：</p>

            {deployType === 'vps' && (
              <SummaryTable rows={[
                ['类型', 'VPS + gost HTTPS+TLS'],
                ['服务器', `${ssh.host}:${ssh.port}`],
                ['凭据来源', usingManagedNode ? `托管节点（${selectedManagedNode?.name ?? '已选节点'}）` : '本次输入（将保存到托管节点）'],
                ['节点域名', vpsConfig.node_prefix && selectedZone
                  ? `${vpsConfig.node_prefix}.${selectedZone.name}` : selectedZone?.name ?? '—'],
                ['CF Zone', selectedZone?.name ?? '—'],
                ['证书', 'Let\'s Encrypt DNS-01 自动申请'],
                ['gost 端口', '443 (TLS)'],
              ]} />
            )}

            {deployType === 'cf_workers' && (
              <>
                <SummaryTable rows={[
                  ['类型', 'Cloudflare Workers'],
                  ['顶级域名', selectedZone?.name ?? '—'],
                  ['代理 Worker', '自动生成（如 cf-proxy-a7k3）'],
                  ['发布 Worker', '自动生成（如 cf-sub-9x2p）'],
                  ['订阅模板', 'loyalSoldier 分流规则'],
                ]} />
                <div className="rounded-lg border border-sky-500/15 bg-sky-500/[0.04] p-3">
                  <div className="flex gap-2">
                    <Globe size={13} className="mt-px flex-shrink-0 text-sky-400" />
                    <p className="text-[12px] text-sky-300/80">
                      子域名由系统随机生成，绑定至 <span className="font-mono font-semibold">{selectedZone?.name}</span>。
                      代理与订阅发布各使用独立 Worker，无需手动填写任何名称。
                    </p>
                  </div>
                </div>
              </>
            )}

            <div className="rounded-lg border border-amber-500/15 bg-amber-500/[0.04] p-3">
              <div className="flex gap-2">
                <AlertCircle size={14} className="mt-px flex-shrink-0 text-amber-400" />
                <p className="text-[12px] text-amber-300/80">
                  部署过程约需 {deployType === 'vps' ? '5–8' : '2–3'} 分钟，请保持页面开启。
                  {deployType === 'vps' && ' 申请 Let\'s Encrypt 证书时需要 Cloudflare DNS 传播（约 60 秒）。'}
                </p>
              </div>
            </div>
          </div>

          <NavButtons
            onBack={prevStep}
            onNext={startDeploy}
            nextLabel="开始部署"
            nextIcon={<Rocket size={14} />}
          />
        </Card>
      )}

      {/* ── Step: Deploying (SSE stream) ── */}
      {step === 'deploying' && (
        <Card
          title={deployDone ? (deployError ? '部署失败' : '部署完成') : '正在部署...'}
          icon={deployDone
            ? (deployError ? <XCircle size={16} className="text-rose-400" /> : <CheckCircle2 size={16} className="text-emerald-400" />)
            : <Loader2 size={16} className="animate-spin text-brand-light" />}
        >
          {/* Phase progress summary */}
          <PhaseProgress events={events} deployType={deployType} />

          {/* Event log */}
          <div className="mt-4 max-h-80 overflow-y-auto rounded-lg border border-white/[0.06] bg-black/20 px-3 py-2">
            {events.length === 0 && !deployDone && (
              <p className="py-4 text-center text-[12px] text-white/30">等待事件...</p>
            )}
            {events.map((e, i) => (
              <EventRow key={i} event={e} isLast={i === events.length - 1} />
            ))}
            <div ref={eventsEndRef} />
          </div>

          {deployError && (
            <div className="mt-3 rounded-lg border border-rose-500/20 bg-rose-500/[0.05] p-3">
              <p className="text-[12px] text-rose-300">{deployError}</p>
            </div>
          )}

          {deployDone && (
            <div className="mt-4 flex gap-3">
              {!deployError && (
                <a
                  href="/"
                  className="inline-flex items-center gap-2 rounded-lg bg-brand/15 px-4 py-2 text-sm font-semibold text-brand-light hover:bg-brand/20"
                >
                  <Rocket size={14} /> 前往概览
                </a>
              )}
              <button
                onClick={reset}
                className="inline-flex items-center gap-2 rounded-lg bg-white/[0.07] px-4 py-2 text-sm font-medium text-white/70 hover:bg-white/[0.10]"
              >
                <RefreshCw size={13} /> 重新部署
              </button>
            </div>
          )}
        </Card>
      )}

      {/* CF config modal — opened from the banner's 修改 button */}
      {showCFModal && (
        <CFConfigModal
          initial={cfGlobal}
          save={saveCFGlobal}
          onClose={() => setShowCFModal(false)}
          onSaved={() => {
            setShowCFModal(false)
            // Clear validation result so it re-validates with the new token
            setCFResult(null)
            setCFError('')
          }}
        />
      )}

      {/* Basic concepts — viewable regardless of whether the core is running */}
      <div className="flex items-center gap-3">
        <div className="h-px flex-1 bg-white/[0.06]" />
        <span className="text-[11px] font-semibold uppercase tracking-[0.18em] text-white/25">基础概念</span>
        <div className="h-px flex-1 bg-white/[0.06]" />
      </div>
      <WorkflowDiagram activeMode={(qlCfg?.network as { mode?: string } | undefined)?.mode ?? ''} />
      <DNSModeDiagram activeMode={(qlCfg?.dns as { mode?: string } | undefined)?.mode ?? ''} />
    </div>
  )
}

// ── small utility components ───────────────────────────────────────────────────

function Card({
  title, icon, children,
}: {
  title: string
  icon: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="rounded-xl border border-white/[0.07] bg-white/[0.025] p-5 space-y-5">
      <div className="flex items-center gap-2.5">
        <span className="text-brand-light">{icon}</span>
        <h2 className="text-[15px] font-semibold text-white">{title}</h2>
      </div>
      {children}
    </div>
  )
}

function NavButtons({
  onBack, onNext, nextLabel = '下一步', nextDisabled = false, nextIcon,
}: {
  onBack?: () => void
  onNext?: () => void
  nextLabel?: string
  nextDisabled?: boolean
  nextIcon?: React.ReactNode
}) {
  return (
    <div className="flex items-center justify-between border-t border-white/[0.05] pt-4">
      {onBack ? (
        <button
          onClick={onBack}
          className="text-[13px] font-medium text-white/40 hover:text-white/70 transition-colors"
        >
          ← 返回
        </button>
      ) : <span />}
      {onNext && (
        <button
          onClick={() => onNext()}
          disabled={nextDisabled}
          className={[
            'inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-semibold transition-colors',
            nextDisabled
              ? 'bg-white/[0.04] text-white/25 cursor-not-allowed'
              : 'bg-brand/80 text-white hover:bg-brand',
          ].join(' ')}
        >
          {nextIcon}
          {nextLabel}
          {!nextIcon && <ChevronRight size={14} />}
        </button>
      )}
    </div>
  )
}

function SummaryTable({ rows }: { rows: [string, string][] }) {
  return (
    <div className="rounded-lg border border-white/[0.06] divide-y divide-white/[0.04] overflow-hidden">
      {rows.map(([k, v]) => (
        <div key={k} className="flex items-start gap-4 px-3 py-2.5">
          <span className="w-28 flex-shrink-0 text-[12px] text-white/35">{k}</span>
          <span className="text-[13px] font-medium text-white/80">{v}</span>
        </div>
      ))}
    </div>
  )
}

// ── phase progress (groups events by phase) ────────────────────────────────────

const VPS_PHASES = ['existing_check', 'ssh_test', 'env_detect', 'provision', 'cert_dns', 'import', 'configure', 'verify']
const WORKERS_PHASES = ['cf_validate', 'worker_deploy', 'publish', 'import', 'configure', 'verify']

function PhaseProgress({ events, deployType }: { events: QSEvent[]; deployType: QSDeployType }) {
  const phases = deployType === 'vps' ? VPS_PHASES : WORKERS_PHASES

  // Compute per-phase status from events
  const phaseStatus: Record<string, 'pending' | 'running' | 'ok' | 'error' | 'warning'> = {}
  for (const e of events) {
    const prev = phaseStatus[e.phase]
    if (e.status === 'error') phaseStatus[e.phase] = 'error'
    else if (e.status === 'warning' && prev !== 'error') phaseStatus[e.phase] = 'warning'
    else if (e.status === 'running' && !prev) phaseStatus[e.phase] = 'running'
    else if (e.status === 'ok' && prev !== 'error' && prev !== 'warning') phaseStatus[e.phase] = 'ok'
    else if (!prev) phaseStatus[e.phase] = 'running'
  }

  return (
    <div className="flex flex-wrap gap-2">
      {phases.map(p => {
        const s = phaseStatus[p] ?? 'pending'
        const colors: Record<string, string> = {
          pending: 'border-white/[0.06] bg-white/[0.02] text-white/25',
          running: 'border-brand/25 bg-brand/[0.06] text-brand-light',
          ok:      'border-emerald-500/20 bg-emerald-500/[0.05] text-emerald-400',
          error:   'border-rose-500/20 bg-rose-500/[0.05] text-rose-400',
          warning: 'border-amber-500/20 bg-amber-500/[0.05] text-amber-400',
        }
        const icons: Record<string, React.ReactNode> = {
          pending: null,
          running: <Loader2 size={10} className="animate-spin" />,
          ok:      <CheckCircle2 size={10} />,
          error:   <XCircle size={10} />,
          warning: <AlertCircle size={10} />,
        }
        return (
          <div
            key={p}
            className={['flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-[11px] font-medium', colors[s]].join(' ')}
          >
            {icons[s]}
            {phaseLabel(p)}
          </div>
        )
      })}
    </div>
  )
}

// ── QLSection: one-click restart with last saved config ───────────────────────

const NET_MODE_LABEL: Record<string, string> = {
  tproxy: 'TProxy · 透明代理',
  tun:    'TUN · 虚拟网卡',
  redir:  'Redir · 重定向',
  none:   '无接管',
}
const DNS_MODE_LABEL: Record<string, string> = {
  'fake-ip':    'Fake-IP',
  'redir-host': 'Redir-Host',
}
const DNS_STRATEGY_LABEL: Record<string, string> = {
  split:   '分流解析',
  privacy: '隐私解析',
  legacy:  '传统解析',
}

function QLSection({
  qlCfg, qlActive, qlLoading, qlState, qlEvents, qlError, qlEndRef, onLaunch, onReset,
}: {
  qlCfg: Record<string, unknown> | null
  qlActive: ActiveSource | null
  qlLoading: boolean
  qlState: 'idle' | 'launching' | 'done' | 'error'
  qlEvents: LaunchEvent[]
  qlError: string
  qlEndRef: React.RefObject<HTMLDivElement | null>
  onLaunch: () => void
  onReset: () => void
}) {
  if (qlLoading) {
    return (
      <div className="animate-pulse rounded-2xl border border-white/[0.07] bg-white/[0.02] p-6 space-y-4">
        <div className="h-4 w-32 rounded bg-white/[0.06]" />
        <div className="grid grid-cols-3 gap-3">
          {[0, 1, 2].map(i => <div key={i} className="h-16 rounded-xl bg-white/[0.04]" />)}
        </div>
        <div className="h-12 rounded-xl bg-white/[0.04]" />
      </div>
    )
  }

  const hasConfig = qlCfg !== null && qlActive !== null
  const sourceName = qlActive?.sub_name ?? qlActive?.filename ?? '—'

  const network = (qlCfg?.network ?? {}) as Record<string, unknown>
  const dns = (qlCfg?.dns ?? {}) as Record<string, unknown>
  const netMode = (network.mode as string) ?? 'tproxy'
  const dnsMode = (dns.mode as string) ?? 'fake-ip'
  const dnsStrategy = (dns.strategy as string) ?? 'split'
  const hasDoh = Array.isArray(dns.doh) && (dns.doh as string[]).length > 0

  const isActive = qlState === 'launching' || qlState === 'done' || qlState === 'error'

  return (
    <div className={[
      'rounded-2xl border p-6 space-y-5 transition-colors',
      qlState === 'done'
        ? 'border-emerald-500/25 bg-emerald-500/[0.04]'
        : qlState === 'error'
          ? 'border-rose-500/20 bg-rose-500/[0.04]'
          : 'border-brand/20 bg-gradient-to-br from-brand/[0.06] to-transparent',
    ].join(' ')}>

      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2.5">
          <Sparkles size={15} className="text-brand-light" />
          <h2 className="text-[15px] font-semibold text-white">上次运行配置</h2>
        </div>
        {qlState === 'launching' && (
          <span className="flex items-center gap-1.5 text-[12px] text-brand-light">
            <Loader2 size={12} className="animate-spin" /> 正在启动…
          </span>
        )}
        {qlState === 'done' && (
          <span className="flex items-center gap-1.5 text-[12px] text-emerald-400">
            <CheckCircle2 size={12} /> 服务已启动
          </span>
        )}
        {qlState === 'error' && (
          <span className="flex items-center gap-1.5 text-[12px] text-rose-400">
            <XCircle size={12} /> 启动失败
          </span>
        )}
      </div>

      {/* Config summary — idle only */}
      {qlState === 'idle' && hasConfig && (
        <div className="grid grid-cols-3 gap-3">
          {([
            { label: '配置来源', value: sourceName },
            { label: '网络模式', value: NET_MODE_LABEL[netMode] ?? netMode },
            {
              label: 'DNS 解析',
              value: [
                DNS_MODE_LABEL[dnsMode] ?? dnsMode,
                DNS_STRATEGY_LABEL[dnsStrategy] ?? dnsStrategy,
                hasDoh ? 'DoH' : '',
              ].filter(Boolean).join(' · '),
            },
          ] as { label: string; value: string }[]).map(({ label, value }) => (
            <div key={label} className="rounded-xl border border-white/[0.07] bg-white/[0.03] px-3.5 py-3">
              <p className="text-[10px] font-semibold uppercase tracking-[0.16em] text-white/30">{label}</p>
              <p className="mt-1 truncate text-[13px] font-medium text-white/80" title={value}>{value}</p>
            </div>
          ))}
        </div>
      )}

      {/* No config notice */}
      {qlState === 'idle' && !hasConfig && (
        <div className="rounded-xl border border-amber-500/15 bg-amber-500/[0.04] px-4 py-3 text-[13px] text-amber-300/80">
          尚未完成初始配置，请先前往
          <a href="/setup" className="ml-1 font-semibold text-amber-300 underline underline-offset-2">向导配置页</a>
          完成首次设置。
        </div>
      )}

      {/* SSE event log */}
      {isActive && (
        <div className="max-h-52 overflow-y-auto rounded-xl border border-white/[0.07] bg-black/20 px-3 py-2">
          {qlEvents.length === 0 && qlState === 'launching' && (
            <p className="py-3 text-center text-[12px] text-white/30">等待启动事件…</p>
          )}
          {qlEvents.map((e, i) => {
            const icons: Record<string, React.ReactNode> = {
              running: <Loader2 size={11} className="animate-spin text-brand-light" />,
              ok:      <CheckCircle2 size={11} className="text-emerald-400" />,
              error:   <XCircle size={11} className="text-rose-400" />,
              skip:    <span className="inline-block h-1.5 w-1.5 rounded-full bg-white/20" />,
              info:    <span className="inline-block h-1.5 w-1.5 rounded-full bg-white/20" />,
            }
            const icon = icons[e.status ?? 'info'] ?? icons.info
            return (
              <div key={i} className="flex items-start gap-2 py-1.5 text-[12px] border-b border-white/[0.03] last:border-0">
                <span className="mt-px flex-shrink-0">{icon}</span>
                <span className={e.status === 'error' ? 'text-rose-300' : 'text-white/65'}>{e.message}</span>
              </div>
            )
          })}
          <div ref={qlEndRef} />
        </div>
      )}

      {/* Error detail */}
      {qlState === 'error' && qlError && (
        <div className="rounded-lg border border-rose-500/20 bg-rose-500/[0.05] px-3.5 py-2.5 text-[12px] text-rose-300">
          {qlError}
        </div>
      )}

      {/* Action buttons */}
      {qlState === 'idle' && hasConfig && (
        <button
          onClick={onLaunch}
          className="w-full flex items-center justify-center gap-3 rounded-xl bg-brand/80 hover:bg-brand px-6 py-4 text-[15px] font-semibold text-white transition-colors"
        >
          <Rocket size={17} />
          立即启动上次配置
          <ChevronRight size={16} className="opacity-60" />
        </button>
      )}

      {qlState === 'done' && (
        <div className="flex gap-3">
          <a
            href="/"
            className="flex flex-1 items-center justify-center gap-2 rounded-xl border border-emerald-500/25 bg-emerald-500/20 px-4 py-3 text-[14px] font-semibold text-emerald-300 hover:bg-emerald-500/30 transition-colors"
          >
            前往概览 <ChevronRight size={14} />
          </a>
          <button
            onClick={onReset}
            className="flex items-center gap-2 rounded-xl bg-white/[0.06] px-4 py-3 text-[13px] font-medium text-white/55 hover:bg-white/[0.10] transition-colors"
          >
            <RefreshCw size={13} /> 重新启动
          </button>
        </div>
      )}

      {qlState === 'error' && (
        <button
          onClick={onReset}
          className="flex items-center gap-2 rounded-xl bg-white/[0.06] px-4 py-3 text-[13px] font-medium text-white/55 hover:bg-white/[0.10] transition-colors"
        >
          <RefreshCw size={13} /> 重试
        </button>
      )}
    </div>
  )
}

// ── step label helper ──────────────────────────────────────────────────────────
function stepLabel(step: WizardStep): string {
  const labels: Record<WizardStep, string> = {
    select:      '类型选择',
    vps_connect: 'SSH 连接',
    cf_creds:    'CF 账号',
    vps_config:  '节点配置',
    confirm:     '确认',
    deploying:   '部署中',
    done:        '完成',
  }
  return labels[step] ?? step
}
