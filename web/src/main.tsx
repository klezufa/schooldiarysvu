import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from './App'
import { AuthProvider } from './auth'
import { StudentProvider } from './student'
import { ToastProvider } from './components/Toast'
import { ErrorBoundary } from './components/UI'
import './styles.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: (count, error) => !(error instanceof Error && 'status' in error && [401, 403, 404, 409, 422, 429].includes(Number(error.status))) && count < 1, staleTime: 15_000 },
    mutations: { retry: false },
  },
})

const root = document.getElementById('root')
if (!root) throw new Error('Root element not found')

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <ToastProvider>
          <AuthProvider>
            <StudentProvider>
              <ErrorBoundary><App /></ErrorBoundary>
            </StudentProvider>
          </AuthProvider>
        </ToastProvider>
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
