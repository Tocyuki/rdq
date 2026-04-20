/**
 * countMatches returns how many case-insensitive occurrences of `term`
 * appear in `text`. Exposed as a pure helper so callers that need a
 * running total (e.g. aggregating across every cell in a result grid)
 * can accumulate without rendering.
 */
export function countMatches(text: string, term: string): number {
  const needle = term.trim().toLowerCase()
  if (!needle) return 0
  const haystack = text.toLowerCase()
  let n = 0
  let cursor = 0
  while (cursor < haystack.length) {
    const idx = haystack.indexOf(needle, cursor)
    if (idx < 0) break
    n++
    cursor = idx + needle.length
  }
  return n
}
