/** @type {import('tailwindcss').Config} */
/*
 * ClashForge — dual-theme Tailwind config.
 *
 * Colors reference CSS custom properties defined in index.css. The actual
 * values depend on the active theme (`data-theme="neko"` or
 * `data-theme="cyberpunk"`) so the whole palette swaps at runtime without
 * a rebuild.
 *
 * Each color var is stored as a space-separated "R G B" triplet so Tailwind's
 * `<alpha-value>` modifier keeps working (e.g. `bg-brand/20`).
 */
const rgb = (name) => `rgb(var(--color-${name}) / <alpha-value>)`

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        surface: {
          0: rgb('surface-0'),
          1: rgb('surface-1'),
          2: rgb('surface-2'),
          3: rgb('surface-3'),
          4: rgb('surface-4'),
        },
        neon: {
          cyan:    rgb('neon-cyan'),
          magenta: rgb('neon-magenta'),
          yellow:  rgb('neon-yellow'),
          green:   rgb('neon-green'),
          red:     rgb('neon-red'),
          blue:    rgb('neon-blue'),
          violet:  rgb('neon-violet'),
          amber:   rgb('neon-amber'),
        },
        accent: {
          violet:       rgb('accent-violet'),
          'violet-soft':rgb('accent-violet-soft'),
          amber:        rgb('accent-amber'),
          'amber-soft': rgb('accent-amber-soft'),
        },
        brand:          rgb('brand'),
        'brand-light':  rgb('brand-light'),
        'brand-subtle': 'rgb(var(--color-brand) / 0.10)',
        cta:            rgb('cta'),
        'cta-subtle':   'rgb(var(--color-cta) / 0.12)',
        success:        rgb('success'),
        'success-subtle':'rgb(var(--color-success) / 0.12)',
        warning:        rgb('warning'),
        'warning-subtle':'rgb(var(--color-warning) / 0.12)',
        danger:         rgb('danger'),
        'danger-subtle':'rgb(var(--color-danger) / 0.12)',
        muted:          rgb('muted'),
        cream:          rgb('cream'),
        'cream-soft':   rgb('cream-soft'),
        paw:            rgb('paw'),
      },
      fontFamily: {
        sans:    ['Inter', '"Noto Sans SC"', 'system-ui', 'sans-serif'],
        body:    ['Inter', '"Noto Sans SC"', 'system-ui', 'sans-serif'],
        mono:    ['"JetBrains Mono"', '"Fira Code"', 'ui-monospace', 'monospace'],
        heading: ['Nunito', 'Quicksand', 'Inter', 'system-ui', 'sans-serif'],
        display: ['Nunito', 'Quicksand', 'Inter', 'system-ui', 'sans-serif'],
      },
      opacity: { 96: '0.96' },
      backdropBlur: { xs: '2px' },
      boxShadow: {
        'neon-cyan':    'var(--shadow-neon-cyan)',
        'neon-magenta': 'var(--shadow-neon-magenta)',
        'neon-green':   'var(--shadow-neon-green)',
        'neon-yellow':  'var(--shadow-neon-yellow)',
        'neon-red':     'var(--shadow-neon-red)',
        'neon-violet':  'var(--shadow-neon-violet)',
        'neon-amber':   'var(--shadow-neon-amber)',
        glass:    '0 4px 20px rgba(12, 18, 32, 0.35), 0 1px 2px rgba(12,18,32,0.20)',
        'glass-lg':'0 10px 32px rgba(12, 18, 32, 0.40), 0 2px 6px rgba(12,18,32,0.25)',
        hud:      '0 2px 8px rgba(12, 18, 32, 0.25), inset 0 1px 0 rgba(255,255,255,0.04)',
        glow:     'var(--shadow-glow)',
        paw:      'var(--shadow-paw)',
        soft:     '0 1px 2px rgba(12,18,32,0.25), 0 4px 12px rgba(12,18,32,0.20)',
      },
      animation: {
        'fade-in':     'fadeIn 0.25s ease-out',
        'slide-up':    'slideUp 0.25s ease-out',
        'slide-down':  'slideDown 0.25s ease-out',
        'scale-in':    'scaleIn 0.2s ease-out',
        'pulse-soft':  'pulseSoft 2s ease-in-out infinite',
        'shimmer':     'shimmer 2.5s linear infinite',
        'glow-pulse':  'pulseSoft 2.4s ease-in-out infinite',
        'glitch':      'pulseSoft 3s ease-in-out infinite',
        'scanline':    'pulseSoft 4s ease-in-out infinite',
        'flicker':     'pulseSoft 4s ease-in-out infinite',
        'border-flow': 'borderFlow 6s linear infinite',
        'data-stream': 'fadeIn 0.25s ease-out',
        'bounce-soft': 'bounceSoft 0.35s ease-out',
        'tail-wag':    'tailWag 2.4s ease-in-out infinite',
      },
      keyframes: {
        fadeIn:   { '0%': { opacity: '0' }, '100%': { opacity: '1' } },
        slideUp:  { '0%': { opacity: '0', transform: 'translateY(8px)' }, '100%': { opacity: '1', transform: 'translateY(0)' } },
        slideDown:{ '0%': { opacity: '0', transform: 'translateY(-8px)' }, '100%': { opacity: '1', transform: 'translateY(0)' } },
        scaleIn:  { '0%': { opacity: '0', transform: 'scale(0.96)' }, '100%': { opacity: '1', transform: 'scale(1)' } },
        pulseSoft:{ '0%, 100%': { opacity: '1' }, '50%': { opacity: '0.65' } },
        shimmer:  { '0%': { backgroundPosition: '-200% 0' }, '100%': { backgroundPosition: '200% 0' } },
        borderFlow: {
          '0%':   { backgroundPosition: '0% 50%' },
          '50%':  { backgroundPosition: '100% 50%' },
          '100%': { backgroundPosition: '0% 50%' },
        },
        bounceSoft: {
          '0%':   { transform: 'translateY(0) scale(1)' },
          '40%':  { transform: 'translateY(-3px) scale(1.03)' },
          '100%': { transform: 'translateY(0) scale(1)' },
        },
        tailWag: {
          '0%, 100%': { transform: 'rotate(-6deg)' },
          '50%':      { transform: 'rotate(6deg)' },
        },
      },
      borderRadius: {
        sm: '6px',
        DEFAULT: '10px',
        md: '12px',
        lg: '16px',
        xl: '18px',
        '2xl': '20px',
        '3xl': '26px',
      },
    },
  },
  plugins: [],
}
