import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSaveSession, useSession } from '@/hooks/useSession'
import { endpoints } from '@/lib/api/endpoints'
import type { Session } from '@/lib/api/types'
import { cn } from '@/lib/utils'

type Step = 'profile' | 'cluster' | 'secret' | 'database'

interface DraftSession {
  profile: string
  cluster: string
  secret: string
  database: string
  engine: string
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/**
 * ConnectionDialog is the outer shell; the actual wizard body lives in
 * <WizardBody> which is only mounted while `open` is true. This gives us a
 * clean state reset between openings without any effect/ref plumbing:
 * the form state simply does not exist when the dialog is closed.
 */
export function ConnectionDialog({ open, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Connect to database</DialogTitle>
          <DialogDescription>
            Pick a profile, cluster, secret, and database to run queries against.
          </DialogDescription>
        </DialogHeader>
        {open && <WizardBody onClose={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  )
}

function WizardBody({ onClose }: { onClose: () => void }) {
  const session = useSession()
  const save = useSaveSession()

  const initialStep: Step = !session.data?.profile
    ? 'profile'
    : !session.data.cluster
      ? 'cluster'
      : !session.data.secret
        ? 'secret'
        : 'database'
  const [step, setStep] = useState<Step>(initialStep)
  const [draft, setDraft] = useState<DraftSession>({
    profile: session.data?.profile ?? '',
    cluster: session.data?.cluster ?? '',
    secret: session.data?.secret ?? '',
    database: session.data?.database ?? '',
    engine: '',
  })

  async function submit() {
    const next: Session = {
      profile: draft.profile,
      cluster: draft.cluster,
      secret: draft.secret,
      database: draft.database,
      bedrockModel: session.data?.bedrockModel ?? '',
      bedrockLanguage: session.data?.bedrockLanguage ?? '',
    }
    try {
      await save.mutateAsync(next)
      toast.success('Connection saved')
      onClose()
    } catch {
      // onError in the hook already surfaced the toast.
    }
  }

  return (
    <>
        <StepIndicator step={step} />

        {step === 'profile' && (
          <ProfileStep
            value={draft.profile}
            onPick={(profile) => {
              setDraft({ ...draft, profile, cluster: '', secret: '', database: '', engine: '' })
              setStep('cluster')
            }}
          />
        )}
        {step === 'cluster' && draft.profile && (
          <ClusterStep
            profile={draft.profile}
            value={draft.cluster}
            onPick={(cluster, engine) => {
              setDraft({ ...draft, cluster, engine, secret: '', database: '' })
              setStep('secret')
            }}
          />
        )}
        {step === 'secret' && draft.profile && (
          <SecretStep
            profile={draft.profile}
            cluster={draft.cluster}
            value={draft.secret}
            onPick={(secret) => {
              setDraft({ ...draft, secret })
              setStep('database')
            }}
          />
        )}
        {step === 'database' && draft.profile && (
          <DatabaseStep
            profile={draft.profile}
            value={draft.database}
            onPick={(database) => setDraft({ ...draft, database })}
          />
        )}

        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="outline"
            disabled={step === 'profile'}
            onClick={() => setStep(prevStep(step))}
          >
            Back
          </Button>
          <Button
            onClick={submit}
            disabled={
              save.isPending ||
              !draft.profile ||
              !draft.cluster ||
              !draft.secret ||
              !draft.database
            }
          >
            {save.isPending ? 'Saving…' : 'Save'}
          </Button>
        </DialogFooter>
    </>
  )
}

function prevStep(s: Step): Step {
  return s === 'database' ? 'secret' : s === 'secret' ? 'cluster' : 'profile'
}

function StepIndicator({ step }: { step: Step }) {
  const labels: Array<{ key: Step; label: string }> = [
    { key: 'profile', label: '1 Profile' },
    { key: 'cluster', label: '2 Cluster' },
    { key: 'secret', label: '3 Secret' },
    { key: 'database', label: '4 Database' },
  ]
  return (
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      {labels.map(({ key, label }, idx) => (
        <span
          key={key}
          className={
            key === step
              ? 'rounded bg-secondary px-2 py-1 text-secondary-foreground'
              : idx < labels.findIndex((l) => l.key === step)
                ? 'text-foreground'
                : ''
          }
        >
          {label}
        </span>
      ))}
    </div>
  )
}

function ProfileStep({ value, onPick }: { value: string; onPick: (p: string) => void }) {
  const q = useQuery({
    queryKey: ['profiles'],
    queryFn: ({ signal }) => endpoints.listProfiles(signal),
  })
  return (
    <PickerShell label="Profile" loading={q.isLoading} error={q.error}>
      <Command>
        <CommandInput placeholder="Search profiles…" />
        <CommandList>
          <CommandEmpty>No profiles configured in ~/.aws.</CommandEmpty>
          <CommandGroup>
            {q.data?.profiles.map((p) => (
              <CommandItem
                key={p}
                value={p}
                onSelect={() => onPick(p)}
                data-selected={value === p || undefined}
              >
                {p}
              </CommandItem>
            ))}
          </CommandGroup>
        </CommandList>
      </Command>
    </PickerShell>
  )
}

function ClusterStep({
  profile,
  value,
  onPick,
}: {
  profile: string
  value: string
  onPick: (arn: string, engine: string) => void
}) {
  const q = useQuery({
    queryKey: ['clusters', profile],
    queryFn: ({ signal }) => endpoints.listClusters(profile, signal),
  })
  return (
    <PickerShell label="Cluster" loading={q.isLoading} error={q.error}>
      <Command>
        <CommandInput placeholder="Search clusters…" />
        <CommandList>
          <CommandEmpty>No Data-API-enabled Aurora clusters found.</CommandEmpty>
          <CommandGroup>
            {q.data?.clusters.map((c) => (
              <CommandItem
                key={c.arn}
                value={`${c.identifier} ${c.engine} ${c.endpoint}`}
                onSelect={() => onPick(c.arn, c.engine)}
                data-selected={value === c.arn || undefined}
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
    </PickerShell>
  )
}

function SecretStep({
  profile,
  cluster,
  value,
  onPick,
}: {
  profile: string
  cluster: string
  value: string
  onPick: (arn: string) => void
}) {
  const q = useQuery({
    queryKey: ['secrets', profile, cluster],
    queryFn: ({ signal }) => endpoints.listSecrets(profile, cluster, signal),
  })
  return (
    <PickerShell label="Secret" loading={q.isLoading} error={q.error}>
      <Command>
        <CommandInput placeholder="Search secrets…" />
        <CommandList>
          <CommandEmpty>No secrets visible for this profile.</CommandEmpty>
          <CommandGroup
            heading={q.data?.suggested ? 'Suggested for this cluster' : 'All secrets in region'}
          >
            {q.data?.secrets.map((s) => (
              <CommandItem
                key={s.arn}
                value={`${s.name} ${s.description ?? ''}`}
                onSelect={() => onPick(s.arn)}
                data-selected={value === s.arn || undefined}
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
      </Command>
    </PickerShell>
  )
}

// Flat layout — a free-text input is primary here, so PickerShell's
// bordered listbox shell (used by the other three steps) would just
// nest borders around the label.
function DatabaseStep({
  profile,
  value,
  onPick,
}: {
  profile: string
  value: string
  onPick: (name: string) => void
}) {
  const q = useQuery({
    queryKey: ['databases', profile],
    queryFn: ({ signal }) => endpoints.listDatabases(profile, signal),
  })
  const [typed, setTyped] = useState(value)
  const history = q.data?.history ?? []

  return (
    <div className="space-y-5">
      <div className="space-y-1.5">
        <Label htmlFor="db">Database name</Label>
        <Input
          id="db"
          value={typed}
          onChange={(e) => {
            setTyped(e.target.value)
            onPick(e.target.value)
          }}
          placeholder="e.g. app"
          autoFocus
        />
        <p className="text-xs text-muted-foreground">
          Type a database, or pick from your recent list below.
        </p>
      </div>

      {q.isLoading && (
        <p className="text-xs text-muted-foreground">Loading recent…</p>
      )}
      {!!q.error && (
        <p className="text-xs text-destructive">
          {q.error instanceof Error ? q.error.message : 'Could not load history.'}
        </p>
      )}

      {history.length > 0 && (
        <section className="space-y-2">
          <p className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
            Recent
          </p>
          <ul className="overflow-hidden rounded-md border border-border">
            {history.map((h, idx) => {
              const selected = value === h
              return (
                <li
                  key={h}
                  className={cn(idx > 0 && 'border-t border-border')}
                >
                  <button
                    type="button"
                    onClick={() => {
                      setTyped(h)
                      onPick(h)
                    }}
                    className={cn(
                      'flex w-full items-center px-3 py-2 text-left text-sm transition-colors hover:bg-accent hover:text-accent-foreground',
                      selected && 'bg-accent font-medium text-accent-foreground',
                    )}
                  >
                    {h}
                  </button>
                </li>
              )
            })}
          </ul>
        </section>
      )}
    </div>
  )
}

function PickerShell({
  label,
  loading,
  error,
  children,
}: {
  label: string
  loading: boolean
  error: unknown
  children: React.ReactNode
}) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      <div className="rounded-md border border-border">
        {loading && <div className="p-4 text-sm text-muted-foreground">Loading…</div>}
        {!!error && (
          <div className="p-4 text-sm text-destructive">
            {error instanceof Error ? error.message : 'Request failed'}
          </div>
        )}
        {!loading && !error && children}
      </div>
    </div>
  )
}
