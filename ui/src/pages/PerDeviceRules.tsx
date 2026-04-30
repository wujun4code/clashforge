import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import yaml from 'js-yaml'
import {
  AlertCircle,
  Ban,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  Code2,
  Gamepad2,
  Globe,
  GripVertical,
  HelpCircle,
  Laptop,
  LayoutGrid,
  Loader2,
  Monitor,
  Network,
  Plus,
  RefreshCw,
  Save,
  Server,
  Smartphone,
  Tablet,
  Terminal,
  Trash2,
  Tv,
  Wifi,
  X,
  Zap,
} from 'lucide-react'

import { EmptyState, InlineNotice, PageHeader, SectionCard } from '../components/ui'
import {
  getDeviceGroups,
  getNetworkClients,
  previewDeviceGroupsConfig,
  getSourceFile,
  getSources,
  getSubscriptionCache,
  getSubscriptions,
  restartCore,
  updateDeviceGroups,
} from '../api/client'
import type {
  ActiveSource,
  DeviceRouteDevice,
  DeviceRouteGroup,
  DeviceRouteOverride,
  NetworkClient,
  SourceFile,
  Subscription,
} from '../api/client'

const BUILTIN_PROXY_NAMES = ['DIRECT', 'REJECT', 'PASS']
const OVERRIDEABLE_GROUP_TYPES = new Set(['select', 'url-test', 'fallback', 'load-balance'])

// ─── Drag-and-drop data transfer types ───────────────────────────────────────
const DT_DEVICE = 'application/cf-device-ip'
const DT_GROUP_CARD = 'application/cf-group-id'

// ─── Device type detection ────────────────────────────────────────────────────
type DeviceType =
  | 'iphone' | 'ipad' | 'macbook' | 'imac'
  | 'android' | 'windows' | 'linux'
  | 'tv' | 'gaming' | 'nas' | 'router' | 'unknown'

const DEVICE_TYPE_COLOR: Record<DeviceType, string> = {
  iphone:  'text-blue-400',
  ipad:    'text-indigo-400',
  macbook: 'text-slate-300',
  imac:    'text-slate-300',
  android: 'text-green-400',
  windows: 'text-sky-400',
  linux:   'text-orange-400',
  tv:      'text-purple-400',
  gaming:  'text-red-400',
  nas:     'text-yellow-400',
  router:  'text-teal-400',
  unknown: 'text-slate-500',
}

const DEVICE_TYPE_LABEL: Record<DeviceType, string> = {
  iphone:  'iPhone',
  ipad:    'iPad',
  macbook: 'MacBook',
  imac:    'Mac',
  android: 'Android',
  windows: 'Windows PC',
  linux:   'Linux',
  tv:      '智能电视',
  gaming:  '游戏主机',
  nas:     'NAS',
  router:  '路由器',
  unknown: '未知设备',
}

const DEVICE_TYPE_BG: Record<DeviceType, string> = {
  iphone:  'bg-blue-500/12 border-blue-500/20',
  ipad:    'bg-indigo-500/12 border-indigo-500/20',
  macbook: 'bg-slate-500/12 border-slate-500/20',
  imac:    'bg-slate-500/12 border-slate-500/20',
  android: 'bg-green-500/12 border-green-500/20',
  windows: 'bg-sky-500/12 border-sky-500/20',
  linux:   'bg-orange-500/12 border-orange-500/20',
  tv:      'bg-purple-500/12 border-purple-500/20',
  gaming:  'bg-red-500/12 border-red-500/20',
  nas:     'bg-yellow-500/12 border-yellow-500/20',
  router:  'bg-teal-500/12 border-teal-500/20',
  unknown: 'bg-white/4 border-white/10',
}

function detectDeviceType(hostname: string): DeviceType {
  const h = (hostname || '').toLowerCase()
  if (/iphone/.test(h)) return 'iphone'
  if (/ipad/.test(h)) return 'ipad'
  if (/macbook/.test(h)) return 'macbook'
  if (/^mac-|imac/.test(h)) return 'imac'
  if (/android|pixel|samsung|redmi|mi-|xiaomi|huawei|honor|oneplus|oppo|vivo/.test(h)) return 'android'
  if (/windows|thinkpad|lenovo|dell|hp-|surface|asus|acer/.test(h)) return 'windows'
  if (/raspberry|^pi\d|ubuntu|linux/.test(h)) return 'linux'
  if (/appletv|chromecast|firetv|rokutv|smarttv/.test(h)) return 'tv'
  if (/switch|playstation|ps[345]\d|xbox/.test(h)) return 'gaming'
  if (/nas|synology|qnap|ds\d{3}|storage/.test(h)) return 'nas'
  if (/router|openwrt|gateway|modem/.test(h)) return 'router'
  return 'unknown'
}

function DeviceTypeIcon({ type, size = 16 }: { type: DeviceType; size?: number }) {
  const cls = `flex-shrink-0 ${DEVICE_TYPE_COLOR[type]}`
  const p = { size, className: cls }
  switch (type) {
    case 'iphone': case 'android': return <Smartphone {...p} />
    case 'ipad':   return <Tablet {...p} />
    case 'macbook': return <Laptop {...p} />
    case 'imac': case 'windows': case 'linux': return <Monitor {...p} />
    case 'tv':     return <Tv {...p} />
    case 'gaming': return <Gamepad2 {...p} />
    case 'nas':    return <Server {...p} />
    case 'router': return <Wifi {...p} />
    default:       return <HelpCircle {...p} />
  }
}

// ─── Proxy node type detection ───────────────────────────────────────────────
type NodeType = 'ssh' | 'worker' | 'direct' | 'reject' | 'pass' | 'proxy'

function detectNodeType(name: string): NodeType {
  if (name === 'DIRECT') return 'direct'
  if (name === 'REJECT') return 'reject'
  if (name === 'PASS') return 'pass'
  const n = name.toLowerCase()
  if (/\bssh\b|ssh-tunnel|sshtunnel/.test(n)) return 'ssh'
  if (/\bworker\b|cloudflare|cfworker|\bcf\b/.test(n)) return 'worker'
  return 'proxy'
}

function NodeTypeIcon({ type, size = 11 }: { type: NodeType; size?: number }) {
  switch (type) {
    case 'ssh':    return <Terminal size={size} className="flex-shrink-0 text-emerald-400" />
    case 'worker': return <Globe    size={size} className="flex-shrink-0 text-orange-400" />
    case 'direct': return <Zap      size={size} className="flex-shrink-0 text-green-400" />
    case 'reject': return <Ban      size={size} className="flex-shrink-0 text-red-400" />
    case 'pass':   return <Check    size={size} className="flex-shrink-0 text-slate-400" />
    default:       return <Server   size={size} className="flex-shrink-0 text-sky-400" />
  }
}

// ─── Proxy region flag detection ──────────────────────────────────────────────
function detectProxyRegion(name: string): string {
  const n = name.toLowerCase()
  if (/香港|hk\b|hong.?kong/.test(n))         return '🇭🇰'
  if (/日本|jp\b|japan/.test(n))               return '🇯🇵'
  if (/美国|us\b|usa\b|united.?states/.test(n)) return '🇺🇸'
  if (/新加坡|sg\b|singapore/.test(n))          return '🇸🇬'
  if (/台湾|tw\b|taiwan/.test(n))               return '🇹🇼'
  if (/韩国|kr\b|korea/.test(n))               return '🇰🇷'
  if (/英国|uk\b|britain/.test(n))             return '🇬🇧'
  if (/德国|de\b|germany/.test(n))             return '🇩🇪'
  if (/法国|fr\b|france/.test(n))              return '🇫🇷'
  if (/加拿大|ca\b|canada/.test(n))             return '🇨🇦'
  if (/澳洲|澳大利亚|au\b|australia/.test(n))   return '🇦🇺'
  if (/荷兰|nl\b|netherlands/.test(n))         return '🇳🇱'
  if (/土耳其|tr\b|turkey/.test(n))            return '🇹🇷'
  if (/印度|in\b|india/.test(n))               return '🇮🇳'
  if (/巴西|br\b|brazil/.test(n))              return '🇧🇷'
  if (/俄罗斯|ru\b|russia/.test(n))            return '🇷🇺'
  return ''
}

// ─── Shared internal types ────────────────────────────────────────────────────
interface PolicyGroupOption {
  name: string
  type: string
  proxies: string[]
}

interface PolicyOptions {
  groups: PolicyGroupOption[]
  knownProxyNames: string[]
}

interface DevicePoolClient extends NetworkClient {
  label: string
}

type NoticeTone = 'info' | 'success' | 'warning' | 'danger'

interface DeviceRuleSourceOption {
  key: string
  label: string
  description: string
  type: 'file' | 'subscription'
  filename?: string
  subscription?: Subscription
}

// ─── Pure helpers ─────────────────────────────────────────────────────────────
function sourceKeyForFile(filename: string): string {
  return `file:${filename}`
}

function sourceKeyForSubscription(id: string): string {
  return `subscription:${id}`
}

function createDraftID() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `dg_${crypto.randomUUID()}`
  }
  return `dg_${Math.random().toString(36).slice(2, 10)}`
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

