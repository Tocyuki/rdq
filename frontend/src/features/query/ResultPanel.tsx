import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { ExecuteResponseBody } from '@/lib/api/types'

import { CopyMenu, ExportMenu } from './ExportMenu'
import { ResultTable } from './ResultTable'

interface Props {
  result: ExecuteResponseBody | null
  error: string | null
  loading: boolean
}

/**
 * ResultPanel hosts the Table / JSON / Info tabs beneath the editor. The
 * three tabs share one Tabs primitive so tab state is co-located and the
 * browser preserves the user's last selection across re-runs.
 */
export function ResultPanel({ result, error, loading }: Props) {
  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-border bg-card px-3 py-2">
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
            <ResultTable columns={result.columns} rows={result.rows} />
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
}

function EmptyState() {
  return (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
      Run a query (Cmd / Ctrl + Enter) to see results.
    </div>
  )
}
