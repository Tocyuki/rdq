import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { endpoints } from '@/lib/api/endpoints'
import type { AiContext } from '@/lib/api/types'

const queryKey = (cluster: string, database: string) => ['aictx', cluster, database] as const

/**
 * useAiContext fetches the saved (cluster, database) prompt context.
 * Disabled until both keys are set so we never hit /api/aictx with an
 * incomplete pair (the server would reject it). staleTime is Infinity
 * because mutations write the canonical server response straight into
 * the cache via setQueryData — there is nothing background-fresher to
 * fetch between explicit user edits.
 */
export function useAiContext(cluster: string, database: string) {
  return useQuery({
    queryKey: queryKey(cluster, database),
    queryFn: ({ signal }) => endpoints.getAictx(cluster, database, signal),
    enabled: !!cluster && !!database,
    staleTime: Infinity,
  })
}

/**
 * useSaveAiContext persists a new content for the given (cluster, database).
 * On success the cache is updated in place so the editor doesn't briefly
 * show empty content while a refetch is in flight.
 */
export function useSaveAiContext(cluster: string, database: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (content: string) =>
      endpoints.putAictx({ cluster, database, content }),
    onSuccess: (saved: AiContext) => {
      qc.setQueryData(queryKey(cluster, database), saved)
    },
  })
}

/**
 * useDeleteAiContext clears the saved context for (cluster, database).
 */
export function useDeleteAiContext(cluster: string, database: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => endpoints.deleteAictx(cluster, database),
    onSuccess: (cleared: AiContext) => {
      qc.setQueryData(queryKey(cluster, database), cleared)
    },
  })
}
