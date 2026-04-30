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

export interface HealthBrowserIPCheck {
  provider: string
  group?: string
  ok: boolean
  ip?: string
  location?: string
  error?: string
}

export interface HealthBrowserAccessCheck {
  name: string
  group?: string
  url?: string
  ok: boolean
  latency_ms?: number
  error?: string
  stage?: string
}

export interface HealthBrowserReportRequest {
  session_id: string
  checked_at: string
  user_agent?: string
  ip_checks: HealthBrowserIPCheck[]
  access_checks: HealthBrowserAccessCheck[]
}

export interface HealthProbeSummary {
  has_data: boolean
  healthy: boolean
  ip_ok: boolean
  failed_access?: string[]
  checked_at?: string
  stale?: boolean
  error?: string
}

export interface HealthCurrentState {
  state: string
  since: string
  last_reason?: string
  consecutive_failures: number
  consecutive_successes: number
  active_incident_id?: string
  last_router_check?: string
  last_browser_check?: string
}

export interface HealthSummaryData {
  checked_at: string
  current: HealthCurrentState
  router: HealthProbeSummary
  browser: HealthProbeSummary
  open_incidents: number
  pending_notifications: number
  webhook_configured: boolean
  notification_channel?: string
  router_interval_sec?: number
  browser_ttl_sec?: number
}

