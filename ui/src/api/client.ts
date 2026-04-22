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
  network: { mode: string; firewall_backend: string; rules_applied: boolean }
  subscriptions: { total: number; enabled: number; last_updated: string | null }
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
  filter?: { include?: string[]; exclude?: string[]; max_nodes?: number }
}

export interface Connection {
  id: string; upload: number; download: number; start: string
  chains: string[]
  metadata: { host: string; destinationPort: number; type: string; network: string; sourceIP: string }
}

export interface LogEntry { type: string; payload: string; time?: string }

// ---- API calls ----
export const getStatus        = () => request<StatusData>('GET', '/status')
export const startCore        = () => request('POST', '/core/start')
export const stopCore         = () => request('POST', '/core/stop')
export const restartCore      = () => request('POST', '/core/restart')
export const reloadCore       = () => request('POST', '/core/reload')
export const getCoreVersion   = () => request<{current:string;latest:string;has_update:boolean}>('GET', '/core/version')
export const getProxies       = () => request<ProxiesData>('GET', '/proxies')
export const selectProxy      = (group: string, name: string) => request('PUT', `/proxies/${encodeURIComponent(group)}/select`, { name })
export const testLatency      = (proxies: string[]) => request<Record<string,number>>('POST', '/proxies/test-latency', { proxies, url: 'http://www.gstatic.com/generate_204', timeout: 5000 })
export const getConnections   = () => request<{connections: Connection[]}>('GET', '/connections')
export const closeAllConns    = () => request('DELETE', '/connections')
export const getSubscriptions = () => request<{subscriptions: Subscription[]}>('GET', '/subscriptions')
export const addSubscription  = (s: Partial<Subscription>) => request<{id:string}>('POST', '/subscriptions', s)
export const updateSubscription = (id: string, p: Partial<Subscription>) => request('PUT', `/subscriptions/${id}`, p)
export const deleteSubscription = (id: string) => request('DELETE', `/subscriptions/${id}`)
export const triggerSubUpdate   = (id: string) => request('POST', `/subscriptions/${id}/update`)
export const triggerUpdateAll   = () => request('POST', '/subscriptions/update-all')
export const getConfig        = () => request<Record<string,unknown>>('GET', '/config')
export const updateConfig     = (p: Record<string,unknown>) => request('PUT', '/config', p)
export const getOverrides     = () => request<{content:string}>('GET', '/config/overrides')
export const updateOverrides  = (content: string) => request('PUT', '/config/overrides', { content })
