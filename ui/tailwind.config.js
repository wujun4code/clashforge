/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        surface: {
          0: '#060a12',
          1: '#0b1120',
          2: '#101829',
          3: '#182035',
        },
        muted: '#3d5070',
        brand: '#06b6d4',
        'brand-dim': '#0891b2',
        'brand-bright': '#67e8f9',
        accent: '#6366f1',
        success: '#10b981',
        warning: '#f59e0b',
        danger: '#f43f5e',
      },
      fontFamily: {
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
        sans: ['Inter', 'system-ui', 'sans-serif'],
      },
      boxShadow: {
        'glow-brand': '0 0 24px rgba(6,182,212,0.3), 0 0 48px rgba(6,182,212,0.08)',
        'glow-brand-sm': '0 0 12px rgba(6,182,212,0.25)',
        'glow-success': '0 0 16px rgba(16,185,129,0.3)',
        'glow-danger': '0 0 16px rgba(244,63,94,0.3)',
        'glow-warning': '0 0 16px rgba(245,158,11,0.3)',
        'card': '0 2px 8px rgba(0,0,0,0.6), inset 0 1px 0 rgba(255,255,255,0.03)',
        'card-active': '0 0 0 1px rgba(6,182,212,0.4), 0 4px 20px rgba(0,0,0,0.5)',
      },
      animation: {
        'glow-pulse': 'glowPulse 2.5s ease-in-out infinite',
        'slide-in': 'slideIn 0.18s ease-out',
        'fade-in': 'fadeIn 0.15s ease-out',
        'float': 'float 4s ease-in-out infinite',
      },
      keyframes: {
        glowPulse: {
          '0%, 100%': { opacity: '1', filter: 'brightness(1)' },
          '50%': { opacity: '0.6', filter: 'brightness(0.8)' },
        },
        slideIn: {
          from: { opacity: '0', transform: 'translateY(-6px)' },
          to: { opacity: '1', transform: 'translateY(0)' },
        },
        fadeIn: {
          from: { opacity: '0' },
          to: { opacity: '1' },
        },
        float: {
          '0%, 100%': { transform: 'translateY(0)' },
          '50%': { transform: 'translateY(-4px)' },
        },
      },
    },
  },
  plugins: [],
}
