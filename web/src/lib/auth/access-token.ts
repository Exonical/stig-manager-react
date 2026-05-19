// Module-level access-token getter used by the openapi-fetch
// middleware. The AuthProvider injects its `getAccessToken` callback
// via `setAccessTokenGetter` on mount; the middleware reads it via
// `getAccessTokenForClient` on every request. Decoupled from the React
// context to keep the openapi-fetch middleware non-React.

let accessTokenGetter: () => string | undefined = () => undefined

export function setAccessTokenGetter(fn: () => string | undefined) {
  accessTokenGetter = fn
}

export function getAccessTokenForClient(): string | undefined {
  return accessTokenGetter()
}
