import { Check, ChevronDown } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { applyTheme, loadTheme, persistTheme, THEMES, type ThemeId } from './themes'

export function ThemeSwitcher() {
  const [themeId, setThemeId] = useState<ThemeId>(() => loadTheme())
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)

  const currentTheme = useMemo(
    () => THEMES.find((theme) => theme.id === themeId) ?? THEMES[0],
    [themeId]
  )
  const CurrentIcon = currentTheme.icon

  useEffect(() => {
    applyTheme(themeId)
  }, [themeId])

  useEffect(() => {
    if (!open) return

    const handleClickOutside = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false)
      }
    }
    const handleEsc = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }

    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleEsc)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleEsc)
    }
  }, [open])

  const selectTheme = (id: ThemeId) => {
    setThemeId(id)
    persistTheme(id)
    applyTheme(id)
    setOpen(false)
  }

  return (
    <div
      ref={rootRef}
      className={`theme-switcher ${open ? 'theme-switcher-open' : ''}`}
    >
      <button
        type="button"
        className="theme-switcher-trigger"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label="切换界面皮肤"
        onClick={() => setOpen((prev) => !prev)}
        style={{ '--theme-preview': currentTheme.preview, '--theme-accent': currentTheme.accent } as CSSProperties}
      >
        <span className="theme-switcher-swatch" aria-hidden />
        <CurrentIcon size={13} className="theme-switcher-icon" aria-hidden />
        <span className="theme-switcher-copy">
          <span className="theme-switcher-name">{currentTheme.name}</span>
          <span className="theme-switcher-description">{currentTheme.description}</span>
        </span>
        <ChevronDown size={13} className="theme-switcher-caret" aria-hidden />
      </button>

      <div
        className={`theme-switcher-menu ${open ? 'theme-switcher-menu-open' : ''}`}
        role="listbox"
        aria-label="界面皮肤列表"
      >
        {THEMES.map((theme) => {
          const Icon = theme.icon
          const active = theme.id === themeId

          return (
            <button
              key={theme.id}
              type="button"
              role="option"
              aria-selected={active}
              aria-label={`${theme.name}皮肤`}
              className={`theme-switcher-option ${active ? 'theme-switcher-option-active' : ''}`}
              onClick={() => selectTheme(theme.id)}
              style={{ '--theme-preview': theme.preview, '--theme-accent': theme.accent } as CSSProperties}
              title={theme.description}
            >
              <span className="theme-switcher-swatch" aria-hidden />
              <Icon size={13} className="theme-switcher-icon" aria-hidden />
              <span className="theme-switcher-copy">
                <span className="theme-switcher-name">{theme.name}</span>
                <span className="theme-switcher-description">{theme.shortName}</span>
              </span>
              {active ? <Check size={12} className="theme-switcher-check" aria-hidden /> : null}
            </button>
          )
        })}
      </div>
    </div>
  )
}
