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
      <div className="pointer-events-none absolute inset-0"
        style={{
          background: 'radial-gradient(circle at top right, rgb(var(--brand) / 0.18), transparent 38%), radial-gradient(circle at bottom left, rgb(var(--cta) / 0.12), transparent 35%)'
        }}
      />
      <div className="relative flex flex-col gap-[var(--space-lg)] lg:flex-row lg:items-start lg:justify-between">
        <div className="max-w-3xl" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-sm)' }}>
          <div
            className="inline-flex items-center gap-[var(--space-xs)] font-semibold uppercase tracking-[0.26em] text-brand-light"
            style={{
              alignSelf: 'flex-start',
              borderRadius: 'var(--radius-full)',
              border: '1px solid rgb(var(--brand) / 0.20)',
              background: 'rgb(var(--brand-subtle-rgb) / var(--brand-subtle-alpha))',
              padding: '3px var(--space-sm)',
              fontSize: 'var(--text-xs)',
            }}
          >
            <Sparkles size={11} className="text-cta" />
            {eyebrow}
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
            <h1
              className="font-semibold tracking-tight text-white"
              style={{ fontSize: 'var(--text-2xl)' }}
            >
              {title}
            </h1>
            {description ? (
              <p style={{ fontSize: 'var(--text-sm)', lineHeight: '1.6' }} className="max-w-2xl text-muted">
                {description}
              </p>
            ) : null}
          </div>
        </div>

        <div
          className="flex w-full flex-col lg:w-auto lg:min-w-[280px] lg:items-end"
          style={{ gap: 'var(--space-md)' }}
        >
          {actions ? (
            <div className="flex flex-wrap items-center lg:justify-end" style={{ gap: 'var(--space-xs)' }}>
              {actions}
            </div>
          ) : null}
          {metrics?.length ? (
            <div className="grid w-full grid-cols-2 sm:grid-cols-4 lg:w-auto" style={{ gap: 'var(--space-sm)' }}>
              {metrics.map((metric) => (
                <div
                  key={metric.label}
                  className="backdrop-blur-xl"
                  style={{
                    borderRadius: 'var(--radius-lg)',
                    border: '1px solid rgb(var(--border-color) / var(--border-alpha))',
                    background: 'rgb(var(--surface-1) / 0.60)',
                    padding: 'var(--space-sm) var(--space-md)',
                  }}
                >
                  <p
                    className="uppercase text-muted"
                    style={{ fontSize: 'var(--text-xs)', letterSpacing: '0.18em' }}
                  >
                    {metric.label}
                  </p>
                  <p className="mt-2 font-semibold text-white" style={{ fontSize: 'var(--text-sm)' }}>
                    {metric.value}
                  </p>
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
  description?: string | ReactNode
  actions?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn('glass-card glass-section', className)}>
      {title || description || actions ? (
        <div className="panel-header">
          <div>
            {title ? (
              <h2 className="font-semibold text-white" style={{ fontSize: 'var(--text-base)' }}>
                {title}
              </h2>
            ) : null}
            {description ? (
              <p className="text-muted" style={{ marginTop: '4px', fontSize: 'var(--text-sm)', lineHeight: '1.6' }}>
                {description}
              </p>
            ) : null}
          </div>
          {actions ? (
            <div className="flex flex-wrap items-center" style={{ gap: 'var(--space-xs)' }}>
              {actions}
            </div>
          ) : null}
        </div>
      ) : null}
      <div
        style={
          title || description || actions
            ? { padding: 'var(--space-sm) var(--space-lg) var(--space-lg)' }
            : { padding: 'var(--space-md) var(--space-lg)' }
        }
      >
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
            className={cn('segmented-tab cursor-pointer', active && 'segmented-tab-active')}
            title={item.hint}
          >
            {item.icon ? <span className="flex-shrink-0 text-current">{item.icon}</span> : null}
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
      <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
        <p className="font-semibold text-white" style={{ fontSize: 'var(--text-sm)' }}>
          {title}
        </p>
        {description ? (
          <p className="mx-auto max-w-xl text-muted" style={{ fontSize: 'var(--text-xs)', lineHeight: '1.6' }}>
            {description}
          </p>
        ) : null}
      </div>
      {action ? <div style={{ paddingTop: 'var(--space-xs)' }}>{action}</div> : null}
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
    info:    'notice-info',
    success: 'notice-success',
    warning: 'notice-warning',
    danger:  'notice-danger',
  }[tone]

  return (
    <div className={toneClass}>
      <div className="flex flex-wrap items-start justify-between" style={{ gap: 'var(--space-sm)' }}>
        <div>
          {title ? (
            <p
              className="font-semibold uppercase opacity-90"
              style={{ fontSize: 'var(--text-xs)', letterSpacing: '0.16em' }}
            >
              {title}
            </p>
          ) : null}
          <div style={{ marginTop: title ? '4px' : 0, fontSize: 'var(--text-sm)', lineHeight: '1.6' }}>
            {children}
          </div>
        </div>
        {action ? <div className="flex-shrink-0">{action}</div> : null}
      </div>
    </div>
  )
}

export function ModalShell({
  title,
  description,
  icon,
  onClose,
  onBeforeClose,
  children,
  size = 'md',
  dismissible = true,
}: {
  title: string
  description?: string
  icon?: ReactNode
  onClose?: () => void
  /** Return false to cancel close (e.g. dirty-form confirmation). Only fires on backdrop click. */
  onBeforeClose?: () => boolean
  children: ReactNode
  size?: 'sm' | 'md' | 'lg' | 'xl'
  dismissible?: boolean
}) {
  const maxWidth = size === 'sm'
    ? 'max-w-sm'
    : size === 'lg'
      ? 'max-w-2xl'
      : size === 'xl'
        ? 'max-w-3xl'
        : 'max-w-md'

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-[var(--space-md)]"
      style={{ background: 'var(--modal-backdrop)', backdropFilter: 'blur(8px)' }}
      onClick={dismissible ? () => { if (onBeforeClose && !onBeforeClose()) return; onClose?.() } : undefined}
    >
      <div
        className={cn('glass-modal w-full overflow-hidden shadow-glass-lg', maxWidth)}
        style={{
          border: '1px solid rgb(var(--border-color) / calc(var(--border-alpha) + 0.04))',
        }}
        onClick={(event) => event.stopPropagation()}
      >
        <div
          style={{
            borderBottom: '1px solid rgb(var(--border-color) / var(--border-alpha))',
            padding: 'var(--space-md) var(--space-lg)',
          }}
        >
          <div className="flex items-start" style={{ gap: 'var(--space-sm)' }}>
            {icon ? (
              <div
                className="flex-shrink-0 text-brand-light"
                style={{
                  marginTop: '2px',
                  borderRadius: 'var(--radius-md)',
                  border: '1px solid rgb(var(--brand) / 0.20)',
                  background: 'rgb(var(--brand-subtle-rgb) / var(--brand-subtle-alpha))',
                  padding: 'var(--space-xs)',
                }}
              >
                {icon}
              </div>
            ) : null}
            <div className="min-w-0 flex-1">
              <h3 className="font-semibold text-white" style={{ fontSize: 'var(--text-lg)' }}>
                {title}
              </h3>
              {description ? (
                <p className="text-muted" style={{ marginTop: '4px', fontSize: 'var(--text-sm)', lineHeight: '1.6' }}>
                  {description}
                </p>
              ) : null}
            </div>
          </div>
        </div>
        <div style={{ padding: 'var(--space-md) var(--space-lg)' }}>{children}</div>
      </div>
    </div>
  )
}
