import { Check } from 'lucide-react'

interface Props {
  size?: 'sm' | 'md' | 'lg'
  className?: string
}

const sizeClass = {
  sm: 'h-8 w-8',
  md: 'h-10 w-10',
  lg: 'h-14 w-14',
}

const iconClass = {
  sm: 'h-4 w-4',
  md: 'h-5 w-5',
  lg: 'h-7 w-7',
}

export default function BrandMark({ size = 'md', className = '' }: Props) {
  return (
    <span
      className={`relative inline-flex shrink-0 items-center justify-center rounded-full bg-[#20b956] text-white ${sizeClass[size]} ${className}`}
      aria-hidden="true"
    >
      <span className="absolute -bottom-[1px] left-[3px] h-2.5 w-2.5 -rotate-[24deg] rounded-[2px] bg-[#20b956]" />
      <Check className={`relative z-10 ${iconClass[size]}`} strokeWidth={3} />
    </span>
  )
}
