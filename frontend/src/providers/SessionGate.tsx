import { useEffect, useRef } from 'react'

import { ConnectionDialog } from '@/features/connection/ConnectionDialog'
import { sessionIsComplete, useSession } from '@/hooks/useSession'
import { ApiError } from '@/lib/api/client'
import { ErrorCode } from '@/lib/api/error-codes'
import { useUIStore } from '@/stores/uiStore'

/**
 * SessionGate owns the one and only <ConnectionDialog /> instance. Both
 * the first-render auto-open (triggered here when the server reports an
 * incomplete session) and the explicit triggers from ConnectionBar (the
 * "Change" button and profile-switch follow-ups) write to the same
 * Zustand flag, so only one overlay is ever visible at a time.
 *
 * The auto-open is fire-once per browser page lifetime (guarded by a
 * ref). Subsequent incomplete sessions — e.g. the user switching
 * profiles — are reopened explicitly by ConnectionBar, which lets the
 * user dismiss the dialog once without the gate immediately reopening
 * it in a loop.
 */
export function SessionGate({ children }: { children: React.ReactNode }) {
  const session = useSession()
  const open = useUIStore((s) => s.connectionDialogOpen)
  const setOpen = useUIStore((s) => s.setConnectionDialogOpen)
  const autoOpenedRef = useRef(false)

  useEffect(() => {
    if (
      session.isSuccess &&
      !sessionIsComplete(session.data) &&
      !autoOpenedRef.current
    ) {
      autoOpenedRef.current = true
      setOpen(true)
    }
  }, [session.isSuccess, session.data, setOpen])

  if (session.isError) {
    const isUnauthorized =
      session.error instanceof ApiError && session.error.code === ErrorCode.Unauthorized
    return (
      <div className="flex min-h-screen items-center justify-center bg-background px-6">
        <div className="max-w-md rounded-lg border border-border bg-card p-6 text-sm shadow-sm">
          <h1 className="text-base font-semibold tracking-tight">
            {isUnauthorized ? 'GUI session expired' : 'Could not load GUI session'}
          </h1>
          <p className="mt-2 text-muted-foreground">
            {isUnauthorized
              ? 'This tab no longer has a valid per-run GUI token. Re-run `rdq gui` and use the newly opened window.'
              : session.error.message}
          </p>
        </div>
      </div>
    )
  }

  return (
    <>
      {children}
      <ConnectionDialog open={open} onOpenChange={setOpen} />
    </>
  )
}
