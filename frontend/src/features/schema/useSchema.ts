import { useQuery } from '@tanstack/react-query'

import { endpoints } from '@/lib/api/endpoints'

/**
 * useSchema fetches the cached information_schema snapshot for a given
 * cluster + database. The Go server returns a cache hit in < 1 ms even
 * over the loopback; cold misses trigger a fresh fetch with a 30 s cap.
 */
export function useSchema(params: {
  profile: string
  cluster: string
  secret: string
  database: string
}) {
  const { profile, cluster, secret, database } = params
  return useQuery({
    queryKey: ['schema', cluster, database],
    queryFn: ({ signal }) =>
      endpoints.getSchema({ profile, cluster, secret, database }, signal),
    enabled: !!cluster && !!database,
    staleTime: 5 * 60_000,
  })
}
