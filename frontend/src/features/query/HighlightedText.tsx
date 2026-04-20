import { memo } from 'react'

import { cn } from '@/lib/utils'

interface Props {
  text: string
  term: string
  // Starting global match index for this text. Each rendered `<mark>`
  // gets `data-match-index={baseMatchIndex + n}` so ResultPanel can
  // scrollIntoView the active match via a single querySelector.
  baseMatchIndex?: number
  activeMatchIndex?: number
}
function Inner({ text, term, baseMatchIndex = 0, activeMatchIndex = -1 }: Props) {
  const needle = term.trim()
  if (!needle) return <>{text}</>

  const haystack = text.toLowerCase()
  const n = needle.toLowerCase()
  const parts: React.ReactNode[] = []

  let cursor = 0
  let localIdx = 0
  while (cursor < text.length) {
    const idx = haystack.indexOf(n, cursor)
    if (idx < 0) {
      parts.push(text.slice(cursor))
      break
    }
    if (idx > cursor) parts.push(text.slice(cursor, idx))
    const globalIdx = baseMatchIndex + localIdx
    const isActive = globalIdx === activeMatchIndex
    parts.push(
      <mark
        key={idx}
        data-match-index={globalIdx}
        data-active-match={isActive ? 'true' : undefined}
        className={cn(
          'rounded-sm px-0.5',
          isActive
            ? 'bg-orange-400 text-black ring-2 ring-orange-600 dark:bg-orange-500'
            : 'bg-yellow-300 text-black dark:bg-yellow-400',
        )}
      >
        {text.slice(idx, idx + needle.length)}
      </mark>,
    )
    cursor = idx + needle.length
    localIdx++
  }
  return <>{parts}</>
}

export const HighlightedText = memo(Inner)
