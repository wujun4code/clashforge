import { useState } from 'react'
import { ChevronUp } from 'lucide-react'
import { THEMES, type ThemeId, applyTheme, loadTheme, persistTheme } from './themes'

export function ThemeSwitcher() {
  const [theme, setTheme] = useState<ThemeId>(loadTheme)
  const [open, setOpen] = useState(false)

  const active = THEMES.find((t) => t.id === theme) ?? THEMES[0]

  const handleSelect = (id: ThemeId) => {
    setTheme(id)
    applyTheme(id)
    persistTheme(id)
    setOpen(false)
  }

  return (
    <div className="relative">
      {/* Trigger */}
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="glass-card flex w-full cursor-pointer items-center justify-between px-[var(--space-md)] py-[var(--space-sm)] transition-all"
        style={{ borderRadius: 'var(--radius-lg)' }}
      >
        <div className="flex items-center gap-[var(--space-sm)]">
          <div className="flex items-center gap-[var(--space-xs)]">
            <span
              className="block h-3 w-3 flex-shrink-0 shadow-sm"
              style={{ background: active.preview, borderRadius: 'var(--radius-full)' }}
            />
            <active.icon size={13} className="text-muted" />
          </div>
          <span style={{ fontSize: 'var(--text-xs)', fontFamily: 'var(--font-sans)' }} className="font-medium text-muted">
            {active.name}
          </span>
        </div>
        <ChevronUp
          size={13}
          className={`text-muted transition-transform duration-200 ${open ? 'rotate-0' : 'rotate-180'}`}
        />
      </button>

      {/* Dropdown */}
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div
            className="absolute bottom-full left-0 right-0 z-50 mb-2 animate-scale-in border bg-surface-2/95 p-[var(--space-xs)] shadow-glass-lg"
            style={{
              borderRadius: 'var(--radius-xl)',
              borderColor: 'rgb(var(--border-color) / var(--border-alpha))',
              backdropFilter: 'blur(24px)',
            }}
          >
            {THEMES.map((t) => {
              const isActive = t.id === theme
              const Icon = t.icon
              return (
                <button
                  key={t.id}
                  type="button"
                  onClick={() => handleSelect(t.id)}
                  className={`flex w-full cursor-pointer items-center gap-[var(--space-sm)] text-left transition-all ${
                    isActive
                      ? 'btn-ghost-active'
                      : 'text-muted hover:bg-white/[0.05] hover:text-white'
                  }`}
                  style={{
                    borderRadius: 'var(--radius-md)',
                    padding: 'var(--space-sm) var(--space-sm)',
                  }}
                >
                  <span
                    className="block h-4 w-4 flex-shrink-0 shadow-sm"
                    style={{ background: t.preview, borderRadius: 'var(--radius-full)' }}
                  />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-[var(--space-xs)]">
                      <Icon size={12} className={isActive ? 'text-brand-light' : 'text-muted'} />
                      <span style={{ fontSize: 'var(--text-xs)' }} className="font-semibold">
                        {t.name}
                      </span>
                    </div>
                    <p className="mt-0.5 leading-tight text-muted" style={{ fontSize: '0.65rem' }}>
                      {t.description}
                    </p>
                  </div>
                  {isActive && (
                    <span
                      className="ml-auto h-1.5 w-1.5 bg-brand-light flex-shrink-0"
                      style={{ borderRadius: 'var(--radius-full)' }}
                    />
                  )}
                </button>
              )
            })}
          </div>
        </>
      )}
    </div>
  )
}
