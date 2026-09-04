import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import axios from 'axios'
import App from './App.tsx'
import { ConfirmProvider } from '@/components/primitives'
import { clientErrorLogger } from '@/lib/client-error-logger'
import './index.css'

// Global safety net for errors React's ErrorBoundary can't catch — event
// handlers, async callbacks, and uncaught promise rejections. Axios errors are
// deliberately excluded: they're already logged at the response-interceptor
// boundary (lib/api.ts), so logging them here would double-log AND risk a loop
// with the log POST's own failures (the header guard only covers the interceptor).
window.addEventListener('error', (event) => {
  // Skip resource-load (img/script 404) and cross-origin "Script error." noise.
  if (!(event.error instanceof Error)) return;
  clientErrorLogger.logError({
    subsystem: 'web',
    error_type: 'javascript_error',
    message: event.error.message,
    stack_trace: event.error.stack,
    metadata: { filename: event.filename, lineno: event.lineno, colno: event.colno },
  }).catch(() => {});
});

window.addEventListener('unhandledrejection', (event) => {
  if (axios.isAxiosError(event.reason)) return; // already logged by the api interceptor
  const err = event.reason instanceof Error ? event.reason : new Error(String(event.reason));
  clientErrorLogger.logError({
    subsystem: 'web',
    error_type: 'javascript_error',
    message: err.message,
    stack_trace: err.stack,
  }).catch(() => {});
});

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (failureCount, error: any) => {
        if (error?.response?.status === 401) return false;
        return failureCount < 2;
      },
      staleTime: 0,
      refetchOnWindowFocus: false,
    },
  },
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <ConfirmProvider>
          <App />
        </ConfirmProvider>
      </BrowserRouter>
      <ReactQueryDevtools initialIsOpen={false} />
    </QueryClientProvider>
  </React.StrictMode>,
)