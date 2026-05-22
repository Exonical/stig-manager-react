import { LogIn, ShieldCheck } from 'lucide-react'
import { Navigate, useLocation } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import { useAuth } from '@/lib/auth/auth-context'

interface LocationState {
  from?: string
}

export function SignInPage() {
  const { status, login } = useAuth()
  const location = useLocation()
  const from = (location.state as LocationState | undefined)?.from ?? '/'

  if (status.kind === 'signed-in') {
    return <Navigate to={from} replace />
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-[var(--color-background)] p-6">
      <div className="w-full max-w-md space-y-6 rounded-lg border border-[var(--color-border)] bg-[var(--color-card)] p-8 shadow-sm">
        <div className="flex items-center gap-2">
          <ShieldCheck className="size-7 text-sky-500" />
          <h1 className="text-xl font-semibold tracking-tight">STIG Manager</h1>
        </div>
        <p className="text-sm text-[var(--color-muted-foreground)]">
          Sign in with your single-sign-on account to manage STIG evaluations.
        </p>
        {status.kind === 'unconfigured' && (
          <p
            className="rounded-md border border-[var(--color-destructive)]/40 bg-[var(--color-destructive)]/10 p-3 text-xs text-[var(--color-destructive)]"
            data-testid="sign-in-unconfigured"
          >
            OIDC is not configured on this deployment. Set
            {' '}<code>STIGMAN_OIDC_PROVIDER</code> on the API and reload.
          </p>
        )}
        {status.kind === 'error' && (
          <p
            className="rounded-md border border-[var(--color-destructive)]/40 bg-[var(--color-destructive)]/10 p-3 text-xs text-[var(--color-destructive)]"
            data-testid="sign-in-error"
          >
            {status.message}
          </p>
        )}
        <Button
          size="lg"
          className="w-full"
          disabled={status.kind === 'unconfigured'}
          onClick={() => {
            void login()
          }}
          data-testid="sign-in-cta"
        >
          <LogIn className="size-4" /> Sign in
        </Button>
      </div>
    </div>
  )
}
