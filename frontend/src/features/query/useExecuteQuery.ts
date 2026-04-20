import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { ApiError } from '@/lib/api/client'
import { endpoints } from '@/lib/api/endpoints'
import type { ExecuteResponseBody } from '@/lib/api/types'
import { useUIStore } from '@/stores/uiStore'

interface ExecuteArgs {
  profile: string
  cluster: string
  secret: string
  database: string
  sql: string
}

/**
 * useExecuteQuery wraps POST /api/execute in a TanStack mutation and
 * caches the last result in the UI store so AnalyzeDialog can reach it.
 * History is invalidated on every execution (success or failure) so the
 * HistoryPanel reflects new entries appended by the server.
 *
 * Error handling: a read-only rejection from the server carries
 * code="read_only" on ApiError. We upgrade the toast for that case
 * to mention the Settings toggle so the user has a clear next step.
 */
export function useExecuteQuery() {
  const qc = useQueryClient()
  const setLastResult = useUIStore((s) => s.setLastResult)

  return useMutation<ExecuteResponseBody, Error, ExecuteArgs>({
    mutationFn: (args) => endpoints.execute(args),
    onSuccess: (data, vars) => {
      setLastResult(data)
      qc.invalidateQueries({ queryKey: ['history', vars.profile, vars.database] })
    },
    onError: (err, vars) => {
      qc.invalidateQueries({ queryKey: ['history', vars.profile, vars.database] })
      if (err instanceof ApiError && err.code === 'read_only') {
        toast.error('Read-only mode is on — destructive statements are blocked.', {
          description: 'Toggle "Allow writes" in Settings to run this query.',
        })
        return
      }
      toast.error(err.message || 'Query failed')
    },
  })
}
