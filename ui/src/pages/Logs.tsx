import { useEffect, useRef, useState } from 'react'
import { useSSE } from '../hooks/useSSE'
import { Trash2 } from 'lucide-react'

interface LogLine { level: string; msg: string; ts: number; id: number }

const LEVEL_COLOR: Record<string,string> = {
  info:    'text-slate-300',
  debug:   'text-muted',
  warning: 'text-warning',
  warn:    'text-warning',
  error:   'text-danger',
}

let idSeq = 0

export function Logs() {
  const [lines, setLines] = useState<LogLine[]>([])
  const [filter, setFilter] = useState<string>('all')
  const [autoScroll, setAutoScroll] = useState(true)
  const bottomRef = useRef<HTMLDivElement>(null)

  useSSE({
    onLog: (d) => {
      setLines(prev => [...prev, { ...d, id: idSeq++ }].slice(-500))
    }
  })

  useEffect(() => {
    if (autoScroll) bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [lines, autoScroll])

  const filtered = filter === 'all' ? lines : lines.filter(l => l.level === filter)

  return (
    <div className="p-6 flex flex-col gap-5 h-full max-w-6xl mx-auto" style={{ maxHeight: 'calc(100vh - 2rem)' }}>
      <div className="flex items-center justify-between flex-shrink-0">
        <h1 className="text-lg font-semibold text-white">日志</h1>
        <div className="flex items-center gap-2">
          {['all','info','warning','error'].map(l => (
            <button key={l} onClick={() => setFilter(l)}
              className={`btn py-1.5 text-xs ${filter === l ? 'btn-primary' : 'btn-ghost'}`}>
              {l === 'all' ? '全部' : l.toUpperCase()}
            </button>
          ))}
          <label className="flex items-center gap-1.5 text-xs text-muted cursor-pointer px-3">
            <input type="checkbox" checked={autoScroll} onChange={e => setAutoScroll(e.target.checked)} className="accent-brand" />
            自动滚动
          </label>
          <button className="btn-ghost flex items-center gap-2 py-1.5" onClick={() => setLines([])}>
            <Trash2 size={13}/> 清空
          </button>
        </div>
      </div>

      <div className="card flex-1 overflow-hidden">
        <div className="h-full overflow-y-auto px-4 py-3 font-mono text-xs space-y-0.5">
          {lines.length === 0 && (
            <p className="text-muted py-4 text-center">等待日志… (需要 SSE 连接)</p>
          )}
          {filtered.map(l => (
            <div key={l.id} className="flex gap-3 hover:bg-white/3 px-1 py-0.5 rounded transition-colors">
              <span className="text-surface-3 flex-shrink-0 tabular-nums w-20">
                {new Date(l.ts * 1000).toLocaleTimeString('zh-CN')}
              </span>
              <span className={`flex-shrink-0 w-12 font-semibold uppercase ${LEVEL_COLOR[l.level] ?? 'text-muted'}`}>
                {l.level}
              </span>
              <span className="text-slate-300 break-all">{l.msg}</span>
            </div>
          ))}
          <div ref={bottomRef}/>
        </div>
      </div>
    </div>
  )
}
