import { useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ModelPicker } from '@/features/ai/ModelPicker'
import { useSession } from '@/hooks/useSession'
import { endpoints } from '@/lib/api/endpoints'

/**
 * SettingsPage exposes two pieces of per-profile configuration: the
 * Bedrock model used by AI dialogs, and the natural language the model
 * replies in. Both pass through PUT /api/session which persists them
 * in state.json so the choice survives restarts.
 */
export function SettingsPage() {
  const session = useSession()
  const [model, setModel] = useState(() => session.data?.bedrockModel ?? '')
  const [language, setLanguage] = useState(() => session.data?.bedrockLanguage ?? '')

  const save = useMutation({
    mutationFn: () => {
      if (!session.data) throw new Error('Session not loaded')
      return endpoints.putSession({
        ...session.data,
        bedrockModel: model,
        bedrockLanguage: language,
      })
    },
    onSuccess: () => {
      toast.success('Settings saved')
      // refetch rather than setQueryData so state.json → session round trip is tested.
      session.refetch()
    },
    onError: (err: Error) => toast.error(err.message),
  })

  return (
    <section className="flex h-full flex-col p-6">
      <h1 className="text-xl font-semibold tracking-tight">Settings</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        AI model &amp; language preferences (per profile, saved to state.json).
      </p>

      <div className="mt-6 grid max-w-lg gap-4">
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
        <div>
          <Button onClick={() => save.mutate()} disabled={save.isPending}>
            {save.isPending ? 'Saving…' : 'Save'}
          </Button>
        </div>
      </div>
    </section>
  )
}
