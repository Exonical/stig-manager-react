import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './styles/globals.css'
import App from './App.tsx'
import { AuthProvider } from './lib/auth/auth-context'
import { AccessTokenBridge } from './lib/auth/access-token-bridge'

const root = document.getElementById('root')
if (!root) throw new Error('Root element #root not found')

createRoot(root).render(
  <StrictMode>
    <AuthProvider>
      <AccessTokenBridge />
      <App />
    </AuthProvider>
  </StrictMode>,
)
