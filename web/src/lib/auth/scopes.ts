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
 *
 * OAuth/OIDC carries the granted scopes in three different places and
 * Keycloak's defaults put them in only one of them, so we check all
 * three in order of decreasing canonicalness:
 *
 *  1. `User.scope` — the literal `scope` field from the token
 *     response, exposed by oidc-client-ts. This is the canonical
 *     source per RFC 6749 §5.1.
 *  2. The access token's `scope` claim — Keycloak puts the granted
 *     scopes here even when the ID token doesn't carry them.
 *  3. The ID token's `scope` claim — falls back here when an
 *     authorization server happens to also project scope into the
 *     ID token (some do; Keycloak does not by default).
 */
export function userScopes(user: User | null | undefined): Set<string> {
  if (!user) return new Set()
  let raw: string | undefined
  if (typeof user.scope === 'string' && user.scope.length > 0) {
    raw = user.scope
  } else if (typeof user.access_token === 'string') {
    raw = scopeFromAccessToken(user.access_token)
  }
  if (!raw) {
    const profile = (user.profile as Record<string, unknown>)['scope']
    if (typeof profile === 'string') raw = profile
  }
  if (!raw) return new Set()
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
 * Best-effort decode of the OAuth `scope` claim out of a JWT access
 * token. Returns undefined for opaque tokens or anything else that
 * isn't a well-formed JWS.
 */
function scopeFromAccessToken(token: string): string | undefined {
  const parts = token.split('.')
  if (parts.length < 2) return undefined
  try {
    let payload = parts[1].replace(/-/g, '+').replace(/_/g, '/')
    while (payload.length % 4 !== 0) payload += '='
    const decoded = JSON.parse(atob(payload)) as Record<string, unknown>
    const scope = decoded['scope']
    return typeof scope === 'string' ? scope : undefined
  } catch {
    return undefined
  }
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
