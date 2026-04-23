import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import { Toaster } from '@/components/ui/sonner'
import { bootstrapAPIToken } from '@/lib/api/session-token'
import { MissingSessionTokenScreen } from '@/providers/MissingSessionTokenScreen'
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