export interface HealthIncident {
  id: string
  status: string
  state: string
  reason: string
  opened_at: string
  updated_at: string
  resolved_at?: string
  router: HealthProbeSummary
  browser: HealthProbeSummary
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

export interface DeviceGroupsResponse {
  device_groups: DeviceRouteGroup[]
  source_key?: string
  active_source_key?: string
  requested_by_source?: boolean
}

export interface DeviceGroupsPreviewResponse {
  source_key: string
  active_source_key?: string
  profile_active?: boolean
  content: string
}

export interface NetworkClient {
  ip: string
  mac?: string
  hostname?: string
  ips?: string[]
  interface?: string
  source?: string
}

export const getStatus        = () => request<StatusData>('GET', '/status')
export const getOverview      = () => request<OverviewData>('GET', '/overview')
export const getOverviewCore  = () => request<OverviewCoreData>('GET', '/overview/core')
export const getOverviewProbes = () => request<OverviewProbeData>('GET', '/overview/probes')
export const getOverviewResources = () => request<OverviewResourceData>('GET', '/overview/resources')
export const getHealthCheck   = (target?: string) => request<HealthCheckData>('GET', `/health/check${target ? `?target=${encodeURIComponent(target)}` : ''}`)
export const getHealthSummary = () => request<HealthSummaryData>('GET', '/health/summary')
export const getHealthIncidents = (limit = 50) => request<{ incidents: HealthIncident[] }>('GET', `/health/incidents?limit=${limit}`)
export const reportHealthBrowser = (payload: HealthBrowserReportRequest) =>
  request<{ ok: boolean; browser: HealthProbeSummary; summary: HealthSummaryData }>('POST', '/health/browser-report', payload)
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
export const getDeviceGroups  = (source_key?: string) =>
  request<DeviceGroupsResponse>('GET', `/config/device-groups${source_key ? `?source_key=${encodeURIComponent(source_key)}` : ''}`)
export const updateDeviceGroups = (device_groups: DeviceRouteGroup[], source_key?: string) =>
  request<{
    updated: boolean
    config_generated: boolean
    core_running?: boolean
    core_reloaded?: boolean
    warning?: string
    reload_error?: string
    profile_active?: boolean
    profile_source_key?: string
    active_source_key?: string
    message?: string
  }>('PUT', '/config/device-groups', source_key ? { device_groups, source_key } : { device_groups })
export const previewDeviceGroupsConfig = (device_groups: DeviceRouteGroup[], source_key?: string) =>
  request<DeviceGroupsPreviewResponse>('POST', '/config/device-groups/preview', source_key ? { device_groups, source_key } : { device_groups })
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

// ---- worker-node proxy ----
export interface WorkerNodeListItem {
  id: string
  name: string
  worker_name: string
  cf_account_id: string
  hostname: string
  worker_url: string
  worker_dev_url: string
  status: 'pending' | 'deployed' | 'error'
  error?: string
  deployed_at?: string
  created_at: string
  updated_at: string
}

export interface WorkerNodeCreateRequest {
  name: string
  worker_name: string
  cf_token: string
  cf_account_id: string
  cf_zone_id: string
  hostname: string
}

export const getWorkerNodes = () =>
  request<{ nodes: WorkerNodeListItem[] }>('GET', '/worker-nodes')
export const createWorkerNode = (req: WorkerNodeCreateRequest) =>
  request<{ node: WorkerNodeListItem; clash_config: string }>('POST', '/worker-nodes', req)
export const redeployWorkerNode = (id: string) =>
  request<{ node: WorkerNodeListItem }>('POST', `/worker-nodes/${encodeURIComponent(id)}/redeploy`)
export const deleteWorkerNode = (id: string) =>
  request<{ status: string }>('DELETE', `/worker-nodes/${encodeURIComponent(id)}`)
export const getWorkerNodeClashConfig = (id: string) =>
  request<{ yaml: string; name: string }>('GET', `/worker-nodes/${encodeURIComponent(id)}/clash-config`)

// ---- publish workflow ----
export type PublishTemplateMode = 'builtin' | 'runtime' | 'custom'

export interface PublishNode {
  id: string
  name: string
  host: string
  domain: string
  status: string
  has_credentials: boolean
  node_type?: 'ssh' | 'worker'
}

export interface PublishTemplatePreset {
  id: string
  name: string
  description: string
}

export interface PublishPreviewPayload {
  node_ids: string[]
  template_mode: PublishTemplateMode
  template_id?: string
  template_content?: string
}

export interface PublishPreviewResponse {
  content: string
  node_count: number
  template_mode: string
  managed_groups?: string[]
}

export interface PublishWorkerConfig {
  id: string
  name: string
  worker_name: string
  worker_url: string
  worker_dev_url: string
  hostname: string
  account_id: string
  namespace_id: string
  zone_id: string
  has_token: boolean
  initialized_at: string
  created_at: string
  updated_at: string
}

export interface PublishWorkerConfigInput {
  id?: string
  name: string
  worker_name: string
  worker_url: string
  worker_dev_url: string
  hostname: string
  account_id: string
  namespace_id: string
  zone_id: string
  token?: string
  initialized_at?: string
}

export interface PublishPermissionCheck {
  name: string
  ok: boolean
  error?: string
}

export interface PublishVerifyTest {
  name: string
  ok: boolean
  detail?: string
}

export interface PublishWorkerVerifyResult {
  ok: boolean
  tests: PublishVerifyTest[]
  used_url?: string
  hello_url?: string
  note?: string
}

export interface PublishRecord {
  id: string
  worker_config_id: string
  worker_name: string
  hostname: string
  base_name: string
  version: number
  file_name: string
  access_url: string
  published_at: string
}

export interface PublishUploadPayload {
  worker_config_id: string
  base_name: string
  content?: string
  node_ids?: string[]
  template_mode?: PublishTemplateMode
  template_id?: string
  template_content?: string
}

export const getPublishNodes = () => request<{ nodes: PublishNode[] }>('GET', '/publish/nodes')
export const getPublishTemplates = () => request<{ templates: PublishTemplatePreset[] }>('GET', '/publish/templates')
export const previewPublishConfig = (payload: PublishPreviewPayload) =>
  request<PublishPreviewResponse>('POST', '/publish/preview', payload)
export const getPublishWorkerConfigs = () =>
  request<{ configs: PublishWorkerConfig[] }>('GET', '/publish/worker-configs')
export const createPublishWorkerConfig = (payload: PublishWorkerConfigInput) =>
  request<{ config: PublishWorkerConfig }>('POST', '/publish/worker-configs', payload)
export const updatePublishWorkerConfig = (id: string, payload: PublishWorkerConfigInput) =>
  request<{ config: PublishWorkerConfig }>('PUT', `/publish/worker-configs/${encodeURIComponent(id)}`, payload)
export const deletePublishWorkerConfig = (id: string) =>
  request<{ deleted: boolean }>('DELETE', `/publish/worker-configs/${encodeURIComponent(id)}`)
export const checkPublishWorkerPermissions = (payload: { token: string; account_id: string; zone_id?: string }) =>
  request<{ ok: boolean; checks: PublishPermissionCheck[]; account_id: string }>('POST', '/publish/worker/check-permissions', payload)
export const createPublishWorkerNamespace = (payload: { token: string; account_id: string; worker_name: string }) =>
  request<{ namespace_id: string; reused: boolean; title: string }>('POST', '/publish/worker/create-namespace', payload)
export const deployPublishWorkerScript = (payload: {
  token: string
  account_id: string
  worker_name: string
  namespace_id: string
  access_token: string
}) => request<{ worker_dev_url: string; workers_subdomain?: string }>('POST', '/publish/worker/deploy-script', payload)
export const bindPublishWorkerDomain = (payload: {
  token: string
  account_id: string
  zone_id: string
  worker_name: string
  hostname: string
}) => request<{ hostname: string; worker_url: string }>('POST', '/publish/worker/bind-domain', payload)
export const verifyAndSavePublishWorker = (payload: {
  name: string
  worker_name: string
  worker_url: string
  worker_dev_url: string
  hostname: string
  account_id: string
  namespace_id: string
  zone_id: string
  access_token: string
}) => request<{ result: PublishWorkerVerifyResult; config?: PublishWorkerConfig }>('POST', '/publish/worker/verify-save', payload)
export const uploadPublishConfig = (payload: PublishUploadPayload) =>
  request<{ record: PublishRecord; file_name: string; version: number; access_url: string }>('POST', '/publish/upload', payload)
export const getPublishRecords = () => request<{ records: PublishRecord[] }>('GET', '/publish/records')
export const deletePublishRecord = (id: string) =>
  request<{ deleted: boolean; warning?: string }>('DELETE', `/publish/records/${encodeURIComponent(id)}`)

// ---- geodata management ----
export interface GeoDataFileStatus {
  name: string
  filename: string
  path: string
  exists: boolean
  size_bytes: number
  mod_time: string
}

export interface GeoDataFileResult {
  name: string
  status: 'ok' | 'error'
  size_bytes?: number
  message?: string
  error?: string
}

export interface GeoDataUpdateRecord {
  id: string
  started_at: string
  finished_at?: string
  status: 'running' | 'ok' | 'error'
  proxy_server: string
  files: GeoDataFileResult[]
  error?: string
}

export interface GeoDataStatus {
  files: GeoDataFileStatus[]
  latest: GeoDataUpdateRecord | null
  is_running: boolean
}

export interface GeoDataConfig {
  auto_geoip: boolean
  geoip_interval: string
  auto_geosite: boolean
  geosite_interval: string
  proxy_server: string
  geoip_url: string
  geosite_url: string
}

export interface GeoDataLogs {
  records: GeoDataUpdateRecord[]
  is_running: boolean
}

export const getGeoDataStatus  = () => request<GeoDataStatus>('GET', '/geodata/status')
export const getGeoDataConfig  = () => request<GeoDataConfig>('GET', '/geodata/config')
export const updateGeoDataConfig = (cfg: Partial<GeoDataConfig>) => request<{ updated: boolean }>('PUT', '/geodata/config', cfg)
export const triggerGeoDataUpdate = (proxy_server?: string) =>
  request<{ id: string; status: string }>('POST', '/geodata/update', { proxy_server: proxy_server ?? '' })
export const getGeoDataLogs    = () => request<GeoDataLogs>('GET', '/geodata/logs')
