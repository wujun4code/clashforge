import type { SVGProps } from 'react'

/**
 * ClashForge cat mascot — simple, round, blue-and-white.
 *
 * Inspired by the Clash project's cat vibe but drawn from scratch so no
 * trademarks are borrowed. Keep it geometric & friendly.
 */
export function CatMascot({
  size = 32,
  ...rest
}: { size?: number } & Omit<SVGProps<SVGSVGElement>, 'width' | 'height'>) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 64 64"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
      {...rest}
    >
      {/* Ears */}
      <path d="M14 20 L18 7 L28 16 Z" fill="#6AA8E0" />
      <path d="M50 20 L46 7 L36 16 Z" fill="#6AA8E0" />
      <path d="M17 17 L19.5 11 L24 15 Z" fill="#F4A6B5" />
      <path d="M47 17 L44.5 11 L40 15 Z" fill="#F4A6B5" />
      {/* Head */}
      <ellipse cx="32" cy="34" rx="20" ry="18" fill="#6AA8E0" />
      {/* White cheek/chin patch */}
      <ellipse cx="32" cy="40" rx="13" ry="9" fill="#FAF4E7" />
      {/* Eyes */}
      <ellipse cx="25" cy="32" rx="2.4" ry="3.2" fill="#1A1F2E" />
      <ellipse cx="39" cy="32" rx="2.4" ry="3.2" fill="#1A1F2E" />
      <circle cx="25.9" cy="31" r="0.8" fill="#FAF4E7" />
      <circle cx="39.9" cy="31" r="0.8" fill="#FAF4E7" />
      {/* Nose (tiny triangle) */}
      <path d="M30.5 38 L33.5 38 L32 40 Z" fill="#F4A6B5" />
      {/* Smile */}
      <path
        d="M29 41 Q32 43.5 35 41"
        stroke="#1A1F2E"
        strokeWidth="1.3"
        strokeLinecap="round"
        fill="none"
      />
      {/* Whiskers */}
      <path d="M18 37 L24 38" stroke="#FAF4E7" strokeWidth="0.9" strokeLinecap="round" />
      <path d="M18 40 L24 40" stroke="#FAF4E7" strokeWidth="0.9" strokeLinecap="round" />
      <path d="M46 37 L40 38" stroke="#FAF4E7" strokeWidth="0.9" strokeLinecap="round" />
      <path d="M46 40 L40 40" stroke="#FAF4E7" strokeWidth="0.9" strokeLinecap="round" />
    </svg>
  )
}

/** Tiny paw-print glyph – for accents, empty states, success toasts. */
export function PawPrint({
  size = 16,
  ...rest
}: { size?: number } & Omit<SVGProps<SVGSVGElement>, 'width' | 'height'>) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="currentColor"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
      {...rest}
    >
      <ellipse cx="6" cy="9" rx="2" ry="2.5" />
      <ellipse cx="18" cy="9" rx="2" ry="2.5" />
      <ellipse cx="9" cy="5" rx="1.7" ry="2.2" />
      <ellipse cx="15" cy="5" rx="1.7" ry="2.2" />
      <path d="M12 11c-3.5 0-6 2.5-6 5 0 1.8 1.3 3 3 3 1 0 1.6-.4 3-.4s2 .4 3 .4c1.7 0 3-1.2 3-3 0-2.5-2.5-5-6-5Z" />
    </svg>
  )
}

/** Napping cat for loading states. */
export function NappingCat({ size = 64, ...rest }: { size?: number } & Omit<SVGProps<SVGSVGElement>, 'width' | 'height'>) {
  return (
    <svg width={size} height={size} viewBox="0 0 96 64" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true" {...rest}>
      {/* Body */}
      <ellipse cx="48" cy="48" rx="34" ry="12" fill="#6AA8E0" />
      <ellipse cx="48" cy="48" rx="28" ry="8" fill="#8FC1EC" opacity="0.7" />
      {/* Head tucked */}
      <ellipse cx="22" cy="42" rx="11" ry="10" fill="#6AA8E0" />
      {/* Ear */}
      <path d="M16 35 L18 27 L24 33 Z" fill="#6AA8E0" />
      {/* Closed eye */}
      <path d="M18 42 Q21 44 24 42" stroke="#1A1F2E" strokeWidth="1.2" strokeLinecap="round" fill="none" />
      {/* Nose */}
      <circle cx="14" cy="44" r="1" fill="#F4A6B5" />
      {/* Z z z */}
      <text x="68" y="22" fontFamily="Nunito, sans-serif" fontWeight="700" fontSize="10" fill="#C8B5E8">z</text>
      <text x="74" y="16" fontFamily="Nunito, sans-serif" fontWeight="700" fontSize="13" fill="#C8B5E8">Z</text>
      <text x="82" y="10" fontFamily="Nunito, sans-serif" fontWeight="700" fontSize="16" fill="#C8B5E8">Z</text>
    </svg>
  )
}
