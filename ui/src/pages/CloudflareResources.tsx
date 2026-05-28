import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  AlertCircle,
  CheckCircle2,
  CloudCog,
  Database,
  Loader2,
  RefreshCw,
  Trash2,
  Wrench,
} from 'lucide-react'
import {
  deleteCloudflareResources,
  listCloudflareResources,
  type CloudflareDeleteResultItem,
  type CloudflareNamespaceResource,
  type CloudflareResourceSummary,
  type CloudflareWorkerResource,
} from '../api/client'
import { CFGate, CFConfigBanner, CFConfigModal, useCFConfig } from '../components/CFConfig'
import { EmptyState, InlineNotice, PageHeader, SectionCard } from '../components/ui'

function asSet(items: string[]): Set<string> {
  return new Set(items.map((item) => item.trim()).filter(Boolean))
}

function mergeUnique(base: string[], extra: string[]): string[] {
  const out = new Set(base.map((item) => item.trim()).filter(Boolean))
  for (const item of extra) {
    const value = item.trim()
    if (!value) continue
    out.add(value)
  }
  return Array.from(out)
}

function fmtTime(raw?: string): string {
  if (!raw) return '—'
  const t = new Date(raw)
  if (Number.isNaN(t.getTime())) return raw
  return t.toLocaleString()
}

