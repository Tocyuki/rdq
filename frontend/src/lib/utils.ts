import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/**
 * cn concatenates Tailwind class strings with deduplication. This is the
 * shadcn/ui convention used by every component: `className={cn(base, extra)}`.
 */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
