import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  ChevronLeft,
  ChevronRight,
  Eye,
  Filter as FilterIcon,
  PencilLine,
  Plus,
  RefreshCw,
  X,
} from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { sessionIsComplete, useSession } from '@/hooks/useSession'
import { useSchema } from '@/features/schema/useSchema'
import { cn } from '@/lib/utils'
import {
  type PreviewFilter,
  type PreviewFilterOp,
  type PreviewSort,
  useUIStore,
} from '@/stores/uiStore'

import { ResultTable } from './ResultTable'
import {
  buildPreviewSQL,
  PREVIEW_COUNT_QUERY_KEY,
  PREVIEW_PAGE_SIZE,
  PREVIEW_QUERY_KEY,
  usePreviewCount,
  usePreviewQuery,
} from './usePreviewQuery'

const OPERATOR_GROUPS: Array<{
  label: string
  ops: Array<{ value: PreviewFilterOp; label: string; symbol?: string; needsValue: boolean }>
}> = [
  {
    label: 'Comparison',
    ops: [
      { value: '=', label: 'Equals', symbol: '=', needsValue: true },
      { value: '<>', label: 'Not equal', symbol: '<>', needsValue: true },
      { value: '>', label: 'Greater than', symbol: '>', needsValue: true },
      { value: '<', label: 'Less than', symbol: '<', needsValue: true },
      { value: '>=', label: 'Greater or equal', symbol: '>=', needsValue: true },
      { value: '<=', label: 'Less or equal', symbol: '<=', needsValue: true },
    ],
  },
  {
    label: 'Text',
    ops: [
      { value: 'contains', label: 'Contains', needsValue: true },
      { value: 'starts with', label: 'Starts with', needsValue: true },
      { value: 'ends with', label: 'Ends with', needsValue: true },
    ],
  },
  {
    label: 'Null checks',
    ops: [
      { value: 'is null', label: 'Is null', needsValue: false },
      { value: 'is not null', label: 'Is not null', needsValue: false },
    ],
  },
]

function operatorNeedsValue(op: PreviewFilterOp): boolean {
  for (const group of OPERATOR_GROUPS) {
    for (const o of group.ops) {
      if (o.value === op) return o.needsValue
    }
  }
  return true
}

function operatorLabel(op: PreviewFilterOp): string {
  for (const group of OPERATOR_GROUPS) {
    for (const o of group.ops) {
      if (o.value === op) return o.symbol ?? o.label
    }
  }
  return op
}

interface ColumnInfo {
  name: string
  type: string
}

/**
 * PreviewPanel is the Supabase / Drizzle Studio-style table viewer
 * shown on the /query route when previewTarget is non-null.
 * Layout (top → bottom):
 *   1. Title row    — table identity, refresh / Open-in-editor / close
 *   2. Filter row   — active filter chips + add-filter popover
 *   3. Sort row     — Sort popover + active-sort chip
 *   4. Data grid    — ResultTable
 *   5. Footer       — pagination controls and total record count
 *
 * Sort, filters, and pagination are all reflected in the SQL (ORDER BY,
 * WHERE, LIMIT/OFFSET) so the server returns an already-shaped result.
 */
