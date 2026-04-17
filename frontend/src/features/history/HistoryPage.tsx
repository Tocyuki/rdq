/**
 * HistoryPage will list the per-profile SQL history with search, favourite,
 * and "load into editor" actions once F4 wires it to /api/history.
 */
export function HistoryPage() {
  return (
    <section className="flex h-full flex-col gap-4 p-6">
      <h1 className="text-xl font-semibold tracking-tight">History</h1>
      <p className="text-sm text-muted-foreground">
        Recent SQL executions appear here (arriving in Phase F4).
      </p>
    </section>
  )
}
