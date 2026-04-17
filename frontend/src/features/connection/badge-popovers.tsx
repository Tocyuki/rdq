import { forwardRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'

import { Badge, type BadgeProps } from '@/components/ui/badge'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { endpoints } from '@/lib/api/endpoints'
import { cn } from '@/lib/utils'

/**
 * Connection badges wrap a Radix Popover each so the user can change a
 * single part of the connection without walking the full 4-step wizard.
 * The badge itself is the popover trigger, with a subtle caret and
 * hover ring telling the user it is interactive.
 */

interface BadgeTriggerProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  label: string
  variant?: BadgeProps['variant']
}

/**
 * BadgeTrigger is the element Radix PopoverTrigger slots its props into
 * via `asChild`. It has to forward the ref **and** spread the remaining
 * props (onClick, onKeyDown, aria-expanded, data-state, …) or clicks
 * silently fall through and the popover never opens.
 */
const BadgeTrigger = forwardRef<HTMLButtonElement, BadgeTriggerProps>(
  ({ label, variant = 'secondary', className, ...rest }, ref) => (
    <button
      ref={ref}
      type="button"
      className={cn('cursor-pointer outline-none', className)}
      {...rest}
    >
      <Badge variant={variant} className="font-mono hover:ring-2 hover:ring-ring">
        {label} ▾
      </Badge>
    </button>
  ),
)
BadgeTrigger.displayName = 'BadgeTrigger'

interface ProfileBadgeProps {
  current: string
  variant?: BadgeProps['variant']
  onChange: (profile: string) => void
}

