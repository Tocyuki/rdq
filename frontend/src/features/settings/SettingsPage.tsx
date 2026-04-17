/**
 * SettingsPage will host AI model / language selection and the
 * production-environment toggle in Phase F5. For now it is a placeholder
 * so the routing shell is complete.
 */
export function SettingsPage() {
  return (
    <section className="flex h-full flex-col gap-4 p-6">
      <h1 className="text-xl font-semibold tracking-tight">Settings</h1>
      <p className="text-sm text-muted-foreground">
        AI model / language / production flag (coming in Phase F5).
      </p>
    </section>
  )
}
