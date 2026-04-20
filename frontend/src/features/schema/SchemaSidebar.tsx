import { useMemo, useState } from 'react'
import { ChevronRight, Copy, Table as TableIcon } from 'lucide-react'
import { toast } from 'sonner'

import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { HighlightedText } from '@/features/query/HighlightedText'
import { sessionIsComplete, useSession } from '@/hooks/useSession'
import { cn } from '@/lib/utils'
import { useUIStore } from '@/stores/uiStore'

import { useSchema } from './useSchema'

interface GroupedTable {
  schema: string
  table: string
  columns: { name: string; type: string }[]
}

/**
 * SchemaSidebar is the left pane inside QueryPage that lets users browse
 * information_schema.
 *
 * Interactions:
 *   - Click a table row: expand / collapse its column list.
 *   - Double-click a table / column: append the qualified identifier to
 *     the editor buffer via pendingEditorText.
 *   - Select (click) a column to reveal a Copy button on the right, or
 *     hover any row to see the same button. Copying emits `table.column`
 *     for columns and the bare table name for table rows so the text is
 *     ready to paste into a SELECT.
 */
export function SchemaSidebar() {
  const session = useSession()
  const schema = useSchema({
    profile: session.data?.profile ?? '',
    cluster: session.data?.cluster ?? '',
    secret: session.data?.secret ?? '',
    database: session.data?.database ?? '',
  })
  const requestEditorText = useUIStore((s) => s.requestEditorText)
  const currentSql = useUIStore((s) => s.sql)

  const [filter, setFilter] = useState('')
  const [openTables, setOpenTables] = useState<Set<string>>(new Set())
  const [selected, setSelected] = useState<string | null>(null)

  const tables = useMemo<GroupedTable[]>(() => {
    const cols = schema.data?.columns ?? []
    const acc = new Map<string, GroupedTable>()
    for (const c of cols) {
      const key = `${c.schema}.${c.table}`
      let g = acc.get(key)
      if (!g) {
        g = { schema: c.schema, table: c.table, columns: [] }
        acc.set(key, g)
      }
      g.columns.push({ name: c.name, type: c.type })
    }
    return Array.from(acc.values()).sort((a, b) =>
      `${a.schema}.${a.table}`.localeCompare(`${b.schema}.${b.table}`),
    )
  }, [schema.data])

  const filtered = useMemo(() => {
    if (!filter) return tables
    const needle = filter.toLowerCase()
    return tables.filter(
      (t) =>
        t.table.toLowerCase().includes(needle) ||
        t.schema.toLowerCase().includes(needle) ||
        t.columns.some((c) => c.name.toLowerCase().includes(needle)),
    )
  }, [tables, filter])

  if (!sessionIsComplete(session.data)) {
    // Keep the aside structure so the surrounding PanelGroup has stable
    // children — otherwise mounting it later would reset the user's
    // chosen split sizes.
    return (
      <aside className="flex h-full w-full flex-col border-r border-border bg-card">
        <div className="flex items-center gap-2 border-b border-border px-3 py-2">
          <h2 className="text-xs font-semibold tracking-wide text-muted-foreground">
            Schema
          </h2>
        </div>
        <div className="flex flex-1 items-center justify-center p-3 text-xs text-muted-foreground">
          Pick a connection to see tables.
        </div>
      </aside>
    )
  }

  async function copyToken(token: string) {
    try {
      await navigator.clipboard.writeText(token)
      toast.success(`Copied "${token}"`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Copy failed')
    }
  }

  return (
    <aside className="flex h-full w-full flex-col border-r border-border bg-card">
      <div className="flex items-center gap-2 border-b border-border px-3 py-2">
        <h2 className="text-xs font-semibold tracking-wide text-muted-foreground">
          Schema
        </h2>
        {schema.data?.fromCache && (
          <span className="text-[10px] text-muted-foreground">(cached)</span>
        )}
      </div>
      <div className="border-b border-border px-3 py-2">
        <Input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Filter…"
          className="h-7 text-xs"
        />
      </div>
      <ScrollArea className="min-h-0 flex-1">
        {schema.isLoading && (
          <div className="p-3 text-xs text-muted-foreground">Loading…</div>
        )}
        {schema.isError && (
          <div className="p-3 text-xs text-destructive">
            {(schema.error as Error).message}
          </div>
        )}
        {!schema.isLoading && filtered.length === 0 && (
          <div className="p-3 text-xs text-muted-foreground">No tables.</div>
        )}
        <ul className="space-y-0.5 p-2 text-xs">
          {filtered.map((t) => {
            const key = `${t.schema}.${t.table}`
            const open = openTables.has(key)
            return (
              <li key={key}>
                <Row
                  selected={selected === key}
                  onSelect={() => setSelected(key)}
                  onDoubleClick={() =>
                    requestEditorText(appendToken(currentSql, t.table))
                  }
                  onToggle={() => {
                    setOpenTables((prev) => {
                      const next = new Set(prev)
                      if (next.has(key)) next.delete(key)
                      else next.add(key)
                      return next
                    })
                  }}
                  onCopy={() => copyToken(t.table)}
                  copyTitle={`Copy "${t.table}"`}
                >
                  <ChevronRight
                    className={cn(
                      'size-3.5 shrink-0 text-muted-foreground transition-transform',
                      open && 'rotate-90',
                    )}
                  />
                  <TableIcon className="size-3.5 shrink-0 text-muted-foreground" />
                  <span className="truncate">
                    <HighlightedText text={t.table} term={filter} />
                  </span>
                  <span className="ml-auto shrink-0 text-[10px] tabular-nums text-muted-foreground">
                    {t.columns.length}
                  </span>
                </Row>
                {open && (
                  <ul className="ml-5 mt-0.5 space-y-0.5">
                    {t.columns.map((c) => {
                      const colKey = `${key}.${c.name}`
                      const qualified = `${t.table}.${c.name}`
                      return (
                        <li key={c.name}>
                          <Row
                            selected={selected === colKey}
                            onSelect={() => setSelected(colKey)}
                            onDoubleClick={() =>
                              requestEditorText(appendToken(currentSql, qualified))
                            }
                            onCopy={() => copyToken(qualified)}
                            copyTitle={`Copy "${qualified}"`}
                          >
                            <span className="truncate">
                              <HighlightedText text={c.name} term={filter} />
                            </span>
                            <span className="ml-auto shrink-0 text-[10px] tabular-nums text-muted-foreground">
                              {c.type}
                            </span>
                          </Row>
                        </li>
                      )
                    })}
                  </ul>
                )}
              </li>
            )
          })}
        </ul>
      </ScrollArea>
      <div className="border-t border-border px-3 py-2 text-[10px] text-muted-foreground">
        Double-click to insert · click to select &amp; copy
      </div>
    </aside>
  )
}

function Row({
  selected,
  onSelect,
  onToggle,
  onDoubleClick,
  onCopy,
  copyTitle,
  children,
}: {
  selected: boolean
  onSelect: () => void
  onToggle?: () => void
  onDoubleClick: () => void
  onCopy: () => void
  copyTitle: string
  children: React.ReactNode
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => {
        onSelect()
        onToggle?.()
      }}
      onDoubleClick={onDoubleClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onSelect()
          onToggle?.()
        }
      }}
      className={cn(
        'group flex w-full items-center gap-1 rounded px-1 py-0.5 text-left cursor-pointer',
        'hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:outline-none',
        selected && 'bg-accent text-accent-foreground',
      )}
    >
      {children}
      <button
        type="button"
        title={copyTitle}
        aria-label={copyTitle}
        onClick={(e) => {
          e.stopPropagation()
          onCopy()
        }}
        className={cn(
          'ml-1 flex size-5 shrink-0 items-center justify-center rounded text-muted-foreground transition-opacity',
          'hover:bg-background hover:text-foreground',
          // Always visible when selected; otherwise fade in on hover.
          selected ? 'opacity-100' : 'opacity-0 group-hover:opacity-100 focus-visible:opacity-100',
        )}
      >
        <Copy className="size-3.5" />
      </button>
    </div>
  )
}

function appendToken(current: string, token: string): string {
  if (!current) return token
  if (current.endsWith(' ') || current.endsWith('\n')) return current + token
  return current + ' ' + token
}
