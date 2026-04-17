import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'

/**
 * ConnectionBar is the thin top strip that always shows the active profile,
 * cluster, and database so the user can tell at a glance what they are
 * about to run a SQL against. Phase 2 will populate it from a real session
 * hook; for now it renders a neutral placeholder so Phase 1 can focus on
 * the shell layout.
 */
export function ConnectionBar() {
  return (
    <header className="flex h-12 shrink-0 items-center gap-3 border-b border-border bg-card px-4 text-sm">
      <span className="font-semibold tracking-tight">rdq</span>
      <Separator orientation="vertical" className="h-4" />
      <span className="text-muted-foreground">
        Not connected —{' '}
        <span className="text-foreground">choose a profile to begin</span>
      </span>
      <div className="flex-1" />
      <Badge variant="outline" className="font-mono">
        localhost
      </Badge>
    </header>
  )
}
