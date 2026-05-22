// Helpers for inspecting OIDC scope claims.
//
// Operators occasionally deploy with a scope prefix
// (`STIGMAN_CLIENT_SCOPE_PREFIX`) — e.g. `stig-manager:` becomes
// `mycorp/stig-manager:` everywhere. The helpers here normalise the
// scope string so the rest of the SPA can match against the canonical
// upstream scope names regardless of prefix.

import type { User } from 'oidc-client-ts'

import { readEnv } from '@/lib/env'

export type ScopeName =
  | 'stig-manager:stig'
  | 'stig-manager:stig:read'
  | 'stig-manager:collection'
  | 'stig-manager:collection:read'
  | 'stig-manager:user'
  | 'stig-manager:user:read'
  | 'stig-manager:op'
  | 'stig-manager:op:read'

function stripPrefix(scope: string, prefix: string | undefined): string {
  if (!prefix) return scope
  return scope.startsWith(prefix) ? scope.slice(prefix.length) : scope
}

/**
 * Returns the set of canonical scope names the signed-in user has.
 */
export function userScopes(user: User | null | undefined): Set<string> {
  if (!user) return new Set()
  const raw = (user.profile as Record<string, unknown>)['scope']
  if (typeof raw !== 'string') return new Set()
  let prefix: string | undefined
  try {
    prefix = readEnv().oauth.scopePrefix
  } catch {
    prefix = undefined
  }
  const parts = raw.split(/\s+/).filter(Boolean)
  return new Set(parts.map((s) => stripPrefix(s, prefix)))
}

/**
 * Best-effort display name for the signed-in user. Falls back to
 * configured claims, then to `preferred_username`, then to `sub`.
 */
export function userDisplayName(user: User | null | undefined): string {
  if (!user) return ''
  const profile = user.profile as Record<string, unknown>
  const candidates = [
    profile['name'],
    profile['preferred_username'],
    profile['email'],
    profile['sub'],
  ]
  for (const candidate of candidates) {
    if (typeof candidate === 'string' && candidate.length > 0) return candidate
  }
  return ''
}

/**
 * True when the user holds the given canonical scope name.
 */
export function hasScope(user: User | null | undefined, scope: ScopeName): boolean {
  return userScopes(user).has(scope)
}
