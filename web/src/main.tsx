import { QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from 'react-router-dom'

import './styles/globals.css'
import { queryClient } from './app/query-client'
import { router } from './app/router'
import { AccessTokenBridge } from './lib/auth/access-token-bridge'
import { AuthProvider } from './lib/auth/auth-context'

const root = document.getElementById('root')
if (!root) throw new Error('Root element #root not found')

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <AccessTokenBridge />
        <RouterProvider router={router} />
      </AuthProvider>
    </QueryClientProvider>
  </StrictMode>,
)
