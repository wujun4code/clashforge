import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  Activity,
  CheckCircle2,
  Cpu,
  ExternalLink,
  HardDrive,
  Loader2,
  RefreshCw,
  X,
  Zap,
} from 'lucide-react'

import {
  getOverviewCore,
  getOverviewProbes,
  getOverviewResources,
  getClashforgeVersion,
  restartCore,
  getProxies,
  selectProxy,
  testLatency,
} from '../api/client'
import type { ClashforgeVersionData } from '../api/client'
import type {
  OverviewAccessCheck,
  OverviewCoreData,
  OverviewIPCheck,
  OverviewModule,
  OverviewProbeData,
  OverviewResourceData,
} from '../api/client'
import type { ProxyNode } from '../api/client'
import { PageHeader, SectionCard, SegmentedTabs } from '../components/ui'
import { useSSE } from '../hooks/useSSE'
import { useStore } from '../store'
import { formatBytes, formatGB, formatMB, formatPercent, formatUptime } from '../utils/format'

type SectionKey = 'probes' | 'resources'

type ProxyMap = Record<string, ProxyNode>

const TYPE_ORDER = ['Selector', 'Fallback', 'URLTest', 'LoadBalance']
const IGNORED = ['DIRECT', 'REJECT', 'GLOBAL', 'PASS', 'REJECT-DROP', 'Compatible']

interface BrowserIPCheck {
  provider: string
  group: string
  ok: boolean
  ip?: string
  location?: string
  error?: string
}

interface BrowserAccessCheck {
  name: string
  group?: string
  url: string
  description: string
  via: string
  ok: boolean
  latency_ms?: number
  error?: string
}

interface BrowserProbeData {
  checked_at: string
  ip_checks: BrowserIPCheck[]
  access_checks: BrowserAccessCheck[]
}

interface ProbeSnapshot {
  router: OverviewProbeData | null
  browser: BrowserProbeData | null
}

interface ProbeHealth {
  hasData: boolean
  ipOK: boolean
  failedAccess: string[]
  healthy: boolean
}

function evaluateRouterProbeHealth(data: OverviewProbeData | null): ProbeHealth {
  if (!data) return { hasData: false, ipOK: false, failedAccess: [], healthy: false }
  const ipOK = data.ip_checks.some((item) => item.ok)
  const failedAccess = data.access_checks.filter((item) => !item.ok).map((item) => item.name)
  return {
    hasData: true,
    ipOK,
    failedAccess,
    healthy: ipOK && failedAccess.length === 0,
  }
}

function evaluateBrowserProbeHealth(data: BrowserProbeData | null): ProbeHealth {
  if (!data) return { hasData: false, ipOK: false, failedAccess: [], healthy: false }
  const ipOK = data.ip_checks.some((c) => c.ok)
  const failedAccess = data.access_checks.filter((item) => !item.ok).map((item) => item.name)
  return {
    hasData: true,
    ipOK,
    failedAccess,
    healthy: ipOK && failedAccess.length === 0,
  }
}

async function fetchWithTimeout(input: RequestInfo | URL, init: RequestInit, timeoutMs: number) {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), timeoutMs)
  try {
    return await fetch(input, { ...init, signal: controller.signal })
  } finally {
    window.clearTimeout(timer)
  }
}

async function runBrowserProbeData(targets: OverviewAccessCheck[]): Promise<BrowserProbeData> {
  const ipProviders: { provider: string; group: string; fetch: () => Promise<{ ip: string; location: string }> }[] = [
    {
      provider: 'UpaiYun',
      group: '国内',
      fetch: async () => {
        const res = await fetchWithTimeout('https://pubstatic.b0.upaiyun.com/?_upnode', { cache: 'no-store' }, 7000)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const payload = await res.json() as { remote_addr?: string; remote_addr_location?: { country?: string; province?: string; city?: string; isp?: string } }
        if (!payload.remote_addr) throw new Error('empty')
        const loc = payload.remote_addr_location
        const location = loc ? [loc.country, loc.province, loc.city, loc.isp].filter(Boolean).join(' · ') : ''
        return { ip: payload.remote_addr, location }
      },
    },
    {
      provider: 'IP.SB',
      group: '国外',
      fetch: async () => {
        const res = await fetchWithTimeout('https://api.ip.sb/geoip', { cache: 'no-store' }, 7000)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const payload = await res.json() as { ip?: string; country?: string; city?: string; region?: string; isp?: string }
        if (!payload.ip) throw new Error('empty')
        const location = [payload.city, payload.region, payload.country].filter(Boolean).join(' · ')
        return { ip: payload.ip, location }
      },
    },
    {
      provider: 'IPInfo',
      group: '国外',
      fetch: async () => {
        const res = await fetchWithTimeout('https://ipinfo.io/json', { cache: 'no-store' }, 7000)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const payload = await res.json() as { ip?: string; city?: string; region?: string; country?: string; org?: string }
        if (!payload.ip) throw new Error('empty')
        const location = [payload.city, payload.region, payload.country, payload.org].filter(Boolean).join(' · ')
        return { ip: payload.ip, location }
      },
    },
  ]

  const ip_checks: BrowserIPCheck[] = await Promise.all(
    ipProviders.map(async (p) => {
      try {
        const { ip, location } = await p.fetch()
        return { provider: p.provider, group: p.group, ok: true, ip, location }
      } catch (error) {
        return { provider: p.provider, group: p.group, ok: false, error: error instanceof Error ? error.message : '获取失败' }
      }
    })
  )

  const accessChecks = await Promise.all(targets.map(async (target) => {
    const started = performance.now()
    try {
      await fetchWithTimeout(target.url, { method: 'GET', mode: 'no-cors', cache: 'no-store', redirect: 'follow' }, 8000)
      return {
        name: target.name,
        group: target.group,
        url: target.url,
        description: target.description,
        via: '由当前浏览器客户端直连发起检测',
        ok: true,
        latency_ms: Math.max(1, Math.round(performance.now() - started)),
      } satisfies BrowserAccessCheck
    } catch (error) {
      return {
        name: target.name,
        group: target.group,
        url: target.url,
        description: target.description,
        via: '由当前浏览器客户端直连发起检测',
        ok: false,
        error: error instanceof Error ? error.message : '浏览器访问失败',
      } satisfies BrowserAccessCheck
    }
  }))

  return {
    checked_at: new Date().toISOString(),
    ip_checks,
    access_checks: accessChecks,
  }
}

