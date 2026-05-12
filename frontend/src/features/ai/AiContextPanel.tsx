import { useState } from 'react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import {
  useAiContext,
  useDeleteAiContext,
  useSaveAiContext,
} from '@/features/ai/useAiContext'
import type { AiContext } from '@/lib/api/types'

// The server is the source of truth for the byte cap (echoed via
// AiContext.maxContentBytes); we measure UTF-8 bytes here instead of
// String#length so multi-byte input (Japanese, emoji, …) hits the cap
// at the same boundary the server enforces.
const utf8 = new TextEncoder()
function byteLength(s: string): number {
  return utf8.encode(s).length
}

const PLACEHOLDER = `Examples (free-form, Markdown OK):

# Glossary
- active user: last_login_at is within the last 30 days
- order: orders.deleted_at IS NULL only

# Conventions
- All amount columns are stored in cents (integers).
- Use TIMESTAMPTZ; do not assume server timezone.

# Sample
"top spenders this month" → JOIN users with orders, sum order_total, GROUP BY user, ORDER BY DESC.`

/**
 * AiContextPanel is the per-(cluster, database) prompt-context editor on
 * the Settings page. The text written here is appended to the system
 * prompt of every Bedrock call (Ask, Explain, Review, Analyze) so the
 * model has business-domain context that pure schema introspection
 * cannot provide.
 *
 * Storage is on disk under ~/.rdq/aictx/<sha256(cluster:database)>.json
 * via the GET/PUT/DELETE /api/aictx endpoints.
 *
 * The wrapper handles the async load; the inner Editor only mounts once
 * data is present and is keyed on (cluster, database, updatedAt) so it
 * remounts cleanly after a save or a connection switch — no setState in
 * useEffect needed.
 */
export function AiContextPanel({
  cluster,
  database,
}: {
  cluster: string
  database: string
}) {
  const ctx = useAiContext(cluster, database)

  if (!cluster || !database) {
    return (
      <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
        Select a cluster and database to edit AI context for this connection.
      </div>
    )
  }

  if (ctx.isLoading) {
    return <p className="text-sm text-muted-foreground">Loading AI context…</p>
  }
  if (ctx.isError) {
    return (
      <p className="text-sm text-destructive">
        Could not load AI context: {(ctx.error as Error).message}
      </p>
    )
  }
  if (!ctx.data) {
    return null
  }

  const seedKey = `${cluster}|${database}|${ctx.data.updatedAt ?? 'empty'}`

  return (
    <Editor
      key={seedKey}
      cluster={cluster}
      database={database}
      saved={ctx.data}
    />
  )
}

function Editor({
  cluster,
  database,
  saved,
}: {
  cluster: string
  database: string
  saved: AiContext
}) {
  const [content, setContent] = useState(() => saved.content ?? '')

  const save = useSaveAiContext(cluster, database)
  const del = useDeleteAiContext(cluster, database)

  const trimmed = content.trim()
  const bytes = byteLength(content)
  const maxBytes = saved.maxContentBytes
  const overLimit = bytes > maxBytes
  const dirty = trimmed !== (saved.content ?? '').trim()
  const hasSaved = (saved.content ?? '').trim() !== ''

  const onSave = () => {
    if (!trimmed) {
      toast.error('Content is empty. Use Clear to remove.')
      return
    }
    if (overLimit) {
      toast.error(`Content exceeds ${maxBytes.toLocaleString()} bytes.`)
      return
    }
    save.mutate(trimmed, {
      onSuccess: () => toast.success('AI context saved'),
      onError: (err: Error) => toast.error(err.message),
    })
  }

  const onClear = () => {
    if (!hasSaved && trimmed === '') return
    del.mutate(undefined, {
      onSuccess: () => {
        setContent('')
        toast.success('AI context cleared')
      },
      onError: (err: Error) => toast.error(err.message),
    })
  }

  return (
    <div className="space-y-3">
      <Textarea
        value={content}
        onChange={(e) => setContent(e.target.value)}
        placeholder={PLACEHOLDER}
        className="min-h-[260px] font-mono text-xs"
      />
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span className={overLimit ? 'text-destructive' : undefined}>
          {bytes.toLocaleString()} / {maxBytes.toLocaleString()} bytes
        </span>
        {saved.updatedAt && (
          <span>Last saved: {new Date(saved.updatedAt).toLocaleString()}</span>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button onClick={onSave} disabled={save.isPending || !dirty || overLimit || !trimmed}>
          {save.isPending ? 'Saving…' : 'Save context'}
        </Button>
        <Button
          variant="outline"
          onClick={onClear}
          disabled={del.isPending || (!hasSaved && trimmed === '')}
        >
          {del.isPending ? 'Clearing…' : 'Clear'}
        </Button>
      </div>
    </div>
  )
}
