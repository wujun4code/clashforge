const BASE = '/api/v1'

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const secret = localStorage.getItem('cf_secret') || ''
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(secret ? { Authorization: `Bearer ${secret}` } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  })
  const json = await res.json()
  if (!json.ok) throw new Error(json.error?.message ?? 'request failed')
  return json.data as T
}

// ---- types ----
export interface StatusData {
  metaclash: { version: string; uptime: number; config_file: string }
  core: { state: string; pid: number; restarts: number; uptime: number }
  network: { mode: string; firewall_backend: string; apply_on_start: boolean; rules_applied: boolean }
  dns: { enable: boolean; dnsmasq_mode: string; apply_on_start: boolean }
  subscriptions: { total: number; enabled: number; last_updated: string | null }
}

export interface HealthProcess {
  ok: boolean
  message: string
  pid?: number
  state?: string
  uptime?: number
}

export interface HealthPort {
  name: string
  port: number
  proto: string
  required: boolean
  listening: boolean
  message: string
}

export interface HealthTakeover {
  configured: boolean
  apply_on_start: boolean
  active: boolean
  mode?: string
  backend?: string
  rules_applied?: boolean
  table_present?: boolean
  message: string
}

export interface HealthDNS {
  enabled: boolean
  apply_on_start: boolean
  dnsmasq_mode: string
  active: boolean
  managed_file_present: boolean
  listener_ready: boolean
  message: string
}

export interface HealthProxyTest {
  name: string
  port: number
  listening: boolean
  ok: boolean
  status_code?: number
  duration_ms?: number
  error?: string
}

export interface HealthCheckData {
  checked_at: string
  summary: { healthy: boolean; failures: number; warnings: number }
  process: { clashforge: HealthProcess; mihomo: HealthProcess }
  ports: HealthPort[]
  transparent_proxy: HealthTakeover
  nft: HealthTakeover
  dns: HealthDNS
  proxy_tests: {
    target_url: string
    http: HealthProxyTest
    mixed: HealthProxyTest
    socks: HealthProxyTest
    mihomo_api: HealthProxyTest
  }
}

export interface OverviewSummary {
  core_running: boolean
  clashforge_healthy: boolean
  conflict_count: number
  takeover_ready: number
  message: string
}

export interface OverviewCoreInfo {
  state: string
  pid: number
  uptime: number
  running: boolean
  active_connections: number
}

export interface OverviewSystemUsage {
  cpu_percent: number
  memory_total_mb: number
  memory_used_mb: number
  memory_percent: number
  disk_total_gb: number
  disk_used_gb: number
  disk_percent: number
}

export interface OverviewProcessUsage {
  id: string
  name: string
  pid: number
  running: boolean
  cpu_percent: number
  memory_rss_mb: number
  uptime: number
  command?: string
}

export interface OverviewAppStorage {
  runtime_mb: number
  data_mb: number
  binary_mb: number
  rules_mb: number
  total_mb: number
  rule_assets?: OverviewRuleAsset[]
}

export interface OverviewRuleAsset {
  name: string
  path: string
  size_mb: number
}

export interface OverviewIPCheck {
  provider: string
  group?: string
  ok: boolean
  ip?: string
  location?: string
  error?: string
}

export interface OverviewAccessCheck {
  name: string
  group?: string
  url: string
  description: string
  via: string
  ok: boolean
  status_code?: number
  latency_ms?: number
  error?: string
  stage?: string      // 'proxy_port' | 'dns' | 'timeout' | 'connect'
  dns_result?: string // resolved IP(s) from router-side probe
}

export interface OverviewProcessRef {
  pid: number
  name: string
  command?: string
  service?: string
}

export interface OverviewPortOwner {
  port: number
  proto: string
  owner: string
  pid?: number
  command?: string
}

export interface OverviewAction {
  module: string
  label: string
  mode?: string
  stop_services?: string[]
}

export interface OverviewModule {
  id: string
  title: string
  category: string
  status: 'active' | 'conflict' | 'available' | 'inactive' | string
  current_owner: string
  managed_by_clashforge: boolean
  purpose: string
  takeover_effect: string
  current_mode?: string
  recommended_mode?: string
  takeover_supported: boolean
  action?: OverviewAction
  processes?: OverviewProcessRef[]
  ports?: OverviewPortOwner[]
  notes?: string[]
}

export interface OverviewInfluence {
  id: string
  name: string
  description: string
  affects: string[]
  running: boolean
  stoppable: boolean
  service?: string
  processes?: OverviewProcessRef[]
  ports?: OverviewPortOwner[]
}

