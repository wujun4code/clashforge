import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import {
  Upload, FileText, Globe, CheckCircle2, AlertCircle,
  ChevronRight, Play, Loader2, Wifi, XCircle, ArrowRight,
  Sparkles, RotateCw, Link2, PowerOff, ShieldOff, Database, Radio,
  Minus, Terminal, ShieldCheck, Network, ServerCog, Gauge, Eye, Plus, Trash2,
} from 'lucide-react'
import yaml from 'js-yaml'
import {
  updateOverrides, generateConfig, getMihomoConfig, getConfig, updateConfig,
  stopCore, releaseOverviewTakeover,
  getOverviewCore, getOverviewProbes, getLogs,
  addSubscription, getSubscriptions, syncSubUpdate, enableService,
  saveSource, setActiveSource, getSourceFile, getSources,
  checkSetupPorts, getSubscriptionCache, previewSetupFinalConfig, getDeviceGroups, updateDeviceGroups,
} from '../api/client'
import type { OverviewProbeData, OverviewModule, LogEntry, SourceFile, Subscription, SetupPortCheck, DeviceRouteGroup } from '../api/client'
import { ModalShell } from '../components/ui'

type InitStatus = 'checking' | 'running' | 'ready'

// ── Types ────────────────────────────────────────────────────────────────────

type Step = 'import' | 'dns' | 'network' | 'launch' | 'check'
const STEPS: { id: Step; label: string }[] = [
  { id: 'import',  label: '导入配置' },
  { id: 'dns',     label: 'DNS 设置' },
  { id: 'network', label: '网络设置' },
  { id: 'launch',  label: '启动服务' },
  { id: 'check',   label: '连通检测' },
]

const STEP_DETAILS: Record<Step, { eyebrow: string; title: string; desc: string }> = {
  import:  { eyebrow: '01 · Source',  title: '选择配置来源', desc: '从历史配置、订阅、文件或 YAML 文本开始，先确认生成结果再继续。' },
  dns:     { eyebrow: '02 · Resolve', title: '确认 DNS 接管', desc: '保持默认安全配置，按需调整 fake-ip、监听地址和 dnsmasq 共存方式。' },
  network: { eyebrow: '03 · Route',   title: '设置透明代理', desc: '选择 TProxy / Redir / TUN 与防火墙后端，决定哪些流量进入 Mihomo。' },
  launch:  { eyebrow: '04 · Launch',  title: '启动并验证端口', desc: '实时查看启动日志，确认必需端口都已响应后再进入连通检测。' },
  check:   { eyebrow: '05 · Verify',  title: '验证实际连通', desc: '同时从路由器侧和浏览器侧检测出口 IP、国内外站点与 AI 服务访问。' },
}

interface ClashDNS {
  enable?: boolean
  ipv6?: boolean
  'enhanced-mode'?: string
  listen?: string
  nameserver?: string[]
  fallback?: string[]
  'default-nameserver'?: string[]
  'fake-ip-range'?: string
  'respect-rules'?: boolean
}

interface ClashParsed {
  mode?: string
  port?: number
  'socks-port'?: number
  'mixed-port'?: number
  'allow-lan'?: boolean
  dns?: ClashDNS
}

interface FormDNS {
  enable: boolean
  mode: string          // fake-ip | redir-host
  dnsmasq_mode: string  // none | upstream | replace
  apply_on_start: boolean
  listen: string
  ipv6: boolean
}

interface FormNetwork {
  mode: string           // none | tproxy | redir | tun
  firewall_backend: string
  bypass_lan: boolean
  bypass_china: boolean
  apply_on_start: boolean
  ipv6: boolean
}

// Streaming launch event received from POST /api/v1/setup/launch
interface LaunchEvent {
  type: 'step' | 'info' | 'done'
  step?: string
  status?: 'running' | 'ok' | 'error' | 'skip' | 'info'
  message: string
  detail?: string
  success?: boolean
  error?: string
}

// ── Config preview helpers ──────────────────────────────────────────────────

type LineCat = 'dns' | 'geo' | 'port' | 'device' | 'preserved'
interface AnnotatedLine { text: string; cat: LineCat; label?: string }

const BLOCK_INFO: Record<string, { cat: LineCat; label: string }> = {
  'dns':       { cat: 'dns',  label: 'ClashForge 接管：DNS 配置由向导统一重写' },
  'geox-url':  { cat: 'geo',  label: 'ClashForge 接管：GeoData 路径固定到本地文件' },
}
const PORT_INFO: Record<string, string> = {
  'port':                'ClashForge 接管：HTTP 代理端口',
  'socks-port':          'ClashForge 接管：SOCKS5 代理端口',
  'mixed-port':          'ClashForge 接管：混合代理端口',
  'redir-port':          'ClashForge 接管：透明代理（redir）端口',
  'tproxy-port':         'ClashForge 接管：TProxy 端口',
  'external-controller': 'ClashForge 接管：Mihomo API 地址（仅本地）',
  'geodata-mode':        'ClashForge 接管：GeoData 模式',
}
const DEVICE_RULE_PROVIDER_PREFIX = 'cf-device-group-'
const DEVICE_PROVIDER_PATTERN = /cf-device-group-[a-z0-9-]+/ig
const DEVICE_LINE_LABEL = 'ClashForge 接管：设备分组路由（影子策略组 / RULE-SET / AND 规则）'

interface DevicePreviewSignals {
  providerNames: Set<string>
  shadowGroupNames: Set<string>
}