export function PreviewPanel() {
  const session = useSession()
  const previewTarget = useUIStore((s) => s.previewTarget)
  const setPreviewOffset = useUIStore((s) => s.setPreviewOffset)
  const setPreviewSort = useUIStore((s) => s.setPreviewSort)
  const setPreviewFilters = useUIStore((s) => s.setPreviewFilters)
  const closePreview = useUIStore((s) => s.closePreview)
  const requestEditorText = useUIStore((s) => s.requestEditorText)
  const qc = useQueryClient()
  const navigate = useNavigate()
  const schema = useSchema({
    profile: session.data?.profile ?? '',
    cluster: session.data?.cluster ?? '',
    secret: session.data?.secret ?? '',
    database: session.data?.database ?? '',
  })

  const sessionReady = sessionIsComplete(session.data)

  const previewQuery = usePreviewQuery({
    profile: session.data?.profile ?? '',
    cluster: session.data?.cluster ?? '',
    secret: session.data?.secret ?? '',
    database: session.data?.database ?? '',
    schema: previewTarget?.schema ?? '',
    table: previewTarget?.table ?? '',
    offset: previewTarget?.offset ?? 0,
    sort: previewTarget?.sort ?? null,
    filters: previewTarget?.filters ?? [],
    enabled: sessionReady && previewTarget !== null,
  })

  const countQuery = usePreviewCount({
    profile: session.data?.profile ?? '',
    cluster: session.data?.cluster ?? '',
    secret: session.data?.secret ?? '',
    database: session.data?.database ?? '',
    schema: previewTarget?.schema ?? '',
    table: previewTarget?.table ?? '',
    filters: previewTarget?.filters ?? [],
    enabled: sessionReady && previewTarget !== null,
  })

  const result = previewQuery.data ?? null
  const totalCount = countQuery.data

  const targetSchema = previewTarget?.schema
  const targetTable = previewTarget?.table

  const tableColumns = useMemo<ColumnInfo[]>(() => {
    if (!targetSchema || !targetTable) return []
    const cols = schema.data?.columns ?? []
    return cols
      .filter((c) => c.schema === targetSchema && c.table === targetTable)
      .map((c) => ({ name: c.name, type: c.type }))
  }, [schema.data, targetSchema, targetTable])

  // Esc closes preview unless a dialog or input is in focus.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'Escape') return
      const active = document.activeElement as HTMLElement | null
      if (active?.closest('[role="dialog"]')) return
      if (active?.tagName === 'INPUT' || active?.tagName === 'TEXTAREA') return
      e.preventDefault()
      closePreview()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [closePreview])

  if (!previewTarget) return null
  const target = previewTarget
  const sql = buildPreviewSQL(target.schema, target.table, target.offset, target.sort, target.filters)
  const loading = previewQuery.isLoading || previewQuery.isFetching
  const error = previewQuery.error?.message ?? null
  const countError = countQuery.error?.message ?? null
  const canPrev = target.offset > 0
  // When the count is known, gate Next strictly on remaining rows so a
  // table whose size is exactly a multiple of PREVIEW_PAGE_SIZE doesn't
  // present an empty trailing page. Fall back to a "got a full page →
  // assume more" heuristic while count is still loading.
  const rowsOnPage = result?.rows.length ?? 0
  const canNext =
    totalCount !== undefined
      ? target.offset + rowsOnPage < totalCount
      : rowsOnPage >= PREVIEW_PAGE_SIZE

  function refetch() {
    // Invalidate both the page query and the count query — without the
    // count invalidation the footer keeps showing the pre-Refresh total
    // while the rows below have already updated.
    qc.invalidateQueries({
      queryKey: [PREVIEW_QUERY_KEY, session.data?.cluster, session.data?.database, target.schema, target.table],
    })
    qc.invalidateQueries({
      queryKey: [
        PREVIEW_COUNT_QUERY_KEY,
        session.data?.cluster,
        session.data?.database,
        target.schema,
        target.table,
      ],
    })
  }

  function openInEditor() {
    requestEditorText(sql)
    closePreview()
    navigate('/sql')
  }

  function addFilter(filter: PreviewFilter) {
    setPreviewFilters([...target.filters, filter])
  }

  function updateFilter(index: number, filter: PreviewFilter) {
    const next = target.filters.slice()
    next[index] = filter
    setPreviewFilters(next)
  }

  function removeFilter(index: number) {
    setPreviewFilters(target.filters.filter((_, i) => i !== index))
  }

  return (
    <section className="flex h-full min-w-0 flex-col bg-background">
      {/* Title row */}
      <header className="flex items-center justify-between gap-3 border-b border-border bg-card px-4 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <Eye className="size-4 shrink-0 text-primary" />
          <h1 className="truncate font-mono text-sm font-medium">
            {target.schema}.{target.table}
          </h1>
          {result && !loading && (
            <span className="shrink-0 text-xs text-muted-foreground">
              · {result.durationMs} ms
            </span>
          )}
        </div>
        <div className="flex items-center gap-1">
          <Button
            size="icon"
            variant="ghost"
            className="size-8"
            onClick={refetch}
            disabled={loading}
            title="Refresh"
            aria-label="Refresh"
          >
            <RefreshCw className={cn('size-4', loading && 'animate-spin')} />
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={openInEditor}
            title="Replace the SQL editor buffer with this SELECT and jump to /sql"
          >
            <PencilLine /> Open in editor
          </Button>
          <Button
            size="icon"
            variant="ghost"
            className="size-8"
            onClick={closePreview}
            title="Close preview (Esc)"
            aria-label="Close preview"
          >
            <X className="size-4" />
          </Button>
        </div>
      </header>

      {/* Filter row */}
      <div className="flex items-center gap-2 border-b border-border bg-card px-4 py-2">
        <FilterIcon className="size-3.5 shrink-0 text-muted-foreground" />
        <div className="flex flex-wrap items-center gap-1.5">
          {target.filters.map((f, idx) => (
            <FilterChip
              // Compose the key from the filter shape AND its position so
              // removing a chip in the middle of the row doesn't leak
              // popover state onto the chip that slides into its slot.
              key={`${idx}:${f.column}|${f.op}|${f.value}`}
              filter={f}
              columns={tableColumns}
              onChange={(next) => updateFilter(idx, next)}
              onRemove={() => removeFilter(idx)}
            />
          ))}
          <AddFilterButton columns={tableColumns} onAdd={addFilter} />
          {target.filters.length > 0 && (
            <button
              type="button"
              onClick={() => setPreviewFilters([])}
              className="text-[11px] text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
            >
              Clear all
            </button>
          )}
        </div>
      </div>

      {/* Sort row */}
      <div className="flex items-center gap-2 border-b border-border bg-card px-4 py-1.5">
        <SortPopover
          columns={tableColumns}
          sort={target.sort}
          onChange={setPreviewSort}
        />
        {target.sort && (
          <button
            type="button"
            onClick={() => setPreviewSort(null)}
            className="flex items-center gap-1 rounded-full border border-border bg-muted px-2 py-0.5 text-[11px] text-muted-foreground hover:text-foreground"
            title="Clear sort"
          >
            <span className="font-mono">{target.sort.column}</span>
            {target.sort.dir === 'asc' ? (
              <ArrowUp className="size-3" />
            ) : (
              <ArrowDown className="size-3" />
            )}
            <X className="size-3" />
          </button>
        )}
        <div className="flex-1" />
      </div>

      {error && (
        <div className="border-b border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      <div className="min-h-0 flex-1">
        {result ? (
          result.rows.length === 0 ? (
            <div className="flex h-full flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
              <span>No rows match these filters.</span>
              {target.filters.length > 0 && (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setPreviewFilters([])}
                >
                  Clear filters
                </Button>
              )}
            </div>
          ) : (
            <ResultTable columns={result.columns} rows={result.rows} />
          )
        ) : loading ? (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
            Loading {target.schema}.{target.table}…
          </div>
        ) : (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
            No data.
          </div>
        )}
      </div>

      <PaginationFooter
        offset={target.offset}
        rowsOnPage={rowsOnPage}
        totalCount={totalCount}
        countError={countError}
        loading={loading}
        countLoading={countQuery.isLoading || countQuery.isFetching}
        canPrev={canPrev}
        canNext={canNext}
        onSetOffset={setPreviewOffset}
      />
    </section>
  )
}

