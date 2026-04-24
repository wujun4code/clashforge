import { useEffect, useRef, useState, useCallback } from 'react'
import { getConnections, closeAllConns, getLogs } from '../api/client'
import type { Connection } from '../api/client'
import { useSSE } from '../hooks/useSSE'
import { formatBytes } from '../utils/format'
import { Activity, Copy, ListTree, RefreshCw, ScrollText, Trash2 } from 'lucide-react'
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

interface LogLine { id: number; level: string; msg: string; ts: number; fields?: Record<string, unknown> }

// Fields skipped from chip display (shown elsewhere or not useful per-row)
const SKIP_FIELDS = new Set(['side', 'component', 'stream', 'batch'])
// These fields are shown first in the chip list
const PRIORITY_FIELDS = ['phase', 'status', 'provider', 'name', 'target', 'ok', 'error', 'pid']

const LEVEL_BADGE: Record<string, string> = {
  info:    'text-sky-300 border-sky-500/30 bg-sky-500/10',
  debug:   'text-slate-400 border-slate-600/30 bg-slate-500/10',
  warning: 'text-amber-300 border-amber-500/30 bg-amber-500/10',
  warn:    'text-amber-300 border-amber-500/30 bg-amber-500/10',
  error:   'text-red-300 border-red-500/30 bg-red-500/10',
}

const SIDE_BADGE: Record<string, { label: string; cls: string }> = {
  router: { label: 'ROUTER', cls: 'text-blue-300 bg-blue-500/15 border-blue-500/30' },
  system: { label: 'SYSTEM', cls: 'text-purple-300 bg-purple-500/15 border-purple-500/30' },
}

const FIELD_CLS: Record<string, string> = {
  ok:   'text-emerald-300 bg-emerald-500/10 border-emerald-500/25',
  err:  'text-red-300 bg-red-500/10 border-red-500/25',
  warn: 'text-amber-300 bg-amber-500/10 border-amber-500/25',
  info: 'text-sky-300 bg-sky-500/10 border-sky-500/25',
  muted:'text-slate-400 bg-white/[0.04] border-white/[0.08]',
  dim:  'text-slate-600 bg-transparent border-transparent',
}

function fieldStyle(key: string, val: unknown): { text: string; variant: keyof typeof FIELD_CLS } {
  if (key === 'status') {
    const s = String(val)
    if (['ok', 'started', 'applied', 'configured', 'loaded'].includes(s)) return { text: s, variant: 'ok' }
    if (['failed', 'error'].includes(s)) return { text: s, variant: 'err' }
    if (['skipped', 'disabled'].includes(s)) return { text: s, variant: 'dim' }
    return { text: s, variant: 'muted' }
  }
  if (key === 'ok' || key === 'core_running' || key === 'transparent_proxy' || key === 'dns_redirect' || key === 'dns_engine') {
    return { text: val ? '✓ true' : '✗ false', variant: val ? 'ok' : 'err' }
  }
  if (key === 'fake_ip' && val === true)  return { text: 'fake-ip!', variant: 'warn' }
  if (key === 'error' && val)             return { text: String(val).slice(0, 70) + (String(val).length > 70 ? '…' : ''), variant: 'err' }
  if (key === 'phase')                    return { text: String(val), variant: 'info' }
  if (key === 'provider' || key === 'name' || key === 'target') return { text: String(val), variant: 'info' }
  if (key.endsWith('_ms') || key === 'latency') {
    const ms = Number(val)
    return { text: `${ms}ms`, variant: ms > 2000 ? 'warn' : ms > 500 ? 'muted' : 'ok' }
  }
  if (key === 'pid') return { text: String(val), variant: 'dim' }
  if (typeof val === 'boolean') return { text: val ? 'true' : 'false', variant: val ? 'ok' : 'muted' }
  if (Array.isArray(val))       return { text: (val as unknown[]).join(', '), variant: 'muted' }
  return { text: String(val), variant: 'muted' }
}

function FieldChip({ fieldKey, value }: { fieldKey: string; value: unknown }) {
  const { text, variant } = fieldStyle(fieldKey, value)
  return (
    <span className={`inline-flex items-center gap-0.5 rounded border px-1.5 py-0.5 text-[10px] font-mono leading-tight select-text ${FIELD_CLS[variant]}`}>
      <span className="opacity-50">{fieldKey.replace(/_/g, '·')}</span>
      <span className="opacity-30">=</span>
      <span className="font-semibold">{text}</span>
    </span>
  )
}

