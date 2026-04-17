import { useState } from 'react'
import { Plug } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { ConnectionDialog } from '@/features/connection/ConnectionDialog'
import { sessionIsComplete, useSession } from '@/hooks/useSession'

/**
 * shortARN returns the tail segment of an AWS ARN so the connection bar
 * stays readable. Falls back to the whole string if the expected "/" or
 * ":" layout is missing.
 */
function shortARN(arn: string) {
  if (!arn) return ''
  const slash = arn.lastIndexOf('/')
  if (slash >= 0) return arn.slice(slash + 1)
  const colon = arn.lastIndexOf(':')
  return colon >= 0 ? arn.slice(colon + 1) : arn
}

export function ConnectionBar() {
  const session = useSession()
  const [dialogOpen, setDialogOpen] = useState(false)
  const ready = sessionIsComplete(session.data)

  return (
    <header className="flex h-12 shrink-0 items-center gap-3 border-b border-border bg-card px-4 text-sm">
      <span className="font-semibold tracking-tight">rdq</span>
      <Separator orientation="vertical" className="h-4" />

      {ready && session.data ? (
        <>
          <Badge variant="secondary" className="font-mono">
            {session.data.profile}
          </Badge>
          <span className="text-muted-foreground">/</span>
          <Badge variant="outline" className="font-mono">
            {shortARN(session.data.cluster)}
          </Badge>
          <span className="text-muted-foreground">/</span>
          <Badge variant="outline" className="font-mono">
            {session.data.database}
          </Badge>
        </>
      ) : (
        <span className="text-muted-foreground">
          Not connected —{' '}
          <span className="text-foreground">choose a profile to begin</span>
        </span>
      )}

      <div className="flex-1" />
      <Button size="sm" variant="outline" onClick={() => setDialogOpen(true)}>
        <Plug />
        {ready ? 'Change' : 'Connect'}
      </Button>
      <ConnectionDialog open={dialogOpen} onOpenChange={setDialogOpen} />
    </header>
  )
}
