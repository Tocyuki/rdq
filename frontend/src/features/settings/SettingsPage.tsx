import { useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useState } from 'react'
import { AlertTriangle, Lock, Unlock } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ModelPicker } from '@/features/ai/ModelPicker'
import { useSession } from '@/hooks/useSession'
import { endpoints } from '@/lib/api/endpoints'
import type { Session } from '@/lib/api/types'

type TriState = 'unset' | 'true' | 'false'

function triStateFromSession(v: boolean | undefined): TriState {
  if (v === undefined) return 'unset'
  return v ? 'true' : 'false'
}

function triStateToBool(v: TriState): boolean | undefined {
  if (v === 'unset') return undefined
  return v === 'true'
}

/**
 * SettingsPage exposes per-profile preferences: Bedrock model + language,
 * the production warning flag, and the read-only guard. All pass through
 * PUT /api/session which persists them to state.json so choices survive
 * restarts.
 *
 * Rendering is gated on `session.isSuccess`: the inner <SettingsForm />
 * is mounted only after /api/session has resolved, so its `useState`
 * initializers see the current values instead of falling back to the
 * blank "unset" defaults when the component mounts before the query
 * completes. Without the gate a browser refresh would display empty
 * selections even when state.json holds real values.
 */
export function SettingsPage() {
  const session = useSession()

  return (
    <section className="flex h-full flex-col p-6">
      <h1 className="text-xl font-semibold tracking-tight">Settings</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        Per-profile preferences, saved to <code className="font-mono">~/.rdq/state.json</code>.
      </p>

      {session.isLoading && (
        <p className="mt-6 text-sm text-muted-foreground">Loading session…</p>
      )}
      {session.isError && (
        <p className="mt-6 text-sm text-destructive">
          Could not load session: {(session.error as Error).message}
        </p>
      )}
      {session.isSuccess && session.data && (
        <SettingsForm key={session.data.profile} initial={session.data} onSaved={session.refetch} />
      )}
    </section>
  )
}

function SettingsForm({
  initial,
  onSaved,
}: {
  initial: Session
  onSaved: () => unknown
}) {
  const [model, setModel] = useState(() => initial.bedrockModel ?? '')
  const [language, setLanguage] = useState(() => initial.bedrockLanguage ?? '')
  const [production, setProduction] = useState<TriState>(() =>
    triStateFromSession(initial.isProduction),
  )
  const [readOnly, setReadOnly] = useState<TriState>(() =>
    triStateFromSession(initial.isReadOnly),
  )

  const save = useMutation({
    mutationFn: () =>
      endpoints.putSession({
        ...initial,
        bedrockModel: model,
        bedrockLanguage: language,
        isProduction: triStateToBool(production),
        isReadOnly: triStateToBool(readOnly),
      }),
    onSuccess: () => {
      toast.success('Settings saved')
      onSaved()
    },
    onError: (err: Error) => toast.error(err.message),
  })

  return (
    <div className="mt-6 grid max-w-lg gap-6">
      <div className="space-y-1">
        <Label>Bedrock model</Label>
        <ModelPicker value={model} onChange={setModel} />
        <p className="text-xs text-muted-foreground">
          Inference profiles (cross-region failover) are preferred; foundation models are the fallback.
        </p>
      </div>

      <div className="space-y-1">
        <Label htmlFor="lang">Response language</Label>
        <Input
          id="lang"
          value={language}
          onChange={(e) => setLanguage(e.target.value)}
          placeholder="Japanese, English, ..."
        />
        <p className="text-xs text-muted-foreground">
          The natural language the model uses when responding. SQL keywords stay in English regardless.
        </p>
      </div>

      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">Read-only mode</legend>
        <p className="text-xs text-muted-foreground">
          When enabled, only statements that begin with SELECT / WITH / SHOW / EXPLAIN /
          DESCRIBE / DESC / TABLE / VALUES can be executed. Destructive operations
          (INSERT / UPDATE / DELETE / ALTER / DROP / TRUNCATE / …) are rejected
          before reaching AWS. The default is <em>on</em> so fresh installs are safe.
        </p>
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <TriStateOption
            value="unset"
            current={readOnly}
            onPick={setReadOnly}
            label="Unanswered (defaults to on)"
          />
          <TriStateOption
            value="true"
            current={readOnly}
            onPick={setReadOnly}
            label="Read-only (safe)"
            icon={<Lock className="size-3" />}
          />
          <TriStateOption
            value="false"
            current={readOnly}
            onPick={setReadOnly}
            label="Allow writes"
            icon={<Unlock className="size-3" />}
            destructive
          />
        </div>
      </fieldset>

      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">Production environment</legend>
        <p className="text-xs text-muted-foreground">
          When marked as production, the connection bar switches to a warning colour so
          destructive statements are less likely to slip in unnoticed.
        </p>
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <TriStateOption
            value="unset"
            current={production}
            onPick={setProduction}
            label="Unanswered"
          />
          <TriStateOption
            value="false"
            current={production}
            onPick={setProduction}
            label="Not production"
          />
          <TriStateOption
            value="true"
            current={production}
            onPick={setProduction}
            label="Production"
            icon={<AlertTriangle className="size-3" />}
            destructive
          />
        </div>
      </fieldset>

      <div>
        <Button onClick={() => save.mutate()} disabled={save.isPending}>
          {save.isPending ? 'Saving…' : 'Save'}
        </Button>
      </div>
    </div>
  )
}

function TriStateOption({
  value,
  current,
  onPick,
  label,
  icon,
  destructive,
}: {
  value: TriState
  current: TriState
  onPick: (v: TriState) => void
  label: string
  icon?: React.ReactNode
  destructive?: boolean
}) {
  const active = current === value
  return (
    <Button
      type="button"
      size="sm"
      variant={active ? (destructive ? 'destructive' : 'default') : 'outline'}
      onClick={() => onPick(value)}
    >
      {icon}
      {label}
    </Button>
  )
}
