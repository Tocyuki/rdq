import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import { Toaster } from '@/components/ui/sonner'
import { bootstrapAPIToken } from '@/lib/api/session-token'
import { SessionGate } from '@/providers/SessionGate'
import App from './App.tsx'
import './index.css'

/**
 * Central query client for the SPA. staleTime of 30s keeps profile /
 * cluster / secret lists from re-fetching on every focus while still being
 * short enough that the user sees changes after a deliberate refresh.
 * retry is 1 because these endpoints all talk to AWS through our own
 * backend; transient failures are common and cheap to retry.
 */
const qc = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 30_000, retry: 1, refetchOnWindowFocus: false },
    mutations: { retry: 0 },
  },
})

const hasAPIToken = bootstrapAPIToken()

function MissingSessionTokenScreen() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6">
      <div className="max-w-md rounded-lg border border-border bg-card p-6 text-sm shadow-sm">
        <h1 className="text-base font-semibold tracking-tight">GUI session not authorized</h1>
        <p className="mt-2 text-muted-foreground">
          This page was opened without the per-run launch token. Start the GUI with{' '}
          <code className="font-mono">rdq gui</code> and use the browser window it opens,
          or the full launch URL printed by <code className="font-mono">rdq gui --no-open</code>.
        </p>
      </div>
    </div>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    {hasAPIToken ? (
      <QueryClientProvider client={qc}>
        <BrowserRouter>
          <SessionGate>
            <App />
          </SessionGate>
          <Toaster />
        </BrowserRouter>
      </QueryClientProvider>
    ) : (
      <MissingSessionTokenScreen />
    )}
  </StrictMode>,
)
