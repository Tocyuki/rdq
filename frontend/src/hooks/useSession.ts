import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { endpoints } from '@/lib/api/endpoints'
import type { Session } from '@/lib/api/types'

const SESSION_KEY = ['session'] as const

/**
 * useSession is the single place in the SPA that knows about the current
 * connection pointer. Pages read it, ConnectionDialog writes it, and the
 * surrounding TanStack Query cache invalidation keeps list endpoints that
 * depend on the profile in sync.
 */
export function useSession() {
  return useQuery({
    queryKey: SESSION_KEY,
    queryFn: ({ signal }) => endpoints.getSession(signal),
    // Session rarely changes behind our back (the server only updates it
    // via our own PUT), so cache it forever within a page lifetime.
    staleTime: Infinity,
  })
}

export function useSaveSession() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (s: Session) => endpoints.putSession(s),
    onSuccess: (_void, session) => {
      qc.setQueryData(SESSION_KEY, session)
      // Any list keyed on the previous profile should re-fetch.
      qc.invalidateQueries({ queryKey: ['clusters'] })
      qc.invalidateQueries({ queryKey: ['secrets'] })
      qc.invalidateQueries({ queryKey: ['databases'] })
      qc.invalidateQueries({ queryKey: ['history'] })
      qc.invalidateQueries({ queryKey: ['schema'] })
    },
    onError: (err: unknown) => {
      const message = err instanceof Error ? err.message : 'save failed'
      toast.error(`Could not save session: ${message}`)
    },
  })
}

/**
 * sessionIsComplete reports whether the session is ready for queries; the
 * SessionGate uses it to decide whether to open ConnectionDialog on launch.
 */
export function sessionIsComplete(s: Session | undefined): boolean {
  if (!s) return false
  return !!(s.profile && s.cluster && s.secret && s.database)
}
