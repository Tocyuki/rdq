import { create } from 'zustand'

import type { ExecuteResponseBody } from '@/lib/api/types'

/**
 * UIStore holds client-only state that is shared across components but
 * does not belong in TanStack Query (which owns server state).
 *
 * - `sql`: the current editor buffer, so Run / Ask / Review can see it
 *   without lifting every one of them into the QueryPage component.
 * - `pendingEditorText`: a one-shot slot consumed by SqlEditor on mount
 *   to replace the doc. AI "Insert into editor", history "Load", and
 *   schema tree double-click all funnel through this slot so they share
 *   the same dispatch path.
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