export interface OverviewData {
  checked_at: string
  summary: OverviewSummary
  resources: {
    system: OverviewSystemUsage
    processes: OverviewProcessUsage[]
    app: OverviewAppStorage
  }
  ip_checks: OverviewIPCheck[]
  access_checks: OverviewAccessCheck[]
  modules: OverviewModule[]
  influences: OverviewInfluence[]
}

export interface OverviewCoreData {
  checked_at: string
  core: OverviewCoreInfo
  summary: OverviewSummary
  modules: OverviewModule[]
  influences: OverviewInfluence[]
}

export interface OverviewProbeData {
  checked_at: string
  ip_checks: OverviewIPCheck[]
  access_checks: OverviewAccessCheck[]
}

export interface OverviewResourceData {
  checked_at: string
  resources: {
    system: OverviewSystemUsage
    processes: OverviewProcessUsage[]
    app: OverviewAppStorage
  }
}

export interface OverviewTakeoverResponse {
  updated: boolean
  message: string
  stopped?: string[]
  needs_restart?: boolean
  overview: OverviewCoreData
}

export interface OverviewReleaseResponse {
  updated: boolean
  message: string
  overview: OverviewCoreData
}

export interface ProxyNode {
  name: string; type: string; server?: string; port?: number
  history?: { time: string; delay: number }[]
  alive?: boolean
  now?: string; all?: string[]; udp?: boolean
}

export interface ProxiesData { proxies: Record<string, ProxyNode> }

export interface Subscription {
  id: string; name: string; type: string; url?: string
  enabled: boolean; last_updated?: string; node_count?: number
  user_agent?: string; interval?: string
  has_cache?: boolean
  filter?: { include?: string[]; exclude?: string[]; max_nodes?: number }
}

export interface Connection {
  id: string; upload: number; download: number; start: string
  chains: string[]
  metadata: { host: string; destinationPort: number; type: string; network: string; sourceIP: string }
}

export interface LogEntry { level: string; msg: string; ts: number; fields?: Record<string, unknown> }

export interface DeviceRouteDevice {
  ip: string
  prefix: number
  hostname?: string
}

export interface DeviceRouteOverride {
  original_group: string
  proxies: string[]
}

export interface DeviceRouteGroup {
  id: string
  name: string
  devices: DeviceRouteDevice[]
  overrides: DeviceRouteOverride[]
  order: number
}

export interface NetworkClient {
  ip: string
  mac?: string
  hostname?: string
  interface?: string
  source?: string
}

export const getStatus        = () => request<StatusData>('GET', '/status')
export const getOverview      = () => request<OverviewData>('GET', '/overview')
export const getOverviewCore  = () => request<OverviewCoreData>('GET', '/overview/core')
export const getOverviewProbes = () => request<OverviewProbeData>('GET', '/overview/probes')
export const getOverviewResources = () => request<OverviewResourceData>('GET', '/overview/resources')
export const getHealthCheck   = (target?: string) => request<HealthCheckData>('GET', `/health/check${target ? `?target=${encodeURIComponent(target)}` : ''}`)
export const takeoverOverviewModule = (payload: { module: string; mode?: string; stop_services?: string[] }) => request<OverviewTakeoverResponse>('POST', '/overview/takeover', payload)
export const releaseOverviewTakeover = () => request<OverviewReleaseResponse>('POST', '/overview/release')
export const startCore        = () => request('POST', '/core/start')
export const stopCore         = () => request('POST', '/core/stop')
export const restartCore      = () => request('POST', '/core/restart')
export const reloadCore       = () => request('POST', '/core/reload')
export const getCoreVersion   = () => request<{current:string;latest:string;has_update:boolean}>('GET', '/core/version')
export interface ClashforgeVersionData {
  current: string
  latest: string
  has_update: boolean
  download_url: string
  release_url: string
  release_notes: string
  channel: string
}
export const getClashforgeVersion = (channel: 'stable' | 'preview' = 'stable') =>
  request<ClashforgeVersionData>('GET', `/clashforge/version?channel=${channel}`)
