import { useMemo, useState } from 'react'
import {
  ChevronRight,
  Columns3,
  Database as DatabaseIcon,
  Eye,
  Table as TableIcon,
} from 'lucide-react'
import { toast } from 'sonner'

import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
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

type SchemaGroup = readonly [string, GroupedTable[]]

interface SchemaSidebarProps {
  /**
   * tableClickAction controls what a bare table row click does:
   *   - 'preview' (default): open the Supabase-style preview overlay
   *     (`SELECT * FROM schema.table LIMIT 100`) — used by the table
   *     editor on /query.
   *   - 'insert':  append the bare table name to the SQL editor
   *     buffer — used by /sql, where the click pattern is mostly
   *     "I want this identifier in my query."
   *
   * In both modes, ⌘ / Ctrl-click always inserts so the keyboard
   * shortcut behaves consistently across pages. The hover-to-reveal
   * `<>` button always opens the column-list popover regardless of
   * mode.
   */
  tableClickAction?: 'preview' | 'insert'
}

/**
 * SchemaSidebar is the flat schema browser shared by the table editor
 * and SQL editor pages. Schema headers fold; table rows are flat (no
 * inline column accordion — columns live behind the per-row popover).
 */
export function SchemaSidebar({ tableClickAction = 'preview' }: SchemaSidebarProps = {}) {
  const session = useSession()
  const schema = useSchema({
    profile: session.data?.profile ?? '',
    cluster: session.data?.cluster ?? '',
    secret: session.data?.secret ?? '',
    database: session.data?.database ?? '',
  })
  const requestEditorText = useUIStore((s) => s.requestEditorText)
  const currentSql = useUIStore((s) => s.sql)
  const openPreview = useUIStore((s) => s.openPreview)
  const previewTarget = useUIStore((s) => s.previewTarget)

  const [filter, setFilter] = useState('')
  const [closedSchemas, setClosedSchemas] = useState<Set<string>>(new Set())

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

  const schemaGroups = useMemo<SchemaGroup[]>(() => {
    const bySchema = new Map<string, GroupedTable[]>()
    for (const t of tables) {
      const list = bySchema.get(t.schema) ?? []
      list.push(t)
      bySchema.set(t.schema, list)
    }
    return Array.from(bySchema.entries()).sort(([a], [b]) => a.localeCompare(b))
  }, [tables])

  const filteredGroups = useMemo<SchemaGroup[]>(() => {
    if (!filter) return schemaGroups
    const needle = filter.toLowerCase()
    const out: SchemaGroup[] = []
    for (const [schemaName, schemaTables] of schemaGroups) {
      if (schemaName.toLowerCase().includes(needle)) {
        out.push([schemaName, schemaTables])
        continue
      }
      const matched = schemaTables.filter(
        (t) =>
          t.table.toLowerCase().includes(needle) ||
          t.columns.some((c) => c.name.toLowerCase().includes(needle)),
      )
      if (matched.length > 0) out.push([schemaName, matched])
    }
    return out
  }, [schemaGroups, filter])

  if (!sessionIsComplete(session.data)) {
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

  const filterActive = filter.length > 0

  function handleTableClick(t: GroupedTable, e: React.MouseEvent | React.KeyboardEvent) {
    const modifier = ('metaKey' in e && e.metaKey) || ('ctrlKey' in e && e.ctrlKey)
    if (tableClickAction === 'insert' || modifier) {
      requestEditorText(appendToken(currentSql, t.table))
      toast.success(`Inserted "${t.table}"`)
      return
    }
    openPreview(t.schema, t.table)
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
        {!schema.isLoading && filteredGroups.length === 0 && (
          <div className="p-3 text-xs text-muted-foreground">No tables.</div>
        )}
        <ul className="space-y-0.5 p-2 text-xs">
          {filteredGroups.map(([schemaName, schemaTables]) => {
            const schemaOpen = filterActive || !closedSchemas.has(schemaName)
            const totalCols = schemaTables.reduce(
              (sum, t) => sum + t.columns.length,
              0,
            )
            return (
              <li key={schemaName}>
                <button
                  type="button"
                  onClick={() => {
                    setClosedSchemas((prev) => {
                      const next = new Set(prev)
                      if (next.has(schemaName)) next.delete(schemaName)
                      else next.add(schemaName)
                      return next
                    })
                  }}
                  className={cn(
                    'flex w-full items-center gap-1 rounded px-1 py-0.5 text-left',
                    'hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:outline-none',
                  )}
                >
                  <ChevronRight
                    className={cn(
                      'size-3.5 shrink-0 text-muted-foreground transition-transform',
                      schemaOpen && 'rotate-90',
                    )}
                  />
                  <DatabaseIcon className="size-3.5 shrink-0 text-muted-foreground" />
                  <span className="truncate font-medium">
                    <HighlightedText text={schemaName} term={filter} />
                  </span>
                  <span className="ml-auto shrink-0 text-[10px] tabular-nums text-muted-foreground">
                    {schemaTables.length}·{totalCols}
                  </span>
                </button>
                {schemaOpen && (
                  <ul className="ml-4 mt-0.5 space-y-0.5 border-l border-border/50 pl-2">
                    {schemaTables.map((t) => {
                      const key = `${t.schema}.${t.table}`
                      const isActive =
                        previewTarget?.schema === t.schema &&
                        previewTarget?.table === t.table
                      return (
                        <li key={key}>
                          <TableRow
                            table={t}
                            filter={filter}
                            isActive={isActive}
                            onClick={(e) => handleTableClick(t, e)}
                          />
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
      <div className="border-t border-border px-3 py-2 text-[10px] leading-tight text-muted-foreground">
        {tableClickAction === 'insert'
          ? 'Click table to insert'
          : 'Click table to preview · ⌘-click to insert'}
      </div>
    </aside>
  )
}

function TableRow({
  table,
  filter,
  isActive,
  onClick,
}: {
  table: GroupedTable
  filter: string
  isActive: boolean
  onClick: (e: React.MouseEvent | React.KeyboardEvent) => void
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onClick(e)
        }
      }}
      className={cn(
        'group relative flex w-full items-center gap-1 rounded px-1 py-0.5 text-left cursor-pointer',
        'hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:outline-none',
        isActive && 'bg-primary/15 text-foreground ring-1 ring-primary/30',
      )}
    >
      {isActive ? (
        <Eye className="size-3.5 shrink-0 text-primary" />
      ) : (
        <TableIcon className="size-3.5 shrink-0 text-muted-foreground" />
      )}
      <span className="truncate">
        <HighlightedText text={table.table} term={filter} />
      </span>
      <span className="ml-auto shrink-0 text-[10px] tabular-nums text-muted-foreground">
        {table.columns.length}
      </span>
      <div
        className={cn(
          'flex shrink-0 items-center gap-0.5 transition-opacity',
          isActive
            ? 'opacity-100'
            : 'opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100',
        )}
      >
        <ColumnsPopover table={table} />
      </div>
    </div>
  )
}

function ColumnsPopover({ table }: { table: GroupedTable }) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          title={`Columns of ${table.table}`}
          aria-label={`Columns of ${table.table}`}
          onClick={(e) => e.stopPropagation()}
          className="flex size-5 items-center justify-center rounded text-muted-foreground hover:bg-background hover:text-foreground"
        >
          <Columns3 className="size-3.5" />
        </button>
      </PopoverTrigger>
      <PopoverContent
        side="right"
        align="start"
        sideOffset={8}
        className="w-72 p-0"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="border-b border-border px-3 py-2 text-xs font-medium">
          {table.schema}.{table.table}
          <span className="ml-2 text-muted-foreground">{table.columns.length} cols</span>
        </div>
        <div className="max-h-72 overflow-auto">
          <ul className="divide-y divide-border/60 text-xs">
            {table.columns.map((c) => (
              <li key={c.name} className="flex items-baseline gap-2 px-3 py-1.5">
                <span className="font-mono">{c.name}</span>
                <span className="ml-auto text-[11px] text-muted-foreground">{c.type}</span>
              </li>
            ))}
          </ul>
        </div>
      </PopoverContent>
    </Popover>
  )
}

function appendToken(current: string, token: string): string {
  if (!current) return token
  if (current.endsWith(' ') || current.endsWith('\n')) return current + token
  return current + ' ' + token
}
