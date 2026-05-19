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
}

/**
 * Lists Collections visible to the signed-in user.
 */
export async function fetchCollections(): Promise<CollectionSummary[]> {
  const result = await apiClient.GET('/collections', {})
  if (!result.response.ok || !result.data) {
    throw new Error(`collections: HTTP ${result.response.status}`)
  }
  return result.data as unknown as CollectionSummary[]
}
