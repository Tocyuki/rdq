import { toast } from 'sonner'
import { useState } from 'react'
import { AlertTriangle, Lock, Unlock } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { AiContextPanel } from '@/features/ai/AiContextPanel'
import { ModelPicker } from '@/features/ai/ModelPicker'
import { useSaveSession, useSession } from '@/hooks/useSession'
import type { Session } from '@/lib/api/types'

/**
 * SettingsPage exposes per-profile preferences: Bedrock model + language,
 * the production warning flag, and the read-only guard. All pass through
 * PUT /api/session which persists them to state.json so choices survive
 * restarts.
 *
 * Rendering is gated on `session.isSuccess`: the inner <SettingsForm />
 * is mounted only after /api/session has resolved, so its `useState`
 * initializers see the current values instead of the binary defaults
 * when the component mounts before the query completes. Without the gate
 * a browser refresh could briefly display the defaults even when
 * state.json holds explicit choices.
 */
export function SettingsPage() {
  const session = useSession()

  return (
    <section className="flex h-full flex-col overflow-y-auto p-6 pb-12">
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
        <SettingsForm key={session.data.profile} initial={session.data} />
      )}
    </section>
  )
}

// Read-only defaults to on so a fresh install cannot drop a table by
// accident. Production defaults to off (warning colour opt-in).
// Mirrors the guard in ConnectionBar.
function defaultReadOnly(v: boolean | undefined): boolean {
  return v !== false
}
function defaultProduction(v: boolean | undefined): boolean {
  return v === true
}

function SettingsForm({ initial }: { initial: Session }) {
  const [model, setModel] = useState(() => initial.bedrockModel ?? '')
  const [language, setLanguage] = useState(() => initial.bedrockLanguage ?? '')
  const [production, setProduction] = useState<boolean>(() =>
    defaultProduction(initial.isProduction),
  )
  const [readOnly, setReadOnly] = useState<boolean>(() =>
    defaultReadOnly(initial.isReadOnly),
  )
  const [autoRun, setAutoRun] = useState<boolean>(() => initial.autoRunReadOnly === true)

  // useSaveSession seeds the session cache from the server's rehydrated
  // response via setQueryData, so we do not need a follow-up refetch
  // here. That used to live in this component as a separate
  // session.refetch() call inside onSuccess, which made the mutation
  // appear to stay pending across the GET round-trip — the redundant
  // refetch is what was causing the "Saving…" label to linger.
  const save = useSaveSession()

  const onSave = () => {
    save.mutate(
      {
        ...initial,
        bedrockModel: model,
        bedrockLanguage: language,
        isProduction: production,
        isReadOnly: readOnly,
        autoRunReadOnly: autoRun,
      },
      {
        onSuccess: () => toast.success('Settings saved'),
      },
    )
  }

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

      <div className="space-y-2">
        <div className="flex items-center gap-3">
          <Switch id="auto-run" checked={autoRun} onCheckedChange={setAutoRun} />
          <Label htmlFor="auto-run" className="cursor-pointer text-sm font-medium">
            Auto-run AI SQL (read-only)
          </Label>
        </div>
        <p className="text-xs text-muted-foreground">
          When on, SQL the model returns from Ask is executed immediately if the server
          classifies it as a pure read (SELECT / WITH / SHOW / EXPLAIN / DESCRIBE / DESC /
          TABLE / VALUES). Anything destructive falls through to the normal{' '}
          <em>insert into editor → Run</em> flow. Manual editor SQL is not affected.
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
          <BoolOption
            active={readOnly === true}
            onPick={() => setReadOnly(true)}
            label="Read-only (safe)"
            icon={<Lock className="size-3" />}
          />
          <BoolOption
            active={readOnly === false}
            onPick={() => setReadOnly(false)}
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
          <BoolOption
            active={production === false}
            onPick={() => setProduction(false)}
            label="Not production"
          />
          <BoolOption
            active={production === true}
            onPick={() => setProduction(true)}
            label="Production"
            icon={<AlertTriangle className="size-3" />}
            destructive
          />
        </div>
      </fieldset>

      <div>
        <Button onClick={onSave} disabled={save.isPending}>
          {save.isPending ? 'Saving…' : 'Save'}
        </Button>
      </div>

      <section className="mt-4 space-y-2 border-t pt-6">
        <div>
          <h2 className="text-sm font-semibold">AI context for this database</h2>
          <p className="text-xs text-muted-foreground">
            Free-form text injected into every Bedrock prompt for{' '}
            <code className="font-mono">{initial.database || '(no database)'}</code> on{' '}
            <code className="font-mono">{initial.cluster ? truncateArn(initial.cluster) : '(no cluster)'}</code>.
            Use it for glossary terms, business rules, and example queries — anything the schema alone cannot tell the model.
          </p>
        </div>
        <AiContextPanel cluster={initial.cluster} database={initial.database} />
      </section>
    </div>
  )
}

// truncateArn shortens an ARN to "<service>:…:<resource>" so the
// settings header stays readable.
function truncateArn(arn: string): string {
  const parts = arn.split(':')
  if (parts.length < 6) return arn
  return `${parts[2]}:…:${parts[parts.length - 1]}`
}

function BoolOption({
  active,
  onPick,
  label,
  icon,
  destructive,
}: {
  active: boolean
  onPick: () => void
  label: string
  icon?: React.ReactNode
  destructive?: boolean
}) {
  return (
    <Button
      type="button"
      size="sm"
      variant={active ? (destructive ? 'destructive' : 'default') : 'outline'}
      onClick={onPick}
    >
      {icon}
      {label}
    </Button>
  )
}
