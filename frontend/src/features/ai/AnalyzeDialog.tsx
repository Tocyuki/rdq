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
import { toCSV } from '@/lib/csv'
import { useUIStore } from '@/stores/uiStore'

import { AiMarkdown } from './AiMarkdown'
import { ModelPicker } from './ModelPicker'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const analyzeBlobLimit = 8 * 1024

export function AnalyzeDialog({ open, onOpenChange }: Props) {
  const session = useSession()
  const sql = useUIStore((s) => s.sql)
  const lastResult = useUIStore((s) => s.lastResult)
  const [model, setModel] = useState(() => session.data?.bedrockModel ?? '')
  const [language, setLanguage] = useState(() => session.data?.bedrockLanguage ?? 'Japanese')
  const [focus, setFocus] = useState('')

  const hasResult = !!lastResult && lastResult.rows.length > 0

  const analyze = useMutation({
    mutationFn: () => {
      if (!lastResult) throw new Error('No result available to analyze')
      let resultBlob = toCSV(lastResult.columns, lastResult.rows)
      if (resultBlob.length > analyzeBlobLimit) {
        resultBlob = resultBlob.slice(0, analyzeBlobLimit) + '\n... (truncated)'
      }
      return endpoints.analyze({
        profile: session.data?.profile ?? '',
        cluster: session.data?.cluster ?? '',
        database: session.data?.database ?? '',
        modelId: model,
        language,
        sql,
        resultBlob,
        focus,
      })
    },
    onError: (err: Error) => toast.error(err.message),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Analyze result</DialogTitle>
          <DialogDescription>
            Ask the model to look for patterns, outliers, or quality issues in the last result.
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
              placeholder="e.g. find anomalies, summarise by category"
            />
          </div>
        </div>
        {!hasResult && (
          <p className="rounded-md border border-border bg-muted/40 p-3 text-xs text-muted-foreground">
            Run a query first — Analyze operates on the most recent result.
          </p>
        )}
        <ScrollArea className="max-h-[50vh] rounded-md border border-border bg-muted/30 p-3">
          {analyze.data ? (
            <AiMarkdown markdown={analyze.data.text} />
          ) : (
            <p className="text-xs text-muted-foreground">
              Press Analyze to send the current result to the model.
            </p>
          )}
        </ScrollArea>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Close
          </Button>
          <Button
            onClick={() => analyze.mutate()}
            disabled={analyze.isPending || !hasResult || !model}
          >
            {analyze.isPending ? 'Analyzing…' : 'Analyze'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
