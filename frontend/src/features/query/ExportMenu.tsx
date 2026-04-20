import { Copy, Download } from 'lucide-react'
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
 * rowsToJSON produces the pretty-printed JSON representation of a result
 * with the column order preserved (matches the payload shown in the JSON
 * tab). Shared between the Export download and the Copy-to-clipboard
 * flows so both surfaces stay in sync.
 */
function rowsToJSON(result: ExecuteResponseBody): string {
  return JSON.stringify(
    result.rows.map((row) =>
      Object.fromEntries(result.columns.map((c, i) => [c, row[i]])),
    ),
    null,
    2,
  )
}

/**
 * ExportMenu offers CSV and JSON file downloads for the current result.
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
            downloadJSON('rdq', rowsToJSON(result))
            toast.success('JSON downloaded')
          }}
        >
          JSON
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/**
 * CopyMenu writes the current result to the OS clipboard. Same CSV / JSON
 * choices the Export dropdown offers — users pick the format that
 * matches where they are pasting into (spreadsheet vs JSON tool).
 */
export function CopyMenu({ result }: Props) {
  const disabled = !result || result.rows.length === 0

  async function writeToClipboard(content: string, label: string) {
    try {
      await navigator.clipboard.writeText(content)
      toast.success(`${label} copied to clipboard`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Copy failed')
    }
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" variant="outline" disabled={disabled}>
          <Copy />
          Copy
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          disabled={disabled}
          onSelect={() => {
            if (!result) return
            void writeToClipboard(toCSV(result.columns, result.rows), 'CSV')
          }}
        >
          CSV
        </DropdownMenuItem>
        <DropdownMenuItem
          disabled={disabled}
          onSelect={() => {
            if (!result) return
            void writeToClipboard(rowsToJSON(result), 'JSON')
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
