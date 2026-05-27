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
  Zap,
} from 'lucide-react'
import {
  quickStartValidateCF,
  quickStartValidateVPS,
  QUICKSTART_DEPLOY_URL,
  type QSDeployRequest,
  type QSDeployType,
  type QSEvent,
  type QSCFZone,
  type QSValidateCFResult,
  type QSValidateVPSResult,
} from '../api/client'

// ── types ──────────────────────────────────────────────────────────────────────

type WizardStep =
  | 'select'         // Step 0: choose VPS or CF Workers
  | 'vps_connect'    // Path B Step 1: SSH credentials
  | 'cf_creds'       // Step: CF token + zone select (both paths)
  | 'workers_config' // Path A only: worker name + custom domain
  | 'vps_config'     // Path B only: node prefix / node name
  | 'confirm'        // Both: review & deploy
  | 'deploying'      // SSE stream
  | 'done'           // Success or failure

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

interface WorkersForm {
  worker_name: string
  custom_domain: string
  zone_id: string
}

interface VPSConfigForm {
  node_prefix: string
  node_name: string
}

// ── helpers ───────────────────────────────────────────────────────────────────

function slugify(s: string) {
  return s.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/-+/g, '-').replace(/^-|-$/g, '').slice(0, 63) || 'cf-proxy'
}

