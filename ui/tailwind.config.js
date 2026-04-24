/** @type {import('tailwindcss').Config} */
/*
 * ClashForge "Cat Mascot + Geek" palette.
 *
 * NOTE: The old Tailwind token names (neon.*, surface.*, shadow.neon-*) are
 * kept intentionally so existing pages keep compiling. Their values have been
 * remapped from cyberpunk neon to a soft, warm, Clash-cat-inspired theme.
 *
 *   surface.*    warm ink / night-sky background (soft dark)
 *   neon.cyan    cat blue (primary)
 *   neon.magenta paw-pad pink
 *   neon.yellow  pumpkin / honey (warning accent)
 *   neon.green   catnip green (success)
 *   neon.red     strawberry red (danger)
 *   neon.blue    deeper cat blue
 *   neon.violet  soft lavender
 *   neon.amber   tangerine
 */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        // Soft warm-dark background stack. Still developer-friendly, but cozy
        // rather than near-black. Accepts Tailwind alpha modifiers.
        surface: {
          0: 'rgb(26 31 46 / <alpha-value>)',   // night sky
          1: 'rgb(34 41 56 / <alpha-value>)',
          2: 'rgb(43 51 70 / <alpha-value>)',
          3: 'rgb(53 64 85 / <alpha-value>)',
          4: 'rgb(65 77 102 / <alpha-value>)',
        },
        // Kept for backwards compat but remapped to soft cat colors
        neon: {
          cyan:    '#6AA8E0',   // cat blue
          magenta: '#F4A6B5',   // paw pad pink
          yellow:  '#F5B86B',   // pumpkin / honey
          green:   '#8FD4A8',   // catnip green
          red:     '#E87E7E',   // strawberry red
          blue:    '#4A86C5',   // deeper cat blue
          violet:  '#C8B5E8',   // soft lavender
          amber:   '#F5B066',   // tangerine
        },
        accent: {
          violet:       '#C8B5E8',
          'violet-soft':'#DCCBF0',
          amber:        '#F5B066',
          'amber-soft': '#FACF9B',
        },
        // Semantic – lean on the soft cat palette
        brand:          '#6AA8E0',
        'brand-light':  '#8FC1EC',
        'brand-subtle': 'rgba(106, 168, 224, 0.10)',
        cta:            '#F4A6B5',
        'cta-subtle':   'rgba(244, 166, 181, 0.12)',
        success:        '#8FD4A8',
        'success-subtle':'rgba(143, 212, 168, 0.12)',
        warning:        '#F5B86B',
        'warning-subtle':'rgba(245, 184, 107, 0.12)',
        danger:         '#E87E7E',
        'danger-subtle':'rgba(232, 126, 126, 0.12)',
        muted:          '#8EA0B8',
        // Cat-specific tokens
        cream:          '#F3E8D5',
        'cream-soft':   '#FAF4E7',
        paw:            '#F4A6B5',
      },
      fontFamily: {
        // Headings: friendly rounded sans (Nunito / Quicksand)
        // Body: Inter + Noto Sans SC for zh
        // Mono: JetBrains Mono / Fira Code – preserves the geek feel
        sans:    ['Inter', '"Noto Sans SC"', 'system-ui', 'sans-serif'],
        body:    ['Inter', '"Noto Sans SC"', 'system-ui', 'sans-serif'],
        mono:    ['"JetBrains Mono"', '"Fira Code"', 'ui-monospace', 'monospace'],
        heading: ['Nunito', 'Quicksand', 'Inter', 'system-ui', 'sans-serif'],
        display: ['Nunito', 'Quicksand', 'Inter', 'system-ui', 'sans-serif'],
      },
      opacity: { 96: '0.96' },
      backdropBlur: { xs: '2px' },
      boxShadow: {
        // Soft, diffuse shadows – no glow. Names kept for back-compat.
        'neon-cyan':    '0 4px 16px rgba(106,168,224,0.18), 0 2px 4px rgba(106,168,224,0.10)',
        'neon-magenta': '0 4px 16px rgba(244,166,181,0.20), 0 2px 4px rgba(244,166,181,0.10)',
        'neon-green':   '0 4px 16px rgba(143,212,168,0.20), 0 2px 4px rgba(143,212,168,0.10)',
        'neon-yellow':  '0 4px 16px rgba(245,184,107,0.20), 0 2px 4px rgba(245,184,107,0.10)',
        'neon-red':     '0 4px 16px rgba(232,126,126,0.22), 0 2px 4px rgba(232,126,126,0.10)',
        'neon-violet':  '0 4px 16px rgba(200,181,232,0.22), 0 2px 4px rgba(200,181,232,0.10)',
        'neon-amber':   '0 4px 16px rgba(245,176,102,0.22), 0 2px 4px rgba(245,176,102,0.10)',
        glass:    '0 4px 20px rgba(12, 18, 32, 0.35), 0 1px 2px rgba(12,18,32,0.20)',
        'glass-lg':'0 10px 32px rgba(12, 18, 32, 0.40), 0 2px 6px rgba(12,18,32,0.25)',
        hud:      '0 2px 8px rgba(12, 18, 32, 0.25), inset 0 1px 0 rgba(255,255,255,0.04)',
        glow:     '0 4px 20px rgba(106,168,224,0.25)', // soft brand halo
        paw:      '0 2px 6px rgba(244,166,181,0.30)',  // pink paw-print accent
        soft:     '0 1px 2px rgba(12,18,32,0.25), 0 4px 12px rgba(12,18,32,0.20)',
      },
      animation: {
        'fade-in':     'fadeIn 0.25s ease-out',
        'slide-up':    'slideUp 0.25s ease-out',
        'slide-down':  'slideDown 0.25s ease-out',
        'scale-in':    'scaleIn 0.2s ease-out',
        'pulse-soft':  'pulseSoft 2s ease-in-out infinite',
        'shimmer':     'shimmer 2.5s linear infinite',
        // Legacy names kept as gentle fallbacks so any stray `animate-glitch`
        // etc. on pages no longer causes harsh motion.
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
      // Generous, friendly radii
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
