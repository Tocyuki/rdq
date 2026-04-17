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
import { useUIStore } from '@/stores/uiStore'

import { SqlEditor } from './editor/SqlEditor'
import { ResultPanel } from './ResultPanel'
import { useExecuteQuery } from './useExecuteQuery'

/**
 * QueryPage hosts the editor + result stack. Run is invoked via either
 * the Run button or Cmd/Ctrl+Enter inside the editor — both paths call
 * the same useExecuteQuery mutation.
 *
 * F4 adds the left-side SchemaSidebar; it reads from the same
 * /api/schema cache so double-clicking a column instantly inserts it
 * into the editor via pendingEditorText.
 */
export function QueryPage() {
  const session = useSession()
  const sql = useUIStore((s) => s.sql)
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
    <div className="flex h-full">
      <SchemaSidebar />
      <section className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center justify-between gap-3 border-b border-border bg-card px-4 py-2">
          <div>
            <h1 className="text-sm font-semibold tracking-tight">Query</h1>
            <p className="text-xs text-muted-foreground">
              Cmd / Ctrl + Enter to run
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
    </div>
  )
}
