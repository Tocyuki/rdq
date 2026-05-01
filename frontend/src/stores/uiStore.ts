import { create } from 'zustand'
import type { EditorView } from '@codemirror/view'

import type { ExecuteResponseBody } from '@/lib/api/types'

export type PreviewSort = { column: string; dir: 'asc' | 'desc' } | null

export type PreviewFilterOp =
  | '='
  | '<>'
  | '>'
  | '<'
  | '>='
  | '<='
  | 'contains'
  | 'starts with'
  | 'ends with'
  | 'is null'
  | 'is not null'

export interface PreviewFilter {
  column: string
  op: PreviewFilterOp
  value: string
}

/**
 * UIStore holds client-only state that is shared across components but
 * does not belong in TanStack Query (which owns server state).
 *
 * - `sql`: the current editor buffer, so Run / Ask / Review can see it
 *   without lifting every one of them into the SqlEditorPage component.
 * - `pendingEditorText`: a one-shot slot consumed by SqlEditor on mount
 *   to replace the doc. AI "Insert into editor", history "Load", and
 *   schema tree click-to-insert all funnel through this slot so they
 *   share the same dispatch path.
 * - `lastResult`: the most recent /api/execute response, needed by the
 *   Analyze flow to attach the actual rows to the prompt.
 */
interface UIState {
  sql: string
  setSql: (sql: string) => void

  pendingEditorText: string | null
  requestEditorText: (text: string) => void
  clearEditorText: () => void

  lastResult: ExecuteResponseBody | null
  setLastResult: (r: ExecuteResponseBody | null) => void

  /**
   * connectionDialogOpen centralises the open state of the
   * <ConnectionDialog /> so the dialog lives in exactly one place
   * (SessionGate) while both SessionGate (first-load auto-open) and
   * ConnectionBar (profile-switch / "Change" button) can drive it.
   * Previously the dialog was mounted twice, which stacked two backdrops
   * on profile switches.
   */
  connectionDialogOpen: boolean
  setConnectionDialogOpen: (open: boolean) => void

  /**
   * editorView is the CodeMirror view instance. Kept in the store so
   * the Run button (outside the editor subtree) can resolve the
   * currently-selected text at run time without lifting state. Holding
   * only the view — not the selection itself — keeps updates to
   * mount/unmount frequency; selection is read on demand via
   * `view.state.selection.main`.
   */
  editorView: EditorView | null
  setEditorView: (view: EditorView | null) => void

  /**
   * previewTarget drives the Supabase / Drizzle Studio-style table
   * preview shown by <PreviewPanel /> on the /query route. Non-null =
   * preview is showing the named table. The SQL editor buffer is never
   * touched while preview is on, so the user's in-progress SQL on the
   * /sql route stays intact when they hop over to browse data.
   *
   * `sort` and `filters` are both reflected in the SQL (ORDER BY +
   * WHERE) so paging happens against the server-side filtered set,
   * not just the in-memory 100-row page.
   */
  previewTarget:
    | {
        schema: string
        table: string
        offset: number
        sort: PreviewSort
        filters: PreviewFilter[]
      }
    | null
  openPreview: (schema: string, table: string) => void
  setPreviewOffset: (offset: number) => void
  setPreviewSort: (sort: PreviewSort) => void
  setPreviewFilters: (filters: PreviewFilter[]) => void
  closePreview: () => void
}

export const useUIStore = create<UIState>((set) => ({
  sql: '',
  setSql: (sql) => set({ sql }),

  pendingEditorText: null,
  requestEditorText: (text) => set({ pendingEditorText: text }),
  clearEditorText: () => set({ pendingEditorText: null }),

  lastResult: null,
  setLastResult: (r) => set({ lastResult: r }),

  connectionDialogOpen: false,
  setConnectionDialogOpen: (open) => set({ connectionDialogOpen: open }),

  editorView: null,
  setEditorView: (view) => set({ editorView: view }),

  previewTarget: null,
  openPreview: (schema, table) =>
    set({ previewTarget: { schema, table, offset: 0, sort: null, filters: [] } }),
  setPreviewOffset: (offset) =>
    set((s) => (s.previewTarget ? { previewTarget: { ...s.previewTarget, offset } } : s)),
  setPreviewSort: (sort) =>
    set((s) =>
      s.previewTarget ? { previewTarget: { ...s.previewTarget, sort, offset: 0 } } : s,
    ),
  setPreviewFilters: (filters) =>
    set((s) =>
      s.previewTarget ? { previewTarget: { ...s.previewTarget, filters, offset: 0 } } : s,
    ),
  closePreview: () => set({ previewTarget: null }),
}))

/**
 * resolveRunSql returns the SQL that should actually be dispatched to
 * the Data API. When the user has a non-empty selection in the editor
 * only that slice is run (common SQL-client QoL). Otherwise the full
 * buffer is used. Keeping this as a pure function of (fullSql, view)
 * keeps the Run button and Cmd/Ctrl+Enter paths in lockstep — both
 * resolve the effective SQL the same way at dispatch time.
 */
export function resolveRunSql(fullSql: string, view: EditorView | null): string {
  if (!view) return fullSql
  const sel = view.state.selection.main
  if (sel.empty) return fullSql
  return view.state.sliceDoc(sel.from, sel.to)
}
