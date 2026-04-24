import type { ReactNode } from 'react'
import { Sparkles } from 'lucide-react'

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
  metrics?: Array<{ label: string; value: string }>
}) {
  return (
    <section className="hero-panel relative overflow-hidden">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_top_right,rgba(167,139,250,0.26),transparent_34%),radial-gradient(circle_at_bottom_left,rgba(249,115,22,0.16),transparent_32%)]" />
      <div className="relative flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
        <div className="max-w-3xl space-y-3">
          <div className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.04] px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.28em] text-brand-light">
            <Sparkles size={12} className="text-cta" />
            {eyebrow}
          </div>
          <div className="space-y-2">
            <h1 className="text-2xl font-semibold tracking-tight text-white md:text-3xl">{title}</h1>
            {description ? <p className="max-w-2xl text-sm leading-6 text-[#B9B6D3] md:text-[15px]">{description}</p> : null}
          </div>
        </div>

        <div className="flex w-full flex-col gap-4 lg:w-auto lg:min-w-[320px] lg:items-end">
          {actions ? <div className="flex flex-wrap items-center gap-2 lg:justify-end">{actions}</div> : null}
          {metrics?.length ? (
            <div className="grid w-full grid-cols-2 gap-3 sm:grid-cols-4 lg:w-auto">
              {metrics.map((metric) => (
                <div key={metric.label} className="rounded-2xl border border-white/8 bg-black/10 px-4 py-3 backdrop-blur-xl">
                  <p className="text-[10px] uppercase tracking-[0.2em] text-muted">{metric.label}</p>
                  <p className="mt-2 text-sm font-semibold text-white">{metric.value}</p>
                </div>
              ))}
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
}: {
  title?: string
  description?: string
  actions?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn('glass-card glass-section', className)}>
      {title || description || actions ? (
        <div className="panel-header">
          <div>
            {title ? <h2 className="text-base font-semibold text-white">{title}</h2> : null}
            {description ? <p className="mt-1 text-sm leading-6 text-muted">{description}</p> : null}
          </div>
          {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
        </div>
      ) : null}
      <div className={title || description || actions ? 'px-5 pb-5 pt-1 md:px-6 md:pb-6' : 'p-5 md:p-6'}>{children}</div>
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
      <div className="empty-state-icon">{icon ?? <Sparkles size={18} />}</div>
      <div className="space-y-1.5">
        <p className="text-sm font-semibold text-white">{title}</p>
        {description ? <p className="mx-auto max-w-xl text-xs leading-6 text-muted">{description}</p> : null}
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
    info: 'border-brand/25 bg-brand-subtle/35 text-[#E9E3FF]',
    success: 'border-success/25 bg-success-subtle/30 text-[#DCFCE7]',
    warning: 'border-warning/25 bg-warning-subtle/30 text-[#FEF3C7]',
    danger: 'border-danger/25 bg-danger-subtle/30 text-[#FECACA]',
  }[tone]

  return (
    <div className={cn('rounded-2xl border px-4 py-3', toneClass)}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          {title ? <p className="text-xs font-semibold uppercase tracking-[0.18em] opacity-90">{title}</p> : null}
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
      className="fixed inset-0 z-50 flex items-center justify-center bg-[#05030d]/70 p-4 backdrop-blur-md"
      onClick={dismissible ? onClose : undefined}
    >
      <div
        className={cn('glass-card glass-modal w-full overflow-hidden border-white/12 shadow-glass-lg', maxWidth)}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="border-b border-white/8 px-5 py-4 md:px-6">
          <div className="flex items-start gap-3">
            {icon ? <div className="mt-0.5 rounded-2xl border border-white/10 bg-white/[0.04] p-2 text-brand-light">{icon}</div> : null}
            <div className="min-w-0 flex-1">
              <h3 className="text-lg font-semibold text-white">{title}</h3>
              {description ? <p className="mt-1 text-sm leading-6 text-muted">{description}</p> : null}
            </div>
          </div>
        </div>
        <div className="px-5 py-5 md:px-6">{children}</div>
      </div>
    </div>
  )
}
