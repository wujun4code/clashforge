import { useCallback, useEffect, useRef, useState } from 'react'
import { Activity, AlertCircle, CheckCircle2, Loader2, X } from 'lucide-react'
import { DIAG_NODE_URL } from '../api/client'
import type { NodeDiagCheck, NodeDiagSummary } from '../api/client'

// ── SSE streaming ──────────────────────────────────────────────────────────────

async function streamDiagSSE(
  url: string,
  secret: string,
  onCheck: (c: NodeDiagCheck) => void,
  onDone: (s: NodeDiagSummary) => void,
  signal?: AbortSignal,
) {
  const resp = await fetch(url, {
    method: 'POST',
    headers: { Authorization: `Bearer ${secret}` },
    signal,
  })
  if (!resp.ok) {
    let detail = `请求失败 (${resp.status})`
    try { const b = await resp.json(); detail = b?.error?.message ?? detail } catch { /* ignore */ }
    throw new Error(detail)
  }
  const reader = resp.body?.getReader()
  if (!reader) return
  const dec = new TextDecoder()
  let buf = ''
  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buf += dec.decode(value, { stream: true })
    const lines = buf.split('\n')
    buf = lines.pop() ?? ''
    for (const line of lines) {
      if (!line.startsWith('data: ')) continue
      try {
        const d = JSON.parse(line.slice(6))
        if (d.type === 'done') onDone(d.summary)
        else if (d.type === 'check') onCheck(d.check)
      } catch { /* skip malformed */ }
    }
  }
}

// ── helpers ────────────────────────────────────────────────────────────────────

const CATEGORY_LABELS: Record<string, string> = {
  network: '网络',
  process: '进程',
  system:  '系统',
  cert:    '证书',
}

function StatusIcon({ status }: { status: string }) {
  switch (status) {
    case 'ok':    return <CheckCircle2 size={13} className="text-emerald-400 shrink-0" />
    case 'warn':  return <AlertCircle  size={13} className="text-amber-400 shrink-0" />
    case 'error': return <X            size={13} className="text-red-400 shrink-0" />
    case 'skip':  return <span className="inline-block w-3 h-3 rounded-full bg-white/20 shrink-0" />
    default:      return <Loader2      size={13} className="text-brand shrink-0 animate-spin" />
  }
}

// ── NodeDiagPanel ──────────────────────────────────────────────────────────────

export function NodeDiagPanel({
  nodeId,
  onSummary,
}: {
  nodeId: string
  onSummary?: (summary: NodeDiagSummary) => void
}) {
  const [checks, setChecks] = useState<NodeDiagCheck[]>([])
  const [summary, setSummary] = useState<NodeDiagSummary | null>(null)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const containerRef = useRef<HTMLDivElement>(null)

  const run = useCallback(async () => {
    setChecks([]); setSummary(null); setError(''); setExpanded(new Set()); setRunning(true)
    const secret = localStorage.getItem('cf_secret') || ''
    const abort = new AbortController()
    try {
      await streamDiagSSE(
        DIAG_NODE_URL(nodeId),
        secret,
        c => setChecks(prev => [...prev, c]),
        s => { setSummary(s); setRunning(false); onSummary?.(s) },
        abort.signal,
      )
    } catch (e) {
      if ((e as Error).name !== 'AbortError') setError(e instanceof Error ? e.message : '诊断失败')
      setRunning(false)
    }
  }, [nodeId, onSummary])

  useEffect(() => { void run() }, [run])

  useEffect(() => {
    containerRef.current?.scrollTo({ top: containerRef.current.scrollHeight, behavior: 'smooth' })
  }, [checks])

  const toggle = (id: string) =>
    setExpanded(prev => { const n = new Set(prev); n.has(id) ? n.delete(id) : n.add(id); return n })

  const grouped = checks.reduce<Record<string, NodeDiagCheck[]>>((acc, c) => {
    ;(acc[c.category ?? 'other'] ??= []).push(c)
    return acc
  }, {})

  const summaryColor = !summary ? '' :
    summary.error > 0 ? 'border-red-500/20 bg-red-500/[0.04] text-red-300' :
    summary.warn  > 0 ? 'border-amber-500/20 bg-amber-500/[0.04] text-amber-300' :
    'border-emerald-500/20 bg-emerald-500/[0.04] text-emerald-300'

  return (
    <div className="space-y-3">
      {/* Live check list */}
      <div ref={containerRef} className="space-y-3 max-h-[40vh] overflow-y-auto pr-1">
        {Object.entries(grouped).map(([cat, catChecks]) => (
          <div key={cat} className="space-y-1">
            <p className="text-[10px] font-semibold uppercase tracking-[0.18em] text-white/30 px-1">
              {CATEGORY_LABELS[cat] ?? cat}
            </p>
            <div className="rounded-xl border border-white/[0.08] bg-white/[0.02] divide-y divide-white/[0.05] overflow-hidden">
              {catChecks.map(c => (
                <div key={c.id}>
                  <div
                    className={`flex items-center gap-2.5 px-3 py-2.5 text-xs ${c.detail ? 'cursor-pointer hover:bg-white/[0.03]' : ''}`}
                    onClick={() => c.detail && toggle(c.id)}
                  >
                    <StatusIcon status={c.status} />
                    <span className={`flex-1 font-medium ${
                      c.status === 'error' ? 'text-red-300' :
                      c.status === 'warn'  ? 'text-amber-300' :
                      c.status === 'skip'  ? 'text-white/35' :
                      'text-white/80'
                    }`}>{c.name}</span>
                    {c.value && <span className="shrink-0 font-mono text-[11px] text-white/35">{c.value}</span>}
                  </div>
                  <p className="px-3 pb-2 -mt-1 pl-8 text-[11px] text-white/30 leading-relaxed">{c.message}</p>
                  {c.detail && expanded.has(c.id) && (
                    <pre className="mx-3 mb-2 max-h-32 overflow-y-auto rounded-lg border border-white/[0.08] bg-black/30 px-3 py-2 font-mono text-[10px] text-white/60 whitespace-pre-wrap">
                      {c.detail}
                    </pre>
                  )}
                </div>
              ))}
            </div>
          </div>
        ))}

        {running && checks.length === 0 && (
          <div className="flex items-center gap-2 text-xs text-white/30 py-4 justify-center">
            <Loader2 size={14} className="animate-spin" /> 通过 SSH 连接节点，执行诊断检查…
          </div>
        )}
        {running && checks.length > 0 && (
          <div className="flex items-center gap-2 text-xs text-white/30 px-1">
            <Loader2 size={11} className="animate-spin" /> 检查中…
          </div>
        )}
      </div>

      {/* Summary */}
      {summary && (
        <div className={`rounded-xl border px-3 py-2.5 text-xs flex items-center gap-3 ${summaryColor}`}>
          <Activity size={13} className="shrink-0" />
          <span className="font-semibold">诊断完成</span>
          <span className="text-emerald-400/90">✓ {summary.ok}</span>
          {summary.warn  > 0 && <span className="text-amber-400/90">⚠ {summary.warn}</span>}
          {summary.error > 0 && <span className="text-red-400/90">✗ {summary.error}</span>}
          {summary.skip  > 0 && <span className="text-white/30">— {summary.skip} 跳过</span>}
        </div>
      )}

      {error && (
        <div className="rounded-xl border border-red-500/20 bg-red-500/5 px-3 py-2 text-xs text-red-300 flex items-center gap-2">
          <AlertCircle size={12} /> {error}
        </div>
      )}
    </div>
  )
}
