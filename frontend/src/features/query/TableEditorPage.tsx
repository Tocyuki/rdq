import { SquareCode, Table } from 'lucide-react'
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'

import { SchemaSidebar } from '@/features/schema/SchemaSidebar'
import { useUIStore } from '@/stores/uiStore'

import { PreviewPanel } from './PreviewPanel'

/**
 * TableEditorPage is the Supabase-style "Table Editor": a schema
 * sidebar on the left and a preview pane on the right that fills with
 * the clicked table's contents. SQL editing lives on the separate
 * /sql route (see SqlEditorPage). Mounted at /query for backward
 * compatibility with the previous nav structure.
 */
export function TableEditorPage() {
  const previewTarget = useUIStore((s) => s.previewTarget)

  return (
    <div className="h-full">
      <PanelGroup direction="horizontal" autoSaveId="rdq:query:horizontal" className="h-full">
        <Panel defaultSize={22} minSize={14} maxSize={45}>
          <SchemaSidebar />
        </Panel>
        <PanelResizeHandle className="group w-1 bg-border transition-colors hover:bg-ring data-[resize-handle-state=drag]:bg-ring" />
        <Panel defaultSize={78} minSize={35}>
          {previewTarget ? <PreviewPanel /> : <WelcomeState />}
        </Panel>
      </PanelGroup>
    </div>
  )
}

function WelcomeState() {
  return (
    <div className="flex h-full items-center justify-center bg-background p-8">
      <div className="flex max-w-sm flex-col items-center gap-3 text-center">
        <div className="flex size-12 items-center justify-center rounded-full bg-primary/10 text-primary">
          <Table className="size-6" />
        </div>
        <h2 className="text-base font-semibold tracking-tight">Pick a table to browse</h2>
        <p className="text-sm text-muted-foreground">
          Click any table on the left to see the first 100 rows. Use the
          filter and sort controls to narrow it down. To run a hand-written
          SQL query, switch to the{' '}
          <SquareCode className="inline size-3.5 align-text-bottom" /> SQL editor
          in the sidebar.
        </p>
      </div>
    </div>
  )
}
