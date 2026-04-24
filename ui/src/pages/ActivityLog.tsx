import { useEffect, useRef, useState } from 'react'
import { getConnections, closeAllConns, getLogs } from '../api/client'
import type { Connection } from '../api/client'
import { useSSE } from '../hooks/useSSE'
import { formatBytes } from '../utils/format'
import { RefreshCw, Trash2 } from 'lucide-react'

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
          <span className="badge badge-muted">{conns.length}</span>
        </div>
        <div className="flex gap-2">
          <button className="btn-ghost flex items-center gap-2" onClick={refresh}>
            <RefreshCw size={14} /> 刷新
          </button>
          <button className="btn-ghost border-danger/40 text-danger hover:bg-danger/10 flex items-center gap-2" onClick={handleCloseAll}>
            <Trash2 size={14} /> 清理全部
          </button>
        </div>
      </div>

      <div className="glass-card overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/5 text-xs text-muted uppercase tracking-wider">
                <th className="px-4 py-3 text-left">目标</th>
                <th className="px-4 py-3 text-left">协议</th>
                <th className="px-4 py-3 text-left">代理链</th>
                <th className="px-4 py-3 text-right">上传</th>
                <th className="px-4 py-3 text-right">下载</th>
              </tr>
            </thead>
            <tbody>
              {loading && (
                <tr><td colSpan={5} className="px-4 py-8 text-center text-muted">加载中…</td></tr>
              )}
              {!loading && conns.length === 0 && (
                <tr><td colSpan={5} className="px-4 py-8 text-center text-muted">暂无活跃连接</td></tr>
              )}
              {conns.map(c => (
                <tr key={c.id} className="border-b border-white/5 hover:bg-white/3 transition-colors">
                  <td className="px-4 py-3 font-mono text-xs text-slate-200 max-w-xs truncate">
                    {c.metadata.host || c.metadata.sourceIP}:{c.metadata.destinationPort}
                  </td>
                  <td className="px-4 py-3">
                    <span className="badge badge-muted">{c.metadata.network || c.metadata.type}</span>
                  </td>
                  <td className="px-4 py-3 text-xs text-muted">{c.chains?.join(' → ')}</td>
                  <td className="px-4 py-3 text-right font-mono text-xs text-brand">{formatBytes(c.upload, '')}</td>
                  <td className="px-4 py-3 text-right font-mono text-xs text-success">{formatBytes(c.download, '')}</td>
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

const LEVEL_COLOR: Record<string, string> = {
  info:    'text-slate-300',
  debug:   'text-muted',
  warning: 'text-warning',
  warn:    'text-warning',
  error:   'text-danger',
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

  return (
    <div className="space-y-4" style={{ minHeight: '60vh' }}>
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <p className="text-sm text-muted">实时日志流，每 3 秒刷新 + SSE 推送。</p>
        <div className="flex items-center gap-2 flex-wrap">
          {['all', 'info', 'warning', 'error'].map(l => (
            <button
              key={l}
              onClick={() => setFilter(l)}
              className={`btn py-1.5 text-xs ${filter === l ? 'btn-primary' : 'btn-ghost'}`}
            >
              {l === 'all' ? '全部' : l.toUpperCase()}
            </button>
          ))}
          <label className="flex items-center gap-1.5 text-xs text-muted cursor-pointer px-2">
            <input type="checkbox" checked={autoScroll} onChange={e => setAutoScroll(e.target.checked)} className="accent-brand" />
            自动滚动
          </label>
          <button className="btn-ghost flex items-center gap-2 py-1.5" onClick={fetchLogs}>
            <RefreshCw size={13} /> 刷新
          </button>
          <button className="btn-ghost flex items-center gap-2 py-1.5" onClick={() => setLines([])}>
            <Trash2 size={13} /> 清空
          </button>
        </div>
      </div>

      <div className="glass-card overflow-hidden" style={{ height: '60vh' }}>
        <div className="h-full overflow-y-auto px-4 py-3 font-mono text-xs space-y-0.5">
          {filtered.length === 0 && (
            <p className="text-muted py-6 text-center">
              暂无日志。确认 ClashForge 服务已启动，日志每 3 秒自动刷新。
            </p>
          )}
          {filtered.map(l => (
            <div key={l.id} className="flex gap-3 hover:bg-white/3 px-1 py-0.5 rounded transition-colors">
              <span className="text-surface-3 flex-shrink-0 tabular-nums w-20">
                {l.ts ? new Date(l.ts * 1000).toLocaleTimeString('zh-CN') : '—'}
              </span>
              <span className={`flex-shrink-0 w-12 font-semibold uppercase ${LEVEL_COLOR[(l.level ?? '').toLowerCase()] ?? 'text-muted'}`}>
                {l.level ?? '—'}
              </span>
              <span className="text-slate-300 break-all">{l.msg ?? ''}</span>
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

  return (
    <div className="p-6 space-y-5 max-w-6xl mx-auto">
      <div>
        <p className="text-[11px] uppercase tracking-[0.24em] text-muted">Activity</p>
        <h1 className="text-base font-semibold text-white mt-1">活动日志</h1>
      </div>

      <div className="flex gap-2">
        <button
          className={`btn-ghost flex items-center gap-2 ${tab === 'connections' ? 'border-brand/40 text-white' : ''}`}
          onClick={() => setTab('connections')}
        >
          连接
        </button>
        <button
          className={`btn-ghost flex items-center gap-2 ${tab === 'logs' ? 'border-brand/40 text-white' : ''}`}
          onClick={() => setTab('logs')}
        >
          日志
        </button>
      </div>

      {tab === 'connections' && <ConnectionsPanel />}
      {tab === 'logs' && <LogsPanel />}
    </div>
  )
}
