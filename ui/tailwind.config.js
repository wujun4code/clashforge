/** @type {import('tailwindcss').Config} */
/*
 * ClashForge — dual-theme Tailwind config.
 *
 * Color tokens reference CSS custom properties defined in index.css, so
 * the whole palette swaps at runtime when `data-theme` flips between
 * `original` (main branch palette) and `neko` (cat-themed soft warm).
 *
 * Each --color-* var stores a space-separated "R G B" triplet so Tailwind's
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
        muted:           rgb('muted'),
        brand:           rgb('brand'),
        'brand-light':   rgb('brand-light'),
        'brand-subtle':  'rgb(var(--color-brand) / 0.12)',
        cta:             rgb('cta'),
        'cta-subtle':    'rgb(var(--color-cta) / 0.12)',
        success:         rgb('success'),
        'success-subtle':'rgb(var(--color-success) / 0.12)',
        warning:         rgb('warning'),
        'warning-subtle':'rgb(var(--color-warning) / 0.12)',
        danger:          rgb('danger'),
        'danger-subtle': 'rgb(var(--color-danger) / 0.12)',
        accent: {
          violet:        rgb('accent-violet'),
          'violet-soft': rgb('accent-violet-soft'),
          amber:         rgb('accent-amber'),
          'amber-soft':  rgb('accent-amber-soft'),
        },
        cream:        rgb('cream'),
        'cream-soft': rgb('cream-soft'),
        paw:          rgb('paw'),
      },
      fontFamily: {
        sans:    ['Fira Sans', 'Inter', '"Noto Sans SC"', 'system-ui', 'sans-serif'],
        mono:    ['Fira Code', '"JetBrains Mono"', 'monospace'],
        heading: ['Fira Code', 'Nunito', 'monospace'],
      },
      backdropBlur: {
        xs: '2px',
      },
      boxShadow: {
        glass:         'var(--shadow-glass)',
        'glass-lg':    'var(--shadow-glass-lg)',
        glow:          'var(--shadow-glow)',
        'glow-orange': 'var(--shadow-glow-orange)',
        'glow-green':  'var(--shadow-glow-green)',
      },
      animation: {
        'fade-in':    'fadeIn 0.3s ease-out',
        'slide-up':   'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'scale-in':   'scaleIn 0.2s ease-out',
        'pulse-soft': 'pulseSoft 2s ease-in-out infinite',
        'shimmer':    'shimmer 2s linear infinite',
        'glow-pulse': 'glowPulse 2s ease-in-out infinite',
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(8px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-8px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' },
        },
        pulseSoft: {
          '0%, 100%': { opacity: '1' },
          '50%': { opacity: '0.7' },
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' },
        },
        glowPulse: {
          '0%, 100%': { boxShadow: '0 0 10px rgba(124, 58, 237, 0.15)' },
          '50%': { boxShadow: '0 0 25px rgba(124, 58, 237, 0.30)' },
        },
      },
      borderRadius: {
        '2xl': '16px',
        '3xl': '20px',
      },
    },
  },
  plugins: [],
}
