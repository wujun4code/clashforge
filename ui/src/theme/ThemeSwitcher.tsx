import { Gem } from 'lucide-react'

export function ThemeSwitcher() {
  return (
    <div
      className="flex w-full items-center gap-[var(--space-sm)] border bg-white/[0.025] cursor-default"
      style={{
        borderRadius: 'var(--radius-md)',
        borderColor: 'rgb(var(--border-color) / var(--border-alpha))',
        padding: 'var(--space-xs) var(--space-sm)',
      }}
    >
      <span
        className="block h-3 w-3 flex-shrink-0"
        style={{
          background: 'linear-gradient(135deg, #8B5CF6 0%, #4C1D95 100%)',
          borderRadius: 'var(--radius-full)',
          boxShadow: '0 0 8px rgba(139,92,246,0.60)',
        }}
      />
      <Gem size={12} className="text-brand-light flex-shrink-0" />
      <span
        className="font-medium text-muted"
        style={{ fontSize: 'var(--text-xs)', fontFamily: 'var(--font-sans)' }}
      >
        星云
      </span>
      <span
        className="ml-auto font-mono tracking-widest text-brand-light/40"
        style={{ fontSize: '0.65rem', letterSpacing: '0.18em' }}
      >
        NEBULA
      </span>
    </div>
  )
}
