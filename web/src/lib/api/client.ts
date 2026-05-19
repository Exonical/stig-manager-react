// Typed HTTP client for the STIG Manager API.
//
// The schema in ./schema.ts is regenerated from docs/openapi/stig-manager.yaml
// via `pnpm gen:api`. openapi-fetch wraps fetch() with full type-checking on
// paths, parameters, request bodies, and responses — see
// https://openapi-ts.dev/openapi-fetch/.

import createClient from 'openapi-fetch'

import type { paths } from './schema'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api'

export const apiClient = createClient<paths>({
  baseUrl: API_BASE_URL,
  credentials: 'include',
})

export type { paths, components, operations } from './schema'
