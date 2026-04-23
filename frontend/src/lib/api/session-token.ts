const storageKey = 'rdq:api-token'
const launchTokenParam = 'rdq-token'

let cachedToken: string | null = null

/**
 * bootstrapAPIToken imports the one-time launch token from the URL fragment
 * the Go server opened in the browser, persists it in sessionStorage for
 * same-tab reloads, and strips it back out of the visible address bar.
 */
export function bootstrapAPIToken(): boolean {
  if (typeof window === 'undefined') return false

  const fromHash = new URLSearchParams(window.location.hash.replace(/^#/, '')).get(
    launchTokenParam,
  )
  if (fromHash) {
    cachedToken = fromHash
    try {
      window.sessionStorage.setItem(storageKey, fromHash)
    } catch {
      // Fall back to the in-memory copy; a single page lifetime is still
      // enough for the current GUI run.
    }
    window.history.replaceState(
      window.history.state,
      '',
      window.location.pathname + window.location.search,
    )
    return true
  }

  try {
    const stored = window.sessionStorage.getItem(storageKey)
    if (stored) {
      cachedToken = stored
      return true
    }
  } catch {
    // Ignore storage failures; the missing-token screen will render below.
  }
  return false
}

export function getAPIToken(): string | null {
  if (cachedToken) return cachedToken
  if (typeof window === 'undefined') return null
  try {
    cachedToken = window.sessionStorage.getItem(storageKey)
  } catch {
    return null
  }
  return cachedToken
}
