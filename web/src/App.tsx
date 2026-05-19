import { ShieldCheck } from 'lucide-react'
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
          <ThemeToggle />
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-6 py-16">
        <section className="space-y-4 pb-12">
          <h1 className="text-4xl font-bold tracking-tight">Milestone 1</h1>
          <p className="max-w-2xl text-[var(--color-muted-foreground)]">
            React 19 + shadcn/ui SPA and a Go API skeleton, wired together for
            local development. The full feature set lands in Milestones 2–16.
          </p>
        </section>

        <section className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          <Card>
            <CardHeader>
              <CardTitle>Frontend</CardTitle>
              <CardDescription>
                Vite, React 19, TypeScript (strict), Tailwind v4, shadcn/ui.
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
                Go 1.23, chi router, OpenAPI v1 surface. Postgres 18 via pgx.
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
                Calls <code>GET /api/op/appinfo</code> on the Go backend
                through the Vite dev proxy.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <Button onClick={loadAppInfo} disabled={appInfo.status === 'loading'}>
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

export default App
