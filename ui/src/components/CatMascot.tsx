type CatMascotProps = {
  size?: number
  className?: string
  mood?: 'default' | 'sleepy' | 'excited'
}

export function CatMascot({ size = 56, className = '', mood = 'default' }: CatMascotProps) {
  const eyePath = mood === 'sleepy'
    ? 'M13 19.5c1.4-1 2.8-1 4.2 0M26.8 19.5c1.4-1 2.8-1 4.2 0'
    : mood === 'excited'
      ? 'M12.6 16.8h5.4v5.4h-5.4zM26 16.8h5.4v5.4H26z'
      : 'M12.6 17.2h5.4v4.6h-5.4zM26 17.2h5.4v4.6H26z'

  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 44 44"
      width={size}
      height={size}
      className={className}
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      <defs>
        <linearGradient id="catShell" x1="8" y1="6" x2="35" y2="38" gradientUnits="userSpaceOnUse">
          <stop stopColor="#A78BFA" />
          <stop offset="0.55" stopColor="#8B5CF6" />
          <stop offset="1" stopColor="#EC4899" />
        </linearGradient>
        <linearGradient id="catVisor" x1="9" y1="14" x2="34" y2="29" gradientUnits="userSpaceOnUse">
          <stop stopColor="#0F172A" />
          <stop offset="1" stopColor="#1E1B4B" />
        </linearGradient>
      </defs>

      <path d="M12 13 9.1 5.7c-.3-.9.7-1.6 1.5-1l6.1 4.8" fill="#F472B6" fillOpacity=".9" />
      <path d="M32 13l2.9-7.3c.3-.9-.7-1.6-1.5-1l-6.1 4.8" fill="#F59E0B" fillOpacity=".95" />
      <path d="M22 6.2c8.6 0 15.6 6.7 15.6 14.9S30.6 36 22 36 6.4 29.3 6.4 21.1 13.4 6.2 22 6.2Z" fill="url(#catShell)" />
      <path d="M10.5 14.4h23c1.8 0 3.3 1.5 3.3 3.3v8.2c0 1.8-1.5 3.3-3.3 3.3h-23c-1.8 0-3.3-1.5-3.3-3.3v-8.2c0-1.8 1.5-3.3 3.3-3.3Z" fill="url(#catVisor)" stroke="#C4B5FD" strokeOpacity=".7" />
      <path d={eyePath} stroke="#67E8F9" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" fill="#67E8F9" />
      <path d="M20.3 24.6 22 26l1.7-1.4" stroke="#FDE68A" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M17.8 27.8c1.4 1 2.8 1.5 4.2 1.5 1.4 0 2.8-.5 4.2-1.5" stroke="#FDF2F8" strokeWidth="1.5" strokeLinecap="round" />
      <path d="M14.7 26.3 9.8 27.1M14.8 23.7 8.5 23.7M29.2 26.3l4.9.8M29.2 23.7h6.3" stroke="#FBCFE8" strokeWidth="1.35" strokeLinecap="round" opacity=".95" />
      <circle cx="16.8" cy="33.3" r="1.35" fill="#FDF2F8" fillOpacity=".95" />
      <circle cx="21.2" cy="34.7" r="1.35" fill="#FDF2F8" fillOpacity=".95" />
      <circle cx="25.6" cy="33.3" r="1.35" fill="#FDF2F8" fillOpacity=".95" />
      <circle cx="30" cy="34.7" r="1.35" fill="#FDF2F8" fillOpacity=".95" />
    </svg>
  )
}
