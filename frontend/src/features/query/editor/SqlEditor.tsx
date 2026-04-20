import { useEffect, useMemo, useState } from 'react'
import CodeMirror from '@uiw/react-codemirror'
import type { EditorView } from '@codemirror/view'

import { useUIStore } from '@/stores/uiStore'
import { createSqlExtensions, engineFromClusterEngine, type Engine } from './extensions'
import { toSqlSchemaHint } from '@/features/schema/schemaToCompletions'
import type { Schema } from '@/lib/api/types'

interface Props {
  engineHint?: string
  schema?: Schema
  onRun: () => void
}

/**
 * SqlEditor is the CodeMirror wrapper. It is the single owner of the
 * editor state:
 *   - Reads the current buffer from Zustand on render.
 *   - Writes back on every onChange.
 *   - Consumes pendingEditorText (written by AI dialogs, History, etc.)
 *     via an imperative dispatch so the SPA can replace the doc without
 *     forcing a full React re-render.
 *
 * The EditorView is tracked via `onCreateEditor` → useState so the
 * pending-text effect fires both when `pending` changes *and* when the
 * view becomes available. An earlier ref-only implementation silently
 * skipped the first run when CodeMirror had not yet mounted the view.
 */
export function SqlEditor({ engineHint, schema, onRun }: Props) {
  const sql = useUIStore((s) => s.sql)
  const setSql = useUIStore((s) => s.setSql)
  const pending = useUIStore((s) => s.pendingEditorText)
  const clearPending = useUIStore((s) => s.clearEditorText)
  const [view, setView] = useState<EditorView | null>(null)

  const engine: Engine = engineFromClusterEngine(engineHint)
  const schemaHint = useMemo(() => toSqlSchemaHint(schema), [schema])
  const extensions = useMemo(
    () => createSqlExtensions({ engine, onRun, schema: schemaHint }),
    [engine, onRun, schemaHint],
  )

  useEffect(() => {
    if (!pending || !view) return
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: pending },
    })
    setSql(pending)
    clearPending()
  }, [pending, view, setSql, clearPending])

  return (
    <div className="h-full overflow-hidden rounded-md border border-border">
      <CodeMirror
        value={sql}
        theme="dark"
        height="100%"
        // ReactCodeMirror's outer <div> is auto-sized by default, so
        // `.cm-editor { height: 100% }` (injected by the height prop)
        // collapses to content height and mouse-wheel scroll never
        // engages. Forcing h-full on the wrapper gives CodeMirror a
        // concrete parent height and the internal .cm-scroller then
        // overflows and receives wheel events as expected.
        className="h-full"
        extensions={extensions}
        onChange={setSql}
        onCreateEditor={setView}
        basicSetup={{
          lineNumbers: true,
          foldGutter: true,
          highlightActiveLine: true,
        }}
      />
    </div>
  )
}
