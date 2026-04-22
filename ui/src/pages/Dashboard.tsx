import { useEffect, useState } from 'react'
import { useStore } from '../store'
import { useSSE } from '../hooks/useSSE'
import {
  getStatus, startCore, stopCore, restartCore, reloadCore,
  triggerUpdateAll, getSubscriptions, getOverrides
} from '../api/client'
import type { StatusData } from '../api/client'
import { formatBytes, formatUptime } from '../utils/format'
import {
  AreaChart, Area, XAxis, YAxis, ResponsiveContainer, Tooltip
} from 'recharts'
import { RefreshCw, RotateCcw, Zap, Download, Play, Square, AlertTriangle, CheckCircle2 } from 'lucide-react'
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
  const [loading, setLoading] = useState<string | null>(null)
  const [guideDismissed, setGuideDismissed] = useState(false)
  const navigate = useNavigate()
  const readiness = useReadiness()

  useSSE({
    onCoreState: (d) => { setCoreState(d.state, d.pid); if (d.state === 'running') readiness.recheck() },
    onTraffic:   (d) => pushTraffic(d),
    onConnCount: (d) => setConnCount(d.total),
  })

  const refresh = () => getStatus().then(setStatus).catch(() => null)
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
      <h1 className="text-lg font-semibold text-white">概览</h1>

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

        <div className="card px-5 py-5">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-sm font-semibold text-slate-300">网络状态</h2>
            <button className="btn-ghost py-1 px-2 text-xs flex items-center gap-1.5" onClick={refresh}>
              <Zap size={12}/> 刷新
            </button>
          </div>
          {status ? (
            <div className="space-y-2.5 text-sm">
              {[
                ['透明代理模式', status.network.mode],
                ['防火墙后端', status.network.firewall_backend],
                ['规则已应用', status.network.rules_applied ? '是' : '否'],
                ['订阅数量', `${status.subscriptions.enabled} / ${status.subscriptions.total} 启用`],
                ['版本', status.metaclash.version],
                ['运行时长', formatUptime(status.metaclash.uptime)],
              ].map(([k, v]) => (
                <div key={k} className="flex justify-between">
                  <span className="text-muted">{k}</span>
                  <span className="text-slate-200 font-medium">{v}</span>
                </div>
              ))}
            </div>
          ) : <p className="text-muted text-sm">加载中…</p>}
        </div>
      </div>

      {/* quick actions */}
      <div className="card px-5 py-4">
        <div className="flex items-center gap-3 flex-wrap">
          <span className="text-xs text-muted font-medium uppercase tracking-wider">快捷操作</span>
          <button className="btn-ghost flex items-center gap-1.5 text-xs py-1.5" onClick={() => action('update', triggerUpdateAll)} disabled={loading === 'update'}>
            <Download size={12} className={loading === 'update' ? 'animate-bounce' : ''}/> 更新所有订阅
          </button>
          <button className="btn-ghost flex items-center gap-1.5 text-xs py-1.5" onClick={refresh}>
            <RefreshCw size={12}/> 刷新
          </button>
        </div>
      </div>
    </div>
  )
}
