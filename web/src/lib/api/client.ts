// Typed HTTP client for the STIG Manager API.
//
// The schema in ./schema.ts is regenerated from docs/openapi/stig-manager.yaml
// via `pnpm gen:api`. openapi-fetch wraps fetch() with full type-checking on
// paths, parameters, request bodies, and responses — see
// https://openapi-ts.dev/openapi-fetch/.

import createClient, { type Middleware } from 'openapi-fetch'

import { getAccessTokenForClient } from '../auth/access-token'

import type { paths } from './schema'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api'

const bearerMiddleware: Middleware = {
  async onRequest({ request }) {
    const token = getAccessTokenForClient()
    if (token) {
      request.headers.set('Authorization', `Bearer ${token}`)
    }
    return request
  },
}

export const apiClient = createClient<paths>({
  baseUrl: API_BASE_URL,
  credentials: 'include',
})

apiClient.use(bearerMiddleware)

export type { paths, components, operations } from './schema'
