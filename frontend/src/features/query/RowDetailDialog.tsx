import { Copy } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'

interface Props {
  open: boolean
  onClose: () => void
  columns: string[]
  row: unknown[] | undefined
}

/**
 * RowDetailDialog renders a single row as pretty-printed JSON with the
 * original column order preserved. Using manual serialization avoids the
 * alphabetical sort JSON.stringify applies to objects and matches the
 * RowJSON output the Go runner package produces for the TUI.
 *
 * UX details:
 *   - A Copy button in the header sends the rendered JSON to the OS
 *     clipboard.
 *   - onOpenAutoFocus is prevented so Radix does not pull keyboard focus
 *     onto the close ("×") button when the dialog opens; that was making
 *     the × visibly active and distracting from the JSON payload users
 *     actually want to read.
 */
export function RowDetailDialog({ open, onClose, columns, row }: Props) {
  if (!row) return null
  const json = renderRow(columns, row)

  async function copyJSON() {
    try {
      await navigator.clipboard.writeText(json)
      toast.success('Row JSON copied to clipboard')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Copy failed')
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent
        className="sm:max-w-2xl"
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <DialogHeader>
          <div className="flex items-start justify-between gap-3">
            <div className="space-y-1">
              <DialogTitle>Row detail</DialogTitle>
              <DialogDescription>
                Full JSON, column order preserved ({columns.length} fields).
              </DialogDescription>
            </div>
            <Button
              size="sm"
              variant="outline"
              onClick={copyJSON}
              className="mr-8"
              title="Copy JSON to clipboard"
            >
              <Copy />
              Copy
            </Button>
          </div>
        </DialogHeader>
        <ScrollArea className="max-h-[60vh] rounded-md border border-border bg-muted/40 p-3">
          <pre className="whitespace-pre-wrap text-xs font-mono">{json}</pre>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  )
}

function renderRow(columns: string[], row: unknown[]) {
  const entries = columns.map((c, i) => {
    const v = row[i]
    const json = JSON.stringify(v, null, 2)
    return `  ${JSON.stringify(c)}: ${indent(json ?? 'null')}`
  })
  return '{\n' + entries.join(',\n') + '\n}'
}

function indent(s: string) {
  return s.split('\n').join('\n  ')
}
