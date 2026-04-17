import { useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useState } from 'react'
import { AlertTriangle } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ModelPicker } from '@/features/ai/ModelPicker'
import { useSession } from '@/hooks/useSession'
import { endpoints } from '@/lib/api/endpoints'

type ProductionTriState = 'unset' | 'true' | 'false'

function triStateFromSession(v: boolean | undefined): ProductionTriState {
  if (v === undefined) return 'unset'
  return v ? 'true' : 'false'
}

function triStateToBool(v: ProductionTriState): boolean | undefined {
  if (v === 'unset') return undefined
  return v === 'true'
}

/**
 * SettingsPage exposes the three pieces of per-profile configuration
 * that are too important to hide but too specific to belong on the
 * ConnectionBar: the Bedrock model, the response language, and the
 * production-environment flag that paints the ConnectionBar with a
 * warning colour. All three pass through PUT /api/session which
 * persists them to state.json so choices survive restarts.
 */
export function SettingsPage() {
  const session = useSession()
  const [model, setModel] = useState(() => session.data?.bedrockModel ?? '')
  const [language, setLanguage] = useState(() => session.data?.bedrockLanguage ?? '')
  const [production, setProduction] = useState<ProductionTriState>(() =>
    triStateFromSession(session.data?.isProduction),
  )

  const save = useMutation({
    mutationFn: () => {
      if (!session.data) throw new Error('Session not loaded')
      return endpoints.putSession({
        ...session.data,
        bedrockModel: model,
        bedrockLanguage: language,
        isProduction: triStateToBool(production),
      })
    },
    onSuccess: () => {
      toast.success('Settings saved')
      session.refetch()
    },
    onError: (err: Error) => toast.error(err.message),
  })

  return (
    <section className="flex h-full flex-col p-6">
      <h1 className="text-xl font-semibold tracking-tight">Settings</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        Per-profile preferences, saved to <code className="font-mono">~/.rdq/state.json</code>.
      </p>

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
          <legend className="text-sm font-medium">Production environment</legend>
          <p className="text-xs text-muted-foreground">
            When marked as production, the connection bar switches to a warning colour so
            destructive statements are less likely to slip in unnoticed.
          </p>
          <div className="flex flex-wrap items-center gap-2 pt-1">
            <ProductionOption
              value="unset"
              current={production}
              onPick={setProduction}
              label="Unanswered"
            />
            <ProductionOption
              value="false"
              current={production}
              onPick={setProduction}
              label="Not production"
            />
            <ProductionOption
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
    </section>
  )
}

function ProductionOption({
  value,
  current,
  onPick,
  label,
  icon,
  destructive,
}: {
  value: ProductionTriState
  current: ProductionTriState
  onPick: (v: ProductionTriState) => void
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