function PaginationFooter({
  offset,
  rowsOnPage,
  totalCount,
  countError,
  loading,
  countLoading,
  canPrev,
  canNext,
  onSetOffset,
}: {
  offset: number
  rowsOnPage: number
  totalCount: number | undefined
  countError: string | null
  loading: boolean
  countLoading: boolean
  canPrev: boolean
  canNext: boolean
  onSetOffset: (offset: number) => void
}) {
  const currentPage = Math.floor(offset / PREVIEW_PAGE_SIZE) + 1
  const totalPages =
    totalCount !== undefined
      ? Math.max(1, Math.ceil(totalCount / PREVIEW_PAGE_SIZE))
      : undefined
  const rangeStart = rowsOnPage > 0 ? offset + 1 : 0
  const rangeEnd = offset + rowsOnPage

  // Local "draft" page input so typing doesn't dispatch on every key.
  // Sync from offset → currentPage whenever offset changes externally
  // (e.g. filter / sort reset bumps the store back to page 1).
  const [draftPage, setDraftPage] = useState(String(currentPage))
  const [prevPage, setPrevPage] = useState(currentPage)
  if (currentPage !== prevPage) {
    setPrevPage(currentPage)
    setDraftPage(String(currentPage))
  }

  function commitPage() {
    const n = Math.max(1, Math.min(totalPages ?? Infinity, parseInt(draftPage, 10) || 1))
    setDraftPage(String(n))
    onSetOffset((n - 1) * PREVIEW_PAGE_SIZE)
  }

  return (
    <footer className="flex items-center justify-between gap-3 border-t border-border bg-card px-4 py-1.5 text-xs">
      <div className="flex items-center gap-1.5">
        <Button
          size="icon"
          variant="ghost"
          className="size-7"
          onClick={() => onSetOffset(Math.max(0, offset - PREVIEW_PAGE_SIZE))}
          disabled={!canPrev || loading}
          title="Previous page"
          aria-label="Previous page"
        >
          <ChevronLeft className="size-3.5" />
        </Button>
        <span className="text-muted-foreground">Page</span>
        <input
          type="text"
          inputMode="numeric"
          value={draftPage}
          onChange={(e) => setDraftPage(e.target.value.replace(/[^\d]/g, ''))}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              commitPage()
            }
          }}
          onBlur={commitPage}
          className="h-6 w-12 rounded border border-border bg-background px-1.5 text-center font-mono tabular-nums focus:outline-none focus:ring-1 focus:ring-ring"
          aria-label="Page number"
        />
        <span className="text-muted-foreground">
          of{' '}
          <span className="tabular-nums text-foreground">
            {totalPages !== undefined ? totalPages : '—'}
          </span>
        </span>
        <Button
          size="icon"
          variant="ghost"
          className="size-7"
          onClick={() => onSetOffset(offset + PREVIEW_PAGE_SIZE)}
          disabled={!canNext || loading}
          title="Next page"
          aria-label="Next page"
        >
          <ChevronRight className="size-3.5" />
        </Button>
        <span className="ml-1 rounded border border-border bg-muted px-1.5 py-0.5 tabular-nums text-muted-foreground">
          {PREVIEW_PAGE_SIZE} rows
        </span>
      </div>
      <div className="text-muted-foreground tabular-nums">
        {countLoading && totalCount === undefined ? (
          <span>Counting…</span>
        ) : totalCount !== undefined ? (
          <span>
            {rangeStart === 0 ? 0 : rangeStart}
            {rangeEnd > rangeStart && `–${rangeEnd}`} of {totalCount.toLocaleString()} record
            {totalCount === 1 ? '' : 's'}
          </span>
        ) : countError ? (
          <span className="text-destructive" title={countError}>
            count failed
          </span>
        ) : (
          <span>—</span>
        )}
      </div>
    </footer>
  )
}

