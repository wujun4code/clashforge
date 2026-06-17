import { useEffect, useState } from 'react'
import {
  getConfig, updateConfig, resetClashForge, getClashforgeVersion,
  getSelfUpdateConfig, updateSelfUpdateConfig, triggerSelfUpdate,
} from '../api/client'
import type { ClashforgeVersionData, SelfUpdateConfig } from '../api/client'
import { Save, Loader2, RotateCcw, ShieldAlert, RefreshCw, Download, Radio, Zap } from 'lucide-react'
import { InlineNotice, ModalShell, PageHeader, SectionCard, SelectInput } from '../components/ui'

const CHANNEL_KEY = 'cf_update_channel'

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="flex-shrink-0 w-44">
        <label className="text-sm text-slate-300">{label}</label>
        {hint && <p className="text-xs text-muted mt-0.5 leading-4">{hint}</p>}
      </div>
      <div className="flex-1">{children}</div>
    </div>
  )
}

function TextInput({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <input
      className="w-full bg-surface-2 border border-white/10 rounded-xl px-3 py-2 text-sm text-white outline-none focus:border-brand transition-colors"
      value={value}
      onChange={e => onChange(e.target.value)}
    />
  )
}


export function Settings() {
  const [cfg, setCfg] = useState<Record<string, unknown> | null>(null)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [saveError, setSaveError] = useState('')

  type DangerTarget = 'reset'
  const [dangerConfirm, setDangerConfirm] = useState<DangerTarget | null>(null)
  const [dangerRunning, setDangerRunning] = useState(false)
  const [dangerResult, setDangerResult] = useState<{ ok: boolean; message: string } | null>(null)
  const [resetDone, setResetDone] = useState(false)

  // ── Update checker ─────────────────────────────────────────────────────────
  type UpdateChannel = 'stable' | 'preview'
  const [channel, setChannel] = useState<UpdateChannel>(
    () => (localStorage.getItem(CHANNEL_KEY) as UpdateChannel) || 'stable'
  )
  const [checkingUpdate, setCheckingUpdate] = useState(false)
  const [updateData, setUpdateData] = useState<ClashforgeVersionData | null>(null)
  const [updateError, setUpdateError] = useState('')

  const changeChannel = (ch: UpdateChannel) => {
    setChannel(ch)
    localStorage.setItem(CHANNEL_KEY, ch)
    setUpdateData(null)
    setUpdateError('')
  }

  const checkUpdate = async () => {
    setCheckingUpdate(true)
    setUpdateError('')
    setUpdateData(null)
    try {
      const data = await getClashforgeVersion(channel)
      setUpdateData(data)
    } catch (e: unknown) {
      setUpdateError(String(e))
    } finally {
      setCheckingUpdate(false)
    }
  }

  // ── Auto self-update ───────────────────────────────────────────────────────
  const [selfUpdate, setSelfUpdate] = useState<SelfUpdateConfig | null>(null)
  const [selfUpdateSaving, setSelfUpdateSaving] = useState(false)
  const [selfUpdateSaved, setSelfUpdateSaved] = useState(false)
  const [selfUpdateSaveErr, setSelfUpdateSaveErr] = useState('')
  const [triggering, setTriggering] = useState(false)
  const [triggerResult, setTriggerResult] = useState<{ ok: boolean; message: string } | null>(null)

  const loadSelfUpdate = () => getSelfUpdateConfig().then(setSelfUpdate).catch(() => null)

  const saveSelfUpdate = async () => {
    if (!selfUpdate) return
    setSelfUpdateSaving(true); setSelfUpdateSaveErr(''); setSelfUpdateSaved(false)
    try {
      await updateSelfUpdateConfig({
        auto_self_update: selfUpdate.auto_self_update,
        self_update_time: selfUpdate.self_update_time,
        self_update_channel: selfUpdate.self_update_channel,
      })
      setSelfUpdateSaved(true)
      setTimeout(() => setSelfUpdateSaved(false), 2000)
    } catch (e: unknown) {
      setSelfUpdateSaveErr(String(e))
    } finally { setSelfUpdateSaving(false) }
  }

  const doTrigger = async () => {
    setTriggering(true); setTriggerResult(null)
    try {
      await triggerSelfUpdate()
      setTriggerResult({ ok: true, message: '自动更新已在后台启动，稍后服务将平滑重启。' })
      setTimeout(loadSelfUpdate, 3000)
    } catch (e: unknown) {
      setTriggerResult({ ok: false, message: e instanceof Error ? e.message : String(e) })
    } finally { setTriggering(false) }
  }

  useEffect(() => {
    getConfig().then(setCfg).catch(() => null)
    loadSelfUpdate()
  }, [])

  const set = (path: string[], value: unknown) => {
    if (!cfg) return
    const updated = JSON.parse(JSON.stringify(cfg))
    let cur = updated
    for (let i = 0; i < path.length - 1; i++) cur = cur[path[i]] as Record<string, unknown>
    cur[path[path.length - 1]] = value
    setCfg(updated)
  }

  const get = (path: string[]): unknown => {
    if (!cfg) return ''
    let cur = cfg as Record<string, unknown>
    for (const k of path) cur = (cur[k] ?? '') as Record<string, unknown>
    return cur
  }

  const saveGeneral = async () => {
    if (!cfg) return
    setSaving(true); setSaveError('')
    try {
      await updateConfig(cfg)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (e: unknown) {
      setSaveError(String(e))
    } finally { setSaving(false) }
  }

  const handleDangerAction = async (_target: DangerTarget) => {
    setDangerRunning(true)
    setDangerResult(null)
    try {
      const res = await resetClashForge()
      setDangerResult({ ok: res.ok, message: res.message })
      if (res.ok) setResetDone(true)
    } catch (e: unknown) {
      setDangerResult({ ok: false, message: e instanceof Error ? e.message : String(e) })
    } finally {
      setDangerRunning(false)
      setDangerConfirm(null)
    }
  }

  if (!cfg) return <div className="p-6 text-muted text-sm">加载中…</div>

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Settings"
        title="高级管理与系统行为"
        description="调整 ClashForge 的核心路径、日志策略与系统级动作。所有改动只作用于管理体验与展示层，不改变业务流程。"
        actions={
          <button className="btn-primary flex items-center gap-2" onClick={saveGeneral} disabled={saving}>
            <Save size={14} /> {saving ? '保存中…' : '保存配置'}
          </button>
        }
        metrics={[
          { label: '保存状态', value: saved ? '已保存' : saving ? '进行中' : '待提交' },
          { label: '危险区', value: '受保护' },
        ]}
      />

      {saveError && (
        <InlineNotice tone="danger" title="保存失败">
          {saveError}
        </InlineNotice>
      )}
      {saved && (
        <InlineNotice tone="success" title="保存成功">
          配置已写入 ClashForge，可继续进行其他调整。
        </InlineNotice>
      )}

      <SectionCard title="核心参数" description="用于管理 mihomo 启动路径、重启策略与日志级别。">
        <div className="space-y-4">
          <Field label="mihomo 路径">
            <TextInput value={get(['core', 'binary']) as string} onChange={v => set(['core', 'binary'], v)} />
          </Field>
          <Field label="最大重启次数">
            <TextInput value={String(get(['core', 'max_restarts']) ?? 3)} onChange={v => set(['core', 'max_restarts'], parseInt(v) || 3)} />
          </Field>
          <Field label="日志级别">
            <SelectInput
              value={get(['log', 'level']) as string || 'info'}
              onChange={v => set(['log', 'level'], v)}
              options={[
                { value: 'debug', label: 'Debug' },
                { value: 'info', label: 'Info' },
                { value: 'warn', label: 'Warn' },
                { value: 'error', label: 'Error' },
              ]}
            />
          </Field>
          <Field label="GeoData 格式" hint="false = mmdb 格式（country.mmdb），true = dat 格式（GeoIP.dat / GeoSite.dat）">
            <div className="flex gap-2">
              {([false, true] as const).map(v => (
                <button
                  key={String(v)}
                  onClick={() => set(['core', 'geodata_mode'], v)}
                  className={[
                    'rounded-xl border px-3 py-1.5 text-xs font-medium transition-colors',
                    get(['core', 'geodata_mode']) === v
                      ? 'border-brand bg-brand/10 text-brand'
                      : 'border-white/10 bg-surface-2 text-slate-400 hover:border-white/20 hover:text-white',
                  ].join(' ')}
                >
                  {v ? 'dat 模式' : 'mmdb 模式'}
                </button>
              ))}
            </div>
          </Field>
        </div>
      </SectionCard>

      <SectionCard title="危险操作" description="以下行为会影响配置资产与运行态，请谨慎处理。" className="border-danger/20">
        <div className="space-y-4">
          {dangerResult && (
            <InlineNotice tone={dangerResult.ok ? 'success' : 'danger'} title={dangerResult.ok ? '操作完成' : '操作失败'}>
              <div className="space-y-2">
                <p className="whitespace-pre-wrap text-sm leading-6">{dangerResult.message}</p>
                {resetDone ? (
                  <p className="text-xs text-muted">
                    请等待约 3 秒后刷新页面。
                    <button className="ml-2 text-brand hover:underline" onClick={() => window.location.reload()}>立即刷新</button>
                  </p>
                ) : null}
              </div>
            </InlineNotice>
          )}

          <div className="rounded-[24px] border border-danger/15 bg-[linear-gradient(180deg,rgba(239,68,68,0.12),rgba(239,68,68,0.05))] p-5">
            <div className="flex items-start gap-3">
              <div className="rounded-2xl border border-danger/20 bg-danger/10 p-2.5 text-danger">
                <ShieldAlert size={18} />
              </div>
              <div className="flex-1">
                <p className="text-base font-semibold text-white">重置 ClashForge</p>
                <p className="mt-2 text-sm leading-6 text-danger/80">
                  清除所有订阅、配置覆盖与生成文件，恢复为出厂默认设置，并自动重启进程。页面会短暂断开连接。
                </p>
              </div>
            </div>
            <div className="mt-4 flex flex-wrap gap-3">
              <button
                className="btn-danger flex items-center gap-2"
                onClick={() => { setDangerConfirm('reset'); setDangerResult(null) }}
                disabled={dangerRunning}
              >
                <RotateCcw size={13} /> 重置为出厂状态
              </button>
            </div>
          </div>
        </div>
      </SectionCard>

      {/* ── Auto self-update ── */}
      <SectionCard
        title="自动更新"
        description="ClashForge 将在设定时间通过已有的 mihomo 代理自动检测并安装最新版本，更新前后配置完全保留。"
        actions={
          <button
            className="btn-primary flex items-center gap-2"
            onClick={saveSelfUpdate}
            disabled={selfUpdateSaving || !selfUpdate}
          >
            <Save size={14} /> {selfUpdateSaving ? '保存中…' : '保存'}
          </button>
        }
      >
        {selfUpdateSaveErr && (
          <InlineNotice tone="danger" title="保存失败">{selfUpdateSaveErr}</InlineNotice>
        )}
        {selfUpdateSaved && (
          <InlineNotice tone="success" title="已保存">自动更新配置已更新。</InlineNotice>
        )}
        {triggerResult && (
          <InlineNotice tone={triggerResult.ok ? 'success' : 'danger'} title={triggerResult.ok ? '已触发' : '触发失败'}>
            {triggerResult.message}
          </InlineNotice>
        )}

        {selfUpdate ? (
          <div className="space-y-4">
            {/* Enable toggle */}
            <Field label="启用自动更新" hint="启用后将在设定时间自动检测并安装最新版本">
              <div className="flex gap-2">
                {([true, false] as const).map(v => (
                  <button
                    key={String(v)}
                    onClick={() => setSelfUpdate(s => s ? { ...s, auto_self_update: v } : s)}
                    className={[
                      'rounded-xl border px-3 py-1.5 text-xs font-medium transition-colors',
                      selfUpdate.auto_self_update === v
                        ? 'border-brand bg-brand/10 text-brand'
                        : 'border-white/10 bg-surface-2 text-slate-400 hover:border-white/20 hover:text-white',
                    ].join(' ')}
                  >
                    {v ? '启用' : '禁用'}
                  </button>
                ))}
              </div>
            </Field>

            {/* Update time */}
            <Field label="更新时间" hint="每日自动触发时刻，格式 HH:MM（本地时间），默认 02:00">
              <input
                type="time"
                className="bg-surface-2 border border-white/10 rounded-xl px-3 py-2 text-sm text-white outline-none focus:border-brand transition-colors"
                value={selfUpdate.self_update_time}
                onChange={e => setSelfUpdate(s => s ? { ...s, self_update_time: e.target.value } : s)}
                disabled={!selfUpdate.auto_self_update}
              />
            </Field>

            {/* Channel */}
            <Field label="更新频道" hint="正式版仅含稳定发布；体验版包含 beta/alpha 预发布">
              <div className="flex gap-2">
                {(['stable', 'preview'] as const).map(ch => (
                  <button
                    key={ch}
                    onClick={() => setSelfUpdate(s => s ? { ...s, self_update_channel: ch } : s)}
                    className={[
                      'flex items-center gap-1.5 rounded-xl border px-3 py-1.5 text-xs font-medium transition-colors',
                      selfUpdate.self_update_channel === ch
                        ? 'border-brand bg-brand/10 text-brand'
                        : 'border-white/10 bg-surface-2 text-slate-400 hover:border-white/20 hover:text-white',
                    ].join(' ')}
                    disabled={!selfUpdate.auto_self_update}
                  >
                    <Radio size={11} />
                    {ch === 'stable' ? '正式版' : '体验版'}
                  </button>
                ))}
              </div>
            </Field>

            {/* Manual trigger */}
            <Field label="立即更新" hint="跳过定时器，立即在后台执行一次更新检查与安装">
              <div className="flex items-center gap-3">
                <button
                  className="btn-primary flex items-center gap-2"
                  onClick={doTrigger}
                  disabled={triggering || selfUpdate.is_running}
                >
                  {(triggering || selfUpdate.is_running)
                    ? <><Loader2 size={13} className="animate-spin" />更新中…</>
                    : <><Zap size={13} />立即检查并更新</>}
                </button>
                {selfUpdate.is_running && (
                  <span className="text-xs text-slate-400">后台更新任务正在进行中</span>
                )}
              </div>
            </Field>

            {/* Last run status */}
            {selfUpdate.last_run && (
              <Field label="上次运行">
                <div className="rounded-xl border border-white/10 bg-surface-2 px-4 py-3 text-sm space-y-1">
                  <div className="flex flex-wrap gap-3 items-center">
                    <span className="text-slate-400 text-xs">
                      {new Date(selfUpdate.last_run.run_at).toLocaleString()}
                    </span>
                    {selfUpdate.last_run.success && (
                      <span className="rounded-md bg-green-500/10 px-2 py-0.5 text-xs font-semibold text-green-400">更新成功</span>
                    )}
                    {selfUpdate.last_run.skipped && (
                      <span className="rounded-md bg-white/5 px-2 py-0.5 text-xs text-slate-500">
                        {selfUpdate.last_run.skip_reason ?? '已是最新版本'}
                      </span>
                    )}
                    {!selfUpdate.last_run.success && !selfUpdate.last_run.skipped && (
                      <span className="rounded-md bg-red-500/10 px-2 py-0.5 text-xs font-semibold text-red-400">失败</span>
                    )}
                  </div>
                  {selfUpdate.last_run.new_version && (
                    <p className="text-xs text-slate-400">
                      {selfUpdate.last_run.old_version} → <span className="text-green-400 font-mono">{selfUpdate.last_run.new_version}</span>
                    </p>
                  )}
                  {selfUpdate.last_run.error && (
                    <p className="text-xs text-red-400 font-mono break-all">{selfUpdate.last_run.error}</p>
                  )}
                </div>
              </Field>
            )}
          </div>
        ) : (
          <p className="text-sm text-muted">加载中…</p>
        )}
      </SectionCard>

      <SectionCard
        title="软件更新"
        description="手动检查 ClashForge 最新版本，并选择更新频道。"
        actions={
          <button
            className="btn-primary flex items-center gap-2"
            onClick={checkUpdate}
            disabled={checkingUpdate}
          >
            <RefreshCw size={14} className={checkingUpdate ? 'animate-spin' : ''} />
            {checkingUpdate ? '检查中…' : '检查更新'}
          </button>
        }
      >
        <div className="space-y-4">
          {/* Channel picker */}
          <Field label="更新频道" hint="正式版仅含稳定发布；体验版包含 alpha/beta 预发布版本">
            <div className="flex gap-2">
              {(['stable', 'preview'] as const).map(ch => (
                <button
                  key={ch}
                  onClick={() => changeChannel(ch)}
                  className={[
                    'flex items-center gap-1.5 rounded-xl border px-3 py-1.5 text-xs font-medium transition-colors',
                    channel === ch
                      ? 'border-brand bg-brand/10 text-brand'
                      : 'border-white/10 bg-surface-2 text-slate-400 hover:border-white/20 hover:text-white',
                  ].join(' ')}
                >
                  <Radio size={11} />
                  {ch === 'stable' ? '正式版' : '体验版'}
                </button>
              ))}
            </div>
          </Field>

          {updateError && (
            <InlineNotice tone="danger" title="检查失败">{updateError}</InlineNotice>
          )}

          {updateData && (
            <div className="space-y-3">
              {/* Version info row */}
              <div className="flex flex-wrap gap-3 rounded-xl border border-white/10 bg-surface-2 px-4 py-3 text-sm">
                <span className="text-slate-400">当前版本 <span className="font-mono text-white">{updateData.current}</span></span>
                <span className="text-slate-600">→</span>
                <span className="text-slate-400">
                  最新版本{' '}
                  <span className={`font-mono ${updateData.has_update ? 'text-green-400' : 'text-white'}`}>
                    {updateData.latest || '—'}
                  </span>
                </span>
                {updateData.has_update && (
                  <span className="ml-auto rounded-md bg-green-500/10 px-2 py-0.5 text-xs font-semibold text-green-400">
                    有新版本
                  </span>
                )}
                {!updateData.has_update && updateData.latest && (
                  <span className="ml-auto rounded-md bg-white/5 px-2 py-0.5 text-xs text-slate-500">
                    已是最新
                  </span>
                )}
              </div>

              {/* Download button */}
              {updateData.has_update && updateData.download_url && (
                <div className="flex flex-wrap gap-2">
                  <a
                    href={updateData.download_url}
                    target="_blank"
                    rel="noreferrer"
                    className="btn-primary flex items-center gap-2 text-xs"
                  >
                    <Download size={13} /> 下载安装脚本
                  </a>
                  {updateData.release_url && (
                    <a
                      href={updateData.release_url}
                      target="_blank"
                      rel="noreferrer"
                      className="btn-ghost flex items-center gap-2 text-xs"
                    >
                      查看 Release
                    </a>
                  )}
                </div>
              )}

              {/* Release notes */}
              {updateData.release_notes && (
                <div className="rounded-xl border border-white/10 bg-surface-2">
                  <p className="border-b border-white/10 px-4 py-2 text-xs font-semibold uppercase tracking-widest text-slate-500">
                    Release Notes
                  </p>
                  <pre className="max-h-64 overflow-y-auto whitespace-pre-wrap px-4 py-3 font-mono text-xs leading-6 text-slate-300">
                    {updateData.release_notes}
                  </pre>
                </div>
              )}
            </div>
          )}
        </div>
      </SectionCard>

      {dangerConfirm && (
        <ModalShell
          title="确认重置 ClashForge"
          description="该操作将清除所有订阅、配置覆盖与生成配置，并自动重启 ClashForge 进程。"
          icon={<RotateCcw size={16} />}
          onClose={() => !dangerRunning && setDangerConfirm(null)}
          size="sm"
          dismissible={!dangerRunning}
        >
          <div className="space-y-5">
            <p className="text-sm leading-6 text-muted">
              重启后你需要重新运行配置向导。若当前正在排查问题，建议先备份现有配置再继续。
            </p>
            <div className="flex gap-3 pt-1">
              <button className="btn-ghost flex-1" onClick={() => setDangerConfirm(null)} disabled={dangerRunning}>
                取消
              </button>
              <button
                className="btn-danger flex-1 justify-center"
                onClick={() => handleDangerAction(dangerConfirm)}
                disabled={dangerRunning}
              >
                {dangerRunning
                  ? <><Loader2 size={13} className="animate-spin" />执行中…</>
                  : <><RotateCcw size={13} />确认重置</>}
              </button>
            </div>
          </div>
        </ModalShell>
      )}
    </div>
  )
}