function normalizeYamlScalarToken(value: string): string {
  return value.trim().replace(/^["']+/, '').replace(/["']+$/, '').trim()
}

function collectDevicePreviewSignals(lines: string[]): DevicePreviewSignals {
  const providerNames = new Set<string>()
  const shadowGroupNames = new Set<string>()

  for (const line of lines) {
    const providerMatches = line.match(DEVICE_PROVIDER_PATTERN)
    if (providerMatches) {
      for (const name of providerMatches) providerNames.add(name.toLowerCase())
    }
    DEVICE_PROVIDER_PATTERN.lastIndex = 0

    const ruleSetMatch = line.match(/RULE-SET,(cf-device-group-[a-z0-9-]+),([^,]+),src,no-resolve/i)
    if (ruleSetMatch) {
      providerNames.add(ruleSetMatch[1].toLowerCase())
      const shadowName = normalizeYamlScalarToken(ruleSetMatch[2])
      if (shadowName) shadowGroupNames.add(shadowName)
    }

    const andRuleMatch = line.match(/AND,\(\(RULE-SET,(cf-device-group-[a-z0-9-]+),src,no-resolve\),\(.+\)\),(.+)$/i)
    if (andRuleMatch) {
      providerNames.add(andRuleMatch[1].toLowerCase())
      const shadowName = normalizeYamlScalarToken(andRuleMatch[2])
      if (shadowName) shadowGroupNames.add(shadowName)
    }
  }

  return { providerNames, shadowGroupNames }
}

function annotateLines(content: string): AnnotatedLine[] {
  const lines = content.split('\n')
  const deviceSignals = collectDevicePreviewSignals(lines)
  const hasDeviceManagedPart = deviceSignals.providerNames.size > 0 || deviceSignals.shadowGroupNames.size > 0
  const result: AnnotatedLine[] = []
  let blockCat: LineCat | '' = ''
  let topLevelBlock = ''
  let managedProviderIndent = -1
  let managedProxyGroupIndent = -1

  for (const text of lines) {
    const indent = text.search(/\S/)
    const trimmed = text.trim()

    if (indent === 0 && trimmed !== '') {
      blockCat = ''
      managedProviderIndent = -1
      managedProxyGroupIndent = -1
      topLevelBlock = ''
    }

    if (indent === 0 || indent === -1) {
      const m = text.match(/^([a-z][a-z0-9-]*):/)
      const key = m?.[1] ?? ''
      topLevelBlock = key

      if (BLOCK_INFO[key]) {
        blockCat = BLOCK_INFO[key].cat
        result.push({ text, cat: blockCat, label: BLOCK_INFO[key].label })
        continue
      }
      if (PORT_INFO[key]) {
        result.push({ text, cat: 'port', label: PORT_INFO[key] })
        continue
      }
      if (hasDeviceManagedPart && (key === 'rule-providers' || key === 'proxy-groups' || key === 'rules')) {
        result.push({ text, cat: 'device', label: DEVICE_LINE_LABEL })
        continue
      }
      result.push({ text, cat: 'preserved' })
    } else {
      let cat: LineCat = blockCat || 'preserved'
      let label: string | undefined

      if (hasDeviceManagedPart) {
        if (topLevelBlock === 'rule-providers') {
          if (managedProviderIndent >= 0 && indent > managedProviderIndent) {
            cat = 'device'
            label = DEVICE_LINE_LABEL
          } else {
            if (managedProviderIndent >= 0 && indent <= managedProviderIndent) managedProviderIndent = -1
            const providerEntry = text.match(/^\s*([^\s:#][^:]*)\s*:\s*$/)
            const providerName = normalizeYamlScalarToken(providerEntry?.[1] ?? '').toLowerCase()
            if (providerName && deviceSignals.providerNames.has(providerName)) {
              managedProviderIndent = indent
              cat = 'device'
              label = DEVICE_LINE_LABEL
            }
          }
        }

        if (topLevelBlock === 'proxy-groups') {
          if (managedProxyGroupIndent >= 0 && indent > managedProxyGroupIndent) {
            cat = 'device'
            label = DEVICE_LINE_LABEL
          } else {
            if (managedProxyGroupIndent >= 0 && indent <= managedProxyGroupIndent) managedProxyGroupIndent = -1
            const groupNameLine = text.match(/^\s*-\s*name:\s*(.+)\s*$/)
            if (groupNameLine) {
              const groupName = normalizeYamlScalarToken(groupNameLine[1])
              if (groupName && deviceSignals.shadowGroupNames.has(groupName)) {
                managedProxyGroupIndent = indent
                cat = 'device'
                label = DEVICE_LINE_LABEL
              }
            }
          }
        }

        if (topLevelBlock === 'rules' && text.toLowerCase().includes(DEVICE_RULE_PROVIDER_PREFIX)) {
          cat = 'device'
          label = DEVICE_LINE_LABEL
        }

        if (cat === 'preserved' && text.toLowerCase().includes(DEVICE_RULE_PROVIDER_PREFIX)) {
          cat = 'device'
          label = DEVICE_LINE_LABEL
        }
      }

      result.push(label ? { text, cat, label } : { text, cat })
    }
  }
  return result
}

const CAT_ROW: Record<LineCat, string> = {
  dns:       'bg-blue-500/10 border-l-2 border-blue-400/50',
  geo:       'bg-violet-500/10 border-l-2 border-violet-400/50',
  port:      'bg-amber-500/10 border-l-2 border-amber-400/50',
  device:    'bg-emerald-500/10 border-l-2 border-emerald-400/55',
  preserved: '',
}
const CAT_LABEL: Record<LineCat, string> = {
  dns:       'text-blue-300/70',
  geo:       'text-violet-300/70',
  port:      'text-amber-300/70',
  device:    'text-emerald-300/75',
  preserved: '',
}

const BUILTIN_PROXY_NAMES = ['DIRECT', 'REJECT', 'PASS']
const OVERRIDEABLE_GROUP_TYPES = new Set(['select', 'url-test', 'fallback', 'load-balance'])

interface RoutePolicyGroupOption {
  name: string
  type: string
  proxies: string[]
}

interface RoutePolicyOptions {
  groups: RoutePolicyGroupOption[]
  knownProxyNames: string[]
}

function toRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  return value as Record<string, unknown>
}

function toStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value
    .map((item) => (typeof item === 'string' ? item.trim() : ''))
    .filter((item) => item !== '')
}

function dedupe(items: string[]): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const item of items) {
    if (seen.has(item)) continue
    seen.add(item)
    result.push(item)
  }
  return result
}

function defaultPrefixForIP(ip: string): number {
  return ip.includes(':') ? 128 : 32
}

function clampPrefix(value: number, ip: string): number {
  const max = ip.includes(':') ? 128 : 32
  if (!Number.isFinite(value)) return defaultPrefixForIP(ip)
  const rounded = Math.trunc(value)
  if (rounded < 1) return 1
  if (rounded > max) return max
  return rounded
}

function normalizeDeviceGroupsForSetup(groups: DeviceRouteGroup[]): DeviceRouteGroup[] {
  return [...groups]
    .sort((a, b) => (a.order ?? 0) - (b.order ?? 0))
    .map((group, index) => {
      const devices = (group.devices ?? [])
        .map((device) => {
          const ip = (device.ip ?? '').trim()
          const hostname = (device.hostname ?? '').trim()
          return {
            ip,
            prefix: clampPrefix(
              typeof device.prefix === 'number' ? device.prefix : defaultPrefixForIP(ip || '0.0.0.0'),
              ip || '0.0.0.0',
            ),
            ...(hostname ? { hostname } : {}),
          }
        })
        .filter((device) => device.ip !== '')
      const overrides = (group.overrides ?? [])
        .map((override) => ({
          original_group: (override.original_group ?? '').trim(),
          proxies: dedupe((override.proxies ?? []).map((proxy) => proxy.trim()).filter(Boolean)),
        }))
        .filter((override) => override.original_group !== '')
      return {
        id: (group.id ?? '').trim() || `dg_setup_${index + 1}`,
        name: (group.name ?? '').trim(),
        devices,
        overrides,
        order: index,
      }
    })
}

function sanitizeDeviceGroupsForSetup(groups: DeviceRouteGroup[]): DeviceRouteGroup[] {
  return groups.map((group, index) => {
    const devices = (group.devices ?? [])
      .map((device) => {
        const ip = (device.ip ?? '').trim()
        if (!ip) return null
        const hostname = (device.hostname ?? '').trim()
        return {
          ip,
          prefix: clampPrefix(device.prefix, ip),
          ...(hostname ? { hostname } : {}),
        }
      })
      .filter((device): device is NonNullable<typeof device> => device !== null)
    const overrides = (group.overrides ?? [])
      .map((override) => {
        const originalGroup = (override.original_group ?? '').trim()
        const proxies = dedupe((override.proxies ?? []).map((proxy) => proxy.trim()).filter(Boolean))
        if (!originalGroup || proxies.length === 0) return null
        return {
          original_group: originalGroup,
          proxies,
        }
      })
      .filter((override): override is NonNullable<typeof override> => override !== null)
    return {
      id: (group.id ?? '').trim() || `dg_setup_${index + 1}`,
      name: (group.name ?? '').trim(),
      devices,
      overrides,
      order: index,
    }
  })
}

function serializeDeviceGroupsForSetup(groups: DeviceRouteGroup[]): string {
  return JSON.stringify(sanitizeDeviceGroupsForSetup(groups))
}

function parseRoutePolicyOptions(content: string): RoutePolicyOptions {
  if (!content.trim()) {
    return { groups: [], knownProxyNames: [...BUILTIN_PROXY_NAMES] }
  }
  try {
    const parsed = yaml.load(content)
    const root = toRecord(parsed)
    if (!root) {
      return { groups: [], knownProxyNames: [...BUILTIN_PROXY_NAMES] }
    }

    const knownProxyNames = new Set<string>(BUILTIN_PROXY_NAMES)
    const proxiesRaw = Array.isArray(root.proxies) ? root.proxies : []
    for (const item of proxiesRaw) {
      const proxy = toRecord(item)
      if (!proxy) continue
      const name = typeof proxy.name === 'string' ? proxy.name.trim() : ''
      if (!name) continue
      knownProxyNames.add(name)
    }

    const groupsRaw = Array.isArray(root['proxy-groups']) ? root['proxy-groups'] : []
    const groups: RoutePolicyGroupOption[] = []
    for (const item of groupsRaw) {
      const group = toRecord(item)
      if (!group) continue
      const name = typeof group.name === 'string' ? group.name.trim() : ''
      const type = typeof group.type === 'string' ? group.type.trim().toLowerCase() : ''
      if (!name || !OVERRIDEABLE_GROUP_TYPES.has(type)) continue
      const members = dedupe(toStringArray(group.proxies).filter((member) => knownProxyNames.has(member)))
      if (members.length === 0) continue
      groups.push({ name, type, proxies: members })
    }

    groups.sort((a, b) => a.name.localeCompare(b.name, 'zh-Hans-CN'))
    return {
      groups,
      knownProxyNames: Array.from(knownProxyNames),
    }
  } catch {
    return { groups: [], knownProxyNames: [...BUILTIN_PROXY_NAMES] }
  }
}

function ConfigPreview({ content, onContinue }: { content: string; onContinue: () => void }) {
  const lines = annotateLines(content)
  const legend = [
    { style: 'bg-blue-500/25 text-blue-200',   label: 'DNS 配置（已根据您的选择重写）' },
    { style: 'bg-amber-500/25 text-amber-200', label: '端口 / API 地址（ClashForge 统一管理）' },
    { style: 'bg-violet-500/25 text-violet-200', label: 'GeoData 设置（使用本地文件）' },
    { style: 'bg-emerald-500/25 text-emerald-200', label: '设备分组接管字段（影子策略组 / RULE-SET / AND）' },
  ]
  return (
    <div className="space-y-4">
      <div className="glass-card px-5 py-4 space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <FileText size={15} className="text-brand" />
            <h2 className="text-sm font-semibold text-slate-200">生成配置预览</h2>
          </div>
          <span className="text-xs text-muted">{lines.length} 行</span>
        </div>
        <div className="flex flex-wrap gap-2 items-center">
          {legend.map(l => (
            <span key={l.label} className={`inline-flex text-xs px-2 py-0.5 rounded-md ${l.style}`}>{l.label}</span>
          ))}
          <span className="text-xs text-muted">无底色 = 原样保留</span>
        </div>
        <div className="rounded-xl bg-black/30 border border-white/8 overflow-auto max-h-96 text-xs font-mono select-text">
          {lines.map((ln, i) => (
            <div key={i} className={`flex items-start gap-2 px-2 py-px leading-5 ${CAT_ROW[ln.cat]}`}>
              <span className="select-none text-white/20 w-7 flex-shrink-0 text-right tabular-nums">{i + 1}</span>
              <span className="flex-1 text-slate-200 whitespace-pre">{ln.text || ' '}</span>
              {ln.label && (
                <span className={`flex-shrink-0 text-[10px] pl-3 self-center ${CAT_LABEL[ln.cat]}`}>← {ln.label}</span>
              )}
            </div>
          ))}
        </div>
      </div>
      <button className="btn-primary w-full flex items-center justify-center gap-2" onClick={onContinue}>
        <ArrowRight size={14} />确认，继续 DNS 设置
      </button>
    </div>
  )
}

function LaunchConfigPreview({
  content,
  loading,
  error,
  onRefresh,
  title = '最终运行配置预览',
  description = '高亮区域为 ClashForge 接管项',
  refreshLabel = '刷新预览',
}: {
  content: string
  loading: boolean
  error: string
  onRefresh: () => void
  title?: string
  description?: string
  refreshLabel?: string
}) {
  const lines = content ? annotateLines(content) : []
  const legend = [
    { style: 'bg-blue-500/25 text-blue-200', label: 'DNS 接管字段' },
    { style: 'bg-amber-500/25 text-amber-200', label: '端口 / API 接管字段' },
    { style: 'bg-violet-500/25 text-violet-200', label: 'GeoData 接管字段' },
    { style: 'bg-emerald-500/25 text-emerald-200', label: '设备分组接管字段' },
  ]

  return (
    <div className="glass-card px-5 py-5 space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold text-slate-200">{title}</h2>
          <p className="mt-1 text-xs text-muted">{description}</p>
        </div>
        <button
          className="btn-ghost text-xs flex items-center gap-1.5"
          onClick={onRefresh}
          disabled={loading}
        >
          <RotateCw size={12} className={loading ? 'animate-spin' : ''} />
          {loading ? '刷新中…' : refreshLabel}
        </button>
      </div>

      <div className="flex flex-wrap gap-2 items-center">
        {legend.map(l => (
          <span key={l.label} className={`inline-flex text-xs px-2 py-0.5 rounded-md ${l.style}`}>{l.label}</span>
        ))}
      </div>

      {loading && (
        <div className="flex items-center gap-2 text-sm text-muted py-2">
          <Loader2 size={14} className="animate-spin text-brand" />
          正在生成最终配置预览…
        </div>
      )}

      {!loading && error && (
        <div className="rounded-xl border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger">
          {error}
        </div>
      )}

      {!loading && !error && (
        <div className="rounded-xl bg-black/30 border border-white/8 overflow-auto max-h-[26rem] text-xs font-mono select-text">
          {lines.length === 0 && (
            <div className="px-3 py-3 text-muted">当前没有可展示的配置内容</div>
          )}
          {lines.map((ln, i) => (
            <div key={i} className={`flex items-start gap-2 px-2 py-px leading-5 ${CAT_ROW[ln.cat]}`}>
              <span className="select-none text-white/20 w-7 flex-shrink-0 text-right tabular-nums">{i + 1}</span>
              <span className="flex-1 text-slate-200 whitespace-pre">{ln.text || ' '}</span>
              {ln.label && (
                <span className={`flex-shrink-0 text-[10px] pl-3 self-center ${CAT_LABEL[ln.cat]}`}>← {ln.label}</span>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Small UI helpers ─────────────────────────────────────────────────────────

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="grid gap-3 rounded-xl border border-white/[0.06] bg-white/[0.018] px-4 py-3.5 transition-colors hover:border-white/[0.11] sm:grid-cols-[180px_1fr]">
      <div>
        <label className="text-sm font-medium text-slate-200">{label}</label>
        {hint && <p className="mt-1 text-xs leading-5 text-muted">{hint}</p>}
      </div>
      <div className="min-w-0 sm:pt-0.5">{children}</div>
    </div>
  )
}

function TextInput({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder?: string }) {
  return (
    <input
      className="glass-input min-h-11"
      value={value}
      onChange={e => onChange(e.target.value)}
      placeholder={placeholder}
    />
  )
}

function SelectInput({ value, onChange, options }: { value: string; onChange: (v: string) => void; options: { value: string; label: string }[] }) {
  return (
    <select
      className="theme-select glass-input min-h-11 appearance-none cursor-pointer"
      value={value}
      onChange={e => onChange(e.target.value)}
    >
      {options.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
    </select>
  )
}

function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label?: string }) {
  return (
    <div className="flex min-h-11 items-center gap-3">
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        onClick={() => onChange(!checked)}
        className={`relative h-7 w-12 flex-shrink-0 rounded-full border transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-0 cursor-pointer ${checked ? 'border-brand/40 bg-brand shadow-[0_0_18px_rgba(139,92,246,0.25)]' : 'border-white/10 bg-surface-3'}`}
      >
        <span className={`absolute top-0.5 h-6 w-6 rounded-full bg-white shadow transition-all ${checked ? 'left-5' : 'left-0.5'}`} />
      </button>
      {label && <span className={`text-sm font-medium ${checked ? 'text-slate-100' : 'text-muted'}`}>{label}</span>}
    </div>
  )
}

function StepBar({ step }: { step: Step }) {
  const idx = STEPS.findIndex(s => s.id === step)
  return (
    <div className="glass-card px-3 py-3">
      <div className="grid min-w-[720px] grid-cols-5 gap-2">
        {STEPS.map((s, i) => {
          const done = i < idx
          const active = i === idx
          return (
            <div
              key={s.id}
              className={`relative overflow-hidden border px-3 py-2.5 transition-all ${
                active
                  ? 'border-brand/35 bg-brand/[0.10] text-white shadow-[0_0_20px_rgba(139,92,246,0.14)]'
                  : done
                    ? 'border-success/20 bg-success/[0.045] text-success'
                    : 'border-white/[0.06] bg-white/[0.018] text-muted'
              }`}
              style={{ borderRadius: 'var(--radius-md)' }}
            >
              <div className="flex items-center gap-2">
                <span className={`flex h-5 w-5 items-center justify-center rounded-full border text-[10px] font-bold ${
                  done
                    ? 'border-success/25 bg-success/15 text-success'
                    : active
                      ? 'border-brand/40 bg-brand/25 text-brand-light'
                      : 'border-white/12 bg-white/[0.03] text-white/28'
                }`}>
                  {done ? <CheckCircle2 size={12} /> : i + 1}
                </span>
                <span className="truncate text-xs font-semibold">{s.label}</span>
              </div>
              <p className={`mt-1 truncate text-[10px] ${active ? 'text-brand-light/70' : done ? 'text-success/65' : 'text-muted/70'}`}>{STEP_DETAILS[s.id].eyebrow.split(' · ')[1]}</p>
              <div className={`mt-2 h-0.5 rounded-full ${done ? 'bg-success/40' : active ? 'bg-brand/55' : 'bg-white/[0.06]'}`} />
            </div>
          )
        })}
      </div>
    </div>
  )
}

function InfoBadge({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between py-1.5 border-b border-white/5 last:border-0">
      <span className="text-xs text-muted">{label}</span>
      <span className="text-xs text-slate-200 font-mono">{value}</span>
    </div>
  )
}

// ── Browser probe helpers (mirrors Dashboard.tsx) ─────────────────────────────

async function fetchWithTimeout(input: RequestInfo | URL, init: RequestInit, ms: number) {
  const ctrl = new AbortController()
  const t = window.setTimeout(() => ctrl.abort(), ms)
  try { return await fetch(input, { ...init, signal: ctrl.signal }) }
  finally { window.clearTimeout(t) }
}

interface BrowserProbeResult {
  ipOK: boolean; ip?: string; ipError?: string
  accessOK: boolean; accessChecks: { name: string; group?: string; ok: boolean; latency_ms?: number; error?: string }[]
}

async function runBrowserProbe(): Promise<BrowserProbeResult> {
  let ipOK = false, ip: string | undefined, ipError: string | undefined
  try {
    const r = await fetchWithTimeout('https://api.ip.sb/geoip', { cache: 'no-store' }, 7000)
    const d = await r.json() as { ip?: string }
    ipOK = !!d.ip; ip = d.ip
  } catch (e) {
    ipError = e instanceof Error ? e.message : '获取失败'
  }

  const targets = [
    { name: '淘宝',      group: '国内', url: 'https://www.taobao.com' },
    { name: '网易云音乐',  group: '国内', url: 'https://music.163.com' },
    { name: 'Google',    group: '国外', url: 'https://www.google.com' },
    { name: 'GitHub',    group: '国外', url: 'https://github.com' },
    { name: 'OpenAI',    group: 'AI',   url: 'https://chat.openai.com' },
    { name: 'Gemini',    group: 'AI',   url: 'https://gemini.google.com' },
  ]
  const accessChecks = await Promise.all(targets.map(async t => {
    const start = performance.now()
    try {
      await fetchWithTimeout(t.url, { mode: 'no-cors', cache: 'no-store' }, 8000)
      return { name: t.name, group: t.group, ok: true, latency_ms: Math.round(performance.now() - start) }
    } catch (e) {
      return { name: t.name, group: t.group, ok: false, error: e instanceof Error ? e.message : '访问失败' }
    }
  }))
  return { ipOK, ip, ipError, accessOK: accessChecks.every(c => c.ok), accessChecks }
}

// ── Main component ────────────────────────────────────────────────────────────

export function Setup() {
  const navigate = useNavigate()
  const location = useLocation()
  const navState = (location.state as {
    preselectSaved?: { kind: 'file'; filename: string } | { kind: 'sub'; id: string; name: string; url?: string }
    activateSub?: { id: string; name: string; url?: string } // legacy state, keep compatibility
    activateFile?: { filename: string } // legacy state, keep compatibility
  } | null)
  const activateSub = navState?.activateSub
  const activateFile = navState?.activateFile
  const preselectSaved = navState?.preselectSaved
    ?? (activateSub ? { kind: 'sub' as const, id: activateSub.id, name: activateSub.name, url: activateSub.url } : undefined)
    ?? (activateFile ? { kind: 'file' as const, filename: activateFile.filename } : undefined)
  const fileRef = useRef<HTMLInputElement>(null)

  // ── init guard: check if core is already running ──
  const [initStatus, setInitStatus] = useState<InitStatus>('checking')
  const [stopping, setStopping] = useState(false)
  const [stopError, setStopError] = useState('')
  const [runningModules, setRunningModules] = useState<OverviewModule[]>([])

  useEffect(() => {
    getOverviewCore().then(async data => {
      if (data.core.state === 'running') {
        setRunningModules(data.modules ?? [])
        if (activateSub || activateFile) {
          // Auto-stop when switching config via activate
          setStopping(true)
          try {
            await stopCore().catch(() => null)   // idempotent: ignore "already stopped"
            await releaseOverviewTakeover()      // always release DNS/nft
            setInitStatus('ready')
          } catch (e) {
            setStopError(e instanceof Error ? e.message : '停止失败')
            setInitStatus('running')
          } finally {
            setStopping(false)
          }
        } else {
          setInitStatus('running')
        }
      } else {
        setInitStatus('ready')
      }
    }).catch(() => setInitStatus('ready'))
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const handleStopAll = async () => {
    setStopping(true); setStopError('')
    try {
      await stopCore().catch(() => null)   // idempotent: ignore "already stopped" / OOM crash
      await releaseOverviewTakeover()      // always release DNS/nft regardless of core state
      setInitStatus('ready')
    } catch (e) {
      setStopError(e instanceof Error ? e.message : '操作失败')
    } finally { setStopping(false) }
  }

  // ── step state ──
  const [step, setStep] = useState<Step>('import')

  // ── import step ──
  type ImportMode = 'file' | 'paste' | 'url' | 'existing' | 'existing_file' | 'saved'
  const initMode = (): ImportMode => {
    // Keep setup flow consistent: always enter via "已保存配置",
    // then preselect the intended source.
    return 'saved'
  }
  const [importMode, setImportMode] = useState<ImportMode>(initMode)

  // ── saved sources/subs list ──
  const [savedFiles, setSavedFiles] = useState<SourceFile[]>([])
  const [savedSubs, setSavedSubs] = useState<Subscription[]>([])
  const [savedLoading, setSavedLoading] = useState(false)
  const [selectedSaved, setSelectedSaved] = useState<{ kind: 'file'; filename: string } | { kind: 'sub'; sub: Subscription } | null>(null)
  const [subImportChoice, setSubImportChoice] = useState<'cache' | 'live' | null>(null)
  const [subLiveFailed, setSubLiveFailed] = useState(false)
  const [subCacheModalOpen, setSubCacheModalOpen] = useState(false)
  const [subCacheModalLoading, setSubCacheModalLoading] = useState(false)
  const [subCacheModalError, setSubCacheModalError] = useState('')
  const [subCacheModalContent, setSubCacheModalContent] = useState('')
  const [pasteContent, setPasteContent] = useState('')
  const [uploadedFileName, setUploadedFileName] = useState<string | null>(null)
  const [remoteUrl, setRemoteUrl] = useState('')
  const [importing, setImporting] = useState(false)
  const [importError, setImportError] = useState('')
  const [clashParsed, setClashParsed] = useState<ClashParsed | null>(null)

  // ── dns form ──
  const [dns, setDns] = useState<FormDNS>({
    enable: true, mode: 'fake-ip', dnsmasq_mode: 'none',
    apply_on_start: true, listen: '0.0.0.0:7874', ipv6: false,
  })

  // ── network form ──
  const [net, setNet] = useState<FormNetwork>({
    mode: 'tproxy', firewall_backend: 'auto',
    bypass_lan: true, bypass_china: true, apply_on_start: true, ipv6: false,
  })

  // ── import preview ──
  const [previewContent, setPreviewContent] = useState('')

  // ── launch step ──
  const [launching, setLaunching] = useState(false)
  const [launchDone, setLaunchDone] = useState(false)
  const [launchError, setLaunchError] = useState('')
  const [launchLog, setLaunchLog] = useState<LaunchEvent[]>([])
  const [launchConfigPreview, setLaunchConfigPreview] = useState('')
  const [launchConfigLoading, setLaunchConfigLoading] = useState(false)
  const [launchConfigError, setLaunchConfigError] = useState('')
  const [launchPolicyOptions, setLaunchPolicyOptions] = useState<RoutePolicyOptions>({ groups: [], knownProxyNames: [...BUILTIN_PROXY_NAMES] })
  const [launchPolicyError, setLaunchPolicyError] = useState('')
  const [launchDeviceGroups, setLaunchDeviceGroups] = useState<DeviceRouteGroup[]>([])
  const [launchDeviceSnapshot, setLaunchDeviceSnapshot] = useState('[]')
  const [launchDeviceLoading, setLaunchDeviceLoading] = useState(false)
  const [launchDeviceSaving, setLaunchDeviceSaving] = useState(false)
  const [launchDeviceError, setLaunchDeviceError] = useState('')
  const [launchDeviceNotice, setLaunchDeviceNotice] = useState('')
  const [runningConfigPreview, setRunningConfigPreview] = useState('')
  const [runningConfigLoading, setRunningConfigLoading] = useState(false)
  const [runningConfigError, setRunningConfigError] = useState('')

  // ── port check step (after launch, before connectivity check) ──
  const [portChecking, setPortChecking] = useState(false)
  const [portChecks, setPortChecks] = useState<SetupPortCheck[] | null>(null)
  const portCheckAllOk = portChecks !== null && portChecks.every(c => c.ok)

  // ── check step ──
  const [checking, setChecking] = useState(false)
  const [routerProbe, setRouterProbe] = useState<OverviewProbeData | null>(null)
  const [browserProbe, setBrowserProbe] = useState<BrowserProbeResult | null>(null)
  const [probeLogs, setProbeLogs] = useState<LogEntry[]>([])
  const [checkDone, setCheckDone] = useState(false)

  // ── completion ──
  const [autoStartCore, setAutoStartCore] = useState(true)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')

  // ── Load existing ClashForge config to pre-fill forms ──
  useEffect(() => {
    getConfig().then(cfg => {
      const c = cfg as Record<string, Record<string, unknown>>
      if (c.dns) {
        setDns(prev => ({
          ...prev,
          enable: c.dns?.enable !== undefined ? Boolean(c.dns.enable) : prev.enable,
          mode: String(c.dns?.mode || prev.mode),
          dnsmasq_mode: String(c.dns?.dnsmasq_mode || prev.dnsmasq_mode),
          apply_on_start: c.dns?.apply_on_start !== undefined ? Boolean(c.dns.apply_on_start) : prev.apply_on_start,
        }))
      }
      if (c.network) {
        setNet(prev => ({
          ...prev,
          mode: String(c.network?.mode || prev.mode),
          firewall_backend: String(c.network?.firewall_backend || prev.firewall_backend),
          bypass_lan: c.network?.bypass_lan !== undefined ? Boolean(c.network.bypass_lan) : prev.bypass_lan,
          bypass_china: c.network?.bypass_china !== undefined ? Boolean(c.network.bypass_china) : prev.bypass_china,
          apply_on_start: c.network?.apply_on_start !== undefined ? Boolean(c.network.apply_on_start) : prev.apply_on_start,
          ipv6: c.network?.ipv6 !== undefined ? Boolean(c.network.ipv6) : prev.ipv6,
        }))
      }
    }).catch(() => null)
  }, [])

  // ── load saved sources when on saved tab ──
  useEffect(() => {
    if (importMode !== 'saved') return
    setSavedLoading(true)
    Promise.all([
      getSources().catch(() => ({ files: [] as SourceFile[], active_source: null })),
      getSubscriptions().catch(() => ({ subscriptions: [] as Subscription[] })),
    ]).then(([s, sub]) => {
      setSavedFiles(s.files ?? [])
      setSavedSubs(sub.subscriptions ?? [])
    }).finally(() => setSavedLoading(false))
  }, [importMode])

  // If we navigated from "流量规则 -> 启动", preselect that target while still
  // keeping the user in the normal saved-source setup flow.
  useEffect(() => {
    if (importMode !== 'saved' || !preselectSaved) return
    if (preselectSaved.kind === 'file') {
      const exists = savedFiles.some((f) => f.filename === preselectSaved.filename)
      if (exists) {
        setSelectedSaved((prev) =>
          prev?.kind === 'file' && prev.filename === preselectSaved.filename
            ? prev
            : { kind: 'file', filename: preselectSaved.filename },
        )
      }
      return
    }
    const matched = savedSubs.find((s) => s.id === preselectSaved.id)
    if (matched) {
      setSelectedSaved((prev) =>
        prev?.kind === 'sub' && prev.sub.id === matched.id
          ? prev
          : { kind: 'sub', sub: matched },
      )
    }
  }, [importMode, preselectSaved, savedFiles, savedSubs])

  // ── reset subscription import choice when selection changes ──
  useEffect(() => {
    setSubImportChoice(null)
    setSubLiveFailed(false)
    setSubCacheModalOpen(false)
    setSubCacheModalLoading(false)
    setSubCacheModalError('')
    setSubCacheModalContent('')
  }, [selectedSaved])

  // ── helpers ──
  const dnsSet = useCallback(<K extends keyof FormDNS>(k: K, v: FormDNS[K]) =>
    setDns(prev => ({ ...prev, [k]: v })), [])
  const netSet = useCallback(<K extends keyof FormNetwork>(k: K, v: FormNetwork[K]) =>
    setNet(prev => ({ ...prev, [k]: v })), [])

  const buildLaunchPayload = useCallback(() => ({
    dns: { enable: dns.enable, mode: dns.mode, dnsmasq_mode: dns.dnsmasq_mode, apply_on_start: dns.apply_on_start },
    network: { mode: net.mode, firewall_backend: net.firewall_backend, bypass_lan: net.bypass_lan, bypass_china: net.bypass_china, apply_on_start: net.apply_on_start, ipv6: net.ipv6 },
  }), [dns, net])

  const refreshLaunchConfigPreview = useCallback(async () => {
    setLaunchConfigLoading(true)
    setLaunchConfigError('')
    setLaunchPolicyError('')
    try {
      const { content } = await previewSetupFinalConfig(buildLaunchPayload())
      setLaunchConfigPreview(content)
      const parsed = parseRoutePolicyOptions(content)
      setLaunchPolicyOptions(parsed)
      if (content.trim() && parsed.groups.length === 0) {
        setLaunchPolicyError('当前即将运行的配置中未发现可覆盖的策略组或可选节点。')
      }
    } catch (e: unknown) {
      setLaunchPolicyOptions({ groups: [], knownProxyNames: [...BUILTIN_PROXY_NAMES] })
      setLaunchPolicyError('读取预览失败，暂时无法获取可绑定节点列表。')
      setLaunchConfigError(e instanceof Error ? e.message : String(e))
    } finally {
      setLaunchConfigLoading(false)
    }
  }, [buildLaunchPayload])

  const handleViewCachedSubscription = useCallback(async (sub: Subscription) => {
    setSubCacheModalOpen(true)
    setSubCacheModalLoading(true)
    setSubCacheModalError('')
    setSubCacheModalContent('')
    try {
      const { content } = await getSubscriptionCache(sub.id)
      setSubCacheModalContent(content)
    } catch (e: unknown) {
      setSubCacheModalError(e instanceof Error ? e.message : String(e))
    } finally {
      setSubCacheModalLoading(false)
    }
  }, [])

  const refreshRunningConfigPreview = useCallback(async () => {
    setRunningConfigLoading(true)
    setRunningConfigError('')
    try {
      const { content } = await getMihomoConfig()
      setRunningConfigPreview(content)
    } catch (e: unknown) {
      setRunningConfigPreview('')
      setRunningConfigError(e instanceof Error ? e.message : '读取运行配置失败')
    } finally {
      setRunningConfigLoading(false)
    }
  }, [])

  const loadLaunchDeviceGroups = useCallback(async () => {
    setLaunchDeviceLoading(true)
    setLaunchDeviceError('')
    setLaunchDeviceNotice('')
    try {
      const { device_groups } = await getDeviceGroups()
      const normalized = normalizeDeviceGroupsForSetup(device_groups ?? [])
      setLaunchDeviceGroups(normalized)
      setLaunchDeviceSnapshot(serializeDeviceGroupsForSetup(normalized))
    } catch (e: unknown) {
      setLaunchDeviceGroups([])
      setLaunchDeviceSnapshot('[]')
      setLaunchDeviceError(e instanceof Error ? e.message : '读取设备路由配置失败')
    } finally {
      setLaunchDeviceLoading(false)
    }
  }, [])

  const setLaunchGroupField = useCallback((groupID: string, updater: (group: DeviceRouteGroup) => DeviceRouteGroup) => {
    setLaunchDeviceGroups((prev) => prev.map((group) => (group.id === groupID ? updater(group) : group)))
  }, [])

  const removeLaunchOverride = useCallback((groupID: string, overrideIndex: number) => {
    setLaunchGroupField(groupID, (current) => ({
      ...current,
      overrides: current.overrides.filter((_, idx) => idx !== overrideIndex),
    }))
  }, [setLaunchGroupField])

  const syncLaunchDeviceGroups = useCallback(async () => {
    setLaunchDeviceError('')
    setLaunchDeviceNotice('')
    setLaunchDeviceSaving(true)
    try {
      const sanitized = sanitizeDeviceGroupsForSetup(launchDeviceGroups)
      const resp = await updateDeviceGroups(sanitized)
      setLaunchDeviceGroups(sanitized)
      setLaunchDeviceSnapshot(serializeDeviceGroupsForSetup(sanitized))

      if (!resp.config_generated) {
        setLaunchDeviceError(resp.warning || '设备路由已保存，但最终配置生成失败。')
        return false
      }

      if (resp.core_running === false) {
        setLaunchDeviceNotice('设备路由代理绑定已同步，启动后会自动生效。')
      } else if (resp.core_reloaded) {
        setLaunchDeviceNotice('设备路由代理绑定已同步，并已热加载到运行中的内核。')
      } else if (resp.reload_error) {
        setLaunchDeviceError(`设备路由已同步，但热加载失败：${resp.reload_error}`)
      } else {
        setLaunchDeviceNotice('设备路由代理绑定已同步到“设备路由”页面。')
      }

      await refreshLaunchConfigPreview()
      return true
    } catch (e: unknown) {
      setLaunchDeviceError(e instanceof Error ? e.message : '同步设备路由失败')
      return false
    } finally {
      setLaunchDeviceSaving(false)
    }
  }, [launchDeviceGroups, refreshLaunchConfigPreview])

  // ── fill forms from parsed Clash YAML ──
  const applyClashParsed = useCallback((parsed: ClashParsed) => {
    setClashParsed(parsed)
    if (parsed.dns) {
      setDns(prev => ({
        ...prev,
        enable: parsed.dns?.enable !== undefined ? parsed.dns.enable : prev.enable,
        mode: parsed.dns?.['enhanced-mode'] === 'redir-host' ? 'redir-host' : 'fake-ip',
        ipv6: parsed.dns?.ipv6 !== undefined ? parsed.dns.ipv6 : prev.ipv6,
        listen: parsed.dns?.listen || prev.listen,
      }))
    }
  }, [])

  // ── import: file upload ──
  const handleFile = useCallback((file: File) => {
    const reader = new FileReader()
    reader.onload = ev => {
      const content = (ev.target?.result as string) || ''
      setPasteContent(content)
      setUploadedFileName(file.name)
      setImportMode('file')
    }
    reader.readAsText(file)
  }, [])

  // ── import: save overrides + generate + parse ──
  const handleImport = useCallback(async () => {
    setImporting(true); setImportError('')
    try {
      const showPreview = async () => {
        const { content } = await getMihomoConfig().catch(() => ({ content: '' }))
        setPreviewContent(content)
      }

      // Saved config/sub selection mode
      if (importMode === 'saved') {
        if (!selectedSaved) { setImportError('请选择一个配置'); return }
        if (selectedSaved.kind === 'file') {
          const { content } = await getSourceFile(selectedSaved.filename)
          try {
            const parsed = yaml.load(content) as ClashParsed
            if (parsed && typeof parsed === 'object') applyClashParsed(parsed)
          } catch { /* ignore */ }
          await setActiveSource({ type: 'file', filename: selectedSaved.filename })
          await updateOverrides(content)
          await generateConfig()
          setClashParsed({})
        } else {
          const sub = selectedSaved.sub
          // Default: use local cache if available (avoids network dead-loop).
          const wantLive = subImportChoice === 'live'
          const hasCache = !!sub.has_cache
          if (wantLive || !hasCache) {
            try {
              await syncSubUpdate(sub.id)
            } catch (e: unknown) {
              if (hasCache) {
                setSubLiveFailed(true)
                setImportError('在线更新失败，请选择"使用本地缓存"继续。（' + (e instanceof Error ? e.message : String(e)) + '）')
                return
              }
              throw e
            }
          } else {
            // Using cached version — no network fetch needed
          }
          await setActiveSource({ type: 'subscription', sub_id: sub.id, sub_name: sub.name })
          await generateConfig()
          setClashParsed({})
        }
        await showPreview()
        return
      }

      // Existing subscription mode: download (sync) + generate
      if (importMode === 'existing' && activateSub) {
        await syncSubUpdate(activateSub.id)
        await setActiveSource({ type: 'subscription', sub_id: activateSub.id, sub_name: activateSub.name })
        await generateConfig()
        setClashParsed({})
        await showPreview()
        return
      }
      // Existing source file mode: load content from disk
      if (importMode === 'existing_file' && activateFile) {
        const { content } = await getSourceFile(activateFile.filename)
        try {
          const parsed = yaml.load(content) as ClashParsed
          if (parsed && typeof parsed === 'object') applyClashParsed(parsed)
        } catch {
          // ignore YAML parse errors – backend validates
        }
        await setActiveSource({ type: 'file', filename: activateFile.filename })
        await updateOverrides(content)
        await generateConfig()
        setClashParsed({})
        await showPreview()
        return
      }
      if (importMode === 'url') {
        // Reuse existing subscription with same URL instead of creating a duplicate.
        if (!remoteUrl.trim()) { setImportError('请输入订阅链接'); return }
        const subName = (() => { try { return new URL(remoteUrl).hostname } catch { return '远程订阅' } })()
        const existing = await getSubscriptions().catch(() => ({ subscriptions: [] }))
        const matched = existing.subscriptions.find(s => s.url === remoteUrl.trim())
        const subId = matched
          ? matched.id
          : (await addSubscription({ name: subName, url: remoteUrl, type: 'clash', enabled: true })).id
        await syncSubUpdate(subId)
        await setActiveSource({ type: 'subscription', sub_id: subId, sub_name: subName })
        await generateConfig()
        setClashParsed({})
        await showPreview()
        return
      }
      const yamlContent = pasteContent
      if (!yamlContent.trim()) { setImportError('内容为空，请粘贴或上传配置文件'); return }

      // Parse client-side for form pre-fill
      try {
        const parsed = yaml.load(yamlContent) as ClashParsed
        if (parsed && typeof parsed === 'object') applyClashParsed(parsed)
      } catch {
        // ignore YAML parse errors – backend validates
      }

      // Save source file (paste → auto date+ver; upload → original filename)
      const suggestedName = (importMode === 'file' && uploadedFileName) ? uploadedFileName : undefined
      const { filename } = await saveSource(yamlContent, suggestedName).catch(() => ({ filename: '' }))
      if (filename) {
        await setActiveSource({ type: 'file', filename })
      }

      // Save as overrides and generate
      await updateOverrides(yamlContent)
      await generateConfig()
      await showPreview()
    } catch (e: unknown) {
      setImportError(e instanceof Error ? e.message : String(e))
    } finally { setImporting(false) }
  }, [importMode, activateSub, activateFile, pasteContent, uploadedFileName, remoteUrl, applyClashParsed, selectedSaved, subImportChoice])

  // ── port check: verify each managed port after launch ──
  const handlePortCheck = useCallback(async () => {
    setPortChecking(true)
    setPortChecks(null)
    try {
      const { checks } = await checkSetupPorts()
      setPortChecks(checks)
    } catch (e) {
      setPortChecks([{
        name: '端口检测请求失败',
        description: e instanceof Error ? e.message : '未知错误',
        port: 0,
        required: true,
        ok: false,
        error: e instanceof Error ? e.message : '未知错误',
      }])
    } finally {
      setPortChecking(false)
    }
  }, [])

  const launchDeviceDirty = useMemo(
    () => serializeDeviceGroupsForSetup(launchDeviceGroups) !== launchDeviceSnapshot,
    [launchDeviceGroups, launchDeviceSnapshot],
  )

  const launchGroupNamePrefixes = useMemo(
    () => launchDeviceGroups.map((group) => group.name.trim()).filter(Boolean),
    [launchDeviceGroups],
  )

  const launchPolicyGroups = useMemo(() => (
    launchPolicyOptions.groups.filter((group) => {
      for (const prefix of launchGroupNamePrefixes) {
        if (group.name.startsWith(`${prefix} - `)) return false
      }
      return true
    })
  ), [launchGroupNamePrefixes, launchPolicyOptions.groups])

  const launchPolicyGroupMap = useMemo(
    () => new Map(launchPolicyGroups.map((group) => [group.name, group])),
    [launchPolicyGroups],
  )

  const launchKnownProxySet = useMemo(
    () => new Set(launchPolicyOptions.knownProxyNames),
    [launchPolicyOptions.knownProxyNames],
  )

  const addLaunchOverride = useCallback((groupID: string) => {
    setLaunchGroupField(groupID, (current) => {
      const used = new Set(current.overrides.map((item) => item.original_group))
      const preferred = launchPolicyGroups.find((item) => !used.has(item.name)) ?? launchPolicyGroups[0]
      if (!preferred) return current
      return {
        ...current,
        overrides: [...current.overrides, {
          original_group: preferred.name,
          proxies: preferred.proxies.length > 0 ? [preferred.proxies[0]] : [],
        }],
      }
    })
  }, [launchPolicyGroups, setLaunchGroupField])

  // ── launch (streaming SSE from POST /api/v1/setup/launch) ──
  const logEndRef = useRef<HTMLDivElement>(null)
  const handleLaunch = useCallback(async () => {
    setLaunchError('')
    if (launchDeviceLoading) {
      setLaunchError('设备路由配置仍在加载，请稍候后再启动。')
      return
    }
    if (launchDeviceDirty) {
      const ok = await syncLaunchDeviceGroups()
      if (!ok) {
        setLaunchError('设备路由配置同步失败，请先处理后再启动服务。')
        return
      }
    }

    setLaunching(true)
    setLaunchLog([])
    setLaunchDone(false)

    const secret = localStorage.getItem('cf_secret') || ''
    let res: Response
    try {
      res = await fetch('/api/v1/setup/launch', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(secret ? { Authorization: `Bearer ${secret}` } : {}),
        },
        body: JSON.stringify(buildLaunchPayload()),
      })
    } catch (e) {
      setLaunchError(e instanceof Error ? e.message : '无法连接到服务器')
      setLaunching(false)
      return
    }

    const reader = res.body?.getReader()
    if (!reader) {
      setLaunchError('服务器未返回数据流')
      setLaunching(false)
      return
    }

    const decoder = new TextDecoder()
    let buf = ''
    try {
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        const lines = buf.split('\n')
        buf = lines.pop() ?? ''
        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          try {
            const ev: LaunchEvent = JSON.parse(line.slice(6))
            setLaunchLog(prev => [...prev, ev])
            setTimeout(() => logEndRef.current?.scrollIntoView({ behavior: 'smooth' }), 50)
            if (ev.type === 'done') {
              setLaunchDone(ev.success ?? false)
              if (!ev.success) setLaunchError(ev.error ?? '启动失败')
              setLaunching(false)
              void refreshLaunchConfigPreview()
              if (ev.success) {
                // Auto-run port check after brief settle delay
                setTimeout(() => { void handlePortCheck() }, 800)
              }
              return
            }
          } catch { /* ignore unparseable line */ }
        }
      }
    } catch (e) {
      setLaunchError(e instanceof Error ? e.message : '数据流读取错误')
    } finally {
      setLaunching(false)
    }
  }, [buildLaunchPayload, handlePortCheck, launchDeviceDirty, launchDeviceLoading, refreshLaunchConfigPreview, syncLaunchDeviceGroups])

  // ── prepare final runtime config preview when entering launch step ──
  useEffect(() => {
    if (step !== 'launch') return
    void Promise.all([refreshLaunchConfigPreview(), loadLaunchDeviceGroups()])
  }, [loadLaunchDeviceGroups, refreshLaunchConfigPreview, step])

  useEffect(() => {
    if (initStatus !== 'running') return
    void refreshRunningConfigPreview()
  }, [initStatus, refreshRunningConfigPreview])

  // ── complete (save autostart + navigate) ──
  const handleComplete = useCallback(async () => {
    setSaving(true); setSaveError('')
    try {
      const cfg = await getConfig()
      const updated = {
        ...cfg,
        core: {
          ...(cfg as Record<string, unknown>).core as Record<string, unknown>,
          auto_start_core: autoStartCore,
        },
      }
      await updateConfig(updated as Record<string, unknown>)
      if (autoStartCore) {
        await enableService().catch(() => null)
      }
      navigate('/')
    } catch (e: unknown) {
      setSaveError(e instanceof Error ? e.message : '保存失败')
    } finally { setSaving(false) }
  }, [autoStartCore, navigate])

  // ── connectivity check ──
  const handleCheck = useCallback(async () => {
    setChecking(true); setRouterProbe(null); setBrowserProbe(null); setProbeLogs([])
    try {
      const [rp, bp] = await Promise.all([
        getOverviewProbes().catch(() => null),
        runBrowserProbe(),
      ])
      setRouterProbe(rp)
      setBrowserProbe(bp)

      const routerOK = rp ? rp.ip_checks.some(c => c.ok) : false
      const browserOK = bp.ipOK

      if (!routerOK || !browserOK) {
        const { logs } = await getLogs('info', 50)
        setProbeLogs(logs)
      }
      setCheckDone(true)
    } finally { setChecking(false) }
  }, [])

  const overallOK = routerProbe
    ? routerProbe.ip_checks.some(c => c.ok) && (browserProbe?.ipOK ?? false)
    : false

  // ── Render steps ──────────────────────────────────────────────────────────

  // Guard: checking
  if (initStatus === 'checking') {
    return (
      <div className="min-h-full bg-gradient-to-b from-surface-0 to-surface-1 flex items-center justify-center">
        <div className="flex items-center gap-3 text-muted text-sm">
          <Loader2 size={18} className="animate-spin text-brand" />
          正在检测当前运行状态…
        </div>
      </div>
    )
  }

  // Guard: core is running — require stopping before re-running wizard
  if (initStatus === 'running') {
    const managed = runningModules.filter(m => m.managed_by_clashforge)
    return (
      <div className="min-h-full bg-gradient-to-b from-surface-0 to-surface-1 px-6 py-8">
        <div className="mx-auto max-w-6xl space-y-6">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-xl bg-brand/20 flex items-center justify-center">
              <Sparkles size={18} className="text-brand" />
            </div>
            <div>
              <div>
                <p className="text-[11px] uppercase tracking-[0.24em] text-muted">Proxy</p>
                <h1 className="text-base font-bold text-white mt-1">代理服务</h1>
              </div>
              <p className="text-xs text-muted">重新配置前需要先停止当前服务</p>
            </div>
          </div>

          <div className="grid gap-6 xl:grid-cols-[420px_minmax(0,1fr)]">
            <div className="glass-card px-5 py-5 space-y-4">
              <div className="flex items-center gap-3">
                <span className="inline-flex h-2.5 w-2.5 rounded-full bg-success animate-pulse" />
                <p className="text-sm font-semibold text-white">内核正在运行</p>
              </div>
              <p className="text-sm text-muted leading-6">
                ClashForge 内核当前处于运行状态，并已接管以下系统服务。
                要重新配置，请先停止内核并退出所有接管，然后再继续。
              </p>

              {managed.length > 0 && (
                <div className="rounded-xl border border-white/8 bg-black/10 px-4 py-3 space-y-2">
                  <p className="text-xs text-muted uppercase tracking-wider font-semibold">当前已接管</p>
                  {managed.map(m => (
                    <div key={m.id} className="flex items-center gap-2 text-xs">
                      <ShieldOff size={12} className="text-warning" />
                      <span className="text-slate-300">{m.title}</span>
                      <span className="text-muted">— {m.current_owner}</span>
                    </div>
                  ))}
                </div>
              )}

              {stopError && (
                <div className="flex items-center gap-2 text-xs text-danger">
                  <AlertCircle size={13} />{stopError}
                </div>
              )}

              <button
                className="w-full flex items-center justify-center gap-2.5 py-4 text-sm font-bold rounded-xl
                         bg-danger/90 hover:bg-danger text-white border border-danger/60 hover:border-danger
                         shadow-lg shadow-danger/30 transition-all active:scale-[0.98] disabled:opacity-60"
                onClick={handleStopAll}
                disabled={stopping}
              >
                {stopping
                  ? <><Loader2 size={16} className="animate-spin" />停止中…</>
                  : <><PowerOff size={16} />停止内核 + 退出所有接管</>}
              </button>
            </div>

            <div className="space-y-4">
              <LaunchConfigPreview
                content={runningConfigPreview}
                loading={runningConfigLoading}
                error={runningConfigError}
                onRefresh={() => { void refreshRunningConfigPreview() }}
                title="正在运行配置预览"
                description="展示当前完整的运行配置（mihomo-config.yaml）；高亮区域为 ClashForge 接管和统一设置项（含设备分组规则），行尾有具体说明。"
                refreshLabel="刷新运行配置"
              />
              <div className="rounded-xl border border-white/10 bg-black/20 px-4 py-3 text-xs text-muted leading-6">
                蓝色高亮表示 DNS 接管字段，橙色高亮表示端口与 API 接管字段，紫色高亮表示 GeoData 接管字段，
                绿色高亮表示设备分组接管字段（影子策略组、设备 RULE-SET、AND 规则与对应 provider）。这些字段会由 ClashForge 统一维护，避免运行时配置漂移。
              </div>
            </div>
          </div>
        </div>
      </div>
    )
  }

  const activeStep = STEP_DETAILS[step]
  const progress = ((STEPS.findIndex(s => s.id === step) + 1) / STEPS.length) * 100

  return (
    <div className="relative min-h-full overflow-hidden px-4 py-4 sm:px-6 lg:px-8">
      <div className="pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(circle_at_20%_0%,rgba(139,92,246,0.16),transparent_34%),radial-gradient(circle_at_90%_8%,rgba(249,115,22,0.10),transparent_30%),linear-gradient(180deg,rgb(var(--surface-0)),rgb(var(--surface-1)))]" />
      <div className="pointer-events-none absolute inset-x-0 top-0 -z-10 h-px bg-gradient-to-r from-transparent via-brand/50 to-transparent" />

      <div className="mx-auto max-w-6xl space-y-5">

        {/* Header */}
        <div className="hero-panel !p-0">
          <div className="absolute inset-0 bg-[linear-gradient(120deg,rgba(139,92,246,0.12),transparent_34%),radial-gradient(circle_at_82%_18%,rgba(34,197,94,0.10),transparent_28%)]" />
          <div className="relative z-10 grid gap-5 p-5 lg:grid-cols-[1fr_320px]">
            <div className="flex min-w-0 flex-col justify-between gap-6">
              <div className="flex items-start gap-4">
                <div className="flex h-12 w-12 flex-shrink-0 items-center justify-center border border-brand/25 bg-brand/[0.10] shadow-[0_0_26px_rgba(139,92,246,0.24)]" style={{ borderRadius: 'var(--radius-lg)' }}>
                  <Sparkles size={22} className="text-brand-light" />
                </div>
                <div className="min-w-0">
                  <p className="text-[10px] font-semibold uppercase tracking-[0.30em] text-brand-light/60">ClashForge Setup</p>
                  <h1 className="mt-1 text-2xl font-bold tracking-tight text-white sm:text-3xl">代理服务向导</h1>
                  <p className="mt-2 max-w-2xl text-sm leading-6 text-muted">把配置导入、DNS、透明代理、启动日志和连通验证收进一个清晰流程。重点操作更醒目，危险状态更早暴露。</p>
                </div>
              </div>

              <div className="grid gap-2 sm:grid-cols-3">
                <div className="rounded-xl border border-white/[0.07] bg-white/[0.035] px-3 py-3">
                  <div className="flex items-center gap-2 text-xs text-muted"><ShieldCheck size={13} className="text-success" /> 安全默认</div>
                  <p className="mt-1 text-sm font-semibold text-slate-100">IPv6 泄露防护</p>
                </div>
                <div className="rounded-xl border border-white/[0.07] bg-white/[0.035] px-3 py-3">
                  <div className="flex items-center gap-2 text-xs text-muted"><Network size={13} className="text-brand-light" /> 路由接管</div>
                  <p className="mt-1 text-sm font-semibold text-slate-100">TProxy / Redir / TUN</p>
                </div>
                <div className="rounded-xl border border-white/[0.07] bg-white/[0.035] px-3 py-3">
                  <div className="flex items-center gap-2 text-xs text-muted"><Gauge size={13} className="text-warning" /> 验证闭环</div>
                  <p className="mt-1 text-sm font-semibold text-slate-100">端口 + 出口 IP</p>
                </div>
              </div>
            </div>

            <div className="rounded-2xl border border-white/[0.08] bg-black/20 p-4 shadow-inner">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <p className="text-[10px] font-semibold uppercase tracking-[0.24em] text-brand-light/65">{activeStep.eyebrow}</p>
                  <h2 className="mt-1 text-lg font-bold text-white">{activeStep.title}</h2>
                </div>
                <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-brand/25 bg-brand/10 text-brand-light">
                  <ServerCog size={18} />
                </div>
              </div>
              <p className="mt-3 text-sm leading-6 text-muted">{activeStep.desc}</p>
              <div className="mt-5">
                <div className="mb-2 flex items-center justify-between text-[11px] text-muted">
                  <span>进度</span>
                  <span>{Math.round(progress)}%</span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-white/[0.06]">
                  <div className="h-full rounded-full bg-gradient-to-r from-brand to-success shadow-[0_0_18px_rgba(139,92,246,0.35)] transition-all duration-300" style={{ width: `${progress}%` }} />
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Step bar */}
        <div className="overflow-x-auto pb-1">
          <StepBar step={step} />
        </div>

        {/* ─── Step 1: Import ─────────────────────────────────────────────── */}
        {step === 'import' && previewContent && (
          <ConfigPreview
            content={previewContent}
            onContinue={() => { setPreviewContent(''); setStep('dns') }}
          />
        )}
        {step === 'import' && !previewContent && (
          <div className="space-y-4">
            {/* Mode tabs */}
            <div className="glass-card px-5 py-5 space-y-4">
              <div className="flex items-center gap-2 mb-1">
                <FileText size={16} className="text-brand" />
                <h2 className="text-sm font-semibold text-slate-200">选择导入方式</h2>
              </div>
              {importMode !== 'existing' && importMode !== 'existing_file' && (
                <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
                  {([
                    { id: 'saved', icon: <Database size={15} />, label: '已保存配置', hint: '从历史文件或订阅继续' },
                    { id: 'paste', icon: <FileText size={15} />, label: '粘贴 YAML', hint: '直接粘贴完整配置' },
                    { id: 'file',  icon: <Upload size={15} />,   label: '上传文件', hint: '.yaml / .yml 本地文件' },
                    { id: 'url',   icon: <Link2 size={15} />,    label: '订阅链接', hint: '拉取远程 Clash 订阅' },
                  ] as const).map(m => {
                    const active = importMode === m.id
                    return (
                      <button
                        key={m.id}
                        type="button"
                        onClick={() => setImportMode(m.id)}
                        className={`group flex min-h-[74px] items-start gap-3 border px-3 py-3 text-left transition-all ${
                          active
                            ? 'border-brand/45 bg-brand/[0.11] shadow-[0_0_18px_rgba(139,92,246,0.16)]'
                            : 'border-white/[0.07] bg-white/[0.025] hover:border-white/[0.14] hover:bg-white/[0.045]'
                        }`}
                        style={{ borderRadius: 'var(--radius-lg)' }}
                      >
                        <span className={`mt-0.5 flex h-8 w-8 flex-shrink-0 items-center justify-center border ${
                          active
                            ? 'border-brand/35 bg-brand/20 text-brand-light'
                            : 'border-white/[0.07] bg-white/[0.035] text-white/38 group-hover:text-white/65'
                        }`} style={{ borderRadius: 'var(--radius-md)' }}>
                          {m.icon}
                        </span>
                        <span className="min-w-0">
                          <span className={`block text-[13px] font-semibold ${active ? 'text-white' : 'text-slate-300'}`}>{m.label}</span>
                          <span className="mt-1 block text-[11px] leading-4 text-muted">{m.hint}</span>
                        </span>
                      </button>
                    )
                  })}
                </div>
              )}

              {importMode === 'saved' && (
                <div className="space-y-3">
                  {savedLoading && <p className="text-xs text-muted">加载中…</p>}
                  {!savedLoading && savedFiles.length === 0 && savedSubs.length === 0 && (
                    <p className="text-xs text-muted">暂无已保存的配置，请使用其他方式导入。</p>
                  )}
                  {savedFiles.length > 0 && (
                    <div className="space-y-1.5">
                      <p className="text-xs font-semibold text-muted uppercase tracking-wider">配置文件</p>
                      {savedFiles.map(f => {
                        const selected = selectedSaved?.kind === 'file' && selectedSaved.filename === f.filename
                        return (
                          <button
                            key={f.filename}
                            onClick={() => setSelectedSaved({ kind: 'file', filename: f.filename })}
                            className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl border text-left transition-all ${selected ? 'border-brand/60 bg-brand/10' : 'border-white/8 bg-black/10 hover:border-white/20'}`}
                          >
                            <FileText size={14} className={selected ? 'text-brand flex-shrink-0' : 'text-muted flex-shrink-0'} />
                            <div className="flex-1 min-w-0">
                              <p className={`text-sm font-mono font-medium truncate ${selected ? 'text-brand' : 'text-slate-200'}`}>{f.filename}</p>
                              <p className="text-xs text-muted mt-0.5">{(f.size_bytes / 1024).toFixed(1)} KB · {new Date(f.created_at).toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</p>
                            </div>
                            {selected && <CheckCircle2 size={14} className="text-brand flex-shrink-0" />}
                          </button>
                        )
                      })}
                    </div>
                  )}
                  {savedSubs.length > 0 && (
                    <div className="space-y-1.5">
                      <p className="text-xs font-semibold text-muted uppercase tracking-wider">订阅配置</p>
                      {savedSubs.map(sub => {
                        const selected = selectedSaved?.kind === 'sub' && selectedSaved.sub.id === sub.id
                        return (
                          <button
                            key={sub.id}
                            onClick={() => setSelectedSaved({ kind: 'sub', sub })}
                            className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl border text-left transition-all ${selected ? 'border-brand/60 bg-brand/10' : 'border-white/8 bg-black/10 hover:border-white/20'}`}
                          >
                            <Radio size={14} className={selected ? 'text-brand flex-shrink-0' : 'text-muted flex-shrink-0'} />
                            <div className="flex-1 min-w-0">
                              <p className={`text-sm font-medium truncate ${selected ? 'text-brand' : 'text-slate-200'}`}>{sub.name}</p>
                              <p className="text-xs text-muted mt-0.5">{sub.node_count ? `${sub.node_count} 节点 · ` : ''}{sub.url ? sub.url : '无 URL'}</p>
                            </div>
                            {selected && <CheckCircle2 size={14} className="text-brand flex-shrink-0" />}
                          </button>
                        )
                      })}
                      {/* Cache vs live-update choice for selected subscription */}
                      {selectedSaved?.kind === 'sub' && selectedSaved.sub.has_cache && (
                        <div className="mt-2 rounded-xl border border-white/10 bg-black/20 px-4 py-3 space-y-2">
                          <p className="text-xs font-semibold text-slate-300">订阅更新方式</p>
                          <div className="flex gap-2">
                            <button
                              onClick={() => { setSubImportChoice('cache'); setSubLiveFailed(false) }}
                              className={`flex-1 flex items-center justify-center gap-1.5 py-2 rounded-lg border text-xs font-medium transition-all ${(subImportChoice === 'cache' || subImportChoice === null) ? 'border-brand/60 bg-brand/15 text-brand' : 'border-white/10 bg-white/5 text-muted hover:border-white/20'}`}
                            >
                              <Database size={12} />使用本地缓存
                            </button>
                            <button
                              onClick={() => { setSubImportChoice('live'); setSubLiveFailed(false) }}
                              className={`flex-1 flex items-center justify-center gap-1.5 py-2 rounded-lg border text-xs font-medium transition-all ${subImportChoice === 'live' ? 'border-brand/60 bg-brand/15 text-brand' : 'border-white/10 bg-white/5 text-muted hover:border-white/20'}`}
                            >
                              <Link2 size={12} />在线更新订阅
                            </button>
                          </div>
                          <button
                            onClick={() => void handleViewCachedSubscription(selectedSaved.sub)}
                            className="w-full flex items-center justify-center gap-1.5 py-2 rounded-lg border border-white/12 bg-white/[0.04] text-xs text-slate-200 hover:border-white/25 hover:bg-white/[0.08] transition-colors"
                          >
                            <Eye size={12} />查看当前缓存配置
                          </button>
                          {(subImportChoice === 'cache' || subImportChoice === null) && selectedSaved.sub.last_updated && (
                            <p className="text-[11px] text-muted">
                              缓存时间：{new Date(selectedSaved.sub.last_updated).toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}
                              {selectedSaved.sub.node_count ? `  ·  ${selectedSaved.sub.node_count} 节点` : ''}
                            </p>
                          )}
                          {subLiveFailed && (
                            <div className="flex items-center gap-2 text-xs text-warning">
                              <AlertCircle size={12} />在线更新失败。请点击"使用本地缓存"继续。
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}

              {importMode === 'existing' && activateSub && (
                <div className="space-y-3">
                  <p className="text-xs text-muted leading-5">将拉取以下订阅的最新节点并重新生成配置。</p>
                  <div className="rounded-xl bg-brand/10 border border-brand/30 px-4 py-3 space-y-1">
                    <p className="text-sm font-semibold text-white">{activateSub.name}</p>
                    {activateSub.url && <p className="text-xs text-muted truncate">{activateSub.url}</p>}
                  </div>
                  <button
                    className="text-xs text-muted hover:text-white underline underline-offset-2 transition-colors"
                    onClick={() => setImportMode('paste')}
                  >
                    切换到手动导入
                  </button>
                </div>
              )}

              {importMode === 'existing_file' && activateFile && (
                <div className="space-y-3">
                  <p className="text-xs text-muted leading-5">将加载以下保存的配置文件并重新生成配置。</p>
                  <div className="rounded-xl bg-brand/10 border border-brand/30 px-4 py-3 space-y-1">
                    <p className="text-sm font-semibold text-white font-mono">{activateFile.filename}</p>
                    <p className="text-xs text-muted">来自配置文件列表</p>
                  </div>
                  <button
                    className="text-xs text-muted hover:text-white underline underline-offset-2 transition-colors"
                    onClick={() => setImportMode('paste')}
                  >
                    切换到手动导入
                  </button>
                </div>
              )}

              {importMode === 'paste' && (
                <div className="space-y-3">
                  <p className="text-xs text-muted leading-5">
                    粘贴完整的 Clash / Mihomo YAML 配置（本地配置文件或订阅下载内容）。
                  </p>
                  <textarea
                    className="w-full bg-surface-2 border border-white/10 rounded-xl px-3 py-3 text-xs text-white font-mono outline-none focus:border-brand transition-colors resize-none"
                    rows={16}
                    placeholder={'port: 7890\nsocks-port: 7891\ndns:\n  enable: true\n  enhanced-mode: fake-ip\n  listen: 0.0.0.0:7874\n  ...'}
                    value={pasteContent}
                    onChange={e => setPasteContent(e.target.value)}
                    spellCheck={false}
                  />
                </div>
              )}

              {importMode === 'file' && (
                <div className="space-y-3">
                  <p className="text-xs text-muted leading-5">上传 .yaml / .yml 格式的配置文件。</p>
                  <div
                    className="border-2 border-dashed border-white/15 rounded-2xl px-6 py-12 flex flex-col items-center gap-3 hover:border-brand/40 hover:bg-brand/5 transition-all cursor-pointer"
                    onClick={() => fileRef.current?.click()}
                    onDragOver={e => e.preventDefault()}
                    onDrop={e => { e.preventDefault(); const f = e.dataTransfer.files[0]; if (f) handleFile(f) }}
                  >
                    <Upload size={28} className="text-muted" />
                    <div className="text-center">
                      <p className="text-sm text-slate-300 font-medium">点击上传或拖放文件</p>
                      <p className="text-xs text-muted mt-1">.yaml / .yml 格式</p>
                    </div>
                    <input
                      ref={fileRef} type="file" accept=".yaml,.yml,.txt" className="hidden"
                      onChange={e => { const f = e.target.files?.[0]; if (f) handleFile(f) }}
                    />
                  </div>
                  {pasteContent && (
                    <div className="rounded-xl bg-success/10 border border-success/20 px-4 py-2 flex items-center gap-2">
                      <CheckCircle2 size={14} className="text-success flex-shrink-0" />
                      <span className="text-xs text-success">文件已加载，共 {pasteContent.split('\n').length} 行</span>
                    </div>
                  )}
                </div>
              )}

              {importMode === 'url' && (
                <div className="space-y-3">
                  <p className="text-xs text-muted leading-5">
                    输入 Clash 订阅链接，后端将自动拉取并解析节点。
                    此方式会创建一条新的订阅记录，后续可在「订阅」页管理。
                  </p>
                  <TextInput value={remoteUrl} onChange={setRemoteUrl} placeholder="https://example.com/clash-subscribe?token=..." />
                </div>
              )}

              {importError && (
                <div className="flex items-center gap-2 text-xs text-danger">
                  <AlertCircle size={13} />{importError}
                </div>
              )}

              <button
                className="btn-primary w-full flex items-center justify-center gap-2"
                onClick={handleImport}
                disabled={
                  importing ||
                  (
                    (importMode === 'saved' && !selectedSaved) ||
                    (importMode === 'existing' && !activateSub) ||
                    (importMode === 'existing_file' && !activateFile) ||
                    (importMode === 'url' && !remoteUrl.trim()) ||
                    ((importMode === 'paste' || importMode === 'file') && !pasteContent.trim())
                  )
                }
              >
                {importing
                  ? <><Loader2 size={14} className="animate-spin" />解析中…</>
                  : <><ArrowRight size={14} />解析并继续</>}
              </button>
            </div>

            <p className="text-xs text-muted text-center">如果还没有配置文件，可以直接跳过 →
              <button className="ml-1 text-brand hover:underline" onClick={() => { setClashParsed({}); setStep('dns') }}>
                跳过导入，手动设置
              </button>
            </p>
          </div>
        )}

        {/* ─── Step 2: DNS ────────────────────────────────────────────────── */}
        {step === 'dns' && (
          <div className="space-y-4">
            {clashParsed?.dns && (
              <div className="glass-card px-5 py-4 bg-brand/5 border-brand/20 space-y-1">
                <p className="text-xs font-semibold text-brand mb-2">从配置文件中读取到 DNS 设置</p>
                {clashParsed.dns.enable !== undefined && <InfoBadge label="DNS 启用" value={String(clashParsed.dns.enable)} />}
                {clashParsed.dns['enhanced-mode'] && <InfoBadge label="DNS 模式" value={clashParsed.dns['enhanced-mode']} />}
                {clashParsed.dns.listen && <InfoBadge label="监听地址" value={clashParsed.dns.listen} />}
                {clashParsed.dns['fake-ip-range'] && <InfoBadge label="fake-ip 段" value={clashParsed.dns['fake-ip-range']} />}
                {(clashParsed.dns.nameserver ?? []).length > 0 && (
                  <InfoBadge label="上游 DNS" value={(clashParsed.dns.nameserver ?? []).join(', ')} />
                )}
              </div>
            )}

            <div className="glass-card px-5 py-5 space-y-5">
              <h2 className="text-sm font-semibold text-slate-200 border-b border-white/5 pb-3">DNS 设置</h2>

              <Field label="启用 Mihomo DNS" hint="关闭时 Mihomo 使用系统 DNS，不接管查询">
                <Toggle checked={dns.enable} onChange={v => dnsSet('enable', v)} label={dns.enable ? '已启用' : '已禁用'} />
              </Field>

              <Field label="DNS 解析模式" hint="fake-ip 模式下虚构 IP 实现规则分流">
                <SelectInput
                  value={dns.mode} onChange={v => dnsSet('mode', v)}
                  options={[{ value: 'fake-ip', label: 'Fake-IP（推荐）' }, { value: 'redir-host', label: 'Redir-Host' }]}
                />
              </Field>

              <Field label="DNS 监听地址">
                <TextInput value={dns.listen} onChange={v => dnsSet('listen', v)} placeholder="0.0.0.0:7874" />
              </Field>

              <Field label="IPv6 DNS" hint="默认关闭，避免 IPv6 泄露">
                <Toggle checked={dns.ipv6} onChange={v => dnsSet('ipv6', v)} label={dns.ipv6 ? '启用' : '禁用'} />
              </Field>

              <div className="border-t border-white/5 pt-4 space-y-4">
                <h3 className="text-xs font-semibold text-muted uppercase tracking-wider">ClashForge DNS 接管</h3>

                <Field label="dnsmasq 共存模式" hint="none=不干预 dnsmasq；upstream=设为上游；replace=完全替换">
                  <SelectInput
                    value={dns.dnsmasq_mode} onChange={v => dnsSet('dnsmasq_mode', v)}
                    options={[
                      { value: 'none',     label: '不干预 dnsmasq（默认）' },
                      { value: 'upstream', label: 'Mihomo 作为 dnsmasq 上游' },
                      { value: 'replace',  label: '完全替换 dnsmasq' },
                    ]}
                  />
                </Field>

                <Field label="启动时接管 DNS" hint="开启后 ClashForge 启动时自动将 DNS 查询引向 Mihomo">
                  <Toggle checked={dns.apply_on_start} onChange={v => dnsSet('apply_on_start', v)} label={dns.apply_on_start ? '是' : '否'} />
                </Field>
              </div>
            </div>

            <div className="flex gap-3">
              <button className="btn-ghost flex-1" onClick={() => setStep('import')}>← 返回</button>
              <button className="btn-primary flex-1 flex items-center justify-center gap-2" onClick={() => setStep('network')}>
                下一步：网络设置 <ChevronRight size={14} />
              </button>
            </div>
          </div>
        )}

        {/* ─── Step 3: Network ────────────────────────────────────────────── */}
        {step === 'network' && (
          <div className="space-y-4">
            {clashParsed && (
              <div className="glass-card px-5 py-4 bg-brand/5 border-brand/20 space-y-1">
                <p className="text-xs font-semibold text-brand mb-2">从配置文件中读取到基本信息</p>
                {clashParsed.mode && <InfoBadge label="代理模式" value={clashParsed.mode} />}
                {clashParsed.port && <InfoBadge label="HTTP 端口" value={String(clashParsed.port)} />}
                {clashParsed['mixed-port'] && <InfoBadge label="混合端口" value={String(clashParsed['mixed-port'])} />}
                {clashParsed['socks-port'] && <InfoBadge label="SOCKS 端口" value={String(clashParsed['socks-port'])} />}
                {clashParsed['allow-lan'] !== undefined && <InfoBadge label="允许局域网" value={String(clashParsed['allow-lan'])} />}
              </div>
            )}

            <div className="glass-card px-5 py-5 space-y-5">
              <h2 className="text-sm font-semibold text-slate-200 border-b border-white/5 pb-3">透明代理 / 网络设置</h2>

              <Field label="透明代理模式" hint="tproxy 适用于 OpenWrt；redir 兼容性更好；tun 模式需内核支持">
                <SelectInput
                  value={net.mode} onChange={v => netSet('mode', v)}
                  options={[
                    { value: 'tproxy', label: 'TProxy（OpenWrt 推荐）' },
                    { value: 'redir',  label: 'Redir-TCP' },
                    { value: 'tun',    label: 'TUN' },
                    { value: 'none',   label: '不接管（仅启动内核）' },
                  ]}
                />
              </Field>

              <Field label="防火墙后端" hint="auto 会自动探测 nftables/iptables">
                <SelectInput
                  value={net.firewall_backend} onChange={v => netSet('firewall_backend', v)}
                  options={[
                    { value: 'auto',      label: '自动探测' },
                    { value: 'nftables',  label: 'nftables' },
                    { value: 'iptables',  label: 'iptables' },
                    { value: 'none',      label: '不配置防火墙' },
                  ]}
                />
              </Field>

              <Field label="绕过局域网" hint="局域网流量不走透明代理">
                <Toggle checked={net.bypass_lan} onChange={v => netSet('bypass_lan', v)} label={net.bypass_lan ? '是' : '否'} />
              </Field>

              <Field label="绕过中国大陆 IP" hint="国内 IP 直连，减少延迟">
                <Toggle checked={net.bypass_china} onChange={v => netSet('bypass_china', v)} label={net.bypass_china ? '是' : '否'} />
              </Field>

              <Field label="启动时自动接管流量" hint="开启后 ClashForge 启动时立即应用透明代理规则">
                <Toggle checked={net.apply_on_start} onChange={v => netSet('apply_on_start', v)} label={net.apply_on_start ? '是' : '否'} />
              </Field>

              <Field label="IPv6 透明代理" hint="同时拦截 IPv6 流量，防止浏览器优先走 IPv6 绕过代理（路由器需有公网 IPv6 才有效）">
                <Toggle checked={net.ipv6} onChange={v => netSet('ipv6', v)} label={net.ipv6 ? '开启' : '关闭'} />
              </Field>
            </div>

            <div className="flex gap-3">
              <button className="btn-ghost flex-1" onClick={() => setStep('dns')}>← 返回</button>
              <button className="btn-primary flex-1 flex items-center justify-center gap-2" onClick={() => setStep('launch')}>
                下一步：启动服务 <ChevronRight size={14} />
              </button>
            </div>
          </div>
        )}

        {/* ─── Step 4: Launch ─────────────────────────────────────────────── */}
        {step === 'launch' && (
          <div className="space-y-4">
            <div className="glass-card px-5 py-5 space-y-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <h2 className="text-sm font-semibold text-slate-200">设备路由代理校准</h2>
                  <p className="mt-1 text-xs text-muted">
                    如果你已经配置了设备分组，这里可以按“当前即将运行配置”重新绑定代理。同步后会直接更新到设备路由页面。
                  </p>
                </div>
                <button
                  className="btn-ghost text-xs flex items-center gap-1.5"
                  onClick={() => { void loadLaunchDeviceGroups() }}
                  disabled={launchDeviceLoading || launchDeviceSaving}
                >
                  <RotateCw size={12} className={launchDeviceLoading ? 'animate-spin' : ''} />
                  {launchDeviceLoading ? '加载中…' : '刷新分组'}
                </button>
              </div>

              {launchPolicyError && (
                <div className="rounded-xl border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">
                  {launchPolicyError}
                </div>
              )}
              {launchDeviceError && (
                <div className="rounded-xl border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger">
                  {launchDeviceError}
                </div>
              )}
              {launchDeviceNotice && (
                <div className="rounded-xl border border-success/30 bg-success/10 px-3 py-2 text-xs text-success">
                  {launchDeviceNotice}
                </div>
              )}

              {launchDeviceLoading ? (
                <div className="flex items-center gap-2 text-sm text-muted py-2">
                  <Loader2 size={14} className="animate-spin text-brand" />
                  正在加载设备分组…
                </div>
              ) : launchDeviceGroups.length === 0 ? (
                <div className="rounded-xl border border-dashed border-white/15 bg-black/20 px-3 py-4 text-xs text-muted">
                  当前还没有设备分组配置。你可以在“设备路由”页面创建分组后，再回到这里做启动前校准。
                </div>
              ) : (
                <div className="space-y-3">
                  {launchDeviceGroups.map((group, groupIndex) => (
                    <div key={group.id || `launch-group-${groupIndex}`} className="rounded-xl border border-white/10 bg-black/20 px-3 py-3 space-y-3">
                      <div className="flex items-center justify-between gap-3">
                        <div className="min-w-0">
                          <p className="text-sm font-semibold text-slate-100 truncate">
                            {group.name || `未命名分组 ${groupIndex + 1}`}
                          </p>
                          <p className="mt-0.5 text-[11px] text-muted">
                            {group.devices.length} 台设备 · {group.overrides.length} 条策略覆盖
                          </p>
                        </div>
                        <button
                          className="btn-ghost h-7 px-2.5 text-xs"
                          onClick={() => addLaunchOverride(group.id)}
                          disabled={launchPolicyGroups.length === 0}
                        >
                          <Plus size={12} />
                          添加覆盖
                        </button>
                      </div>

                      {group.overrides.length === 0 ? (
                        <p className="rounded-lg border border-dashed border-white/15 px-3 py-3 text-xs text-muted">
                          {launchPolicyGroups.length === 0 ? '当前配置中没有可覆盖策略组。' : '该分组暂未配置策略覆盖，可点击右上角“添加覆盖”。'}
                        </p>
                      ) : (
                        group.overrides.map((override, overrideIndex) => {
                          const selectedGroup = launchPolicyGroupMap.get(override.original_group)
                          const knownMembers = selectedGroup?.proxies ?? []
                          const memberOptions = dedupe([...knownMembers, ...override.proxies])

                          return (
                            <div key={`${group.id}-override-${overrideIndex}`} className="rounded-lg border border-white/10 bg-black/25 px-3 py-3 space-y-2.5">
                              <div className="flex items-start gap-2">
                                <select
                                  className="theme-select glass-input min-h-[34px] h-[34px] w-full"
                                  value={override.original_group}
                                  onChange={(event) => {
                                    const nextGroupName = event.target.value
                                    const nextMembers = new Set(launchPolicyGroupMap.get(nextGroupName)?.proxies ?? [])
                                    setLaunchGroupField(group.id, (current) => ({
                                      ...current,
                                      overrides: current.overrides.map((item, idx) => {
                                        if (idx !== overrideIndex) return item
                                        const cleaned = item.proxies.filter((proxy) => nextMembers.has(proxy))
                                        return {
                                          ...item,
                                          original_group: nextGroupName,
                                          proxies: cleaned,
                                        }
                                      }),
                                    }))
                                  }}
                                >
                                  <option value="">选择策略组</option>
                                  {launchPolicyGroups.map((item) => (
                                    <option key={item.name} value={item.name}>
                                      {item.name}
                                    </option>
                                  ))}
                                  {override.original_group && !launchPolicyGroupMap.has(override.original_group) ? (
                                    <option value={override.original_group}>
                                      {override.original_group}（失效引用）
                                    </option>
                                  ) : null}
                                </select>
                                <button
                                  className="btn-ghost btn-icon-sm h-[34px] w-[34px] min-w-[34px] flex-shrink-0 text-danger hover:bg-danger/10"
                                  title="删除覆盖"
                                  onClick={() => removeLaunchOverride(group.id, overrideIndex)}
                                >
                                  <Trash2 size={13} />
                                </button>
                              </div>

                              {!selectedGroup && (
                                <p className="text-[11px] text-warning">
                                  当前配置中未找到策略组“{override.original_group}”，请先在配置中确认该策略组仍存在。
                                </p>
                              )}

                              {memberOptions.length === 0 ? (
                                <p className="text-xs text-muted">
                                  {override.original_group ? '当前策略组没有可选代理节点。' : '先选择策略组后再选节点。'}
                                </p>
                              ) : (
                                <div className="flex flex-wrap gap-2">
                                  {memberOptions.map((member) => {
                                    const selected = override.proxies.includes(member)
                                    const known = launchKnownProxySet.has(member)
                                    return (
                                      <button
                                        key={member}
                                        type="button"
                                        className={[
                                          'rounded-md border px-2 py-1 text-xs font-medium transition-all',
                                          selected
                                            ? 'border-brand/50 bg-brand/15 text-brand-light'
                                            : known
                                              ? 'border-white/12 bg-white/5 text-slate-300 hover:bg-white/8'
                                              : 'border-warning/35 bg-warning/10 text-warning',
                                        ].join(' ')}
                                        onClick={() => {
                                          setLaunchGroupField(group.id, (current) => ({
                                            ...current,
                                            overrides: current.overrides.map((item, idx) => {
                                              if (idx !== overrideIndex) return item
                                              if (item.proxies.includes(member)) {
                                                return { ...item, proxies: item.proxies.filter((proxy) => proxy !== member) }
                                              }
                                              return { ...item, proxies: dedupe([...item.proxies, member]) }
                                            }),
                                          }))
                                        }}
                                      >
                                        {member}
                                      </button>
                                    )
                                  })}
                                </div>
                              )}

                              {knownMembers.length > 0 && (
                                <div className="flex items-center justify-between gap-2">
                                  <span className="text-[11px] text-muted">
                                    已选 {override.proxies.length} / {knownMembers.length}
                                  </span>
                                  <div className="flex items-center gap-2">
                                    <button
                                      className="btn-ghost h-6 px-2 text-[11px]"
                                      onClick={() => {
                                        setLaunchGroupField(group.id, (current) => ({
                                          ...current,
                                          overrides: current.overrides.map((item, idx) => (
                                            idx === overrideIndex ? { ...item, proxies: [...knownMembers] } : item
                                          )),
                                        }))
                                      }}
                                    >
                                      全选
                                    </button>
                                    <button
                                      className="btn-ghost h-6 px-2 text-[11px]"
                                      onClick={() => {
                                        setLaunchGroupField(group.id, (current) => ({
                                          ...current,
                                          overrides: current.overrides.map((item, idx) => (
                                            idx === overrideIndex ? { ...item, proxies: [] } : item
                                          )),
                                        }))
                                      }}
                                    >
                                      清空
                                    </button>
                                  </div>
                                </div>
                              )}
                            </div>
                          )
                        })
                      )}
                    </div>
                  ))}
                </div>
              )}

              <div className="flex items-center justify-between gap-3 rounded-lg border border-white/10 bg-black/20 px-3 py-2.5">
                <span className={`text-xs ${launchDeviceDirty ? 'text-warning' : 'text-muted'}`}>
                  {launchDeviceDirty ? '有未同步的设备路由修改' : '设备路由配置已同步'}
                </span>
                <button
                  className="btn-primary text-xs flex items-center gap-1.5"
                  onClick={() => { void syncLaunchDeviceGroups() }}
                  disabled={launchDeviceSaving || launchDeviceLoading || !launchDeviceDirty}
                >
                  {launchDeviceSaving ? <><Loader2 size={12} className="animate-spin" />同步中…</> : '同步到设备路由'}
                </button>
              </div>
            </div>

            <LaunchConfigPreview
              content={launchConfigPreview}
              loading={launchConfigLoading}
              error={launchConfigError}
              onRefresh={() => { void refreshLaunchConfigPreview() }}
            />

            <div className="glass-card px-5 py-5 space-y-4">
              {/* Config summary */}
              <h2 className="text-sm font-semibold text-slate-200 border-b border-white/5 pb-3">启动服务</h2>
              <div className="space-y-2 text-xs">
                <div className="flex items-center gap-2 text-muted">
                  <Wifi size={12} className="text-brand" />
                  <span className="text-slate-300">DNS：</span>
                  {dns.enable ? `启用 · ${dns.mode} · ${dns.dnsmasq_mode}` : '禁用'}
                  {dns.apply_on_start && dns.enable && <span className="text-brand ml-1">（启动时接管）</span>}
                </div>
                <div className="flex items-center gap-2 text-muted">
                  <Globe size={12} className="text-brand" />
                  <span className="text-slate-300">透明代理：</span>
                  {net.mode === 'none' ? '不接管' : `${net.mode.toUpperCase()} · ${net.firewall_backend}`}
                  {net.apply_on_start && net.mode !== 'none' && <span className="text-brand ml-1">（启动时接管）</span>}
                </div>
                <div className="flex items-center gap-2 text-muted">
                  <Sparkles size={12} className="text-brand" />
                  <span className="text-slate-300">绕过局域网：</span>{net.bypass_lan ? '是' : '否'}
                  <span className="text-slate-300 ml-2">绕过国内 IP：</span>{net.bypass_china ? '是' : '否'}
                  {net.ipv6 && <span className="text-slate-300 ml-2">IPv6 透明代理：开启</span>}
                </div>
              </div>

              {/* Streaming launch log panel */}
              {launchLog.length > 0 && (
                <div className="rounded-xl bg-black/40 border border-white/8 overflow-hidden">
                  <div className="flex items-center gap-2 px-4 py-2 border-b border-white/8 bg-white/3">
                    <Terminal size={13} className="text-brand" />
                    <span className="text-xs font-semibold text-slate-300">启动日志</span>
                    {launching && <Loader2 size={12} className="animate-spin text-brand ml-auto" />}
                  </div>
                  <div className="px-2 py-2 max-h-72 overflow-y-auto space-y-px font-mono text-xs">
                    {launchLog.map((ev, i) => {
                      if (ev.type === 'done') return null
                      if (ev.type === 'info') return (
                        <div key={i} className="flex items-start gap-2 px-2 py-0.5 text-slate-400">
                          <span className="flex-shrink-0 text-white/20 mt-px">›</span>
                          <span className="flex-1 leading-5">{ev.message}</span>
                        </div>
                      )
                      // type === 'step'
                      const icon = ev.status === 'running'
                        ? <Loader2 size={12} className="animate-spin text-brand flex-shrink-0 mt-px" />
                        : ev.status === 'ok'
                          ? <CheckCircle2 size={12} className="text-success flex-shrink-0 mt-px" />
                          : ev.status === 'error'
                            ? <XCircle size={12} className="text-danger flex-shrink-0 mt-px" />
                            : <Minus size={12} className="text-muted flex-shrink-0 mt-px" />
                      const textColor = ev.status === 'ok' ? 'text-slate-200' : ev.status === 'error' ? 'text-red-300' : ev.status === 'running' ? 'text-white' : 'text-slate-400'
                      return (
                        <div key={i} className="px-2 py-0.5">
                          <div className={`flex items-start gap-2 ${textColor}`}>
                            {icon}
                            <span className="flex-1 leading-5">{ev.message}</span>
                          </div>
                          {ev.detail && (
                            <div className="pl-6 text-muted leading-4 mt-0.5">{ev.detail}</div>
                          )}
                        </div>
                      )
                    })}
                    <div ref={logEndRef} />
                  </div>
                </div>
              )}

              {/* Launch error banner */}
              {launchError && !launching && (
                <div className="rounded-xl bg-danger/10 border border-danger/20 px-4 py-3 space-y-2">
                  <div className="flex items-start gap-2">
                    <XCircle size={15} className="text-danger flex-shrink-0 mt-0.5" />
                    <div>
                      <p className="text-sm font-semibold text-danger">启动失败</p>
                      <p className="text-xs text-muted mt-0.5">{launchError}</p>
                    </div>
                  </div>
                  <button
                    className="w-full flex items-center justify-center gap-2 py-2 text-xs font-semibold rounded-lg bg-white/5 border border-white/10 text-slate-300 hover:bg-white/10 transition-colors"
                    onClick={handleLaunch}
                  >
                    <RotateCw size={13} />重试
                  </button>
                </div>
              )}

              {/* Launch button (shown before first launch attempt) */}
              {launchLog.length === 0 && !launchDone && (
                <button
                  className="btn-primary w-full flex items-center justify-center gap-2 py-3 text-sm font-semibold"
                  onClick={handleLaunch}
                  disabled={launching || launchDeviceSaving || launchDeviceLoading}
                >
                  {launching
                    ? <><Loader2 size={16} className="animate-spin" />正在启动…</>
                    : <><Play size={16} />一键启动内核 + 应用接管</>}
                </button>
              )}
            </div>

            {/* ── Port verification panel (shown after launch succeeds) ── */}
            {launchDone && (
              <div className="glass-card px-5 py-5 space-y-4">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Wifi size={15} className="text-brand" />
                    <h2 className="text-sm font-semibold text-slate-200">端口服务验证</h2>
                    {portChecking && <Loader2 size={13} className="animate-spin text-brand" />}
                  </div>
                  <button
                    className="btn-ghost text-xs flex items-center gap-1.5"
                    onClick={handlePortCheck}
                    disabled={portChecking}
                  >
                    <RotateCw size={12} className={portChecking ? 'animate-spin' : ''} />
                    {portChecking ? '检测中…' : '重新检测'}
                  </button>
                </div>

                {portChecking && !portChecks && (
                  <div className="flex items-center gap-3 text-sm text-muted py-2">
                    <Loader2 size={15} className="animate-spin text-brand" />
                    正在逐一检测各服务端口…
                  </div>
                )}

                {portChecks && (
                  <div className="space-y-2">
                    {portChecks.map((c, i) => (
                      <div
                        key={i}
                        className={`flex items-start gap-3 px-3 py-2.5 rounded-xl border text-xs ${
                          portChecking
                            ? 'border-white/10 bg-black/10'
                            : c.ok
                              ? 'border-success/25 bg-success/8'
                              : 'border-danger/25 bg-danger/8'
                        }`}
                      >
                        <div className="flex-shrink-0 mt-0.5">
                          {portChecking
                            ? <Loader2 size={13} className="animate-spin text-muted" />
                            : c.ok
                              ? <CheckCircle2 size={13} className="text-success" />
                              : <XCircle size={13} className="text-danger" />}
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <span className={`font-semibold ${portChecking ? 'text-slate-300' : c.ok ? 'text-success' : 'text-danger'}`}>
                              {c.name}
                            </span>
                            {c.ok && c.latency_ms !== undefined && (
                              <span className="text-muted">{c.latency_ms} ms</span>
                            )}
                            {c.required && (
                              <span className="text-[10px] px-1.5 py-0.5 rounded bg-brand/20 text-brand font-medium">必需</span>
                            )}
                          </div>
                          <p className="text-muted mt-0.5 leading-4">{c.description}</p>
                          {!c.ok && c.error && (
                            <p className="text-danger/80 mt-0.5 leading-4 font-mono text-[10px]">{c.error}</p>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                )}

                {portChecks && !portChecking && (
                  <div className={`rounded-xl px-4 py-3 flex items-center gap-2 ${portCheckAllOk ? 'bg-success/10 border border-success/20' : 'bg-warning/10 border border-warning/20'}`}>
                    {portCheckAllOk
                      ? <><CheckCircle2 size={14} className="text-success flex-shrink-0" /><p className="text-sm font-semibold text-success">所有端口验证通过 ✓ 可以进入连通检测</p></>
                      : <><AlertCircle size={14} className="text-warning flex-shrink-0" /><p className="text-sm font-semibold text-warning">部分端口未响应，请重新检测或检查配置</p></>}
                  </div>
                )}
              </div>
            )}

            <div className="flex gap-3">
              <button className="btn-ghost flex-1" onClick={() => setStep('network')} disabled={launching}>← 返回</button>
              <button
                className={`flex-1 flex items-center justify-center gap-2 py-2.5 text-sm font-semibold rounded-xl border transition-all ${
                  portCheckAllOk
                    ? 'btn-primary'
                    : 'bg-surface-2 border-white/10 text-muted cursor-not-allowed opacity-50'
                }`}
                onClick={() => setStep('check')}
                disabled={!portCheckAllOk}
                title={portCheckAllOk ? undefined : '请等待所有端口检测通过后再继续'}
              >
                开始连通检测 <ChevronRight size={14} />
              </button>
            </div>
          </div>
        )}

        {/* ─── Step 5: Check ──────────────────────────────────────────────── */}
        {step === 'check' && (
          <div className="space-y-4">
            <div className="glass-card px-5 py-5 space-y-4">
              <div className="flex items-center justify-between">
                <h2 className="text-sm font-semibold text-slate-200">出口 IP / 连通检测</h2>
                <button
                  className="btn-ghost text-xs flex items-center gap-1.5"
                  onClick={handleCheck}
                  disabled={checking}
                >
                  <RotateCw size={12} className={checking ? 'animate-spin' : ''} />
                  {checking ? '检测中…' : '重新检测'}
                </button>
              </div>

              {!checkDone && !checking && (
                <button
                  className="btn-primary w-full flex items-center justify-center gap-2"
                  onClick={handleCheck}
                >
                  <Wifi size={14} />开始检测
                </button>
              )}

              {checking && (
                <div className="flex items-center gap-3 text-sm text-muted">
                  <Loader2 size={16} className="animate-spin text-brand" />
                  正在从路由器和浏览器两侧发起检测…
                </div>
              )}

              {/* Router probe results */}
              {routerProbe && (
                <div className="space-y-2">
                  <p className="text-xs font-semibold text-muted uppercase tracking-wider">路由器侧（服务端检测）</p>
                  {routerProbe.ip_checks.reduce((acc, c, i) => {
                    const prev = i > 0 ? routerProbe.ip_checks[i - 1] : null
                    if (!prev || prev.group !== c.group) {
                      acc.push(<p key={`ipg-${c.group}`} className="text-[10px] uppercase tracking-wider text-muted mt-1">{c.group ?? '其他'}组</p>)
                    }
                    acc.push(
                      <div key={i} className={`flex items-start gap-2 text-xs rounded-xl px-3 py-2 ${c.ok ? 'bg-success/8 border border-success/20' : 'bg-danger/8 border border-danger/20'}`}>
                        {c.ok
                          ? <CheckCircle2 size={13} className="text-success flex-shrink-0 mt-0.5" />
                          : <XCircle size={13} className="text-danger flex-shrink-0 mt-0.5" />}
                        <div>
                          <span className={c.ok ? 'text-success' : 'text-danger'}>{c.provider}</span>
                          {c.ok && c.ip && <span className="ml-2 text-slate-300 font-mono">{c.ip}</span>}
                          {c.ok && c.location && <span className="ml-1 text-muted">({c.location})</span>}
                          {!c.ok && c.error && <span className="ml-2 text-muted">{c.error}</span>}
                        </div>
                      </div>
                    )
                    return acc
                  }, [] as React.ReactNode[])}
                  {routerProbe.access_checks.reduce((acc, c, i) => {
                    const prev = i > 0 ? routerProbe.access_checks[i - 1] : null
                    if (!prev || prev.group !== c.group) {
                      acc.push(<p key={`acg-${c.group}`} className="text-[10px] uppercase tracking-wider text-muted mt-1">{c.group ?? '其他'}组</p>)
                    }
                    acc.push(
                      <div key={i} className={`flex items-start gap-2 text-xs rounded-xl px-3 py-2 ${c.ok ? 'bg-success/8 border border-success/20' : 'bg-danger/8 border border-danger/20'}`}>
                        {c.ok
                          ? <CheckCircle2 size={13} className="text-success flex-shrink-0 mt-0.5" />
                          : <XCircle size={13} className="text-danger flex-shrink-0 mt-0.5" />}
                        <div>
                          <span className={c.ok ? 'text-success' : 'text-danger'}>{c.name}</span>
                          {c.ok && c.latency_ms && <span className="ml-2 text-muted">{c.latency_ms} ms</span>}
                          {!c.ok && c.error && <span className="ml-2 text-muted">{c.error}</span>}
                        </div>
                      </div>
                    )
                    return acc
                  }, [] as React.ReactNode[])}
                </div>
              )}

              {/* Browser probe results */}
              {browserProbe && (
                <div className="space-y-2">
                  <p className="text-xs font-semibold text-muted uppercase tracking-wider">浏览器侧（前端直连检测）</p>
                  <div className={`flex items-start gap-2 text-xs rounded-xl px-3 py-2 ${browserProbe.ipOK ? 'bg-success/8 border border-success/20' : 'bg-danger/8 border border-danger/20'}`}>
                    {browserProbe.ipOK
                      ? <CheckCircle2 size={13} className="text-success flex-shrink-0 mt-0.5" />
                      : <XCircle size={13} className="text-danger flex-shrink-0 mt-0.5" />}
                    <div>
                      <span className={browserProbe.ipOK ? 'text-success' : 'text-danger'}>出口 IP 检测 (IP.SB)</span>
                      {browserProbe.ipOK && browserProbe.ip && <span className="ml-2 text-slate-300 font-mono">{browserProbe.ip}</span>}
                      {!browserProbe.ipOK && browserProbe.ipError && <span className="ml-2 text-muted">{browserProbe.ipError}</span>}
                    </div>
                  </div>
                  {browserProbe.accessChecks.reduce((acc, c, i) => {
                    const prev = i > 0 ? browserProbe.accessChecks[i - 1] : null
                    if (!prev || prev.group !== c.group) {
                      acc.push(<p key={`bg-${c.group}`} className="text-[10px] uppercase tracking-wider text-muted mt-1">{c.group ?? '其他'}组</p>)
                    }
                    acc.push(
                      <div key={i} className={`flex items-start gap-2 text-xs rounded-xl px-3 py-2 ${c.ok ? 'bg-success/8 border border-success/20' : 'bg-danger/8 border border-danger/20'}`}>
                        {c.ok
                          ? <CheckCircle2 size={13} className="text-success flex-shrink-0 mt-0.5" />
                          : <XCircle size={13} className="text-danger flex-shrink-0 mt-0.5" />}
                        <div>
                          <span className={c.ok ? 'text-success' : 'text-danger'}>{c.name}</span>
                          {c.ok && c.latency_ms && <span className="ml-2 text-muted">{c.latency_ms} ms</span>}
                          {!c.ok && c.error && <span className="ml-2 text-muted">{c.error}</span>}
                        </div>
                      </div>
                    )
                    return acc
                  }, [] as React.ReactNode[])}
                </div>
              )}

              {/* Overall result */}
              {checkDone && overallOK && (
                <div className="space-y-3">
                  <div className="rounded-xl bg-success/10 border border-success/20 px-5 py-4 flex items-start gap-3">
                    <CheckCircle2 size={20} className="text-success flex-shrink-0 mt-0.5" />
                    <div>
                      <p className="text-sm font-bold text-success">全部检测通过！</p>
                      <p className="text-xs text-muted mt-1">代理工作正常，路由器和浏览器均可正常访问外网。</p>
                    </div>
                  </div>
                  <div className="glass-card px-5 py-4 space-y-4">
                    <h3 className="text-sm font-semibold text-slate-200">完成设置</h3>
                    <Field label="开机自动启动内核" hint="路由器重启后自动启动 ClashForge 并自动启动 Mihomo 内核">
                      <Toggle checked={autoStartCore} onChange={setAutoStartCore} label={autoStartCore ? '启用' : '禁用'} />
                    </Field>
                    {saveError && (
                      <div className="flex items-center gap-2 text-xs text-danger">
                        <AlertCircle size={13} />{saveError}
                      </div>
                    )}
                    <button
                      className="btn-primary w-full flex items-center justify-center gap-2 py-3 text-sm font-semibold"
                      onClick={handleComplete}
                      disabled={saving}
                    >
                      {saving
                        ? <><Loader2 size={16} className="animate-spin" />保存中…</>
                        : <><ArrowRight size={16} />完成配置，进入概览</>}
                    </button>
                  </div>
                </div>
              )}

              {checkDone && !overallOK && (
                <div className="space-y-3">
                  <div className="rounded-xl bg-danger/10 border border-danger/20 px-4 py-3 flex items-start gap-2">
                    <AlertCircle size={15} className="text-danger flex-shrink-0 mt-0.5" />
                    <div>
                      <p className="text-sm font-semibold text-danger">部分检测未通过</p>
                      <p className="text-xs text-muted mt-1">请检查以下日志排查问题，或返回上一步重新配置。</p>
                    </div>
                  </div>

                  {probeLogs.length > 0 && (
                    <div className="glass-card px-4 py-4 space-y-2">
                      <p className="text-xs font-semibold text-muted uppercase tracking-wider">ClashForge 最近日志</p>
                      <div className="max-h-64 overflow-y-auto space-y-1">
                        {probeLogs.map((l, i) => (
                          <div key={i} className={`text-xs font-mono px-2 py-1 rounded ${
                            (l as unknown as Record<string,string>).level === 'error' ? 'text-danger bg-danger/5' :
                            (l as unknown as Record<string,string>).level === 'warn'  ? 'text-warning bg-warning/5' :
                            'text-slate-400'
                          }`}>
                            {(l as unknown as Record<string,string>).ts ? new Date(Number((l as unknown as Record<string,string>).ts) * 1000).toLocaleTimeString() : ''}
                            {' '}
                            <span className="font-semibold uppercase">[{(l as unknown as Record<string,string>).level}]</span>
                            {' '}{(l as unknown as Record<string,string>).msg}
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>

            <div className="flex gap-3">
              <button className="btn-ghost flex-1" onClick={() => setStep('launch')}>← 返回</button>
              <button className="btn-ghost flex-1" onClick={() => navigate('/')}>
                跳过，直接进入概览
              </button>
            </div>
          </div>
        )}
      </div>
      {subCacheModalOpen && selectedSaved?.kind === 'sub' && (
        <ModalShell
          title={`缓存配置 · ${selectedSaved.sub.name}`}
          description="这是当前订阅在本地缓存的原始配置内容（raw YAML）。"
          icon={<Database size={16} />}
          onClose={() => !subCacheModalLoading && setSubCacheModalOpen(false)}
          size="lg"
          dismissible={!subCacheModalLoading}
        >
          <div className="space-y-3">
            {subCacheModalLoading && (
              <div className="flex items-center gap-2 text-sm text-muted">
                <Loader2 size={14} className="animate-spin text-brand" />
                正在加载缓存配置…
              </div>
            )}
            {!subCacheModalLoading && subCacheModalError && (
              <div className="rounded-xl border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger">
                {subCacheModalError}
              </div>
            )}
            {!subCacheModalLoading && !subCacheModalError && (
              <div className="max-h-[60vh] overflow-auto rounded-xl border border-white/10 bg-black/30 p-3">
                <pre className="whitespace-pre text-xs leading-5 text-slate-200 font-mono select-text">{subCacheModalContent}</pre>
              </div>
            )}
            <div className="flex justify-end">
              <button className="btn-ghost" onClick={() => setSubCacheModalOpen(false)} disabled={subCacheModalLoading}>
                关闭
              </button>
            </div>
          </div>
        </ModalShell>
      )}
    </div>
  )
}
