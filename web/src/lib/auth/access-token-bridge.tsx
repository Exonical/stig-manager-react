import * as React from 'react'

import { setAccessTokenGetter } from './access-token'
import { useAuth } from './auth-context'

/**
 * Registers the AuthContext's `getAccessToken` callback into the
 * module-level slot read by the openapi-fetch middleware. Rendered
 * inside AuthProvider so the live `getAccessToken` closure (which sees
 * silent-renew updates) is always the one in use.
 */
export function AccessTokenBridge() {
  const { getAccessToken } = useAuth()
  React.useEffect(() => {
    setAccessTokenGetter(getAccessToken)
    return () => setAccessTokenGetter(() => undefined)
  }, [getAccessToken])
  return null
}