export const getProxies       = () => request<ProxiesData>('GET', '/proxies')
export const selectProxy      = (group: string, name: string) => request('PUT', `/proxies/${encodeURIComponent(group)}/select`, { name })
export const testLatency      = async (proxies: string[]): Promise<Record<string, number>> => {
  const TEST_URL = 'http://www.gstatic.com/generate_204'
  const TIMEOUT  = 5000
  const results = await Promise.allSettled(
    proxies.map(name =>
      request<{ delay: number }>('GET', `/proxies/${encodeURIComponent(name)}/delay?url=${encodeURIComponent(TEST_URL)}&timeout=${TIMEOUT}`)
        .then(d => ({ name, delay: d.delay }))
        .catch(() => ({ name, delay: -1 }))
    )
  )
  return Object.fromEntries(
    results.map((r, i) => [proxies[i], r.status === 'fulfilled' ? r.value.delay : -1])
  )
}
export const getConnections   = () => request<{connections: Connection[]}>('GET', '/connections')
export const closeAllConns    = () => request('DELETE', '/connections')
export const getSubscriptions = () => request<{subscriptions: Subscription[]}>('GET', '/subscriptions')
export const addSubscription  = (s: Partial<Subscription>) => request<{id:string}>('POST', '/subscriptions', s)
export const updateSubscription = (id: string, p: Partial<Subscription>) => request('PUT', `/subscriptions/${id}`, p)
export const deleteSubscription = (id: string) => request('DELETE', `/subscriptions/${id}`)
export const triggerSubUpdate   = (id: string) => request('POST', `/subscriptions/${id}/update`)
export const syncSubUpdate      = (id: string) => request('POST', `/subscriptions/${id}/sync-update`)
export const getSubscriptionCache = (id: string) => request<{id: string; content: string}>('GET', `/subscriptions/${id}/cache`)
export const triggerUpdateAll   = () => request('POST', '/subscriptions/update-all')
export const getConfig        = () => request<Record<string,unknown>>('GET', '/config')
export const updateConfig     = (p: Record<string,unknown>) => request('PUT', '/config', p)
export const getOverrides     = () => request<{content:string}>('GET', '/config/overrides')
export const updateOverrides  = (content: string) => request('PUT', '/config/overrides', { content })
export const generateConfig   = () => request<{generated: boolean; config_file: string}>('POST', '/config/generate')
export const getMihomoConfig  = () => request<{content:string}>('GET', '/config/mihomo')
export const getDeviceGroups  = () => request<{device_groups: DeviceRouteGroup[]}>('GET', '/config/device-groups')
export const updateDeviceGroups = (device_groups: DeviceRouteGroup[]) =>
  request<{
    updated: boolean
    config_generated: boolean
    core_running?: boolean
    core_reloaded?: boolean
    warning?: string
    reload_error?: string
  }>('PUT', '/config/device-groups', { device_groups })
export const getNetworkClients = () => request<{clients: NetworkClient[]}>('GET', '/network/clients')
export const getLogs          = (level = 'info', limit = 200) => request<{logs: LogEntry[]}>('GET', `/logs?level=${level}&limit=${limit}`)
export const clearLogs        = () => request<{ok: boolean}>('DELETE', '/logs')
export const pauseLogs        = () => request<{ok: boolean; paused: boolean}>('POST', '/logs/pause')
export const resumeLogs       = () => request<{ok: boolean; paused: boolean}>('POST', '/logs/resume')
export const getLogsStatus    = () => request<{paused: boolean}>('GET', '/logs/status')
export const enableService    = () => request<{enabled: boolean}>('POST', '/service/enable')
export const stopService      = (target: 'openclash' | 'clashforge-full') => request<{ok: boolean; target: string; output: string}>('POST', '/system/stop-service', { target })
export interface ConflictService { name: string; label: string; running: boolean; pids?: number[] }
export const detectConflicts  = () => request<{conflicts: ConflictService[]; has_conflict: boolean}>('GET', '/system/conflicts')
export const resetClashForge  = () => request<{ok: boolean; message: string}>('POST', '/system/reset')

// ---- setup port check ----
export interface SetupPortCheck {
  name: string
  description: string
  port: number
  required: boolean
  ok: boolean
  latency_ms?: number
  error?: string
}
export const checkSetupPorts = () => request<{ checks: SetupPortCheck[] }>('GET', '/setup/port-check')
export const previewSetupFinalConfig = (payload: {
  dns: { enable: boolean; mode: string; dnsmasq_mode: string; apply_on_start: boolean }
  network: { mode: string; firewall_backend: string; bypass_lan: boolean; bypass_china: boolean; apply_on_start: boolean; ipv6: boolean }
}) => request<{ config_file: string; content: string }>('POST', '/setup/final-config-preview', payload)

