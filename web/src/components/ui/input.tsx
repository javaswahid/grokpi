import * as React from 'react'
import { cn } from '@/lib/utils'

const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => {
    return (
      <input
        type={type}
        className={cn(
          'flex h-8 w-full rounded-[4px] bg-surface hover:bg-surface-hover focus:bg-surface-active border border-input border-b-border-card px-3 py-1.5 text-[13px] text-foreground shadow-[inset_0_1px_2px_rgba(0,0,0,0.02)] transition-all duration-150 placeholder:text-muted/80 focus:border-b-primary focus:outline-none disabled:cursor-not-allowed disabled:opacity-50 file:border-0 file:bg-transparent file:text-sm file:font-medium',
          className
        )}
        ref={ref}
        {...props}
      />
    )
  }
)
Input.displayName = 'Input'

export { Input }
