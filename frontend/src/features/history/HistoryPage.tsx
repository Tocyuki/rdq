import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Copy, FileInput, Star, StarOff } from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useSession } from '@/hooks/useSession'
import { useUIStore } from '@/stores/uiStore'
import type { HistoryEntry } from '@/lib/api/types'
import { cn } from '@/lib/utils'

import { useHistory, useSetFavorite } from './useHistory'

/**
 * HistoryPage lists executed statements for the active (profile, database)
 * with a substring search, favourite toggle, and two explicit actions:
 *
 *   - Row click (or Load button): paste the SQL into the editor and
 *     navigate to /query. Execution is intentionally NOT triggered —
 *     the user confirms by pressing Cmd/Ctrl+Enter or the Run button.
 *   - Copy button: copy the SQL text to the OS clipboard.
 */
export function HistoryPage() {
  const session = useSession()
  const profile = session.data?.profile ?? ''
  const database = session.data?.database ?? ''
  const q = useHistory(profile, database)
  const favMut = useSetFavorite(profile, database)
  const requestEditorText = useUIStore((s) => s.requestEditorText)
  const navigate = useNavigate()

  const [filter, setFilter] = useState('')
  const [favouritesOnly, setFavouritesOnly] = useState(false)

  const filtered = useMemo(() => {
    const entries = q.data?.entries ?? []
    const needle = filter.trim().toLowerCase()
    return entries.filter((e) => {
      if (favouritesOnly && !e.favorite) return false
      if (needle && !e.sql.toLowerCase().includes(needle)) return false
      return true
    })
  }, [q.data, filter, favouritesOnly])

  if (!profile || !database) {
    return (
      <section className="p-6 text-sm text-muted-foreground">
        Select a profile and database to view history.
      </section>
    )
  }

  function loadIntoEditor(sql: string) {
    // autoRun intentionally false: paste only. User executes via Run
    // button or Cmd/Ctrl+Enter after reviewing the statement.
    requestEditorText(sql)
    toast.success('Loaded into editor')
    navigate('/query')
  }

  async function copyToClipboard(sql: string) {
    try {
      await navigator.clipboard.writeText(sql)
      toast.success('SQL copied to clipboard')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Copy failed')
    }
  }

  return (
    <section className="flex h-full flex-col">
      <header className="flex items-center gap-3 border-b border-border bg-card px-4 py-2">
        <h1 className="text-sm font-semibold tracking-tight">History</h1>
        <Input
          placeholder="Search SQL…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="h-8 max-w-sm"
        />
        <Button
          size="sm"
          variant={favouritesOnly ? 'default' : 'outline'}
          onClick={() => setFavouritesOnly((v) => !v)}
        >
          <Star />
          Favourites
        </Button>
        <div className="flex-1" />
        <span className="text-xs text-muted-foreground">
          {filtered.length} / {q.data?.entries.length ?? 0} entries
        </span>
      </header>

      {q.isLoading && <div className="p-6 text-sm text-muted-foreground">Loading…</div>}
      {q.isError && (
        <div className="p-6 text-sm text-destructive">
          {(q.error as Error).message}
        </div>
      )}

      <ScrollArea className="min-h-0 flex-1">
        <ul className="divide-y divide-border">
          {filtered.map((entry) => (
            <li key={entry.at}>
              <HistoryRow
                entry={entry}
                onFavorite={() =>
                  favMut.mutate({ at: entry.at, favorite: !entry.favorite })
                }
                onLoad={() => loadIntoEditor(entry.sql)}
                onCopy={() => copyToClipboard(entry.sql)}
              />
            </li>
          ))}
          {filtered.length === 0 && !q.isLoading && (
            <li className="p-6 text-center text-sm text-muted-foreground">
              No entries match the current filter.
            </li>
          )}
        </ul>
      </ScrollArea>
    </section>
  )
}

function HistoryRow({
  entry,
  onFavorite,
  onLoad,
  onCopy,
}: {
  entry: HistoryEntry
  onFavorite: () => void
  onLoad: () => void
  onCopy: () => void
}) {
  const when = new Date(entry.at)
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onLoad}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onLoad()
        }
      }}
      className={cn(
        'flex items-start gap-3 p-3 transition-colors',
        'hover:bg-muted/50 focus-visible:bg-muted/70 focus-visible:outline-none',
        'cursor-pointer',
      )}
      title="Click to load into editor"
    >
      <div
        className="flex flex-col items-center gap-1 pt-1"
        onClick={(e) => e.stopPropagation()}
      >
        <button
          type="button"
          title={entry.favorite ? 'Unfavourite' : 'Favourite'}
          onClick={onFavorite}
          className="text-muted-foreground hover:text-foreground"
        >
          {entry.favorite ? (
            <Star className="size-4 fill-current" />
          ) : (
            <StarOff className="size-4" />
          )}
        </button>
      </div>
      <div className="min-w-0 flex-1 overflow-hidden">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <time dateTime={entry.at}>{when.toLocaleString()}</time>
          <span>·</span>
          <span>{entry.durationMs} ms</span>
          <Badge variant={entry.ok ? 'secondary' : 'destructive'} className="ml-1">
            {entry.ok ? 'ok' : 'error'}
          </Badge>
        </div>
        <pre className="mt-1 whitespace-pre-wrap break-all font-mono text-[13px]">
          {entry.sql}
        </pre>
        {!entry.ok && entry.error && (
          <div className="mt-1 text-xs text-destructive">{entry.error}</div>
        )}
      </div>
      <div
        className="flex shrink-0 items-center gap-1"
        onClick={(e) => e.stopPropagation()}
      >
        <Button
          size="sm"
          variant="outline"
          onClick={onCopy}
          title="Copy SQL to clipboard"
        >
          <Copy />
          Copy
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={onLoad}
          title="Load into editor"
        >
          <FileInput />
          Load
        </Button>
      </div>
    </div>
  )
}