// ---- rule providers ----
export interface RuleProvider {
  name: string
  type: string
  vehicleType: string
  behavior: string
  format: string
  ruleCount: number
  updatedAt: string
  file_path?: string
  size_mb: number
}
export interface RuleSearchResult {
  provider: string
  behavior: string
  matches: string[]
  total: number
}
export const getRuleProviders   = () => request<{providers: RuleProvider[]}>('GET', '/rules/providers')
export const syncRuleProvider   = (name: string) => request<{ok:boolean;name:string}>('POST', `/rules/providers/${encodeURIComponent(name)}/sync`)
export const syncAllRuleProviders = () => request<{ok:boolean;results:{name:string;ok:boolean;error?:string}[]}>('POST', '/rules/providers/sync-all')
export const searchRules        = (q: string, provider?: string) => {
  const qs = provider ? `q=${encodeURIComponent(q)}&provider=${encodeURIComponent(provider)}` : `q=${encodeURIComponent(q)}`
  return request<{query:string;results:RuleSearchResult[]}>('GET', `/rules/search?${qs}`)
}

// ---- config sources ----
export interface SourceFile {
  filename: string
  created_at: string
  size_bytes: number
  active: boolean
}

export interface ActiveSource {
  type: 'file' | 'subscription'
  filename?: string
  sub_id?: string
  sub_name?: string
}

export const getSources       = () => request<{files: SourceFile[]; active_source: ActiveSource | null}>('GET', '/config/sources')
export const saveSource       = (content: string, suggested_name?: string) => request<{filename: string}>('POST', '/config/sources', { content, suggested_name })
export const getSourceFile    = (filename: string) => request<{filename: string; content: string}>('GET', `/config/sources/${encodeURIComponent(filename)}`)
export const deleteSourceFile = (filename: string) => request('DELETE', `/config/sources/${encodeURIComponent(filename)}`)
export const getActiveSource  = () => request<{active_source: ActiveSource | null}>('GET', '/config/active-source')
export const setActiveSource  = (as: ActiveSource) => request<{updated: boolean}>('PUT', '/config/active-source', as)

// ---- node server management ----
export interface NodeListItem {
  id: string
  name: string
  host: string
  port: number
  username: string
  domain: string
  status: 'pending' | 'connected' | 'deploying' | 'deployed' | 'error'
  deployed_at?: string
  cert_expiry?: string
  error?: string
  deploy_log?: string
  created_at: string
  updated_at: string
}

export interface NodeCreateRequest {
  name: string
  host: string
  port: number
  username: string
  password: string
  domain: string
  email: string
  cf_token: string
  cf_account_id: string
  cf_zone_id: string
}

export interface NodeProbeResult {
  name: string
  url: string
  ok: boolean
  status_code?: number
  latency_ms?: number
  error?: string
}

export interface CloudflareZone {
  id: string
  name: string
  status: string
}

export const getNodeSSHPubKey = () => request<{ public_key: string }>('GET', '/nodes/ssh-pubkey')
export const getCloudflareZones = (payload: { cf_token: string; cf_account_id?: string }) =>
  request<{ zones: CloudflareZone[] }>('POST', '/nodes/cloudflare/zones', payload)
export const getNodes = () => request<{ nodes: NodeListItem[] }>('GET', '/nodes')
export const getNode = (id: string) => request<{ node: NodeListItem }>('GET', `/nodes/${encodeURIComponent(id)}`)
export const createNode = (node: NodeCreateRequest) => request<{ node: NodeListItem }>('POST', '/nodes', node)
export const updateNode = (id: string, node: Partial<NodeCreateRequest>) => request<{ node: NodeListItem }>('PUT', `/nodes/${encodeURIComponent(id)}`, node)
export const deleteNode = (id: string) => request<{ ok: boolean }>('DELETE', `/nodes/${encodeURIComponent(id)}`)
export const testNodeConnection = (id: string) => request<{ ok: boolean; message: string }>('POST', `/nodes/${encodeURIComponent(id)}/test`)
export const probeNode = (id: string, mode: 'ip' | 'domain' = 'ip') =>
  request<{ mode: 'ip' | 'domain'; proxy_host: string; proxy_port: number; probe_results: NodeProbeResult[]; summary: { ok: number; total: number; success: boolean } }>('POST', `/nodes/${encodeURIComponent(id)}/probe`, { mode })

export interface DomainProbeResult {
  domain: string
  checked_at: string
  dns_ips?: string[]
  dns_error?: string
  ok: boolean
  latency_ms?: number
  status_code?: number
  error?: string
}

export const probeDomain = (domain: string) =>
  request<DomainProbeResult>('POST', '/health/probe-domain', { domain })


