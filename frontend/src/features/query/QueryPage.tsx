import { useCallback, useState } from 'react'
import { Bot, Play, ShieldCheck, Sparkles, Wand2 } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { sessionIsComplete, useSession } from '@/hooks/useSession'
import { AnalyzeDialog } from '@/features/ai/AnalyzeDialog'
import { AskDialog } from '@/features/ai/AskDialog'
import { ExplainDialog } from '@/features/ai/ExplainDialog'
import { ReviewDialog } from '@/features/ai/ReviewDialog'
import { useSchema } from '@/features/schema/useSchema'
import { SchemaSidebar } from '@/features/schema/SchemaSidebar'
import { ApiError } from '@/lib/api/client'
import { ErrorCode } from '@/lib/api/error-codes'
import { resolveRunSql, useUIStore } from '@/stores/uiStore'

import { ConfirmRunDialog } from './ConfirmRunDialog'
import { SqlEditor } from './editor/SqlEditor'
import { ResultPanel } from './ResultPanel'
import { useExecuteQuery } from './useExecuteQuery'

interface PendingConfirm {
  args: {
    profile: string
    cluster: string
    secret: string
    database: string
    sql: string
  }
  reason: string
}

/**
 * QueryPage hosts the editor + result stack. Run is invoked via either
 * the Run button or Cmd/Ctrl+Enter inside the editor — both paths call
 * the same useExecuteQuery mutation.
 *
 * Destructive-statement guard: when /api/execute returns
 * errCodeConfirmationRequired (HTTP 409) we capture the args that
 * triggered it in `pendingConfirm`, show <ConfirmRunDialog>, and on
 * "Run anyway" re-invoke the mutation with `confirmed: true`. The
 * mutation's default onError suppresses the toast for this code so
 * the dialog is the only surface for the prompt.
 */
export function QueryPage() {
  const session = useSession()
  const sql = useUIStore((s) => s.sql)
  const editorView = useUIStore((s) => s.editorView)
  const execute = useExecuteQuery()
  const schema = useSchema({
    profile: session.data?.profile ?? '',
    cluster: session.data?.cluster ?? '',
    secret: session.data?.secret ?? '',
    database: session.data?.database ?? '',
  })
  const [askOpen, setAskOpen] = useState(false)
  const [reviewOpen, setReviewOpen] = useState(false)
  const [analyzeOpen, setAnalyzeOpen] = useState(false)
  const [explainOpen, setExplainOpen] = useState(false)
  const [pendingConfirm, setPendingConfirm] = useState<PendingConfirm | null>(null)

  const runQuery = useCallback(() => {
    if (!sessionIsComplete(session.data)) {
      toast.error('Pick a profile, cluster, secret, and database first.')
      return
    }
    const effectiveSql = resolveRunSql(sql, editorView)
    if (!effectiveSql.trim()) {
      toast.error('Enter a SQL statement to run.')
      return
    }
    const data = session.data!
    const args = {
      profile: data.profile,
      cluster: data.cluster,
      secret: data.secret,
      database: data.database,
      sql: effectiveSql,
    }
    execute.mutate(args, {
      onError: (err) => {
        if (err instanceof ApiError && err.code === ErrorCode.ConfirmationRequired) {
          setPendingConfirm({ args, reason: err.message })
        }
      },
    })
  }, [session.data, sql, editorView, execute])

  const confirmAndRun = useCallback(() => {
    if (!pendingConfirm) return
    // Close the dialog synchronously; the result (success or AWS error)
    // surfaces in the Result panel below, not back on the modal.
    const args = { ...pendingConfirm.args, confirmed: true }
    setPendingConfirm(null)
    execute.mutate(args)
  }, [pendingConfirm, execute])

  const errorMessage = execute.error?.message ?? null

  return (
    <div className="flex h-full">
      <SchemaSidebar />
      <section className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center justify-between gap-3 border-b border-border bg-card px-4 py-2">
          <div>
            <h1 className="text-sm font-semibold tracking-tight">Query</h1>
            <p className="text-xs text-muted-foreground">
              Cmd / Ctrl + Enter to run · selection, if any, runs alone
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button size="sm" variant="outline" onClick={() => setAskOpen(true)}>
              <Sparkles /> Ask
            </Button>
            <Button size="sm" variant="outline" onClick={() => setReviewOpen(true)}>
              <ShieldCheck /> Review
            </Button>
            <Button size="sm" variant="outline" onClick={() => setAnalyzeOpen(true)}>
              <Wand2 /> Analyze
            </Button>
            {execute.error && (
              <Button size="sm" variant="outline" onClick={() => setExplainOpen(true)}>
                <Bot /> Explain
              </Button>
            )}
            <Button
              size="sm"
              onClick={runQuery}
              disabled={execute.isPending || !sessionIsComplete(session.data)}
            >
              <Play />
              {execute.isPending ? 'Running…' : 'Run'}
            </Button>
          </div>
        </div>

        <div className="min-h-0 flex-[2] overflow-hidden p-3">
          <SqlEditor engineHint="" schema={schema.data ?? undefined} onRun={runQuery} />
        </div>

        <div className="min-h-0 flex-[3] border-t border-border">
          <ResultPanel
            result={execute.data ?? null}
            error={errorMessage}
            loading={execute.isPending}
          />
        </div>
      </section>
      <AskDialog open={askOpen} onOpenChange={setAskOpen} />
      <ReviewDialog open={reviewOpen} onOpenChange={setReviewOpen} />
      <AnalyzeDialog open={analyzeOpen} onOpenChange={setAnalyzeOpen} />
      <ExplainDialog
        open={explainOpen}
        onOpenChange={setExplainOpen}
        initialError={errorMessage ?? undefined}
      />
      <ConfirmRunDialog
        open={pendingConfirm !== null}
        reason={pendingConfirm?.reason ?? null}
        sql={pendingConfirm?.args.sql ?? ''}
        pending={execute.isPending}
        onCancel={() => setPendingConfirm(null)}
        onConfirm={confirmAndRun}
      />
    </div>
  )
}
