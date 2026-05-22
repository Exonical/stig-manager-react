// Inner route guard: when present on a route, requires the signed-in
// user to also hold one of the configured OIDC scopes. Routes that
// would 401 from the API should declare their scope here so we can
// render a clearer 'unauthorized' page instead of letting the user
// click through to a failing request.


import { useAuth } from '@/lib/auth/auth-context'
import { hasScope, type ScopeName } from '@/lib/auth/scopes'

interface RequireScopeProps {
  scope: ScopeName | ScopeName[]
  children: React.ReactNode
}

export function RequireScope({ scope, children }: RequireScopeProps) {
  const { status } = useAuth()
  const user = status.kind === 'signed-in' ? status.user : null
  const scopes = Array.isArray(scope) ? scope : [scope]
  const ok = scopes.some((s) => hasScope(user, s))
  if (!ok) {
    return (
      <div className="mx-auto max-w-2xl px-6 py-16 text-center">
        <h1 className="text-2xl font-semibold">Insufficient permissions</h1>
        <p className="mt-3 text-sm text-[var(--color-muted-foreground)]">
          Your account is missing the required OIDC scope
          {scopes.length > 1 ? 's' : ''}:{' '}
          <code>{scopes.join(' ')}</code>. Ask an administrator to grant access.
        </p>
      </div>
    )
  }
  return <>{children}</>
}
