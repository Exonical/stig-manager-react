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

/**
 * Asset record returned by GET /assets and GET /assets/{assetId}.
 * Mirrors the upstream `AssetProjected` schema (mostly) — extra
 * projections like `statusStats` and `stigs` are available when
 * `projection` is provided as a query parameter.
 */
export type Asset = {
  assetId: string
  collection?: { collectionId?: string; name?: string }
  description?: string | null
  fqdn?: string | null
  ip?: string | null
  mac?: string | null
  metadata?: Record<string, string>
  name: string
  noncomputing?: boolean
  labelIds?: string[]
  labels?: Array<{ labelId?: string; name?: string; color?: string | null }>
  stigs?: AssetStig[]
}

export type AssetStig = {
  benchmarkId: string
  revisionStr?: string
  benchmarkDate?: string | null
  revisionPinned?: boolean
  ruleCount?: number | null
}

/**
 * Lists Assets visible to the signed-in user. `collectionId` is
 * effectively required for the SPA to scope the call to a single
 * Collection.
 */
export async function fetchAssets(params: {
  collectionId: string
  name?: string
  labelId?: string
}): Promise<Asset[]> {
  const query: Record<string, string | string[]> = {
    collectionId: params.collectionId,
    projection: 'stigs',
  }
  if (params.name) query['name'] = params.name
  if (params.labelId) query['labelId'] = [params.labelId]
  const result = await apiClient.GET('/assets', {
    params: { query: query as never },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`assets: HTTP ${result.response.status}`)
  }
  return result.data as unknown as Asset[]
}

/**
 * Fetches a single Asset by id (with stigs projection by default).
 */
export async function fetchAsset(assetId: string): Promise<Asset> {
  const result = await apiClient.GET('/assets/{assetId}', {
    params: {
      path: { assetId },
      query: { projection: ['stigs'] } as never,
    },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`asset ${assetId}: HTTP ${result.response.status}`)
  }
  return result.data as unknown as Asset
}

/**
 * Payload for `POST /assets`. Mirrors `AssetCreateOrReplace`. Assets
 * are scoped to a single Collection; `stigs` is the list of mapped
 * benchmark ids.
 */
export type AssetForm = {
  collectionId: string
  name: string
  description: string | null
  fqdn?: string | null
  ip: string | null
  mac?: string | null
  noncomputing: boolean
  metadata?: Record<string, string>
  stigs: string[]
  labelNames?: string[]
}

export async function createAsset(input: AssetForm): Promise<Asset> {
  const result = await apiClient.POST('/assets', {
    body: input as never,
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`create asset: HTTP ${result.response.status}`)
  }
  return result.data as unknown as Asset
}

export type AssetUpdateInput = Partial<AssetForm> & { collectionId?: string }

export async function updateAsset(
  assetId: string,
  input: AssetUpdateInput,
): Promise<Asset> {
  const result = await apiClient.PATCH('/assets/{assetId}', {
    params: { path: { assetId } },
    body: input as never,
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`update asset: HTTP ${result.response.status}`)
  }
  return result.data as unknown as Asset
}

export async function deleteAsset(assetId: string): Promise<Asset> {
  const result = await apiClient.DELETE('/assets/{assetId}', {
    params: { path: { assetId } },
  })
  if (!result.response.ok) {
    throw new Error(`delete asset: HTTP ${result.response.status}`)
  }
  return result.data as unknown as Asset
}

/**
 * STIGs currently mapped onto an Asset (with effective revision).
 */
export async function fetchAssetStigs(assetId: string): Promise<AssetStig[]> {
  const result = await apiClient.GET('/assets/{assetId}/stigs', {
    params: { path: { assetId } },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`asset stigs: HTTP ${result.response.status}`)
  }
  return result.data as unknown as AssetStig[]
}

/**
 * Rule record exposed by `GET /stigs/{benchmarkId}/revisions/{revisionStr}/rules`.
 */
export type Rule = {
  ruleId: string
  version: string
  title: string
  severity: string
  groupId?: string
  groupTitle?: string
}

export async function fetchRulesByRevision(
  benchmarkId: string,
  revisionStr: string,
): Promise<Rule[]> {
  const result = await apiClient.GET(
    '/stigs/{benchmarkId}/revisions/{revisionStr}/rules',
    { params: { path: { benchmarkId, revisionStr } } },
  )
  if (!result.response.ok || !result.data) {
    throw new Error(`rules: HTTP ${result.response.status}`)
  }
  return result.data as unknown as Rule[]
}

/**
 * Review row as returned by `GET /collections/{cid}/reviews/{aid}` and
 * `GET /collections/{cid}/reviews/{aid}/{ruleId}` (latter returns one
 * full record or 204).
 */
