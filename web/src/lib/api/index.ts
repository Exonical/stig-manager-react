// Public surface of the typed API client.
//
// Higher-level wrappers can live here; for now we re-export the generated
// schema types and the openapi-fetch instance configured in ./client.
export { apiClient } from './client'
export type { paths, components, operations } from './client'

import { apiClient } from './client'

/**
 * Minimal AppInfo shape returned by the scaffold backend.
 *
 * Upstream's AppInfo schema is much richer (collections, request counters,
 * users, groups, etc.) — see components.schemas.AppInfo in
 * docs/openapi/stig-manager.yaml. APIServer.GetAppInfo currently returns
 * only build metadata; later milestones will return the full upstream
 * shape and this type will be replaced with components['schemas']['AppInfo'].
 */
export type AppInfo = {
  version: string
  commit?: string
  buildDate?: string
}

/**
 * Fetches build metadata from `GET /op/appinfo`.
 */
export async function fetchAppInfo(): Promise<AppInfo> {
  const result = await apiClient.GET('/op/appinfo', {})
  if (!result.response.ok || !result.data) {
    throw new Error(`appinfo: HTTP ${result.response.status}`)
  }
  return result.data as unknown as AppInfo
}

/**
 * Collection summary returned by `GET /collections`. Mirrors the
 * Collection schema in docs/openapi/stig-manager.yaml.
 */
export type CollectionSummary = {
  collectionId: string
  name: string
  description?: string | null
  metadata?: Record<string, string>
  created?: string
  settings?: unknown
}

/**
 * Lists Collections visible to the signed-in user. Supports a server
 * side `name` substring filter — the API matches `name-match=contains`
 * by default.
 */
export async function fetchCollections(params?: {
  name?: string
}): Promise<CollectionSummary[]> {
  const query: Record<string, string> = {}
  if (params?.name) query['name'] = params.name
  const result = await apiClient.GET('/collections', {
    params: { query },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`collections: HTTP ${result.response.status}`)
  }
  return result.data as unknown as CollectionSummary[]
}

/**
 * Fetches a single Collection. Returns the full Collection shape
 * including `settings` + `metadata`.
 */
export async function fetchCollection(
  collectionId: string,
): Promise<CollectionSummary> {
  const result = await apiClient.GET('/collections/{collectionId}', {
    params: { path: { collectionId } },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`collection ${collectionId}: HTTP ${result.response.status}`)
  }
  return result.data as unknown as CollectionSummary
}

/**
 * Information about the authenticated user. Returned by `GET /user`.
 */
export type CurrentUser = {
  userId: string
  username: string
  displayName?: string
  email?: string | null
  privileges?: {
    admin?: boolean
    createCollection?: boolean
  }
  collectionGrants?: Array<{
    roleId?: number
    collection?: { collectionId?: string; name?: string }
  }>
}

/**
 * Fetches the authenticated user's app_user record. The API upserts
 * the row on first call so first-time visitors get a stable userId
 * immediately.
 */
export async function fetchCurrentUser(): Promise<CurrentUser> {
  const result = await apiClient.GET('/user', {})
  if (!result.response.ok || !result.data) {
    throw new Error(`user: HTTP ${result.response.status}`)
  }
  return result.data as unknown as CurrentUser
}

/**
 * Payload for `POST /collections`. Mirrors CollectionCreateOrReplace.
 */
export type CreateCollectionInput = {
  name: string
  description?: string
  metadata?: Record<string, string>
  grants: Array<{ userId: string; roleId: 1 | 2 | 3 | 4 }>
}

/**
 * Creates a Collection. Returns the newly-created Collection record.
 */
export async function createCollection(
  input: CreateCollectionInput,
): Promise<CollectionSummary> {
  // openapi-fetch type-checks the body against the generated schema,
  // which is stricter than the API actually accepts (the schema's
  // grants union types don't widen cleanly to ours). Bypass with an
  // unsafe cast: the runtime API validates the JSON shape itself.
  const result = await apiClient.POST('/collections', {
    body: input as never,
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`create collection: HTTP ${result.response.status}`)
  }
  return result.data as unknown as CollectionSummary
}
