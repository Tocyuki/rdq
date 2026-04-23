/**
 * Shown when the SPA boots without the per-run GUI launch token. Users land
 * here if they open the loopback URL directly instead of going through the
 * browser window `rdq gui` opens for them.
 */
export function MissingSessionTokenScreen() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6">
      <div className="max-w-md rounded-lg border border-border bg-card p-6 text-sm shadow-sm">
        <h1 className="text-base font-semibold tracking-tight">GUI session not authorized</h1>
        <p className="mt-2 text-muted-foreground">
          This page was opened without the per-run launch token. Start the GUI with{' '}
          <code className="font-mono">rdq gui</code> and use the browser window it opens,
          or the full launch URL printed by <code className="font-mono">rdq gui --no-open</code>.
        </p>
      </div>
    </div>
  )
}
