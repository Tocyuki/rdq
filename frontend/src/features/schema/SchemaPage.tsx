/**
 * SchemaPage will render information_schema as a searchable tree in F4.
 */
export function SchemaPage() {
  return (
    <section className="flex h-full flex-col gap-4 p-6">
      <h1 className="text-xl font-semibold tracking-tight">Schema</h1>
      <p className="text-sm text-muted-foreground">
        Tables &amp; columns with autocomplete wiring (coming in Phase F4).
      </p>
    </section>
  )
}
