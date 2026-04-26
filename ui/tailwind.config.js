/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        surface: {
          0: 'rgb(var(--surface-0) / <alpha-value>)',
          1: 'rgb(var(--surface-1) / <alpha-value>)',
          2: 'rgb(var(--surface-2) / <alpha-value>)',
          3: 'rgb(var(--surface-3) / <alpha-value>)',
          4: 'rgb(var(--surface-4) / <alpha-value>)',
        },
        muted: 'rgb(var(--muted) / <alpha-value>)',
        brand: 'rgb(var(--brand) / <alpha-value>)',
        'brand-light': 'rgb(var(--brand-light) / <alpha-value>)',
        'brand-subtle': 'rgb(var(--brand-subtle-rgb) / var(--brand-subtle-alpha))',
        cta: 'rgb(var(--cta) / <alpha-value>)',
        'cta-subtle': 'rgb(var(--cta-subtle-rgb) / var(--cta-subtle-alpha))',
        success: 'rgb(var(--success) / <alpha-value>)',
        'success-subtle': 'rgb(var(--success-subtle-rgb) / var(--success-subtle-alpha))',
        warning: 'rgb(var(--warning) / <alpha-value>)',
        'warning-subtle': 'rgb(var(--warning-subtle-rgb) / var(--warning-subtle-alpha))',
        danger: 'rgb(var(--danger) / <alpha-value>)',
        'danger-subtle': 'rgb(var(--danger-subtle-rgb) / var(--danger-subtle-alpha))',
      },
      fontFamily: {
        sans: ['var(--font-sans)', 'system-ui', 'sans-serif'],
        mono: ['var(--font-mono)', 'JetBrains Mono', 'monospace'],
        heading: ['var(--font-heading)', 'monospace'],
      },
      spacing: {
        'unit-xs':  'var(--space-xs)',
        'unit-sm':  'var(--space-sm)',
        'unit-md':  'var(--space-md)',
        'unit-lg':  'var(--space-lg)',
        'unit-xl':  'var(--space-xl)',
        'unit-2xl': 'var(--space-2xl)',
      },
      fontSize: {
        'theme-xs':   'var(--text-xs)',
        'theme-sm':   'var(--text-sm)',
        'theme-base': 'var(--text-base)',
        'theme-lg':   'var(--text-lg)',
        'theme-xl':   'var(--text-xl)',
        'theme-2xl':  'var(--text-2xl)',
        'theme-3xl':  'var(--text-3xl)',
      },
      borderRadius: {
        'theme-sm':  'var(--radius-sm)',
        'theme-md':  'var(--radius-md)',
        'theme-lg':  'var(--radius-lg)',
        'theme-xl':  'var(--radius-xl)',
        'theme-full': 'var(--radius-full)',
      },
      backdropBlur: {
        xs: '2px',
      },
      boxShadow: {
        glass: 'var(--shadow-card)',
        'glass-lg': 'var(--shadow-card-hover)',
        glow: 'var(--shadow-glow-brand)',
        'glow-orange': 'var(--shadow-glow-cta)',
        'glow-success': '0 0 20px rgb(var(--success) / 0.2)',
      },
      transitionTimingFunction: {
        bounce: 'var(--ease-bounce)',
        snap: 'var(--ease-snap)',
        instant: 'var(--ease-instant)',
        theme: 'var(--ease-default)',
      },
      transitionDuration: {
        'theme-fast': 'var(--dur-fast)',
        'theme-normal': 'var(--dur-normal)',
        'theme-slow': 'var(--dur-slow)',
      },
      animation: {
        'fade-in': 'fadeIn var(--dur-normal) var(--ease-default)',
        'slide-up': 'slideUp var(--dur-normal) var(--ease-default)',
        'slide-down': 'slideDown var(--dur-normal) var(--ease-default)',
        'scale-in': 'scaleIn var(--dur-fast) var(--ease-default)',
        'pulse-soft': 'pulseSoft 2s ease-in-out infinite',
        'shimmer': 'shimmer 2s linear infinite',
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
          '0%, 100%': { boxShadow: 'var(--shadow-glow-brand)' },
          '50%': { boxShadow: 'var(--shadow-glow-cta)' },
        },
      },
    },
  },
  plugins: [],
}
