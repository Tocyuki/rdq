import { useCallback } from 'react'
import { Play } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { sessionIsComplete, useSession } from '@/hooks/useSession'
import { useUIStore } from '@/stores/uiStore'

import { SqlEditor } from './editor/SqlEditor'
import { ResultPanel } from './ResultPanel'
import { useExecuteQuery } from './useExecuteQuery'

/**
 * QueryPage hosts the editor + result stack. Run is invoked via either
 * the Run button or Cmd/Ctrl+Enter inside the editor — both paths call
 * the same useExecuteQuery mutation.
 *
 * The editor height is fixed at 40% of the pane and the result panel
 * fills the rest; a proper Resizable split lands when UX needs it —
 * the static split is plenty for the MVP.
 */
export function QueryPage() {
  const session = useSession()
  const sql = useUIStore((s) => s.sql)
  const execute = useExecuteQuery()

  const runQuery = useCallback(() => {
    if (!sessionIsComplete(session.data)) {
      toast.error('Pick a profile, cluster, secret, and database first.')
      return
    }
    if (!sql.trim()) {
      toast.error('Enter a SQL statement to run.')
      return
    }
    const data = session.data!
    execute.mutate({
      profile: data.profile,
      cluster: data.cluster,
      secret: data.secret,
      database: data.database,
      sql,
    })
  }, [session.data, sql, execute])

  const errorMessage = execute.error?.message ?? null

  return (
    <section className="flex h-full flex-col">
      <div className="flex items-center justify-between gap-3 border-b border-border bg-card px-4 py-2">
        <div>
          <h1 className="text-sm font-semibold tracking-tight">Query</h1>
          <p className="text-xs text-muted-foreground">
            Cmd / Ctrl + Enter to run
          </p>
        </div>
        <Button
          size="sm"
          onClick={runQuery}
          disabled={execute.isPending || !sessionIsComplete(session.data)}
        >
          <Play />
          {execute.isPending ? 'Running…' : 'Run'}
        </Button>
      </div>

      <div className="min-h-0 flex-[2] overflow-hidden p-3">
        <SqlEditor engineHint="" onRun={runQuery} />
      </div>

      <div className="min-h-0 flex-[3] border-t border-border">
        <ResultPanel
          result={execute.data ?? null}
          error={errorMessage}
          loading={execute.isPending}
        />
      </div>
    </section>
  )
}
