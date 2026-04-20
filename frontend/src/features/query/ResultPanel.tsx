import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from 'react'
import { ChevronDown, ChevronUp, Search, X } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { ExecuteResponseBody } from '@/lib/api/types'

import { CopyMenu, ExportMenu } from './ExportMenu'
import { ResultTable } from './ResultTable'

interface Props {
  result: ExecuteResponseBody | null
  error: string | null
  loading: boolean
}

export interface ResultPanelHandle {
  /**
   * Opens the find-in-page bar and focuses the input. Used by QueryPage's
   * Cmd/Ctrl+F interceptor so the shortcut lands on the result grid even
   * when the editor does not have focus.
   */
  openSearch: () => void
}

export const ResultPanel = forwardRef<ResultPanelHandle, Props>(function ResultPanel(
  { result, error, loading },
  ref,
) {
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [matchCount, setMatchCount] = useState(0)
  const [activeMatch, setActiveMatch] = useState(-1)
  const inputRef = useRef<HTMLInputElement>(null)
  const scrollRootRef = useRef<HTMLDivElement>(null)

  const openSearch = useCallback(() => {
    setSearchOpen(true)
    // Focus after render; also select existing text so the user can retype
    // without hitting Backspace first — the common "re-find" flow.
    queueMicrotask(() => {
      inputRef.current?.focus()
      inputRef.current?.select()
    })
  }, [])

  useImperativeHandle(ref, () => ({ openSearch }), [openSearch])

  const closeSearch = useCallback(() => {
    setSearchOpen(false)
    setSearchTerm('')
    setMatchCount(0)
    setActiveMatch(-1)
  }, [])

  const stepMatch = useCallback(
    (dir: 1 | -1) => {
      if (matchCount === 0) return
      setActiveMatch((i) => {
        if (i < 0) return dir === 1 ? 0 : matchCount - 1
        return (i + dir + matchCount) % matchCount
      })
    },
    [matchCount],
  )

  // Wait for React to commit the new `data-active-match` before the
  // scrollIntoView lookup.
  useEffect(() => {
    if (activeMatch < 0) return
    const root = scrollRootRef.current
    if (!root) return
    const el = root.querySelector<HTMLElement>(
      `[data-match-index="${activeMatch}"]`,
    )
    el?.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'smooth' })
  }, [activeMatch])

  const trimmedTerm = searchTerm.trim()

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between gap-2 border-b border-border bg-card px-3 py-2">
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          {loading && <span>Running…</span>}
          {!loading && result && (
            <>
              <span>
                {result.rows.length} row{result.rows.length === 1 ? '' : 's'}
              </span>
              <span>·</span>
              <span>{result.durationMs} ms</span>
              {result.updated > 0 && (
                <>
                  <span>·</span>
                  <span>{result.updated} rows affected</span>
                </>
              )}
            </>
          )}
          {!loading && !result && !error && <span>No result yet.</span>}
        </div>
        <div className="flex items-center gap-2">
          {searchOpen && (
            <div className="flex items-center gap-1">
              <Search className="size-3.5 text-muted-foreground" />
              <Input
                ref={inputRef}
                value={searchTerm}
                onChange={(e) => {
                  setSearchTerm(e.target.value)
                  // Reset: the next Enter starts from match 0.
                  setActiveMatch(-1)
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Escape') {
                    e.preventDefault()
                    closeSearch()
                    return
                  }
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    stepMatch(e.shiftKey ? -1 : 1)
                  }
                }}
                placeholder="Find in result…"
                className="h-7 w-48 text-xs"
              />
              {trimmedTerm && (
                <span className="text-[11px] tabular-nums text-muted-foreground">
                  {matchCount === 0
                    ? 'No matches'
                    : `${activeMatch < 0 ? 0 : activeMatch + 1} / ${matchCount}`}
                </span>
              )}
              <Button
                size="icon"
                variant="ghost"
                className="size-7"
                onClick={() => stepMatch(-1)}
                disabled={matchCount === 0}
                title="Previous match (Shift+Enter)"
                aria-label="Previous match"
              >
                <ChevronUp className="size-3.5" />
              </Button>
              <Button
                size="icon"
                variant="ghost"
                className="size-7"
                onClick={() => stepMatch(1)}
                disabled={matchCount === 0}
                title="Next match (Enter)"
                aria-label="Next match"
              >
                <ChevronDown className="size-3.5" />
              </Button>
              <Button
                size="icon"
                variant="ghost"
                className="size-7"
                onClick={closeSearch}
                title="Close search (Esc)"
                aria-label="Close search"
              >
                <X className="size-3.5" />
              </Button>
            </div>
          )}
          {!searchOpen && (
            <Button
              size="icon"
              variant="ghost"
              className="size-7"
              onClick={openSearch}
              disabled={!result}
              title="Find in result (Cmd/Ctrl+F)"
              aria-label="Find in result"
            >
              <Search className="size-3.5" />
            </Button>
          )}
          <CopyMenu result={result} />
          <ExportMenu result={result} />
        </div>
      </div>

      {error && (
        <div className="border-b border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      <Tabs defaultValue="table" className="flex min-h-0 flex-1 flex-col">
        <TabsList className="mx-3 mt-2 self-start">
          <TabsTrigger value="table">Table</TabsTrigger>
          <TabsTrigger value="json">JSON</TabsTrigger>
          <TabsTrigger value="info">Info</TabsTrigger>
        </TabsList>
        <TabsContent value="table" className="min-h-0 flex-1">
          {result ? (
            <div ref={scrollRootRef} className="h-full">
              <ResultTable
                columns={result.columns}
                rows={result.rows}
                searchTerm={searchOpen ? trimmedTerm : ''}
                onMatchCount={setMatchCount}
                activeMatchIndex={activeMatch}
              />
            </div>
          ) : (
            <EmptyState />
          )}
        </TabsContent>
        <TabsContent value="json" className="min-h-0 flex-1 overflow-auto px-3 py-2">
          {result ? (
            <pre className="whitespace-pre-wrap break-all font-mono text-[13px]">
              {JSON.stringify(
                result.rows.map((row) =>
                  Object.fromEntries(result.columns.map((c, i) => [c, row[i]])),
                ),
                null,
                2,
              )}
            </pre>
          ) : (
            <EmptyState />
          )}
        </TabsContent>
        <TabsContent value="info" className="min-h-0 flex-1 px-3 py-2 text-sm">
          {result ? (
            <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1">
              <dt className="text-muted-foreground">Columns</dt>
              <dd>{result.columns.length}</dd>
              <dt className="text-muted-foreground">Rows</dt>
              <dd>{result.rows.length}</dd>
              <dt className="text-muted-foreground">Rows affected</dt>
              <dd>{result.updated}</dd>
              <dt className="text-muted-foreground">Duration</dt>
              <dd>{result.durationMs} ms</dd>
            </dl>
          ) : (
            <EmptyState />
          )}
        </TabsContent>
      </Tabs>
    </div>
  )
})

function EmptyState() {
  return (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
      Run a query (Cmd / Ctrl + Enter) to see results.
    </div>
  )
}
