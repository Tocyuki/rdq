import { useState } from 'react'
import { AlertTriangle, Plug } from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { ConnectionDialog } from '@/features/connection/ConnectionDialog'
import {
  ClusterBadge,
  DatabaseBadge,
  ProfileBadge,
} from '@/features/connection/badge-popovers'
import { useSaveSession, useSession } from '@/hooks/useSession'
import type { Session } from '@/lib/api/types'
import { cn } from '@/lib/utils'

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
  const save = useSaveSession()
  const [dialogOpen, setDialogOpen] = useState(false)

  const data: Session = session.data ?? {
    profile: '',
    cluster: '',
    secret: '',
    database: '',
    bedrockModel: '',
    bedrockLanguage: '',
  }

  /**
   * handleProfileChange (plan item 1, A案): a profile switch invalidates
   * every downstream selection, so we clear cluster/secret/database and
   * reopen the full wizard under the new profile's resources.
   *
   * isProduction is deliberately dropped so the server treats the value
   * as "SPA did not touch this" and leaves the new profile's stored
   * production flag intact. Without this, the old profile's flag would
   * bleed into the new profile's state.json entry.
   */
  function handleProfileChange(nextProfile: string) {
    if (nextProfile === data.profile) return
    const { isProduction: _drop, ...rest } = data
    void _drop
    save.mutate(
      {
        ...rest,
        profile: nextProfile,
        cluster: '',
        secret: '',
        database: '',
      },
      {
        onSuccess: () => {
          toast.success(`Switched to profile ${nextProfile}`)
          setDialogOpen(true)
        },
      },
    )
  }

  /**
   * handleClusterChange (plan item 2, B案): the ClusterBadge's internal
   * two-step flow delivers cluster + secret together. We clear the
   * database since schema caches are keyed on (cluster, database) and
   * prompting the user via the DatabaseBadge afterwards is cheaper than
   * reconstructing the cluster flow.
   */
  function handleClusterChange({
    cluster,
    secret,
  }: {
    cluster: string
    engine: string
    secret: string
  }) {
    save.mutate(
      {
        ...data,
        cluster,
        secret,
        database: '',
      },
      {
        onSuccess: () => {
          toast.success('Cluster + secret updated')
        },
      },
    )
  }

  /**
   * handleDatabaseChange is the simplest swap: state.json's history or
   * a free-text value just replaces data.database. Cluster / secret are
   * preserved because the DB is a logical namespace inside them.
   */
  function handleDatabaseChange(nextDB: string) {
    if (!nextDB || nextDB === data.database) return
    save.mutate(
      { ...data, database: nextDB },
      { onSuccess: () => toast.success(`Switched to database ${nextDB}`) },
    )
  }

  const isProduction = data.isProduction === true

  return (
    <header
      className={cn(
        'flex h-12 shrink-0 items-center gap-3 border-b px-4 text-sm transition-colors',
        isProduction
          ? 'border-production/60 bg-production text-production-foreground'
          : 'border-border bg-card',
      )}
    >
      <span className="font-semibold tracking-tight">rdq</span>
      {isProduction && (
        <Badge variant="production" className="gap-1 uppercase tracking-wider">
          <AlertTriangle className="size-3" />
          Production
        </Badge>
      )}
      <Separator
        orientation="vertical"
        className={cn('h-4', isProduction && 'bg-production-foreground/40')}
      />

      <ProfileBadge current={data.profile} onChange={handleProfileChange} />
      <span className={cn(isProduction ? 'text-production-foreground/70' : 'text-muted-foreground')}>
        /
      </span>
      <ClusterBadge
        profile={data.profile}
        current={data.cluster}
        currentLabel={shortARN(data.cluster)}
        onChange={handleClusterChange}
      />
      <span className={cn(isProduction ? 'text-production-foreground/70' : 'text-muted-foreground')}>
        /
      </span>
      <DatabaseBadge
        profile={data.profile}
        current={data.database}
        onChange={handleDatabaseChange}
      />

      <div className="flex-1" />
      <Button
        size="sm"
        variant={isProduction ? 'secondary' : 'outline'}
        onClick={() => setDialogOpen(true)}
      >
        <Plug />
        {data.profile && data.cluster && data.secret && data.database ? 'Change' : 'Connect'}
      </Button>
      <ConnectionDialog open={dialogOpen} onOpenChange={setDialogOpen} />
    </header>
  )
}