function normalizeIncoming(groups: DeviceRouteGroup[]): DeviceRouteGroup[] {
  return [...groups]
    .sort((a, b) => (a.order ?? 0) - (b.order ?? 0))
    .map((group, index) => {
      const normalizedDevices = (group.devices ?? []).map((device) => {
        const ip = (device.ip ?? '').trim()
        return {
          ip,
          prefix: clampPrefix(
            typeof device.prefix === 'number' ? device.prefix : defaultPrefixForIP(ip || '0.0.0.0'),
            ip || '0.0.0.0',
          ),
          hostname: (device.hostname ?? '').trim(),
        } satisfies DeviceRouteDevice
      })

      const normalizedOverrides = (group.overrides ?? []).map((override) => ({
        original_group: (override.original_group ?? '').trim(),
        proxies: dedupe((override.proxies ?? []).map((item) => item.trim()).filter(Boolean)),
      }))

      return {
        id: (group.id ?? '').trim() || createDraftID(),
        name: (group.name ?? '').trim(),
        devices: normalizedDevices,
        overrides: normalizedOverrides,
        order: index,
      } satisfies DeviceRouteGroup
    })
}

function sanitizeForSave(groups: DeviceRouteGroup[]): DeviceRouteGroup[] {
  return groups.map((group, index) => {
    const devices: DeviceRouteDevice[] = []
    for (const device of group.devices ?? []) {
      const ip = device.ip.trim()
      if (!ip) continue
      const hostname = (device.hostname ?? '').trim()
      devices.push({
        ip,
        prefix: clampPrefix(device.prefix, ip),
        ...(hostname ? { hostname } : {}),
      })
    }

    const overrides = (group.overrides ?? [])
      .map((override) => {
        const originalGroup = override.original_group.trim()
        const proxies = dedupe((override.proxies ?? []).map((item) => item.trim()).filter(Boolean))
        if (!originalGroup || proxies.length === 0) return null
        return {
          original_group: originalGroup,
          proxies,
        } satisfies DeviceRouteOverride
      })
      .filter((item): item is DeviceRouteOverride => item !== null)

    return {
      id: group.id || createDraftID(),
      name: group.name.trim(),
      devices,
      overrides,
      order: index,
    } satisfies DeviceRouteGroup
  })
}

function parsePolicyOptions(content: string): PolicyOptions {
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
  const groups: PolicyGroupOption[] = []
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
}

function serializeSnapshot(groups: DeviceRouteGroup[]): string {
  return JSON.stringify(sanitizeForSave(groups))
}

function normalizeNetworkClients(clients: NetworkClient[]): DevicePoolClient[] {
  const out: DevicePoolClient[] = []
  for (const client of clients) {
    const ip = (client.ip ?? '').trim()
    if (!ip) continue
    const hostname = (client.hostname ?? '').trim()
    const mac = (client.mac ?? '').trim()
    const base = hostname || '未命名设备'
    const parts = [base, ip]
    if (mac) parts.push(mac)
    out.push({
      ip,
      ...(mac ? { mac } : {}),
      ...(hostname ? { hostname } : {}),
      ...(client.ips?.length ? { ips: client.ips } : {}),
      ...(client.interface ? { interface: client.interface } : {}),
      ...(client.source ? { source: client.source } : {}),
      label: parts.join(' · '),
    })
  }
  return out
}

function newDevice(): DeviceRouteDevice {
  return { ip: '', prefix: 32, hostname: '' }
}

function newGroup(): DeviceRouteGroup {
  return {
    id: createDraftID(),
    name: '',
    devices: [],
    overrides: [],
    order: 0,
  }
}

function sourceKeyFromActiveSource(active: ActiveSource | null): string {
  if (!active) return ''
  if (active.type === 'file' && active.filename) {
    return sourceKeyForFile(active.filename)
  }
  if (active.type === 'subscription' && active.sub_id) {
    return sourceKeyForSubscription(active.sub_id)
  }
  return ''
}

function buildDeviceRuleSourceOptions(files: SourceFile[], subscriptions: Subscription[]): DeviceRuleSourceOption[] {
  const fileOptions = [...files]
    .sort((a, b) => {
      const at = Date.parse(a.created_at || '')
      const bt = Date.parse(b.created_at || '')
      return Number.isFinite(bt) && Number.isFinite(at) ? bt - at : 0
    })
    .map((file) => ({
      key: sourceKeyForFile(file.filename),
      label: file.filename,
      description: '配置文件',
      type: 'file' as const,
      filename: file.filename,
    }))

  const subOptions = [...subscriptions]
    .sort((a, b) => a.name.localeCompare(b.name, 'zh-Hans-CN'))
    .map((subscription) => ({
      key: sourceKeyForSubscription(subscription.id),
      label: subscription.name,
      description: `订阅${subscription.node_count ? ` · ${subscription.node_count} 节点` : ''}`,
      type: 'subscription' as const,
      subscription,
    }))

  return [...fileOptions, ...subOptions]
}

// ─── Visual Mode: GroupCard ───────────────────────────────────────────────────
interface GroupCardProps {
  group: DeviceRouteGroup
  poolClientMap: Map<string, DevicePoolClient>
  policyGroups: PolicyGroupOption[]
  policyGroupMap: Map<string, PolicyGroupOption>
  knownProxySet: Set<string>
  draggingIP: string | null
  draggingGroupID: string | null
  isDropTarget: boolean
  setGroupField: (id: string, updater: (g: DeviceRouteGroup) => DeviceRouteGroup) => void
  onRemove: () => void
  onDragGroupStart: () => void
  onDragGroupEnd: () => void
  onDropDevice: (ip: string) => void
  onDropGroup: (sourceID: string) => void
  onSetDropTarget: (id: string | null) => void
}