export function ProfileBadge({ current, variant = 'secondary', onChange }: ProfileBadgeProps) {
  const [open, setOpen] = useState(false)
  const q = useQuery({
    queryKey: ['profiles'],
    queryFn: ({ signal }) => endpoints.listProfiles(signal),
    enabled: open,
  })
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <BadgeTrigger label={current || 'pick profile'} variant={variant} title="Switch profile" />
      </PopoverTrigger>
      <PopoverContent>
        <Command>
          <CommandInput placeholder="Search profiles…" />
          <CommandList>
            {q.isLoading && <div className="p-3 text-xs text-muted-foreground">Loading…</div>}
            <CommandEmpty>No profiles configured.</CommandEmpty>
            <CommandGroup>
              {q.data?.profiles.map((p) => (
                <CommandItem
                  key={p}
                  value={p}
                  onSelect={() => {
                    setOpen(false)
                    onChange(p)
                  }}
                  data-selected={p === current || undefined}
                >
                  {p}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

interface ClusterBadgeProps {
  profile: string
  current: string
  currentLabel: string
  variant?: BadgeProps['variant']
  /**
   * onChange fires after both a cluster pick and its follow-up secret
   * pick so the parent can commit them in one PUT /api/session call.
   * When the user opens the badge and selects a cluster, the internal
   * state transitions to 'secret' and the popover swaps to the secret
   * picker without closing — the secret ARN is then delivered along
   * with the chosen cluster ARN / engine as a single callback.
   */
  onChange: (args: { cluster: string; engine: string; secret: string }) => void
}

type Stage = 'cluster' | 'secret'

/**
 * ClusterBadge is a two-step popover that owns both the cluster choice
 * and the immediate secret follow-up (plan item 2, B案). Using the same
 * popover surface for both steps means the cascading flow happens in a
 * single visual spot anchored to the cluster badge.
 */
export function ClusterBadge({ profile, current, currentLabel, variant = 'outline', onChange }: ClusterBadgeProps) {
  const [open, setOpen] = useState(false)
  const [stage, setStage] = useState<Stage>('cluster')
  const [pendingCluster, setPendingCluster] = useState<{ arn: string; engine: string } | null>(null)

  const clusters = useQuery({
    queryKey: ['clusters', profile],
    queryFn: ({ signal }) => endpoints.listClusters(profile, signal),
    enabled: open && !!profile,
  })
  const secrets = useQuery({
    queryKey: ['secrets', profile, pendingCluster?.arn ?? ''],
    queryFn: ({ signal }) => endpoints.listSecrets(profile, pendingCluster?.arn, signal),
    enabled: open && stage === 'secret' && !!pendingCluster && !!profile,
  })

  function reset() {
    setStage('cluster')
    setPendingCluster(null)
  }

  return (
    <Popover
      open={open}
      onOpenChange={(o) => {
        setOpen(o)
        if (!o) reset()
      }}
    >
      <PopoverTrigger asChild>
        <button
          type="button"
          className="cursor-pointer outline-none"
          title="Switch cluster"
        >
          <Badge variant={variant} className="font-mono hover:ring-2 hover:ring-ring">
            {currentLabel || 'pick cluster'} ▾
          </Badge>
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-[26rem]">
        {stage === 'cluster' ? (
          <Command>
            <CommandInput placeholder="Search clusters…" />
            <CommandList>
              {clusters.isLoading && (
                <div className="p-3 text-xs text-muted-foreground">Loading…</div>
              )}
              <CommandEmpty>No Data-API-enabled clusters found.</CommandEmpty>
              <CommandGroup>
                {clusters.data?.clusters.map((c) => (
                  <CommandItem
                    key={c.arn}
                    value={`${c.identifier} ${c.engine} ${c.endpoint}`}
                    onSelect={() => {
                      if (c.arn === current) {
                        setOpen(false)
                        return
                      }
                      setPendingCluster({ arn: c.arn, engine: c.engine })
                      setStage('secret')
                    }}
                    data-selected={c.arn === current || undefined}
                  >
                    <div className="flex flex-col">
                      <span>
                        {c.identifier}{' '}
                        <span className="text-xs text-muted-foreground">[{c.engine}]</span>
                      </span>
                      <span className="text-xs text-muted-foreground">{c.endpoint}</span>
                    </div>
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        ) : (
          <Command>
            <CommandInput placeholder="Pick a secret for the new cluster…" />
            <CommandList>
              {secrets.isLoading && (
                <div className="p-3 text-xs text-muted-foreground">Loading…</div>
              )}
              <CommandEmpty>No secrets visible.</CommandEmpty>
              <CommandGroup
                heading={secrets.data?.suggested ? 'Suggested for this cluster' : 'All secrets'}
              >
                {secrets.data?.secrets.map((s) => (
                  <CommandItem
                    key={s.arn}
                    value={`${s.name} ${s.description ?? ''}`}
                    onSelect={() => {
                      if (!pendingCluster) return
                      setOpen(false)
                      onChange({
                        cluster: pendingCluster.arn,
                        engine: pendingCluster.engine,
                        secret: s.arn,
                      })
                      reset()
                    }}
                  >
                    <div className="flex flex-col">
                      <span>{s.name}</span>
                      {s.description && (
                        <span className="text-xs text-muted-foreground">{s.description}</span>
                      )}
                    </div>
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
            <div className="flex justify-end border-t border-border px-2 py-1">
              <button
                className="text-xs text-muted-foreground underline-offset-2 hover:underline"
                onClick={() => setStage('cluster')}
              >
                ← back to clusters
              </button>
            </div>
          </Command>
        )}
      </PopoverContent>
    </Popover>
  )
}

interface DatabaseBadgeProps {
  profile: string
  current: string
  variant?: BadgeProps['variant']
  onChange: (database: string) => void
}

export function DatabaseBadge({ profile, current, variant = 'outline', onChange }: DatabaseBadgeProps) {
  const [open, setOpen] = useState(false)
  const [typed, setTyped] = useState(current)
  const q = useQuery({
    queryKey: ['databases', profile],
    queryFn: ({ signal }) => endpoints.listDatabases(profile, signal),
    enabled: open && !!profile,
  })

  function commit(value: string) {
    if (!value) return
    setOpen(false)
    onChange(value)
  }

  return (
    <Popover
      open={open}
      onOpenChange={(o) => {
        setOpen(o)
        if (o) setTyped(current)
      }}
    >
      <PopoverTrigger asChild>
        <BadgeTrigger label={current || 'pick db'} variant={variant} title="Switch database" />
      </PopoverTrigger>
      <PopoverContent>
        <div className="space-y-2 p-3">
          <Input
            autoFocus
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                commit(typed)
              }
            }}
            placeholder="Database name (press Enter)"
          />
          {q.data?.history && q.data.history.length > 0 && (
            <Command className="rounded-md border border-border">
              <CommandList>
                <CommandGroup heading="Recent">
                  {q.data.history.map((h) => (
                    <CommandItem
                      key={h}
                      value={h}
                      onSelect={() => commit(h)}
                      data-selected={h === current || undefined}
                    >
                      {h}
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
