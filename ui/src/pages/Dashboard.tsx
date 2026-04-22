import { useEffect, useState } from 'react'
import { useStore } from '../store'
import { useSSE } from '../hooks/useSSE'
import { getStatus, startCore, stopCore, restartCore, reloadCore, triggerUpdateAll } from '../api/client'
import type { StatusData } from '../api/client'
import { formatBytes, formatUptime } from '../utils/format'
import {
  AreaChart, Area, XAxis, YAxis, ResponsiveContainer, Tooltip
} from 'recharts'
import { RefreshCw, RotateCcw, Zap, Download, Play, Square } from 'lucide-react'

function StatCard({ label, value, sub, color = 'text-white' }: {
  label: string; value: string; sub?: string; color?: string
}) {
  return (
    <div className="stat-card flex flex-col gap-1 min-w-0">
      <p className="text-xs text-muted font-medium uppercase tracking-wider">{label}</p>
      <p className={`text-2xl font-bold tabular-nums ${color}`}>{value}</p>
      {sub && <p className="text-xs text-muted truncate">{sub}</p>}
    </div>
  )
}

function CoreControl({ state, loading, onStart, onStop, onRestart, onReload }:{
  state: string
  loading: string | null
  onStart: () => void
  onStop: () => void
  onRestart: () => void
  onReload: () => void
}) {
  const isRunning = (state as string) === 'running'
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
    <div className="card px-5 py-5">
      <div className="flex items-center justify-between mb-5">
        <h2 className="text-sm font-semibold text-slate-300">Mihomo 核心</h2>
        <div className="flex items-center gap-2">
          <span className={`w-2 h-2 rounded-full ${stateColor} ${state === 'running' ? 'animate-pulse' : ''}`}/>
          <span className={`text-sm font-semibold ${isRunning ? 'text-success' : 'text-danger'}`}>
            {stateLabel}
          </span>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3">
        {!isRunning ? (
          <button
            className="btn-primary col-span-2 flex items-center justify-center gap-2 py-3"
            onClick={onStart}
            disabled={!!loading || state === 'starting'}
          >
            <Play size={16} className={isLoading('start') ? 'animate-pulse' : ''} />
            {isLoading('start') ? '启动中…' : '启动核心'}
          </button>
        ) : (
          <>
            <button
              className="btn-danger flex items-center justify-center gap-2 py-2.5"
              onClick={onStop}
          disabled={!!loading || (state as string) === 'stopping' || (state as string) === 'stopped'}
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

  useSSE({
    onCoreState: (d) => setCoreState(d.state, d.pid),
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
  const isRunning = state === 'running'

  return (
    <div className="p-6 space-y-6 max-w-6xl mx-auto">
      <h1 className="text-lg font-semibold text-white">概览</h1>

      {/* stat cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          label="核心状态"
          value={state}
          sub={status ? `PID ${status.core.pid} · 重启 ${status.core.restarts} 次` : '—'}
          color={isRunning ? 'text-success' : 'text-danger'}
        />
        <StatCard label="上传速率" value={formatBytes(currentUp)} color="text-brand" />
        <StatCard label="下载速率" value={formatBytes(currentDown)} color="text-success" />
        <StatCard
          label="活跃连接"
          value={String(connCount)}
          sub={status ? `运行 ${formatUptime(status.core.uptime)}` : '—'}
        />
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
                <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.3}/>
                <stop offset="95%" stopColor="#3b82f6" stopOpacity={0}/>
              </linearGradient>
              <linearGradient id="gDown" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#22c55e" stopOpacity={0.3}/>
                <stop offset="95%" stopColor="#22c55e" stopOpacity={0}/>
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
          onStart={() => action('start', startCore)}
          onStop={() => action('stop', stopCore)}
          onRestart={() => action('restart', restartCore)}
          onReload={() => action('reload', reloadCore)}
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
                ['metaclash 版本', status.metaclash.version],
                ['运行时长', formatUptime(status.metaclash.uptime)],
              ].map(([k, v]) => (
                <div key={k} className="flex justify-between">
                  <span className="text-muted">{k}</span>
                  <span className="text-slate-200 font-medium">{v}</span>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-muted text-sm">加载中…</p>
          )}
        </div>
      </div>

      {/* quick actions row */}
      <div className="card px-5 py-4">
        <div className="flex items-center gap-3 flex-wrap">
          <span className="text-xs text-muted font-medium uppercase tracking-wider">快捷操作</span>
          <button className="btn-ghost flex items-center gap-1.5 text-xs py-1.5" onClick={() => action('update', triggerUpdateAll)} disabled={loading === 'update'}>
            <Download size={12} className={loading === 'update' ? 'animate-bounce' : ''}/> 更新所有订阅
          </button>
          <button className="btn-ghost flex items-center gap-1.5 text-xs py-1.5" onClick={refresh}>
            <RefreshCw size={12}/> 刷新页面
          </button>
        </div>
      </div>
    </div>
  )
}