export type Review = {
  ruleId?: string
  ruleIds?: string[]
  result: ReviewResult
  detail: string
  comment: string
  status?: { label?: ReviewStatusLabel; text?: string | null; ts?: string; user?: { userId?: string; username?: string } }
  ts?: string
  touchTs?: string
  userId?: string
  username?: string
  rule?: { ruleId?: string; version?: string; title?: string; severity?: string }
}

export type ReviewResult =
  | 'fail'
  | 'pass'
  | 'notapplicable'
  | 'notchecked'
  | 'unknown'
  | 'error'
  | 'notselected'
  | 'informational'
  | 'fixed'

export type ReviewStatusLabel = 'saved' | 'submitted' | 'accepted' | 'rejected'

export async function fetchReviewsByAsset(
  collectionId: string,
  assetId: string,
): Promise<Review[]> {
  const result = await apiClient.GET(
    '/collections/{collectionId}/reviews/{assetId}',
    { params: { path: { collectionId, assetId } } },
  )
  if (!result.response.ok) {
    throw new Error(`reviews: HTTP ${result.response.status}`)
  }
  return (result.data as unknown as Review[] | undefined) ?? []
}

export async function fetchReviewByAssetRule(
  collectionId: string,
  assetId: string,
  ruleId: string,
): Promise<Review | null> {
  const result = await apiClient.GET(
    '/collections/{collectionId}/reviews/{assetId}/{ruleId}',
    { params: { path: { collectionId, assetId, ruleId } } },
  )
  // 204 No Content means there is no review yet for this rule.
  if (result.response.status === 204) return null
  if (!result.response.ok) {
    throw new Error(`review: HTTP ${result.response.status}`)
  }
  return (result.data as unknown as Review | undefined) ?? null
}

export type ReviewPutInput = {
  result: ReviewResult
  detail: string
  comment: string
  status?: ReviewStatusLabel | { label: ReviewStatusLabel; text?: string | null }
}

export async function putReviewByAssetRule(
  collectionId: string,
  assetId: string,
  ruleId: string,
  body: ReviewPutInput,
): Promise<Review> {
  const result = await apiClient.PUT(
    '/collections/{collectionId}/reviews/{assetId}/{ruleId}',
    {
      params: { path: { collectionId, assetId, ruleId } },
      body: body as never,
    },
  )
  if (!result.response.ok || !result.data) {
    throw new Error(`put review: HTTP ${result.response.status}`)
  }
  return result.data as unknown as Review
}

/**
 * Lists STIGs mapped in a Collection (with asset counts).
 */
export type CollectionStig = {
  benchmarkId: string
  revisionStr?: string
  benchmarkDate?: string | null
  revisionPinned?: boolean
  ruleCount?: number | null
  assetCount?: number | null
  title?: string
}

export async function fetchCollectionStigs(
  collectionId: string,
): Promise<CollectionStig[]> {
  const result = await apiClient.GET('/collections/{collectionId}/stigs', {
    params: { path: { collectionId } },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`collection stigs: HTTP ${result.response.status}`)
  }
  return result.data as unknown as CollectionStig[]
}

/**
 * Cross-product bulk-review mutation: applies a single source review
 * to every (asset, rule) pair resolved from the supplied criteria.
 * Mirrors the `ReviewBatch` schema in docs/openapi/stig-manager.yaml.
 *
 * `assets` is exclusive-or between asset IDs and benchmark IDs; same
 * for `rules`. The server enforces the same xor with a 400. `action`
 * defaults to `merge` when omitted; we surface it explicitly here so
 * the UI doesn't accidentally rely on the server default. `dryRun`
 * is required by the schema (default false) so we always send a
 * concrete value.
 */
export type ReviewBatchInput = {
  assets: { assetIds: string[] } | { benchmarkIds: string[] }
  rules: { ruleIds: string[] } | { benchmarkIds: string[] }
  source: {
    review: {
      result?: ReviewResult
      detail?: string
      comment?: string
      status?:
        | ReviewStatusLabel
        | { label: ReviewStatusLabel; text?: string | null }
    }
  }
  action: 'insert' | 'update' | 'merge'
  dryRun: boolean
  updateFilters?: unknown[]
}

export type ReviewBatchResponse = {
  inserted: number
  updated: number
  failedValidation: number
  validationErrors: Array<{
    assetId?: string
    ruleId?: string
    error?: string
  }>
}

export type ReviewBatchResponseDryRun = {
  willInsert: number
  willUpdate: number
  willFailValidation: number
  validationErrors: Array<{
    assetId?: string
    ruleId?: string
    error?: string
  }>
}

export async function postReviewBatch(
  collectionId: string,
  body: ReviewBatchInput,
): Promise<ReviewBatchResponse | ReviewBatchResponseDryRun> {
  const result = await apiClient.POST('/collections/{collectionId}/reviews', {
    params: { path: { collectionId } },
    body: body as never,
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`batch review: HTTP ${result.response.status}`)
  }
  return result.data as unknown as
    | ReviewBatchResponse
    | ReviewBatchResponseDryRun
}
