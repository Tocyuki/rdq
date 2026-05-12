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
import type { Message } from '@/lib/api/types'
import { useUIStore } from '@/stores/uiStore'

import { AiMarkdown } from './AiMarkdown'
import { ModelPicker } from './ModelPicker'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  onAutoRun: (sql: string) => void
}

/**
 * AskDialog turns natural language into SQL via /api/ai/ask. The
 * conversation is local to the dialog session so multi-turn context is
 * preserved while the user iterates. "Insert into editor" replaces the
 * CodeMirror buffer via the pendingEditorText slot.
 */
export function AskDialog({ open, onOpenChange, onAutoRun }: Props) {
  const session = useSession()
  const [model, setModel] = useState(() => session.data?.bedrockModel ?? '')
  const [language, setLanguage] = useState(() => session.data?.bedrockLanguage ?? 'Japanese')
  const [prompt, setPrompt] = useState('')
  const [messages, setMessages] = useState<Message[]>([])
  const requestEditorText = useUIStore((s) => s.requestEditorText)

  const ask = useMutation({
    mutationFn: (next: Message[]) =>
      endpoints.ask({
        profile: session.data?.profile ?? '',
        cluster: session.data?.cluster ?? '',
        database: session.data?.database ?? '',
        modelId: model,
        language,
        messages: next,
      }),
  })

  async function onSubmit() {
    if (!prompt.trim() || !model) {
      toast.error(!model ? 'Pick a model first.' : 'Enter a prompt.')
      return
    }
    const nextMessages: Message[] = [...messages, { role: 'user', text: prompt }]
    setMessages(nextMessages)
    setPrompt('')
    try {
      const res = await ask.mutateAsync(nextMessages)
      setMessages([...nextMessages, { role: 'assistant', text: res.sql }])

      // Auto-run shortcut: when the per-profile toggle is on AND the
      // server's runner.IsReadOnlySQL agrees the statement is a pure
      // read, fire it straight into /api/execute, drop the SQL into the
      // editor for transparency, and close the dialog so results land
      // on the main panel. Anything destructive (or with auto-run off)
      // falls through to the existing "Insert into editor" / Run flow.
      const auto =
        session.data?.autoRunReadOnly === true &&
        res.autoRunnable &&
        !!session.data?.profile &&
        !!session.data.cluster &&
        !!session.data.secret &&
        !!session.data.database
      if (auto) {
        requestEditorText(res.sql)
        onOpenChange(false)
        onAutoRun(res.sql)
        toast.success('Auto-running read-only SQL…')
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Ask failed')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Ask AI</DialogTitle>
          <DialogDescription>
            Describe what you want to query; the model returns SQL you can insert into the editor.
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
        </div>

        <ScrollArea className="max-h-[35vh] rounded-md border border-border bg-muted/30 p-3">
          {messages.length === 0 ? (
            <p className="text-xs text-muted-foreground">No turns yet — type a request below.</p>
          ) : (
            messages.map((m, i) => (
              <div key={i} className="mb-3">
                <div className="mb-1 text-[10px] uppercase tracking-wide text-muted-foreground">
                  {m.role}
                </div>
                {m.role === 'assistant' ? (
                  <AiMarkdown markdown={'```sql\n' + m.text + '\n```'} />
                ) : (
                  <p className="whitespace-pre-wrap text-sm">{m.text}</p>
                )}
                {m.role === 'assistant' && (
                  <div className="mt-1 flex gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        requestEditorText(m.text)
                        toast.success('Inserted into editor')
                        onOpenChange(false)
                      }}
                    >
                      Insert into editor
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => navigator.clipboard.writeText(m.text)}
                    >
                      Copy
                    </Button>
                  </div>
                )}
              </div>
            ))
          )}
        </ScrollArea>

        <div className="space-y-1">
          <Label>Your question</Label>
          <Input
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                e.preventDefault()
                onSubmit()
              }
            }}
            placeholder="Find the top 10 customers by revenue this month"
          />
          <p className="text-[10px] text-muted-foreground">Cmd / Ctrl + Enter to send</p>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => setMessages([])} disabled={ask.isPending}>
            Clear
          </Button>
          <Button onClick={onSubmit} disabled={ask.isPending}>
            {ask.isPending ? 'Asking…' : 'Ask'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