function GroupCard({
  group, poolClientMap, policyGroups,
  draggingIP, draggingGroupID, isDropTarget,
  setGroupField, onRemove, onDragGroupStart, onDragGroupEnd,
  onDropDevice, onDropGroup, onSetDropTarget,
}: GroupCardProps) {
  const [editingName, setEditingName] = useState(false)
  const [localName, setLocalName] = useState(group.name)
  const [deviceDropActive, setDeviceDropActive] = useState(false)
  const nameRef = useRef<HTMLInputElement>(null)
  const isDraggingThis = draggingGroupID === group.id

  const commitName = () => {
    setEditingName(false)
    const trimmed = localName.trim()
    if (trimmed !== group.name) {
      setGroupField(group.id, (g) => ({ ...g, name: trimmed }))
    }
  }

  // All unique non-builtin proxies across all policy groups
  const allProxies = useMemo(() => {
    const seen = new Set<string>()
    const result: string[] = []
    for (const pg of policyGroups) {
      for (const proxy of pg.proxies) {
        if (!seen.has(proxy) && !BUILTIN_PROXY_NAMES.includes(proxy)) {
          seen.add(proxy)
          result.push(proxy)
        }
      }
    }
    return result
  }, [policyGroups])

  // Currently selected exit node (single value extracted from overrides)
  const selectedProxy = useMemo(() => (
    group.overrides
      .flatMap((o) => o.proxies)
      .find((p) => !BUILTIN_PROXY_NAMES.includes(p)) ?? null
  ), [group.overrides])

  return (
    <div
      className={[
        'rounded-xl border flex flex-col transition-all duration-150',
        isDraggingThis
          ? 'opacity-25 border-white/10 bg-black/20'
          : isDropTarget && draggingGroupID
            ? 'border-brand/50 bg-brand/5 shadow-[0_0_0_2px_rgb(var(--brand)/0.15)]'
            : 'border-white/12 bg-black/25 hover:border-white/20',
      ].join(' ')}
      onDragOver={(e) => {
        if (e.dataTransfer.types.includes(DT_GROUP_CARD) && !isDraggingThis) {
          e.preventDefault()
          onSetDropTarget(group.id)
        }
      }}
      onDragLeave={(e) => {
        if (!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node)) {
          onSetDropTarget(null)
        }
      }}
      onDrop={(e) => {
        const sourceID = e.dataTransfer.getData(DT_GROUP_CARD)
        if (sourceID && sourceID !== group.id) {
          e.preventDefault()
          onDropGroup(sourceID)
        }
        onSetDropTarget(null)
      }}
    >
      {/* Header — draggable handle for group reordering */}
      <div
        draggable
        onDragStart={(e) => {
          e.dataTransfer.setData(DT_GROUP_CARD, group.id)
          e.dataTransfer.effectAllowed = 'move'
          onDragGroupStart()
        }}
        onDragEnd={onDragGroupEnd}
        className="flex items-center gap-2 px-3 py-2.5 border-b border-white/8 cursor-grab active:cursor-grabbing select-none"
      >
        <GripVertical size={14} className="text-slate-600 flex-shrink-0" />

        <div className="flex-1 min-w-0">
          {editingName ? (
            <input
              ref={nameRef}
              autoFocus
              className="glass-input h-7 w-full text-sm"
              value={localName}
              onChange={(e) => setLocalName(e.target.value)}
              onBlur={commitName}
              onKeyDown={(e) => {
                if (e.key === 'Enter') commitName()
                if (e.key === 'Escape') { setEditingName(false); setLocalName(group.name) }
              }}
            />
          ) : (
            <button
              className="text-sm font-semibold text-white hover:text-brand-light transition-colors truncate w-full text-left"
              onClick={() => { setEditingName(true); setLocalName(group.name) }}
            >
              {group.name || <span className="text-slate-500 font-normal italic">点击设置分组名</span>}
            </button>
          )}
        </div>

        <span className="text-[10px] text-slate-500 flex-shrink-0 tabular-nums">
          {group.devices.length} 台
        </span>

        <button
          className="h-6 w-6 flex items-center justify-center rounded text-slate-600 hover:text-danger hover:bg-danger/10 transition-colors flex-shrink-0"
          onClick={onRemove}
          title="删除分组"
        >
          <Trash2 size={12} />
        </button>
      </div>

      {/* Devices — drop zone */}
      <div
        className={[
          'p-3 min-h-[72px] transition-all',
          deviceDropActive && draggingIP ? 'bg-brand/6' : '',
        ].join(' ')}
        onDragOver={(e) => {
          if (e.dataTransfer.types.includes(DT_DEVICE)) {
            e.preventDefault()
            e.stopPropagation()
            setDeviceDropActive(true)
          }
        }}
        onDragLeave={(e) => {
          if (!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node)) {
            setDeviceDropActive(false)
          }
        }}
        onDrop={(e) => {
          const ip = e.dataTransfer.getData(DT_DEVICE)
          if (ip) { e.preventDefault(); e.stopPropagation(); onDropDevice(ip) }
          setDeviceDropActive(false)
        }}
      >
        {group.devices.length === 0 ? (
          <div className={[
            'flex items-center justify-center rounded-lg border border-dashed h-14 text-xs transition-all',
            deviceDropActive && draggingIP
              ? 'border-brand/60 text-brand-light bg-brand/8'
              : 'border-white/15 text-slate-600',
          ].join(' ')}>
            {deviceDropActive && draggingIP ? '松开以添加' : '拖入设备'}
          </div>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {group.devices.map((device, idx) => {
              const client = poolClientMap.get(device.ip)
              const hostname = device.hostname || client?.hostname || ''
              const type = detectDeviceType(hostname)
              return (
                <div
                  key={`${device.ip}-${idx}`}
                  className="group/chip flex items-center gap-1.5 rounded-lg border border-white/12 bg-white/4 pl-2 pr-1 py-1.5 text-xs"
                >
                  <DeviceTypeIcon type={type} size={13} />
                  <div className="leading-tight">
                    <div className="text-slate-200 font-medium">{hostname || device.ip}</div>
                    {hostname && (
                      <div className="text-[10px] font-mono text-slate-500">{device.ip}</div>
                    )}
                  </div>
                  <button
                    className="ml-0.5 h-5 w-5 rounded flex items-center justify-center text-slate-600 hover:text-danger hover:bg-danger/10 transition-colors opacity-0 group-hover/chip:opacity-100"
                    onClick={() => setGroupField(group.id, (g) => ({
                      ...g, devices: g.devices.filter((_, i) => i !== idx),
                    }))}
                    title="移出分组"
                  >
                    <X size={10} />
                  </button>
                </div>
              )
            })}
            {draggingIP && (
              <div className={[
                'flex items-center justify-center rounded-lg border border-dashed px-3 py-1.5 text-[10px] transition-all',
                deviceDropActive ? 'border-brand/50 text-brand-light bg-brand/6' : 'border-white/15 text-slate-600',
              ].join(' ')}>
                + 拖入
              </div>
            )}
          </div>
        )}
      </div>

      {/* Exit node — single-select, no builtins */}
      <div className="px-3 pb-3 border-t border-white/8 pt-2.5">
        <p className="text-[10px] font-semibold uppercase tracking-[0.15em] text-slate-500 mb-2">出口策略</p>

        {allProxies.length === 0 ? (
          <p className="text-[11px] text-slate-600 italic">请先加载配置来源以显示可用节点</p>
        ) : (
          <div className="flex flex-wrap gap-1">
            {allProxies.map((proxy) => {
              const isSelected = selectedProxy === proxy
              const flag = detectProxyRegion(proxy)
              const nodeType = detectNodeType(proxy)
              return (
                <button
                  key={proxy}
                  className={[
                    'inline-flex items-center gap-1 rounded-md border px-1.5 py-1 text-[11px] transition-all cursor-pointer',
                    isSelected
                      ? 'border-brand/55 bg-brand/18 text-brand-light font-semibold shadow-[0_0_0_1px_rgb(var(--brand)/0.2)]'
                      : 'border-white/10 bg-white/3 text-slate-400 hover:bg-white/8 hover:text-slate-200 hover:border-white/18',
                  ].join(' ')}
                  onClick={() => {
                    if (isSelected) {
                      // Deselect — clear all overrides
                      setGroupField(group.id, (g) => ({ ...g, overrides: [] }))
                    } else {
                      // Select — one override per policy group that contains this proxy
                      const matching = policyGroups.filter((pg) => pg.proxies.includes(proxy))
                      const targets = matching.length > 0 ? matching : policyGroups
                      setGroupField(group.id, (g) => ({
                        ...g,
                        overrides: targets.map((pg) => ({ original_group: pg.name, proxies: [proxy] })),
                      }))
                    }
                  }}
                >
                  <NodeTypeIcon type={nodeType} size={10} />
                  {flag && <span className="text-[12px] leading-none">{flag}</span>}
                  <span>{proxy}</span>
                  {isSelected && <Check size={8} className="flex-shrink-0 opacity-80" />}
                </button>
              )
            })}
          </div>
        )}

        {!selectedProxy && allProxies.length > 0 && (
          <p className="text-[11px] text-slate-600 italic mt-1.5">未选择 — 走全局默认策略</p>
        )}
      </div>
    </div>
  )
}

// ─── Visual Mode layout ───────────────────────────────────────────────────────
interface VisualModeProps {
  groups: DeviceRouteGroup[]
  setGroups: React.Dispatch<React.SetStateAction<DeviceRouteGroup[]>>
  setGroupField: (id: string, updater: (g: DeviceRouteGroup) => DeviceRouteGroup) => void
  poolClients: DevicePoolClient[]
  poolClientMap: Map<string, DevicePoolClient>
  policyGroups: PolicyGroupOption[]
  policyGroupMap: Map<string, PolicyGroupOption>
  knownProxySet: Set<string>
  loadingClients: boolean
  onRefreshClients: () => void
  onAddGroup: () => void
  onRemoveGroup: (id: string) => void
}

