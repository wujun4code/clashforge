import type { ReactNode } from 'react'
import { Terminal, X } from 'lucide-react'

function cn(...parts: Array<string | false | null | undefined>) {
  return parts.filter(Boolean).join(' ')
}

/* ── PageHeader ─────────────────────────────────────────── */
export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  metrics,
}: {
  eyebrow: string
  title: string
  description?: string
  actions?: ReactNode
  metrics?: Array<{ label: string; value: string; color?: 'cyan' | 'green' | 'yellow' | 'magenta' | 'red' }>
}) {
  // token names kept for backwards compat, but now map to the soft cat palette
  const metricColor: Record<string, string> = {
    cyan:    '#6AA8E0',
    green:   '#8FD4A8',
    yellow:  '#F5B86B',
    magenta: '#F4A6B5',
    red:     '#E87E7E',
  }

  return (
    <section className="hero-panel animate-fade-in">
      <div className="relative flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
        <div className="max-w-3xl space-y-3">
          {/* Eyebrow tag */}
          <div className="inline-flex items-center gap-2 rounded-full px-3 py-1 text-[11px] font-semibold tracking-wide text-brand border border-brand/30 bg-brand/10">
            <Terminal size={11} />
            {eyebrow}
          </div>

          <div className="space-y-2">
            <h1 className="font-display text-2xl font-extrabold tracking-tight text-display-gradient md:text-3xl">
              {title}
            </h1>
            {description ? (
              <p className="max-w-2xl text-sm leading-7 text-[color:var(--text-secondary)]">
                {description}
              </p>
            ) : null}
          </div>
        </div>

        <div className="flex w-full flex-col gap-4 lg:w-auto lg:min-w-[320px] lg:items-end">
          {actions ? (
            <div className="flex flex-wrap items-center gap-2 lg:justify-end">{actions}</div>
          ) : null}

          {metrics?.length ? (
            <div className="grid w-full grid-cols-2 gap-2 sm:grid-cols-4 lg:w-auto">
              {metrics.map((m) => {
                const c = m.color ?? 'cyan'
                return (
                  <div
                    key={m.label}
                    className="rounded-xl border border-white/10 bg-surface-2/50 px-3 py-2.5 backdrop-blur-xl transition-colors hover:border-brand/30"
                  >
                    <p className="text-[10px] uppercase tracking-wider text-[color:var(--text-muted)]">{m.label}</p>
                    <p
                      className="font-mono mt-1.5 text-sm font-bold tabular-nums"
                      style={{ color: metricColor[c] }}
                    >
                      {m.value}
                    </p>
                  </div>
                )
              })}
            </div>
          ) : null}
        </div>
      </div>
    </section>
  )
}

