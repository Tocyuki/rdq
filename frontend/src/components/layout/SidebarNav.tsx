import { NavLink } from 'react-router-dom'
import { Database, History, Settings, SquareCode, Table2 } from 'lucide-react'

import { cn } from '@/lib/utils'

/**
 * SidebarNav is the thin vertical rail of icons on the left edge of the
 * app. Icon-only to preserve real estate for the SQL editor / results.
 */
const items = [
  { to: '/query', label: 'Table editor', Icon: Table2 },
  { to: '/sql', label: 'SQL editor', Icon: SquareCode },
  { to: '/history', label: 'History', Icon: History },
  { to: '/schema', label: 'Schema', Icon: Database },
  { to: '/settings', label: 'Settings', Icon: Settings },
]

export function SidebarNav() {
  return (
    <nav
      aria-label="Primary"
      className="flex h-full w-14 flex-col items-center gap-1 border-r border-border bg-card py-3"
    >
      {items.map(({ to, label, Icon }) => (
        <NavLink
          key={to}
          to={to}
          title={label}
          aria-label={label}
          className={({ isActive }) =>
            cn(
              'flex size-10 items-center justify-center rounded-md text-muted-foreground transition-colors',
              'hover:bg-accent hover:text-accent-foreground',
              isActive && 'bg-accent text-accent-foreground',
            )
          }
        >
          <Icon className="size-5" />
        </NavLink>
      ))}
    </nav>
  )
}
