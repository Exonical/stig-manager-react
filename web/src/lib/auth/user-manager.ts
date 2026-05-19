import { UserManager, WebStorageStateStore, type UserManagerSettings } from 'oidc-client-ts'

import { buildScopes, readEnv } from '../env'

let cached: UserManager | null = null

/**
 * Returns the singleton oidc-client-ts UserManager, lazily constructed
 * from `window.STIGMAN.Env`. Calling this in a non-OIDC environment
 * (env.oauth.authority empty) throws synchronously — wrap the call in
 * the AuthProvider's status check.
 */
export function getUserManager(): UserManager {
  if (cached) return cached
  const env = readEnv()
  if (!env.oauth.authority) {
    throw new Error(
      'OIDC authority is empty. Set STIGMAN_OIDC_PROVIDER or ' +
        'STIGMAN_CLIENT_OIDC_PROVIDER on the API container so /js/Env.js ' +
        'reports a non-empty oauth.authority.',
    )
  }
  const settings: UserManagerSettings = {
    authority: env.oauth.authority,
    client_id: env.oauth.clientId,
    redirect_uri: window.location.origin + '/',
    post_logout_redirect_uri: window.location.origin + '/',
    response_type: 'code',
    response_mode: (env.oauth.responseMode as 'fragment' | 'query') ?? 'fragment',
    scope: buildScopes(env.oauth),
    automaticSilentRenew: true,
    loadUserInfo: false,
    userStore: new WebStorageStateStore({ store: window.localStorage }),
    // PKCE is implied by response_type=code.
    extraQueryParams: env.oauth.audienceValue
      ? { audience: env.oauth.audienceValue }
      : undefined,
  }
  cached = new UserManager(settings)
  return cached
}

/**
 * Returns true when the current URL contains the OAuth `code` + `state`
 * pair the provider redirects back with after a successful login. The
 * AuthProvider runs `signinCallback()` exactly once in this case.
 */
export function isAuthCallback(): boolean {
  const params = new URLSearchParams(
    window.location.search || window.location.hash.replace(/^#/, ''),
  )
  return params.has('code') && params.has('state')
}
