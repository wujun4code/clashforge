import { useEffect, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Cpu,
  Download,
  HardDrive,
  Play,
  RefreshCw,
  RotateCcw,
  Server,
  Settings,
  ShieldAlert,
  ShieldCheck,
  Square,
  Zap,
} from 'lucide-react'
import { Area, AreaChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'

import { useStore } from '../store'
import { useSSE } from '../hooks/useSSE'
import {
  generateConfig,
  getConfig,
  getOverview,
  getOverrides,
  getStatus,
  getSubscriptions,
  reloadCore,
  restartCore,
  startCore,
  stopCore,
  takeoverOverviewModule,
  triggerUpdateAll,
  updateConfig,
} from '../api/client'
import type {
  OverviewAccessCheck,
  OverviewData,
  OverviewIPCheck,
  OverviewInfluence,
  OverviewModule,
  OverviewPortOwner,
  StatusData,
} from '../api/client'
import { formatBytes, formatGB, formatMB, formatPercent, formatUptime } from '../utils/format'

interface ReadinessState {
  hasSubscriptions: boolean
  hasOverrides: boolean
  ready: boolean
  loading: boolean
}

interface NoticeState {
  tone: 'success' | 'error' | 'info'
  message: string
}

function useReadiness() {
  const [state, setState] = useState<ReadinessState>({ hasSubscriptions: false, hasOverrides: false, ready: false, loading: true })
  const check = async () => {
    setState((current) => ({ ...current, loading: true }))
    const [subs, overrides] = await Promise.all([
      getSubscriptions().catch(() => ({ subscriptions: [] })),
      getOverrides().catch(() => ({ content: '' })),
    ])
    const hasSubs = (subs.subscriptions?.length ?? 0) > 0
    const hasYaml = (overrides.content?.trim().length ?? 0) > 10
    setState({ hasSubscriptions: hasSubs, hasOverrides: hasYaml, ready: hasSubs || hasYaml, loading: false })
  }
  useEffect(() => { check() }, [])
  return { ...state, recheck: check }
}

function SetupGuide({ onDismiss }: { onDismiss: () => void }) {
  const navigate = useNavigate()
  return (
    <div className="card border-warning/30 bg-warning/5 px-6 py-5">
      <div className="flex gap-3 items-start">
        <AlertTriangle size={20} className="text-warning flex-shrink-0 mt-0.5" />
        <div className="flex-1">
          <p className="font-semibold text-white text-sm">先准备代理源，再看概览会更准确</p>
          <p className="text-muted text-xs mt-1 leading-5">
            概览页里的出口 IP、访问检查、透明代理接管结果都依赖可用节点。先添加订阅或 YAML，再启动核心，状态会更完整。
          </p>
          <div className="mt-4 grid grid-cols-1 sm:grid-cols-2 gap-3">
            <button className="btn-primary flex items-center gap-2 justify-center py-3" onClick={() => navigate('/subscriptions')}>
              <span className="text-lg">📋</span>
              <div className="text-left">
                <div className="font-semibold text-sm">添加订阅链接</div>
                <div className="text-xs opacity-75">导入机场订阅 URL</div>
              </div>
            </button>
            <button className="btn-ghost flex items-center gap-2 justify-center py-3 border border-white/10" onClick={() => navigate('/settings')}>
              <span className="text-lg">📄</span>
              <div className="text-left">
                <div className="font-semibold text-sm">上传 / 粘贴 YAML</div>
                <div className="text-xs text-muted">使用已有 Clash 配置</div>
              </div>
            </button>
          </div>
          <button className="text-xs text-muted mt-3 hover:text-slate-300 underline" onClick={onDismiss}>
            暂时关闭引导
          </button>
        </div>
      </div>
    </div>
  )
}

function CoreControl({ state, loading, ready, onStart, onStop, onRestart, onReload, onNavigateConfig }: {
  state: string
  loading: string | null
  ready: boolean
  onStart: () => void
  onStop: () => void
  onRestart: () => void
  onReload: () => void
  onNavigateConfig: () => void
}) {
  const isRunning = state === 'running'
  const isLoading = (name: string) => loading === name

  const stateColor = {
    running: 'bg-success',
    stopped: 'bg-danger',
    error: 'bg-danger',
    starting: 'bg-warning',
    stopping: 'bg-warning',
  }[state] ?? 'bg-muted'

  const stateLabel = {
    running: '运行中',
    stopped: '已停止',
    error: '错误',
    starting: '启动中',
    stopping: '停止中',
  }[state] ?? state

  return (
    <div className="card px-5 py-5 h-full">
      <div className="flex items-center justify-between mb-5">
        <div>
          <h2 className="text-sm font-semibold text-slate-200">Mihomo 核心</h2>
          <p className="text-xs text-muted mt-1">负责代理入口、规则匹配和节点切换。</p>
        </div>
        <div className="flex items-center gap-2">
          <span className={`w-2 h-2 rounded-full ${stateColor} ${isRunning ? 'animate-pulse' : ''}`}/>
          <span className={`text-sm font-semibold ${isRunning ? 'text-success' : 'text-danger'}`}>{stateLabel}</span>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3">
        {!isRunning ? (
          ready ? (
            <button className="btn-primary col-span-2 flex items-center justify-center gap-2 py-3 text-base" onClick={onStart} disabled={!!loading || state === 'starting'}>
              <Play size={16} className={isLoading('start') ? 'animate-pulse' : ''} />
              {isLoading('start') ? '启动中…' : '启动核心'}
            </button>
          ) : (
            <button className="col-span-2 flex items-center justify-center gap-2 py-3 rounded-xl border border-dashed border-warning/40 bg-warning/5 text-warning text-sm font-medium hover:bg-warning/10 transition-all" onClick={onNavigateConfig}>
              <AlertTriangle size={15}/>
              先准备节点，再启动核心
            </button>
          )
        ) : (
          <>
            <button className="btn-danger flex items-center justify-center gap-2 py-2.5" onClick={onStop} disabled={!!loading}>
              <Square size={14} className={isLoading('stop') ? 'animate-pulse' : ''} />
              {isLoading('stop') ? '停止中…' : '停止核心'}
            </button>
            <button className="btn-ghost flex items-center justify-center gap-2 py-2.5" onClick={onRestart} disabled={!!loading}>
              <RotateCcw size={14} className={isLoading('restart') ? 'animate-spin' : ''} />
              {isLoading('restart') ? '重启中…' : '重启'}
            </button>
            <button className="btn-ghost col-span-2 flex items-center justify-center gap-2 py-2.5" onClick={onReload} disabled={!!loading}>
              <RefreshCw size={14} className={isLoading('reload') ? 'animate-spin' : ''} />
              {isLoading('reload') ? '重载中…' : '热重载配置'}
            </button>
          </>
        )}
      </div>

      {ready && !isRunning && (
        <div className="flex items-center gap-1.5 mt-3 text-xs text-success">
          <CheckCircle2 size={12}/>
          <span>代理源已就绪，可以直接启动</span>
        </div>
      )}
    </div>
  )
}

function Toggle({ checked, onChange, disabled }: { checked: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <button
      onClick={() => !disabled && onChange(!checked)}
      disabled={disabled}
      className={`w-9 h-5 rounded-full transition-colors relative flex-shrink-0 ${checked ? 'bg-brand' : 'bg-surface-3'} ${disabled ? 'opacity-50 cursor-not-allowed' : ''}`}
    >
      <span className={`absolute top-0.5 w-4 h-4 bg-white rounded-full transition-transform ${checked ? 'left-4' : 'left-0.5'}`}/>
    </button>
  )
}

function NetworkSettings({ status, onRefresh }: { status: StatusData | null; onRefresh: () => void }) {
  const navigate = useNavigate()
  const [cfg, setCfg] = useState<Record<string, unknown> | null>(null)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  useEffect(() => { getConfig().then(setCfg).catch(() => null) }, [])

  const get = (path: string[]): unknown => {
    if (!cfg) return ''
    let cur = cfg as Record<string, unknown>
    for (const key of path) cur = (cur[key] ?? '') as Record<string, unknown>
    return cur
  }

  const set = async (path: string[], value: unknown) => {
    if (!cfg) return
    const updated = JSON.parse(JSON.stringify(cfg))
    let cur = updated
    for (let i = 0; i < path.length - 1; i++) cur = cur[path[i]] as Record<string, unknown>
    cur[path[path.length - 1]] = value
    setCfg(updated)
    setSaving(true)
    try {
      await updateConfig(updated)
      setSaved(true)
      setTimeout(() => setSaved(false), 1500)
      onRefresh()
    } catch {
      // ignore
    } finally {
      setSaving(false)
    }
  }

  const mode = get(['network', 'mode']) as string || 'none'
  const firewall = get(['network', 'firewall_backend']) as string || 'none'
  const applyOnStart = !!get(['network', 'apply_on_start'])
  const bypassLan = !!get(['network', 'bypass_lan'])
  const dnsEnable = !!get(['dns', 'enable'])
  const dnsApplyOnStart = !!get(['dns', 'apply_on_start'])

  return (
    <div className="card px-5 py-5 h-full">
      <div className="flex items-center justify-between mb-4">
        <div>
          <h2 className="text-sm font-semibold text-slate-200">接管偏好</h2>
          <p className="text-xs text-muted mt-1">决定 ClashForge 默认在启动时接管哪些能力。</p>
        </div>
        <div className="flex items-center gap-2">
          {saved && <span className="text-xs text-success">已保存</span>}
          {saving && <span className="text-xs text-muted">保存中…</span>}
          <button className="btn-ghost py-1 px-2 text-xs flex items-center gap-1.5" onClick={() => navigate('/settings')}>
            <Settings size={11}/> 高级
          </button>
        </div>
      </div>

      {status ? (
        <div className="space-y-3 text-sm">
          <div className="grid grid-cols-2 gap-3 rounded-2xl border border-white/5 bg-black/10 px-4 py-3">
            <div>
              <p className="text-[11px] uppercase tracking-wide text-muted">版本</p>
              <p className="text-slate-200 font-medium mt-1">{status.metaclash.version}</p>
            </div>
            <div>
              <p className="text-[11px] uppercase tracking-wide text-muted">节点来源</p>
              <p className="text-slate-200 font-medium mt-1">{status.subscriptions.enabled} / {status.subscriptions.total} 启用</p>
            </div>
          </div>

          {cfg && (
            <div className="border-t border-white/5 pt-3 space-y-3">
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted">透明代理模式</span>
                <select value={mode} onChange={e => set(['network', 'mode'], e.target.value)} className="bg-surface-2 border border-white/10 rounded-lg px-2 py-1 text-xs text-white outline-none focus:border-brand">
                  {['none', 'tproxy', 'redir', 'tun'].map(option => <option key={option} value={option}>{option}</option>)}
                </select>
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted">防火墙后端</span>
                <select value={firewall} onChange={e => set(['network', 'firewall_backend'], e.target.value)} className="bg-surface-2 border border-white/10 rounded-lg px-2 py-1 text-xs text-white outline-none focus:border-brand">
                  {['none', 'auto', 'nftables', 'iptables'].map(option => <option key={option} value={option}>{option}</option>)}
                </select>
              </div>
              <div className="flex items-center justify-between"><span className="text-muted">启动时接管透明代理</span><Toggle checked={applyOnStart} onChange={value => set(['network', 'apply_on_start'], value)} /></div>
              <div className="flex items-center justify-between"><span className="text-muted">绕过局域网</span><Toggle checked={bypassLan} onChange={value => set(['network', 'bypass_lan'], value)} /></div>
              <div className="flex items-center justify-between"><span className="text-muted">启用 DNS 引擎</span><Toggle checked={dnsEnable} onChange={value => set(['dns', 'enable'], value)} /></div>
              <div className="flex items-center justify-between"><span className="text-muted">启动时接管 DNS 入口</span><Toggle checked={dnsApplyOnStart} onChange={value => set(['dns', 'apply_on_start'], value)} /></div>
            </div>
          )}

          <p className="text-xs text-muted leading-5 pt-1 border-t border-white/5">
            概览页里的“让 ClashForge 接管”会优先沿用这里的偏好；如果仍是默认值，系统会采用更安全的推荐模式。
          </p>
        </div>
      ) : <p className="text-muted text-sm">加载中…</p>}
    </div>
  )
}

function SectionHeader({ eyebrow, title, description, action }: { eyebrow: string; title: string; description: string; action?: ReactNode }) {
  return (
    <div className="flex items-end justify-between gap-4 flex-wrap">
      <div>
        <p className="text-[11px] uppercase tracking-[0.24em] text-muted">{eyebrow}</p>
        <h2 className="text-xl font-semibold text-white mt-2">{title}</h2>
        <p className="text-sm text-muted mt-2 max-w-3xl leading-6">{description}</p>
      </div>
      {action}
    </div>
  )
}

function NoticeBanner({ notice }: { notice: NoticeState | null }) {
  if (!notice) return null
  const toneClass = {
    success: 'border-success/30 bg-success/10 text-success',
    error: 'border-danger/30 bg-danger/10 text-danger',
    info: 'border-brand/30 bg-brand/10 text-blue-200',
  }[notice.tone]
  return (
    <div className={`card px-4 py-3 border ${toneClass}`}>
      <p className="text-sm font-medium">{notice.message}</p>
    </div>
  )
}

function SummaryPill({ tone, label }: { tone: 'success' | 'warning' | 'danger' | 'muted'; label: string }) {
  const className = {
    success: 'border-success/25 bg-success/10 text-success',
    warning: 'border-warning/25 bg-warning/10 text-warning',
    danger: 'border-danger/25 bg-danger/10 text-danger',
    muted: 'border-white/10 bg-white/5 text-slate-300',
  }[tone]
  return <span className={`inline-flex items-center rounded-full border px-3 py-1 text-xs font-medium ${className}`}>{label}</span>
}

function HeroMetric({ title, value, detail }: { title: string; value: string; detail: string }) {
  return (
    <div className="rounded-2xl border border-white/8 bg-white/[0.03] px-4 py-3">
      <p className="text-[11px] uppercase tracking-[0.18em] text-muted">{title}</p>
      <p className="text-lg font-semibold text-white mt-2 truncate">{value}</p>
      <p className="text-xs text-muted mt-2 leading-5">{detail}</p>
    </div>
  )
}

function SummaryHero({ status, overview, currentUp, currentDown, connCount, onRefresh, onUpdateAll, onGenerate, loading }: {
  status: StatusData | null
  overview: OverviewData | null
  currentUp: number
  currentDown: number
  connCount: number
  onRefresh: () => void
  onUpdateAll: () => void
  onGenerate: () => void
  loading: string | null
}) {
  const isHealthy = !!overview?.summary.clashforge_healthy
  return (
    <div className="card overflow-hidden border-white/10">
      <div className="bg-gradient-to-br from-slate-900 via-slate-900 to-surface-1 px-6 py-6">
        <div className="flex items-start justify-between gap-6 flex-wrap">
          <div className="max-w-3xl">
            <div className="flex items-center gap-3 flex-wrap">
              <h1 className="text-2xl font-semibold text-white">概览</h1>
              {status && <span className="badge badge-muted font-mono">{status.metaclash.version}</span>}
              <SummaryPill tone={isHealthy ? 'success' : 'warning'} label={isHealthy ? '运行正常' : '需要处理'} />
            </div>
            <p className="text-sm text-slate-300/90 mt-3 leading-6">
              {overview?.summary.message ?? '正在加载 ClashForge 的运行环境、接管状态、出口信息和资源占用。'}
            </p>
            <div className="flex items-center gap-2 flex-wrap mt-4">
              <SummaryPill tone={overview?.summary.core_running ? 'success' : 'danger'} label={overview?.summary.core_running ? '核心已运行' : '核心未运行'} />
              <SummaryPill tone={(overview?.summary.conflict_count ?? 0) > 0 ? 'warning' : 'success'} label={`影响服务 ${(overview?.summary.conflict_count ?? 0)} 个`} />
              <SummaryPill tone={(overview?.summary.takeover_ready ?? 0) > 0 ? 'warning' : 'muted'} label={`待接管模块 ${(overview?.summary.takeover_ready ?? 0)} 个`} />
            </div>
          </div>

          <div className="flex items-center gap-2 flex-wrap">
            <button className="btn-ghost flex items-center gap-2" onClick={onRefresh} disabled={loading === 'refresh'}>
              <RefreshCw size={14} className={loading === 'refresh' ? 'animate-spin' : ''} />
              刷新
            </button>
            <button className="btn-ghost flex items-center gap-2" onClick={onUpdateAll} disabled={loading === 'update'}>
              <Download size={14} className={loading === 'update' ? 'animate-bounce' : ''} />
              更新订阅
            </button>
            <button className="btn-ghost flex items-center gap-2" onClick={onGenerate} disabled={loading === 'generate'}>
              <Zap size={14} className={loading === 'generate' ? 'animate-spin' : ''} />
              重新生成
            </button>
          </div>
        </div>

        <div className="grid grid-cols-2 xl:grid-cols-5 gap-3 mt-6">
          <HeroMetric title="活跃连接" value={`${connCount}`} detail="当前路由转发中的连接" />
          <HeroMetric title="上传速率" value={formatBytes(currentUp)} detail="实时上行流量" />
          <HeroMetric title="下载速率" value={formatBytes(currentDown)} detail="实时下行流量" />
          <HeroMetric title="运行时长" value={status ? formatUptime(status.metaclash.uptime) : '--'} detail="ClashForge 服务存活时间" />
          <HeroMetric title="规则状态" value={status?.network.rules_applied ? '已生效' : '未接管'} detail="透明代理规则是否已经落地" />
        </div>
      </div>
    </div>
  )
}

function UsageTile({ icon, title, value, detail }: { icon: ReactNode; title: string; value: string; detail: string }) {
  return (
    <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4">
      <div className="flex items-center gap-2 text-slate-300">
        {icon}
        <p className="text-sm font-medium">{title}</p>
      </div>
      <p className="text-lg font-semibold text-white mt-3">{value}</p>
      <p className="text-xs text-muted mt-2 leading-5">{detail}</p>
    </div>
  )
}

function MiniMetric({ title, value, mono = false }: { title: string; value: string; mono?: boolean }) {
  return (
    <div>
      <p className="text-[11px] uppercase tracking-[0.18em] text-muted">{title}</p>
      <p className={`text-sm text-slate-100 mt-2 ${mono ? 'font-mono text-xs break-all' : 'font-medium'}`}>{value}</p>
    </div>
  )
}

function trimMiddle(value: string, max: number): string {
  if (value.length <= max) return value
  const part = Math.floor((max - 1) / 2)
  return `${value.slice(0, part)}…${value.slice(-part)}`
}

function ResourceCard({ overview }: { overview: OverviewData | null }) {
  const system = overview?.resources.system
  const processes = overview?.resources.processes ?? []
  const app = overview?.resources.app

  return (
    <div className="card px-5 py-5 h-full">
      <SectionHeader eyebrow="Resources" title="资源占用" description="同时查看整机资源和 ClashForge / Mihomo 两个关键进程的 CPU、内存、磁盘占用。" />
      {!overview ? (
        <p className="text-sm text-muted mt-4">加载中…</p>
      ) : (
        <div className="space-y-5 mt-5">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <UsageTile icon={<Cpu size={16} />} title="整机 CPU" value={formatPercent(system?.cpu_percent ?? 0)} detail="过去一个采样窗口的整体 CPU 占用" />
            <UsageTile icon={<Activity size={16} />} title="整机内存" value={`${formatMB(system?.memory_used_mb ?? 0)} / ${formatMB(system?.memory_total_mb ?? 0)}`} detail={`已用 ${formatPercent(system?.memory_percent ?? 0)}`} />
            <UsageTile icon={<HardDrive size={16} />} title="系统磁盘" value={`${formatGB(system?.disk_used_gb ?? 0)} / ${formatGB(system?.disk_total_gb ?? 0)}`} detail={`已用 ${formatPercent(system?.disk_percent ?? 0)}`} />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {processes.map((process) => (
              <div key={process.id} className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <p className="text-sm font-semibold text-slate-100">{process.name}</p>
                    <p className="text-xs text-muted mt-1">PID {process.pid || '--'} · {process.running ? '运行中' : '未运行'}</p>
                  </div>
                  <SummaryPill tone={process.running ? 'success' : 'danger'} label={process.running ? '在线' : '离线'} />
                </div>
                <div className="grid grid-cols-2 gap-3 mt-4">
                  <MiniMetric title="CPU" value={formatPercent(process.cpu_percent)} />
                  <MiniMetric title="内存" value={formatMB(process.memory_rss_mb)} />
                  <MiniMetric title="运行时长" value={formatUptime(process.uptime)} />
                  <MiniMetric title="命令" value={process.command ? trimMiddle(process.command, 28) : '--'} mono />
                </div>
              </div>
            ))}
          </div>

          {app && (
            <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4">
              <div className="flex items-center gap-2 mb-3 text-slate-200">
                <Server size={16} />
                <p className="text-sm font-semibold">ClashForge 的磁盘空间占用</p>
              </div>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm">
                <MiniMetric title="运行时目录" value={formatMB(app.runtime_mb)} />
                <MiniMetric title="数据目录" value={formatMB(app.data_mb)} />
                <MiniMetric title="程序文件" value={formatMB(app.binary_mb)} />
                <MiniMetric title="总占用" value={formatMB(app.total_mb)} />
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

const CustomTooltip = ({ active, payload }: { active?: boolean; payload?: { value: number; name: string }[] }) => {
  if (!active || !payload?.length) return null
  return (
    <div className="bg-surface-2 border border-white/10 rounded-xl px-3 py-2 text-xs">
      {payload.map((item) => (
        <div key={item.name} className="flex gap-2">
          <span className={item.name === 'up' ? 'text-brand' : 'text-success'}>{item.name === 'up' ? '↑' : '↓'}</span>
          <span className="text-slate-200">{formatBytes(item.value, '/s')}</span>
        </div>
      ))}
    </div>
  )
}

function TrafficCard({ trafficHistory }: { trafficHistory: { ts: number | string; up: number; down: number }[] }) {
  return (
    <div className="card px-5 py-5">
      <SectionHeader eyebrow="Traffic" title="实时流量" description="最近 60 秒的上传 / 下载趋势，用来判断核心是否正在实际转发流量。" />
      <div className="mt-5">
        <ResponsiveContainer width="100%" height={180}>
          <AreaChart data={trafficHistory} margin={{ top: 0, right: 0, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id="gUp" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#38bdf8" stopOpacity={0.28}/>
                <stop offset="95%" stopColor="#38bdf8" stopOpacity={0}/>
              </linearGradient>
              <linearGradient id="gDown" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#22c55e" stopOpacity={0.28}/>
                <stop offset="95%" stopColor="#22c55e" stopOpacity={0}/>
              </linearGradient>
            </defs>
            <XAxis dataKey="ts" hide />
            <YAxis tickFormatter={(value) => formatBytes(value, '')} width={64} tick={{ fontSize: 11, fill: '#94a3b8' }} />
            <Tooltip content={<CustomTooltip />} />
            <Area type="monotone" dataKey="up" stroke="#38bdf8" fill="url(#gUp)" strokeWidth={2} dot={false} isAnimationActive={false} />
            <Area type="monotone" dataKey="down" stroke="#22c55e" fill="url(#gDown)" strokeWidth={2} dot={false} isAnimationActive={false} />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}

function ProbeGrid<T>({ title, eyebrow, description, checks, renderItem }: { title: string; eyebrow: string; description: string; checks: T[]; renderItem: (item: T) => ReactNode }) {
  return (
    <div className="card px-5 py-5 h-full">
      <SectionHeader eyebrow={eyebrow} title={title} description={description} />
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3 mt-5">
        {checks.map((item) => renderItem(item))}
      </div>
    </div>
  )
}

function IPCard({ item }: { item: OverviewIPCheck }) {
  return (
    <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4 min-h-[140px]">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm font-semibold text-slate-100">{item.provider}</p>
        <SummaryPill tone={item.ok ? 'success' : 'danger'} label={item.ok ? '已解析' : '失败'} />
      </div>
      {item.ok ? (
        <>
          <p className="text-xl font-semibold text-white mt-4">{item.ip}</p>
          <p className="text-sm text-muted mt-2 leading-6">{item.location || '未返回位置信息'}</p>
        </>
      ) : (
        <p className="text-sm text-danger mt-4 leading-6">{item.error || '无法获取出口 IP'}</p>
      )}
    </div>
  )
}

function AccessCard({ item }: { item: OverviewAccessCheck }) {
  return (
    <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4 min-h-[160px]">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-sm font-semibold text-slate-100">{item.name}</p>
          <p className="text-xs text-muted mt-1">{item.description}</p>
        </div>
        <SummaryPill tone={item.ok ? 'success' : 'danger'} label={item.ok ? '连接正常' : '访问失败'} />
      </div>
      <div className="mt-4 space-y-2">
        <p className="text-sm text-slate-200">{item.ok ? `${item.latency_ms ?? 0} ms` : (item.error || '请求失败')}</p>
        <p className="text-xs text-muted leading-5">{item.via}</p>
        <p className="text-xs text-muted break-all">{item.url}</p>
      </div>
    </div>
  )
}

function PortChip({ port }: { port: OverviewPortOwner }) {
  return (
    <span className="inline-flex items-center rounded-full border border-white/10 bg-white/5 px-3 py-1 text-xs text-slate-300">
      {port.proto.toUpperCase()} {port.port} · {port.owner}
    </span>
  )
}

function ModuleCard({ module, loading, onTakeover }: { module: OverviewModule; loading: boolean; onTakeover: (module: OverviewModule) => void }) {
  const tone = module.managed_by_clashforge ? 'success' : module.status === 'conflict' ? 'warning' : 'muted'
  const icon = module.managed_by_clashforge ? <ShieldCheck size={16} className="text-success" /> : <ShieldAlert size={16} className="text-warning" />
  const canTakeover = module.takeover_supported && !!module.action && !module.managed_by_clashforge

  return (
    <div className="rounded-[24px] border border-white/8 bg-surface-1/70 px-5 py-5 flex flex-col gap-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-slate-100">
            {icon}
            <h3 className="text-base font-semibold">{module.title}</h3>
          </div>
          <p className="text-xs text-muted mt-2 uppercase tracking-[0.18em]">{module.category}</p>
        </div>
        <SummaryPill tone={tone} label={module.managed_by_clashforge ? '已接管' : module.status === 'conflict' ? '存在占用' : '可接管'} />
      </div>

      <div className="rounded-2xl border border-white/6 bg-black/10 px-4 py-3">
        <p className="text-[11px] uppercase tracking-[0.18em] text-muted">当前由谁负责</p>
        <p className="text-sm text-slate-100 mt-2 leading-6">{module.current_owner}</p>
      </div>

      <div>
        <p className="text-[11px] uppercase tracking-[0.18em] text-muted">这个模块是做什么的</p>
        <p className="text-sm text-slate-200 mt-2 leading-6">{module.purpose}</p>
      </div>

      <div>
        <p className="text-[11px] uppercase tracking-[0.18em] text-muted">接管后的效果</p>
        <p className="text-sm text-slate-200 mt-2 leading-6">{module.takeover_effect}</p>
      </div>

      {(module.current_mode || module.recommended_mode) && (
        <div className="flex items-center gap-2 flex-wrap">
          {module.current_mode && <SummaryPill tone="muted" label={`当前模式 ${module.current_mode}`} />}
          {module.recommended_mode && <SummaryPill tone="warning" label={`推荐模式 ${module.recommended_mode}`} />}
        </div>
      )}

      {(module.processes?.length || module.ports?.length) ? (
        <div className="grid grid-cols-1 gap-3">
          {!!module.processes?.length && (
            <div>
              <p className="text-[11px] uppercase tracking-[0.18em] text-muted">相关进程</p>
              <div className="flex flex-wrap gap-2 mt-2">
                {module.processes.slice(0, 4).map((process) => (
                  <span key={`${module.id}-${process.pid}`} className="inline-flex items-center rounded-full border border-white/10 bg-white/5 px-3 py-1 text-xs text-slate-300">
                    {process.name}#{process.pid}
                  </span>
                ))}
              </div>
            </div>
          )}
          {!!module.ports?.length && (
            <div>
              <p className="text-[11px] uppercase tracking-[0.18em] text-muted">相关端口</p>
              <div className="flex flex-wrap gap-2 mt-2">
                {module.ports.slice(0, 6).map((port) => (
                  <PortChip key={`${module.id}-${port.proto}-${port.port}-${port.pid || 0}`} port={port} />
                ))}
              </div>
            </div>
          )}
        </div>
      ) : null}

      {!!module.notes?.length && (
        <div className="rounded-2xl border border-white/6 bg-black/10 px-4 py-3 space-y-2">
          {module.notes.slice(0, 3).map((note) => (
            <p key={`${module.id}-${note}`} className="text-xs text-muted leading-5">{note}</p>
          ))}
        </div>
      )}

      {module.takeover_supported && (
        <div className="pt-1">
          {canTakeover ? (
            <button className="btn-primary w-full flex items-center justify-center gap-2" onClick={() => onTakeover(module)} disabled={loading}>
              {loading ? <RefreshCw size={14} className="animate-spin" /> : <Zap size={14} />}
              {loading ? '接管中…' : (module.action?.label || '让 ClashForge 接管')}
            </button>
          ) : (
            <div className="rounded-xl border border-success/20 bg-success/10 px-4 py-3 text-sm text-success font-medium">
              这个模块当前已经由 ClashForge 接管
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function InfluencePanel({ influences }: { influences: OverviewInfluence[] }) {
  const running = influences.filter((item) => item.running)
  return (
    <div className="card px-5 py-5">
      <SectionHeader eyebrow="Influences" title="影响 ClashForge 的其他服务" description="这里列出当前会占用 NFT、DNS、透明代理端口，或者会直接影响 ClashForge 运行结果的其他服务 / 进程。" />
      {running.length === 0 ? (
        <div className="rounded-2xl border border-success/15 bg-success/10 px-4 py-4 mt-5 text-sm text-success">
          当前没有检测到明显的外部占用服务，ClashForge 可以独立管理透明代理、DNS 和本地控制面板。
        </div>
      ) : (
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-3 mt-5">
          {running.map((influence) => (
            <div key={influence.id} className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-sm font-semibold text-slate-100">{influence.name}</p>
                  <p className="text-xs text-muted mt-2 leading-5">{influence.description}</p>
                </div>
                <SummaryPill tone={influence.stoppable ? 'warning' : 'muted'} label={influence.stoppable ? '可停止' : '需手动处理'} />
              </div>
              <div className="flex flex-wrap gap-2 mt-4">
                {influence.affects.map((item) => <SummaryPill key={`${influence.id}-${item}`} tone="muted" label={item} />)}
              </div>
              {!!influence.processes?.length && (
                <div className="mt-4">
                  <p className="text-[11px] uppercase tracking-[0.18em] text-muted">进程</p>
                  <div className="flex flex-wrap gap-2 mt-2">
                    {influence.processes.map((process) => (
                      <span key={`${influence.id}-${process.pid}`} className="inline-flex items-center rounded-full border border-white/10 bg-white/5 px-3 py-1 text-xs text-slate-300">
                        {process.name}#{process.pid}
                      </span>
                    ))}
                  </div>
                </div>
              )}
              {!!influence.ports?.length && (
                <div className="mt-4">
                  <p className="text-[11px] uppercase tracking-[0.18em] text-muted">占用端口</p>
                  <div className="flex flex-wrap gap-2 mt-2">
                    {influence.ports.map((port) => (
                      <PortChip key={`${influence.id}-${port.proto}-${port.port}-${port.pid || 0}`} port={port} />
                    ))}
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export function Dashboard() {
  const { trafficHistory, currentUp, currentDown, connCount, coreState, setCoreState, pushTraffic, setConnCount } = useStore()
  const [status, setStatus] = useState<StatusData | null>(null)
  const [overview, setOverview] = useState<OverviewData | null>(null)
  const [loading, setLoading] = useState<string | null>(null)
  const [guideDismissed, setGuideDismissed] = useState(false)
  const [notice, setNotice] = useState<NoticeState | null>(null)
  const navigate = useNavigate()
  const readiness = useReadiness()

  useSSE({
    onCoreState: (data) => { setCoreState(data.state, data.pid); if (data.state === 'running') readiness.recheck() },
    onTraffic: (data) => pushTraffic(data),
    onConnCount: (data) => setConnCount(data.total),
  })

  const refresh = async () => {
    const [nextStatus, nextOverview] = await Promise.all([
      getStatus().catch(() => null),
      getOverview().catch(() => null),
    ])
    if (nextStatus) setStatus(nextStatus)
    if (nextOverview) setOverview(nextOverview)
  }

  useEffect(() => {
    refresh()
    const timer = setInterval(refresh, 6000)
    return () => clearInterval(timer)
  }, [])

  const action = async (name: string, fn: () => Promise<unknown>, successMessage?: string) => {
    setLoading(name)
    try {
      await fn()
      if (successMessage) setNotice({ tone: 'success', message: successMessage })
    } catch (error) {
      const message = error instanceof Error ? error.message : '操作失败'
      setNotice({ tone: 'error', message })
    } finally {
      setLoading(null)
      refresh()
    }
  }

  const handleTakeover = async (module: OverviewModule) => {
    if (!module.action || module.managed_by_clashforge) return
    setLoading(`takeover:${module.id}`)
    try {
      const result = await takeoverOverviewModule({
        module: module.action.module,
        mode: module.action.mode,
        stop_services: module.action.stop_services,
      })
      setOverview(result.overview)
      const suffix = result.stopped?.length ? `；已停止 ${result.stopped.join('、')}` : ''
      setNotice({ tone: 'success', message: `${result.message}${suffix}` })
    } catch (error) {
      const message = error instanceof Error ? error.message : '接管失败'
      setNotice({ tone: 'error', message })
    } finally {
      setLoading(null)
      const nextStatus = await getStatus().catch(() => null)
      if (nextStatus) setStatus(nextStatus)
    }
  }

  const state = status?.core.state ?? coreState
  const isRunning = state === 'running'
  const showGuide = !guideDismissed && !readiness.loading && !readiness.ready && !isRunning

  return (
    <div className="p-6 space-y-6 max-w-7xl mx-auto">
      <SummaryHero
        status={status}
        overview={overview}
        currentUp={currentUp}
        currentDown={currentDown}
        connCount={connCount}
        onRefresh={() => action('refresh', async () => { await refresh() })}
        onUpdateAll={() => action('update', triggerUpdateAll, '已触发订阅更新')}
        onGenerate={() => action('generate', generateConfig, '已重新生成运行时配置')}
        loading={loading}
      />

      <NoticeBanner notice={notice} />

      {showGuide && <SetupGuide onDismiss={() => setGuideDismissed(true)} />}

      <div className="grid grid-cols-1 xl:grid-cols-3 gap-4">
        <CoreControl
          state={state}
          loading={loading}
          ready={readiness.ready}
          onStart={() => action('start', startCore, 'Mihomo 核心已启动')}
          onStop={() => action('stop', stopCore, 'Mihomo 核心已停止')}
          onRestart={() => action('restart', restartCore, 'Mihomo 核心已重启')}
          onReload={() => action('reload', reloadCore, '配置已热重载')}
          onNavigateConfig={() => navigate('/subscriptions')}
        />
        <NetworkSettings status={status} onRefresh={refresh} />
        <ResourceCard overview={overview} />
      </div>

      <TrafficCard trafficHistory={trafficHistory} />

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
        <ProbeGrid
          eyebrow="Identity"
          title="出口 IP 地址"
          description="通过 ClashForge 的 mixed 入口向多个公共服务查询出口 IP，用来判断当前实际出站身份。"
          checks={overview?.ip_checks ?? []}
          renderItem={(item: OverviewIPCheck) => <IPCard key={item.provider} item={item} />}
        />
        <ProbeGrid
          eyebrow="Connectivity"
          title="访问检查"
          description="把常见站点做成直观卡片，直接看国内 / 国际网站是否已经通过 ClashForge 打通。"
          checks={overview?.access_checks ?? []}
          renderItem={(item: OverviewAccessCheck) => <AccessCard key={item.name} item={item} />}
        />
      </div>

      <div className="card px-5 py-5">
        <SectionHeader
          eyebrow="Takeover"
          title="服务接管中心"
          description="这里不仅告诉你这些模块是干什么的、现在由谁负责，还允许你直接让 ClashForge 接管透明代理、NFT 规则和 DNS 入口。"
          action={<p className="text-xs text-muted">刷新时间 {overview?.checked_at ? new Date(overview.checked_at).toLocaleTimeString() : '--'}</p>}
        />
        {!overview ? (
          <p className="text-sm text-muted mt-5">加载中…</p>
        ) : (
          <div className="grid grid-cols-1 xl:grid-cols-2 gap-4 mt-5">
            {overview.modules.map((module) => (
              <ModuleCard key={module.id} module={module} loading={loading === `takeover:${module.id}`} onTakeover={handleTakeover} />
            ))}
          </div>
        )}
      </div>

      <InfluencePanel influences={overview?.influences ?? []} />
    </div>
  )
}