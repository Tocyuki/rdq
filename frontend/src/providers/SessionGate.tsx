import { useState } from 'react'

import { ConnectionDialog } from '@/features/connection/ConnectionDialog'
import { sessionIsComplete, useSession } from '@/hooks/useSession'

/**
 * SessionGate is the app-level guardrail: as soon as /api/session resolves
 * with an incomplete connection the dialog opens so the user is nudged
 * into filling profile/cluster/secret/database before anything else can
 * fail on empty values.
 *
 * The open state is derived rather than an effect-driven useState write:
 * if the session is loaded and incomplete, show the dialog, unless the
 * user has already dismissed it this session. This keeps React 19's
 * "no setState in effects" rule satisfied and prevents a cascading render
 * on first paint.
 */
export function SessionGate({ children }: { children: React.ReactNode }) {
  const session = useSession()
  const [dismissed, setDismissed] = useState(false)

  const needsSetup = session.isSuccess && !sessionIsComplete(session.data)
  const open = needsSetup && !dismissed

  return (
    <>
      {children}
      <ConnectionDialog
        open={open}
        onOpenChange={(next) => {
          if (!next) setDismissed(true)
        }}
      />
    </>
  )
}
