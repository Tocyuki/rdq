import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
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
import { ScrollArea } from '@/components/ui/scroll-area'
import { Textarea } from '@/components/ui/textarea'
import { useSession } from '@/hooks/useSession'
import { endpoints } from '@/lib/api/endpoints'
import { useUIStore } from '@/stores/uiStore'

import { AiMarkdown } from './AiMarkdown'
import { ModelPicker } from './ModelPicker'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialError?: string
}

/**
 * ExplainDialog asks the model to diagnose a SQL execution error. It is
 * typically opened from the result panel's error branch but can also be
 * triggered manually; users can paste an arbitrary error message into
 * the textarea if the last-result flow isn't the right context.
 */
export function ExplainDialog({ open, onOpenChange, initialError }: Props) {
  const session = useSession()
  const sql = useUIStore((s) => s.sql)
  const [model, setModel] = useState(() => session.data?.bedrockModel ?? '')
  const [language, setLanguage] = useState(() => session.data?.bedrockLanguage ?? 'Japanese')
  const [errorMsg, setErrorMsg] = useState(initialError ?? '')

  const explain = useMutation({
    mutationFn: () =>
      endpoints.explain({
        profile: session.data?.profile ?? '',
        cluster: session.data?.cluster ?? '',
        database: session.data?.database ?? '',
        modelId: model,
        language,
        sql,
        errorMsg,
      }),
    onError: (err: Error) => toast.error(err.message),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Explain error</DialogTitle>
          <DialogDescription>
            Ask the model to suggest why a statement failed and how to fix it.
          </DialogDescription>
        </DialogHeader>
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1">
            <Label>Model</Label>
            <ModelPicker value={model} onChange={setModel} />
          </div>
          <div className="space-y-1">
            <Label>Language</Label>
            <Input value={language} onChange={(e) => setLanguage(e.target.value)} />
          </div>
          <div className="col-span-2 space-y-1">
            <Label>Error message</Label>
            <Textarea
              value={errorMsg}
              onChange={(e) => setErrorMsg(e.target.value)}
              rows={3}
              placeholder="e.g. ERROR: relation 'users' does not exist"
            />
          </div>
        </div>
        <ScrollArea className="max-h-[50vh] rounded-md border border-border bg-muted/30 p-3">
          {explain.data ? (
            <AiMarkdown markdown={explain.data.text} />
          ) : (
            <p className="text-xs text-muted-foreground">
              Press Explain to send the SQL + error to the model.
            </p>
          )}
        </ScrollArea>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Close
          </Button>
          <Button
            onClick={() => explain.mutate()}
            disabled={explain.isPending || !errorMsg || !model}
          >
            {explain.isPending ? 'Explaining…' : 'Explain'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
