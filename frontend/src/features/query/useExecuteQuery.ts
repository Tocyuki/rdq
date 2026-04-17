import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

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
      toast.error(err.message || 'Query failed')
    },
  })
}
