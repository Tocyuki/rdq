import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { endpoints } from '@/lib/api/endpoints'

/**
 * useHistory fetches recent queries for the (profile, database) pair.
 * Empty profile/database short-circuits to an empty list so the
 * HistoryPage can render before the user has finished the wizard.
 */
export function useHistory(profile: string, database: string) {
  return useQuery({
    queryKey: ['history', profile, database],
    queryFn: ({ signal }) => endpoints.listHistory(profile, database, signal),
    enabled: !!profile && !!database,
  })
}

/**
 * useSetFavorite toggles the star flag on a single history entry,
 * identified by its RFC3339Nano `at` timestamp. The optimistic cache
 * update is deliberately skipped — the history JSONL file is the
 * source of truth, and a refetch is only a few KB.
 */
export function useSetFavorite(profile: string, database: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (args: { at: string; favorite: boolean }) =>
      endpoints.setFavorite(args),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['history', profile, database] })
    },
    onError: (err: unknown) => {
      toast.error(err instanceof Error ? err.message : 'favourite failed')
    },
  })
}
