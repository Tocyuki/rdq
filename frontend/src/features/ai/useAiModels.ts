import { useQuery } from '@tanstack/react-query'

import { endpoints } from '@/lib/api/endpoints'

/**
 * useAiModels lists Bedrock models/inference profiles for the current
 * profile. Enabled only when a profile is set so we do not spam the
 * Bedrock control plane before the user finishes the connection wizard.
 */
export function useAiModels(profile: string) {
  return useQuery({
    queryKey: ['ai-models', profile],
    queryFn: ({ signal }) => endpoints.listModels(profile, signal),
    enabled: !!profile,
    staleTime: 5 * 60_000,
  })
}
