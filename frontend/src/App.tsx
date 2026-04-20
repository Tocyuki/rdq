import { Navigate, Route, Routes } from 'react-router-dom'

import { AppShell } from '@/components/layout/AppShell'
import { HistoryPage } from '@/features/history/HistoryPage'
import { QueryPage } from '@/features/query/QueryPage'
import { SchemaPage } from '@/features/schema/SchemaPage'
import { SettingsPage } from '@/features/settings/SettingsPage'

/**
 * App is the route table. All routes share AppShell so the sidebar and
 * connection bar never remount across navigations — this keeps CodeMirror
 * state, result tables, etc. alive when the user flips between tabs.
 */
export default function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<Navigate to="/query" replace />} />
        <Route path="/query" element={<QueryPage />} />
        <Route path="/history" element={<HistoryPage />} />
        <Route path="/schema" element={<SchemaPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/query" replace />} />
      </Route>
    </Routes>
  )
}