export function CloudflareResources() {
  const {
    config: cfConfig,
    loading: cfConfigLoading,
    save: saveCFConfig,
    reload: reloadCFConfig,
  } = useCFConfig()
  const [showCFModal, setShowCFModal] = useState(false)

  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [accountID, setAccountID] = useState('')
  const [workers, setWorkers] = useState<CloudflareWorkerResource[]>([])
  const [namespaces, setNamespaces] = useState<CloudflareNamespaceResource[]>([])
  const [summary, setSummary] = useState<CloudflareResourceSummary>({
    workers_total: 0,
    workers_managed: 0,
    namespaces_total: 0,
    namespaces_managed: 0,
  })

  const [workerFilter, setWorkerFilter] = useState('')
  const [nsFilter, setNSFilter] = useState('')
  const [onlyUnmanaged, setOnlyUnmanaged] = useState(true)
  const [onlyBound, setOnlyBound] = useState(false)
  const [linkSelections, setLinkSelections] = useState(true)

  const [selectedWorkers, setSelectedWorkers] = useState<string[]>([])
  const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>([])

  const [purgeLocalConfigs, setPurgeLocalConfigs] = useState(true)
  const [purgeLocalRuleSets, setPurgeLocalRuleSets] = useState(true)
  const [deleting, setDeleting] = useState(false)
  const [deleteResults, setDeleteResults] = useState<CloudflareDeleteResultItem[]>([])
  const [deleteWarnings, setDeleteWarnings] = useState<string[]>([])

  const loadResources = useCallback(async () => {
    const token = cfConfig?.cf_token?.trim() ?? ''
    const acc = cfConfig?.cf_account_id?.trim() ?? ''
    if (!token || !acc) {
      setAccountID('')
      setWorkers([])
      setNamespaces([])
      setSummary({
        workers_total: 0,
        workers_managed: 0,
        namespaces_total: 0,
        namespaces_managed: 0,
      })
      return
    }
    setLoading(true)
    setError('')
    try {
      const data = await listCloudflareResources({ token, account_id: acc })
      setAccountID(data.account_id)
      setWorkers(data.workers ?? [])
      setNamespaces(data.namespaces ?? [])
      setSummary(data.summary ?? {
        workers_total: 0,
        workers_managed: 0,
        namespaces_total: 0,
        namespaces_managed: 0,
      })
      setSelectedWorkers((prev) => prev.filter((name) => (data.workers ?? []).some((w) => w.name === name)))
      setSelectedNamespaces((prev) => prev.filter((id) => (data.namespaces ?? []).some((n) => n.id === id)))
    } catch (e) {
      setError(e instanceof Error ? e.message : '加载 Cloudflare 资源失败')
    } finally {
      setLoading(false)
    }
  }, [cfConfig?.cf_account_id, cfConfig?.cf_token])

  useEffect(() => {
    void loadResources()
  }, [loadResources])

  const filteredWorkers = useMemo(() => {
    const key = workerFilter.trim().toLowerCase()
    return workers.filter((item) => {
      if (onlyUnmanaged && item.managed) return false
      if (onlyBound && (item.kv_namespace_ids ?? []).length === 0) return false
      if (!key) return true
      return item.name.toLowerCase().includes(key)
    })
  }, [workers, workerFilter, onlyBound, onlyUnmanaged])

  const filteredNamespaces = useMemo(() => {
    const key = nsFilter.trim().toLowerCase()
    return namespaces.filter((item) => {
      if (onlyUnmanaged && item.managed) return false
      if (onlyBound && (item.bound_workers ?? []).length === 0) return false
      if (!key) return true
      return item.title.toLowerCase().includes(key) || item.id.toLowerCase().includes(key)
    })
  }, [namespaces, nsFilter, onlyBound, onlyUnmanaged])

  const workerKVMap = useMemo(() => {
    const map = new Map<string, string[]>()
    for (const item of workers) {
      map.set(item.name, item.kv_namespace_ids ?? [])
    }
    return map
  }, [workers])

  const namespaceWorkerMap = useMemo(() => {
    const map = new Map<string, string[]>()
    for (const item of namespaces) {
      map.set(item.id, item.bound_workers ?? [])
    }
    return map
  }, [namespaces])

  const selectedWorkerSet = useMemo(() => asSet(selectedWorkers), [selectedWorkers])
  const selectedNamespaceSet = useMemo(() => asSet(selectedNamespaces), [selectedNamespaces])

  const selectedTotal = selectedWorkers.length + selectedNamespaces.length
  const deleteFailed = deleteResults.filter((item) => !item.deleted)

  const toggleWorker = (name: string) => {
    const selecting = !selectedWorkerSet.has(name)
    setSelectedWorkers((prev) => {
      const next = new Set(prev)
      if (selecting) next.add(name)
      else next.delete(name)
      return Array.from(next)
    })
    if (selecting && linkSelections) {
      const linkedNamespaces = workerKVMap.get(name) ?? []
      if (linkedNamespaces.length > 0) {
        setSelectedNamespaces((prev) => mergeUnique(prev, linkedNamespaces))
      }
    }
  }

  const toggleNamespace = (id: string) => {
    const selecting = !selectedNamespaceSet.has(id)
    setSelectedNamespaces((prev) => {
      const next = new Set(prev)
      if (selecting) next.add(id)
      else next.delete(id)
      return Array.from(next)
    })
    if (selecting && linkSelections) {
      const linkedWorkers = namespaceWorkerMap.get(id) ?? []
      if (linkedWorkers.length > 0) {
        setSelectedWorkers((prev) => mergeUnique(prev, linkedWorkers))
      }
    }
  }

  const runDelete = useCallback(async () => {
    const token = cfConfig?.cf_token?.trim() ?? ''
    const acc = cfConfig?.cf_account_id?.trim() ?? ''
    if (!token || !acc) return
    if (selectedTotal === 0) return

    const confirmed = window.confirm(
      `将删除 ${selectedWorkers.length} 个 Worker 和 ${selectedNamespaces.length} 个 KV 命名空间。此操作不可恢复，是否继续？`,
    )
    if (!confirmed) return

    setDeleting(true)
    setDeleteResults([])
    setDeleteWarnings([])
    setError('')
    try {
      const data = await deleteCloudflareResources({
        token,
        account_id: acc,
        worker_names: selectedWorkers,
        namespace_ids: selectedNamespaces,
        purge_local_configs: purgeLocalConfigs,
        purge_local_rulesets: purgeLocalRuleSets,
      })
      setDeleteResults(data.results ?? [])
      setDeleteWarnings(data.warnings ?? [])
      setSelectedWorkers([])
      setSelectedNamespaces([])
      await loadResources()
    } catch (e) {
      setError(e instanceof Error ? e.message : '删除失败')
    } finally {
      setDeleting(false)
    }
  }, [
    cfConfig?.cf_account_id,
    cfConfig?.cf_token,
    loadResources,
    purgeLocalConfigs,
    purgeLocalRuleSets,
    selectedNamespaces,
    selectedTotal,
    selectedWorkers,
  ])

  return (
    <CFGate config={cfConfig} loading={cfConfigLoading} save={saveCFConfig}>
      <div className="space-y-6">
        <PageHeader
          eyebrow="Cloudflare Cleanup"
          title="Cloudflare Worker / KV 管理"
          description="开发阶段用于集中清理 Cloudflare 账号下的 Worker 脚本和 KV 命名空间，支持批量选择和一键删除。"
          metrics={[
            { label: '账户', value: accountID || (cfConfig?.cf_account_id || '—') },
            { label: 'Workers', value: String(summary.workers_total) },
            { label: 'KV', value: String(summary.namespaces_total) },
            { label: '绑定关系', value: String(summary.bindings_total ?? 0) },
            { label: '已选资源', value: String(selectedTotal) },
          ]}
          actions={(
            <button className="btn-ghost" onClick={() => void loadResources()} disabled={loading || deleting}>
              {loading ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
              刷新资源
            </button>
          )}
        />

        <CFConfigBanner config={cfConfig} loading={cfConfigLoading} onConfigure={() => setShowCFModal(true)} />

        {error && (
          <InlineNotice tone="danger" title="操作失败">
            {error}
          </InlineNotice>
        )}

        {deleteResults.length > 0 && (
          <InlineNotice tone={deleteFailed.length === 0 ? 'success' : 'warning'} title="删除结果">
            已处理 {deleteResults.length} 项，失败 {deleteFailed.length} 项。
          </InlineNotice>
        )}

        {deleteWarnings.length > 0 && (
          <InlineNotice tone="warning" title="本地清理提示">
            <div className="space-y-1">
              {deleteWarnings.map((w, idx) => (
                <p key={`${w}-${idx}`}>{w}</p>
              ))}
            </div>
          </InlineNotice>
        )}

        <SectionCard
          title="批量操作"
          description="默认仅显示未被 ClashForge 管理的资源，避免误删正在使用的发布 Worker。"
          actions={(
            <button
              className="btn-danger"
              onClick={() => void runDelete()}
              disabled={deleting || selectedTotal === 0}
            >
              {deleting ? <Loader2 size={14} className="animate-spin" /> : <Trash2 size={14} />}
              删除选中资源
            </button>
          )}
        >
          <div className="grid gap-3 md:grid-cols-2">
            <label className="block">
              <span className="text-xs text-muted">Worker 过滤</span>
              <input
                className="glass-input mt-1"
                value={workerFilter}
                onChange={(e) => setWorkerFilter(e.target.value)}
                placeholder="按名称过滤，如 cf-sub-"
              />
            </label>
            <label className="block">
              <span className="text-xs text-muted">KV 过滤</span>
              <input
                className="glass-input mt-1"
                value={nsFilter}
                onChange={(e) => setNSFilter(e.target.value)}
                placeholder="按 title / id 过滤，如 kv-"
              />
            </label>
          </div>
          <div className="mt-3 flex flex-wrap items-center gap-4 text-sm">
            <label className="inline-flex items-center gap-2">
              <input type="checkbox" checked={onlyUnmanaged} onChange={(e) => setOnlyUnmanaged(e.target.checked)} />
              <span>仅显示未托管资源</span>
            </label>
            <label className="inline-flex items-center gap-2">
              <input type="checkbox" checked={onlyBound} onChange={(e) => setOnlyBound(e.target.checked)} />
              <span>仅显示存在 KV 绑定关系</span>
            </label>
            <label className="inline-flex items-center gap-2">
              <input type="checkbox" checked={linkSelections} onChange={(e) => setLinkSelections(e.target.checked)} />
              <span>选择时联动绑定资源</span>
            </label>
            <label className="inline-flex items-center gap-2">
              <input type="checkbox" checked={purgeLocalConfigs} onChange={(e) => setPurgeLocalConfigs(e.target.checked)} />
              <span>同步清理本地 Worker 配置</span>
            </label>
            <label className="inline-flex items-center gap-2">
              <input type="checkbox" checked={purgeLocalRuleSets} onChange={(e) => setPurgeLocalRuleSets(e.target.checked)} />
              <span>同步清理本地规则集</span>
            </label>
          </div>
        </SectionCard>

        <SectionCard title="Worker 脚本" description={`当前匹配 ${filteredWorkers.length} 项`} className="overflow-hidden">
          {filteredWorkers.length === 0 ? (
            <EmptyState title="未找到 Worker 脚本" icon={<CloudCog size={18} />} />
          ) : (
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <button
                  className="btn-ghost h-8 px-3 text-xs"
                  onClick={() => {
                    const workerNames = filteredWorkers.map((item) => item.name)
                    setSelectedWorkers(workerNames)
                    if (linkSelections) {
                      const linkedNamespaces = filteredWorkers.flatMap((item) => item.kv_namespace_ids ?? [])
                      setSelectedNamespaces((prev) => mergeUnique(prev, linkedNamespaces))
                    }
                  }}
                >
                  全选当前 Worker
                </button>
                <button className="btn-ghost h-8 px-3 text-xs" onClick={() => setSelectedWorkers([])}>清空 Worker 选择</button>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="text-left text-white/50">
                    <tr className="border-b border-white/10">
                      <th className="px-2 py-2 w-10 whitespace-nowrap">选中</th>
                      <th className="px-2 py-2">名称</th>
                      <th className="px-2 py-2">KV 绑定</th>
                      <th className="px-2 py-2">状态</th>
                      <th className="px-2 py-2">更新时间</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredWorkers.map((item) => (
                      <tr key={item.name} className="border-b border-white/5">
                        <td className="px-2 py-2">
                          <input type="checkbox" checked={selectedWorkerSet.has(item.name)} onChange={() => toggleWorker(item.name)} />
                        </td>
                        <td className="px-2 py-2 font-mono text-xs">{item.name}</td>
                        <td className="px-2 py-2 text-xs text-white/70">
                          {(item.kv_namespace_ids ?? []).length > 0 ? (
                            <span className="font-mono" title={(item.kv_namespace_ids ?? []).join('\n')}>
                              {item.kv_namespace_ids?.length} 个
                            </span>
                          ) : (
                            <span className="text-white/35">—</span>
                          )}
                        </td>
                        <td className="px-2 py-2">
                          {item.managed ? (
                            <span className="inline-flex items-center gap-1 text-emerald-300"><CheckCircle2 size={12} />托管</span>
                          ) : (
                            <span className="inline-flex items-center gap-1 text-amber-300"><Wrench size={12} />未托管</span>
                          )}
                        </td>
                        <td className="px-2 py-2 text-white/55">{fmtTime(item.modified_on || item.created_on)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </SectionCard>

        <SectionCard title="KV 命名空间" description={`当前匹配 ${filteredNamespaces.length} 项`} className="overflow-hidden">
          {filteredNamespaces.length === 0 ? (
            <EmptyState title="未找到 KV 命名空间" icon={<Database size={18} />} />
          ) : (
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <button
                  className="btn-ghost h-8 px-3 text-xs"
                  onClick={() => {
                    const namespaceIDs = filteredNamespaces.map((item) => item.id)
                    setSelectedNamespaces(namespaceIDs)
                    if (linkSelections) {
                      const linkedWorkers = filteredNamespaces.flatMap((item) => item.bound_workers ?? [])
                      setSelectedWorkers((prev) => mergeUnique(prev, linkedWorkers))
                    }
                  }}
                >
                  全选当前 KV
                </button>
                <button className="btn-ghost h-8 px-3 text-xs" onClick={() => setSelectedNamespaces([])}>清空 KV 选择</button>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="text-left text-white/50">
                    <tr className="border-b border-white/10">
                      <th className="px-2 py-2 w-10 whitespace-nowrap">选中</th>
                      <th className="px-2 py-2">Title</th>
                      <th className="px-2 py-2">Namespace ID</th>
                      <th className="px-2 py-2">绑定 Worker</th>
                      <th className="px-2 py-2">状态</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredNamespaces.map((item) => (
                      <tr key={item.id} className="border-b border-white/5">
                        <td className="px-2 py-2">
                          <input type="checkbox" checked={selectedNamespaceSet.has(item.id)} onChange={() => toggleNamespace(item.id)} />
                        </td>
                        <td className="px-2 py-2">{item.title || '—'}</td>
                        <td className="px-2 py-2 font-mono text-xs text-white/60">{item.id}</td>
                        <td className="px-2 py-2 text-xs text-white/70">
                          {(item.bound_workers ?? []).length > 0 ? (
                            <span className="font-mono" title={(item.bound_workers ?? []).join('\n')}>
                              {item.bound_workers?.length} 个
                            </span>
                          ) : (
                            <span className="text-white/35">—</span>
                          )}
                        </td>
                        <td className="px-2 py-2">
                          {item.managed ? (
                            <span className="inline-flex items-center gap-1 text-emerald-300"><CheckCircle2 size={12} />托管</span>
                          ) : (
                            <span className="inline-flex items-center gap-1 text-amber-300"><AlertCircle size={12} />未托管</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </SectionCard>
      </div>

      {showCFModal && (
        <CFConfigModal
          initial={cfConfig}
          save={saveCFConfig}
          onClose={() => setShowCFModal(false)}
          onSaved={() => {
            setShowCFModal(false)
            void reloadCFConfig()
          }}
        />
      )}
    </CFGate>
  )
}
