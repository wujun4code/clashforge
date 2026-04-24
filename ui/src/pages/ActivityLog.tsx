import { useEffect, useRef, useState } from 'react'
import { getConnections, closeAllConns, getLogs } from '../api/client'
import type { Connection } from '../api/client'
import { useSSE } from '../hooks/useSSE'
import { formatBytes } from '../utils/format'
import { Activity, ListTree, RefreshCw, ScrollText, Trash2 } from 'lucide-react'
import { EmptyState, PageHeader, SectionCard, SegmentedTabs } from '../components/ui'

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

      <div className="table-shell">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="table-header-row">
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
                <tr>
                  <td colSpan={5} className="px-4 py-8">
                    <EmptyState
                      title="暂无活跃连接"
                      description="当设备或应用开始通过代理链转发流量时，这里会实时显示目标、链路与吞吐量。"
                      icon={<ListTree size={18} />}
                    />
                  </td>
                </tr>
              )}
              {conns.map(c => (
                <tr key={c.id} className="table-row">
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
      <SectionCard
        title="实时日志流"
        description="每 3 秒轮询一次，并叠加 SSE 推送，适合快速观察服务波动与错误走势。"
        actions={
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
            <label className="flex min-h-[44px] items-center gap-1.5 text-xs text-muted cursor-pointer px-2">
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
        }
      >
        <div className="table-shell" style={{ height: '60vh' }}>
          <div className="h-full overflow-y-auto px-4 py-3 font-mono text-xs space-y-0.5">
            {filtered.length === 0 && (
              <EmptyState
                title="还没有匹配到日志"
                description="确认 ClashForge 服务已启动，或调整日志级别与筛选条件后重新查看。"
                icon={<ScrollText size={18} />}
              />
            )}
            {filtered.map(l => (
              <div key={l.id} className="flex gap-3 rounded-xl px-2 py-1.5 transition-colors hover:bg-white/3">
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
      </SectionCard>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

type Tab = 'connections' | 'logs'

export function ActivityLog() {
  const [tab, setTab] = useState<Tab>('connections')

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Activity"
        title="活动与日志中心"
        description="把活跃连接、代理链路与实时日志放进同一个观察面板，便于快速定位拥塞、异常与回源行为。"
        metrics={[
          { label: '视图', value: tab === 'connections' ? '连接面板' : '日志面板' },
          { label: '刷新', value: tab === 'connections' ? '2 秒轮询' : '3 秒 + SSE' },
        ]}
      />

      <SegmentedTabs
        items={[
          { value: 'connections', label: '连接', icon: <Activity size={14} />, hint: '查看实时连接与代理链' },
          { value: 'logs', label: '日志', icon: <ScrollText size={14} />, hint: '查看实时日志流与错误' },
        ]}
        value={tab}
        onChange={setTab}
      />

      {tab === 'connections' && (
        <SectionCard
          title="连接总览"
          description="实时查看每条连接的目标地址、协议类型、代理链与上下行吞吐。"
        >
          <ConnectionsPanel />
        </SectionCard>
      )}
      {tab === 'logs' && <LogsPanel />}
    </div>
  )
}