function phaseLabel(phase: string): string {
  const labels: Record<string, string> = {
    cf_validate:  'CF 验证',
    worker_deploy:'Workers 部署',
    ssh_test:     'SSH 连接',
    env_detect:   '环境检测',
    provision:    '服务部署',
    cert_dns:     'DNS & 证书',
    import:       '订阅导入',
    configure:    '自动配置',
    verify:       '连通验证',
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

  // SSH form (VPS only)
  const [ssh, setSSH] = useState<SSHForm>({
    host: '', port: '22', user: 'root', auth_type: 'password', password: '', priv_key: '',
  })
  const [sshValidating, setSSHValidating] = useState(false)
  const [sshResult, setSSHResult] = useState<QSValidateVPSResult | null>(null)
  const [sshError, setSSHError] = useState('')

  // CF form
  const [cf, setCF] = useState<CFForm>({ token: '', account_id: '' })
  const [cfValidating, setCFValidating] = useState(false)
  const [cfResult, setCFResult] = useState<QSValidateCFResult | null>(null)
  const [cfError, setCFError] = useState('')
  const [selectedZoneId, setSelectedZoneId] = useState('')

  // CF Workers config
  const [workers, setWorkers] = useState<WorkersForm>({
    worker_name: '', custom_domain: '', zone_id: '',
  })
  const [workerNameEdited, setWorkerNameEdited] = useState(false)

  // VPS config
  const [vpsConfig, setVPSConfig] = useState<VPSConfigForm>({ node_prefix: 'node', node_name: '' })
  const [vpsNameEdited, setVPSNameEdited] = useState(false)

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
        if (deployType === 'cf_workers') {
          setWorkers(w => ({ ...w, zone_id: res.zones![0].id }))
        }
      }
    } catch (e) {
      setCFError(e instanceof Error ? e.message : '请求失败')
    } finally { setCFValidating(false) }
  }, [cf.token, cf.account_id, deployType])

  // ── SSH validation ──────────────────────────────────────────────────────────
  const validateSSH = useCallback(async () => {
    if (!ssh.host.trim()) { setSSHError('请输入服务器地址'); return }
    setSSHValidating(true); setSSHError(''); setSSHResult(null)
    try {
      const res = await quickStartValidateVPS({
        host: ssh.host.trim(),
        port: parseInt(ssh.port) || 22,
        user: ssh.user.trim() || 'root',
        auth_type: ssh.auth_type,
        password: ssh.auth_type === 'password' ? ssh.password : undefined,
        priv_key: ssh.auth_type === 'key' ? ssh.priv_key.trim() : undefined,
      })
      setSSHResult(res)
      if (!res.valid) setSSHError(res.error ?? '连接失败')
    } catch (e) {
      setSSHError(e instanceof Error ? e.message : '请求失败')
    } finally { setSSHValidating(false) }
  }, [ssh])

  // ── deploy ──────────────────────────────────────────────────────────────────
  const startDeploy = useCallback(async () => {
    setStep('deploying')
    setEvents([])
    setDeployError('')
    setDeployDone(false)
    setDeployId('')

    const selectedZone = cfResult?.zones?.find(z => z.id === selectedZoneId)

    const req: QSDeployRequest = {
      deploy_type: deployType,
      cloudflare: {
        token: cf.token.trim(),
        account_id: cf.account_id.trim(),
        zone_id: selectedZoneId,
        zone_name: selectedZone?.name ?? '',
      },
    }

    if (deployType === 'cf_workers') {
      req.node_name = workers.custom_domain || workers.worker_name
      req.workers_domain = {
        worker_name: workers.worker_name,
        custom_domain: workers.custom_domain,
        zone_id: workers.zone_id || selectedZoneId,
      }
    } else {
      req.node_prefix = vpsConfig.node_prefix.trim()
      req.node_name = vpsConfig.node_name.trim() || undefined
      req.vps = {
        host: ssh.host.trim(),
        port: parseInt(ssh.port) || 22,
        user: ssh.user.trim() || 'root',
        auth_type: ssh.auth_type,
        password: ssh.auth_type === 'password' ? ssh.password : undefined,
        priv_key: ssh.auth_type === 'key' ? ssh.priv_key.trim() : undefined,
      }
    }

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
  }, [deployType, cf, selectedZoneId, cfResult, workers, vpsConfig, ssh])

  // ── zone auto-select on CF validate ───────────────────────────────────────
  const selectedZone: QSCFZone | undefined = cfResult?.zones?.find(z => z.id === selectedZoneId)

  const handleZoneSelect = (id: string) => {
    setSelectedZoneId(id)
    if (deployType === 'cf_workers') setWorkers(w => ({ ...w, zone_id: id }))
  }

  // ── step navigation helpers ────────────────────────────────────────────────
  const stepOrder: WizardStep[] =
    deployType === 'cf_workers'
      ? ['select', 'cf_creds', 'workers_config', 'confirm', 'deploying', 'done']
      : ['select', 'vps_connect', 'cf_creds', 'vps_config', 'confirm', 'deploying', 'done']

  const stepIdx = stepOrder.indexOf(step)
  const prevStep = () => { if (stepIdx > 0) setStep(stepOrder[stepIdx - 1]) }
  const nextStep = () => { if (stepIdx < stepOrder.length - 1) setStep(stepOrder[stepIdx + 1]) }

  // ── reset ──────────────────────────────────────────────────────────────────
  const reset = () => {
    setStep('select')
    setSSH({ host: '', port: '22', user: 'root', auth_type: 'password', password: '', priv_key: '' })
    setSSHResult(null); setSSHError('')
    setCF({ token: '', account_id: '' }); setCFResult(null); setCFError(''); setSelectedZoneId('')
    setWorkers({ worker_name: '', custom_domain: '', zone_id: '' }); setWorkerNameEdited(false)
    setVPSConfig({ node_prefix: 'node', node_name: '' }); setVPSNameEdited(false)
    setEvents([]); setDeployError(''); setDeployDone(false); setDeployId('')
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
          几步完成出口节点部署与 ClashForge 自动配置。
        </p>
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
                  自建 SOCKS5+TLS 节点，兼容全部服务，适合长期使用。
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
          </div>

          <NavButtons
            onBack={prevStep}
            onNext={nextStep}
            nextLabel="下一步"
            nextDisabled={!sshResult?.valid}
          />
        </Card>
      )}

      {/* ── Step: CF credentials ── */}
      {step === 'cf_creds' && (
        <Card title="Cloudflare 凭据" icon={<Globe size={16} />}>
          <div className="space-y-4">
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

            <div className="flex items-center gap-3">
              <button
                onClick={validateCF}
                disabled={cfValidating || !cf.token.trim()}
                className="inline-flex items-center gap-2 rounded-lg bg-white/[0.07] px-4 py-2 text-sm font-medium text-white/80 hover:bg-white/[0.10] disabled:opacity-50"
              >
                {cfValidating ? <Loader2 size={13} className="animate-spin" /> : <ShieldCheck size={13} />}
                验证 Token
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

      {/* ── Step: CF Workers config ── */}
      {step === 'workers_config' && (
        <Card title="Workers 配置" icon={<Zap size={16} />}>
          <div className="space-y-4">
            <FieldRow label="Worker 子域名前缀" hint={`自定义域名将是: <前缀>.${selectedZone?.name ?? 'yourdomain.com'}`}>
              <Input
                value={workers.custom_domain.split('.')[0] ?? ''}
                onChange={v => {
                  const prefix = v.trim()
                  setWorkers(w => ({
                    ...w,
                    custom_domain: prefix && selectedZone ? `${prefix}.${selectedZone.name}` : prefix,
                  }))
                  if (!workerNameEdited) {
                    setWorkers(w => ({ ...w, worker_name: slugify(prefix || 'proxy') }))
                  }
                }}
                placeholder="proxy"
              />
            </FieldRow>

            {selectedZone && workers.custom_domain && (
              <div className="flex items-center gap-2 rounded-lg border border-emerald-500/15 bg-emerald-500/[0.04] px-3 py-2">
                <Globe size={12} className="text-emerald-400" />
                <span className="font-mono text-[12px] text-emerald-300">
                  {workers.custom_domain.includes('.') ? workers.custom_domain : `${workers.custom_domain}.${selectedZone.name}`}
                </span>
              </div>
            )}

            <FieldRow label="Worker 名称" hint="Cloudflare 后台的 Worker 标识符">
              <Input
                value={workers.worker_name}
                onChange={v => { setWorkers(w => ({ ...w, worker_name: slugify(v) })); setWorkerNameEdited(true) }}
                placeholder="cf-proxy"
                mono
              />
            </FieldRow>

            <FieldRow label="节点名称 (可选)" hint="订阅中显示的名称，留空则使用域名">
              <Input
                value={workers.custom_domain}
                onChange={() => {}}
                placeholder={workers.custom_domain || '自动生成'}
                disabled
              />
            </FieldRow>
          </div>

          <NavButtons
            onBack={prevStep}
            onNext={nextStep}
            nextLabel="预览部署"
            nextDisabled={!workers.worker_name.trim() || !workers.custom_domain.trim()}
          />
        </Card>
      )}

      {/* ── Step: VPS config ── */}
      {step === 'vps_config' && (
        <Card title="节点配置" icon={<Server size={16} />}>
          <div className="space-y-4">
            <FieldRow label="DNS 记录前缀" hint={`节点域名: <前缀>.${selectedZone?.name ?? 'yourdomain.com'}`}>
              <Input
                value={vpsConfig.node_prefix}
                onChange={v => {
                  setVPSConfig(c => ({ ...c, node_prefix: v }))
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
                <span className="font-mono text-[12px] text-emerald-300">
                  {vpsConfig.node_prefix ? `${vpsConfig.node_prefix}.${selectedZone.name}` : selectedZone.name}
                  <span className="ml-1 text-emerald-400/50">→ {ssh.host}</span>
                </span>
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

          <NavButtons onBack={prevStep} onNext={nextStep} nextLabel="预览部署" />
        </Card>
      )}

      {/* ── Step: Confirm ── */}
      {step === 'confirm' && (
        <Card title="确认部署" icon={<Rocket size={16} />}>
          <div className="space-y-3">
            <p className="text-sm text-white/50">即将执行以下操作：</p>

            {deployType === 'vps' && (
              <SummaryTable rows={[
                ['类型', 'VPS + gost SOCKS5+TLS'],
                ['服务器', `${ssh.host}:${ssh.port}`],
                ['节点域名', vpsConfig.node_prefix && selectedZone
                  ? `${vpsConfig.node_prefix}.${selectedZone.name}` : selectedZone?.name ?? '—'],
                ['CF Zone', selectedZone?.name ?? '—'],
                ['证书', 'Let\'s Encrypt DNS-01 自动申请'],
                ['gost 端口', '443 (TLS)'],
              ]} />
            )}

            {deployType === 'cf_workers' && (
              <SummaryTable rows={[
                ['类型', 'Cloudflare Workers'],
                ['Worker', workers.worker_name],
                ['自定义域名', workers.custom_domain],
                ['CF Zone', selectedZone?.name ?? '—'],
              ]} />
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
          onClick={onNext}
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

const VPS_PHASES = ['ssh_test', 'env_detect', 'provision', 'cert_dns', 'import', 'configure', 'verify']
const WORKERS_PHASES = ['cf_validate', 'worker_deploy', 'import', 'configure', 'verify']

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

// ── step label helper ──────────────────────────────────────────────────────────
function stepLabel(step: WizardStep): string {
  const labels: Record<WizardStep, string> = {
    select:         '类型选择',
    vps_connect:    'SSH 连接',
    cf_creds:       'CF 账号',
    workers_config: 'Workers 配置',
    vps_config:     '节点配置',
    confirm:        '确认',
    deploying:      '部署中',
    done:           '完成',
  }
  return labels[step] ?? step
}