/**
 * AddFilterButton holds the two-step Supabase flow inside one popover:
 * pick a column, then pick an operator + value, then Apply. Keeps the
 * UI compact and avoids wizard-style multi-popover chaining.
 */
function AddFilterButton({
  columns,
  onAdd,
}: {
  columns: ColumnInfo[]
  onAdd: (filter: PreviewFilter) => void
}) {
  const [open, setOpen] = useState(false)

  function apply(filter: PreviewFilter) {
    onAdd(filter)
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button size="sm" variant="ghost" className="h-7 gap-1 px-2 text-xs">
          <Plus className="size-3.5" /> Add filter
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-80 p-0" align="start">
        <FilterEditor
          columns={columns}
          initial={null}
          onApply={apply}
          onCancel={() => setOpen(false)}
        />
      </PopoverContent>
    </Popover>
  )
}

/**
 * FilterChip is the rendered representation of one applied filter.
 * Clicking it opens the editor popover preloaded with the existing
 * values; ✕ removes it without opening the editor.
 */
function FilterChip({
  filter,
  columns,
  onChange,
  onRemove,
}: {
  filter: PreviewFilter
  columns: ColumnInfo[]
  onChange: (filter: PreviewFilter) => void
  onRemove: () => void
}) {
  const [open, setOpen] = useState(false)
  const showsValue = operatorNeedsValue(filter.op)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <div className="flex items-center rounded-md border border-border bg-muted text-[11px]">
        <PopoverTrigger asChild>
          <button
            type="button"
            className="flex items-center gap-1 px-2 py-1 hover:bg-accent hover:text-accent-foreground"
          >
            <span className="font-mono font-medium">{filter.column}</span>
            <span className="text-muted-foreground">{operatorLabel(filter.op)}</span>
            {showsValue && (
              <span className="font-mono text-foreground">
                {filter.value || <em className="text-muted-foreground">empty</em>}
              </span>
            )}
          </button>
        </PopoverTrigger>
        <button
          type="button"
          onClick={onRemove}
          aria-label="Remove filter"
          className="flex size-6 items-center justify-center text-muted-foreground hover:text-foreground"
        >
          <X className="size-3" />
        </button>
      </div>
      <PopoverContent className="w-80 p-0" align="start">
        <FilterEditor
          columns={columns}
          initial={filter}
          onApply={(f) => {
            onChange(f)
            setOpen(false)
          }}
          onCancel={() => setOpen(false)}
        />
      </PopoverContent>
    </Popover>
  )
}

