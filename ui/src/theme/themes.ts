import { Cat, Gem, BriefcaseBusiness, type LucideIcon } from 'lucide-react'

export type ThemeId = 'nebula' | 'clashcat' | 'enterprise'

export interface ThemeMeta {
  id: ThemeId
  name: string
  icon: LucideIcon
  description: string
  /** Small preview: CSS gradient string */
  preview: string
}

export const THEMES: ThemeMeta[] = [
  {
    id: 'nebula',
    name: '星云',
    icon: Gem,
    description: '玻璃拟态 · 暗黑紫晶',
    preview: 'linear-gradient(135deg, #7C3AED 0%, #0B0A1A 100%)',
  },
  {
    id: 'clashcat',
    name: '萌猫',
    icon: Cat,
    description: '火焰橘 · 俏皮暖棕',
    preview: 'linear-gradient(135deg, #F97316 0%, #1C1917 100%)',
  },
  {
    id: 'enterprise',
    name: '商务',
    icon: BriefcaseBusiness,
    description: '深蓝 · 专业克制',
    preview: 'linear-gradient(135deg, #2563EB 0%, #0F172A 100%)',
  },
]

export const DEFAULT_THEME: ThemeId = 'nebula'

const STORAGE_KEY = 'clashforge-theme'

export function loadTheme(): ThemeId {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === 'nebula' || stored === 'clashcat' || stored === 'enterprise') {
      return stored
    }
  } catch { /* localStorage unavailable */ }
  return DEFAULT_THEME
}

export function persistTheme(id: ThemeId): void {
  try {
    localStorage.setItem(STORAGE_KEY, id)
  } catch { /* noop */ }
}

export function applyTheme(id: ThemeId): void {
  document.documentElement.setAttribute('data-theme', id)
}
