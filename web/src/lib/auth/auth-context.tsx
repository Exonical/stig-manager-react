import * as React from 'react'
import type { User } from 'oidc-client-ts'

import { getUserManager, isAuthCallback } from './user-manager'

export type AuthStatus =
  | { kind: 'initialising' }
  | { kind: 'unconfigured'; reason: string }
  | { kind: 'signed-out' }
  | { kind: 'signed-in'; user: User }
  | { kind: 'error'; message: string }

interface AuthContextValue {
  status: AuthStatus
  login: () => Promise<void>
  logout: () => Promise<void>
  getAccessToken: () => string | undefined
}

const AuthContext = React.createContext<AuthContextValue | null>(null)

export function useAuth(): AuthContextValue {
  const ctx = React.useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within <AuthProvider>')
  return ctx
}

interface AuthProviderProps {
  children: React.ReactNode
}

/**
 * AuthProvider bootstraps oidc-client-ts: it restores any persisted
 * user, completes the redirect callback when present, and subscribes to
 * silent-renew events so the access token attached to API requests
 * stays fresh. When `window.STIGMAN.Env.oauth.authority` is empty the
 * provider yields a stable `unconfigured` status — the rest of the SPA
 * can still render and the API will surface the lack of auth as 401s
 * on protected calls.
 */
export function AuthProvider({ children }: AuthProviderProps) {
  const [status, setStatus] = React.useState<AuthStatus>({ kind: 'initialising' })
  const userRef = React.useRef<User | null>(null)

  React.useEffect(() => {
    let cancelled = false
    let manager: ReturnType<typeof getUserManager>
    try {
      manager = getUserManager()
    } catch (err) {
      const message = err instanceof Error ? err.message : 'unknown error'
      setStatus({ kind: 'unconfigured', reason: message })
      return
    }

    async function bootstrap() {
      try {
        if (isAuthCallback()) {
          const user = await manager.signinCallback()
          if (user) {
            userRef.current = user
            // Strip the OAuth response params from the URL.
            const clean = window.location.pathname
            window.history.replaceState({}, '', clean)
            if (!cancelled) setStatus({ kind: 'signed-in', user })
            return
          }
        }
        const user = await manager.getUser()
        if (cancelled) return
        if (user && !user.expired) {
          userRef.current = user
          setStatus({ kind: 'signed-in', user })
        } else {
          userRef.current = null
          setStatus({ kind: 'signed-out' })
        }
      } catch (err) {
        if (cancelled) return
        setStatus({
          kind: 'error',
          message: err instanceof Error ? err.message : 'auth bootstrap failed',
        })
      }
    }

    void bootstrap()

    const onUserLoaded = (user: User) => {
      userRef.current = user
      setStatus({ kind: 'signed-in', user })
    }
    const onUserUnloaded = () => {
      userRef.current = null
      setStatus({ kind: 'signed-out' })
    }
    const onSilentRenewError = (err: Error) => {
      setStatus({ kind: 'error', message: err.message })
    }

    manager.events.addUserLoaded(onUserLoaded)
    manager.events.addUserUnloaded(onUserUnloaded)
    manager.events.addSilentRenewError(onSilentRenewError)

    return () => {
      cancelled = true
      manager.events.removeUserLoaded(onUserLoaded)
      manager.events.removeUserUnloaded(onUserUnloaded)
      manager.events.removeSilentRenewError(onSilentRenewError)
    }
  }, [])

  const login = React.useCallback(async () => {
    const manager = getUserManager()
    await manager.signinRedirect()
  }, [])

  const logout = React.useCallback(async () => {
    const manager = getUserManager()
    try {
      await manager.signoutRedirect()
    } catch {
      await manager.removeUser()
      userRef.current = null
      setStatus({ kind: 'signed-out' })
    }
  }, [])

  const getAccessToken = React.useCallback((): string | undefined => {
    return userRef.current?.access_token
  }, [])

  const value: AuthContextValue = React.useMemo(
    () => ({ status, login, logout, getAccessToken }),
    [status, login, logout, getAccessToken],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}


