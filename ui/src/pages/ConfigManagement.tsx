import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  getSubscriptions,
  addSubscription,
  deleteSubscription,
  deleteSourceFile,
  triggerSubUpdate,
  triggerUpdateAll,
  getMihomoConfig,
  getRuleProviders,
  syncRuleProvider,
  syncAllRuleProviders,
  searchRules,
  getSources,
  getOverviewCore,
  stopCore,
} from '../api/client'
import type { Subscription, RuleProvider, RuleSearchResult, SourceFile, ActiveSource } from '../api/client'
import {
  FolderCog, List, RefreshCw, Plus, Trash2, MoreVertical, Zap, Eye,
  Shield, Search, ChevronDown, ChevronRight, Play, FileText, Database,
  Radio, AlertCircle, X,
} from 'lucide-react'

// ── Types ─────────────────────────────────────────────────────────────────────

type PendingActivate =
  | { kind: 'file'; filename: string; displayName: string }
  | { kind: 'sub'; id: string; name: string; url?: string }

type ConfirmDelete =
  | { kind: 'file'; id: string; name: string }
  | { kind: 'sub'; id: string; name: string }

// ── Stop & Switch Dialog ──────────────────────────────────────────────────────

function StopAndSwitchDialog({ target, onCancel, onStopped }: {
  target: PendingActivate
  onCancel: () => void
  onStopped: () => void
}) {
  const [stopping, setStopping] = useState(false)
  const [error, setError] = useState('')

  const handle = async () => {
    setStopping(true)
    setError('')
    try {
      await stopCore()
      for (let i = 0; i < 20; i++) {
        await new Promise(r => setTimeout(r, 500))
        const d = await getOverviewCore().catch(() => null)
        if (!d || d.core.state !== 'running') { onStopped(); return }
      }
      setError('等待服务停止超时，请手动确认后重试')
    } catch { setError('停止服务失败，请重试') }
    finally { setStopping(false) }
  }

  const name = target.kind === 'file' ? target.displayName : target.name

  return (
    <div
      className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 p-4 animate-fade-in"
      onClick={!stopping ? onCancel : undefined}
    >
      <div
        className="bg-surface-1 rounded-2xl border border-warning/20 w-full max-w-sm p-6 space-y-4 animate-slide-in"
        style={{ boxShadow: '0 0 40px rgba(245,158,11,0.08)' }}
        onClick={e => e.stopPropagation()}
      >
        <div className="flex items-start gap-3">
          <div className="w-10 h-10 rounded-xl bg-warning/15 border border-warning/20 flex items-center justify-center flex-shrink-0">
            <AlertCircle size={18} className="text-warning" />
          </div>
          <div className="flex-1">
            <h3 className="text-base font-semibold text-white">停止当前服务</h3>
            <p className="text-sm text-muted mt-1.5">
              切换到「<span className="text-white font-medium">{name}</span>」需要先停止正在运行的服务，确认停止？
            </p>
          </div>
          {!stopping && (
            <button className="btn-icon" onClick={onCancel}>
              <X size={14} />
            </button>
          )}
        </div>
        {error && (
          <p className="text-xs text-danger bg-danger/10 border border-danger/20 rounded-xl px-3 py-2">{error}</p>
        )}
        <div className="flex gap-3 pt-1">
          <button className="btn-ghost flex-1" onClick={onCancel} disabled={stopping}>取消</button>
          <button
            className="btn-warning flex-1 flex items-center justify-center gap-2"
            onClick={handle}
            disabled={stopping}
          >
            {stopping
              ? <><RefreshCw size={12} className="animate-spin" /> 停止中…</>
              : '停止并切换'
            }
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Delete Confirm Dialog ─────────────────────────────────────────────────────

function DeleteConfirmDialog({ target, onCancel, onConfirm, deleting }: {
  target: ConfirmDelete
  onCancel: () => void
  onConfirm: () => void
  deleting: boolean
}) {
  return (
    <div
      className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 p-4 animate-fade-in"
      onClick={!deleting ? onCancel : undefined}
    >
      <div
        className="bg-surface-1 rounded-2xl border border-danger/20 w-full max-w-sm p-6 space-y-4 animate-slide-in"
        style={{ boxShadow: '0 0 40px rgba(244,63,94,0.08)' }}
        onClick={e => e.stopPropagation()}
      >
        <div className="flex items-start gap-3">
          <div className="w-10 h-10 rounded-xl bg-danger/15 border border-danger/20 flex items-center justify-center flex-shrink-0">
            <Trash2 size={16} className="text-danger" />
          </div>
          <div>
            <h3 className="text-base font-semibold text-white">删除配置</h3>
            <p className="text-sm text-muted mt-1.5">
              确认删除「<span className="text-white font-medium">{target.name}</span>」？此操作不可撤销。
            </p>
          </div>
        </div>
        <div className="flex gap-3 pt-1">
          <button className="btn-ghost flex-1" onClick={onCancel} disabled={deleting}>取消</button>
          <button className="btn-danger flex-1" onClick={onConfirm} disabled={deleting}>
            {deleting ? '删除中…' : '确认删除'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Source Files Panel ────────────────────────────────────────────────────────────

function sourceTypeLabel(filename: string) {
  if (/^\d{8}_v\d+\.yaml$/.test(filename)) return '粘贴'
  return '上传'
}

function SourceFilesPanel({ coreRunning }: { coreRunning: boolean }) {
  const navigate = useNavigate()
  const [files, setFiles] = useState<SourceFile[]>([])
  const [subs, setSubs] = useState<Subscription[]>([])
  const [activeSource, setActiveSourceState] = useState<ActiveSource | null>(null)
  const [loading, setLoading] = useState(true)
  const [updatingSub, setUpdatingSub] = useState<string | null>(null)
  const [pendingActivate, setPendingActivate] = useState<PendingActivate | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<ConfirmDelete | null>(null)
  const [deleting, setDeleting] = useState(false)

  const refresh = async () => {
    setLoading(true)
    const [sourcesData, subsData] = await Promise.all([
      getSources().catch(() => ({ files: [] as SourceFile[], active_source: null as ActiveSource | null })),
      getSubscriptions().catch(() => ({ subscriptions: [] as Subscription[] })),
    ])
    setFiles(sourcesData.files ?? [])
    setActiveSourceState(sourcesData.active_source)
    setSubs(subsData.subscriptions ?? [])
    setLoading(false)
  }

  useEffect(() => { refresh() }, [])

  const handleUpdateSub = async (id: string) => {
    setUpdatingSub(id)
    await triggerSubUpdate(id).catch(() => null)
    setUpdatingSub(null)
  }

  const isFileActive = (filename: string) =>
    activeSource?.type === 'file' && activeSource.filename === filename
  const isSubActive = (id: string) =>
    activeSource?.type === 'subscription' && activeSource.sub_id === id

  const doActivate = (item: PendingActivate) => {
    if (item.kind === 'file') {
      navigate('/setup', { state: { activateFile: { filename: item.filename } } })
    } else {
      navigate('/setup', { state: { activateSub: { id: item.id, name: item.name, url: item.url } } })
    }
  }

  const handleActivate = (item: PendingActivate) => {
    if (coreRunning) {
      setPendingActivate(item)
    } else {
      doActivate(item)
    }
  }

  const doDelete = async () => {
    if (!confirmDelete) return
    setDeleting(true)
    if (confirmDelete.kind === 'file') {
      await deleteSourceFile(confirmDelete.id).catch(() => null)
    } else {
      await deleteSubscription(confirmDelete.id).catch(() => null)
    }
    setDeleting(false)
    setConfirmDelete(null)
    refresh()
  }

  const isEmpty = !loading && files.length === 0 && subs.length === 0

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <p className="text-xs text-muted leading-5">
          所有导入的配置来源。点击「启动」通过配置向导重新应用该配置。
          {coreRunning && activeSource && (
            <span className="ml-1 text-success font-medium">
              当前运行: {activeSource.type === 'file' ? activeSource.filename : activeSource.sub_name || activeSource.sub_id}
            </span>
          )}
        </p>
        <button className="btn-ghost flex items-center gap-2 text-xs" onClick={refresh} disabled={loading}>
          <RefreshCw size={13} className={loading ? 'animate-spin' : ''} /> 刷新
        </button>
      </div>

      {loading && (
        <div className="card px-5 py-10 text-center text-muted text-sm flex items-center justify-center gap-2">
          <RefreshCw size={14} className="animate-spin" /> 加载中…
        </div>
      )}

      {isEmpty && (
        <div className="card px-5 py-10 text-center space-y-2">
          <p className="text-muted text-sm">暂无配置文件记录</p>
          <p className="text-xs text-muted/60">通过「配置向导」导入配置后，记录将显示在这里。</p>
        </div>
      )}

      {files.length > 0 && (
        <div className="space-y-2">
          <p className="section-label px-1">保存的配置文件</p>
          {files.map(f => {
            const active = isFileActive(f.filename)
            const running = active && coreRunning
            return (
              <div
                key={f.filename}
                className={`card px-5 py-4 flex items-center gap-4 transition-all ${
                  active ? 'border-brand/25 bg-brand/[0.04]' : 'hover:border-white/10'
                }`}
              >
                <FileText size={15} className={active ? 'text-brand flex-shrink-0' : 'text-muted flex-shrink-0'} />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <p className={`font-mono text-sm font-semibold ${active ? 'text-brand' : 'text-white'}`}>{f.filename}</p>
                    <span className="badge-muted">{sourceTypeLabel(f.filename)}</span>
                    {running && <span className="badge-success">运行中</span>}
                    {active && !coreRunning && <span className="badge-muted">上次使用</span>}
                  </div>
                  <p className="text-xs text-muted mt-0.5">
                    {new Date(f.created_at).toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}
                    {' · '}{(f.size_bytes / 1024).toFixed(1)} KB
                  </p>
                </div>
                <div className="flex items-center gap-2 flex-shrink-0">
                  {!running && (
                    <button
                      className="btn-ghost flex items-center gap-1.5 text-xs py-1.5 px-3 text-brand hover:bg-brand/10 border-brand/20"
                      onClick={() => handleActivate({ kind: 'file', filename: f.filename, displayName: f.filename })}
                    >
                      <Play size={12} /> 启动
                    </button>
                  )}
                  <button
                    className={`btn-icon ${running ? 'opacity-30 cursor-not-allowed' : 'hover:text-danger hover:bg-danger/10'}`}
                    title={running ? '正在运行中，无法删除' : '删除'}
                    disabled={running}
                    onClick={() => !running && setConfirmDelete({ kind: 'file', id: f.filename, name: f.filename })}
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            )
          })}
        </div>
      )}

      {subs.length > 0 && (
        <div className="space-y-2">
          <p className="section-label px-1">订阅配置</p>
          {subs.map(sub => {
            const active = isSubActive(sub.id)
            const running = active && coreRunning
            const lastUpdated = sub.last_updated
              ? new Date(sub.last_updated).toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
              : '从未更新'
            return (
              <div
                key={sub.id}
                className={`card px-5 py-4 flex items-center gap-4 transition-all ${
                  active ? 'border-brand/25 bg-brand/[0.04]' : 'hover:border-white/10'
                }`}
              >
                <Radio size={15} className={active ? 'text-brand flex-shrink-0' : 'text-muted flex-shrink-0'} />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <p className={`text-sm font-semibold ${active ? 'text-brand' : 'text-white'}`}>{sub.name}</p>
                    <span className="badge-muted">订阅</span>
                    {running && <span className="badge-success">运行中</span>}
                    {active && !coreRunning && <span className="badge-muted">上次使用</span>}
                  </div>
                  <p className="text-xs text-muted mt-0.5">
                    {sub.node_count ? `${sub.node_count} 节点` : '—'}{' · 上次更新: '}{lastUpdated}
                  </p>
                  {sub.url && <p className="text-xs text-muted truncate mt-0.5">{sub.url}</p>}
                </div>
                <div className="flex items-center gap-2 flex-shrink-0">
                  <button
                    className="btn-ghost flex items-center gap-1.5 text-xs py-1.5 px-3"
                    onClick={() => handleUpdateSub(sub.id)}
                    disabled={updatingSub === sub.id}
                  >
                    <RefreshCw size={12} className={updatingSub === sub.id ? 'animate-spin' : ''} /> 更新
                  </button>
                  {!running && (
                    <button
                      className="btn-ghost flex items-center gap-1.5 text-xs py-1.5 px-3 text-brand hover:bg-brand/10 border-brand/20"
                      onClick={() => handleActivate({ kind: 'sub', id: sub.id, name: sub.name, url: sub.url })}
                    >
                      <Play size={12} /> 启动
                    </button>
                  )}
                  <button
                    className={`btn-icon ${running ? 'opacity-30 cursor-not-allowed' : 'hover:text-danger hover:bg-danger/10'}`}
                    title={running ? '正在运行中，无法删除' : '删除'}
                    disabled={running}
                    onClick={() => !running && setConfirmDelete({ kind: 'sub', id: sub.id, name: sub.name })}
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            )
          })}
        </div>
      )}

      {pendingActivate && (
        <StopAndSwitchDialog
          target={pendingActivate}
          onCancel={() => setPendingActivate(null)}
          onStopped={() => { setPendingActivate(null); doActivate(pendingActivate) }}
        />
      )}

      {confirmDelete && (
        <DeleteConfirmDialog
          target={confirmDelete}
          onCancel={() => setConfirmDelete(null)}
          onConfirm={doDelete}
          deleting={deleting}
        />
      )}
    </div>
  )
}

// ── Subscriptions sub-section ────────────────────────────────────────────────

function SubCard({ sub, isRunning, onDelete, onUpdate, onActivate }: {
  sub: Subscription
  isRunning: boolean
  onDelete: () => void
  onUpdate: () => void
  onActivate: () => void
}) {
  const [menu, setMenu] = useState(false)
  const lastUpdated = sub.last_updated
    ? new Date(sub.last_updated).toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
    : '从未更新'

  return (
    <div className={`card px-5 py-4 flex items-start gap-4 transition-all ${isRunning ? 'border-brand/25 bg-brand/[0.04]' : 'hover:border-white/10'}`}>
      <Radio size={15} className={`${isRunning ? 'text-brand' : 'text-muted'} flex-shrink-0 mt-0.5`} />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <p className={`font-semibold text-sm ${isRunning ? 'text-brand' : 'text-white'}`}>{sub.name}</p>
          {isRunning ? <span className="badge-success">运行中</span> : <span className="badge-muted">闲置</span>}
        </div>
        <p className="text-xs text-muted mt-1">
          {sub.node_count ? `${sub.node_count} 节点` : '—'} · 上次更新: {lastUpdated}
        </p>
        {sub.url && <p className="text-xs text-muted truncate mt-0.5">{sub.url}</p>}
      </div>
      <div className="relative flex-shrink-0">
        <div className="flex gap-2 items-center">
          <button className="btn-ghost flex items-center gap-1.5 text-xs py-1.5 px-3" onClick={onUpdate}>
            <RefreshCw size={12} /> 更新
          </button>
          <button className="btn-icon" onClick={() => setMenu(m => !m)}>
            <MoreVertical size={15} />
          </button>
        </div>
        {menu && (
          <div className="absolute right-0 top-10 z-10 bg-surface-2 border border-white/[0.08] rounded-2xl overflow-hidden w-44 animate-slide-in"
            style={{ boxShadow: '0 8px 32px rgba(0,0,0,0.5)' }}>
            {!isRunning && (
              <button
                className="w-full flex items-center gap-2.5 px-4 py-2.5 text-sm text-brand hover:bg-brand/10 transition-colors cursor-pointer"
                onClick={() => { setMenu(false); onActivate() }}
              >
                <Play size={13} /> 切换到此配置
              </button>
            )}
            <button
              className={`w-full flex items-center gap-2.5 px-4 py-2.5 text-sm transition-colors cursor-pointer ${isRunning ? 'text-muted cursor-not-allowed opacity-40' : 'text-danger hover:bg-danger/10'}`}
              title={isRunning ? '正在运行中，无法删除' : undefined}
              disabled={isRunning}
              onClick={() => { if (!isRunning) { setMenu(false); onDelete() } }}
            >
              <Trash2 size={13} /> 删除
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

function AddModal({ onClose, onAdded }: { onClose: () => void; onAdded: () => void }) {
  const [form, setForm] = useState({ name: '', url: '', user_agent: 'clash-meta', interval: '6h', enabled: true })
  const [saving, setSaving] = useState(false)

  const save = async () => {
    if (!form.name || !form.url) return
    setSaving(true)
    await addSubscription({ ...form, type: 'url' }).catch(() => null)
    setSaving(false)
    onAdded()
    onClose()
  }

  return (
    <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 p-4 animate-fade-in" onClick={onClose}>
      <div
        className="bg-surface-1 rounded-2xl border border-white/[0.08] w-full max-w-md p-6 space-y-4 animate-slide-in"
        style={{ boxShadow: '0 0 40px rgba(6,182,212,0.06)' }}
        onClick={e => e.stopPropagation()}
      >
        <div className="flex items-center gap-3">
          <div className="w-9 h-9 rounded-xl bg-brand/15 border border-brand/20 flex items-center justify-center flex-shrink-0">
            <Plus size={16} className="text-brand" />
          </div>
          <h2 className="text-base font-semibold text-white">添加订阅</h2>
        </div>
        <div className="space-y-3">
          {[
            { key: 'name', label: '名称', placeholder: '我的机场' },
            { key: 'url', label: '订阅 URL', placeholder: 'https://...' },
            { key: 'user_agent', label: 'User-Agent', placeholder: 'clash-meta' },
            { key: 'interval', label: '更新间隔', placeholder: '6h' },
          ].map(({ key, label, placeholder }) => (
            <div key={key}>
              <label className="section-label block mb-1.5">{label}</label>
              <input
                className="input"
                placeholder={placeholder}
                value={(form as unknown as Record<string, string>)[key] ?? ''}
                onChange={e => setForm(f => ({ ...f, [key]: e.target.value }))}
              />
            </div>
          ))}
        </div>
        <div className="flex gap-3 pt-1">
          <button className="btn-ghost flex-1" onClick={onClose}>取消</button>
          <button className="btn-primary flex-1 flex items-center justify-center gap-2" onClick={save} disabled={saving}>
            {saving ? <><RefreshCw size={13} className="animate-spin" />保存中…</> : <><Plus size={13} />添加</>}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Subscriptions sub-section ─────────────────────────────────────────────────

function SubscriptionsPanel({ coreRunning, activeSource }: { coreRunning: boolean; activeSource: ActiveSource | null }) {
  const navigate = useNavigate()
  const [subs, setSubs] = useState<Subscription[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [loading, setLoading] = useState(true)
  const [pendingActivate, setPendingActivate] = useState<PendingActivate | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<ConfirmDelete | null>(null)
  const [deleting, setDeleting] = useState(false)

  const refresh = () => getSubscriptions()
    .then(d => { setSubs(d.subscriptions ?? []); setLoading(false) })
    .catch(() => setLoading(false))

  useEffect(() => { refresh() }, [])

  const isSubRunning = (id: string) =>
    activeSource?.type === 'subscription' && activeSource.sub_id === id && coreRunning

  const handleDelete = async () => {
    if (!confirmDelete) return
    setDeleting(true)
    await deleteSubscription(confirmDelete.id).catch(() => null)
    setDeleting(false)
    setConfirmDelete(null)
    refresh()
  }

  const handleUpdate = async (id: string) => {
    await triggerSubUpdate(id).catch(() => null)
  }

  const doActivate = (sub: Subscription) => {
    navigate('/setup', { state: { activateSub: { id: sub.id, name: sub.name, url: sub.url } } })
  }

  const handleActivate = (sub: Subscription) => {
    if (coreRunning) {
      setPendingActivate({ kind: 'sub', id: sub.id, name: sub.name, url: sub.url })
    } else {
      doActivate(sub)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <p className="text-sm text-muted">管理节点订阅源，支持 Clash / SS / Trojan / VLESS / VMess 格式。</p>
        <div className="flex gap-2">
          <button className="btn-ghost flex items-center gap-2" onClick={() => triggerUpdateAll().then(refresh)}>
            <RefreshCw size={14} /> 全部更新
          </button>
          <button className="btn-primary flex items-center gap-2" onClick={() => setShowAdd(true)}>
            <Plus size={14} /> 添加订阅
          </button>
        </div>
      </div>

      {loading && (
        <div className="card px-5 py-8 text-center text-muted text-sm">加载中…</div>
      )}
      {!loading && subs.length === 0 && (
        <div className="card px-5 py-8 text-center text-muted text-sm">
          还没有订阅。点击「添加订阅」开始配置。
        </div>
      )}
      {subs.map(sub => (
        <SubCard
          key={sub.id}
          sub={sub}
          isRunning={isSubRunning(sub.id)}
          onDelete={() => setConfirmDelete({ kind: 'sub', id: sub.id, name: sub.name })}
          onUpdate={() => handleUpdate(sub.id)}
          onActivate={() => handleActivate(sub)}
        />
      ))}

      {showAdd && <AddModal onClose={() => setShowAdd(false)} onAdded={refresh} />}

      {pendingActivate && (
        <StopAndSwitchDialog
          target={pendingActivate}
          onCancel={() => setPendingActivate(null)}
          onStopped={() => {
            const item = pendingActivate
            setPendingActivate(null)
            const sub = subs.find(s => s.id === (item as { id: string }).id)
            if (sub) doActivate(sub)
          }}
        />
      )}

      {confirmDelete && (
        <DeleteConfirmDialog
          target={confirmDelete}
          onCancel={() => setConfirmDelete(null)}
          onConfirm={handleDelete}
          deleting={deleting}
        />
      )}
    </div>
  )
}

// ── Running config view ───────────────────────────────────────────────────────

function RunningConfigPanel({ activeSource }: { activeSource: ActiveSource | null }) {
  const [content, setContent] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const load = () => {
    setLoading(true)
    getMihomoConfig()
      .then(d => setContent(d.content))
      .catch(() => setContent(''))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  return (
    <div className="space-y-4">
      {activeSource && (
        <div className="card px-4 py-3 flex items-start gap-3 border-success/20 bg-success/[0.04]">
          <Database size={15} className="text-success flex-shrink-0 mt-0.5" />
          <div className="min-w-0">
            <p className="text-xs font-semibold text-success mb-0.5">当前运行配置来源</p>
            <p className="text-sm text-white font-mono truncate">
              {activeSource.type === 'file' ? activeSource.filename : activeSource.sub_name || activeSource.sub_id}
            </p>
            <p className="text-xs text-muted mt-0.5">{activeSource.type === 'file' ? '保存的配置文件' : '订阅配置'}</p>
          </div>
        </div>
      )}
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <p className="text-sm text-muted">ClashForge 生成的运行中配置（只读）。</p>
        <button className="btn-ghost flex items-center gap-2 text-xs" onClick={load} disabled={loading}>
          <RefreshCw size={13} className={loading ? 'animate-spin' : ''} /> 刷新
        </button>
      </div>
      {loading ? (
        <div className="card px-4 py-8 text-sm text-muted text-center flex items-center justify-center gap-2">
          <RefreshCw size={13} className="animate-spin" /> 加载中…
        </div>
      ) : content === '' ? (
        <div className="card px-4 py-8 text-sm text-muted text-center">
          配置文件不存在（核心尚未启动或尚未生成配置）
        </div>
      ) : (
        <textarea
          className="w-full h-[32rem] bg-surface-2/60 border border-white/[0.08] rounded-2xl px-4 py-3 text-sm font-mono text-slate-300 outline-none resize-none focus:border-brand/40 transition-colors"
          value={content ?? ''}
          readOnly
          spellCheck={false}
        />
      )}
    </div>
  )
}

// ── Rule providers sub-section ────────────────────────────────────────────────

function behaviorLabel(b: string) {
  if (b === 'domain') return '域名'
  if (b === 'ipcidr') return 'IP 段'
  if (b === 'classical') return '混合'
  return b
}

function vehicleTag(v: string) {
  const cls = v?.toUpperCase() === 'HTTP'
    ? 'bg-brand/10 border-brand/25 text-brand'
    : 'bg-white/[0.05] border-white/[0.1] text-muted'
  return (
    <span className={`inline-flex rounded-full border px-2 py-0.5 text-[10px] font-semibold ${cls}`}>{v}</span>
  )
}

function ProviderRow({ p, onSync, syncing }: { p: RuleProvider; onSync: () => void; syncing: boolean }) {
  const [expanded, setExpanded] = useState(false)
  const updatedAt = p.updatedAt
    ? new Date(p.updatedAt).toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
    : '—'
  return (
    <div className={`card overflow-hidden transition-all ${expanded ? 'border-white/10' : 'hover:border-white/10'}`}>
      <button
        className="w-full flex items-center gap-3 px-4 py-3 hover:bg-white/[0.02] transition-colors text-left cursor-pointer"
        onClick={() => setExpanded(e => !e)}
      >
        {expanded
          ? <ChevronDown size={13} className="text-muted flex-shrink-0" />
          : <ChevronRight size={13} className="text-muted flex-shrink-0" />}
        <span className="text-sm font-semibold text-slate-100 flex-1 min-w-0 truncate">{p.name}</span>
        <div className="flex items-center gap-2 flex-shrink-0">
          {vehicleTag(p.vehicleType)}
          <span className="badge-muted hidden sm:inline">{behaviorLabel(p.behavior)}</span>
          <span className="text-xs text-slate-300 tabular-nums font-mono">{(p.ruleCount ?? 0).toLocaleString()}<span className="text-muted text-[10px] ml-0.5">条</span></span>
          <span className="text-[10px] text-muted hidden sm:inline">{p.size_mb > 0 ? `${p.size_mb.toFixed(1)} MB` : '—'}</span>
          <span className="text-[10px] text-muted hidden md:inline">{updatedAt}</span>
        </div>
        <button
          className="btn-ghost py-1 px-2.5 text-xs ml-2 flex items-center gap-1.5 flex-shrink-0"
          onClick={e => { e.stopPropagation(); onSync() }}
          disabled={syncing}
        >
          <RefreshCw size={11} className={syncing ? 'animate-spin' : ''} />
          同步
        </button>
      </button>
      {expanded && (
        <div className="border-t border-white/[0.06] px-4 py-3 bg-black/10">
          <p className="text-[10px] text-muted font-mono break-all">{p.file_path || '路径未知'}</p>
        </div>
      )}
    </div>
  )
}

function RulesPanel() {
  const [providers, setProviders] = useState<RuleProvider[]>([])
  const [loading, setLoading] = useState(true)
  const [syncingAll, setSyncingAll] = useState(false)
  const [syncingNames, setSyncingNames] = useState<Set<string>>(new Set())
  const [syncStatus, setSyncStatus] = useState('')

  // search state
  const [query, setQuery] = useState('')
  const [searching, setSearching] = useState(false)
  const [searchResults, setSearchResults] = useState<RuleSearchResult[] | null>(null)
  const searchDebounce = useRef<ReturnType<typeof setTimeout> | null>(null)

  const load = () => {
    setLoading(true)
    getRuleProviders()
      .then(d => setProviders(d.providers ?? []))
      .catch(() => null)
      .finally(() => setLoading(false))
  }
  useEffect(() => { load() }, [])

  const handleSyncOne = async (name: string) => {
    setSyncingNames(s => new Set(s).add(name))
    setSyncStatus('')
    await syncRuleProvider(name).catch(() => null)
    setSyncingNames(s => { const n = new Set(s); n.delete(name); return n })
    load()
    setSyncStatus(`${name} 已同步`)
    setTimeout(() => setSyncStatus(''), 3000)
  }

  const handleSyncAll = async () => {
    setSyncingAll(true)
    setSyncStatus('')
    const res = await syncAllRuleProviders().catch(() => null)
    setSyncingAll(false)
    load()
    if (res) {
      const failed = res.results.filter(r => !r.ok)
      setSyncStatus(failed.length ? `同步完成，${failed.length} 个失败` : `全部 ${res.results.length} 个规则已同步`)
    }
    setTimeout(() => setSyncStatus(''), 5000)
  }

  // Debounced search
  useEffect(() => {
    if (searchDebounce.current) clearTimeout(searchDebounce.current)
    if (!query.trim()) { setSearchResults(null); return }
    searchDebounce.current = setTimeout(() => {
      setSearching(true)
      searchRules(query.trim())
        .then(d => setSearchResults(d.results))
        .catch(() => setSearchResults([]))
        .finally(() => setSearching(false))
    }, 400)
    return () => { if (searchDebounce.current) clearTimeout(searchDebounce.current) }
  }, [query])

  const totalRules = providers.reduce((s, p) => s + (p.ruleCount ?? 0), 0)
  const totalSizeMB = providers.reduce((s, p) => s + (p.size_mb ?? 0), 0)

  return (
    <div className="space-y-4">
      {/* Header bar */}
      <div className="flex items-start justify-between gap-3 flex-wrap">
        <div>
          <p className="text-sm text-muted">
            共 <span className="text-slate-300 font-medium">{providers.length}</span> 个规则集，
            <span className="text-slate-300 font-medium"> {totalRules.toLocaleString()}</span> 条规则，
            占用 <span className="text-slate-300 font-medium">{totalSizeMB.toFixed(1)} MB</span>
          </p>
        </div>
        <div className="flex items-center gap-2">
          {syncStatus && <span className="text-xs text-success">{syncStatus}</span>}
          <button className="btn-ghost flex items-center gap-2" onClick={load} disabled={loading}>
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} /> 刷新
          </button>
          <button className="btn-primary flex items-center gap-2" onClick={handleSyncAll} disabled={syncingAll}>
            <Zap size={14} className={syncingAll ? 'animate-pulse' : ''} />
            {syncingAll ? '同步中…' : '强制同步全部'}
          </button>
        </div>
      </div>

      {/* Search */}
      <div className="relative">
        <Search size={14} className="absolute left-3.5 top-1/2 -translate-y-1/2 text-muted pointer-events-none" />
        <input
          className="w-full bg-surface-2 border border-white/10 rounded-xl pl-9 pr-4 py-2.5 text-sm text-white outline-none focus:border-brand transition-colors placeholder:text-muted"
          placeholder="搜索域名 / IP 是否在规则中，例如：google.com、8.8.8.8"
          value={query}
          onChange={e => setQuery(e.target.value)}
        />
        {searching && <RefreshCw size={12} className="absolute right-3.5 top-1/2 -translate-y-1/2 text-muted animate-spin" />}
      </div>

      {/* Search results */}
      {searchResults !== null && (
        <div className="space-y-3">
          {searchResults.length === 0 ? (
            <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-5 text-sm text-muted text-center">
              未在任何规则集中找到「{query}」
            </div>
          ) : (
            searchResults.map(r => (
              <div key={r.provider} className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4">
                <div className="flex items-center gap-2 mb-2">
                  <span className="text-sm font-semibold text-slate-100">{r.provider}</span>
                  <span className="text-[10px] text-muted">{behaviorLabel(r.behavior)}</span>
                  <span className="text-xs text-success ml-auto">{r.total} 条匹配</span>
                </div>
                <div className="space-y-0.5 max-h-40 overflow-y-auto">
                  {r.matches.map((m, i) => (
                    <p key={i} className="text-xs font-mono text-slate-300 px-2 py-0.5 rounded bg-white/5">{m}</p>
                  ))}
                  {r.total > r.matches.length && (
                    <p className="text-xs text-muted px-2 py-0.5">…还有 {r.total - r.matches.length} 条</p>
                  )}
                </div>
              </div>
            ))
          )}
        </div>
      )}

      {/* Providers list */}
      {searchResults === null && (
        <div className="space-y-2">
          {loading && (
            <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-8 text-sm text-muted text-center">加载中…</div>
          )}
          {!loading && providers.length === 0 && (
            <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-8 text-sm text-muted text-center">
              未找到规则集。请确认核心已启动且 rule-provider 配置正确。
            </div>
          )}
          {providers
            .slice()
            .sort((a, b) => (b.ruleCount ?? 0) - (a.ruleCount ?? 0))
            .map(p => (
              <ProviderRow
                key={p.name}
                p={p}
                onSync={() => handleSyncOne(p.name)}
                syncing={syncingNames.has(p.name)}
              />
            ))}
        </div>
      )}
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

type Tab = 'sources' | 'rules' | 'subscriptions' | 'running'

export function ConfigManagement() {
  const [tab, setTab] = useState<Tab>('sources')
  const [coreRunning, setCoreRunning] = useState(false)
  const [activeSource, setActiveSourceData] = useState<ActiveSource | null>(null)
  const [coreChecked, setCoreChecked] = useState(false)

  useEffect(() => {
    getOverviewCore()
      .then(d => setCoreRunning(d.core.state === 'running'))
      .catch(() => setCoreRunning(false))
      .finally(() => setCoreChecked(true))
    getSources()
      .then(d => setActiveSourceData(d.active_source))
      .catch(() => null)
  }, [])

  // Auto-switch away from runtime-only tabs if core is not running
  useEffect(() => {
    if (coreChecked && !coreRunning && (tab === 'rules' || tab === 'running')) {
      setTab('sources')
    }
  }, [coreRunning, coreChecked, tab])

  const allTabs = [
    { key: 'sources' as Tab, icon: <Database size={14} />, label: '配置文件' },
    { key: 'subscriptions' as Tab, icon: <List size={14} />, label: '订阅' },
    ...(coreRunning ? [
      { key: 'rules' as Tab, icon: <Shield size={14} />, label: '规则集' },
      { key: 'running' as Tab, icon: <Eye size={14} />, label: '运行中配置' },
    ] : []),
  ]

  return (
    <div className="p-6 space-y-5 max-w-4xl mx-auto">
      <div className="flex items-center gap-3">
        <div className="w-8 h-8 rounded-xl bg-brand/15 border border-brand/20 flex items-center justify-center">
          <FolderCog size={15} className="text-brand" />
        </div>
        <h1 className="text-lg font-semibold text-white">配置管理</h1>
      </div>

      {/* Tab switcher */}
      <div className="flex items-center gap-2 flex-wrap">
        <div className="flex items-center bg-surface-1/80 rounded-2xl border border-white/[0.06] p-1.5 gap-1">
          {allTabs.map(({ key, icon, label }) => (
            <button
              key={key}
              onClick={() => setTab(key)}
              className={`flex items-center gap-2 px-4 py-2 rounded-xl text-sm font-medium transition-all cursor-pointer ${
                tab === key
                  ? 'bg-brand/15 text-brand border border-brand/25'
                  : 'text-muted hover:text-slate-300 hover:bg-white/[0.04]'
              }`}
            >
              {icon}{label}
            </button>
          ))}
        </div>
        {!coreRunning && coreChecked && (
          <span className="flex items-center gap-1.5 text-xs text-muted bg-surface-2/60 border border-white/[0.06] px-3 py-1.5 rounded-xl">
            <AlertCircle size={12} /> 服务未运行，规则集和运行配置不可用
          </span>
        )}
      </div>

      {tab === 'sources'       && <SourceFilesPanel coreRunning={coreRunning} />}
      {tab === 'subscriptions' && <SubscriptionsPanel coreRunning={coreRunning} activeSource={activeSource} />}
      {tab === 'rules'         && coreRunning && <RulesPanel />}
      {tab === 'running'       && coreRunning && <RunningConfigPanel activeSource={activeSource} />}
    </div>
  )
}
