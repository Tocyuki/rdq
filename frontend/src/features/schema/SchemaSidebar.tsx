import { useMemo, useState } from 'react'
import { ChevronRight, Table as TableIcon } from 'lucide-react'

import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
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
 * information_schema and double-click a column / table to insert its
 * qualified identifier into the editor via the UI store's
 * pendingEditorText slot.
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
    return null
  }

  return (
    <aside className="flex h-full w-64 flex-col border-r border-border bg-card">
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
                <button
                  className="flex w-full items-center gap-1 rounded px-1 py-0.5 text-left hover:bg-accent hover:text-accent-foreground"
                  onClick={() => {
                    setOpenTables((prev) => {
                      const next = new Set(prev)
                      if (next.has(key)) next.delete(key)
                      else next.add(key)
                      return next
                    })
                  }}
                  onDoubleClick={() => {
                    requestEditorText(appendToken(currentSql, t.table))
                  }}
                >
                  <ChevronRight
                    className={cn('size-3 transition-transform', open && 'rotate-90')}
                  />
                  <TableIcon className="size-3 text-muted-foreground" />
                  <span className="truncate">{t.table}</span>
                  <span className="ml-auto text-[10px] text-muted-foreground">
                    {t.columns.length}
                  </span>
                </button>
                {open && (
                  <ul className="ml-5 mt-0.5 space-y-0.5">
                    {t.columns.map((c) => (
                      <li key={c.name}>
                        <button
                          className="flex w-full items-center gap-2 rounded px-1 py-0.5 text-left hover:bg-accent hover:text-accent-foreground"
                          onDoubleClick={() => {
                            requestEditorText(
                              appendToken(currentSql, `${t.table}.${c.name}`),
                            )
                          }}
                        >
                          <span className="truncate">{c.name}</span>
                          <span className="ml-auto text-[10px] text-muted-foreground">
                            {c.type}
                          </span>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </li>
            )
          })}
        </ul>
      </ScrollArea>
      <div className="border-t border-border px-3 py-2 text-[10px] text-muted-foreground">
        Double-click a table or column to insert.
      </div>
    </aside>
  )
}

function appendToken(current: string, token: string): string {
  if (!current) return token
  if (current.endsWith(' ') || current.endsWith('\n')) return current + token
  return current + ' ' + token
}
