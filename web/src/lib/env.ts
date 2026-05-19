// Runtime configuration delivered by the API as /js/Env.js.
//
// The API renders this script as `const STIGMAN = { Env: {...} }` and we
// load it synchronously from index.html so the SPA can read OIDC
// settings at module-load time without any build-time configuration.
// The shape matches upstream STIG Manager so the same env-var-driven
// configuration patterns work.

export interface StigmanEnv {
  version: string
  apiBase: string
  commit?: {
    sha?: string
    branch?: string
    tag?: string
    describe?: string
  }
  oauth: StigmanEnvOAuth
}

export interface StigmanEnvOAuth {
  authority: string
  clientId: string
  extraScopes?: string
  scopePrefix?: string
  responseMode?: string
  audienceValue?: string
  strictPkce?: boolean
  claims?: {
    username?: string
    name?: string
    email?: string
    privileges?: string
    scope?: string
    assertion?: string
  }
}

declare global {
  interface Window {
    STIGMAN?: { Env?: StigmanEnv }
  }
}

/**
 * Default scopes upstream's SPA requests on login. Matches
 * NUWCDIVNPT/stig-manager so existing Keycloak realm imports keep
 * working.
 */
export const DEFAULT_SCOPES = [
  'openid',
  'stig-manager:stig',
  'stig-manager:stig:read',
  'stig-manager:collection',
  'stig-manager:user',
  'stig-manager:user:read',
  'stig-manager:op',
] as const

/**
 * Reads `window.STIGMAN.Env`. Throws if the script tag never loaded —
 * which is a configuration error in index.html, not a runtime
 * recoverable state.
 */
export function readEnv(): StigmanEnv {
  const env = window.STIGMAN?.Env
  if (!env) {
    throw new Error(
      'STIGMAN.Env not found. The SPA expects /js/Env.js to be served by ' +
        'the API and loaded before the React bundle.',
    )
  }
  return env
}

/**
 * Builds the full scope string for an OIDC login by combining the
 * defaults with any deployment-provided extras.
 */
export function buildScopes(env: StigmanEnvOAuth): string {
  const extras = env.extraScopes ? env.extraScopes.split(/\s+/).filter(Boolean) : []
  const all = [...DEFAULT_SCOPES, ...extras]
  if (env.scopePrefix) {
    return all.map((s) => (s === 'openid' ? s : env.scopePrefix + s)).join(' ')
  }
  return all.join(' ')
}
