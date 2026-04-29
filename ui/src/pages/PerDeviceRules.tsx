import { useCallback, useEffect, useMemo, useState } from 'react'
import yaml from 'js-yaml'
import {
  AlertCircle,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  Loader2,
  Network,
  Plus,
  RefreshCw,
  Save,
  Smartphone,
  Trash2,
} from 'lucide-react'

import { EmptyState, InlineNotice, PageHeader, SectionCard } from '../components/ui'
import {
  getDeviceGroups,
  getMihomoConfig,
  getNetworkClients,
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
    devices: [newDevice()],
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

export function PerDeviceRules() {
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
  }, [])

  const loadPolicyOptions = useCallback(async (source: DeviceRuleSourceOption | null) => {
    setLoadingOptions(true)
    setOptionsError('')
    try {
      if (!source) {
        setOptions({ groups: [], knownProxyNames: [...BUILTIN_PROXY_NAMES] })
        setOptionsError('请先在“配置管理”中准备至少一个配置文件或订阅。')
        return
      }

      let content = ''
      if (source.type === 'file' && source.filename) {
        const data = await getSourceFile(source.filename)
        content = data.content ?? ''
      } else if (source.type === 'subscription' && source.subscription) {
        try {
          const data = await getSubscriptionCache(source.subscription.id)
          content = data.content ?? ''
        } catch (cacheError) {
          if (source.key === activeSourceKey) {
            const running = await getMihomoConfig()
            content = running.content ?? ''
          } else {
            throw cacheError
          }
        }
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
  }, [activeSourceKey])

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

  const loadFinalConfig = useCallback(async () => {
    setLoadingFinalConfig(true)
    setFinalConfigError('')
    try {
      const data = await getMihomoConfig()
      setFinalConfigContent(data.content ?? '')
    } catch (error) {
      setFinalConfigContent('')
      setFinalConfigError(error instanceof Error ? error.message : '读取最终配置失败')
    } finally {
      setLoadingFinalConfig(false)
    }
  }, [])

  const refreshAll = useCallback(async () => {
    setLoading(true)
    setNotice(null)
    try {
      await loadSourceOptions()
      await Promise.all([loadNetworkClients(), loadFinalConfig()])
    } catch (error) {
      setNotice({
        tone: 'danger',
        title: '加载失败',
        text: error instanceof Error ? error.message : '读取配置失败',
      })
    } finally {
      setLoading(false)
    }
  }, [loadFinalConfig, loadNetworkClients, loadSourceOptions])

  useEffect(() => {
    void refreshAll()
  }, [refreshAll])

  useEffect(() => {
    if (!selectedSourceKey) {
      setGroups([])
      setSnapshot('[]')
      setOptions({ groups: [], knownProxyNames: [...BUILTIN_PROXY_NAMES] })
      return
    }

    setLoading(true)
    setNotice(null)
    void Promise.all([
      loadDeviceGroups(selectedSourceKey),
      loadPolicyOptions(selectedSource),
    ])
      .catch((error) => {
        setNotice({
          tone: 'danger',
          title: '加载失败',
          text: error instanceof Error ? error.message : '读取配置失败',
        })
      })
      .finally(() => setLoading(false))
  }, [loadDeviceGroups, loadPolicyOptions, selectedSource, selectedSourceKey])

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

      await Promise.all([loadPolicyOptions(selectedSource), loadFinalConfig()])
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
            {selectedSource ? `${selectedSource.description}` : '请先在“配置管理”导入配置来源'}
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
                            暂无设备，点击“添加设备”从设备池中选择。
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

      <SectionCard
        title="最终配置原文"
        description="保存设备分组后，这里会展示当前完整的 Mihomo 最终配置文件。"
        actions={(
          <button
            className="btn-ghost flex items-center gap-2"
            onClick={() => { void loadFinalConfig() }}
            disabled={loadingFinalConfig}
          >
            <RefreshCw size={14} className={loadingFinalConfig ? 'animate-spin' : ''} />
            刷新配置
          </button>
        )}
      >
        {finalConfigError ? (
          <InlineNotice tone="warning" title="读取失败">
            {finalConfigError}
          </InlineNotice>
        ) : null}

        {loadingFinalConfig ? (
          <div className="flex min-h-[140px] items-center justify-center rounded-lg border border-white/10 bg-black/20 text-sm text-muted">
            <Loader2 size={14} className="mr-2 animate-spin text-brand" />
            正在加载最终配置…
          </div>
        ) : finalConfigContent.trim() ? (
          <textarea
            className="glass-textarea h-[460px] resize-y text-xs leading-5"
            value={finalConfigContent}
            readOnly
            spellCheck={false}
          />
        ) : (
          <div className="rounded-lg border border-dashed border-white/15 bg-black/20 px-4 py-8 text-center text-sm text-muted">
            尚未生成最终配置，请先点击“保存并应用”。
          </div>
        )}
      </SectionCard>

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
    </div>
  )
}
