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
 */
export function RowDetailDialog({ open, onClose, columns, row }: Props) {
  if (!row) return null
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Row detail</DialogTitle>
          <DialogDescription>
            Full JSON, column order preserved ({columns.length} fields).
          </DialogDescription>
        </DialogHeader>
        <ScrollArea className="max-h-[60vh] rounded-md border border-border bg-muted/40 p-3">
          <pre className="whitespace-pre-wrap text-xs font-mono">
            {renderRow(columns, row)}
          </pre>
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
