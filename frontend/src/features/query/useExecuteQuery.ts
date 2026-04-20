import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { ApiError } from '@/lib/api/client'
import { endpoints } from '@/lib/api/endpoints'
import { ErrorCode } from '@/lib/api/error-codes'
import type { ExecuteResponseBody } from '@/lib/api/types'
import { useUIStore } from '@/stores/uiStore'

interface ExecuteArgs {
  profile: string
  cluster: string
  secret: string
  database: string
  sql: string
  confirmed?: boolean
}

/**
 * useExecuteQuery wraps POST /api/execute in a TanStack mutation and
 * caches the last result in the UI store so AnalyzeDialog can reach it.
 * See `ExecuteResponse.NeedsConfirmation` in internal/server/types.go
 * for the confirmation-flow contract.
 */
export function useExecuteQuery() {
  const qc = useQueryClient()
  const setLastResult = useUIStore((s) => s.setLastResult)

  return useMutation<ExecuteResponseBody, Error, ExecuteArgs>({
    mutationFn: (args) => endpoints.execute(args),
    onSuccess: (data, vars) => {
      if (!data.needsConfirmation) {
        setLastResult(data)
      }
      qc.invalidateQueries({ queryKey: ['history', vars.profile, vars.database] })
    },
    onError: (err, vars) => {
      qc.invalidateQueries({ queryKey: ['history', vars.profile, vars.database] })
      if (err instanceof ApiError && err.code === ErrorCode.ReadOnly) {
        toast.error('Read-only mode is on — destructive statements are blocked.', {
          description: 'Toggle "Allow writes" in Settings to run this query.',
        })
        return
      }
      toast.error(err.message || 'Query failed')
    },
  })
}
