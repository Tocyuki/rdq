import { AlertTriangle, Lock, Plug, Unlock } from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  ClusterBadge,
  DatabaseBadge,
  ProfileBadge,
} from '@/features/connection/badge-popovers'
import { useSaveSession, useSession } from '@/hooks/useSession'
import type { Session } from '@/lib/api/types'
import { cn } from '@/lib/utils'
import { useUIStore } from '@/stores/uiStore'

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
  // The <ConnectionDialog /> itself is mounted once inside SessionGate.
  // ConnectionBar just drives its open state via Zustand so we never
  // render two overlays at once during a profile switch.
  const setDialogOpen = useUIStore((s) => s.setConnectionDialogOpen)

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
   * `isProduction`, `isReadOnly`, and `autoRunReadOnly` are deliberately
   * dropped from the payload. Server-side these fields are conservative:
   * a nil (undefined) value means "SPA did not touch this" and the
   * existing state.json value for the destination profile is preserved.
   * Sending the old profile's flags would leak them into the new
   * profile's stored entry.
   */
  function handleProfileChange(nextProfile: string) {
    if (nextProfile === data.profile) return
    const {
      isProduction: _drop1,
      isReadOnly: _drop2,
      autoRunReadOnly: _drop3,
      ...rest
    } = data
    void _drop1
    void _drop2
    void _drop3
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
  // Read-only defaults to ON when the backend has never seen an answer
  // (undefined). Mirror that default here so the badge is shown until
  // the user explicitly flips the flag off in Settings.
  const isReadOnly = data.isReadOnly !== false

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
      {isReadOnly ? (
        <Badge
          variant={isProduction ? 'secondary' : 'outline'}
          className="gap-1 uppercase tracking-wider"
          title="Read-only mode — only SELECT-like statements will execute. Change in Settings."
        >
          <Lock className="size-3" />
          Read-only
        </Badge>
      ) : (
        <Badge
          variant={isProduction ? 'destructive' : 'outline'}
          className="gap-1 uppercase tracking-wider"
          title="Writes are allowed — INSERT/UPDATE/DELETE will execute. Toggle in Settings."
        >
          <Unlock className="size-3" />
          Allow writes
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
        variant={isProduction ? 'secondary' : 'outline'}
        onChange={handleClusterChange}
      />
      <span className={cn(isProduction ? 'text-production-foreground/70' : 'text-muted-foreground')}>
        /
      </span>
      <DatabaseBadge
        profile={data.profile}
        current={data.database}
        variant={isProduction ? 'secondary' : 'outline'}
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
    </header>
  )
}
