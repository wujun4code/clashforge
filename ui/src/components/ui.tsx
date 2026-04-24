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
  const metricGlow: Record<string, string> = {
    cyan:    '0 0 8px rgba(0,245,255,0.5)',
    green:   '0 0 8px rgba(0,255,136,0.5)',
    yellow:  '0 0 8px rgba(255,230,0,0.5)',
    magenta: '0 0 8px rgba(255,0,170,0.5)',
    red:     '0 0 8px rgba(255,34,85,0.5)',
  }
  const metricColor: Record<string, string> = {
    cyan:    '#00F5FF',
    green:   '#00FF88',
    yellow:  '#FFE600',
    magenta: '#FF00AA',
    red:     '#FF2255',
  }

  return (
    <section className="hero-panel relative overflow-hidden animate-fade-in">
      {/* Ambient gradient */}
      <div className="pointer-events-none absolute inset-0"
        style={{ background: 'radial-gradient(circle at top right, rgba(0,245,255,0.08) 0%, transparent 40%), radial-gradient(circle at bottom left, rgba(255,0,170,0.06) 0%, transparent 40%)' }} />

      <div className="relative flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
        <div className="max-w-3xl space-y-3">
          {/* Eyebrow tag */}
          <div
            className="inline-flex items-center gap-2 px-3 py-1 text-[10px] font-semibold uppercase tracking-[0.3em] text-neon-cyan"
            style={{ border: '1px solid rgba(0,245,255,0.25)', background: 'rgba(0,245,255,0.06)' }}
          >
            <Terminal size={10} />
            {eyebrow}
          </div>

          <div className="space-y-2">
            <h1
              className="font-mono text-2xl font-bold tracking-[0.04em] text-display-gradient md:text-3xl"
            >
              {title}
            </h1>
            {description ? (
              <p className="font-mono max-w-2xl text-xs leading-6 text-muted md:text-sm">
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
                    className="px-3 py-2.5 backdrop-blur-xl"
                    style={{
                      border: '1px solid rgba(0,245,255,0.12)',
                      background: 'rgba(0,245,255,0.03)',
                    }}
                  >
                    <p className="font-mono text-[9px] uppercase tracking-[0.2em] text-muted">{m.label}</p>
                    <p
                      className="font-mono mt-1.5 text-sm font-bold"
                      style={{ color: metricColor[c], textShadow: metricGlow[c] }}
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
  accent = 'cyan',
}: {
  title?: string
  description?: string
  actions?: ReactNode
  children: ReactNode
  className?: string
  accent?: 'cyan' | 'magenta' | 'yellow' | 'green' | 'red'
}) {
  const accentColor: Record<string, string> = {
    cyan:    'rgba(0,245,255,0.25)',
    magenta: 'rgba(255,0,170,0.25)',
    yellow:  'rgba(255,230,0,0.25)',
    green:   'rgba(0,255,136,0.25)',
    red:     'rgba(255,34,85,0.25)',
  }

  return (
    <section className={cn('glass-card glass-section', className)}>
      {title || description || actions ? (
        <div className="panel-header">
          <div>
            {title ? (
              <h2
                className="font-mono text-sm font-semibold uppercase tracking-[0.1em] text-white"
                style={{ textShadow: `0 0 8px ${accentColor[accent]}` }}
              >
                <span className="text-neon-cyan/50 mr-1">{'>'}</span>{title}
              </h2>
            ) : null}
            {description ? (
              <p className="font-mono mt-1 text-xs leading-6 text-muted">{description}</p>
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
      <div className="empty-state-icon">{icon ?? <Terminal size={18} />}</div>
      <div className="space-y-1.5">
        <p className="font-mono text-sm font-semibold uppercase tracking-[0.08em] text-white">{title}</p>
        {description ? (
          <p className="font-mono mx-auto max-w-xl text-xs leading-6 text-muted">{description}</p>
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

  const prefix = {
    info:    '[INFO]',
    success: '[OK]',
    warning: '[WARN]',
    danger:  '[ERR]',
  }[tone]

  return (
    <div className={toneClass}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          {title ? (
            <p className="font-mono text-[10px] font-bold uppercase tracking-[0.2em] opacity-90">
              {prefix} {title}
            </p>
          ) : null}
          <div className="font-mono mt-1 text-xs leading-6">{children}</div>
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
      style={{ background: 'rgba(2,4,8,0.85)' }}
      onClick={dismissible ? onClose : undefined}
    >
      {/* Scanline overlay on backdrop */}
      <div
        className="pointer-events-none absolute inset-0"
        style={{
          backgroundImage: 'repeating-linear-gradient(0deg, transparent, transparent 2px, rgba(0,245,255,0.02) 2px, rgba(0,245,255,0.02) 4px)',
        }}
      />

      <div
        className={cn('glass-modal w-full overflow-hidden animate-scale-in', maxWidth)}
        style={{
          clipPath: 'polygon(0 0, calc(100% - 14px) 0, 100% 14px, 100% 100%, 14px 100%, 0 calc(100% - 14px))',
          boxShadow: '0 0 40px rgba(0,245,255,0.1), 0 0 80px rgba(0,245,255,0.05)',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Modal header */}
        <div
          className="flex items-start gap-3 px-5 py-4 md:px-6"
          style={{ borderBottom: '1px solid rgba(0,245,255,0.10)' }}
        >
          {icon ? (
            <div
              className="mt-0.5 flex-shrink-0 p-2 text-neon-cyan"
              style={{ border: '1px solid rgba(0,245,255,0.25)', background: 'rgba(0,245,255,0.06)' }}
            >
              {icon}
            </div>
          ) : null}
          <div className="min-w-0 flex-1">
            <h3
              className="font-mono text-base font-bold uppercase tracking-[0.08em] text-white"
              style={{ textShadow: '0 0 10px rgba(0,245,255,0.4)' }}
            >
              {title}
            </h3>
            {description ? (
              <p className="font-mono mt-1 text-xs leading-5 text-muted">{description}</p>
            ) : null}
          </div>
          {dismissible && onClose ? (
            <button
              type="button"
              onClick={onClose}
              className="flex-shrink-0 p-1.5 text-muted hover:text-neon-cyan transition-colors cursor-pointer"
              aria-label="关闭"
            >
              <X size={14} />
            </button>
          ) : null}
        </div>

        {/* Top-right corner accent */}
        <div
          className="pointer-events-none absolute top-0 right-0"
          style={{ width: 14, height: 14, background: 'linear-gradient(225deg, rgba(0,245,255,0.5) 0%, transparent 60%)' }}
        />

        <div className="px-5 py-5 md:px-6">{children}</div>
      </div>
    </div>
  )
}