function VisualMode({
  groups, setGroups, setGroupField,
  poolClients, poolClientMap,
  policyGroups, policyGroupMap, knownProxySet,
  loadingClients, onRefreshClients, onAddGroup, onRemoveGroup,
}: VisualModeProps) {
  const [poolSearch, setPoolSearch] = useState('')
  const [draggingIP, setDraggingIP] = useState<string | null>(null)
  const [draggingGroupID, setDraggingGroupID] = useState<string | null>(null)
  const [dropTargetGroupID, setDropTargetGroupID] = useState<string | null>(null)

  const assignedIPs = useMemo(() => {
    const map = new Map<string, string>()
    for (const group of groups) {
      for (const device of group.devices) {
        if (device.ip) map.set(device.ip, group.name || '分组')
      }
    }
    return map
  }, [groups])

  const filteredPool = useMemo(() => {
    const q = poolSearch.trim().toLowerCase()
    if (!q) return poolClients
    return poolClients.filter((c) =>
      c.ip.includes(q) || (c.hostname || '').toLowerCase().includes(q)
    )
  }, [poolClients, poolSearch])

  const handleDropDevice = useCallback((groupID: string, ip: string) => {
    const client = poolClientMap.get(ip)
    setGroupField(groupID, (g) => {
      if (g.devices.some((d) => d.ip === ip)) return g
      return {
        ...g,
        devices: [...g.devices, {
          ip,
          prefix: defaultPrefixForIP(ip),
          hostname: client?.hostname || '',
        }],
      }
    })
  }, [poolClientMap, setGroupField])

  const handleDropGroup = useCallback((targetID: string, sourceID: string) => {
    setGroups((prev) => {
      const si = prev.findIndex((g) => g.id === sourceID)
      const ti = prev.findIndex((g) => g.id === targetID)
      if (si < 0 || ti < 0 || si === ti) return prev
      const next = [...prev]
      const [item] = next.splice(si, 1)
      next.splice(ti, 0, item)
      return next
    })
  }, [setGroups])

  return (
    <div className="space-y-5">
      {/* Device Pool */}
      <div className="rounded-xl border border-white/10 bg-black/20 p-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <div>
            <p className="text-sm font-semibold text-white">在线设备池</p>
            <p className="text-xs text-slate-500 mt-0.5">
              {loadingClients
                ? '正在扫描…'
                : `已发现 ${poolClients.length} 台设备 · 拖拽卡片到下方分组`}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <input
              className="glass-input h-8 w-44 text-xs"
              placeholder="搜索主机名 / IP…"
              value={poolSearch}
              onChange={(e) => setPoolSearch(e.target.value)}
            />
            <button
              className="btn-ghost h-8 px-3 text-xs flex items-center gap-1.5"
              onClick={onRefreshClients}
              disabled={loadingClients}
            >
              <RefreshCw size={12} className={loadingClients ? 'animate-spin' : ''} />
              扫描
            </button>
          </div>
        </div>

        {poolClients.length === 0 && !loadingClients ? (
          <div className="rounded-lg border border-dashed border-white/15 py-5 text-center text-xs text-slate-500">
            未发现在线设备，请点击扫描
          </div>
        ) : (
          <div className="flex flex-wrap gap-3">
            {filteredPool.map((client) => {
              const type = detectDeviceType(client.hostname || '')
              const assignedGroup = assignedIPs.get(client.ip)
              const isDragging = draggingIP === client.ip
              const extraIPv6 = (client.ips ?? []).filter(ip => ip.includes(':')).length
              return (
                <div
                  key={client.ip}
                  draggable
                  onDragStart={(e) => {
                    e.dataTransfer.setData(DT_DEVICE, client.ip)
                    e.dataTransfer.effectAllowed = 'copy'
                    setDraggingIP(client.ip)
                  }}
                  onDragEnd={() => setDraggingIP(null)}
                  className={[
                    'flex items-center gap-2.5 rounded-xl border w-[200px] px-3 py-2.5 text-xs transition-all select-none',
                    isDragging
                      ? 'opacity-30 scale-95 border-brand/40 bg-brand/10'
                      : 'border-white/12 bg-white/3 cursor-grab active:cursor-grabbing hover:border-white/22 hover:bg-white/6',
                  ].join(' ')}
                >
                  {/* Device icon */}
                  <div className={[
                    'w-9 h-9 rounded-lg border flex items-center justify-center flex-shrink-0',
                    DEVICE_TYPE_BG[type],
                  ].join(' ')}>
                    <DeviceTypeIcon type={type} size={20} />
                  </div>
                  {/* Info column */}
                  <div className="min-w-0 flex-1 flex flex-col gap-0.5">
                    {/* Hostname */}
                    <span className="font-semibold text-slate-200 text-[11px] truncate leading-tight">
                      {client.hostname || '未知设备'}
                    </span>
                    {/* Device type label */}
                    <span className="text-[9px] text-slate-500 uppercase tracking-wide leading-none">
                      {DEVICE_TYPE_LABEL[type]}
                    </span>
                    {/* IPv4 address */}
                    <span className="text-[10px] font-mono text-slate-400 leading-none mt-0.5">{client.ip}</span>
                    {/* MAC address */}
                    {client.mac && (
                      <span className="text-[9px] font-mono text-slate-600 leading-none">{client.mac}</span>
                    )}
                    {/* Bottom row: assigned badge + IPv6 indicator */}
                    {(assignedGroup || extraIPv6 > 0) && (
                      <div className="flex items-center gap-1 mt-0.5 flex-wrap">
                        {assignedGroup && (
                          <span className="rounded-full bg-brand/20 border border-brand/25 px-1.5 py-0.5 text-[9px] text-brand-light font-medium leading-none">
                            {assignedGroup}
                          </span>
                        )}
                        {extraIPv6 > 0 && (
                          <span className="rounded-full bg-slate-700/60 border border-white/10 px-1.5 py-0.5 text-[9px] text-slate-400 font-mono leading-none">
                            +{extraIPv6} IPv6
                          </span>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              )
            })}
            {filteredPool.length === 0 && poolSearch && (
              <p className="text-xs text-slate-500 py-1">无匹配结果</p>
            )}
          </div>
        )}
      </div>

      {/* Groups kanban */}
      <div className="grid gap-4 lg:grid-cols-2 xl:grid-cols-3">
        {groups.map((group) => (
          <GroupCard
            key={group.id}
            group={group}
            poolClientMap={poolClientMap}
            policyGroups={policyGroups}
            policyGroupMap={policyGroupMap}
            knownProxySet={knownProxySet}
            draggingIP={draggingIP}
            draggingGroupID={draggingGroupID}
            isDropTarget={dropTargetGroupID === group.id}
            setGroupField={setGroupField}
            onRemove={() => onRemoveGroup(group.id)}
            onDragGroupStart={() => setDraggingGroupID(group.id)}
            onDragGroupEnd={() => setDraggingGroupID(null)}
            onDropDevice={(ip) => handleDropDevice(group.id, ip)}
            onDropGroup={(sourceID) => handleDropGroup(group.id, sourceID)}
            onSetDropTarget={setDropTargetGroupID}
          />
        ))}

        <button
          className="flex min-h-[180px] cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-white/15 bg-white/2 text-slate-600 transition-all hover:border-brand/40 hover:bg-brand/4 hover:text-brand-light"
          onClick={onAddGroup}
        >
          <Plus size={20} />
          <span className="text-xs">新建分组</span>
        </button>
      </div>
    </div>
  )
}

// ─── Live config file preview ─────────────────────────────────────────────────
function HighlightedYAML({ content, deviceGroupNames }: { content: string; deviceGroupNames: string[] }) {
  const lines = content.split('\n')
  return (
    <pre className="p-4 text-xs leading-[1.65] font-mono overflow-x-auto whitespace-pre">
      {lines.map((line, i) => {
        const trimmed = line.trim()
        if (!trimmed) return <div key={i}>{' '}</div>
        if (trimmed.startsWith('#')) {
          return <div key={i} className="text-slate-600">{line}</div>
        }
        const isDeviceGroupLine = deviceGroupNames.some((n) => n && line.includes(n))
        if (isDeviceGroupLine) {
          return <div key={i} className="text-brand-light bg-brand/6 -mx-4 px-4">{line}</div>
        }
        if (/^\s*-\s+(AND|SRC-IP-CIDR|RULE-SET|MATCH|DOMAIN|GEOIP)/.test(line)) {
          return <div key={i} className="text-amber-300/85">{line}</div>
        }
        if (/^[a-z][\w-]*:/.test(line)) {
          return <div key={i} className="text-sky-300 mt-1">{line}</div>
        }
        if (/^\s+- (name|type|proxies):/.test(line)) {
          return <div key={i} className="text-slate-300">{line}</div>
        }
        return <div key={i} className="text-slate-400">{line}</div>
      })}
    </pre>
  )
}

interface ConfigPreviewPanelProps {
  yaml: string
  loading: boolean
  error: string
  groups: DeviceRouteGroup[]
  policyGroupMap: Map<string, PolicyGroupOption>
  onRefresh: () => void
}

function ConfigPreviewPanel({ yaml, loading, error, groups, policyGroupMap, onRefresh }: ConfigPreviewPanelProps) {
  const [yamlCollapsed, setYamlCollapsed] = useState(false)
  const deviceGroupNames = groups.map((g) => g.name.trim()).filter(Boolean)
  const lineCount = yaml ? yaml.split('\n').length : 0

  const namedGroups = groups.filter((g) => g.name.trim())

  return (
    <SectionCard
      title="分流规则与生效配置"
      description="基于当前选中的来源缓存 YAML 与分组规则实时合并预览；YAML 中蓝色高亮为设备分组生成的影子策略组和规则。"
      actions={(
        <div className="flex items-center gap-2">
          <button
            className="btn-ghost h-7 px-2.5 text-xs flex items-center gap-1.5"
            onClick={onRefresh}
            disabled={loading}
          >
            <RefreshCw size={12} className={loading ? 'animate-spin' : ''} />
            刷新配置
          </button>
        </div>
      )}
    >
      {error && (
        <InlineNotice tone="warning" title="读取失败">{error}</InlineNotice>
      )}

      {/* ── Plain-language rule explanation ── */}
      {namedGroups.length > 0 && (
        <div className="mb-5 space-y-2">
          <p className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500 mb-3">
            分流规则解读
          </p>
          {namedGroups.map((group) => {
            const selectedProxy = group.overrides
              .flatMap((o) => o.proxies)
              .find((p) => !BUILTIN_PROXY_NAMES.includes(p)) ?? null
            const shadowName = selectedProxy
              ? `${group.name} - ${group.overrides[0]?.original_group ?? '节点选择'}`
              : null
            const deviceCIDRs = group.devices
              .filter((d) => d.ip)
              .map((d) => `${d.ip}/${d.prefix}`)

            return (
              <div key={group.id} className="rounded-lg border border-white/10 bg-black/20 px-3 py-2.5 space-y-2">
                {/* Group header */}
                <div className="flex flex-wrap items-center gap-2">
                  <Network size={12} className="text-brand flex-shrink-0" />
                  <span className="text-[13px] font-semibold text-white">{group.name}</span>
                  <span className="text-[10px] text-slate-500">{group.devices.length} 台设备</span>
                  {selectedProxy ? (
                    <span className="inline-flex items-center gap-1 rounded-full border border-brand/30 bg-brand/12 px-2 py-0.5 text-[10px] text-brand-light">
                      <NodeTypeIcon type={detectNodeType(selectedProxy)} size={9} />
                      {detectProxyRegion(selectedProxy)}{detectProxyRegion(selectedProxy) ? ' ' : ''}{selectedProxy}
                    </span>
                  ) : (
                    <span className="text-[10px] text-slate-500 italic">走全局默认策略</span>
                  )}
                </div>

                {/* Generated rules detail */}
                {selectedProxy && (
                  <div className="ml-4 space-y-1.5 text-[11px]">
                    {shadowName && (
                      <div className="flex items-start gap-1.5 text-slate-400">
                        <span className="text-slate-600 flex-shrink-0 mt-0.5">↳</span>
                        <span>生成影子策略组：
                          <code className="ml-1 rounded bg-brand/10 border border-brand/20 px-1 py-0.5 text-[10px] text-brand-light font-mono">
                            {shadowName}
                          </code>
                        </span>
                      </div>
                    )}
                    {deviceCIDRs.length > 0 && (
                      <div className="flex items-start gap-1.5 text-slate-400">
                        <span className="text-slate-600 flex-shrink-0 mt-0.5">↳</span>
                        <div>
                          <span>设备来源规则 (rule-provider ipcidr)：</span>
                          <div className="mt-1 flex flex-wrap gap-1">
                            {deviceCIDRs.map((cidr) => (
                              <code key={cidr} className="rounded bg-white/5 border border-white/10 px-1.5 py-0.5 text-[10px] text-slate-300 font-mono">
                                {cidr}
                              </code>
                            ))}
                          </div>
                        </div>
                      </div>
                    )}
                    {group.overrides.map((ov, i) => {
                      const pg = policyGroupMap.get(ov.original_group)
                      return (
                        <div key={i} className="flex items-start gap-1.5 text-slate-400">
                          <span className="text-slate-600 flex-shrink-0 mt-0.5">↳</span>
                          <span>
                            命中 <code className="rounded bg-white/5 border border-white/10 px-1 text-[10px] text-slate-300 font-mono">{ov.original_group}</code>
                            {pg ? ` (${pg.type})` : ''} 规则时 → 走
                            <code className="ml-1 rounded bg-brand/10 border border-brand/20 px-1 text-[10px] text-brand-light font-mono">{shadowName}</code>
                          </span>
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {/* ── YAML with syntax highlighting ── */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <p className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">
            来源缓存 + 设备分流 合并预览
          </p>
          <div className="flex items-center gap-2">
            {lineCount > 0 && (
              <span className="text-[10px] text-slate-600 tabular-nums">{lineCount} 行</span>
            )}
            <button
              className="flex items-center gap-1 text-[10px] text-slate-500 hover:text-slate-300 transition-colors"
              onClick={() => setYamlCollapsed((v) => !v)}
            >
              {yamlCollapsed ? <ChevronDown size={11} /> : <ChevronUp size={11} />}
              {yamlCollapsed ? '展开' : '收起'}
            </button>
          </div>
        </div>

        {!yamlCollapsed && (
          loading ? (
            <div className="flex items-center justify-center gap-2 rounded-xl border border-white/10 bg-black/20 py-10 text-sm text-muted">
              <Loader2 size={14} className="animate-spin text-brand" />
              正在读取配置…
            </div>
          ) : yaml.trim() ? (
            <div className="rounded-xl border border-white/10 bg-black/30 overflow-auto max-h-[600px]">
              <HighlightedYAML content={yaml} deviceGroupNames={deviceGroupNames} />
            </div>
          ) : (
            <div className="rounded-xl border border-dashed border-white/12 bg-black/15 py-8 text-center text-xs text-slate-500">
              尚未生成配置文件，请先保存一次分组。
            </div>
          )
        )}
      </div>
    </SectionCard>
  )
}

// ─── Main page component ──────────────────────────────────────────────────────
export function PerDeviceRules() {
  const [viewMode, setViewMode] = useState<'visual' | 'geek'>('visual')

  const [sourceOptions, setSourceOptions] = useState<DeviceRuleSourceOption[]>([])
  const [selectedSourceKey, setSelectedSourceKey] = useState('')
  const [activeSourceKey, setActiveSourceKey] = useState('')

  const [groups, setGroups] = useState<DeviceRouteGroup[]>([])
  const [options, setOptions] = useState<PolicyOptions>({ groups: [], knownProxyNames: [...BUILTIN_PROXY_NAMES] })
  const [poolClients, setPoolClients] = useState<DevicePoolClient[]>([])
  const [finalConfigContent, setFinalConfigContent] = useState('')

  const [loading, setLoading] = useState(true)
  const [loadingSources, setLoadingSources] = useState(false)
  const [loadingOptions, setLoadingOptions] = useState(false)
  const [loadingClients, setLoadingClients] = useState(false)
  const [loadingFinalConfig, setLoadingFinalConfig] = useState(false)
  const [saving, setSaving] = useState(false)

  const [snapshot, setSnapshot] = useState('[]')
  const [optionsError, setOptionsError] = useState('')
  const [clientsError, setClientsError] = useState('')
  const [finalConfigError, setFinalConfigError] = useState('')
  const [notice, setNotice] = useState<{ tone: NoticeTone; title: string; text: string } | null>(null)

  const selectedSource = useMemo(
    () => sourceOptions.find((item) => item.key === selectedSourceKey) ?? null,
    [selectedSourceKey, sourceOptions],
  )

  const loadSourceOptions = useCallback(async () => {
    setLoadingSources(true)
    try {
      const [sourcesData, subscriptionsData] = await Promise.all([
        getSources().catch(() => ({ files: [] as SourceFile[], active_source: null as ActiveSource | null })),
        getSubscriptions().catch(() => ({ subscriptions: [] as Subscription[] })),
      ])
      const builtOptions = buildDeviceRuleSourceOptions(
        sourcesData.files ?? [],
        subscriptionsData.subscriptions ?? [],
      )
      const activeKey = sourceKeyFromActiveSource(sourcesData.active_source ?? null)
      setSourceOptions(builtOptions)
      setActiveSourceKey(activeKey)
      setSelectedSourceKey((current) => {
        if (current && builtOptions.some((item) => item.key === current)) return current
        if (activeKey && builtOptions.some((item) => item.key === activeKey)) return activeKey
        return builtOptions[0]?.key ?? ''
      })
    } finally {
      setLoadingSources(false)
    }
  }, [])

  const loadDeviceGroups = useCallback(async (sourceKey: string) => {
    const data = await getDeviceGroups(sourceKey || undefined)
    const normalized = normalizeIncoming(data.device_groups ?? [])
    setGroups(normalized)
    setSnapshot(serializeSnapshot(normalized))
    return normalized
  }, [])

  const loadPolicyOptions = useCallback(async (source: DeviceRuleSourceOption | null) => {
    setLoadingOptions(true)
    setOptionsError('')
    try {
      if (!source) {
        setOptions({ groups: [], knownProxyNames: [...BUILTIN_PROXY_NAMES] })
        setOptionsError('请先在"配置管理"中准备至少一个配置文件或订阅。')
        return
      }

      let content = ''
      if (source.type === 'file' && source.filename) {
        const data = await getSourceFile(source.filename)
        content = data.content ?? ''
      } else if (source.type === 'subscription' && source.subscription) {
        const data = await getSubscriptionCache(source.subscription.id)
        content = data.content ?? ''
      }

      content = content.trim()
      if (!content) {
        setOptions({ groups: [], knownProxyNames: [...BUILTIN_PROXY_NAMES] })
        setOptionsError('当前来源没有可解析的配置内容，请先更新缓存或导入配置。')
        return
      }
      setOptions(parsePolicyOptions(content))
    } catch (error) {
      setOptions({ groups: [], knownProxyNames: [...BUILTIN_PROXY_NAMES] })
      setOptionsError(error instanceof Error ? error.message : '读取策略组选项失败')
    } finally {
      setLoadingOptions(false)
    }
  }, [])

  const loadNetworkClients = useCallback(async () => {
    setLoadingClients(true)
    setClientsError('')
    try {
      const data = await getNetworkClients()
      setPoolClients(normalizeNetworkClients(data.clients ?? []))
    } catch (error) {
      setPoolClients([])
      setClientsError(error instanceof Error ? error.message : '扫描设备失败')
    } finally {
      setLoadingClients(false)
    }
  }, [])

  const loadFinalConfig = useCallback(async (sourceKey: string, nextGroups: DeviceRouteGroup[]) => {
    setLoadingFinalConfig(true)
    setFinalConfigError('')
    try {
      if (!sourceKey) {
        setFinalConfigContent('')
        return
      }
      const sanitized = sanitizeForSave(nextGroups)
      const data = await previewDeviceGroupsConfig(sanitized, sourceKey)
      setFinalConfigContent(data.content ?? '')
    } catch (error) {
      setFinalConfigContent('')
      setFinalConfigError(error instanceof Error ? error.message : '读取预览配置失败')
    } finally {
      setLoadingFinalConfig(false)
    }
  }, [])

  const refreshAll = useCallback(async () => {
    setLoading(true)
    setNotice(null)
    try {
      await loadSourceOptions()
      await loadNetworkClients()
    } catch (error) {
      setNotice({
        tone: 'danger',
        title: '加载失败',
        text: error instanceof Error ? error.message : '读取配置失败',
      })
    } finally {
      setLoading(false)
    }
  }, [loadNetworkClients, loadSourceOptions])

  useEffect(() => {
    void refreshAll()
  }, [refreshAll])

  useEffect(() => {
    if (!selectedSourceKey) {
      setGroups([])
      setSnapshot('[]')
      setOptions({ groups: [], knownProxyNames: [...BUILTIN_PROXY_NAMES] })
      setFinalConfigContent('')
      setFinalConfigError('')
      return
    }

    setLoading(true)
    setNotice(null)
    void Promise.all([
      loadDeviceGroups(selectedSourceKey),
      loadPolicyOptions(selectedSource),
    ])
      .then(([loadedGroups]) => loadFinalConfig(selectedSourceKey, loadedGroups))
      .catch((error) => {
        setNotice({
          tone: 'danger',
          title: '加载失败',
          text: error instanceof Error ? error.message : '读取配置失败',
        })
      })
      .finally(() => setLoading(false))
  }, [loadDeviceGroups, loadFinalConfig, loadPolicyOptions, selectedSource, selectedSourceKey])

  useEffect(() => {
    if (!selectedSourceKey || loading) return
    const timer = setTimeout(() => {
      void loadFinalConfig(selectedSourceKey, groups)
    }, 350)
    return () => clearTimeout(timer)
  }, [groups, loadFinalConfig, loading, selectedSourceKey])

  const groupNamePrefixes = useMemo(
    () => groups.map((group) => group.name.trim()).filter(Boolean),
    [groups],
  )

  const policyGroups = useMemo(() => (
    options.groups.filter((group) => {
      for (const prefix of groupNamePrefixes) {
        if (group.name.startsWith(`${prefix} - `)) return false
      }
      return true
    })
  ), [groupNamePrefixes, options.groups])

  const policyGroupMap = useMemo(
    () => new Map(policyGroups.map((group) => [group.name, group])),
    [policyGroups],
  )

  const knownProxySet = useMemo(
    () => new Set(options.knownProxyNames),
    [options.knownProxyNames],
  )

  const poolClientMap = useMemo(
    () => new Map(poolClients.map((client) => [client.ip, client])),
    [poolClients],
  )

  const dirty = useMemo(
    () => serializeSnapshot(groups) !== snapshot,
    [groups, snapshot],
  )

  const groupCount = groups.length
  const deviceCount = useMemo(
    () => groups.reduce((count, group) => count + group.devices.length, 0),
    [groups],
  )
  const overrideCount = useMemo(
    () => groups.reduce((count, group) => count + group.overrides.length, 0),
    [groups],
  )

  const staleStats = useMemo(() => {
    let staleGroups = 0
    let staleProxies = 0
    for (const group of groups) {
      for (const override of group.overrides) {
        if (!policyGroupMap.has(override.original_group)) staleGroups += 1
        for (const proxy of override.proxies) {
          if (!knownProxySet.has(proxy)) staleProxies += 1
        }
      }
    }
    return { staleGroups, staleProxies }
  }, [groups, knownProxySet, policyGroupMap])

  const setGroupField = useCallback((groupID: string, updater: (group: DeviceRouteGroup) => DeviceRouteGroup) => {
    setGroups((prev) => prev.map((group) => (group.id === groupID ? updater(group) : group)))
  }, [])

  const addGroup = () => {
    setGroups((prev) => [...prev, newGroup()])
  }

  const removeGroup = (groupID: string) => {
    setGroups((prev) => prev.filter((group) => group.id !== groupID))
  }

  const moveGroup = (groupID: string, direction: -1 | 1) => {
    setGroups((prev) => {
      const index = prev.findIndex((item) => item.id === groupID)
      if (index < 0) return prev
      const nextIndex = index + direction
      if (nextIndex < 0 || nextIndex >= prev.length) return prev
      const next = [...prev]
      const [picked] = next.splice(index, 1)
      next.splice(nextIndex, 0, picked)
      return next
    })
  }

  const addDevice = (groupID: string) => {
    setGroupField(groupID, (group) => {
      const usedIPs = new Set(group.devices.map((item) => item.ip).filter(Boolean))
      const candidate = poolClients.find((client) => !usedIPs.has(client.ip))
      const next = candidate
        ? {
            ip: candidate.ip,
            prefix: defaultPrefixForIP(candidate.ip),
            hostname: candidate.hostname || '',
          }
        : newDevice()
      return {
        ...group,
        devices: [...group.devices, next],
      }
    })
  }

  const removeDevice = (groupID: string, index: number) => {
    setGroupField(groupID, (group) => ({
      ...group,
      devices: group.devices.filter((_, i) => i !== index),
    }))
  }

  const addOverride = (groupID: string) => {
    setGroupField(groupID, (group) => {
      if (policyGroups.length === 0) return group
      const used = new Set(group.overrides.map((item) => item.original_group))
      const preferred = policyGroups.find((item) => !used.has(item.name)) ?? policyGroups[0]
      const defaultProxy = preferred.proxies[0] ? [preferred.proxies[0]] : []
      return {
        ...group,
        overrides: [...group.overrides, { original_group: preferred.name, proxies: defaultProxy }],
      }
    })
  }

  const removeOverride = (groupID: string, index: number) => {
    setGroupField(groupID, (group) => ({
      ...group,
      overrides: group.overrides.filter((_, i) => i !== index),
    }))
  }

  const saveGroups = async (applyMode: 'reload' | 'restart' = 'reload') => {
    setNotice(null)
    if (!selectedSourceKey) {
      setNotice({ tone: 'warning', title: '请选择配置来源', text: '请先选择要绑定策略覆盖的配置文件或订阅。' })
      return
    }
    const sanitized = sanitizeForSave(groups)
    const nameSet = new Set<string>()
    for (const group of sanitized) {
      if (!group.name) {
        setNotice({ tone: 'warning', title: '保存前检查', text: '每个设备分组都需要填写名称。' })
        return
      }
      if (nameSet.has(group.name)) {
        setNotice({ tone: 'warning', title: '保存前检查', text: `分组名称重复：${group.name}` })
        return
      }
      nameSet.add(group.name)
    }

    setSaving(true)
    try {
      const data = await updateDeviceGroups(sanitized, selectedSourceKey)
      setGroups(sanitized)
      setSnapshot(serializeSnapshot(sanitized))

      if (data.profile_active === false) {
        setNotice({
          tone: 'success',
          title: '已保存到配置档案',
          text: data.message || '设备分组已全局更新，当前来源的策略覆盖已保存。当前运行配置未改动。',
        })
      } else if (!data.config_generated) {
        setNotice({
          tone: 'warning',
          title: '已保存，但生成配置失败',
          text: data.warning || '设备分组已保存，但运行配置生成失败。',
        })
      } else if (applyMode === 'restart') {
        try {
          await restartCore()
          setNotice({
            tone: 'success',
            title: '保存并重启成功',
            text: '设备路由已写入、配置已生成并完成内核重启，新规则已生效。',
          })
        } catch (error) {
          setNotice({
            tone: 'warning',
            title: '已保存，但重启失败',
            text: error instanceof Error ? error.message : '内核重启失败，请稍后重试。',
          })
        }
      } else if (data.core_running === false) {
        setNotice({
          tone: 'success',
          title: '保存成功',
          text: '设备路由已写入并生成配置。内核当前未运行，规则会在下次启动时生效。',
        })
      } else if (data.core_reloaded) {
        setNotice({
          tone: 'success',
          title: '保存并热加载成功',
          text: '设备路由已写入并热加载到运行中的内核，无需重新走 Setup。',
        })
      } else {
        const reloadErr = data.reload_error ? ` 详情：${data.reload_error}` : ''
        setNotice({
          tone: 'warning',
          title: '已保存，但热加载失败',
          text: `${data.warning || '设备路由已保存并生成配置，但内核未成功热加载。'}${reloadErr}`,
        })
      }

      await Promise.all([
        loadPolicyOptions(selectedSource),
        loadFinalConfig(selectedSourceKey, sanitized),
      ])
    } catch (error) {
      setNotice({
        tone: 'danger',
        title: '保存失败',
        text: error instanceof Error ? error.message : '写入失败，请稍后重试',
      })
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex min-h-[42vh] items-center justify-center">
        <div className="flex items-center gap-3 text-sm text-muted">
          <Loader2 size={16} className="animate-spin text-brand" />
          正在加载设备分组配置…
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Device Route"
        title="按设备分组路由"
        description="为不同设备分组配置专属节点覆盖，系统将自动生成对应的 AND 规则与影子策略组。"
        actions={(
          <>
            {/* View mode toggle */}
            <div className="flex items-center rounded-lg border border-white/12 bg-black/30 p-0.5">
              <button
                className={[
                  'flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-all',
                  viewMode === 'visual'
                    ? 'bg-brand/20 text-brand-light shadow-sm'
                    : 'text-muted hover:text-slate-300',
                ].join(' ')}
                onClick={() => setViewMode('visual')}
              >
                <LayoutGrid size={12} />
                可视化
              </button>
              <button
                className={[
                  'flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-all',
                  viewMode === 'geek'
                    ? 'bg-brand/20 text-brand-light shadow-sm'
                    : 'text-muted hover:text-slate-300',
                ].join(' ')}
                onClick={() => setViewMode('geek')}
              >
                <Code2 size={12} />
                极客
              </button>
            </div>

            <button
              className="btn-ghost flex items-center gap-2"
              onClick={() => { void refreshAll() }}
              disabled={saving || loadingSources || loadingOptions || loadingClients || loadingFinalConfig}
            >
              <RefreshCw size={14} className={(loadingSources || loadingOptions || loadingClients || loadingFinalConfig) ? 'animate-spin' : ''} />
              刷新
            </button>
            <button
              className="btn-primary flex items-center gap-2"
              onClick={() => { void saveGroups('reload') }}
              disabled={saving || !dirty || !selectedSourceKey}
            >
              {saving ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
              {saving ? '处理中…' : '保存并热加载'}
            </button>
            <button
              className="btn-ghost flex items-center gap-2"
              onClick={() => { void saveGroups('restart') }}
              disabled={saving || !dirty || !selectedSourceKey}
            >
              {saving ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
              {saving ? '处理中…' : '保存并重启内核'}
            </button>
          </>
        )}
        metrics={[
          { label: '分组', value: `${groupCount}` },
          { label: '设备', value: `${deviceCount}` },
          { label: '覆盖项', value: `${overrideCount}` },
          { label: '状态', value: dirty ? '有未保存修改' : '已同步' },
        ]}
      />

      {notice ? (
        <InlineNotice tone={notice.tone} title={notice.title}>
          {notice.text}
        </InlineNotice>
      ) : null}

      <SectionCard
        title="策略覆盖来源"
        description="设备分组全局共享；下方策略覆盖会按你选择的配置来源分别保存。"
        actions={(
          activeSourceKey && selectedSourceKey && activeSourceKey === selectedSourceKey ? (
            <span className="badge badge-success">当前运行来源</span>
          ) : (
            <span className="badge badge-muted">编辑离线来源</span>
          )
        )}
      >
        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
          <label className="space-y-1.5 text-sm">
            <span className="text-xs text-muted">选择配置文件 / 订阅</span>
            <select
              className="theme-select glass-input min-h-[40px] h-[40px] w-full"
              value={selectedSourceKey}
              onChange={(event) => {
                const nextKey = event.target.value
                if (dirty && nextKey !== selectedSourceKey) {
                  const ok = window.confirm('当前来源有未保存修改，切换后将丢失这些修改。确定切换吗？')
                  if (!ok) return
                }
                setSelectedSourceKey(nextKey)
              }}
              disabled={loadingSources || saving}
            >
              {sourceOptions.length === 0 ? (
                <option value="">暂无可选来源</option>
              ) : (
                sourceOptions.map((item) => (
                  <option key={item.key} value={item.key}>
                    {item.label}
                  </option>
                ))
              )}
            </select>
          </label>
          <div className="text-xs text-muted md:text-right">
            {selectedSource ? `${selectedSource.description}` : '请先在"配置管理"导入配置来源'}
          </div>
        </div>
      </SectionCard>

      {optionsError ? (
        <InlineNotice tone="warning" title="策略组选项不可用">
          {optionsError}
        </InlineNotice>
      ) : null}

      {clientsError ? (
        <InlineNotice tone="warning" title="在线设备扫描失败">
          {clientsError}。你仍可继续手动填写设备信息。
        </InlineNotice>
      ) : null}

      {(staleStats.staleGroups > 0 || staleStats.staleProxies > 0) ? (
        <InlineNotice tone="warning" title="检测到失效引用">
          订阅更新后有 {staleStats.staleGroups} 个策略组引用、{staleStats.staleProxies} 个节点引用已失效，请在下方覆盖配置中修复。
        </InlineNotice>
      ) : null}

      {/* ── Visual Mode ──────────────────────────────────────────────────────── */}
      {viewMode === 'visual' && (
        <VisualMode
          groups={groups}
          setGroups={setGroups}
          setGroupField={setGroupField}
          poolClients={poolClients}
          poolClientMap={poolClientMap}
          policyGroups={policyGroups}
          policyGroupMap={policyGroupMap}
          knownProxySet={knownProxySet}
          loadingClients={loadingClients}
          onRefreshClients={() => { void loadNetworkClients() }}
          onAddGroup={addGroup}
          onRemoveGroup={removeGroup}
        />
      )}

      {/* ── Geek Mode ────────────────────────────────────────────────────────── */}
      {viewMode === 'geek' && (
        <SectionCard
          title="设备分组与覆盖"
          description="每个分组可从在线设备池里选择客户端，并针对订阅中的策略组选择节点子集。保存后可热加载到运行中的内核。"
          actions={(
            <button className="btn-ghost flex items-center gap-2" onClick={addGroup}>
              <Plus size={14} />
              新增分组
            </button>
          )}
        >
          {groups.length === 0 ? (
            <EmptyState
              title="还没有设备分组"
              description="创建分组后可直接从在线设备池勾选设备并配置策略覆盖。保存时会自动重建配置并支持热加载。"
              action={(
                <button className="btn-primary flex items-center gap-2" onClick={addGroup}>
                  <Plus size={14} />
                  新建第一个分组
                </button>
              )}
            />
          ) : (
            <div className="space-y-4">
              {groups.map((group, groupIndex) => {
                const canMoveUp = groupIndex > 0
                const canMoveDown = groupIndex < groups.length - 1

                return (
                  <section
                    key={group.id}
                    className="rounded-xl border border-white/10 bg-black/15 px-4 py-4"
                  >
                    <div className="flex flex-wrap items-center gap-3">
                      <div className="flex min-w-0 flex-1 items-center gap-2">
                        <Network size={14} className="flex-shrink-0 text-brand" />
                        <input
                          className="glass-input min-h-[34px] h-[34px] w-full max-w-sm"
                          placeholder="分组名称，例如 iPhone / Work-PC"
                          value={group.name}
                          onChange={(event) => {
                            const nextName = event.target.value
                            setGroupField(group.id, (current) => ({ ...current, name: nextName }))
                          }}
                        />
                      </div>
                      <span className="badge badge-muted">
                        {group.devices.length} 设备 · {group.overrides.length} 覆盖
                      </span>
                      <div className="flex items-center gap-1">
                        <button
                          className="btn-ghost px-2"
                          title="上移分组"
                          onClick={() => moveGroup(group.id, -1)}
                          disabled={!canMoveUp}
                        >
                          <ChevronUp size={14} />
                        </button>
                        <button
                          className="btn-ghost px-2"
                          title="下移分组"
                          onClick={() => moveGroup(group.id, 1)}
                          disabled={!canMoveDown}
                        >
                          <ChevronDown size={14} />
                        </button>
                        <button
                          className="btn-ghost px-2 text-danger hover:bg-danger/10"
                          title="删除分组"
                          onClick={() => removeGroup(group.id)}
                        >
                          <Trash2 size={14} />
                        </button>
                      </div>
                    </div>

                    <div className="mt-4 grid gap-4 2xl:grid-cols-2">
                      <div className="rounded-lg border border-white/10 bg-surface-2/40 p-3">
                        <div className="mb-3 flex items-center justify-between">
                          <div>
                            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-muted">设备列表</p>
                            <p className="mt-1 text-[11px] text-muted">
                              {loadingClients
                                ? '正在扫描路由器在线设备…'
                                : `已发现 ${poolClients.length} 台在线设备，可直接选择`}
                            </p>
                          </div>
                          <div className="flex items-center gap-2">
                            <button
                              className="btn-ghost h-7 px-2.5 text-xs"
                              onClick={() => { void loadNetworkClients() }}
                              disabled={loadingClients}
                            >
                              <RefreshCw size={12} className={loadingClients ? 'animate-spin' : ''} />
                              刷新设备池
                            </button>
                            <button
                              className="btn-ghost h-7 px-2.5 text-xs"
                              onClick={() => addDevice(group.id)}
                            >
                              <Plus size={12} />
                              添加设备
                            </button>
                          </div>
                        </div>
                        <div className="space-y-2">
                          {group.devices.length === 0 ? (
                            <p className="rounded-md border border-dashed border-white/15 px-3 py-3 text-xs text-muted">
                              暂无设备，点击"添加设备"从设备池中选择。
                            </p>
                          ) : (
                            group.devices.map((device, deviceIndex) => {
                              const selectedClient = poolClientMap.get(device.ip)
                              const unresolvedIP = Boolean(device.ip) && !selectedClient
                              return (
                                <div
                                  key={`${group.id}-device-${deviceIndex}`}
                                  className="rounded-md border border-white/10 bg-black/20 p-2"
                                >
                                  <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_34px]">
                                    <select
                                      className="theme-select glass-input min-h-[34px] h-[34px] w-full"
                                      value={device.ip}
                                      onChange={(event) => {
                                        const nextIP = event.target.value.trim()
                                        setGroupField(group.id, (current) => ({
                                          ...current,
                                          devices: current.devices.map((item, idx) => {
                                            if (idx !== deviceIndex) return item
                                            if (!nextIP) {
                                              return {
                                                ...item,
                                                ip: '',
                                                prefix: defaultPrefixForIP(item.ip || '0.0.0.0'),
                                              }
                                            }
                                            const picked = poolClientMap.get(nextIP)
                                            const keepHostname = (item.hostname ?? '').trim()
                                            return {
                                              ...item,
                                              ip: nextIP,
                                              prefix: defaultPrefixForIP(nextIP),
                                              hostname: keepHostname || picked?.hostname || '',
                                            }
                                          }),
                                        }))
                                      }}
                                    >
                                      <option value="">选择在线设备</option>
                                      {poolClients.map((client) => (
                                        <option key={client.ip} value={client.ip}>
                                          {client.label}
                                        </option>
                                      ))}
                                      {unresolvedIP ? (
                                        <option value={device.ip}>
                                          手动设备 · {device.ip}
                                        </option>
                                      ) : null}
                                    </select>
                                    <input
                                      className="glass-input min-h-[34px] h-[34px]"
                                      placeholder="自定义显示名称（可选）"
                                      value={device.hostname ?? ''}
                                      onChange={(event) => {
                                        const nextHostname = event.target.value
                                        setGroupField(group.id, (current) => ({
                                          ...current,
                                          devices: current.devices.map((item, idx) => (
                                            idx === deviceIndex ? { ...item, hostname: nextHostname } : item
                                          )),
                                        }))
                                      }}
                                    />
                                    <button
                                      className="btn-ghost btn-icon-sm h-[34px] w-[34px] min-w-[34px] flex-shrink-0 text-danger hover:bg-danger/10"
                                      title="删除设备"
                                      onClick={() => removeDevice(group.id, deviceIndex)}
                                    >
                                      <Trash2 size={13} />
                                    </button>
                                  </div>

                                  {(poolClients.length === 0 || unresolvedIP) ? (
                                    <div className="mt-2 grid gap-2 sm:grid-cols-[minmax(0,1fr)_90px]">
                                      <input
                                        className="glass-input min-h-[34px] h-[34px] font-mono"
                                        placeholder="手动填写 IP，例如 192.168.1.100"
                                        value={device.ip}
                                        onChange={(event) => {
                                          const nextIP = event.target.value.trim()
                                          setGroupField(group.id, (current) => ({
                                            ...current,
                                            devices: current.devices.map((item, idx) => {
                                              if (idx !== deviceIndex) return item
                                              const nextPrefix = item.prefix || defaultPrefixForIP(nextIP || item.ip || '0.0.0.0')
                                              return { ...item, ip: nextIP, prefix: clampPrefix(nextPrefix, nextIP || item.ip || '0.0.0.0') }
                                            }),
                                          }))
                                        }}
                                      />
                                      <input
                                        type="number"
                                        min={1}
                                        max={device.ip.includes(':') ? 128 : 32}
                                        className="glass-input min-h-[34px] h-[34px] text-center"
                                        value={device.prefix || defaultPrefixForIP(device.ip || '0.0.0.0')}
                                        onChange={(event) => {
                                          const parsed = Number(event.target.value)
                                          setGroupField(group.id, (current) => ({
                                            ...current,
                                            devices: current.devices.map((item, idx) => (
                                              idx === deviceIndex
                                                ? { ...item, prefix: clampPrefix(parsed, item.ip || device.ip || '0.0.0.0') }
                                                : item
                                            )),
                                          }))
                                        }}
                                      />
                                    </div>
                                  ) : null}

                                  <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted">
                                    <span className="font-mono">
                                      {device.ip ? `IP ${device.ip}` : '尚未选择设备'}
                                    </span>
                                    {selectedClient?.mac ? <span className="font-mono">{selectedClient.mac}</span> : null}
                                    <span>前缀 /{device.prefix || defaultPrefixForIP(device.ip || '0.0.0.0')}</span>
                                    {unresolvedIP ? (
                                      <span className="inline-flex items-center gap-1 text-warning">
                                        <AlertCircle size={11} />
                                        该设备不在当前在线列表中
                                      </span>
                                    ) : null}
                                  </div>
                                </div>
                              )
                            })
                          )}
                        </div>
                      </div>

                      <div className="rounded-lg border border-white/10 bg-surface-2/40 p-3">
                        <div className="mb-3 flex items-center justify-between">
                          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-muted">策略覆盖</p>
                          <button
                            className="btn-ghost h-7 px-2.5 text-xs"
                            onClick={() => addOverride(group.id)}
                            disabled={policyGroups.length === 0}
                          >
                            <Plus size={12} />
                            添加覆盖
                          </button>
                        </div>
                        <div className="space-y-3">
                          {group.overrides.length === 0 ? (
                            <p className="rounded-md border border-dashed border-white/15 px-3 py-3 text-xs text-muted">
                              暂无覆盖，设备将走全局策略组。
                            </p>
                          ) : (
                            group.overrides.map((override, overrideIndex) => {
                              const selectedGroup = policyGroupMap.get(override.original_group)
                              const knownMembers = selectedGroup?.proxies ?? []
                              const memberOptions = dedupe([...knownMembers, ...override.proxies])

                              return (
                                <div
                                  key={`${group.id}-override-${overrideIndex}`}
                                  className="rounded-lg border border-white/10 bg-black/20 p-3"
                                >
                                  <div className="flex items-start gap-2">
                                    <select
                                      className="theme-select glass-input min-h-[34px] h-[34px] w-full"
                                      value={override.original_group}
                                      onChange={(event) => {
                                        const nextGroupName = event.target.value
                                        const nextMembers = new Set(policyGroupMap.get(nextGroupName)?.proxies ?? [])
                                        setGroupField(group.id, (current) => ({
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
                                      {policyGroups.map((item) => (
                                        <option key={item.name} value={item.name}>
                                          {item.name}
                                        </option>
                                      ))}
                                      {override.original_group && !policyGroupMap.has(override.original_group) ? (
                                        <option value={override.original_group}>
                                          {override.original_group}（失效引用）
                                        </option>
                                      ) : null}
                                    </select>
                                    <button
                                      className="btn-ghost btn-icon-sm h-[34px] w-[34px] min-w-[34px] flex-shrink-0 text-danger hover:bg-danger/10"
                                      title="删除覆盖"
                                      onClick={() => removeOverride(group.id, overrideIndex)}
                                    >
                                      <Trash2 size={13} />
                                    </button>
                                  </div>

                                  <div className="mt-2 flex flex-wrap items-center gap-2">
                                    {memberOptions.length === 0 ? (
                                      <span className="text-xs text-muted">
                                        {override.original_group ? '该策略组没有可选节点。' : '先选择策略组后再选节点。'}
                                      </span>
                                    ) : (
                                      memberOptions.map((member) => {
                                        const selected = override.proxies.includes(member)
                                        const known = knownProxySet.has(member)
                                        const flag = detectProxyRegion(member)
                                        return (
                                          <button
                                            key={member}
                                            type="button"
                                            className={[
                                              'inline-flex items-center gap-1 rounded-md border px-2 py-1 text-xs font-medium transition-all',
                                              selected
                                                ? 'border-brand/50 bg-brand/15 text-brand-light'
                                                : known
                                                  ? 'border-white/12 bg-white/5 text-slate-300 hover:bg-white/8'
                                                  : 'border-warning/35 bg-warning/10 text-warning',
                                            ].join(' ')}
                                            onClick={() => {
                                              setGroupField(group.id, (current) => ({
                                                ...current,
                                                overrides: current.overrides.map((item, idx) => {
                                                  if (idx !== overrideIndex) return item
                                                  if (item.proxies.includes(member)) {
                                                    return { ...item, proxies: item.proxies.filter((proxy) => proxy !== member) }
                                                  }
                                                  return { ...item, proxies: [...item.proxies, member] }
                                                }),
                                              }))
                                            }}
                                          >
                                            {flag && <span className="text-[13px] leading-none">{flag}</span>}
                                            {member}
                                          </button>
                                        )
                                      })
                                    )}
                                  </div>

                                  {knownMembers.length > 0 ? (
                                    <div className="mt-2 flex items-center justify-between">
                                      <span className="text-[11px] text-muted">
                                        已选 {override.proxies.length} / {knownMembers.length}
                                      </span>
                                      <div className="flex items-center gap-2">
                                        <button
                                          className="btn-ghost h-6 px-2 text-[11px]"
                                          onClick={() => {
                                            setGroupField(group.id, (current) => ({
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
                                            setGroupField(group.id, (current) => ({
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
                                  ) : null}
                                </div>
                              )
                            })
                          )}
                        </div>
                      </div>
                    </div>
                  </section>
                )
              })}
            </div>
          )}
        </SectionCard>
      )}

      {/* Live config and rule explanation — always visible */}
      <ConfigPreviewPanel
        yaml={finalConfigContent}
        loading={loadingFinalConfig}
        error={finalConfigError}
        groups={groups}
        policyGroupMap={policyGroupMap}
        onRefresh={() => { void loadFinalConfig(selectedSourceKey, groups) }}
      />

      {/* Generation logic hints — geek mode only */}
      {viewMode === 'geek' && (
        <>
          <SectionCard
            title="生成逻辑提示"
            description="保存后 ClashForge 会自动生成影子策略组和按设备 AND 规则，并保留原始 top rules 作为兜底。"
          >
            <div className="grid gap-3 md:grid-cols-3">
              <div className="rounded-lg border border-white/10 bg-surface-2/40 p-3">
                <div className="flex items-center gap-2 text-slate-200">
                  <Network size={14} className="text-brand" />
                  <p className="text-sm font-semibold">影子策略组</p>
                </div>
                <p className="mt-2 text-xs leading-5 text-muted">
                  为每个设备分组与原始策略组组合生成独立策略组，例如
                  <span className="mx-1 font-mono text-slate-300">iPhone - 节点选择</span>。
                </p>
              </div>
              <div className="rounded-lg border border-white/10 bg-surface-2/40 p-3">
                <div className="flex items-center gap-2 text-slate-200">
                  <Smartphone size={14} className="text-brand" />
                  <p className="text-sm font-semibold">按设备匹配</p>
                </div>
                <p className="mt-2 text-xs leading-5 text-muted">
                  对命中的策略规则展开
                  <span className="mx-1 font-mono text-slate-300">RULE-SET(设备组,src) + RULE-SET</span>
                  组合匹配，减少规则体积并按设备精准选路。
                </p>
              </div>
              <div className="rounded-lg border border-white/10 bg-surface-2/40 p-3">
                <div className="flex items-center gap-2 text-slate-200">
                  <CheckCircle2 size={14} className="text-success" />
                  <p className="text-sm font-semibold">安全兜底</p>
                </div>
                <p className="mt-2 text-xs leading-5 text-muted">
                  REJECT / DIRECT 保持全局共享；MATCH 会在其前插入按设备兜底规则，同时保留原始 MATCH 作为最终全局兜底。
                </p>
              </div>
            </div>
            <div className="mt-4 flex items-start gap-2 rounded-lg border border-brand/20 bg-brand/10 px-3 py-3 text-xs text-brand-light">
              <AlertCircle size={14} className="mt-0.5 flex-shrink-0" />
              <p className="leading-5">
                建议为关键设备绑定静态 IP（DHCP Static Lease），否则设备 IP 变化后将无法命中对应分组规则。
              </p>
            </div>
          </SectionCard>
        </>
      )}
    </div>
  )
}
