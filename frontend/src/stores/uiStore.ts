import { create } from 'zustand'

import type { ExecuteResponseBody } from '@/lib/api/types'

/**
 * UIStore holds client-only state that is shared across components but
 * does not belong in TanStack Query (which owns server state).
 *
 * Three things live here for F3:
 *   - `sql`: the current editor buffer, so Run / Ask / Review can see it
 *     without lifting every one of them into the QueryPage component.
 *   - `pendingEditorText`: a one-shot slot the AI dialogs (F5) write into
 *     when the user hits "Insert into editor"; SqlEditor picks it up via
 *     an effect and dispatches it into the CodeMirror view.
 *   - `lastResult`: the most recent /api/execute response, needed by the
 *     Analyze flow to attach the actual rows to the prompt.
 */
interface UIState {
  sql: string
  setSql: (sql: string) => void

  pendingEditorText: string | null
  requestEditorText: (text: string) => void
  clearEditorText: () => void

  lastResult: ExecuteResponseBody | null
  setLastResult: (r: ExecuteResponseBody | null) => void
}

export const useUIStore = create<UIState>((set) => ({
  sql: '',
  setSql: (sql) => set({ sql }),

  pendingEditorText: null,
  requestEditorText: (text) => set({ pendingEditorText: text }),
  clearEditorText: () => set({ pendingEditorText: null }),

  lastResult: null,
  setLastResult: (r) => set({ lastResult: r }),
}))
