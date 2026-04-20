import { AlertTriangle } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

interface Props {
  open: boolean
  reason: string | null
  sql: string
  pending: boolean
  onCancel: () => void
  onConfirm: () => void
}

/**
 * ConfirmRunDialog guards destructive statements (DELETE/UPDATE without
 * WHERE, TRUNCATE) reported by the server as `confirmation_required`.
 * Cancel aborts the whole run; "Run anyway" re-issues the execute
 * mutation with `confirmed: true` so the server skips the gate.
 *
 * The SQL that would be run is previewed in a scrolling pre so the user
 * can double-check which statement is about to go through.
 */
export function ConfirmRunDialog({ open, reason, sql, pending, onCancel, onConfirm }: Props) {
  return (
    <Dialog open={open} onOpenChange={(o) => !o && !pending && onCancel()}>
      <DialogContent
        className="sm:max-w-xl"
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="size-4 text-destructive" />
            Confirm destructive query
          </DialogTitle>
          <DialogDescription>
            {reason ?? 'This statement looks destructive. Run it anyway?'}
          </DialogDescription>
        </DialogHeader>
        <pre className="max-h-[40vh] overflow-auto rounded-md border border-border bg-muted/40 p-3 text-xs font-mono whitespace-pre-wrap break-all">
          {sql}
        </pre>
        <DialogFooter>
          <Button variant="ghost" onClick={onCancel} disabled={pending}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={onConfirm} disabled={pending}>
            {pending ? 'Running…' : 'Run anyway'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
