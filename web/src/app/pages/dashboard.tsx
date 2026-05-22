import { Activity, ServerCog } from 'lucide-react'
import { Link } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useAuth } from '@/lib/auth/auth-context'
import { hasScope, userDisplayName } from '@/lib/auth/scopes'
import { useAppInfo, useCollections, useOpState } from '@/lib/api/hooks'

export function DashboardPage() {
  const { status } = useAuth()
  const user = status.kind === 'signed-in' ? status.user : null

  return (
    <div className="space-y-8">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight">
          Welcome
          {userDisplayName(user) ? `, ${userDisplayName(user)}` : ''}
        </h1>
        <p className="mt-2 text-sm text-[var(--color-muted-foreground)]">
          Pick a Collection to start reviewing, or use the navigation to reach
          the STIG Library, Jobs, or Admin surfaces.
        </p>
      </header>

      <section className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {hasScope(user, 'stig-manager:collection:read') && <CollectionsTile />}
        {hasScope(user, 'stig-manager:op:read') && <OpStateTile />}
        {hasScope(user, 'stig-manager:op:read') && <AppInfoTile />}
      </section>
    </div>
  )
}

function CollectionsTile() {
  const collections = useCollections()
  const count = collections.data?.length ?? 0
  return (
    <Card data-testid="dashboard-collections-tile">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Activity className="size-4 text-sky-500" /> Collections
        </CardTitle>
        <CardDescription>
          {collections.isLoading
            ? 'Loading…'
            : collections.isError
              ? collections.error.message
              : count === 1
                ? '1 collection visible to you.'
                : `${count} collections visible to you.`}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex justify-end">
        <Button asChild size="sm" variant="outline">
          <Link to="/collections" data-testid="dashboard-go-collections">
            Open
          </Link>
        </Button>
      </CardContent>
    </Card>
  )
}

function OpStateTile() {
  const opState = useOpState()
  const deps = opState.data?.dependencies ?? {}
  return (
    <Card data-testid="dashboard-op-state-tile">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ServerCog className="size-4 text-emerald-500" /> System state
        </CardTitle>
        <CardDescription>
          {opState.isLoading
            ? 'Loading…'
            : opState.isError
              ? opState.error.message
              : opState.data
                ? `Service is ${opState.data.status}.`
                : 'unknown'}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <ul className="space-y-1 text-sm">
          {Object.entries(deps).map(([name, dep]) => (
            <li key={name} className="flex items-center justify-between">
              <span className="text-[var(--color-muted-foreground)]">{name}</span>
              <span
                className={
                  dep.status === 'up'
                    ? 'text-emerald-500'
                    : 'text-[var(--color-destructive)]'
                }
              >
                {dep.status}
              </span>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}

function AppInfoTile() {
  const appInfo = useAppInfo()
  return (
    <Card data-testid="dashboard-appinfo-tile">
      <CardHeader>
        <CardTitle>About</CardTitle>
        <CardDescription>
          {appInfo.isLoading
            ? 'Loading…'
            : appInfo.isError
              ? appInfo.error.message
              : appInfo.data
                ? `API ${appInfo.data.version ?? 'dev'}`
                : ''}
        </CardDescription>
      </CardHeader>
      <CardContent className="text-xs text-[var(--color-muted-foreground)]">
        {appInfo.data?.buildDate ? `Built ${appInfo.data.buildDate}` : null}
        {appInfo.data?.commit ? ` · ${appInfo.data.commit.slice(0, 8)}` : null}
      </CardContent>
    </Card>
  )
}
