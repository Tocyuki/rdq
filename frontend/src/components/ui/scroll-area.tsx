import * as React from 'react'
import * as ScrollAreaPrimitive from '@radix-ui/react-scroll-area'

import { cn } from '@/lib/utils'

/**
 * ScrollArea wraps @radix-ui/react-scroll-area with shadcn-style scrollbars.
 *
 * The `[&>div]:!block` override on the Viewport is important: Radix's
 * Viewport wraps children in a helper <div> with `display: table; min-width:
 * 100%;` so the inner content can expand horizontally and a scrollbar can
 * appear. For vertical-only lists (history, schema tree, result panel,
 * etc.) that table layout causes sibling rows to grow in width when any
 * single row's intrinsic content is wide, pushing right-edge action
 * buttons off-screen. Forcing block display keeps the child widths bounded
 * to the viewport so `flex-1 + min-w-0 + break-all` inside our rows work
 * as expected.
 */
export const ScrollArea = React.forwardRef<
  React.ElementRef<typeof ScrollAreaPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof ScrollAreaPrimitive.Root>
>(({ className, children, ...props }, ref) => (
  <ScrollAreaPrimitive.Root
    ref={ref}
    className={cn('relative overflow-hidden', className)}
    {...props}
  >
    <ScrollAreaPrimitive.Viewport className="h-full w-full rounded-[inherit] [&>div]:!block">
      {children}
    </ScrollAreaPrimitive.Viewport>
    <ScrollAreaPrimitive.Scrollbar
      orientation="vertical"
      className="flex touch-none select-none p-0.5 transition-colors h-full w-2.5"
    >
      <ScrollAreaPrimitive.Thumb className="relative flex-1 rounded-full bg-border" />
    </ScrollAreaPrimitive.Scrollbar>
    <ScrollAreaPrimitive.Scrollbar
      orientation="horizontal"
      className="flex touch-none select-none p-0.5 transition-colors w-full h-2.5 flex-col"
    >
      <ScrollAreaPrimitive.Thumb className="relative flex-1 rounded-full bg-border" />
    </ScrollAreaPrimitive.Scrollbar>
    <ScrollAreaPrimitive.Corner />
  </ScrollAreaPrimitive.Root>
))
ScrollArea.displayName = ScrollAreaPrimitive.Root.displayName
