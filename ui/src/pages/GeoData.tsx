import { useEffect, useState, useCallback } from 'react'
import {
  getGeoDataStatus,
  getGeoDataConfig,
  updateGeoDataConfig,
  triggerGeoDataUpdate,
  getGeoDataLogs,
  getProxies,
} from '../api/client'
import type {
  GeoDataStatus,
  GeoDataConfig,
  GeoDataUpdateRecord,
  ProxiesData,
} from '../api/client'
import {
  Database,
  RefreshCw,
  CheckCircle2,
  XCircle,
  Clock,
  Loader2,
  ChevronDown,
  HardDrive,
} from 'lucide-react'

// ── Helpers ──────────────────────────────────────────────────────────

function fmtBytes(n: number) {
  if (!n) return '—'
  if (n >= 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`
  return `${(n / 1024).toFixed(1)} KB`
}

function fmtDate(iso?: string) {
  if (!iso) return '—'
  return new Date(iso).toLocaleString('zh-CN', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function fmtDuration(a?: string, b?: string) {
  if (!a || !b) return ''
  const ms = new Date(b).getTime() - new Date(a).getTime()
  return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`
}

function normalizeProxyValue(v?: string) {
  if (!v || v === 'DIRECT') return ''
  return v
}

// ── Proxy selector ───────────────────────────────────────────────────

function ProxySelect({
  value,
  onChange,
  proxies,
}: {
  value: string
  onChange: (v: string) => void
  proxies: string[]
}) {
  return (
    <div className="relative">
      <select
        value={normalizeProxyValue(value)}
        onChange={e => onChange(normalizeProxyValue(e.target.value))}
        className="w-full appearance-none bg-surface-2 border border-white/10 rounded-xl px-3 py-2.5 pr-8 text-sm text-white outline-none focus:border-brand transition-colors cursor-pointer"
      >
        <option value="">DIRECT（直连，不走代理）</option>
        {proxies.map(p => (
          <option key={p} value={p}>{p}</option>
        ))}
      </select>
      <ChevronDown size={14} className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-white/40" />
    </div>
  )
}

// ── File status card ─────────────────────────────────────────────────

function FileCard({ name, exists, size_bytes, mod_time }: {
  name: string; exists: boolean; size_bytes: number; mod_time: string
}) {
  return (
    <div className="glass-card px-5 py-4 flex items-center gap-4">
      <div className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl ${exists ? 'bg-success/10 text-success' : 'bg-white/5 text-white/30'}`}>
        <HardDrive size={18} />
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-sm font-semibold text-white">{name}</p>
        <p className="text-xs text-muted mt-0.5">
          {exists ? `${fmtBytes(size_bytes)} · 更新于 ${fmtDate(mod_time)}` : '文件不存在，需要下载'}
        </p>
      </div>
      {exists
        ? <span className="badge badge-success">已就绪</span>
        : <span className="badge badge-muted">缺失</span>}
    </div>
  )
}

// ── Update record row ─────────────────────────────────────────────────

function RecordRow({ rec }: { rec: GeoDataUpdateRecord }) {
  const [open, setOpen] = useState(false)
  const isRunning = rec.status === 'running'

  return (
    <div className="border border-white/[0.06] rounded-xl overflow-hidden">
      <button
        className="w-full flex items-center gap-3 px-4 py-3 hover:bg-white/[0.03] transition-colors text-left"
        onClick={() => setOpen(o => !o)}
      >
        <div className="flex-shrink-0">
          {isRunning && <Loader2 size={15} className="animate-spin text-brand" />}
          {rec.status === 'ok' && <CheckCircle2 size={15} className="text-success" />}
          {rec.status === 'error' && <XCircle size={15} className="text-danger" />}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-xs font-medium text-white/80">{fmtDate(rec.started_at)}</span>
            {rec.proxy_server && rec.proxy_server !== 'DIRECT' && (
              <span className="badge badge-muted text-[10px]">{rec.proxy_server}</span>
            )}
            {rec.finished_at && (
              <span className="text-[10px] text-muted">{fmtDuration(rec.started_at, rec.finished_at)}</span>
            )}
          </div>
          {rec.error && <p className="text-xs text-danger mt-0.5 truncate">{rec.error}</p>}
        </div>
        <ChevronDown
          size={13}
          className={`flex-shrink-0 text-white/25 transition-transform ${open ? 'rotate-180' : ''}`}
        />
      </button>

      {open && rec.files && rec.files.length > 0 && (
        <div className="border-t border-white/[0.06] px-4 py-3 space-y-2 bg-surface-0/40">
          {rec.files.map(f => (
            <div key={f.name} className="flex items-center gap-2 text-xs">
              {f.status === 'ok'
                ? <CheckCircle2 size={12} className="text-success flex-shrink-0" />
                : <XCircle size={12} className="text-danger flex-shrink-0" />}
              <span className="text-white/70 font-medium w-24 flex-shrink-0">{f.name}</span>
              <span className="text-muted truncate">{f.message ?? f.error}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Main page ────────────────────────────────────────────────────────

export function GeoData({ embedded = false }: { embedded?: boolean }) {
  const [status, setStatus] = useState<GeoDataStatus | null>(null)
  const [cfg, setCfg] = useState<GeoDataConfig | null>(null)
  const [logs, setLogs] = useState<GeoDataUpdateRecord[]>([])
  const [proxies, setProxies] = useState<string[]>([])

  const [selectedProxy, setSelectedProxy] = useState('')
  const [triggering, setTriggering] = useState(false)
  const [savingCfg, setSavingCfg] = useState(false)
  const [cfgDraft, setCfgDraft] = useState<Partial<GeoDataConfig>>({})

  const load = useCallback(async () => {
    const [s, c, l] = await Promise.allSettled([
      getGeoDataStatus(),
      getGeoDataConfig(),
      getGeoDataLogs(),
    ])
    if (s.status === 'fulfilled') setStatus(s.value)
    if (c.status === 'fulfilled') {
      setCfg(c.value)
      setSelectedProxy(normalizeProxyValue(c.value.proxy_server))
    }
    if (l.status === 'fulfilled') setLogs(l.value.records)
  }, [])

  useEffect(() => {
    load()
    getProxies()
      .then((d: ProxiesData) => {
        const groups = Object.entries(d.proxies)
          .filter(([, v]) =>
            ['Selector', 'URLTest', 'Fallback', 'LoadBalance'].includes(v.type)
          )
          .map(([name]) => name)
          .filter(name => name && name !== 'DIRECT')
        setProxies([...new Set(groups)])
      })
      .catch(() => null)
  }, [load])

  // Poll while running
  useEffect(() => {
    if (!status?.is_running) return
    const id = setInterval(load, 2000)
    return () => clearInterval(id)
  }, [status?.is_running, load])

  const handleTrigger = async () => {
    setTriggering(true)
    try {
      await triggerGeoDataUpdate(selectedProxy || undefined)
      setTimeout(load, 500)
    } catch {
      /* conflict = already running */
    } finally {
      setTriggering(false)
    }
  }

  const handleSaveProxy = async () => {
    setSavingCfg(true)
    try {
      await updateGeoDataConfig({ ...cfgDraft, proxy_server: normalizeProxyValue(selectedProxy) })
      setCfgDraft({})
      await load()
    } finally {
      setSavingCfg(false)
    }
  }

  const patchCfg = (patch: Partial<GeoDataConfig>) => {
    setCfgDraft(d => ({ ...d, ...patch }))
  }

  const merged = { ...cfg, ...cfgDraft } as GeoDataConfig
  const isRunning = status?.is_running ?? false

  return (
    <div className="space-y-6">
      {/* Header */}
      {!embedded && (
        <div className="flex items-center justify-between gap-4">
          <div>
            <h1 className="text-xl font-bold text-white flex items-center gap-2">
              <Database size={20} className="text-brand" />
              路由数据文件
            </h1>
            <p className="text-sm text-muted mt-1">
              GeoIP.dat 和 GeoSite.dat 是 mihomo 规则路由所需的数据文件，建议定期更新。
            </p>
          </div>
        </div>
      )}

      {/* File status cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {status?.files.map(f => (
          <FileCard key={f.name} {...f} />
        )) ?? (
          <>
            <div className="glass-card px-5 py-4 h-16 animate-pulse bg-surface-1" />
            <div className="glass-card px-5 py-4 h-16 animate-pulse bg-surface-1" />
          </>
        )}
      </div>

      {/* Manual update */}
      <div className="glass-card px-5 py-5 space-y-4">
        <h2 className="text-sm font-semibold text-white">手动更新</h2>
        <div className="flex items-end gap-3">
          <div className="flex-1">
            <label className="text-xs text-muted font-medium block mb-1.5">下载代理</label>
            <ProxySelect value={selectedProxy} onChange={setSelectedProxy} proxies={proxies} />
          </div>
          <button
            className="btn-primary flex items-center gap-2 flex-shrink-0"
            onClick={handleTrigger}
            disabled={isRunning || triggering}
          >
            {isRunning
              ? <><Loader2 size={14} className="animate-spin" /> 更新中…</>
              : <><RefreshCw size={14} /> 立即更新</>}
          </button>
        </div>
        {isRunning && (
          <div className="flex items-center gap-2 text-xs text-brand">
            <Loader2 size={12} className="animate-spin" />
            正在下载数据文件，请稍候…
          </div>
        )}
      </div>

      {/* Auto update config */}
      <div className="glass-card px-5 py-5 space-y-4">
        <h2 className="text-sm font-semibold text-white">定时更新</h2>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <label className="flex items-center justify-between gap-3 cursor-pointer">
            <div>
              <p className="text-sm text-white/80">自动更新 GeoIP</p>
              <p className="text-xs text-muted">按间隔自动下载 GeoIP.dat</p>
            </div>
            <input
              type="checkbox"
              className="toggle"
              checked={merged.auto_geoip ?? false}
              onChange={e => patchCfg({ auto_geoip: e.target.checked })}
            />
          </label>

          <label className="flex items-center justify-between gap-3 cursor-pointer">
            <div>
              <p className="text-sm text-white/80">自动更新 GeoSite</p>
              <p className="text-xs text-muted">按间隔自动下载 GeoSite.dat</p>
            </div>
            <input
              type="checkbox"
              className="toggle"
              checked={merged.auto_geosite ?? false}
              onChange={e => patchCfg({ auto_geosite: e.target.checked })}
            />
          </label>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label className="text-xs text-muted font-medium block mb-1.5">更新间隔</label>
            <input
              className="w-full bg-surface-2 border border-white/10 rounded-xl px-3 py-2.5 text-sm text-white outline-none focus:border-brand transition-colors"
              placeholder="168h"
              value={merged.geoip_interval ?? ''}
              onChange={e => patchCfg({ geoip_interval: e.target.value, geosite_interval: e.target.value })}
            />
            <p className="text-xs text-muted mt-1">例：24h、168h（7天），GeoIP 和 GeoSite 共用此间隔</p>
          </div>
          <div>
            <label className="text-xs text-muted font-medium block mb-1.5">定时下载代理</label>
            <ProxySelect
              value={normalizeProxyValue(merged.proxy_server)}
              onChange={v => patchCfg({ proxy_server: normalizeProxyValue(v) })}
              proxies={proxies}
            />
            <p className="text-xs text-muted mt-1">定时任务使用的代理，手动更新也以此为默认值</p>
          </div>
        </div>

        <div className="flex justify-end">
          <button
            className="btn-primary flex items-center gap-2"
            onClick={handleSaveProxy}
            disabled={savingCfg || Object.keys(cfgDraft).length === 0}
          >
            {savingCfg ? <Loader2 size={14} className="animate-spin" /> : null}
            保存配置
          </button>
        </div>
      </div>

      {/* Update history */}
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <Clock size={14} className="text-muted" />
          <h2 className="text-sm font-semibold text-white">更新记录</h2>
          <span className="badge badge-muted ml-1">{logs.length}</span>
        </div>

        {logs.length === 0 ? (
          <div className="glass-card px-5 py-8 text-center">
            <p className="text-sm text-muted">暂无更新记录</p>
          </div>
        ) : (
          <div className="space-y-2">
            {[...logs].reverse().map(rec => (
              <RecordRow key={rec.id} rec={rec} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
