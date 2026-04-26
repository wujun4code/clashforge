import { useEffect } from 'react'
import { type ThemeId, loadTheme, applyTheme } from './themes'

/** On mount: read persisted theme from localStorage, apply to <html>. */
export function ThemeProvider({ children }: { children: React.ReactNode }) {
  useEffect(() => {
    const theme: ThemeId = loadTheme()
    applyTheme(theme)
  }, [])

  return <>{children}</>
}
