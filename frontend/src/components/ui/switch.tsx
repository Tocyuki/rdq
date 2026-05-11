import * as React from 'react'

import { cn } from '@/lib/utils'

interface SwitchProps
  extends Omit<
    React.ButtonHTMLAttributes<HTMLButtonElement>,
    'onChange' | 'type' | 'role' | 'aria-checked' | 'children'
  > {
  checked: boolean
  onCheckedChange: (checked: boolean) => void
}

/**
 * Switch is a self-contained toggle styled to match the rest of the
 * shadcn-flavoured UI. Implemented as a `role="switch"` button so we
 * pull in zero extra dependencies — the project does not include
 * @radix-ui/react-switch and the toggle is needed in only one place.
 *
 * Extending ButtonHTMLAttributes (with the few props we own omitted) lets
 * callers forward arbitrary aria-* / data-* attributes without having to
 * extend this component every time a new accessibility need appears.
 */
export const Switch = React.forwardRef<HTMLButtonElement, SwitchProps>(
  ({ checked, onCheckedChange, disabled, className, ...rest }, ref) => (
    <button
      ref={ref}
      type="button"
      role="switch"
      aria-checked={checked}
      data-state={checked ? 'checked' : 'unchecked'}
      disabled={disabled}
      onClick={() => onCheckedChange(!checked)}
      className={cn(
        'inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border border-transparent transition-colors duration-150',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
        'disabled:cursor-not-allowed disabled:opacity-50',
        checked ? 'bg-primary' : 'bg-input',
        className,
      )}
      {...rest}
    >
      <span
        aria-hidden
        className={cn(
          // 36px track − 16px thumb − 2px padding on each side ≈ 16px travel.
          // translate-x-[2px] / translate-x-[18px] keeps the gap symmetrical.
          'pointer-events-none block size-4 rounded-full bg-background shadow ring-0 transition-transform duration-150',
          checked ? 'translate-x-[18px]' : 'translate-x-[2px]',
        )}
      />
    </button>
  ),
)
Switch.displayName = 'Switch'
