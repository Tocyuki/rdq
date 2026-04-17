import { Download } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { downloadCSV, toCSV } from '@/lib/csv'
import type { ExecuteResponseBody } from '@/lib/api/types'

interface Props {
  result: ExecuteResponseBody | null
}

/**
 * ExportMenu offers CSV and JSON downloads for the current result.
 * JSON is pretty-printed (2 spaces) so piping into another tool is easy.
 */
export function ExportMenu({ result }: Props) {
  const disabled = !result || result.rows.length === 0

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" variant="outline" disabled={disabled}>
          <Download />
          Export
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          disabled={disabled}
          onSelect={() => {
            if (!result) return
            downloadCSV('rdq', toCSV(result.columns, result.rows))
            toast.success('CSV downloaded')
          }}
        >
          CSV
        </DropdownMenuItem>
        <DropdownMenuItem
          disabled={disabled}
          onSelect={() => {
            if (!result) return
            const json = JSON.stringify(
              result.rows.map((row) =>
                Object.fromEntries(result.columns.map((c, i) => [c, row[i]])),
              ),
              null,
              2,
            )
            downloadJSON('rdq', json)
            toast.success('JSON downloaded')
          }}
        >
          JSON
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function downloadJSON(filenameBase: string, content: string) {
  const blob = new Blob([content], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  try {
    const a = document.createElement('a')
    a.href = url
    const d = new Date()
    const pad = (n: number) => String(n).padStart(2, '0')
    const ts =
      d.getFullYear().toString() +
      pad(d.getMonth() + 1) +
      pad(d.getDate()) +
      '-' +
      pad(d.getHours()) +
      pad(d.getMinutes()) +
      pad(d.getSeconds())
    a.download = `${filenameBase}-${ts}.json`
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    a.remove()
  } finally {
    URL.revokeObjectURL(url)
  }
}
