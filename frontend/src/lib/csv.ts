/**
 * toCSV serializes an execute response into an RFC 4180-ish CSV string.
 *   - NULL cells become the empty field.
 *   - Numbers, booleans, strings are passed through String().
 *   - Arrays are rendered as a JSON-like bracketed list matching the Go
 *     server's FormatCell output so the two stay consistent.
 *   - Cells containing commas, quotes, CR, or LF are wrapped in double
 *     quotes; embedded quotes are escaped by doubling.
 *
 * Blobs arrive from the server base64-encoded as a string, so they fall
 * through the string branch without special handling.
 */
export function toCSV(columns: string[], rows: unknown[][]): string {
  const lines: string[] = []
  lines.push(columns.map(escape).join(','))
  for (const row of rows) {
    const cells = columns.map((_, i) => escape(formatCell(row[i])))
    lines.push(cells.join(','))
  }
  return lines.join('\n') + '\n'
}

function formatCell(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'number' || typeof v === 'bigint') return String(v)
  if (typeof v === 'boolean') return String(v)
  if (typeof v === 'string') return v
  if (Array.isArray(v)) {
    const parts = v.map((x) => {
      if (typeof x === 'string') return JSON.stringify(x)
      return formatCell(x)
    })
    return `[${parts.join(', ')}]`
  }
  // Objects or unknown types → JSON.stringify keeps them inspectable.
  try {
    return JSON.stringify(v)
  } catch {
    return String(v)
  }
}

function escape(field: string): string {
  if (/[",\r\n]/.test(field)) {
    return '"' + field.replaceAll('"', '""') + '"'
  }
  return field
}

/**
 * downloadCSV triggers a browser download of the given CSV content as
 * `rdq-<timestamp>.csv`. Using an object URL avoids an intermediate
 * Blob round-trip and lets the browser pick a filename the user sees.
 */
export function downloadCSV(filenameBase: string, csv: string) {
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  try {
    const a = document.createElement('a')
    a.href = url
    a.download = `${filenameBase}-${timestamp()}.csv`
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    a.remove()
  } finally {
    URL.revokeObjectURL(url)
  }
}

function timestamp() {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return (
    d.getFullYear().toString() +
    pad(d.getMonth() + 1) +
    pad(d.getDate()) +
    '-' +
    pad(d.getHours()) +
    pad(d.getMinutes()) +
    pad(d.getSeconds())
  )
}

