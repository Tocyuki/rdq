import { memo, useState } from 'react'

import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/lib/utils'

import { RowDetailDialog } from './RowDetailDialog'

interface Props {
  columns: string[]
  rows: unknown[][]
}

/**
 * ResultTable renders the /api/execute response in a plain table that
 * scrolls independently of the surrounding page. Cells are truncated to
 * keep the grid readable; clicking a row opens a JSON detail dialog so
 * the user can inspect long values without squinting at a truncated cell.
 */
function Inner({ columns, rows }: Props) {
  const [activeRow, setActiveRow] = useState<number | null>(null)

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
                  {c}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, rIdx) => (
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
                    {formatCell(row[cIdx])}
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

/**
 * formatCell is the in-table display format: NULL becomes the literal
 * "NULL", arrays are JSON.stringify'd, primitives go through String().
 * Kept here (not in lib/) because the table view's styling goals differ
 * from CSV formatting.
 */
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
