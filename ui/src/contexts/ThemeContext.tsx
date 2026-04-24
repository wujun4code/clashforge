import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'

export type ThemeName = 'neko' | 'cyberpunk'

const STORAGE_KEY = 'clashforge:theme'
const DEFAULT_THEME: ThemeName = 'neko'

interface ThemeContextValue {
  theme: ThemeName
  setTheme: (theme: ThemeName) => void
  toggleTheme: () => void
}

const ThemeContext = createContext<ThemeContextValue | undefined>(undefined)

function readInitialTheme(): ThemeName {
  if (typeof window === 'undefined') return DEFAULT_THEME
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY)
    if (stored === 'neko' || stored === 'cyberpunk') return stored
  } catch {
    /* ignore */
  }
  return DEFAULT_THEME
}

function applyThemeToDocument(theme: ThemeName, withTransition = true) {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  if (withTransition) {
    root.classList.add('theme-transition')
    window.setTimeout(() => root.classList.remove('theme-transition'), 320)
  }
  root.dataset.theme = theme
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemeName>(readInitialTheme)

  // Apply initial theme synchronously on mount (no transition flash).
  useEffect(() => {
    applyThemeToDocument(theme, false)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const setTheme = useCallback((next: ThemeName) => {
    setThemeState((prev) => {
      if (prev === next) return prev
      applyThemeToDocument(next, true)
      try {
        window.localStorage.setItem(STORAGE_KEY, next)
      } catch {
        /* ignore */
      }
      return next
    })
  }, [])

  const toggleTheme = useCallback(() => {
    setTheme(theme === 'neko' ? 'cyberpunk' : 'neko')
  }, [theme, setTheme])

  const value = useMemo<ThemeContextValue>(
    () => ({ theme, setTheme, toggleTheme }),
    [theme, setTheme, toggleTheme],
  )

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme must be used inside <ThemeProvider>')
  return ctx
}
