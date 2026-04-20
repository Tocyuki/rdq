import { useMemo } from 'react'
import { Link } from 'react-router-dom'

import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { sessionIsComplete, useSession } from '@/hooks/useSession'
import { useState } from 'react'

import { useSchema } from './useSchema'

/**
 * SchemaPage renders the whole information_schema as a searchable table.
 * SchemaSidebar on /query covers the "insert column into editor" flow;
 * this page is the destination when the user wants a broad view of
 * tables and columns in one place.
 */
export function SchemaPage() {
  const session = useSession()
  const schema = useSchema({
    profile: session.data?.profile ?? '',
    cluster: session.data?.cluster ?? '',
    secret: session.data?.secret ?? '',
    database: session.data?.database ?? '',
  })
  const [filter, setFilter] = useState('')

  const filtered = useMemo(() => {
    const cols = schema.data?.columns ?? []
    if (!filter) return cols
    const needle = filter.toLowerCase()
    return cols.filter(
      (c) =>
        c.schema.toLowerCase().includes(needle) ||
        c.table.toLowerCase().includes(needle) ||
        c.name.toLowerCase().includes(needle) ||
        c.type.toLowerCase().includes(needle),
    )
  }, [schema.data, filter])

  if (!sessionIsComplete(session.data)) {
    return (
      <section className="p-6 text-sm text-muted-foreground">
        Connect a profile, cluster, secret, and database first.
      </section>
    )
  }

  return (
    <section className="flex h-full flex-col">
      <header className="flex items-center gap-3 border-b border-border bg-card px-4 py-2">
        <h1 className="text-sm font-semibold tracking-tight">Schema</h1>
        <Input
          placeholder="Filter (schema / table / column / type)"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="h-8 max-w-sm"
        />
        <div className="flex-1" />
        <span className="text-xs text-muted-foreground">
          {filtered.length} / {schema.data?.columns.length ?? 0} columns
          {schema.data?.fromCache && ' · cached'}
        </span>
        <Link
          to="/query"
          className="text-xs text-muted-foreground underline underline-offset-2 hover:text-foreground"
        >
          ← Back to query
        </Link>
      </header>

      <ScrollArea className="min-h-0 flex-1">
        <table className="w-full border-collapse text-sm">
          <thead className="sticky top-0 bg-muted">
            <tr>
              <th className="border-b border-border px-3 py-2 text-left font-semibold text-muted-foreground">
                Schema
              </th>
              <th className="border-b border-border px-3 py-2 text-left font-semibold text-muted-foreground">
                Table
              </th>
              <th className="border-b border-border px-3 py-2 text-left font-semibold text-muted-foreground">
                Column
              </th>
              <th className="border-b border-border px-3 py-2 text-left font-semibold text-muted-foreground">
                Type
              </th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((c, idx) => (
              <tr key={idx} className="border-b border-border/70">
                <td className="px-3 py-1.5 font-mono text-[13px] text-muted-foreground">
                  {c.schema}
                </td>
                <td className="px-3 py-1.5 font-mono text-[13px]">{c.table}</td>
                <td className="px-3 py-1.5 font-mono text-[13px]">{c.name}</td>
                <td className="px-3 py-1.5 font-mono text-[13px] text-muted-foreground">
                  {c.type}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ScrollArea>
    </section>
  )
}
