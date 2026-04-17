import type { Schema } from '@/lib/api/types'

/**
 * toSqlSchemaHint turns a Schema snapshot into the `schema: { table: [...] }`
 * shape that @codemirror/lang-sql expects when you pass its `schema`
 * option. It intentionally drops the schema name and indexes tables
 * by unqualified name because that is how users type them most often;
 * multi-schema databases can still reach `schema.table` via manual
 * completion.
 */
export function toSqlSchemaHint(snapshot: Schema | undefined): Record<string, string[]> {
  if (!snapshot) return {}
  const out: Record<string, string[]> = {}
  for (const col of snapshot.columns) {
    const key = col.table
    if (!out[key]) out[key] = []
    if (!out[key].includes(col.name)) out[key].push(col.name)
  }
  return out
}
