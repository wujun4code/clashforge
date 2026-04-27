import { Cat, Gem, Network, type LucideIcon } from 'lucide-react'

export type ThemeId = 'nebula' | 'meow' | 'infra'

export interface ThemeMeta {
  id: ThemeId
  name: string
  shortName: string
  icon: LucideIcon
  description: string
  preview: string
  accent: string
}

export const THEMES: ThemeMeta[] = [
  {
    id: 'nebula',
    name: '星云',
    shortName: 'Nebula',
    icon: Gem,
    description: '玻璃拟态 · 暗黑紫晶',
    preview: 'linear-gradient(135deg, #8B5CF6 0%, #060517 100%)',
    accent: '#8B5CF6',
  },
  {
    id: 'meow',
    name: '喵云',
    shortName: 'Meow',
    icon: Cat,
    description: '萌系猫猫 · 糖霜夜色',
    preview: 'linear-gradient(135deg, #FF8AC9 0%, #132C2C 58%, #FFE9A8 100%)',
    accent: '#FF8AC9',
  },
  {
    id: 'infra',
    name: '中枢',
    shortName: 'Nexus',
    icon: Network,
    description: '商务专业 · 网络中枢',
    preview: 'linear-gradient(135deg, #38BDF8 0%, #0F172A 48%, #14B8A6 100%)',
    accent: '#38BDF8',
  },
]

export const DEFAULT_THEME: ThemeId = 'nebula'
export const THEME_STORAGE_KEY = 'clashforge.theme'

const THEME_IDS = new Set<ThemeId>(THEMES.map((theme) => theme.id))

export function isThemeId(value: string | null | undefined): value is ThemeId {
  return !!value && THEME_IDS.has(value as ThemeId)
}

export function loadTheme(): ThemeId {
  if (typeof window === 'undefined') return DEFAULT_THEME
  const saved = window.localStorage.getItem(THEME_STORAGE_KEY)
  return isThemeId(saved) ? saved : DEFAULT_THEME
}

export function persistTheme(id: ThemeId): void {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(THEME_STORAGE_KEY, id)
}

export function applyTheme(id: ThemeId): void {
  document.documentElement.setAttribute('data-theme', id)
}
