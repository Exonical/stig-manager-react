import { LogIn, LogOut } from 'lucide-react'

import { ThemeToggle } from '@/components/theme-toggle'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/lib/auth/auth-context'
import { userDisplayName } from '@/lib/auth/scopes'

export function Topbar() {
  const { status, login, logout } = useAuth()
  return (
    <header className="flex h-14 items-center justify-between border-b border-[var(--color-border)] bg-[var(--color-background)] px-6">
      <div className="flex items-center gap-3 md:hidden">
        <span className="text-sm font-semibold">STIG Manager</span>
      </div>
      <div className="ml-auto flex items-center gap-3">
        {status.kind === 'signed-in' ? (
          <>
            <span className="text-sm text-[var(--color-muted-foreground)]">
              Signed in as{' '}
              <strong className="font-medium text-[var(--color-foreground)]">
                {userDisplayName(status.user) || 'user'}
              </strong>
            </span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                void logout()
              }}
              data-testid="sign-out-button"
            >
              <LogOut className="size-4" /> Sign out
            </Button>
          </>
        ) : status.kind === 'signed-out' || status.kind === 'error' ? (
          <Button
            size="sm"
            onClick={() => {
              void login()
            }}
            data-testid="sign-in-button"
          >
            <LogIn className="size-4" /> Sign in
          </Button>
        ) : status.kind === 'unconfigured' ? (
          <span
            className="text-xs text-[var(--color-destructive)]"
            title={status.reason}
          >
            OIDC unconfigured
          </span>
        ) : null}
        <ThemeToggle />
      </div>
    </header>
  )
}