function Pill({ tone, label }: { tone: 'success' | 'warning' | 'danger' | 'muted'; label: string }) {
  const styles: Record<string, { border: string; bg: string; color: string; glow: string }> = {
    success: { border: 'rgba(143,212,168,0.35)', bg: 'rgba(143,212,168,0.08)', color: '#8FD4A8', glow: '0 0 6px rgba(143,212,168,0.5)' },
    warning: { border: 'rgba(245,184,107,0.35)', bg: 'rgba(245,184,107,0.08)', color: '#F5B86B', glow: '0 0 6px rgba(245,184,107,0.5)' },
    danger:  { border: 'rgba(232,126,126,0.35)',  bg: 'rgba(232,126,126,0.08)',  color: '#E87E7E', glow: '0 0 6px rgba(232,126,126,0.5)' },
    muted:   { border: 'rgba(255,255,255,0.10)', bg: 'rgba(255,255,255,0.04)', color: '#8EA0B8', glow: 'none' },
  }
  const s = styles[tone]
  return (
    <span
      className="inline-flex items-center px-2 py-0.5 font-mono text-[10px] font-bold uppercase tracking-[0.15em]"
      style={{ border: `1px solid ${s.border}`, background: s.bg, color: s.color, textShadow: s.glow }}
    >
      {label}
    </span>
  )
}

function ModuleRow({ module }: { module: OverviewModule }) {
  const managed = module.managed_by_clashforge
  const tone: 'success' | 'warning' | 'danger' | 'muted' = managed
    ? 'success'
    : module.status === 'conflict' ? 'warning'
    : module.status === 'inactive' ? 'danger'
    : 'muted'
  const statusLabel = managed ? 'MANAGED' : module.status === 'conflict' ? 'CONFLICT' : module.status === 'inactive' ? 'INACTIVE' : 'STANDBY'
  const accentColor = { success: 'rgba(143,212,168,0.2)', warning: 'rgba(245,184,107,0.2)', danger: 'rgba(232,126,126,0.2)', muted: 'rgba(74,96,128,0.2)' }[tone]

  return (
    <div
      className="px-4 py-3 transition-all duration-200"
      style={{ border: '1px solid rgba(106,168,224,0.08)', background: `linear-gradient(135deg, rgba(6,12,18,0.8), ${accentColor})` }}
    >
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0 flex-1">
          <p className="font-mono text-xs font-semibold uppercase tracking-[0.06em] text-[#EDE6D7] truncate">{module.title}</p>
          <p className="font-mono text-[10px] text-muted mt-0.5 truncate">{module.current_owner}</p>
        </div>
        <Pill tone={tone} label={statusLabel} />
      </div>
    </div>
  )
}

// ── Proxy switcher helpers ──────────────────────────────────────────────────

function LatencyBar({ ms }: { ms: number }) {
  if (!ms || ms <= 0) return <span className="font-mono text-[10px] text-muted">—</span>
  const color = ms < 100 ? '#8FD4A8' : ms < 300 ? '#F5B86B' : '#E87E7E'
  const width = ms < 100 ? '90%' : ms < 300 ? '55%' : '25%'
  return (
    <div className="flex items-center gap-2">
      <div className="w-10 h-0.5 overflow-hidden" style={{ background: 'rgba(106,168,224,0.1)' }}>
        <div className="h-full transition-all duration-500" style={{ width, background: color, boxShadow: `0 0 4px ${color}` }} />
      </div>
      <span className="font-mono text-[10px] tabular-nums" style={{ color, textShadow: `0 0 6px ${color}` }}>{ms}ms</span>
    </div>
  )
}

