import { LogIn, LogOut, ShieldCheck } from 'lucide-react'
import * as React from 'react'

import { ThemeToggle } from '@/components/theme-toggle'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { fetchAppInfo, type AppInfo } from '@/lib/api'
import { useAuth } from '@/lib/auth/auth-context'

type AppInfoState =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'ok'; data: AppInfo }
  | { status: 'error'; message: string }

function App() {
  const [appInfo, setAppInfo] = React.useState<AppInfoState>({ status: 'idle' })

  async function loadAppInfo() {
    setAppInfo({ status: 'loading' })
    try {
      const data = await fetchAppInfo()
      setAppInfo({ status: 'ok', data })
    } catch (err) {
      setAppInfo({
        status: 'error',
        message: err instanceof Error ? err.message : 'unknown error',
      })
    }
  }

  return (
    <div className="min-h-screen">
      <header className="border-b">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
          <div className="flex items-center gap-2">
            <ShieldCheck className="size-6 text-sky-500" />
            <span className="text-lg font-semibold tracking-tight">
              STIG Manager
            </span>
          </div>
          <div className="flex items-center gap-2">
            <AuthHeader />
            <ThemeToggle />
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-6 py-16">
        <section className="space-y-4 pb-12">
          <h1 className="text-4xl font-bold tracking-tight">Milestone 3</h1>
          <p className="max-w-2xl text-[var(--color-muted-foreground)]">
            OIDC PKCE auth is wired end-to-end: the SPA reads OIDC settings
            from <code>/js/Env.js</code>, completes a PKCE login against the
            configured provider, and attaches the access token to API
            requests. The Go API validates tokens against the issuer's
            JWKS and enforces per-route scopes.
          </p>
        </section>

        <section className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          <Card>
            <CardHeader>
              <CardTitle>Frontend</CardTitle>
              <CardDescription>
                Vite, React 19, TypeScript (strict), Tailwind v4, shadcn/ui,
                oidc-client-ts.
              </CardDescription>
            </CardHeader>
            <CardContent className="text-sm text-[var(--color-muted-foreground)]">
              <code>pnpm --filter web dev</code> on port 54000.
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Backend</CardTitle>
              <CardDescription>
                Go 1.26, chi router, JWKS-backed JWT validator, Postgres 18
                via pgx.
              </CardDescription>
            </CardHeader>
            <CardContent className="text-sm text-[var(--color-muted-foreground)]">
              <code>go run ./api/cmd/stigman</code> on port 54001.
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>API connectivity</CardTitle>
              <CardDescription>
                Calls <code>GET /api/op/appinfo</code> (requires the
                <code> stig-manager:op:read</code> scope).
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <Button
                onClick={loadAppInfo}
                disabled={appInfo.status === 'loading'}
              >
                {appInfo.status === 'loading' ? 'Loading…' : 'Fetch app info'}
              </Button>
              {appInfo.status === 'ok' && (
                <pre className="rounded-md bg-[var(--color-secondary)] p-3 text-xs">
                  {JSON.stringify(appInfo.data, null, 2)}
                </pre>
              )}
              {appInfo.status === 'error' && (
                <p className="text-sm text-[var(--color-destructive)]">
                  {appInfo.message}
                </p>
              )}
            </CardContent>
          </Card>
        </section>
      </main>
    </div>
  )
}

function AuthHeader() {
  const { status, login, logout } = useAuth()
  switch (status.kind) {
    case 'initialising':
      return (
        <span className="text-sm text-[var(--color-muted-foreground)]">
          Loading…
        </span>
      )
    case 'unconfigured':
      return (
        <span
          className="text-sm text-[var(--color-muted-foreground)]"
          title={status.reason}
        >
          Auth not configured
        </span>
      )
    case 'signed-out':
      return (
        <Button variant="outline" size="sm" onClick={() => void login()}>
          <LogIn className="mr-2 size-4" />
          Sign in
        </Button>
      )
    case 'signed-in': {
      const claims = status.user.profile as Record<string, unknown>
      const name =
        (claims.preferred_username as string | undefined) ??
        (claims.name as string | undefined) ??
        status.user.profile.sub
      return (
        <div className="flex items-center gap-3">
          <span className="text-sm">
            Signed in as <strong>{name}</strong>
          </span>
          <Button variant="outline" size="sm" onClick={() => void logout()}>
            <LogOut className="mr-2 size-4" />
            Sign out
          </Button>
        </div>
      )
    }
    case 'error':
      return (
        <span className="text-sm text-[var(--color-destructive)]">
          Auth: {status.message}
        </span>
      )
  }
}

export default App
