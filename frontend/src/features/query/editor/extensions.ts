import { autocompletion } from '@codemirror/autocomplete'
import { defaultKeymap, indentWithTab } from '@codemirror/commands'
import { MySQL, PostgreSQL, sql } from '@codemirror/lang-sql'
import { oneDark } from '@codemirror/theme-one-dark'
import { EditorView, keymap } from '@codemirror/view'
import { Prec, type Extension } from '@codemirror/state'

/**
 * Engine identifies the SQL dialect CodeMirror should highlight. Values
 * match what ClusterInfo.engine returns from the Go server
 * ("aurora-mysql", "aurora-postgresql", etc.).
 */
export type Engine = 'mysql' | 'postgres'

export function engineFromClusterEngine(s: string | undefined): Engine {
  if (!s) return 'postgres'
  return s.toLowerCase().includes('postgres') ? 'postgres' : 'mysql'
}

interface Opts {
  engine: Engine
  onRun: () => void
  /** Table → columns map for schema-aware autocomplete. Empty object is fine. */
  schema?: Record<string, string[]>
}

/**
 * createSqlExtensions returns the CodeMirror extension list used by the
 * SqlEditor. It wires the dialect, autocompletion, one-dark theme, and
 * the Mod-Enter "run" shortcut. Passing a non-empty `schema` unlocks
 * table / column name completion inside the editor.
 *
 * The Run shortcut is wrapped in Prec.highest so it wins against the
 * default Mod-Enter binding (insertBlankLine) that @uiw/react-codemirror
 * registers via its bundled basicSetup.
 */
export function createSqlExtensions({ engine, onRun, schema }: Opts): Extension[] {
  return [
    sql({
      dialect: engine === 'mysql' ? MySQL : PostgreSQL,
      upperCaseKeywords: true,
      schema: schema && Object.keys(schema).length > 0 ? schema : undefined,
    }),
    autocompletion(),
    oneDark,
    EditorView.lineWrapping,
    Prec.highest(
      keymap.of([
        {
          key: 'Mod-Enter',
          preventDefault: true,
          run: () => {
            onRun()
            return true
          },
        },
      ]),
    ),
    keymap.of([...defaultKeymap, indentWithTab]),
  ]
}
