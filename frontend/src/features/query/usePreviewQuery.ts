import { useQuery } from '@tanstack/react-query'

import { endpoints } from '@/lib/api/endpoints'
import type { ExecuteResponseBody } from '@/lib/api/types'
import type { PreviewFilter, PreviewSort } from '@/stores/uiStore'

export const PREVIEW_PAGE_SIZE = 100

/**
 * SQL DIALECT NOTE
 *
 * Identifier quoting (`"foo"`) and the `ILIKE` text-match operator are
 * PostgreSQL-only. On Aurora MySQL these emissions will fail. The
 * preview surface is Postgres-first by design; engine-aware emission is
 * tracked but not yet implemented. Schema/table names sourced from
 * information_schema are well-formed identifiers so the quoting fall-
 * back rarely fires in practice.
 */

function safeIdent(s: string): string {
  return /^[a-zA-Z_][a-zA-Z0-9_]*$/.test(s) ? s : `"${s.replace(/"/g, '""')}"`
}

/**
 * quoteValue keeps numeric / boolean literals unquoted so the server-
 * side type coerces them naturally; everything else falls back to a
 * single-quoted string. SQL injection risk is bounded — these filters
 * originate from the same authenticated user who can run any SQL via
 * the editor — but we still escape single quotes so a literal
 * apostrophe in user input doesn't blow up parsing.
 */
function quoteValue(v: string): string {
  const trimmed = v.trim()
  if (/^-?\d+(?:\.\d+)?$/.test(trimmed)) return trimmed
  if (trimmed === 'true' || trimmed === 'false') return trimmed
  return `'${v.replace(/'/g, "''")}'`
}

/**
 * escapeLikePattern escapes the value's `'`, `%`, `_`, and `\` so a
 * literal user input like `10%` filters for the string "10%" instead
 * of "10 followed by anything". The emitter pairs this with `ESCAPE '\'`
 * on the predicate so backslash drives the escape.
 */
function escapeLikePattern(v: string): string {
  return v
    .replace(/\\/g, '\\\\')
    .replace(/%/g, '\\%')
    .replace(/_/g, '\\_')
    .replace(/'/g, "''")
}

function buildPredicate(f: PreviewFilter): string {
  const col = safeIdent(f.column)
  const op = f.op
  if (op === 'is null') return `${col} IS NULL`
  if (op === 'is not null') return `${col} IS NOT NULL`
  if (op === 'contains' || op === 'starts with' || op === 'ends with') {
    const esc = escapeLikePattern(f.value)
    const pat =
      op === 'contains' ? `%${esc}%` : op === 'starts with' ? `${esc}%` : `%${esc}`
    return `${col} ILIKE '${pat}' ESCAPE '\\'`
  }
  return `${col} ${op} ${quoteValue(f.value)}`
}

function buildWhereClause(filters: PreviewFilter[]): string {
  if (filters.length === 0) return ''
  return ` WHERE ${filters.map(buildPredicate).join(' AND ')}`
}

/**
 * buildPreviewSQL returns the SELECT statement used by the table
 * preview overlay. Filters become WHERE predicates ANDed together; sort
 * becomes ORDER BY. See the SQL DIALECT NOTE at the top of this file
 * for engine compatibility.
 */
export function buildPreviewSQL(
  schema: string,
  table: string,
  offset: number,
  sort: PreviewSort = null,
  filters: PreviewFilter[] = [],
): string {
  const where = buildWhereClause(filters)
  const orderBy = sort
    ? ` ORDER BY ${safeIdent(sort.column)} ${sort.dir.toUpperCase()}`
    : ''
  return `SELECT * FROM ${safeIdent(schema)}.${safeIdent(table)}${where}${orderBy} LIMIT ${PREVIEW_PAGE_SIZE} OFFSET ${offset}`
}

/**
 * buildCountSQL returns the COUNT(*) query for the same table+filters
 * the page query uses, so the footer can show "N records" against the
 * filter set.
 */
export function buildCountSQL(
  schema: string,
  table: string,
  filters: PreviewFilter[] = [],
): string {
  const where = buildWhereClause(filters)
  return `SELECT COUNT(*) AS total FROM ${safeIdent(schema)}.${safeIdent(table)}${where}`
}

interface PreviewArgs {
  profile: string
  cluster: string
  secret: string
  database: string
  schema: string
  table: string
  offset: number
  sort: PreviewSort
  filters: PreviewFilter[]
  enabled: boolean
}

interface CountArgs {
  profile: string
  cluster: string
  secret: string
  database: string
  schema: string
  table: string
  filters: PreviewFilter[]
  enabled: boolean
}

function filterCacheKey(filters: PreviewFilter[]): string {
  return filters.map((f) => `${f.column}|${f.op}|${f.value}`).join('&')
}

/**
 * Cache-key prefixes used by both the data and count queries, exported
 * so refresh handlers can invalidate either family without re-deriving
 * the shape.
 */
export const PREVIEW_QUERY_KEY = 'preview' as const
export const PREVIEW_COUNT_QUERY_KEY = 'preview-count' as const

/**
 * usePreviewQuery fetches a 100-row slice of a table for the preview
 * overlay. It piggybacks on POST /api/execute so the read-only / confirm
 * gates and renderer pipeline are identical to a hand-typed SELECT —
 * destructive-statement gating does not fire because the SQL is always
 * a plain SELECT.
 */
export function usePreviewQuery(args: PreviewArgs) {
  const {
    profile,
    cluster,
    secret,
    database,
    schema,
    table,
    offset,
    sort,
    filters,
    enabled,
  } = args
  const sql = buildPreviewSQL(schema, table, offset, sort, filters)
  const sortKey = sort ? `${sort.column}:${sort.dir}` : ''
  return useQuery<ExecuteResponseBody, Error>({
    queryKey: [
      PREVIEW_QUERY_KEY,
      cluster,
      database,
      schema,
      table,
      offset,
      sortKey,
      filterCacheKey(filters),
    ],
    queryFn: ({ signal }) =>
      endpoints.execute({ profile, cluster, secret, database, sql }, signal),
    enabled,
    staleTime: 0,
    gcTime: 30_000,
    retry: false,
  })
}

/**
 * usePreviewCount fetches the total row count for the preview's table
 * + filter set. Kept separate from usePreviewQuery so the data grid
 * can render the moment rows arrive without waiting for COUNT(*) on
 * a large table to come back.
 */
export function usePreviewCount(args: CountArgs) {
  const { profile, cluster, secret, database, schema, table, filters, enabled } = args
  const sql = buildCountSQL(schema, table, filters)
  return useQuery<number, Error>({
    queryKey: [
      PREVIEW_COUNT_QUERY_KEY,
      cluster,
      database,
      schema,
      table,
      filterCacheKey(filters),
    ],
    queryFn: async ({ signal }) => {
      const res = await endpoints.execute(
        { profile, cluster, secret, database, sql },
        signal,
      )
      const cell = res.rows[0]?.[0]
      const n = typeof cell === 'number' ? cell : Number(cell)
      return Number.isFinite(n) ? n : 0
    },
    enabled,
    staleTime: 30_000,
    gcTime: 60_000,
    retry: false,
  })
}
