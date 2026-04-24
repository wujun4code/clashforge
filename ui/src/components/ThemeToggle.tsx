import { useTheme, type ThemeName } from '../contexts/ThemeContext'

const options: Array<{ id: ThemeName; icon: string; label: string; title: string }> = [
  { id: 'neko',      icon: '🐱', label: 'Neko',  title: '猫咪治愈模式' },
  { id: 'cyberpunk', icon: '🌐', label: 'Cyber', title: '赛博朋克模式' },
]

export function ThemeToggle({ compact = false }: { compact?: boolean }) {
  const { theme, setTheme } = useTheme()

  return (
    <div
      className="theme-toggle"
      role="radiogroup"
      aria-label="界面皮肤"
    >
      {options.map((opt) => {
        const active = theme === opt.id
        return (
          <button
            key={opt.id}
            type="button"
            role="radio"
            aria-checked={active}
            title={opt.title}
            onClick={() => setTheme(opt.id)}
            className={[
              'theme-toggle-option',
              active ? 'theme-toggle-option-active' : '',
            ].join(' ')}
          >
            <span aria-hidden className="text-sm leading-none">{opt.icon}</span>
            {!compact && <span>{opt.label}</span>}
          </button>
        )
      })}
    </div>
  )
}
