import { create } from 'zustand'

import type { ExecuteResponseBody } from '@/lib/api/types'

/**
 * UIStore holds client-only state that is shared across components but
 * does not belong in TanStack Query (which owns server state).
 *
 * - `sql`: the current editor buffer, so Run / Ask / Review can see it
 *   without lifting every one of them into the QueryPage component.
 * - `pendingEditorText` + `pendingAutoRun`: a one-shot slot consumed by
 *   SqlEditor on mount. AI dialogs ("Insert into editor") write with
 *   autoRun=false; the History page writes with autoRun=true so the
 *   statement is loaded *and* immediately executed by QueryPage.
 * - `lastResult`: the most recent /api/execute response, needed by the
 *   Analyze flow to attach the actual rows to the prompt.
 */
interface UIState {
  sql: string
  setSql: (sql: string) => void

  pendingEditorText: string | null
  pendingAutoRun: boolean
  requestEditorText: (text: string, options?: { autoRun?: boolean }) => void
  clearEditorText: () => void
  clearAutoRun: () => void

  lastResult: ExecuteResponseBody | null
  setLastResult: (r: ExecuteResponseBody | null) => void
}

export const useUIStore = create<UIState>((set) => ({
  sql: '',
  setSql: (sql) => set({ sql }),

  pendingEditorText: null,
  pendingAutoRun: false,
  requestEditorText: (text, options) =>
    set({ pendingEditorText: text, pendingAutoRun: options?.autoRun === true }),
  clearEditorText: () => set({ pendingEditorText: null }),
  clearAutoRun: () => set({ pendingAutoRun: false }),

  lastResult: null,
  setLastResult: (r) => set({ lastResult: r }),
}))
