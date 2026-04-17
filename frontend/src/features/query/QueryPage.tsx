/**
 * QueryPage is the primary destination of the app: SQL editor on top,
 * results below. Phase 1 ships a placeholder so the routing shell compiles
 * and the user can at least see something load; subsequent phases plug in
 * CodeMirror (F3), the result table, CSV export, and the AI dialogs (F5).
 */
export function QueryPage() {
  return (
    <section className="flex h-full flex-col gap-4 p-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Query</h1>
        <p className="text-sm text-muted-foreground">
          SQL editor &amp; result viewer (coming next phase).
        </p>
      </div>
      <div className="flex flex-1 items-center justify-center rounded-lg border border-dashed border-border text-sm text-muted-foreground">
        The editor lands in Phase F3.
      </div>
    </section>
  )
}
