import { useEffect, useRef, useState } from 'react'
import { getLogs } from '../api/client'
import { useSSE } from '../hooks/useSSE'
import { Trash2, RefreshCw } from 'lucide-react'

interface LogLine { id: number; level: string; msg: string; ts: number }

const LEVEL_COLOR: Record<string,string> = {
  info:    'text-slate-300',
  debug:   'text-muted',
  warning: 'text-warning',
  warn:    'text-warning',
  error:   'text-danger',
}

export function Logs() {
  const [lines, setLines] = useState<LogLine[]>([])
  const [filter, setFilter] = useState<string>('all')
  const [autoScroll, setAutoScroll] = useState(true)
  const bottomRef = useRef<HTMLDivElement>(null)
  const idRef = useRef(0)

  // Polling fallback (catches logs even when core is not running)
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

  // SSE for real-time new entries
  useSSE({
    onLog: (d) => {
      setLines(prev => [...prev, { id: idRef.current++, level: d.level ?? '', msg: d.msg ?? '', ts: d.ts ?? 0 }].slice(-500))
    }
  })

  useEffect(() => {
    if (autoScroll) bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [lines, autoScroll])

  const filtered = filter === 'all' ? lines : lines.filter(l => (l.level ?? '').toLowerCase() === filter)

  return (
    <div className="p-6 flex flex-col gap-5 max-w-6xl mx-auto" style={{ height: 'calc(100vh - 2rem)' }}>
      <div className="flex items-center justify-between flex-shrink-0">
        <div>
          <p className="text-[11px] uppercase tracking-[0.24em] text-muted">Logs</p>
          <h1 className="text-base font-semibold text-white mt-1">系统日志</h1>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          {['all','info','warning','error'].map(l => (
            <button key={l} onClick={() => setFilter(l)}
              className={`btn py-1.5 text-xs ${filter === l ? 'btn-primary' : 'btn-ghost'}`}>
              {l === 'all' ? '全部' : l.toUpperCase()}
            </button>
          ))}
          <label className="flex items-center gap-1.5 text-xs text-muted cursor-pointer px-2">
            <input type="checkbox" checked={autoScroll} onChange={e => setAutoScroll(e.target.checked)} className="accent-brand" />
            自动滚动
          </label>
          <button className="btn-ghost flex items-center gap-2 py-1.5" onClick={fetchLogs}>
            <RefreshCw size={13}/> 刷新
          </button>
          <button className="btn-ghost flex items-center gap-2 py-1.5" onClick={() => setLines([])}>
            <Trash2 size={13}/> 清空
          </button>
        </div>
      </div>

      <div className="glass-card flex-1 overflow-hidden min-h-0">
        <div className="h-full overflow-y-auto px-4 py-3 font-mono text-xs space-y-0.5">
          {lines.length === 0 && (
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
          <div ref={bottomRef}/>
        </div>
      </div>
    </div>
  )
}
