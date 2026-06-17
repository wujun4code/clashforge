import { useEffect, useRef, useState, useCallback } from 'react'
import { getConnections, closeAllConns, getLogs, clearLogs, pauseLogs, resumeLogs, getServiceLog, clearServiceLog } from '../api/client'
import type { Connection, RequestLogEntry, ServiceLogData } from '../api/client'
import { formatBytes } from '../utils/format'
import { Activity, Copy, FileText, ListTree, PauseCircle, PlayCircle, RefreshCw, ScrollText, Search, Trash2, X } from 'lucide-react'
import { EmptyState, PageHeader, SectionCard, SegmentedTabs } from '../components/ui'

// ── Shared copy helper ────────────────────────────────────────────────────────

function copyText(text: string) {
  if (navigator.clipboard) {
    navigator.clipboard.writeText(text).catch(() => null)
  } else {
    const ta = document.createElement('textarea')
    ta.value = text
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    document.body.removeChild(ta)
  }
}

function CopyIconButton({ text, title = '复制', size = 10, className = '' }: { text: string; title?: string; size?: number; className?: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button
      onClick={() => { copyText(text); setCopied(true); setTimeout(() => setCopied(false), 1500) }}
      className={`transition-opacity p-1 rounded text-slate-600 hover:text-slate-300 hover:bg-white/5 flex-shrink-0 ${className}`}
      title={title}
    >
      {copied ? <span className="text-[10px] text-emerald-400 font-mono">✓</span> : <Copy size={size} />}
    </button>
  )
}

// ── Connections panel ─────────────────────────────────────────────────────────

function ConnectionsPanel({ search }: { search: string }) {
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

  const keyword = search.trim().toLowerCase()
  const visible = keyword
    ? conns.filter(c => {
        const domain = (c.metadata.host || c.metadata.sourceIP || '').toLowerCase()
        const chain = (c.chains?.join(' ') ?? '').toLowerCase()
        const proto = (c.metadata.network || c.metadata.type || '').toLowerCase()
        const port = String(c.metadata.destinationPort ?? '')
        return domain.includes(keyword) || chain.includes(keyword) || proto.includes(keyword) || port.includes(keyword)
      })
    : conns

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted">活跃连接</span>
          <span className="badge badge-muted">{visible.length}{keyword ? `/${conns.length}` : ''}</span>
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
              {!loading && visible.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-4 py-8">
                    <EmptyState
                      title={keyword ? `未找到匹配"${search}"的连接` : '暂无活跃连接'}
                      description={keyword ? '尝试其他关键字，或清空搜索框查看全部连接。' : '当设备或应用开始通过代理链转发流量时，这里会实时显示目标、链路与吞吐量。'}
                      icon={<ListTree size={18} />}
                    />
                  </td>
                </tr>
              )}
              {visible.map(c => {
                const domain = c.metadata.host || c.metadata.sourceIP
                return (
                <tr key={c.id} className="table-row group">
                  <td className="px-4 py-3 max-w-xs">
                    <div className="flex items-center gap-1 min-w-0">
                      <span className="font-mono text-xs text-slate-200 truncate">
                        {domain}:{c.metadata.destinationPort}
                      </span>
                      <CopyIconButton
                        text={domain}
                        title="复制域名"
                        size={11}
                        className="opacity-0 group-hover:opacity-100"
                      />
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <span className="badge badge-muted">{c.metadata.network || c.metadata.type}</span>
                  </td>
                  <td className="px-4 py-3 text-xs text-muted">{c.chains?.join(' → ')}</td>
                  <td className="px-4 py-3 text-right font-mono text-xs text-brand">{formatBytes(c.upload, '')}</td>
                  <td className="px-4 py-3 text-right font-mono text-xs text-success">{formatBytes(c.download, '')}</td>
                </tr>
              )})}

            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

// ── Logs panel — HTTP API request log ────────────────────────────────────────

const METHOD_CLS: Record<string, string> = {
  GET:    'text-sky-300 bg-sky-500/10 border-sky-500/30',
  POST:   'text-emerald-300 bg-emerald-500/10 border-emerald-500/30',
  PUT:    'text-amber-300 bg-amber-500/10 border-amber-500/30',
  DELETE: 'text-red-300 bg-red-500/10 border-red-500/30',
  PATCH:  'text-purple-300 bg-purple-500/10 border-purple-500/30',
}

