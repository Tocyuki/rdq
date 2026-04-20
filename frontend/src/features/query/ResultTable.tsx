import { memo, useEffect, useMemo, useState } from 'react'

import { ScrollArea } from '@/components/ui/scroll-area'
import { countMatches } from '@/lib/search'
import { cn } from '@/lib/utils'

import { HighlightedText } from './HighlightedText'
import { RowDetailDialog } from './RowDetailDialog'

interface Props {
  columns: string[]
  rows: unknown[][]
  searchTerm?: string
  onMatchCount?: (count: number) => void
  activeMatchIndex?: number
}

/**
 * ResultTable renders the /api/execute response in a plain table that
 * scrolls independently of the surrounding page. Cells are truncated to
 * keep the grid readable; clicking a row opens a JSON detail dialog so
 * the user can inspect long values without squinting at a truncated cell.
 */
function Inner({
  columns,
  rows,
  searchTerm = '',
  onMatchCount,
  activeMatchIndex = -1,
}: Props) {
  const [activeRow, setActiveRow] = useState<number | null>(null)

  const cellStrings = useMemo(
    () => rows.map((row) => columns.map((_, c) => formatCell(row[c]))),
    [columns, rows],
  )

  /**
   * Walks headers → cells in reading order, assigning each stretch of
   * matches a contiguous global range. The traversal order here is
   * what the Enter navigation follows, so users step through matches
   * top-to-bottom, left-to-right.
   */
  const { totalMatches, headerBases, cellBases } = useMemo(() => {
    const trimmed = searchTerm.trim()
    if (!trimmed) {
      return {
        totalMatches: 0,
        headerBases: [] as number[],
        cellBases: [] as number[][],
      }
    }
    const hb: number[] = []
    let running = 0
    for (const h of columns) {
      hb.push(running)
      running += countMatches(h, trimmed)
    }
    const cb: number[][] = cellStrings.map((row) =>
      row.map((cell) => {
        const base = running
        running += countMatches(cell, trimmed)
        return base
      }),
    )
    return { totalMatches: running, headerBases: hb, cellBases: cb }
  }, [columns, cellStrings, searchTerm])

  useEffect(() => {
    onMatchCount?.(totalMatches)
  }, [totalMatches, onMatchCount])

  if (columns.length === 0) {
    return (
      <div className="flex h-full items-center justify-center p-4 text-sm text-muted-foreground">
        Query returned no result set.
      </div>
    )
  }

  return (
    <>
      <ScrollArea className="h-full">
        <table className="w-full border-collapse text-sm">
          <thead className="bg-muted sticky top-0">
            <tr>
              {columns.map((c, idx) => (
                <th
                  key={`${idx}-${c}`}
                  className="border-b border-border px-3 py-2 text-left font-semibold text-muted-foreground"
                >
                  <HighlightedText
                    text={c}
                    term={searchTerm}
                    baseMatchIndex={headerBases[idx]}
                    activeMatchIndex={activeMatchIndex}
                  />
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((_row, rIdx) => (
              <tr
                key={rIdx}
                className={cn(
                  'cursor-pointer border-b border-border/70 transition-colors hover:bg-muted/50',
                  activeRow === rIdx && 'bg-muted',
                )}
                onClick={() => setActiveRow(rIdx)}
              >
                {columns.map((_, cIdx) => (
                  <td
                    key={cIdx}
                    className="max-w-[32ch] truncate px-3 py-1.5 font-mono text-[13px]"
                  >
                    <HighlightedText
                      text={cellStrings[rIdx][cIdx]}
                      term={searchTerm}
                      baseMatchIndex={cellBases[rIdx]?.[cIdx]}
                      activeMatchIndex={activeMatchIndex}
                    />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </ScrollArea>
      <RowDetailDialog
        open={activeRow !== null}
        onClose={() => setActiveRow(null)}
        columns={columns}
        row={activeRow != null ? rows[activeRow] : undefined}
      />
    </>
  )
}

function formatCell(v: unknown): string {
  if (v === null || v === undefined) return 'NULL'
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'bigint' || typeof v === 'boolean') {
    return String(v)
  }
  try {
    return JSON.stringify(v)
  } catch {
    return String(v)
  }
}

export const ResultTable = memo(Inner)
