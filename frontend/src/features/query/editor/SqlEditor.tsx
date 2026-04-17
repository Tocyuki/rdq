import { useEffect, useMemo, useRef } from 'react'
import CodeMirror, { type ReactCodeMirrorRef } from '@uiw/react-codemirror'

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
 *   - Consumes pendingEditorText (written by AI dialogs) via a
 *     dispatch call so the SPA can replace the editor text without a
 *     round-trip through React state.
 */
export function SqlEditor({ engineHint, schema, onRun }: Props) {
  const sql = useUIStore((s) => s.sql)
  const setSql = useUIStore((s) => s.setSql)
  const pending = useUIStore((s) => s.pendingEditorText)
  const clearPending = useUIStore((s) => s.clearEditorText)
  const cmRef = useRef<ReactCodeMirrorRef>(null)

  const engine: Engine = engineFromClusterEngine(engineHint)
  const schemaHint = useMemo(() => toSqlSchemaHint(schema), [schema])
  const extensions = useMemo(
    () => createSqlExtensions({ engine, onRun, schema: schemaHint }),
    [engine, onRun, schemaHint],
  )

  // Apply pending text from AI Insert action: imperative dispatch is the
  // CodeMirror-native way to replace the doc without a React round trip.
  useEffect(() => {
    if (!pending) return
    const view = cmRef.current?.view
    if (!view) return
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: pending },
    })
    setSql(pending)
    clearPending()
  }, [pending, setSql, clearPending])

  return (
    <div className="h-full overflow-hidden rounded-md border border-border">
      <CodeMirror
        ref={cmRef}
        value={sql}
        theme="dark"
        height="100%"
        extensions={extensions}
        onChange={setSql}
        basicSetup={{
          lineNumbers: true,
          foldGutter: true,
          highlightActiveLine: true,
        }}
      />
    </div>
  )
}
