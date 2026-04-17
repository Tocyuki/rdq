import { Outlet } from 'react-router-dom'

import { ConnectionBar } from './ConnectionBar'
import { SidebarNav } from './SidebarNav'

/**
 * AppShell is the frame every page renders inside: a left sidebar nav, a
 * top connection bar, and an Outlet for route content. The shell itself is
 * fixed to the viewport so page scrolling happens inside the <main> element
 * rather than on the body.
 */
export function AppShell() {
  return (
    <div className="flex h-full w-full">
      <SidebarNav />
      <div className="flex min-w-0 flex-1 flex-col">
        <ConnectionBar />
        <main className="min-h-0 flex-1 overflow-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
