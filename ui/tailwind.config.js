/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        // Cyberpunk surface palette — near-black with deep blue tint
        // Using Tailwind opacity-aware format so `bg-surface-0/96` etc. work in @apply
        surface: {
          0: 'rgb(2 4 8 / <alpha-value>)',
          1: 'rgb(6 12 18 / <alpha-value>)',
          2: 'rgb(10 18 32 / <alpha-value>)',
          3: 'rgb(14 26 46 / <alpha-value>)',
          4: 'rgb(18 36 60 / <alpha-value>)',
        },
        // Neon accent system
        neon: {
          cyan:    '#00F5FF',
          magenta: '#FF00AA',
          yellow:  '#FFE600',
          green:   '#00FF88',
          red:     '#FF2255',
          blue:    '#0080FF',
          violet:  '#A78BFA',
          amber:   '#F97316',
        },
        // Design-system accents (purple + orange pairing)
        accent: {
          violet:       '#7C3AED',
          'violet-soft':'#A78BFA',
          amber:        '#F97316',
          'amber-soft': '#FDBA74',
        },
        // Semantic mappings — point to neon variants
        brand:          '#00F5FF',   // neon cyan as primary brand
        'brand-light':  '#7FEFFF',
        'brand-subtle': 'rgba(0, 245, 255, 0.08)',
        cta:            '#FF00AA',   // neon magenta as CTA
        'cta-subtle':   'rgba(255, 0, 170, 0.10)',
        success:        '#00FF88',
        'success-subtle':'rgba(0, 255, 136, 0.10)',
        warning:        '#FFE600',
        'warning-subtle':'rgba(255, 230, 0, 0.10)',
        danger:         '#FF2255',
        'danger-subtle':'rgba(255, 34, 85, 0.10)',
        muted:          '#4A6080',
      },
      fontFamily: {
        // Body uses Fira Sans (readable), headings & code use Fira Code (technical)
        sans:    ['"Fira Sans"', 'Inter', 'system-ui', 'sans-serif'],
        body:    ['"Fira Sans"', 'Inter', 'system-ui', 'sans-serif'],
        mono:    ['"Fira Code"', 'JetBrains Mono', 'monospace'],
        heading: ['"Fira Code"', 'JetBrains Mono', 'monospace'],
      },
      opacity: { 96: '0.96' },
      backdropBlur: { xs: '2px' },
      boxShadow: {
        'neon-cyan':    '0 0 8px #00F5FF, 0 0 24px rgba(0,245,255,0.4)',
        'neon-magenta': '0 0 8px #FF00AA, 0 0 24px rgba(255,0,170,0.4)',
        'neon-green':   '0 0 8px #00FF88, 0 0 24px rgba(0,255,136,0.4)',
        'neon-yellow':  '0 0 8px #FFE600, 0 0 24px rgba(255,230,0,0.4)',
        'neon-red':     '0 0 8px #FF2255, 0 0 24px rgba(255,34,85,0.4)',
        'neon-violet':  '0 0 8px #A78BFA, 0 0 24px rgba(167,139,250,0.4)',
        'neon-amber':   '0 0 8px #F97316, 0 0 24px rgba(249,115,22,0.4)',
        glass:    '0 4px 24px rgba(0, 0, 0, 0.6), inset 0 1px 0 rgba(0,245,255,0.06)',
        'glass-lg':'0 8px 40px rgba(0, 0, 0, 0.7), inset 0 1px 0 rgba(0,245,255,0.08)',
        hud:      'inset 0 0 30px rgba(0,245,255,0.03), 0 0 0 1px rgba(0,245,255,0.15)',
        glow:     '0 0 12px rgba(0, 245, 255, 0.5), 0 0 24px rgba(0, 245, 255, 0.25)',
      },
      animation: {
        'fade-in':     'fadeIn 0.25s ease-out',
        'slide-up':    'slideUp 0.25s ease-out',
        'slide-down':  'slideDown 0.25s ease-out',
        'scale-in':    'scaleIn 0.2s ease-out',
        'pulse-soft':  'pulseSoft 2s ease-in-out infinite',
        'shimmer':     'shimmer 2.5s linear infinite',
        'glow-pulse':  'glowPulse 2s ease-in-out infinite',
        'glitch':      'glitch 4s steps(1) infinite',
        'scanline':    'scanline 8s linear infinite',
        'flicker':     'flicker 6s steps(1) infinite',
        'border-flow': 'borderFlow 3s linear infinite',
        'data-stream': 'dataStream 1.5s ease-out',
      },
      keyframes: {
        fadeIn:   { '0%': { opacity: '0' }, '100%': { opacity: '1' } },
        slideUp:  { '0%': { opacity: '0', transform: 'translateY(10px)' }, '100%': { opacity: '1', transform: 'translateY(0)' } },
        slideDown:{ '0%': { opacity: '0', transform: 'translateY(-10px)' }, '100%': { opacity: '1', transform: 'translateY(0)' } },
        scaleIn:  { '0%': { opacity: '0', transform: 'scale(0.94)' }, '100%': { opacity: '1', transform: 'scale(1)' } },
        pulseSoft:{ '0%, 100%': { opacity: '1' }, '50%': { opacity: '0.5' } },
        shimmer:  { '0%': { backgroundPosition: '-200% 0' }, '100%': { backgroundPosition: '200% 0' } },
        glowPulse:{
          '0%, 100%': { boxShadow: '0 0 6px rgba(0,245,255,0.2)' },
          '50%':       { boxShadow: '0 0 20px rgba(0,245,255,0.6), 0 0 40px rgba(0,245,255,0.2)' },
        },
        glitch: {
          '0%, 95%, 100%': { transform: 'none', filter: 'none' },
          '96%': { transform: 'translate(-2px, 1px) skewX(-1deg)', filter: 'hue-rotate(90deg)' },
          '97%': { transform: 'translate(2px, -1px) skewX(1deg)', filter: 'hue-rotate(-90deg)' },
          '98%': { transform: 'translate(-1px, 0) skewX(0.5deg)', filter: 'hue-rotate(0deg)' },
        },
        scanline: {
          '0%':   { backgroundPosition: '0 0' },
          '100%': { backgroundPosition: '0 100vh' },
        },
        flicker: {
          '0%, 97%, 100%': { opacity: '1' },
          '98%':            { opacity: '0.85' },
          '99%':            { opacity: '0.95' },
        },
        borderFlow: {
          '0%':   { backgroundPosition: '0% 50%' },
          '50%':  { backgroundPosition: '100% 50%' },
          '100%': { backgroundPosition: '0% 50%' },
        },
        dataStream: {
          '0%':   { opacity: '0', transform: 'translateY(-4px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
      },
      borderRadius: { '2xl': '2px', '3xl': '4px' }, // Sharp angular corners for cyberpunk
    },
  },
  plugins: [],
}