/* ── SectionCard ────────────────────────────────────────── */
export function SectionCard({
  title,
  description,
  actions,
  children,
  className,
}: {
  title?: string
  description?: string
  actions?: ReactNode
  children: ReactNode
  className?: string
  accent?: 'cyan' | 'magenta' | 'yellow' | 'green' | 'red'
}) {
  return (
    <section className={cn('glass-card glass-section', className)}>
      {title || description || actions ? (
        <div className="panel-header">
          <div>
            {title ? (
              <h2 className="font-display text-base font-bold tracking-tight text-[color:var(--text-primary)]">
                {title}
              </h2>
            ) : null}
            {description ? (
              <p className="mt-1 text-xs leading-6 text-[color:var(--text-muted)]">{description}</p>
            ) : null}
          </div>
          {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
        </div>
      ) : null}
      <div className={title || description || actions ? 'px-5 pb-5 pt-3 md:px-6 md:pb-6' : 'p-5 md:p-6'}>
        {children}
      </div>
    </section>
  )
}

/* ── SegmentedTabs ──────────────────────────────────────── */
export function SegmentedTabs<T extends string>({
  items,
  value,
  onChange,
}: {
  items: Array<{ value: T; label: string; icon?: ReactNode; hint?: string; disabled?: boolean }>
  value: T
  onChange: (value: T) => void
}) {
  return (
    <div className="segmented-tabs" role="tablist">
      {items.map((item) => {
        const active = item.value === value
        return (
          <button
            key={item.value}
            type="button"
            role="tab"
            aria-selected={active}
            disabled={item.disabled}
            onClick={() => onChange(item.value)}
            className={cn('segmented-tab', active && 'segmented-tab-active')}
            title={item.hint}
          >
            {item.icon ? <span className="text-current">{item.icon}</span> : null}
            <span>{item.label}</span>
          </button>
        )
      })}
    </div>
  )
}

/* ── EmptyState ─────────────────────────────────────────── */
export function EmptyState({
  title,
  description,
  action,
  icon,
}: {
  title: string
  description?: string
  action?: ReactNode
  icon?: ReactNode
}) {
  return (
    <div className="empty-state">
      <div className="empty-state-icon">{icon ?? <Terminal size={20} />}</div>
      <div className="space-y-1.5">
        <p className="font-display text-base font-bold text-[color:var(--text-primary)]">{title}</p>
        {description ? (
          <p className="mx-auto max-w-xl text-sm leading-6 text-[color:var(--text-muted)]">{description}</p>
        ) : null}
      </div>
      {action ? <div className="pt-1">{action}</div> : null}
    </div>
  )
}

/* ── InlineNotice ───────────────────────────────────────── */
export function InlineNotice({
  tone = 'info',
  title,
  children,
  action,
}: {
  tone?: 'info' | 'success' | 'warning' | 'danger'
  title?: string
  children: ReactNode
  action?: ReactNode
}) {
  const toneClass = {
    info:    'notice-info',
    success: 'notice-success',
    warning: 'notice-warning',
    danger:  'notice-danger',
  }[tone]

  return (
    <div className={toneClass}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          {title ? (
            <p className="font-display text-xs font-bold tracking-wide uppercase opacity-90">
              {title}
            </p>
          ) : null}
          <div className="mt-1 text-sm leading-6">{children}</div>
        </div>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>
    </div>
  )
}

/* ── ModalShell ─────────────────────────────────────────── */
export function ModalShell({
  title,
  description,
  icon,
  onClose,
  children,
  size = 'md',
  dismissible = true,
}: {
  title: string
  description?: string
  icon?: ReactNode
  onClose?: () => void
  children: ReactNode
  size?: 'sm' | 'md' | 'lg'
  dismissible?: boolean
}) {
  const maxWidth = size === 'sm' ? 'max-w-sm' : size === 'lg' ? 'max-w-2xl' : 'max-w-md'

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 backdrop-blur-md"
      style={{ background: 'rgba(12,18,32,0.72)' }}
      onClick={dismissible ? onClose : undefined}
    >
      <div
        className={cn('glass-modal w-full overflow-hidden animate-scale-in', maxWidth)}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Modal header */}
        <div className="flex items-start gap-3 px-5 py-4 md:px-6 border-b border-white/8">
          {icon ? (
            <div className="mt-0.5 flex-shrink-0 p-2 text-brand rounded-xl border border-brand/25 bg-brand/10">
              {icon}
            </div>
          ) : null}
          <div className="min-w-0 flex-1">
            <h3 className="font-display text-lg font-bold text-[color:var(--text-primary)]">
              {title}
            </h3>
            {description ? (
              <p className="mt-1 text-xs leading-5 text-[color:var(--text-muted)]">{description}</p>
            ) : null}
          </div>
          {dismissible && onClose ? (
            <button
              type="button"
              onClick={onClose}
              className="flex-shrink-0 rounded-lg p-1.5 text-[color:var(--text-muted)] hover:bg-surface-2/60 hover:text-[color:var(--text-primary)] transition-colors cursor-pointer"
              aria-label="关闭"
            >
              <X size={16} />
            </button>
          ) : null}
        </div>

        <div className="px-5 py-5 md:px-6">{children}</div>
      </div>
    </div>
  )
}
