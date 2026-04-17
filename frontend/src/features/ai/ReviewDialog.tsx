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
import { useSession } from '@/hooks/useSession'
import { endpoints } from '@/lib/api/endpoints'
import { useUIStore } from '@/stores/uiStore'

import { AiMarkdown } from './AiMarkdown'
import { ModelPicker } from './ModelPicker'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ReviewDialog({ open, onOpenChange }: Props) {
  const session = useSession()
  const sql = useUIStore((s) => s.sql)
  const [model, setModel] = useState(() => session.data?.bedrockModel ?? '')
  const [language, setLanguage] = useState(() => session.data?.bedrockLanguage ?? 'Japanese')
  const [focus, setFocus] = useState('')

  const review = useMutation({
    mutationFn: () =>
      endpoints.review({
        profile: session.data?.profile ?? '',
        cluster: session.data?.cluster ?? '',
        database: session.data?.database ?? '',
        modelId: model,
        language,
        sql,
        focus,
      }),
    onError: (err: Error) => toast.error(err.message),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Review SQL</DialogTitle>
          <DialogDescription>
            Ask the model to critique the current editor buffer for correctness, performance, and style.
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
            <Label>Focus (optional)</Label>
            <Input
              value={focus}
              onChange={(e) => setFocus(e.target.value)}
              placeholder="e.g. performance, NULL handling"
            />
          </div>
        </div>
        <ScrollArea className="max-h-[50vh] rounded-md border border-border bg-muted/30 p-3">
          {review.data ? (
            <AiMarkdown markdown={review.data.text} />
          ) : (
            <p className="text-xs text-muted-foreground">
              Press Review to send the editor SQL to the model.
            </p>
          )}
        </ScrollArea>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Close
          </Button>
          <Button onClick={() => review.mutate()} disabled={review.isPending || !sql || !model}>
            {review.isPending ? 'Reviewing…' : 'Review'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
