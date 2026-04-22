import { useEffect, useState } from 'react'
import { useStore } from '../store'
import { useSSE } from '../hooks/useSSE'
import {
  getHealthCheck,
  getStatus, startCore, stopCore, restartCore, reloadCore,
  triggerUpdateAll, getSubscriptions, getOverrides, generateConfig,
  getConfig, updateConfig,
} from '../api/client'
import type { HealthCheckData, HealthDNS, HealthPort, HealthProcess, HealthProxyTest, HealthTakeover, StatusData } from '../api/client'
import { formatBytes, formatUptime } from '../utils/format'
import {
  AreaChart, Area, XAxis, YAxis, ResponsiveContainer, Tooltip
} from 'recharts'
import { RefreshCw, RotateCcw, Zap, Download, Play, Square, AlertTriangle, CheckCircle2, Settings } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

// ── Config readiness check ─────────────────────────────────────────────────
interface ReadinessState {
  hasSubscriptions: boolean
  hasOverrides: boolean
  ready: boolean
  loading: boolean
}

function useReadiness() {
  const [state, setState] = useState<ReadinessState>({ hasSubscriptions: false, hasOverrides: false, ready: false, loading: true })
  const check = async () => {
    setState(s => ({ ...s, loading: true }))
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

// ── Setup guide (shown when no config) ────────────────────────────────────
function SetupGuide({ onDismiss }: { onDismiss: () => void }) {
  const navigate = useNavigate()
  return (
    <div className="card border-warning/30 bg-warning/5 px-6 py-5">
      <div className="flex gap-3 items-start">
        <AlertTriangle size={20} className="text-warning flex-shrink-0 mt-0.5" />
        <div className="flex-1">
          <p className="font-semibold text-white text-sm">安装完成！先配置再启动</p>
          <p className="text-muted text-xs mt-1 leading-5">
            在启动 Mihomo 核心前，请先添加至少一个代理配置。你可以通过以下任一方式配置：
          </p>
          <div className="mt-4 grid grid-cols-1 sm:grid-cols-2 gap-3">
            <button
              className="btn-primary flex items-center gap-2 justify-center py-3"
              onClick={() => navigate('/subscriptions')}
            >
              <span className="text-lg">📋</span>
              <div className="text-left">
                <div className="font-semibold text-sm">添加订阅链接</div>
                <div className="text-xs opacity-75">机场订阅 URL</div>
              </div>
            </button>
            <button
              className="btn-ghost flex items-center gap-2 justify-center py-3 border border-white/10"
              onClick={() => navigate('/settings')}
            >
              <span className="text-lg">📄</span>
              <div className="text-left">
                <div className="font-semibold text-sm">上传/粘贴 YAML</div>
                <div className="text-xs text-muted">直接使用已有 Clash 配置</div>
              </div>
            </button>
          </div>
          <button className="text-xs text-muted mt-3 hover:text-slate-300 underline" onClick={onDismiss}>
            跳过，我知道怎么做
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Core control panel ─────────────────────────────────────────────────────
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
  const isRunning = (state as string) === 'running'
  const isLoading = (name: string) => loading === name

  const stateColor = {
    running: 'bg-success',
    stopped: 'bg-danger',
    error: 'bg-danger',
    starting: 'bg-warning',
    stopping: 'bg-warning',
  }[(state as string)] ?? 'bg-muted'

  const stateLabel = {
    running: '运行中',
    stopped: '已停止',
    error: '错误',
    starting: '启动中',
    stopping: '停止中',
  }[(state as string)] ?? state

  return (
    <div className="card px-5 py-5">
      <div className="flex items-center justify-between mb-5">
        <h2 className="text-sm font-semibold text-slate-300">Mihomo 核心</h2>
        <div className="flex items-center gap-2">
          <span className={`w-2 h-2 rounded-full ${stateColor} ${(state as string) === 'running' ? 'animate-pulse' : ''}`}/>
          <span className={`text-sm font-semibold ${isRunning ? 'text-success' : 'text-danger'}`}>
            {stateLabel}
          </span>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3">
        {!isRunning ? (
          ready ? (
            <button
              className="btn-primary col-span-2 flex items-center justify-center gap-2 py-3 text-base"
              onClick={onStart}
              disabled={!!loading || (state as string) === 'starting'}
            >
              <Play size={16} className={isLoading('start') ? 'animate-pulse' : ''} />
              {isLoading('start') ? '启动中…' : '▶  启动核心'}
            </button>
          ) : (
            <button
              className="col-span-2 flex items-center justify-center gap-2 py-3 rounded-xl border border-dashed border-warning/40 bg-warning/5 text-warning text-sm font-medium hover:bg-warning/10 transition-all"
              onClick={onNavigateConfig}
            >
              <AlertTriangle size={15}/>
              请先配置代理信息，再启动
            </button>
          )
        ) : (
          <>
            <button
              className="btn-danger flex items-center justify-center gap-2 py-2.5"
              onClick={onStop}
              disabled={!!loading}
            >
              <Square size={14} className={isLoading('stop') ? 'animate-pulse' : ''} />
              {isLoading('stop') ? '停止中…' : '停止核心'}
            </button>
            <button
              className="btn-ghost flex items-center justify-center gap-2 py-2.5"
              onClick={onRestart}
              disabled={!!loading}
            >
              <RotateCcw size={14} className={isLoading('restart') ? 'animate-spin' : ''} />
              {isLoading('restart') ? '重启中…' : '重启'}
            </button>
            <button
              className="btn-ghost col-span-2 flex items-center justify-center gap-2 py-2.5"
              onClick={onReload}
              disabled={!!loading}
            >
              <RefreshCw size={14} className={isLoading('reload') ? 'animate-spin' : ''} />
              {isLoading('reload') ? '重载中…' : '热重载配置'}
            </button>
          </>
        )}
      </div>

      {ready && !isRunning && (
        <div className="flex items-center gap-1.5 mt-3 text-xs text-success">
          <CheckCircle2 size={12}/>
          <span>配置已就绪，可以启动</span>
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
    for (const k of path) cur = (cur[k] ?? '') as Record<string, unknown>
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
    } catch { /* ignore */ } finally { setSaving(false) }
  }

  const mode = get(['network', 'mode']) as string || 'none'
  const firewall = get(['network', 'firewall_backend']) as string || 'none'
  const applyOnStart = !!get(['network', 'apply_on_start'])
  const bypassLan = !!get(['network', 'bypass_lan'])
  const dnsEnable = !!get(['dns', 'enable'])
  const dnsApplyOnStart = !!get(['dns', 'apply_on_start'])

  return (
    <div className="card px-5 py-5">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-sm font-semibold text-slate-300">网络设置</h2>
        <div className="flex items-center gap-2">
          {saved && <span className="text-xs text-success">已保存</span>}
          {saving && <span className="text-xs text-muted">保存中…</span>}
          <button className="btn-ghost py-1 px-2 text-xs flex items-center gap-1.5" onClick={() => navigate('/settings')}>
            <Settings size={11}/> 高级
          </button>
        </div>
      </div>

      {status && (
        <div className="space-y-3 text-sm">
          <div className="flex items-center justify-between">
            <span className="text-muted">订阅数量</span>
            <span className="text-slate-200 font-medium">{status.subscriptions.enabled} / {status.subscriptions.total} 启用</span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-muted">版本</span>
            <span className="text-slate-200 font-medium">{status.metaclash.version}</span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-muted">运行时长</span>
            <span className="text-slate-200 font-medium">{formatUptime(status.metaclash.uptime)}</span>
          </div>

          {cfg && <>
            <div className="border-t border-white/5 pt-3 space-y-3">
              <div className="flex items-center justify-between">
                <span className="text-muted">透明代理模式</span>
                <select
                  value={mode}
                  onChange={e => set(['network', 'mode'], e.target.value)}
                  className="bg-surface-2 border border-white/10 rounded-lg px-2 py-1 text-xs text-white outline-none focus:border-brand"
                >
                  {['none', 'tproxy', 'redir', 'tun'].map(o => <option key={o} value={o}>{o}</option>)}
                </select>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted">防火墙后端</span>
                <select
                  value={firewall}
                  onChange={e => set(['network', 'firewall_backend'], e.target.value)}
                  className="bg-surface-2 border border-white/10 rounded-lg px-2 py-1 text-xs text-white outline-none focus:border-brand"
                >
                  {['none', 'auto', 'nftables', 'iptables'].map(o => <option key={o} value={o}>{o}</option>)}
                </select>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted">启动时接管透明代理</span>
                <Toggle checked={applyOnStart} onChange={v => set(['network', 'apply_on_start'], v)} />
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted">绕过局域网</span>
                <Toggle checked={bypassLan} onChange={v => set(['network', 'bypass_lan'], v)} />
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted">启用 DNS</span>
                <Toggle checked={dnsEnable} onChange={v => set(['dns', 'enable'], v)} />
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted">启动时接管 DNS</span>
                <Toggle checked={dnsApplyOnStart} onChange={v => set(['dns', 'apply_on_start'], v)} />
              </div>
            </div>
            <p className="text-xs text-muted leading-5 pt-1 border-t border-white/5">
              透明代理和 DNS 接管默认关闭。开启后需要重启 clashforge 服务，启动时才会应用。
            </p>
          </>}
        </div>
      )}
      {!status && <p className="text-muted text-sm">加载中…</p>}
    </div>
  )
}

function ToneDot({ ok }: { ok: boolean }) {
  return <span className={`w-2 h-2 rounded-full ${ok ? 'bg-success' : 'bg-danger'} flex-shrink-0`}/>
}

function HealthRow({ title, ok, detail }: { title: string; ok: boolean; detail: string }) {
  return (
    <div className="flex items-start justify-between gap-3 py-2 border-b border-white/5 last:border-b-0">
      <div className="flex items-start gap-2 min-w-0">
        <ToneDot ok={ok} />
        <div className="min-w-0">
          <p className="text-sm text-slate-200 font-medium">{title}</p>
          <p className="text-xs text-muted leading-5 break-words">{detail}</p>
        </div>
      </div>
      <span className={`text-xs font-semibold ${ok ? 'text-success' : 'text-danger'}`}>{ok ? '正常' : '异常'}</span>
    </div>
  )
}

function renderProcessDetail(item: HealthProcess) {
  const extras = [item.state, item.pid ? `PID ${item.pid}` : '', item.uptime ? `运行 ${formatUptime(item.uptime)}` : ''].filter(Boolean)
  return [item.message, extras.join(' · ')].filter(Boolean).join(' · ')
}

function renderPortDetail(item: HealthPort) {
  return `${item.proto.toUpperCase()} ${item.port} · ${item.message}${item.required ? ' · 必需' : ''}`
}

function renderTakeoverDetail(item: HealthTakeover) {
  const extras = [item.mode ? `mode=${item.mode}` : '', item.backend ? `backend=${item.backend}` : '', `启动接管=${item.apply_on_start ? '开' : '关'}`].filter(Boolean)
  return [item.message, extras.join(' · ')].filter(Boolean).join(' · ')
}

function renderDNSDetail(item: HealthDNS) {
  return `${item.message} · dnsmasq_mode=${item.dnsmasq_mode} · 启动接管=${item.apply_on_start ? '开' : '关'} · 监听=${item.listener_ready ? '正常' : '未就绪'}`
}

function renderProxyDetail(item: HealthProxyTest) {
  if (!item.listening) return `端口 ${item.port} 未监听`
  if (!item.ok) return `端口 ${item.port} 请求失败${item.error ? ` · ${item.error}` : ''}`
  const parts = [`端口 ${item.port}`]
  if (item.status_code) parts.push(`HTTP ${item.status_code}`)
  if (item.duration_ms) parts.push(`${item.duration_ms}ms`)
  return parts.join(' · ')
}

function HealthPanel({ health, onRefresh }: { health: HealthCheckData | null; onRefresh: () => void }) {
  if (!health) {
    return (
      <div className="card px-5 py-5">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-sm font-semibold text-slate-300">健康检查</h2>
          <button className="btn-ghost text-xs py-1.5 flex items-center gap-1.5" onClick={onRefresh}>
            <RefreshCw size={12}/> 刷新
          </button>
        </div>
        <p className="text-sm text-muted">加载中…</p>
      </div>
    )
  }

  return (
    <div className="card px-5 py-5 space-y-5">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold text-slate-300">健康检查</h2>
          <p className="text-xs text-muted mt-1">目标站点：{health.proxy_tests.target_url}</p>
        </div>
        <div className="flex items-center gap-3">
          <div className="text-right">
            <p className={`text-sm font-semibold ${health.summary.healthy ? 'text-success' : 'text-danger'}`}>{health.summary.healthy ? '整体正常' : '存在异常'}</p>
            <p className="text-xs text-muted">失败 {health.summary.failures} · 警告 {health.summary.warnings}</p>
          </div>
          <button className="btn-ghost text-xs py-1.5 flex items-center gap-1.5" onClick={onRefresh}>
            <RefreshCw size={12}/> 刷新
          </button>
        </div>
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
        <div className="rounded-2xl border border-white/8 bg-surface-1/60 px-4 py-4">
          <p className="text-xs text-muted uppercase tracking-wider mb-2">进程</p>
          <HealthRow title="clashforge" ok={health.process.clashforge.ok} detail={renderProcessDetail(health.process.clashforge)} />
          <HealthRow title="mihomo" ok={health.process.mihomo.ok} detail={renderProcessDetail(health.process.mihomo)} />
        </div>

        <div className="rounded-2xl border border-white/8 bg-surface-1/60 px-4 py-4">
          <p className="text-xs text-muted uppercase tracking-wider mb-2">接管状态</p>
          <HealthRow title="透明代理" ok={health.transparent_proxy.active || !health.transparent_proxy.apply_on_start} detail={renderTakeoverDetail(health.transparent_proxy)} />
          <HealthRow title="NFT / 防火墙" ok={health.nft.active || !health.nft.apply_on_start} detail={renderTakeoverDetail(health.nft)} />
          <HealthRow title="DNS" ok={health.dns.active || !health.dns.apply_on_start} detail={renderDNSDetail(health.dns)} />
        </div>

        <div className="rounded-2xl border border-white/8 bg-surface-1/60 px-4 py-4 xl:col-span-2">
          <p className="text-xs text-muted uppercase tracking-wider mb-2">端口监听</p>
          <div className="grid grid-cols-1 xl:grid-cols-2 gap-x-6">
            {health.ports.map(port => (
              <HealthRow key={`${port.name}-${port.port}`} title={port.name} ok={port.listening || !port.required} detail={renderPortDetail(port)} />
            ))}
          </div>
        </div>

        <div className="rounded-2xl border border-white/8 bg-surface-1/60 px-4 py-4 xl:col-span-2">
          <p className="text-xs text-muted uppercase tracking-wider mb-2">代理链路验证</p>
          <div className="grid grid-cols-1 xl:grid-cols-2 gap-x-6">
            <HealthRow title="HTTP 代理" ok={health.proxy_tests.http.ok} detail={renderProxyDetail(health.proxy_tests.http)} />
            <HealthRow title="Mixed 代理" ok={health.proxy_tests.mixed.ok} detail={renderProxyDetail(health.proxy_tests.mixed)} />
            <HealthRow title="SOCKS5 代理" ok={health.proxy_tests.socks.ok} detail={renderProxyDetail(health.proxy_tests.socks)} />
            <HealthRow title="Mihomo API" ok={health.proxy_tests.mihomo_api.ok} detail={renderProxyDetail(health.proxy_tests.mihomo_api)} />
          </div>
        </div>
      </div>
    </div>
  )
}

const CustomTooltip = ({ active, payload }: { active?: boolean; payload?: { value: number; name: string }[] }) => {
  if (!active || !payload?.length) return null
  return (
    <div className="bg-surface-2 border border-white/10 rounded-xl px-3 py-2 text-xs">
      {payload.map((p) => (
        <div key={p.name} className="flex gap-2">
          <span className={p.name === 'up' ? 'text-brand' : 'text-success'}>{p.name === 'up' ? '↑' : '↓'}</span>
          <span className="text-slate-200">{formatBytes(p.value, '/s')}</span>
        </div>
      ))}
    </div>
  )
}

export function Dashboard() {
  const { trafficHistory, currentUp, currentDown, connCount, coreState, setCoreState, pushTraffic, setConnCount } = useStore()
  const [status, setStatus] = useState<StatusData | null>(null)
  const [health, setHealth] = useState<HealthCheckData | null>(null)
  const [loading, setLoading] = useState<string | null>(null)
  const [guideDismissed, setGuideDismissed] = useState(false)
  const navigate = useNavigate()
  const readiness = useReadiness()

  useSSE({
    onCoreState: (d) => { setCoreState(d.state, d.pid); if (d.state === 'running') readiness.recheck() },
    onTraffic:   (d) => pushTraffic(d),
    onConnCount: (d) => setConnCount(d.total),
  })

  const refresh = async () => {
    const [nextStatus, nextHealth] = await Promise.all([
      getStatus().catch(() => null),
      getHealthCheck().catch(() => null),
    ])
    if (nextStatus) setStatus(nextStatus)
    if (nextHealth) setHealth(nextHealth)
  }
  useEffect(() => { refresh(); const t = setInterval(refresh, 5000); return () => clearInterval(t) }, [])

  const action = async (name: string, fn: () => Promise<unknown>) => {
    setLoading(name)
    try { await fn() } finally { setLoading(null); refresh() }
  }

  const state = status?.core.state ?? coreState
  const isRunning = (state as string) === 'running'
  const showGuide = !guideDismissed && !readiness.loading && !readiness.ready && !isRunning

  return (
    <div className="p-6 space-y-5 max-w-6xl mx-auto">
      <div className="flex items-center gap-3">
        <h1 className="text-lg font-semibold text-white">概览</h1>
        {status && (
          <span className="badge badge-muted font-mono text-xs">v{status.metaclash.version}</span>
        )}
      </div>

      {/* Setup guide */}
      {showGuide && <SetupGuide onDismiss={() => setGuideDismissed(true)} />}

      {/* stat cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="stat-card flex flex-col gap-1 min-w-0">
          <p className="text-xs text-muted font-medium uppercase tracking-wider">核心状态</p>
          <p className={`text-2xl font-bold tabular-nums ${isRunning ? 'text-success' : 'text-danger'}`}>{state}</p>
          {status && <p className="text-xs text-muted truncate">PID {status.core.pid} · 重启 {status.core.restarts} 次</p>}
        </div>
        <div className="stat-card flex flex-col gap-1"><p className="text-xs text-muted font-medium uppercase tracking-wider">上传速率</p><p className="text-2xl font-bold tabular-nums text-brand">{formatBytes(currentUp)}</p></div>
        <div className="stat-card flex flex-col gap-1"><p className="text-xs text-muted font-medium uppercase tracking-wider">下载速率</p><p className="text-2xl font-bold tabular-nums text-success">{formatBytes(currentDown)}</p></div>
        <div className="stat-card flex flex-col gap-1">
          <p className="text-xs text-muted font-medium uppercase tracking-wider">活跃连接</p>
          <p className="text-2xl font-bold tabular-nums">{connCount}</p>
          {status && <p className="text-xs text-muted">运行 {formatUptime(status.core.uptime)}</p>}
        </div>
      </div>

      {/* traffic chart */}
      <div className="card px-5 py-5">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-sm font-semibold text-slate-300">实时流量（最近 60s）</h2>
          <div className="flex gap-4 text-xs text-muted">
            <span className="flex items-center gap-1.5"><span className="w-2.5 h-1 rounded-full bg-brand inline-block"/><span>上传</span></span>
            <span className="flex items-center gap-1.5"><span className="w-2.5 h-1 rounded-full bg-success inline-block"/><span>下载</span></span>
          </div>
        </div>
        <ResponsiveContainer width="100%" height={160}>
          <AreaChart data={trafficHistory} margin={{ top: 0, right: 0, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id="gUp" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.3}/><stop offset="95%" stopColor="#3b82f6" stopOpacity={0}/>
              </linearGradient>
              <linearGradient id="gDown" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#22c55e" stopOpacity={0.3}/><stop offset="95%" stopColor="#22c55e" stopOpacity={0}/>
              </linearGradient>
            </defs>
            <XAxis dataKey="ts" hide />
            <YAxis tickFormatter={v => formatBytes(v, '')} width={64} tick={{ fontSize: 11, fill: '#94a3b8' }} />
            <Tooltip content={<CustomTooltip />} />
            <Area type="monotone" dataKey="up" stroke="#3b82f6" fill="url(#gUp)" strokeWidth={2} dot={false} isAnimationActive={false} />
            <Area type="monotone" dataKey="down" stroke="#22c55e" fill="url(#gDown)" strokeWidth={2} dot={false} isAnimationActive={false} />
          </AreaChart>
        </ResponsiveContainer>
      </div>

      {/* core control + network info */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <CoreControl
          state={state}
          loading={loading}
          ready={readiness.ready}
          onStart={() => action('start', startCore)}
          onStop={() => action('stop', stopCore)}
          onRestart={() => action('restart', restartCore)}
          onReload={() => action('reload', reloadCore)}
          onNavigateConfig={() => navigate('/subscriptions')}
        />

        <NetworkSettings status={status} onRefresh={refresh} />
      </div>

      <HealthPanel health={health} onRefresh={refresh} />

      {/* quick actions */}
      <div className="card px-5 py-4">
        <div className="flex items-center gap-3 flex-wrap">
          <span className="text-xs text-muted font-medium uppercase tracking-wider">快捷操作</span>
          <button className="btn-ghost flex items-center gap-1.5 text-xs py-1.5" onClick={() => action('update', triggerUpdateAll)} disabled={loading === 'update'}>
            <Download size={12} className={loading === 'update' ? 'animate-bounce' : ''}/> 更新所有订阅
          </button>
          <button className="btn-ghost flex items-center gap-1.5 text-xs py-1.5" onClick={() => action('generate', generateConfig)} disabled={loading === 'generate'}>
            <Zap size={12} className={loading === 'generate' ? 'animate-spin' : ''}/> 重新生成配置
          </button>
          <button className="btn-ghost flex items-center gap-1.5 text-xs py-1.5" onClick={refresh}>
            <RefreshCw size={12}/> 刷新
          </button>
        </div>
      </div>
    </div>
  )
}
