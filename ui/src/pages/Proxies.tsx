import { useEffect, useState } from 'react'
import { getProxies, selectProxy, testLatency } from '../api/client'
import type { ProxyNode } from '../api/client'
import { latencyColor, latencyBarColor, latencyBarWidth } from '../utils/format'
import { RefreshCw, Zap } from 'lucide-react'

type ProxyMap = Record<string, ProxyNode>

const TYPE_ORDER = ['Selector', 'Fallback', 'URLTest', 'LoadBalance']
const IGNORED = ['DIRECT', 'REJECT', 'GLOBAL', 'PASS', 'REJECT-DROP', 'Compatible']

function LatencyBar({ ms }: { ms: number }) {
  if (!ms || ms <= 0) return <span className="text-muted text-xs">—</span>
  return (
    <div className="flex items-center gap-2">
      <div className="w-16 h-1.5 bg-surface-3 rounded-full overflow-hidden">
        <div className={`h-full rounded-full ${latencyBarColor(ms)}`} style={{ width: latencyBarWidth(ms) }} />
      </div>
      <span className={`text-xs tabular-nums font-mono ${latencyColor(ms)}`}>{ms}ms</span>
    </div>
  )
}

function ProxyRow({ node, isSelected, onClick }: { node: ProxyNode; isSelected: boolean; onClick: () => void }) {
  const lastDelay = node.history?.at(-1)?.delay ?? -1
  return (
    <button
      onClick={onClick}
      className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl transition-all text-left ${
        isSelected
          ? 'bg-brand/15 ring-1 ring-brand/40'
          : 'hover:bg-white/5'
      }`}
    >
      <div className={`w-2 h-2 rounded-full flex-shrink-0 ${isSelected ? 'bg-brand' : 'bg-surface-3'}`} />
      <div className="flex-1 min-w-0">
        <p className={`text-sm font-medium truncate ${isSelected ? 'text-brand' : 'text-slate-200'}`}>
          {node.name}
        </p>
        <p className="text-xs text-muted">{node.type}</p>
      </div>
      <LatencyBar ms={lastDelay} />
    </button>
  )
}

function ProxyGroup({ name, group, allProxies, onSelect }: {
  name: string; group: ProxyNode; allProxies: ProxyMap; onSelect: (group: string, proxy: string) => void
}) {
  const [open, setOpen] = useState(true)
  const members = (group.all ?? []).filter(n => !IGNORED.includes(n))

  return (
    <div className="card overflow-hidden">
      <button
        onClick={() => setOpen(o => !o)}
        className="w-full flex items-center justify-between px-5 py-4 hover:bg-white/5 transition-all"
      >
        <div className="flex items-center gap-3">
          <span className="text-sm font-semibold text-white">{name}</span>
          <span className="badge badge-muted">{group.type}</span>
          {group.now && (
            <span className="text-xs text-muted">
              → <span className="text-slate-300 font-medium">{group.now}</span>
            </span>
          )}
        </div>
        <span className="text-muted text-xs">{open ? '▲' : '▼'}</span>
      </button>
      {open && (
        <div className="px-3 pb-3 space-y-0.5 max-h-80 overflow-y-auto">
          {members.map(m => {
            const node = allProxies[m]
            if (!node) return (
              <div key={m} className="px-4 py-2.5 text-sm text-muted">{m}</div>
            )
            return (
              <ProxyRow
                key={m}
                node={node}
                isSelected={group.now === m}
                onClick={() => {
                  if (group.type === 'Selector') onSelect(name, m)
                }}
              />
            )
          })}
          {group.type === 'Selector' && (
            <>
              {['DIRECT', 'REJECT'].filter(n => group.all?.includes(n)).map(n => (
                <button
                  key={n}
                  onClick={() => onSelect(name, n)}
                  className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl transition-all text-left ${
                    group.now === n ? 'bg-brand/15 ring-1 ring-brand/40' : 'hover:bg-white/5'
                  }`}
                >
                  <div className={`w-2 h-2 rounded-full flex-shrink-0 ${group.now === n ? 'bg-brand' : 'bg-surface-3'}`} />
                  <span className={`text-sm font-medium ${group.now === n ? 'text-brand' : 'text-slate-200'}`}>{n}</span>
                </button>
              ))}
            </>
          )}
        </div>
      )}
    </div>
  )
}

export function Proxies() {
  const [proxies, setProxies] = useState<ProxyMap>({})
  const [testing, setTesting] = useState(false)
  const [loading, setLoading] = useState(true)

  const refresh = () => getProxies().then(d => { setProxies(d.proxies ?? {}); setLoading(false) }).catch(() => setLoading(false))
  useEffect(() => { refresh() }, [])

  const groups = Object.entries(proxies)
    .filter(([, v]) => TYPE_ORDER.includes(v.type) && !IGNORED.includes(v.name) && v.name !== 'GLOBAL')
    .sort(([,a],[,b]) => TYPE_ORDER.indexOf(a.type) - TYPE_ORDER.indexOf(b.type))

  const handleSelect = async (group: string, proxy: string) => {
    await selectProxy(group, proxy).catch(() => null)
    refresh()
  }

  const handleTestAll = async () => {
    setTesting(true)
    const nodeNames = Object.entries(proxies)
      .filter(([, v]) => !TYPE_ORDER.includes(v.type) && !IGNORED.includes(v.name))
      .map(([k]) => k)
    await testLatency(nodeNames).catch(() => null)
    refresh()
    setTesting(false)
  }

  return (
    <div className="p-6 space-y-5 max-w-4xl mx-auto">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold text-white">节点</h1>
        <div className="flex gap-2">
          <button className="btn-ghost flex items-center gap-2" onClick={refresh}>
            <RefreshCw size={14}/> 刷新
          </button>
          <button className="btn-primary flex items-center gap-2" onClick={handleTestAll} disabled={testing}>
            <Zap size={14} className={testing ? 'animate-pulse' : ''}/> 测试延迟
          </button>
        </div>
      </div>

      {loading && (
        <div className="card px-5 py-8 text-center text-muted text-sm">加载中…</div>
      )}

      {!loading && groups.length === 0 && (
        <div className="card px-5 py-8 text-center text-muted text-sm">
          未找到代理组。请先添加订阅并更新节点。
        </div>
      )}

      {groups.map(([name, group]) => (
        <ProxyGroup
          key={name}
          name={name}
          group={group}
          allProxies={proxies}
          onSelect={handleSelect}
        />
      ))}
    </div>
  )
}