/**
 * FilterEditor is the body shared by the Add and Edit popovers.
 * Renders Column / Operator selects + Value input. Apply is enabled
 * once column is set and (for value-bearing operators) value is
 * non-empty.
 */
function FilterEditor({
  columns,
  initial,
  onApply,
  onCancel,
}: {
  columns: ColumnInfo[]
  initial: PreviewFilter | null
  onApply: (filter: PreviewFilter) => void
  onCancel: () => void
}) {
  // Deliberately leave column unset when adding — auto-selecting the
  // first column (often `id` of type uuid) hides the picker step and
  // produces predicates the user did not intend.
  const [column, setColumn] = useState<string>(initial?.column ?? '')
  const [op, setOp] = useState<PreviewFilterOp>(initial?.op ?? '=')
  const [value, setValue] = useState<string>(initial?.value ?? '')

  const needsValue = operatorNeedsValue(op)
  const canApply = column !== '' && (!needsValue || value !== '')

  // Focus the value input on first appearance — `autoFocus` only runs
  // on initial mount, so toggling from `is null` (no value field) back
  // to `=` would otherwise leave the user with no focused control.
  const valueInputRef = useRef<HTMLInputElement | null>(null)
  useEffect(() => {
    if (needsValue) valueInputRef.current?.focus()
  }, [needsValue])

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        if (!canApply) return
        onApply({ column, op, value: needsValue ? value : '' })
      }}
    >
      <div className="border-b border-border px-3 py-2 text-xs font-medium">
        {initial ? 'Edit filter' : 'Add filter'}
      </div>
      <div className="space-y-2 p-3">
        <div>
          <label className="mb-1 block text-[11px] text-muted-foreground">Column</label>
          <Select value={column} onValueChange={setColumn}>
            <SelectTrigger className="h-8 w-full text-xs">
              <SelectValue placeholder="Pick a column…" />
            </SelectTrigger>
            <SelectContent>
              {columns.map((c) => (
                <SelectItem key={c.name} value={c.name} className="text-xs">
                  <span className="font-mono">{c.name}</span>
                  <span className="ml-2 text-[10px] text-muted-foreground">{c.type}</span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div>
          <label className="mb-1 block text-[11px] text-muted-foreground">
            Operator
          </label>
          <Select value={op} onValueChange={(v) => setOp(v as PreviewFilterOp)}>
            <SelectTrigger className="h-8 w-full text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {OPERATOR_GROUPS.map((group) => (
                <SelectGroup key={group.label}>
                  <SelectLabel>{group.label}</SelectLabel>
                  {group.ops.map((o) => (
                    <SelectItem key={o.value} value={o.value} className="text-xs">
                      <span>{o.label}</span>
                      {o.symbol && (
                        <span className="ml-2 font-mono text-muted-foreground">
                          {o.symbol}
                        </span>
                      )}
                    </SelectItem>
                  ))}
                </SelectGroup>
              ))}
            </SelectContent>
          </Select>
        </div>
        {needsValue && (
          <div>
            <label className="mb-1 block text-[11px] text-muted-foreground">Value</label>
            <Input
              ref={valueInputRef}
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder={
                op === 'contains'
                  ? 'Substring (case-insensitive)'
                  : op === 'starts with'
                  ? 'Prefix'
                  : op === 'ends with'
                  ? 'Suffix'
                  : 'Enter value…'
              }
              className="h-8 text-xs"
            />
          </div>
        )}
      </div>
      <div className="flex items-center justify-end gap-2 border-t border-border px-3 py-2">
        <Button type="button" size="sm" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" size="sm" disabled={!canApply}>
          Apply
        </Button>
      </div>
    </form>
  )
}

