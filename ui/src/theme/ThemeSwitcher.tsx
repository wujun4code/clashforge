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
        onClick={() => setOpen(!open)}
        className="flex w-full items-center justify-between rounded-2xl border border-white/[0.08] bg-white/[0.03] px-4 py-3 transition-all duration-200 hover:border-white/[0.12] hover:bg-white/[0.05]"
      >
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5">
            <span
              className="block h-3.5 w-3.5 rounded-full shadow-sm"
              style={{ background: active.preview }}
            />
            <active.icon size={14} className="text-muted" />
          </div>
          <span className="text-xs font-medium text-muted">{active.name}</span>
        </div>
        <ChevronUp
          size={14}
          className={`text-muted transition-transform duration-200 ${open ? 'rotate-0' : 'rotate-180'}`}
        />
      </button>

      {/* Dropdown */}
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="absolute bottom-full left-0 right-0 z-50 mb-2 animate-scale-in rounded-2xl border border-white/[0.08] bg-surface-2/95 p-2 shadow-glass-lg backdrop-blur-2xl">
            {THEMES.map((t) => {
              const isActive = t.id === theme
              const Icon = t.icon
              return (
                <button
                  key={t.id}
                  onClick={() => handleSelect(t.id)}
                  className={`flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left transition-all duration-150 ${
                    isActive
                      ? 'bg-brand-subtle text-white'
                      : 'text-muted hover:bg-white/[0.05] hover:text-white'
                  }`}
                >
                  <span
                    className="block h-4 w-4 flex-shrink-0 rounded-full shadow-sm"
                    style={{ background: t.preview }}
                  />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5">
                      <Icon size={13} className={isActive ? 'text-brand-light' : 'text-muted'} />
                      <span className="text-xs font-semibold">{t.name}</span>
                    </div>
                    <p className="mt-0.5 text-[10px] leading-tight text-muted">
                      {t.description}
                    </p>
                  </div>
                  {isActive && (
                    <span className="ml-auto h-1.5 w-1.5 rounded-full bg-brand-light" />
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
