import { api } from './client'
import type {
  AnalyzeRequestBody,
  AskRequestBody,
  AskResponseBody,
  Clusters,
  Databases,
  ExecuteRequestBody,
  ExecuteResponseBody,
  ExplainRequestBody,
  FavoriteBody,
  Health,
  History,
  Models,
  Profiles,
  ReviewRequestBody,
  Schema,
  SchemaRefreshBody,
  Secrets,
  Session,
  TextResponse,
} from './types'

/**
 * endpoints is the typed, path-per-method surface the rest of the SPA
 * talks to. Keeping the URLs here (instead of scattered strings) means
 * the backend routing contract only has to be updated in one place.
 */
export const endpoints = {
  health: (signal?: AbortSignal) => api.get<Health>('/api/health', signal),

  getSession: (signal?: AbortSignal) => api.get<Session>('/api/session', signal),
  putSession: (body: Session, signal?: AbortSignal) =>
    api.put<void>('/api/session', body, signal),

  listProfiles: (signal?: AbortSignal) => api.get<Profiles>('/api/profiles', signal),

  listClusters: (profile: string, signal?: AbortSignal) =>
    api.get<Clusters>(`/api/clusters?profile=${encodeURIComponent(profile)}`, signal),

  listSecrets: (profile: string, cluster?: string, signal?: AbortSignal) => {
    const params = new URLSearchParams({ profile })
    if (cluster) params.set('cluster', cluster)
    return api.get<Secrets>(`/api/secrets?${params}`, signal)
  },

  listDatabases: (profile: string, signal?: AbortSignal) =>
    api.get<Databases>(`/api/databases?profile=${encodeURIComponent(profile)}`, signal),

  execute: (body: ExecuteRequestBody, signal?: AbortSignal) =>
    api.post<ExecuteResponseBody>('/api/execute', body, signal),

  getSchema: (
    params: { profile?: string; cluster: string; secret?: string; database: string },
    signal?: AbortSignal,
  ) => {
    const q = new URLSearchParams()
    if (params.profile) q.set('profile', params.profile)
    q.set('cluster', params.cluster)
    if (params.secret) q.set('secret', params.secret)
    q.set('database', params.database)
    return api.get<Schema>(`/api/schema?${q}`, signal)
  },
  refreshSchema: (body: SchemaRefreshBody, signal?: AbortSignal) =>
    api.post<Schema>('/api/schema/refresh', body, signal),

  listHistory: (profile: string, database: string, signal?: AbortSignal) => {
    const q = new URLSearchParams({ profile, database })
    return api.get<History>(`/api/history?${q}`, signal)
  },
  setFavorite: (body: FavoriteBody, signal?: AbortSignal) =>
    api.post<void>('/api/history/favorite', body, signal),

  listModels: (profile: string, signal?: AbortSignal) =>
    api.get<Models>(`/api/ai/models?profile=${encodeURIComponent(profile)}`, signal),
  ask: (body: AskRequestBody, signal?: AbortSignal) =>
    api.post<AskResponseBody>('/api/ai/ask', body, signal),
  explain: (body: ExplainRequestBody, signal?: AbortSignal) =>
    api.post<TextResponse>('/api/ai/explain', body, signal),
  review: (body: ReviewRequestBody, signal?: AbortSignal) =>
    api.post<TextResponse>('/api/ai/review', body, signal),
  analyze: (body: AnalyzeRequestBody, signal?: AbortSignal) =>
    api.post<TextResponse>('/api/ai/analyze', body, signal),
}
