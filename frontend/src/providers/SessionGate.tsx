import { useEffect, useRef } from 'react'

import { ConnectionDialog } from '@/features/connection/ConnectionDialog'
import { sessionIsComplete, useSession } from '@/hooks/useSession'
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

  return (
    <>
      {children}
      <ConnectionDialog open={open} onOpenChange={setOpen} />
    </>
  )
}
