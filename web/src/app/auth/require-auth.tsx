// Route guard. Renders children only when the user is signed in;
// otherwise dispatches to a sign-in landing page that lets the user
// trigger an OIDC redirect themselves rather than auto-redirecting
// (auto-redirect-on-load made it impossible to read error messages
// during setup, and broke deep links).

import { Navigate, useLocation } from 'react-router-dom'

import { useAuth } from '@/lib/auth/auth-context'

interface RequireAuthProps {
  children: React.ReactNode
}

export function RequireAuth({ children }: RequireAuthProps) {
  const { status } = useAuth()
  const location = useLocation()

  if (status.kind === 'initialising') {
    return <FullPageStatus tone="muted">Loading…</FullPageStatus>
  }
  if (status.kind === 'signed-in') {
    return <>{children}</>
  }
  // Capture the originally-requested URL so SignIn can return there.
  return (
    <Navigate
      to="/sign-in"
      replace
      state={{ from: location.pathname + location.search }}
    />
  )
}

interface FullPageStatusProps {
  children: React.ReactNode
  tone?: 'muted' | 'error'
}

function FullPageStatus({ children, tone = 'muted' }: FullPageStatusProps) {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <p
        className={
          tone === 'error'
            ? 'text-sm text-[var(--color-destructive)]'
            : 'text-sm text-[var(--color-muted-foreground)]'
        }
      >
        {children}
      </p>
    </div>
  )
}
