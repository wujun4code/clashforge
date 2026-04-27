import { Gem, type LucideIcon } from 'lucide-react'

export type ThemeId = 'nebula'

export interface ThemeMeta {
  id: ThemeId
  name: string
  icon: LucideIcon
  description: string
  preview: string
}

export const THEMES: ThemeMeta[] = [
  {
    id: 'nebula',
    name: '星云',
    icon: Gem,
    description: '玻璃拟态 · 暗黑紫晶',
    preview: 'linear-gradient(135deg, #8B5CF6 0%, #060517 100%)',
  },
]

export const DEFAULT_THEME: ThemeId = 'nebula'

export function loadTheme(): ThemeId {
  return 'nebula'
}

export function persistTheme(_id: ThemeId): void {}

export function applyTheme(_id: ThemeId): void {
  document.documentElement.setAttribute('data-theme', 'nebula')
}
