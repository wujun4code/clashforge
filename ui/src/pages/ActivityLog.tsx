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
          <span className="font-mono text-[10px] uppercase tracking-[0.15em] text-muted">ACTIVE_CONNS</span>
          <span
            className="font-mono text-xs font-bold px-2 py-0.5"
            style={{ color: '#00F5FF', border: '1px solid rgba(0,245,255,0.25)', background: 'rgba(0,245,255,0.08)', textShadow: '0 0 6px rgba(0,245,255,0.5)' }}
          >
            {conns.length}
          </span>
        </div>
        <div className="flex gap-1.5">
          <button className="btn-secondary btn-sm flex items-center gap-1.5" onClick={refresh}>
            <RefreshCw size={12} /> SYNC
          </button>
          <button className="btn-danger btn-sm flex items-center gap-1.5" onClick={handleCloseAll}>
            <Trash2 size={12} /> CLOSE_ALL
          </button>
        </div>
      </div>

      <div className="table-shell">
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="table-header-row">
                <th className="px-4 py-2.5 text-left">TARGET</th>
                <th className="px-4 py-2.5 text-left">PROTO</th>
                <th className="px-4 py-2.5 text-left">CHAIN</th>
                <th className="px-4 py-2.5 text-right">TX</th>
                <th className="px-4 py-2.5 text-right">RX</th>
              </tr>
            </thead>
            <tbody>
              {loading && (
                <tr><td colSpan={5} className="px-4 py-8 text-center font-mono text-[10px] text-muted">LOADING...</td></tr>
              )}
              {!loading && conns.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-4 py-8">
                    <EmptyState
                      title="NO_ACTIVE_CONNECTIONS"
                      description="Traffic through proxy chains will appear here in real-time"
                      icon={<ListTree size={18} />}
                    />
                  </td>
                </tr>
              )}
              {conns.map(c => (
                <tr key={c.id} className="table-row">
                  <td className="px-4 py-2.5 font-mono text-[10px] max-w-xs truncate" style={{ color: '#C8E8F0' }}>
                    {c.metadata.host || c.metadata.sourceIP}:{c.metadata.destinationPort}
                  </td>
                  <td className="px-4 py-2.5">
                    <span
                      className="font-mono text-[9px] px-1.5 py-0.5 uppercase"
                      style={{ color: '#4A6080', border: '1px solid rgba(74,96,128,0.25)', background: 'rgba(74,96,128,0.06)' }}
                    >
                      {c.metadata.network || c.metadata.type}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 font-mono text-[9px] text-muted">{c.chains?.join(' → ')}</td>
                  <td className="px-4 py-2.5 text-right font-mono text-[10px]" style={{ color: '#FF00AA', textShadow: '0 0 6px rgba(255,0,170,0.4)' }}>{formatBytes(c.upload, '')}</td>
                  <td className="px-4 py-2.5 text-right font-mono text-[10px]" style={{ color: '#00FF88', textShadow: '0 0 6px rgba(0,255,136,0.4)' }}>{formatBytes(c.download, '')}</td>
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

const LEVEL_STYLE: Record<string, { color: string; border: string; bg: string }> = {
  info:    { color: '#00F5FF', border: 'rgba(0,245,255,0.3)', bg: 'rgba(0,245,255,0.08)' },
  debug:   { color: '#4A6080', border: 'rgba(74,96,128,0.3)', bg: 'rgba(74,96,128,0.05)' },
  warning: { color: '#FFE600', border: 'rgba(255,230,0,0.3)', bg: 'rgba(255,230,0,0.08)' },
  warn:    { color: '#FFE600', border: 'rgba(255,230,0,0.3)', bg: 'rgba(255,230,0,0.08)' },
  error:   { color: '#FF2255', border: 'rgba(255,34,85,0.3)', bg: 'rgba(255,34,85,0.08)' },
}

const SIDE_STYLE: Record<string, { label: string; color: string; border: string; bg: string }> = {
  router: { label: 'ROUTER', color: '#0080FF', border: 'rgba(0,128,255,0.3)', bg: 'rgba(0,128,255,0.08)' },
  system: { label: 'SYSTEM', color: '#FF00AA', border: 'rgba(255,0,170,0.3)', bg: 'rgba(255,0,170,0.08)' },
}

const FIELD_STYLE: Record<string, { color: string; border: string; bg: string }> = {
  ok:   { color: '#00FF88', border: 'rgba(0,255,136,0.25)', bg: 'rgba(0,255,136,0.06)' },
  err:  { color: '#FF2255', border: 'rgba(255,34,85,0.25)', bg: 'rgba(255,34,85,0.06)' },
  warn: { color: '#FFE600', border: 'rgba(255,230,0,0.25)', bg: 'rgba(255,230,0,0.06)' },
  info: { color: '#00F5FF', border: 'rgba(0,245,255,0.25)', bg: 'rgba(0,245,255,0.06)' },
  muted:{ color: '#4A6080', border: 'rgba(74,96,128,0.15)', bg: 'rgba(74,96,128,0.04)' },
  dim:  { color: '#2A3848', border: 'transparent', bg: 'transparent' },
}

// Keep legacy FIELD_CLS for fieldStyle return type compatibility
const FIELD_CLS: Record<string, string> = {
  ok:   '', err: '', warn: '', info: '', muted: '', dim: '',
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
  const s = FIELD_STYLE[variant]
  return (
    <span
      className="inline-flex items-center gap-0.5 px-1.5 py-0.5 font-mono text-[9px] leading-tight select-text"
      style={{ color: s.color, border: `1px solid ${s.border}`, background: s.bg }}
    >
      <span style={{ opacity: 0.5 }}>{fieldKey.replace(/_/g, '·')}</span>
      <span style={{ opacity: 0.3 }}>=</span>
      <span className="font-bold">{text}</span>
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

  const ls = LEVEL_STYLE[level]
  const ss = side ? SIDE_STYLE[side] : null

  return (
    <div className="group flex flex-col gap-1 px-2 py-1.5 transition-colors duration-150 hover:bg-neon-cyan/[0.02]">
      <div className="flex items-center gap-1.5 flex-wrap min-w-0">
        <span className="font-mono tabular-nums text-[10px] flex-shrink-0 w-[60px] select-text" style={{ color: '#2A3848' }}>
          {timeStr}
        </span>
        <span
          className="px-1.5 py-0.5 font-mono text-[9px] font-bold uppercase tracking-[0.12em] flex-shrink-0"
          style={ls ? { color: ls.color, border: `1px solid ${ls.border}`, background: ls.bg, textShadow: `0 0 6px ${ls.color}60` } : { color: '#4A6080', border: '1px solid rgba(74,96,128,0.2)', background: 'transparent' }}
        >
          {entry.level || '?'}
        </span>
        {ss && (
          <span
            className="px-1.5 py-0.5 font-mono text-[9px] font-bold tracking-[0.1em] flex-shrink-0"
            style={{ color: ss.color, border: `1px solid ${ss.border}`, background: ss.bg }}
          >
            {ss.label}
          </span>
        )}
        <span className="font-mono text-[11px] select-text min-w-0 truncate flex-1" style={{ color: '#C8E8F0' }}>
          {entry.msg ?? ''}
        </span>
        <button
          onClick={handleCopy}
          className="opacity-0 group-hover:opacity-100 transition-opacity p-1 flex-shrink-0 ml-auto cursor-pointer"
          style={{ color: '#4A6080' }}
          onMouseEnter={e => { (e.currentTarget as HTMLElement).style.color = '#00F5FF' }}
          onMouseLeave={e => { (e.currentTarget as HTMLElement).style.color = '#4A6080' }}
          title="复制此行"
        >
          {copied
            ? <span className="font-mono text-[10px]" style={{ color: '#00FF88' }}>✓</span>
            : <Copy size={10} />}
        </button>
      </div>
      {sortedKeys.length > 0 && (
        <div className="flex flex-wrap gap-1 pl-[68px]">
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
            <div className="h-px flex-1" style={{ background: 'linear-gradient(90deg, transparent, rgba(0,245,255,0.10))' }} />
            <span className="font-mono text-[9px] px-1" style={{ color: '#2A3848' }}>PROBE·{batch}</span>
            <div className="h-px flex-1" style={{ background: 'linear-gradient(270deg, transparent, rgba(0,245,255,0.10))' }} />
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
        title="LOG_STREAM"
        description="3s_POLL · SSE_PUSH · STRUCTURED_FIELDS · BATCH_GROUPING"
        actions={
          <div className="flex items-center gap-1.5 flex-wrap">
            {(['all', 'info', 'warning', 'error'] as const).map(l => (
              <button
                key={l}
                onClick={() => setFilter(l)}
                className={`btn-xs flex items-center cursor-pointer ${filter === l ? 'btn-primary' : 'btn-secondary'}`}
              >
                {l === 'all' ? 'ALL' : l.toUpperCase()}
              </button>
            ))}
            <label className="flex items-center gap-1 font-mono text-[10px] text-muted cursor-pointer px-2 min-h-[32px]">
              <input type="checkbox" checked={autoScroll} onChange={e => setAutoScroll(e.target.checked)} style={{ accentColor: '#00F5FF' }} />
              AUTO_SCROLL
            </label>
            <button className="btn-secondary btn-xs flex items-center gap-1 cursor-pointer" onClick={fetchLogs}>
              <RefreshCw size={10} /> SYNC
            </button>
            <button className="btn-secondary btn-xs flex items-center gap-1 cursor-pointer" onClick={handleCopyAll}>
              <Copy size={10} /> COPY_ALL
            </button>
            <button className="btn-danger btn-xs flex items-center gap-1 cursor-pointer" onClick={() => setLines([])}>
              <Trash2 size={10} /> CLEAR
            </button>
          </div>
        }
      >
        <div className="table-shell" style={{ height: '60vh' }}>
          <div className="h-full overflow-y-auto px-2 py-2 space-y-0">
            {rows.length === 0 && (
              <EmptyState
                title="NO_LOG_ENTRIES"
                description="Start ClashForge core, or adjust log level and filter conditions"
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
    <div className="space-y-5">
      <PageHeader
        eyebrow="ACTIVITY_LOG"
        title="活动与日志中心"
        description="CONN_TRACE · PROXY_CHAIN · LIVE_LOG · ANOMALY_DETECT"
        metrics={[
          { label: 'VIEW_MODE', value: tab === 'connections' ? 'CONNECTIONS' : 'LOG_STREAM', color: 'cyan' },
          { label: 'POLL_RATE', value: tab === 'connections' ? '2s_POLL' : '3s+SSE', color: 'green' },
        ]}
      />

      <SegmentedTabs
        items={[
          { value: 'connections', label: 'CONNECTIONS', icon: <Activity size={12} />, hint: 'Real-time proxy connections' },
          { value: 'logs', label: 'LOG_STREAM', icon: <ScrollText size={12} />, hint: 'Live log feed with SSE' },
        ]}
        value={tab}
        onChange={setTab}
      />

      {tab === 'connections' && (
        <SectionCard title="CONN_MATRIX" description="REAL_TIME · TARGET · PROTOCOL · CHAIN · THROUGHPUT">
          <ConnectionsPanel />
        </SectionCard>
      )}
      {tab === 'logs' && <LogsPanel />}
    </div>
  )
}
