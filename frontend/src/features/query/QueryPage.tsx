import { useCallback, useEffect, useRef, useState } from 'react'
import { Bot, Play, ShieldCheck, Sparkles, Wand2 } from 'lucide-react'
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { sessionIsComplete, useSession } from '@/hooks/useSession'
import { AnalyzeDialog } from '@/features/ai/AnalyzeDialog'
import { AskDialog } from '@/features/ai/AskDialog'
import { ExplainDialog } from '@/features/ai/ExplainDialog'
import { ReviewDialog } from '@/features/ai/ReviewDialog'
import { useSchema } from '@/features/schema/useSchema'
import { SchemaSidebar } from '@/features/schema/SchemaSidebar'
import { resolveRunSql, useUIStore } from '@/stores/uiStore'

import { ConfirmRunDialog } from './ConfirmRunDialog'
import { SqlEditor } from './editor/SqlEditor'
import { ResultPanel, type ResultPanelHandle } from './ResultPanel'
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
 * QueryPage hosts the editor + result stack. Destructive statements
 * trigger <ConfirmRunDialog> via the `needsConfirmation` response flag;
 * see ExecuteResponse in internal/server/types.go for the contract.
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
  const resultPanelRef = useRef<ResultPanelHandle>(null)

  // Cmd/Ctrl+F: route to the result panel, but leave CodeMirror's own
  // search panel alone when the editor has focus.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const isFind = (e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'f'
      if (!isFind) return
      const target = e.target as HTMLElement | null
      if (target?.closest('.cm-editor')) return
      const tag = target?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA') return
      e.preventDefault()
      resultPanelRef.current?.openSearch()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

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
      onSuccess: (res) => {
        if (res.needsConfirmation) {
          setPendingConfirm({ args, reason: res.confirmReason ?? '' })
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
  const visibleResult = execute.data?.needsConfirmation ? null : execute.data ?? null

  return (
    <div className="h-full">
      <PanelGroup direction="horizontal" autoSaveId="rdq:query:horizontal" className="h-full">
        <Panel defaultSize={20} minSize={12} maxSize={45}>
          <SchemaSidebar />
        </Panel>
        <PanelResizeHandle className="group w-1 bg-border transition-colors hover:bg-ring data-[resize-handle-state=drag]:bg-ring" />
        <Panel defaultSize={80} minSize={35}>
          <section className="flex h-full min-w-0 flex-col">
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

            <PanelGroup
              direction="vertical"
              autoSaveId="rdq:query:vertical"
              className="min-h-0 flex-1"
            >
              <Panel defaultSize={40} minSize={15}>
                <div className="h-full overflow-hidden p-3">
                  <SqlEditor
                    engineHint=""
                    schema={schema.data ?? undefined}
                    onRun={runQuery}
                  />
                </div>
              </Panel>
              <PanelResizeHandle className="group h-1 bg-border transition-colors hover:bg-ring data-[resize-handle-state=drag]:bg-ring" />
              <Panel defaultSize={60} minSize={15}>
                <ResultPanel
                  ref={resultPanelRef}
                  result={visibleResult}
                  error={errorMessage}
                  loading={execute.isPending}
                />
              </Panel>
            </PanelGroup>
          </section>
        </Panel>
      </PanelGroup>
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