interface SortPopoverProps {
  columns: ColumnInfo[]
  sort: PreviewSort
  onChange: (sort: PreviewSort) => void
}

function SortPopover({ columns, sort, onChange }: SortPopoverProps) {
  const [open, setOpen] = useState(false)
  const [column, setColumn] = useState<string>(sort?.column ?? '')
  const [dir, setDir] = useState<'asc' | 'desc'>(sort?.dir ?? 'asc')

  function apply() {
    if (!column) return
    onChange({ column, dir })
    setOpen(false)
  }

  function handleOpenChange(next: boolean) {
    // Same rationale as FilterEditor: do not auto-pick a column on
    // open — make the user choose deliberately.
    if (next) {
      setColumn(sort?.column ?? '')
      setDir(sort?.dir ?? 'asc')
    }
    setOpen(next)
  }

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button size="sm" variant={sort ? 'default' : 'outline'} className="gap-1.5">
          <ArrowUpDown />
          Sort
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-80 p-0">
        <div className="border-b border-border px-3 py-2 text-xs font-medium">
          Sort rows
        </div>
        <div className="space-y-2 p-3">
          <div>
            <label className="mb-1 block text-[11px] text-muted-foreground">Column</label>
            <Select value={column} onValueChange={setColumn}>
              <SelectTrigger className="h-8 w-full text-xs">
                <SelectValue placeholder="Pick a column…" />
              </SelectTrigger>
              <SelectContent>
                {columns.map((c) => (
                  <SelectItem key={c.name} value={c.name} className="text-xs">
                    <span className="font-mono">{c.name}</span>
                    <span className="ml-2 text-[10px] text-muted-foreground">{c.type}</span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div>
            <label className="mb-1 block text-[11px] text-muted-foreground">
              Direction
            </label>
            <div className="flex gap-1">
              <Button
                type="button"
                size="sm"
                variant={dir === 'asc' ? 'default' : 'outline'}
                onClick={() => setDir('asc')}
                className="flex-1 gap-1"
              >
                <ArrowUp className="size-3.5" /> Ascending
              </Button>
              <Button
                type="button"
                size="sm"
                variant={dir === 'desc' ? 'default' : 'outline'}
                onClick={() => setDir('desc')}
                className="flex-1 gap-1"
              >
                <ArrowDown className="size-3.5" /> Descending
              </Button>
            </div>
          </div>
        </div>
        <div className="flex items-center justify-between border-t border-border px-3 py-2">
          <Button
            size="sm"
            variant="ghost"
            onClick={() => {
              onChange(null)
              setOpen(false)
            }}
            disabled={!sort}
          >
            Clear
          </Button>
          <Button size="sm" onClick={apply} disabled={!column}>
            Apply
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