function statusCls(code: number): string {
  if (code >= 500) return 'text-red-300 bg-red-500/10 border-red-500/30'
  if (code >= 400) return 'text-amber-300 bg-amber-500/10 border-amber-500/30'
  if (code >= 300) return 'text-sky-300 bg-sky-500/10 border-sky-500/30'
  if (code >= 200) return 'text-emerald-300 bg-emerald-500/10 border-emerald-500/30'
  return 'text-slate-400 bg-white/5 border-white/10'
}

function LogsPanel({ search }: { search: string }) {
  const [entries, setEntries] = useState<RequestLogEntry[]>([])
  const [paused, setPaused] = useState(false)
  const [autoScroll, setAutoScroll] = useState(true)
  const bottomRef = useRef<HTMLDivElement>(null)

  const fetchLogs = useCallback(() => {
    if (paused) return
    getLogs(500).then(data => {
      if (data?.logs) setEntries(data.logs)
    }).catch(() => null)
  }, [paused])

  useEffect(() => {
    fetchLogs()
    const t = setInterval(fetchLogs, 2000)
    return () => clearInterval(t)
  }, [fetchLogs])

  useEffect(() => {
    if (autoScroll) bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [entries, autoScroll])

  const handleClear = useCallback(async () => {
    await clearLogs().catch(() => null)
    setEntries([])
  }, [])

  const togglePause = useCallback(async () => {
    if (paused) {
      await resumeLogs().catch(() => null)
      setPaused(false)
    } else {
      await pauseLogs().catch(() => null)
      setPaused(true)
    }
  }, [paused])

  const handleCopyAll = useCallback(() => {
    const text = entries
      .map(e => `${new Date(e.ts * 1000).toISOString()}  ${e.method}  ${e.path}  ${e.status}  ${e.latency_ms}ms  ${e.remote_addr}`)
      .join('\n')
    navigator.clipboard.writeText(text).catch(() => null)
  }, [entries])

  const keyword = search.trim().toLowerCase()
  const filtered = keyword
    ? entries.filter(e =>
        e.path.toLowerCase().includes(keyword) ||
        e.method.toLowerCase().includes(keyword) ||
        String(e.status).includes(keyword) ||
        e.remote_addr.includes(keyword)
      )
    : entries

  return (
    <SectionCard
      title="API 请求日志"
      description={
        <span className="flex items-center gap-2 flex-wrap">
          <span>每 2 秒轮询 · 最新 500 条 · 高频轮询端点已过滤</span>
          {paused && <span className="inline-flex items-center gap-1 text-warning text-[11px] font-semibold"><PauseCircle size={11} /> 已暂停</span>}
        </span>
      }
      actions={
        <div className="flex items-center gap-2 flex-wrap">
          <label className="flex min-h-[44px] items-center gap-1.5 text-xs text-muted cursor-pointer px-2">
            <input type="checkbox" checked={autoScroll} onChange={e => setAutoScroll(e.target.checked)} className="accent-brand" />
            自动滚动
          </label>
          <button className="btn-ghost flex items-center gap-2 py-1.5" onClick={fetchLogs}>
            <RefreshCw size={13} /> 刷新
          </button>
          <button
            className={`flex items-center gap-2 py-1.5 btn-ghost text-xs ${paused ? 'text-warning border-warning/40' : ''}`}
            onClick={togglePause}
          >
            {paused ? <><PlayCircle size={13} /> 恢复</> : <><PauseCircle size={13} /> 暂停</>}
          </button>
          <button className="btn-ghost flex items-center gap-2 py-1.5" onClick={handleCopyAll}>
            <Copy size={13} /> 复制全部
          </button>
          <button className="btn-ghost flex items-center gap-2 py-1.5 text-danger/80 hover:text-danger" onClick={handleClear}>
            <Trash2 size={13} /> 清空
          </button>
        </div>
      }
    >
      <div className="table-shell" style={{ height: '60vh' }}>
        <div className="h-full overflow-y-auto px-1 py-1 space-y-px font-mono text-xs">
          {filtered.length === 0 && (
            <EmptyState
              title={keyword ? `未找到匹配"${search}"的请求` : '暂无 API 请求记录'}
              description={keyword ? '尝试其他关键字。' : '执行配置变更、订阅更新、核心启停等操作后记录将在此显示。高频轮询端点（连接监控、概览等）已自动过滤。'}
              icon={<ScrollText size={18} />}
            />
          )}
          {filtered.map((e, i) => {
            const timeStr = new Date(e.ts * 1000).toLocaleTimeString('zh-CN', { hour12: false })
            const methodCls = METHOD_CLS[e.method] ?? 'text-slate-400 bg-white/5 border-white/10'
            const stCls = statusCls(e.status)
            const ip = e.remote_addr.replace(/:\d+$/, '')
            const copyText = `${e.method} ${e.path} ${e.status} ${e.latency_ms}ms`
            return (
              <div key={i} className="group flex items-center gap-2 rounded px-2 py-1 hover:bg-white/[0.03] min-w-0">
                <span className="text-slate-500 tabular-nums flex-shrink-0 w-[66px]">{timeStr}</span>
                <span className={`rounded border px-1.5 py-0.5 text-[10px] font-bold uppercase flex-shrink-0 w-[52px] text-center ${methodCls}`}>
                  {e.method}
                </span>
                <span className={`rounded border px-1.5 py-0.5 text-[10px] font-bold flex-shrink-0 w-[36px] text-center tabular-nums ${stCls}`}>
                  {e.status}
                </span>
                <span className="text-slate-200 flex-1 min-w-0 truncate select-text">{e.path}</span>
                <span className="text-slate-500 flex-shrink-0 tabular-nums w-[52px] text-right">{e.latency_ms}ms</span>
                <span className="text-slate-600 flex-shrink-0 hidden lg:block w-[110px] text-right truncate">{ip}</span>
                <CopyIconButton text={copyText} size={10} className="opacity-0 group-hover:opacity-100" />
              </div>
            )
          })}
          <div ref={bottomRef} />
        </div>
      </div>
    </SectionCard>
  )
}

// ── Service log panel ─────────────────────────────────────────────────────────

const SVC_LEVEL_BADGE: Record<string, string> = {
  debug:   'text-slate-400 border-slate-500/30 bg-slate-500/10',
  info:    'text-sky-300 border-sky-500/30 bg-sky-500/10',
  warn:    'text-yellow-300 border-yellow-500/30 bg-yellow-500/10',
  warning: 'text-yellow-300 border-yellow-500/30 bg-yellow-500/10',
  error:   'text-red-300 border-red-500/30 bg-red-500/10',
}

const STATUS_COLOR: Record<string, string> = {
  applied:    'text-emerald-400',
  ok:         'text-emerald-400',
  started:    'text-emerald-400',
  loaded:     'text-emerald-400',
  configured: 'text-emerald-400',
  detail:     'text-slate-300',
  failed:     'text-red-400',
  error:      'text-red-400',
  skipped:    'text-slate-500',
  disabled:   'text-slate-500',
}

interface SvcLogLine {
  id: number
  level: string
  time: number
  msg: string
  fields: Record<string, unknown>
  raw: string
  isStartup: boolean
}

function parseSvcLine(raw: string, id: number): SvcLogLine {
  try {
    const obj = JSON.parse(raw) as Record<string, unknown>
    const { level, time, message, msg: msgField, ...rest } = obj
    const msg = String(message ?? msgField ?? raw)
    return {
      id,
      level: String(level ?? ''),
      time: Number(time ?? 0),
      msg,
      fields: rest,
      raw,
      isStartup: msg === 'startup_health' || msg === 'startup_summary',
    }
  } catch {
    return { id, level: '', time: 0, msg: raw, fields: {}, raw, isStartup: false }
  }
}

function ServiceLogPanel() {
  const [data, setData] = useState<ServiceLogData | null>(null)
  const [loading, setLoading] = useState(true)
  const [clearing, setClearing] = useState(false)
  const [filter, setFilter] = useState<string>('all')
  const [autoScroll, setAutoScroll] = useState(true)
  const bottomRef = useRef<HTMLDivElement | null>(null)
  const idRef = useRef(0)

  const load = useCallback(() => {
    getServiceLog(1000).then(d => { setData(d); setLoading(false) }).catch(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
    const t = setInterval(load, 5000)
    return () => clearInterval(t)
  }, [load])

  useEffect(() => {
    if (autoScroll) bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [data, autoScroll])

  const handleClear = async () => {
    if (!window.confirm('确认清空磁盘日志文件？清空后无法恢复。')) return
    setClearing(true)
    await clearServiceLog().catch(() => null)
    await load()
    setClearing(false)
  }

  const parsed = (data?.lines ?? []).map(l => parseSvcLine(l, idRef.current++))
  const filtered = filter === 'all'
    ? parsed
    : parsed.filter(l => {
        const lv = l.level.toLowerCase()
        return lv === filter || (filter === 'warn' && lv === 'warning')
      })

  const sizeLabel = data?.size_bytes ? `${(data.size_bytes / 1024).toFixed(1)} KB` : ''

  return (
    <SectionCard
      title="服务日志"
      description={
        <span className="flex items-center gap-3 flex-wrap">
          {data?.file ? <span className="font-mono text-[11px] text-muted">{data.file}</span> : null}
          {sizeLabel ? <span className="text-[11px] text-muted">{sizeLabel}</span> : null}
          <span className="text-[11px] text-muted">每 5 秒自动刷新 · 最新 1000 行</span>
        </span>
      }
      actions={
        <div className="flex items-center gap-2 flex-wrap">
          {(['all', 'info', 'warn', 'error'] as const).map(l => (
            <button key={l} onClick={() => setFilter(l)}
              className={`btn py-1.5 text-xs ${filter === l ? 'btn-primary' : 'btn-ghost'}`}>
              {l === 'all' ? '全部' : l.toUpperCase()}
            </button>
          ))}
          <label className="flex min-h-[44px] items-center gap-1.5 text-xs text-muted cursor-pointer px-2">
            <input type="checkbox" checked={autoScroll} onChange={e => setAutoScroll(e.target.checked)} className="accent-brand" />
            自动滚动
          </label>
          <button className="btn-ghost flex items-center gap-2 py-1.5" onClick={load}>
            <RefreshCw size={13} /> 刷新
          </button>
          <button
            className="btn-ghost flex items-center gap-2 py-1.5 text-danger/80 hover:text-danger"
            onClick={handleClear}
            disabled={clearing}
          >
            <Trash2 size={13} /> {clearing ? '清理中…' : '清理日志文件'}
          </button>
        </div>
      }
    >
      {data?.warning && (
        <div className="mb-3 rounded-lg border border-yellow-500/30 bg-yellow-500/[0.08] px-4 py-3 text-sm text-yellow-300">
          ⚠️ {data.warning}
        </div>
      )}
      <div className="table-shell" style={{ height: '62vh' }}>
        <div className="h-full overflow-y-auto px-2 py-2 space-y-px">
          {loading && (
            <div className="flex items-center justify-center py-10 text-muted text-sm">加载中…</div>
          )}
          {!loading && filtered.length === 0 && (
            <EmptyState
              title="暂无日志"
              description={data?.file ? '服务尚未启动或日志为空。启动服务后等待 5 秒自动刷新。' : '未配置日志文件路径。'}
              icon={<FileText size={18} />}
            />
          )}
          {filtered.map(line => {
            const lv = line.level.toLowerCase()
            const lvBadge = SVC_LEVEL_BADGE[lv] ?? 'text-slate-400 border-white/10 bg-white/5'
            const timeStr = line.time
              ? new Date(line.time * 1000).toLocaleTimeString('zh-CN', { hour12: false })
              : ''
            const phase = line.fields.phase as string | undefined
            const status = line.fields.status as string | undefined
            const warning = line.fields.warning as string | undefined
            const skipKeys = new Set(['side', 'phase', 'status', 'warning'])
            const extraEntries = Object.entries(line.fields).filter(([k]) => !skipKeys.has(k))

            return (
              <div
                key={line.id}
                className={[
                  'rounded px-2 py-1 transition-colors hover:bg-white/[0.03]',
                  line.isStartup ? 'border-l-2 border-brand/40 bg-brand/[0.025]' : '',
                ].join(' ')}
              >
                <div className="flex items-center gap-1.5 flex-wrap min-w-0">
                  {timeStr && (
                    <span className="text-slate-500 font-mono tabular-nums text-[11px] flex-shrink-0 w-[66px]">
                      {timeStr}
                    </span>
                  )}
                  {line.level && (
                    <span className={`rounded border px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider flex-shrink-0 ${lvBadge}`}>
                      {line.level}
                    </span>
                  )}
                  {phase && (
                    <span className="rounded border border-brand/30 bg-brand/10 px-1.5 py-0.5 text-[10px] text-brand-light flex-shrink-0">
                      {phase}
                    </span>
                  )}
                  {status && (
                    <span className={`text-[10px] font-semibold flex-shrink-0 ${STATUS_COLOR[status] ?? 'text-slate-300'}`}>
                      {status}
                    </span>
                  )}
                  <span className="text-slate-200 font-mono text-xs flex-1 min-w-0 break-all">
                    {line.msg}
                  </span>
                </div>
                {warning && (
                  <div className="mt-0.5 pl-[74px] text-[11px] text-yellow-400">{warning}</div>
                )}
                {extraEntries.length > 0 && (
                  <div className="flex flex-wrap gap-x-3 gap-y-0.5 pl-[74px] mt-0.5">
                    {extraEntries.map(([k, v]) => (
                      <span key={k} className="text-[10px]">
                        <span className="text-slate-600">{k}=</span>
                        <span className="text-slate-400 select-text">
                          {Array.isArray(v) ? (v as unknown[]).join(', ') || '—' : String(v)}
                        </span>
                      </span>
                    ))}
                  </div>
                )}
              </div>
            )
          })}
          <div ref={bottomRef} />
        </div>
      </div>
    </SectionCard>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

type Tab = 'connections' | 'logs' | 'service'

export function ActivityLog() {
  const [tab, setTab] = useState<Tab>('connections')
  const [search, setSearch] = useState('')

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Activity"
        title="活动与日志中心"
        description="把活跃连接、代理链路、实时日志与磁盘服务日志放进同一个观察面板，便于断网后回溯故障。"
        metrics={[
          { label: '视图', value: tab === 'connections' ? '连接面板' : tab === 'logs' ? '日志面板' : '服务日志' },
          { label: '刷新', value: tab === 'connections' ? '2 秒轮询' : tab === 'logs' ? '3 秒 + SSE' : '5 秒轮询' },
        ]}
      />

      <SegmentedTabs
        items={[
          { value: 'connections', label: '连接', icon: <Activity size={14} />, hint: '查看实时连接与代理链' },
          { value: 'logs', label: '日志', icon: <ScrollText size={14} />, hint: '查看实时日志流与错误' },
          { value: 'service', label: '服务日志', icon: <FileText size={14} />, hint: '磁盘持久日志，断网后可回溯' },
        ]}
        value={tab}
        onChange={setTab}
      />

      {/* Search bar */}
      {tab !== 'service' && <div className="relative">
        <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted" />
        <input
          type="text"
          value={search}
          onChange={e => setSearch(e.target.value)}
          placeholder={tab === 'connections' ? '搜索域名、代理链、协议…' : '搜索日志消息、域名、字段值…'}
          className="glass-input h-9 w-full pl-8 pr-8 text-sm"
        />
        {search && (
          <button
            onClick={() => setSearch('')}
            className="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded text-muted hover:text-slate-300 transition-colors"
          >
            <X size={13} />
          </button>
        )}
      </div>}

      {tab === 'connections' && (
        <SectionCard
          title="连接总览"
          description="实时查看每条连接的目标地址、协议类型、代理链与上下行吞吐。"
        >
          <ConnectionsPanel search={search} />
        </SectionCard>
      )}
      {tab === 'logs' && <LogsPanel search={search} />}
      {tab === 'service' && <ServiceLogPanel />}
    </div>
  )
}