function LogRow({ entry }: { entry: LogLine }) {
  const [copied, setCopied] = useState(false)
  const level = (entry.level ?? '').toLowerCase()
  const side = entry.fields?.side as string | undefined
  const fields = entry.fields ?? {}

  const allKeys = Object.keys(fields).filter(k => !SKIP_FIELDS.has(k))
  const sortedKeys = [
    ...PRIORITY_FIELDS.filter(k => allKeys.includes(k)),
    ...allKeys.filter(k => !PRIORITY_FIELDS.includes(k)),
  ]

  const handleCopy = () => {
    const parts = [new Date(entry.ts * 1000).toISOString(), entry.level.toUpperCase(), entry.msg]
    for (const k of sortedKeys) parts.push(`${k}=${String(fields[k])}`)
    navigator.clipboard.writeText(parts.join('  ')).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }

  const timeStr = entry.ts
    ? new Date(entry.ts * 1000).toLocaleTimeString('zh-CN', { hour12: false })
    : '—'

  return (
    <div className="group flex flex-col gap-1 rounded-lg px-2 py-1.5 transition-colors hover:bg-white/[0.03]">
      {/* Header row: timestamp · level · side · message · copy */}
      <div className="flex items-center gap-1.5 flex-wrap min-w-0">
        <span className="text-slate-500 font-mono tabular-nums text-[11px] flex-shrink-0 w-[66px] select-text">
          {timeStr}
        </span>
        <span className={`rounded border px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider flex-shrink-0 ${LEVEL_BADGE[level] ?? 'text-slate-400 border-white/10 bg-white/5'}`}>
          {entry.level || '—'}
        </span>
        {side && SIDE_BADGE[side] && (
          <span className={`rounded border px-1.5 py-0.5 text-[10px] font-bold tracking-wider flex-shrink-0 ${SIDE_BADGE[side].cls}`}>
            {SIDE_BADGE[side].label}
          </span>
        )}
        <span className="text-slate-200 font-mono text-xs select-text min-w-0 truncate flex-1">
          {entry.msg ?? ''}
        </span>
        <button
          onClick={handleCopy}
          className="opacity-0 group-hover:opacity-100 transition-opacity p-1 rounded text-slate-600 hover:text-slate-300 hover:bg-white/5 flex-shrink-0 ml-auto"
          title="复制此行"
        >
          {copied
            ? <span className="text-[10px] text-emerald-400 font-mono">✓</span>
            : <Copy size={10} />}
        </button>
      </div>
      {/* Structured field chips */}
      {sortedKeys.length > 0 && (
        <div className="flex flex-wrap gap-1 pl-[74px]">
          {sortedKeys.map(k => <FieldChip key={k} fieldKey={k} value={fields[k]} />)}
        </div>
      )}
    </div>
  )
}

function LogsPanel() {
  const [lines, setLines] = useState<LogLine[]>([])
  const [filter, setFilter] = useState<string>('all')
  const [autoScroll, setAutoScroll] = useState(true)
  const bottomRef = useRef<HTMLDivElement>(null)
  const idRef = useRef(0)

  const toLine = (l: { level?: string; msg?: string; ts?: number; fields?: Record<string, unknown> }): LogLine => ({
    id: idRef.current++,
    level: l.level ?? '',
    msg: l.msg ?? '',
    ts: l.ts ?? 0,
    fields: l.fields,
  })

  const fetchLogs = useCallback(() => {
    getLogs('info', 200).then(data => {
      if (!data?.logs?.length) return
      setLines(data.logs.map(toLine))
    }).catch(() => null)
  }, [])

  useEffect(() => {
    fetchLogs()
    const t = setInterval(fetchLogs, 3000)
    return () => clearInterval(t)
  }, [fetchLogs])

  useSSE({
    onLog: (d) => {
      setLines(prev => [...prev, toLine(d)].slice(-500))
    },
  })

  useEffect(() => {
    if (autoScroll) bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [lines, autoScroll])

  const filtered = filter === 'all' ? lines : lines.filter(l => (l.level ?? '').toLowerCase() === filter)

  const handleCopyAll = useCallback(() => {
    const text = filtered.map(l => {
      const parts = [new Date(l.ts * 1000).toISOString(), l.level.toUpperCase(), l.msg]
      if (l.fields) for (const [k, v] of Object.entries(l.fields)) parts.push(`${k}=${String(v)}`)
      return parts.join('  ')
    }).join('\n')
    navigator.clipboard.writeText(text).catch(() => null)
  }, [filtered])

  // Build rows with batch-group separators
  const rows: React.ReactNode[] = []
  let lastBatch = ''
  for (const entry of filtered) {
    const batch = entry.fields?.batch as string | undefined
    if (batch && batch !== lastBatch) {
      if (lastBatch) {
        rows.push(
          <div key={`sep-${entry.id}`} className="flex items-center gap-2 my-1.5 px-2">
            <div className="h-px flex-1 bg-white/[0.05]" />
            <span className="text-[10px] text-slate-600 font-mono px-1">probe · {batch}</span>
            <div className="h-px flex-1 bg-white/[0.05]" />
          </div>
        )
      }
      lastBatch = batch
    }
    rows.push(<LogRow key={entry.id} entry={entry} />)
  }

  return (
    <div className="space-y-4" style={{ minHeight: '60vh' }}>
      <SectionCard
        title="实时日志流"
        description="每 3 秒轮询 · SSE 推送叠加 · 路由器侧诊断请求含完整链路字段与 batch 分组"
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
            <button className="btn-ghost flex items-center gap-2 py-1.5" onClick={handleCopyAll} title="复制全部可见日志">
              <Copy size={13} /> 复制全部
            </button>
            <button className="btn-ghost flex items-center gap-2 py-1.5" onClick={() => setLines([])}>
              <Trash2 size={13} /> 清空
            </button>
          </div>
        }
      >
        <div className="table-shell" style={{ height: '60vh' }}>
          <div className="h-full overflow-y-auto px-2 py-2 space-y-0">
            {rows.length === 0 && (
              <EmptyState
                title="还没有匹配到日志"
                description="确认 ClashForge 服务已启动，或调整日志级别与筛选条件后重新查看。"
                icon={<ScrollText size={18} />}
              />
            )}
            {rows}
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
