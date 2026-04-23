import type { ApiErrorPayload } from './types'
import { getAPIToken } from './session-token'

/**
 * ApiError preserves both the HTTP status and the server's structured
 * error code so call sites can branch on `err.code === 'origin_denied'`
 * instead of sniffing substrings. The constructor tolerates non-JSON
 * bodies by falling back to the HTTP status text.
 */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly payload: unknown

  constructor(status: number, code: string, message: string, payload: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.payload = payload
  }
}

type HTTPMethod = 'GET' | 'POST' | 'PUT' | 'DELETE'

interface RequestOptions {
  method?: HTTPMethod
  body?: unknown
  signal?: AbortSignal
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const token = getAPIToken()
  const res = await fetch(path, {
    method: opts.method ?? 'GET',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { 'X-RDQ-Token': token } : {}),
    },
    body: opts.body != null ? JSON.stringify(opts.body) : undefined,
    credentials: 'same-origin',
    signal: opts.signal,
  })
  if (res.status === 204) {
    // The caller expected no body; return undefined as T for convenience.
    return undefined as T
  }
  const text = await res.text()
  let parsed: unknown = null
  if (text.length > 0) {
    try {
      parsed = JSON.parse(text)
    } catch {
      // Fall through — non-JSON bodies are still valuable for the error
      // path below.
    }
  }
  if (!res.ok) {
    const err = isErrorPayload(parsed) ? parsed.error : null
    throw new ApiError(
      res.status,
      err?.code ?? `http_${res.status}`,
      err?.message ?? res.statusText,
      parsed,
    )
  }
  return parsed as T
}

function isErrorPayload(v: unknown): v is ApiErrorPayload {
  return (
    typeof v === 'object' &&
    v !== null &&
    'error' in v &&
    typeof (v as { error: unknown }).error === 'object'
  )
}

export const api = {
  get<T>(path: string, signal?: AbortSignal) {
    return request<T>(path, { method: 'GET', signal })
  },
  post<T>(path: string, body: unknown, signal?: AbortSignal) {
    return request<T>(path, { method: 'POST', body, signal })
  },
  put<T>(path: string, body: unknown, signal?: AbortSignal) {
    return request<T>(path, { method: 'PUT', body, signal })
  },
}
