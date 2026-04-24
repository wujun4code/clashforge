import { useEffect, useRef, useState } from 'react'
import { getConnections, closeAllConns, getLogs } from '../api/client'
import type { Connection } from '../api/client'
import { useSSE } from '../hooks/useSSE'
import { formatBytes } from '../utils/format'
import { Activity, RefreshCw, Trash2, Wifi, Terminal } from 'lucide-react'

// ── Connections panel ─────────────────────────────────────────────────────────

function ConnectionsPanel() {
  const [conns, setConns] = useState<Connection[]>([])
  const [loading, setLoading] = useState(true)

  const refresh = () =>
    getConnections()
      .then(d => { setConns(d.connections ?? []); setLoading(false) })
      .catch(() => setLoading(false))

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 2000)
    return () => clearInterval(t)
  }, [])

  const handleCloseAll = async () => {
    await closeAllConns().catch(() => null)
    refresh()
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted">活跃连接</span>
          <span className="badge-brand">{conns.length}</span>
        </div>
        <div className="flex gap-2">
          <button className="btn-ghost flex items-center gap-2 text-xs" onClick={refresh}>
            <RefreshCw size={13} /> 刷新
          </button>
          <button className="btn-danger flex items-center gap-2 text-xs" onClick={handleCloseAll}>
            <Trash2 size={13} /> 清理全部
          </button>
        </div>
      </div>

      <div className="card overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.06]">
                {['目标', '协议', '代理链', '上传', '下载'].map((h, i) => (
                  <th key={h} className={`px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted ${i >= 3 ? 'text-right' : 'text-left'}`}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {loading && (
                <tr><td colSpan={5} className="px-4 py-10 text-center text-muted text-sm">
                  <div className="flex items-center justify-center gap-2">
                    <RefreshCw size={13} className="animate-spin" /> 加载中…
                  </div>
                </td></tr>
              )}
              {!loading && conns.length === 0 && (
                <tr><td colSpan={5} className="px-4 py-10 text-center text-muted text-sm">暂无活跃连接</td></tr>
              )}
              {conns.map(c => (
                <tr key={c.id} className="border-b border-white/[0.04] hover:bg-white/[0.02] transition-colors">
                  <td className="px-4 py-3 font-mono text-xs text-slate-200 max-w-xs truncate">
                    {c.metadata.host || c.metadata.sourceIP}:{c.metadata.destinationPort}
                  </td>
                  <td className="px-4 py-3">
                    <span className="badge-muted">{c.metadata.network || c.metadata.type}</span>
                  </td>
                  <td className="px-4 py-3 text-xs text-muted">{c.chains?.join(' → ')}</td>
                  <td className="px-4 py-3 text-right font-mono text-xs text-warning">{formatBytes(c.upload, '')}</td>
                  <td className="px-4 py-3 text-right font-mono text-xs text-brand">{formatBytes(c.download, '')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

// ── Logs panel ────────────────────────────────────────────────────────────────

interface LogLine { id: number; level: string; msg: string; ts: number }

const LEVEL_STYLE: Record<string, string> = {
  info:    'text-slate-400',
  debug:   'text-muted/60',
  warning: 'text-warning',
  warn:    'text-warning',
  error:   'text-danger',
}

const LEVEL_BADGE: Record<string, string> = {
  info:    'text-slate-400',
  debug:   'text-muted',
  warning: 'text-warning font-bold',
  warn:    'text-warning font-bold',
  error:   'text-danger font-bold',
}

function LogsPanel() {
  const [lines, setLines] = useState<LogLine[]>([])
  const [filter, setFilter] = useState<string>('all')
  const [autoScroll, setAutoScroll] = useState(true)
  const bottomRef = useRef<HTMLDivElement>(null)
  const idRef = useRef(0)

  const fetchLogs = () => {
    getLogs('info', 200).then(data => {
      if (!data?.logs?.length) return
      setLines(data.logs.map(l => ({ id: idRef.current++, level: l.level ?? '', msg: l.msg ?? '', ts: l.ts ?? 0 })))
    }).catch(() => null)
  }

  useEffect(() => {
    fetchLogs()
    const t = setInterval(fetchLogs, 3000)
    return () => clearInterval(t)
  }, [])

  useSSE({
    onLog: (d) => {
      setLines(prev => [...prev, { id: idRef.current++, level: d.level ?? '', msg: d.msg ?? '', ts: d.ts ?? 0 }].slice(-500))
    },
  })

  useEffect(() => {
    if (autoScroll) bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [lines, autoScroll])

  const filtered = filter === 'all' ? lines : lines.filter(l => (l.level ?? '').toLowerCase() === filter)

  const filterLabels = [
    { key: 'all', label: '全部' },
    { key: 'info', label: 'INFO' },
    { key: 'warning', label: 'WARN' },
    { key: 'error', label: 'ERROR' },
  ]

  return (
    <div className="space-y-4" style={{ minHeight: '60vh' }}>
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <p className="text-xs text-muted">实时日志流，每 3 秒刷新 + SSE 推送</p>
        <div className="flex items-center gap-2 flex-wrap">
          <div className="flex items-center bg-surface-2/60 rounded-xl border border-white/[0.07] p-1 gap-0.5">
            {filterLabels.map(({ key, label }) => (
              <button
                key={key}
                onClick={() => setFilter(key)}
                className={`px-3 py-1.5 text-xs font-semibold rounded-lg transition-all cursor-pointer ${
                  filter === key
                    ? 'bg-brand/20 text-brand border border-brand/30'
                    : 'text-muted hover:text-slate-300'
                }`}
              >
                {label}
              </button>
            ))}
          </div>
          <label className="flex items-center gap-1.5 text-xs text-muted cursor-pointer px-1">
            <input type="checkbox" checked={autoScroll} onChange={e => setAutoScroll(e.target.checked)} className="accent-brand" />
            自动滚动
          </label>
          <button className="btn-ghost flex items-center gap-1.5 text-xs py-1.5" onClick={fetchLogs}>
            <RefreshCw size={12} /> 刷新
          </button>
          <button className="btn-ghost flex items-center gap-1.5 text-xs py-1.5 text-danger hover:bg-danger/10" onClick={() => setLines([])}>
            <Trash2 size={12} /> 清空
          </button>
        </div>
      </div>

      <div className="card overflow-hidden" style={{ height: '60vh' }}>
        <div className="h-full overflow-y-auto px-3 py-3 font-mono text-xs space-y-px">
          {filtered.length === 0 && (
            <p className="text-muted py-8 text-center text-sm font-sans">
              暂无日志。确认 ClashForge 服务已启动，日志每 3 秒自动刷新。
            </p>
          )}
          {filtered.map(l => (
            <div key={l.id} className={`flex gap-3 px-2 py-1 rounded-lg hover:bg-white/[0.03] transition-colors ${LEVEL_STYLE[(l.level ?? '').toLowerCase()] ?? ''}`}>
              <span className="text-muted/50 flex-shrink-0 tabular-nums w-20">
                {l.ts ? new Date(l.ts * 1000).toLocaleTimeString('zh-CN') : '—'}
              </span>
              <span className={`flex-shrink-0 w-12 uppercase text-[10px] ${LEVEL_BADGE[(l.level ?? '').toLowerCase()] ?? 'text-muted'}`}>
                {l.level ?? '—'}
              </span>
              <span className="break-all leading-4">{l.msg ?? ''}</span>
            </div>
          ))}
          <div ref={bottomRef} />
        </div>
      </div>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

type Tab = 'connections' | 'logs'

export function ActivityLog() {
  const [tab, setTab] = useState<Tab>('connections')

  const tabs = [
    { key: 'connections' as Tab, icon: <Wifi size={14} />, label: '连接' },
    { key: 'logs' as Tab, icon: <Terminal size={14} />, label: '日志' },
  ]

  return (
    <div className="p-6 space-y-5 max-w-6xl mx-auto">
      <div className="flex items-center gap-3">
        <div className="w-8 h-8 rounded-xl bg-brand/15 border border-brand/20 flex items-center justify-center">
          <Activity size={15} className="text-brand" />
        </div>
        <h1 className="text-lg font-semibold text-white">活动</h1>
      </div>

      <div className="flex items-center bg-surface-1/80 rounded-2xl border border-white/[0.06] p-1.5 gap-1 w-fit">
        {tabs.map(({ key, icon, label }) => (
          <button
            key={key}
            onClick={() => setTab(key)}
            className={`flex items-center gap-2 px-4 py-2 rounded-xl text-sm font-medium transition-all cursor-pointer ${
              tab === key
                ? 'bg-brand/15 text-brand border border-brand/25'
                : 'text-muted hover:text-slate-300 hover:bg-white/[0.04]'
            }`}
          >
            {icon}{label}
          </button>
        ))}
      </div>

      {tab === 'connections' && <ConnectionsPanel />}
      {tab === 'logs' && <LogsPanel />}
    </div>
  )
}