function ProxyGroup({ name, group, allProxies, onSelect }: {
  name: string; group: ProxyNode; allProxies: ProxyMap
  onSelect: (group: string, proxy: string) => void
}) {
  const [open, setOpen] = useState(true)
  const members = (group.all ?? []).filter(n => !IGNORED.includes(n))

  return (
    <div style={{ border: '1px solid rgba(106,168,224,0.10)', overflow: 'hidden' }}>
      <button
        onClick={() => setOpen(o => !o)}
        className="w-full flex items-center justify-between px-4 py-3 cursor-pointer transition-all duration-200"
        style={{ background: open ? 'rgba(106,168,224,0.04)' : 'transparent' }}
        onMouseEnter={e => { (e.currentTarget as HTMLElement).style.background = 'rgba(106,168,224,0.06)' }}
        onMouseLeave={e => { (e.currentTarget as HTMLElement).style.background = open ? 'rgba(106,168,224,0.04)' : 'transparent' }}
      >
        <div className="flex items-center gap-3">
          <span
            className="font-mono text-xs font-bold uppercase tracking-[0.08em]"
            style={{ color: '#6AA8E0', textShadow: '0 0 8px rgba(106,168,224,0.5)' }}
          >
            {name}
          </span>
          <span
            className="font-mono text-[9px] px-1.5 py-0.5 uppercase tracking-[0.1em] text-muted"
            style={{ border: '1px solid rgba(106,168,224,0.15)' }}
          >
            {group.type}
          </span>
          {group.now && (
            <span className="font-mono text-[10px] text-muted">
              {'→ '}<span style={{ color: '#EDE6D7' }}>{group.now}</span>
            </span>
          )}
        </div>
        <span className="font-mono text-[10px] text-muted">{open ? '▲' : '▼'}</span>
      </button>

      {open && (
        <div
          className="px-2 pb-2 space-y-0.5 max-h-64 overflow-y-auto"
          style={{ borderTop: '1px solid rgba(106,168,224,0.06)' }}
        >
          {members.map(m => {
            const node = allProxies[m]
            if (!node) return (
              <div key={m} className="px-3 py-2 font-mono text-[10px] text-muted">{m}</div>
            )
            const isSelected = group.now === m
            const lastDelay = node.history?.at(-1)?.delay ?? -1
            return (
              <button
                key={m}
                onClick={() => { if (group.type === 'Selector') onSelect(name, m) }}
                className="w-full flex items-center gap-3 px-3 py-2 transition-all duration-200 text-left cursor-pointer"
                style={{
                  background: isSelected ? 'rgba(106,168,224,0.08)' : 'transparent',
                  borderLeft: isSelected ? '2px solid #6AA8E0' : '2px solid transparent',
                }}
                onMouseEnter={e => { if (!isSelected) (e.currentTarget as HTMLElement).style.background = 'rgba(106,168,224,0.04)' }}
                onMouseLeave={e => { if (!isSelected) (e.currentTarget as HTMLElement).style.background = 'transparent' }}
              >
                <div
                  className="w-1.5 h-1.5 flex-shrink-0"
                  style={{
                    background: isSelected ? '#6AA8E0' : 'rgba(74,96,128,0.5)',
                    boxShadow: isSelected ? '0 0 6px #6AA8E0' : 'none',
                  }}
                />
                <div className="flex-1 min-w-0">
                  <p
                    className="font-mono text-xs truncate"
                    style={{ color: isSelected ? '#6AA8E0' : '#EDE6D7', textShadow: isSelected ? '0 0 6px rgba(106,168,224,0.5)' : 'none' }}
                  >
                    {node.name}
                  </p>
                  <p className="font-mono text-[9px] text-muted">{node.type}</p>
                </div>
                <LatencyBar ms={lastDelay} />
              </button>
            )
          })}
          {group.type === 'Selector' && ['DIRECT', 'REJECT'].filter(n => group.all?.includes(n)).map(n => {
            const isSelected = group.now === n
            return (
              <button
                key={n}
                onClick={() => onSelect(name, n)}
                className="w-full flex items-center gap-3 px-3 py-2 transition-all duration-200 text-left cursor-pointer"
                style={{
                  background: isSelected ? 'rgba(106,168,224,0.08)' : 'transparent',
                  borderLeft: isSelected ? '2px solid #6AA8E0' : '2px solid transparent',
                }}
              >
                <div className="w-1.5 h-1.5 flex-shrink-0" style={{ background: isSelected ? '#6AA8E0' : 'rgba(74,96,128,0.5)' }} />
                <span className="font-mono text-xs" style={{ color: isSelected ? '#6AA8E0' : '#EDE6D7' }}>{n}</span>
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}

function MetricTile({ icon, label, value, hint, color = 'cyan' }: {
  icon: ReactNode; label: string; value: string; hint?: string; color?: 'cyan' | 'green' | 'magenta' | 'yellow'
}) {
  const c = { cyan: '#6AA8E0', green: '#8FD4A8', magenta: '#F4A6B5', yellow: '#F5B86B' }[color]
  return (
    <div
      className="px-4 py-3 transition-all duration-200 hud-bracket"
      style={{ border: '1px solid rgba(106,168,224,0.10)', background: 'rgba(6,12,18,0.7)' }}
    >
      <div className="flex items-center gap-1.5" style={{ color: 'rgba(74,96,128,0.8)' }}>
        {icon}
        <p className="font-mono text-[9px] uppercase tracking-[0.2em] text-muted">{label}</p>
      </div>
      <p
        className="font-mono text-xl font-bold mt-2 tabular-nums"
        style={{ color: c, textShadow: `0 0 12px ${c}80` }}
      >
        {value}
      </p>
      {hint ? <p className="font-mono text-[10px] text-muted mt-1">{hint}</p> : null}
    </div>
  )
}

function StatusDot({ online }: { online: boolean }) {
  return (
    <span
      className="inline-flex h-1.5 w-1.5 flex-shrink-0"
      aria-hidden
      style={{
        background: online ? '#8FD4A8' : '#E87E7E',
        boxShadow: online ? '0 0 6px #8FD4A8, 0 0 12px rgba(143,212,168,0.4)' : '0 0 6px #E87E7E',
        animation: online ? 'pulseSoft 2s ease-in-out infinite' : 'none',
      }}
    />
  )
}

function groupBy<T extends { group?: string }>(items: T[]): [string, T[]][] {
  const map = new Map<string, T[]>()
  for (const item of items) {
    const g = item.group ?? '其他'
    if (!map.has(g)) map.set(g, [])
    map.get(g)!.push(item)
  }
  return Array.from(map.entries())
}

function GroupTag({ label }: { label: string }) {
  const s = label === '国内'
    ? { color: '#6AA8E0', border: 'rgba(106,168,224,0.3)', bg: 'rgba(106,168,224,0.06)' }
    : label === '国外'
      ? { color: '#F4A6B5', border: 'rgba(244,166,181,0.3)', bg: 'rgba(244,166,181,0.06)' }
      : label === 'AI'
        ? { color: '#F5B86B', border: 'rgba(245,184,107,0.3)', bg: 'rgba(245,184,107,0.06)' }
        : { color: '#8EA0B8', border: 'rgba(74,96,128,0.3)', bg: 'rgba(74,96,128,0.06)' }
  return (
    <span
      className="inline-flex items-center px-2 py-0.5 font-mono text-[9px] font-bold uppercase tracking-[0.15em]"
      style={{ color: s.color, border: `1px solid ${s.border}`, background: s.bg, textShadow: `0 0 6px ${s.color}80` }}
    >
      {label}
    </span>
  )
}

function IPCard({ item }: { item: OverviewIPCheck }) {
  return (
    <div className="px-4 py-3 transition-all duration-200" style={{ border: '1px solid rgba(106,168,224,0.08)', background: 'rgba(6,12,18,0.6)' }}>
      <div className="flex items-center justify-between gap-3">
        <p className="font-mono text-xs font-semibold uppercase tracking-[0.06em] text-[#EDE6D7]">{item.provider}</p>
        <Pill tone={item.ok ? 'success' : 'danger'} label={item.ok ? 'RESOLVED' : 'FAILED'} />
      </div>
      {item.ok ? (
        <>
          <p className="font-mono text-base font-bold mt-2 tabular-nums" style={{ color: '#6AA8E0', textShadow: '0 0 8px rgba(106,168,224,0.5)' }}>
            {item.ip || '--'}
          </p>
          <p className="font-mono text-[10px] text-muted mt-1 leading-5">{item.location || 'NO_LOCATION'}</p>
        </>
      ) : (
        <p className="font-mono text-[10px] mt-2 leading-5" style={{ color: '#E87E7E' }}>{item.error || 'FETCH_FAILED'}</p>
      )}
    </div>
  )
}

function AccessCard({ item }: { item: OverviewAccessCheck }) {
  return (
    <div className="px-4 py-3" style={{ border: '1px solid rgba(106,168,224,0.08)', background: 'rgba(6,12,18,0.6)' }}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-mono text-xs font-semibold uppercase tracking-[0.06em] text-[#EDE6D7] truncate">{item.name}</p>
          <p className="font-mono text-[10px] text-muted mt-0.5">{item.description}</p>
        </div>
        <Pill tone={item.ok ? 'success' : 'danger'} label={item.ok ? 'OK' : 'ERR'} />
      </div>
      <p className="font-mono text-xs mt-2" style={{ color: item.ok ? '#8FD4A8' : '#E87E7E' }}>
        {item.ok ? `${item.latency_ms ?? 0}ms` : (item.error || 'REQUEST_FAILED')}
      </p>
      <p className="font-mono text-[9px] text-muted mt-1 break-all">{item.url}</p>
    </div>
  )
}

function ProcessCard({ name, pid, cpu, memory, uptime, running, command }: {
  name: string; pid: number; cpu: number; memory: number; uptime: number; running: boolean; command?: string
}) {
  return (
    <div className="px-4 py-3" style={{ border: '1px solid rgba(106,168,224,0.10)', background: 'rgba(6,12,18,0.7)' }}>
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <StatusDot online={running} />
          <p className="font-mono text-xs font-bold uppercase tracking-[0.08em]" style={{ color: '#EDE6D7' }}>{name}</p>
        </div>
        <span className="font-mono text-[10px] text-muted">PID_{pid || '--'}</span>
      </div>
      <div className="grid grid-cols-2 gap-2 mt-3">
        {[
          { label: 'CPU', val: formatPercent(cpu), color: cpu > 80 ? '#E87E7E' : cpu > 50 ? '#F5B86B' : '#8FD4A8' },
          { label: 'MEM', val: formatMB(memory), color: '#6AA8E0' },
          { label: 'UPTIME', val: formatUptime(uptime), color: '#F4A6B5' },
          { label: 'STATUS', val: running ? 'ONLINE' : 'OFFLINE', color: running ? '#8FD4A8' : '#E87E7E' },
        ].map(({ label, val, color }) => (
          <div key={label}>
            <p className="font-mono text-[9px] uppercase tracking-[0.2em] text-muted">{label}</p>
            <p className="font-mono text-xs font-bold mt-0.5" style={{ color, textShadow: `0 0 6px ${color}60` }}>{val}</p>
          </div>
        ))}
      </div>
      {command ? <p className="font-mono text-[9px] text-muted mt-2 break-all">{command}</p> : null}
    </div>
  )
}

// ── Probe pane ─────────────────────────────────────────────────────────────

type PaneIPCheck = { provider: string; group?: string; ok: boolean; ip?: string; location?: string; error?: string }
type PaneAccessCheck = { name: string; group?: string; url: string; description: string; via: string; ok: boolean; latency_ms?: number; error?: string }

function ProbePane({ title, subtitle, health, ipChecks, accessChecks, loading }: {
  title: string; subtitle: string; health: ProbeHealth
  ipChecks: PaneIPCheck[]; accessChecks: PaneAccessCheck[]; loading?: boolean
}) {
  const ipGroups = groupBy(ipChecks)
  const accessGroups = groupBy(accessChecks)
  const hasContent = ipChecks.length > 0 || accessChecks.length > 0
  return (
    <div style={{ border: '1px solid rgba(106,168,224,0.10)', overflow: 'hidden', background: 'rgba(2,4,8,0.6)' }}>
      <div
        className="flex items-center justify-between gap-3 px-4 py-3"
        style={{ borderBottom: '1px solid rgba(106,168,224,0.08)', background: 'rgba(106,168,224,0.03)' }}
      >
        <div>
          <div className="flex items-center gap-2">
            <p className="font-mono text-xs font-bold uppercase tracking-[0.08em] text-white">{title}</p>
            {loading && <Loader2 size={11} className="animate-spin text-muted" />}
          </div>
          <p className="font-mono text-[9px] text-muted mt-0.5">{subtitle}</p>
        </div>
        {health.hasData
          ? <Pill tone={health.healthy ? 'success' : 'warning'} label={health.healthy ? 'PASS' : 'DEGRADED'} />
          : <Pill tone="muted" label={loading ? 'SCANNING' : 'IDLE'} />
        }
      </div>
      <div className="px-4 py-4 space-y-4">
        {!hasContent ? (
          <p className="font-mono text-[10px] text-muted py-3 text-center">{loading ? 'SCANNING_NETWORK...' : 'NO_DATA'}</p>
        ) : (
          <>
            {ipGroups.length > 0 && (
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <span className="font-mono text-[9px] font-bold uppercase tracking-[0.2em] text-muted whitespace-nowrap">EGRESS_IP</span>
                  <div className="flex-1 h-px" style={{ background: 'linear-gradient(90deg, rgba(106,168,224,0.15), transparent)' }} />
                </div>
                {ipGroups.map(([group, items]) => (
                  <div key={group} className="space-y-1.5">
                    <GroupTag label={group} />
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-1.5">
                      {items.map((item) => <IPCard key={item.provider} item={item as OverviewIPCheck} />)}
                    </div>
                  </div>
                ))}
              </div>
            )}
            {accessGroups.length > 0 && (
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <span className="font-mono text-[9px] font-bold uppercase tracking-[0.2em] text-muted whitespace-nowrap">ACCESS_CHECK</span>
                  <div className="flex-1 h-px" style={{ background: 'linear-gradient(90deg, rgba(106,168,224,0.15), transparent)' }} />
                </div>
                {accessGroups.map(([group, items]) => (
                  <div key={group} className="space-y-1.5">
                    <GroupTag label={group} />
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-1.5">
                      {items.map((item) => <AccessCard key={item.name} item={item as OverviewAccessCheck} />)}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

const SKIP_VERSION_KEY = 'cf_skip_version'

function UpdateBanner({ data, onSkip }: { data: ClashforgeVersionData; onSkip: () => void }) {
  return (
    <div
      className="relative overflow-hidden px-5 py-4 animate-slide-up"
      style={{
        border: '1px solid rgba(245,184,107,0.3)',
        background: 'linear-gradient(135deg, rgba(245,184,107,0.06), rgba(2,4,8,0.8))',
        clipPath: 'polygon(0 0, calc(100% - 12px) 0, 100% 12px, 100% 100%, 12px 100%, 0 calc(100% - 12px))',
      }}
    >
      {/* Corner accent */}
      <div className="pointer-events-none absolute top-0 right-0" style={{ width: 12, height: 12, background: 'linear-gradient(225deg, rgba(245,184,107,0.6) 0%, transparent 60%)' }} />
      <div className="flex items-center justify-between gap-4 flex-wrap">
        <div className="min-w-0">
          <div
            className="inline-flex items-center gap-1.5 px-2.5 py-1 font-mono text-[10px] font-bold uppercase tracking-[0.2em]"
            style={{ border: '1px solid rgba(245,184,107,0.4)', background: 'rgba(245,184,107,0.08)', color: '#F5B86B', textShadow: '0 0 8px rgba(245,184,107,0.8)' }}
          >
            <Zap size={10} /> UPDATE_AVAILABLE
          </div>
          <p className="font-mono text-sm font-bold mt-2" style={{ color: '#F5B86B', textShadow: '0 0 10px rgba(245,184,107,0.5)' }}>
            v{data.latest} detected
          </p>
          <p className="font-mono text-[10px] text-muted mt-1">current: v{data.current} — upgrade via GitHub Releases</p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <a
            href={data.release_url}
            target="_blank"
            rel="noopener noreferrer"
            className="btn-sm inline-flex items-center gap-1.5 cursor-pointer"
            style={{ border: '1px solid rgba(245,184,107,0.4)', color: '#F5B86B', background: 'rgba(245,184,107,0.08)' }}
          >
            <ExternalLink size={11} />
            RELEASES
          </a>
          <button
            className="btn-sm inline-flex items-center gap-1.5 text-muted cursor-pointer"
            style={{ border: '1px solid rgba(74,96,128,0.3)', background: 'transparent' }}
            onClick={onSkip}
          >
            <X size={11} />
            SKIP
          </button>
        </div>
      </div>
    </div>
  )
}

export function Dashboard() {
  const { currentUp, currentDown, connCount, coreState, setCoreState, pushTraffic, setConnCount } = useStore()

  const [coreData, setCoreData] = useState<OverviewCoreData | null>(null)
  const [probeData, setProbeData] = useState<OverviewProbeData | null>(null)
  const [browserProbeData, setBrowserProbeData] = useState<BrowserProbeData | null>(null)
  const [resourceData, setResourceData] = useState<OverviewResourceData | null>(null)

  const [queryingCore, setQueryingCore] = useState(true)
  const [section, setSection] = useState<SectionKey | null>(null)
  const [loadingSection, setLoadingSection] = useState<SectionKey | null>(null)
  const [loadingBrowserProbe, setLoadingBrowserProbe] = useState(false)
  const [loadingAction, setLoadingAction] = useState<string | null>(null)

  const [versionData, setVersionData] = useState<ClashforgeVersionData | null>(null)
  const [showBanner, setShowBanner] = useState(false)

  // proxy switcher state
  const [proxies, setProxies] = useState<ProxyMap>({})
  const [testingLatency, setTestingLatency] = useState(false)

  useSSE({
    onCoreState: (data) => setCoreState(data.state, data.pid),
    onTraffic: (data) => pushTraffic(data),
    onConnCount: (data) => setConnCount(data.total),
  })

  const refreshCore = useCallback(async (silent = false) => {
    if (!silent) setQueryingCore(true)
    const next = await getOverviewCore().catch(() => null)
    if (next) {
      setCoreData(next)
      setCoreState(next.core.state, next.core.pid)
      if (typeof next.core.active_connections === 'number') {
        setConnCount(next.core.active_connections)
      }
    }
    if (!silent) setQueryingCore(false)
  }, [setConnCount, setCoreState])

  const refreshProxies = useCallback(async () => {
    const data = await getProxies().catch(() => null)
    if (data) setProxies(data.proxies ?? {})
  }, [])

  const refreshProbes = async (): Promise<ProbeSnapshot> => {
    setLoadingSection('probes')
    try {
      const next = await getOverviewProbes().catch(() => null)
      if (!next) {
        setProbeData(null)
        setBrowserProbeData(null)
        return { router: null, browser: null }
      }
      setProbeData(next)
      setLoadingBrowserProbe(true)
      const browser = await runBrowserProbeData(next.access_checks).catch(() => null)
      setBrowserProbeData(browser)
      return { router: next, browser }
    } finally {
      setLoadingBrowserProbe(false)
      setLoadingSection(null)
    }
  }

  const refreshResources = async () => {
    setLoadingSection('resources')
    const next = await getOverviewResources().catch(() => null)
    if (next) setResourceData(next)
    setLoadingSection(null)
  }

  useEffect(() => {
    const bootstrap = setTimeout(() => { void refreshCore(false) }, 0)
    const timer = setInterval(() => { void refreshCore(true) }, 8000)
    return () => { clearTimeout(bootstrap); clearInterval(timer) }
  }, [refreshCore])

  // Only fetch proxies when we know the core is running — avoids 502 spam when Mihomo is stopped
  useEffect(() => {
    if (coreData?.core.state !== 'running') return
    const t = setTimeout(() => { void refreshProxies() }, 0)
    return () => clearTimeout(t)
  }, [coreData, refreshProxies])

  // Version check once on mount
  useEffect(() => {
    const t = setTimeout(async () => {
      const data = await getClashforgeVersion().catch(() => null)
      if (!data?.has_update) return
      const skipped = localStorage.getItem(SKIP_VERSION_KEY)
      if (skipped === data.latest) return
      setVersionData(data)
      setShowBanner(true)
    }, 0)
    return () => clearTimeout(t)
  }, [])

  const openSection = async (target: SectionKey) => {
    setSection(target)
    if (target === 'probes' && !probeData) { await refreshProbes(); return }
    if (target === 'resources' && !resourceData) { await refreshResources() }
  }

  const handleSelectProxy = async (group: string, proxy: string) => {
    await selectProxy(group, proxy).catch(() => null)
    await refreshProxies()
    // auto-run probes to verify the switch
    setSection('probes')
    await refreshProbes()
  }

  const handleTestLatency = async () => {
    setTestingLatency(true)
    const nodeNames = Object.entries(proxies)
      .filter(([, v]) => !TYPE_ORDER.includes(v.type) && !IGNORED.includes(v.name))
      .map(([k]) => k)
    await testLatency(nodeNames).catch(() => null)
    await refreshProxies()
    setTestingLatency(false)
  }

  const effectiveState = queryingCore && !coreData ? 'querying' : (coreData?.core.state || coreState || 'unknown')
  const coreRunning = effectiveState === 'running'

  const routerProbeHealth = useMemo(() => evaluateRouterProbeHealth(probeData), [probeData])
  const browserProbeHealth = useMemo(() => evaluateBrowserProbeHealth(browserProbeData), [browserProbeData])

  const visibleModules = useMemo(() => {
    const preferredOrder = ['proxy_core', 'transparent_proxy', 'nft_firewall', 'dns_entry', 'dns_resolver']
    const byID = new Map((coreData?.modules ?? []).map((item) => [item.id, item]))
    const ordered: OverviewModule[] = []
    for (const id of preferredOrder) {
      const found = byID.get(id)
      if (found) ordered.push(found)
    }
    return ordered
  }, [coreData])

  const proxyGroups = useMemo(() =>
    Object.entries(proxies)
      .filter(([, v]) => TYPE_ORDER.includes(v.type) && !IGNORED.includes(v.name) && v.name !== 'GLOBAL')
      .sort(([, a], [, b]) => TYPE_ORDER.indexOf(a.type) - TYPE_ORDER.indexOf(b.type)),
    [proxies]
  )

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Dashboard"
        title="ClashForge 控制总台"
        description="在一个更现代、更亮眼的控制面板里统一查看核心状态、模块健康、代理选择与连通性诊断。"
        actions={
          <div className="flex items-center gap-2">
            <button
              className="btn-ghost flex items-center gap-2"
              onClick={() => { void refreshCore(false) }}
              disabled={queryingCore}
            >
              <RefreshCw size={14} className={queryingCore ? 'animate-spin' : ''} />
              刷新全局状态
            </button>
            <button
              className="btn-ghost flex items-center gap-2"
              onClick={async () => {
                setLoadingAction('restart')
                await restartCore().catch(() => null)
                setLoadingAction(null)
                await refreshCore(true)
              }}
              disabled={!!loadingAction}
            >
              {loadingAction === 'restart' ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
              重启核心
            </button>
          </div>
        }
        metrics={[
          { label: '核心状态', value: coreRunning ? '运行中' : '未运行' },
          { label: '活跃连接', value: `${connCount}` },
          { label: '上行速率', value: formatBytes(currentUp) },
          { label: '下行速率', value: formatBytes(currentDown) },
        ]}
      />

      {showBanner && versionData && (
        <UpdateBanner
          data={versionData}
          onSkip={() => {
            localStorage.setItem(SKIP_VERSION_KEY, versionData.latest)
            setShowBanner(false)
          }}
        />
      )}

      <SectionCard
        title="核心运行态"
        description={coreData ? `PID ${coreData.core.pid} · 已运行 ${formatUptime(coreData.core.uptime)}` : '等待状态同步'}
      >
        <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
          <MetricTile icon={<Activity size={16} />} label="上传速率" value={formatBytes(currentUp)} hint="实时上行" />
          <MetricTile icon={<Activity size={16} />} label="下载速率" value={formatBytes(currentDown)} hint="实时下行" />
          <MetricTile icon={<CheckCircle2 size={16} />} label="活跃连接" value={`${connCount}`} hint="当前连接数" />
          <MetricTile icon={<Cpu size={16} />} label="核心运行时长" value={coreData ? formatUptime(coreData.core.uptime) : '--'} hint={`PID ${coreData?.core.pid || '--'}`} />
        </div>
      </SectionCard>

      <SectionCard
        title="子模块状态"
        description={coreData?.checked_at ? `更新于 ${new Date(coreData.checked_at).toLocaleTimeString()}` : '等待查询'}
      >
        <div className="grid grid-cols-1 gap-3 xl:grid-cols-3">
          {visibleModules.map((module) => (
            <ModuleRow key={module.id} module={module} />
          ))}
          {!visibleModules.length && (
            <div className="col-span-3 rounded-2xl border border-white/8 bg-black/10 px-4 py-4 text-sm text-muted">
              正在查询模块状态...
            </div>
          )}
        </div>
      </SectionCard>

      <SectionCard
        title="节点切换"
        description="快速浏览代理组、当前选中节点与最近测速结果。"
        actions={
          <div className="flex items-center gap-2">
            <button className="btn-ghost flex items-center gap-2" onClick={refreshProxies}>
              <RefreshCw size={14} /> 刷新
            </button>
            <button className="btn-ghost flex items-center gap-2" onClick={handleTestLatency} disabled={testingLatency}>
              <Zap size={14} className={testingLatency ? 'animate-pulse' : ''} />
              {testingLatency ? '测试中…' : '测速'}
            </button>
          </div>
        }
      >
        {proxyGroups.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-white/15 bg-black/10 px-4 py-5 text-sm text-muted">
            {coreRunning ? '未找到代理组，请先添加订阅并更新节点。' : '内核未运行，无法获取节点列表。'}
          </div>
        ) : (
          <div className="space-y-3">
            {proxyGroups.map(([name, group]) => (
              <ProxyGroup
                key={name}
                name={name}
                group={group}
                allProxies={proxies}
                onSelect={handleSelectProxy}
              />
            ))}
          </div>
        )}
      </SectionCard>

      <SectionCard title="连通性与资源诊断" description="在 IP 检查和资源占用之间切换，快速确认出口工作状态与系统压力。">
        <div className="space-y-4">
          <SegmentedTabs
            items={[
              { value: 'probes', label: 'IP 检查' },
              { value: 'resources', label: '资源占用' },
            ]}
            value={section ?? 'probes'}
            onChange={(value) => { void openSection(value) }}
          />

          {!section ? (
            <div className="rounded-2xl border border-dashed border-white/15 bg-black/10 px-4 py-5 text-sm text-muted">
              切换节点后会自动执行 IP 检查，也可手动点击上方按钮。
            </div>
          ) : null}

          {section === 'probes' ? (
            <div className="space-y-4">
              <div className="flex items-center justify-between gap-3">
                <p className="text-sm text-muted">路由器侧经代理转发，浏览器侧由客户端直连。对比两侧可快速判断代理出口是否工作正常。</p>
                <button className="btn-ghost flex items-center gap-2" onClick={() => { void refreshProbes() }} disabled={loadingSection === 'probes'}>
                  <RefreshCw size={14} className={loadingSection === 'probes' ? 'animate-spin' : ''} />
                  重新检测
                </button>
              </div>

              {!probeData ? (
                <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-5 text-sm text-muted">
                  {loadingSection === 'probes' ? '正在进行联网检测…' : '点击“重新检测”开始。'}
                </div>
              ) : (
                <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
                  <ProbePane
                    title="路由器侧"
                    subtitle="经 ClashForge mixed 端口转发"
                    health={routerProbeHealth}
                    ipChecks={probeData.ip_checks}
                    accessChecks={probeData.access_checks}
                    loading={loadingSection === 'probes'}
                  />
                  <ProbePane
                    title="浏览器侧"
                    subtitle="由当前浏览器客户端直连，不经过代理"
                    health={browserProbeHealth}
                    ipChecks={browserProbeData?.ip_checks ?? []}
                    accessChecks={browserProbeData?.access_checks ?? []}
                    loading={loadingBrowserProbe}
                  />
                </div>
              )}
            </div>
          ) : null}

          {section === 'resources' ? (
            <div className="space-y-4">
              <div className="flex items-center justify-between gap-3">
                <p className="text-sm text-muted">系统资源与 ClashForge 占用。</p>
                <button className="btn-ghost flex items-center gap-2" onClick={() => { void refreshResources() }} disabled={loadingSection === 'resources'}>
                  <RefreshCw size={14} className={loadingSection === 'resources' ? 'animate-spin' : ''} />
                  刷新资源
                </button>
              </div>
              {!resourceData ? (
                <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-5 text-sm text-muted">
                  {loadingSection === 'resources' ? '正在采样…' : '点击“刷新资源”开始加载。'}
                </div>
              ) : (
                <>
                  <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
                    <MetricTile icon={<Cpu size={16} />} label="系统 CPU" value={formatPercent(resourceData.resources.system.cpu_percent)} />
                    <MetricTile icon={<Activity size={16} />} label="系统内存" value={`${formatMB(resourceData.resources.system.memory_used_mb)} / ${formatMB(resourceData.resources.system.memory_total_mb)}`} hint={`已用 ${formatPercent(resourceData.resources.system.memory_percent)}`} />
                    <MetricTile icon={<HardDrive size={16} />} label="系统磁盘" value={`${formatGB(resourceData.resources.system.disk_used_gb)} / ${formatGB(resourceData.resources.system.disk_total_gb)}`} hint={`已用 ${formatPercent(resourceData.resources.system.disk_percent)}`} />
                  </div>
                  <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                    {resourceData.resources.processes.map((item) => (
                      <ProcessCard
                        key={item.id}
                        name={item.name}
                        pid={item.pid}
                        cpu={item.cpu_percent}
                        memory={item.memory_rss_mb}
                        uptime={item.uptime}
                        running={item.running}
                        command={item.command}
                      />
                    ))}
                  </div>
                  <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4">
                    <p className="text-sm font-semibold text-slate-100">ClashForge 磁盘占用</p>
                    <div className="mt-3 grid grid-cols-2 gap-3 text-sm md:grid-cols-5">
                      {[
                        { label: '运行目录', val: formatMB(resourceData.resources.app.runtime_mb) },
                        { label: '数据目录', val: formatMB(resourceData.resources.app.data_mb) },
                        { label: '程序文件', val: formatMB(resourceData.resources.app.binary_mb) },
                        { label: '规则文件', val: formatMB(resourceData.resources.app.rules_mb) },
                        { label: '总占用', val: formatMB(resourceData.resources.app.total_mb) },
                      ].map(({ label, val }) => (
                        <div key={label}>
                          <p className="text-[11px] uppercase tracking-[0.16em] text-muted">{label}</p>
                          <p className="mt-1 text-slate-200">{val}</p>
                        </div>
                      ))}
                    </div>
                    {!!resourceData.resources.app.rule_assets?.length && (
                      <div className="mt-4 space-y-2 border-t border-white/10 pt-3">
                        <p className="text-xs uppercase tracking-[0.16em] text-muted">规则文件明细</p>
                        {resourceData.resources.app.rule_assets.map((asset) => (
                          <div key={`${asset.name}-${asset.path}`} className="flex items-center justify-between gap-3 text-xs">
                            <span className="text-slate-200">{asset.name}</span>
                            <span className="text-muted">{formatMB(asset.size_mb)} · {asset.path}</span>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </>
              )}
            </div>
          ) : null}
        </div>
      </SectionCard>
    </div>
  )
}
