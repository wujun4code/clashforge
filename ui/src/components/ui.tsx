import type { ReactNode } from 'react'
import { Sparkles, X } from 'lucide-react'
import { CatMascot } from './CatMascot'

function cn(...parts: Array<string | false | null | undefined>) {
  return parts.filter(Boolean).join(' ')
}

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
  const metricColor: Record<string, string> = {
    cyan: '#67E8F9',
    green: '#6EE7B7',
    yellow: '#FBBF24',
    magenta: '#F9A8D4',
    red: '#FDA4AF',
  }

  return (
    <section className="hero-panel animate-fade-in">
      <div className="relative flex flex-col gap-6 xl:flex-row xl:items-end xl:justify-between">
        <div className="max-w-3xl space-y-4">
          <div className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/6 px-3 py-1.5 text-[11px] font-semibold uppercase tracking-[0.18em] text-violet-200">
            <Sparkles size={12} />
            {eyebrow}
          </div>

          <div className="flex items-start gap-4">
            <div className="hidden rounded-[22px] border border-white/10 bg-white/8 p-2.5 shadow-lg lg:block">
              <CatMascot size={56} mood="default" />
            </div>
            <div className="space-y-3">
              <h1 className="hero-title text-3xl leading-tight text-white md:text-4xl">
                <span className="cat-gradient-text">{title}</span>
              </h1>
              {description ? (
                <p className="max-w-2xl text-sm leading-7 text-slate-200/85 md:text-[15px]">
                  {description}
                </p>
              ) : null}
            </div>
          </div>
        </div>

        <div className="flex w-full flex-col gap-4 xl:w-[540px] xl:items-end">
          {actions ? <div className="flex flex-wrap items-center gap-2 xl:justify-end">{actions}</div> : null}

          {metrics?.length ? (
            <div className="hero-metrics-grid w-full">
              {metrics.map((m) => {
                const color = metricColor[m.color ?? 'cyan']
                return (
                  <div key={m.label} className="cat-stat-tile">
                    <p className="text-[11px] uppercase tracking-[0.18em] text-slate-300/75">{m.label}</p>
                    <p className="mt-2 text-xl font-semibold text-white md:text-2xl" style={{ color }}>
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

export function SectionCard({
  title,
  description,
  actions,
  children,
  className,
  accent = 'cyan',
}: {
  title?: string
  description?: string
  actions?: ReactNode
  children: ReactNode
  className?: string
  accent?: 'cyan' | 'magenta' | 'yellow' | 'green' | 'red'
}) {
  const accentMap: Record<string, string> = {
    cyan: 'bg-cyan-300',
    magenta: 'bg-pink-300',
    yellow: 'bg-amber-300',
    green: 'bg-emerald-300',
    red: 'bg-rose-300',
  }

  return (
    <section className={cn('glass-card glass-section', className)}>
      {title || description || actions ? (
        <div className="panel-header">
          <div>
            {title ? (
              <div className="flex items-center gap-2">
                <span className={cn('h-2.5 w-2.5 rounded-full', accentMap[accent])} />
                <h2 className="text-base font-semibold text-white">{title}</h2>
              </div>
            ) : null}
            {description ? <p className="mt-2 text-sm leading-6 text-slate-300/82">{description}</p> : null}
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
      <div className="empty-state-icon">{icon ?? <CatMascot size={24} mood="sleepy" />}</div>
      <div className="space-y-1.5">
        <p className="text-base font-semibold text-white">{title}</p>
        {description ? <p className="mx-auto max-w-xl text-sm leading-6 text-slate-300/82">{description}</p> : null}
      </div>
      {action ? <div className="pt-1">{action}</div> : null}
    </div>
  )
}

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
    info: 'notice-info',
    success: 'notice-success',
    warning: 'notice-warning',
    danger: 'notice-danger',
  }[tone]

  return (
    <div className={toneClass}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          {title ? <p className="text-[11px] font-semibold uppercase tracking-[0.16em] opacity-90">{title}</p> : null}
          <div className="mt-1 text-sm leading-6">{children}</div>
        </div>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>
    </div>
  )
}

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
      style={{ background: 'rgba(7, 11, 23, 0.78)' }}
      onClick={dismissible ? onClose : undefined}
    >
      <div
        className={cn('glass-modal w-full overflow-hidden animate-scale-in', maxWidth)}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start gap-3 border-b border-white/10 px-5 py-4 md:px-6">
          {icon ? (
            <div className="mt-0.5 flex-shrink-0 rounded-2xl border border-white/10 bg-white/8 p-2 text-violet-200">
              {icon}
            </div>
          ) : null}
          <div className="min-w-0 flex-1">
            <h3 className="text-lg font-semibold text-white">{title}</h3>
            {description ? <p className="mt-1 text-sm leading-6 text-slate-300/82">{description}</p> : null}
          </div>
          {dismissible && onClose ? (
            <button type="button" onClick={onClose} className="btn-ghost btn-icon-sm" aria-label="关闭">
              <X size={14} />
            </button>
          ) : null}
        </div>
        <div className="px-5 py-5 md:px-6">{children}</div>
      </div>
    </div>
  )
}
