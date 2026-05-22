// Public surface of the typed API client.
//
// Higher-level wrappers can live here; for now we re-export the generated
// schema types and the openapi-fetch instance configured in ./client.
export { apiClient } from './client'
export type { paths, components, operations } from './client'

import { getAccessTokenForClient } from '../auth/access-token'
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

// ---- Metrics (M18e) -----------------------------------------------------
//
// The metrics-summary surface is a family of GET endpoints under
// /collections/{cid}/metrics/summary[/{aggregate}]. Each returns a
// MetricsSummaryAgg* shape from docs/openapi/stig-manager.yaml.
// `MetricsSummary` itself (the inner `metrics` object) is identical
// across aggregates — the outer envelope carries the aggregation
// dimension (collection-level totals, per-asset, per-stig, per-label).

/** Counters keyed by severity (low/medium/high). */
export type MetricsBySeverity = { high: number; low: number; medium: number }

/** Counters keyed by review status. */
export type MetricsStatusCounts = {
  saved: number
  submitted: number
  accepted: number
  rejected: number
}

/** Counters keyed by review result for the summary aggregates. */
export type MetricsResultCounts = {
  fail: number
  notapplicable: number
  other: number
  pass: number
}

/** Common per-aggregate metrics block returned by all summary endpoints. */
export type MetricsSummary = {
  metrics: {
    assessed: number
    assessedBySeverity: MetricsBySeverity
    assessments: number
    assessmentsBySeverity: MetricsBySeverity
    findings: MetricsBySeverity
    maxTouchTs?: string | null
    maxTs?: string | null
    minTs?: string | null
    results: MetricsResultCounts
    statuses: MetricsStatusCounts
  }
}

/** Aggregated metrics for the entire Collection. */
export type MetricsSummaryAggCollection = MetricsSummary & {
  collectionId: string
  name: string
  assets: number
  checklists: number
  stigs: number
}

/** Per-asset metrics row. */
export type MetricsSummaryAggAsset = MetricsSummary & {
  assetId: string
  name: string
  benchmarkIds: string[]
  labels?: Array<{ labelId?: string; name?: string; color?: string | null }>
}

/** Per-STIG metrics row. */
export type MetricsSummaryAggStig = MetricsSummary & {
  benchmarkId: string
  title?: string
  assets: number
  collections?: number
  ruleCount?: number
  revisionStr?: string
  revisionDate?: string | null
}

/** Per-label metrics row. */
export type MetricsSummaryAggLabel = MetricsSummary & {
  labelId: string | null
  name: string | null
  assets: number
}

export async function fetchMetricsSummaryCollection(
  collectionId: string,
): Promise<MetricsSummaryAggCollection> {
  const result = await apiClient.GET(
    '/collections/{collectionId}/metrics/summary/collection',
    { params: { path: { collectionId } } },
  )
  if (!result.response.ok || !result.data) {
    throw new Error(`metrics: HTTP ${result.response.status}`)
  }
  return result.data as unknown as MetricsSummaryAggCollection
}

export async function fetchMetricsSummaryByAsset(
  collectionId: string,
): Promise<MetricsSummaryAggAsset[]> {
  const result = await apiClient.GET(
    '/collections/{collectionId}/metrics/summary/asset',
    { params: { path: { collectionId } } },
  )
  if (!result.response.ok || !result.data) {
    throw new Error(`metrics by asset: HTTP ${result.response.status}`)
  }
  return result.data as unknown as MetricsSummaryAggAsset[]
}

export async function fetchMetricsSummaryByStig(
  collectionId: string,
): Promise<MetricsSummaryAggStig[]> {
  const result = await apiClient.GET(
    '/collections/{collectionId}/metrics/summary/stig',
    { params: { path: { collectionId } } },
  )
  if (!result.response.ok || !result.data) {
    throw new Error(`metrics by stig: HTTP ${result.response.status}`)
  }
  return result.data as unknown as MetricsSummaryAggStig[]
}

// ---- Review History (M18e) ----------------------------------------------

export type ReviewHistoryEntry = {
  ts: string
  touchTs?: string
  result: ReviewResult
  detail?: string
  comment?: string
  ruleId?: string
  status?: { label?: ReviewStatusLabel; text?: string | null }
  userId?: string
  username?: string
  autoResult?: boolean
}

export type ReviewHistoryRule = {
  ruleId: string
  history: ReviewHistoryEntry[]
}

export type ReviewHistoryAsset = {
  assetId: string
  reviewHistories: ReviewHistoryRule[]
}

export type ReviewHistoryStats = {
  collectionHistoryEntryCount: number
  oldestHistoryEntryDate: string
  assetHistoryEntryCounts?: Array<{
    assetId: string
    historyEntryCount?: number
    oldestHistoryEntry?: string | null
  }>
}

export type ReviewHistoryFilters = {
  assetId?: string
  ruleId?: string
  status?: ReviewStatusLabel
  startDate?: string
  endDate?: string
}

export async function fetchReviewHistory(
  collectionId: string,
  filters?: ReviewHistoryFilters,
): Promise<ReviewHistoryAsset[]> {
  const query: Record<string, string> = {}
  if (filters?.assetId) query['assetId'] = filters.assetId
  if (filters?.ruleId) query['ruleId'] = filters.ruleId
  if (filters?.status) query['status'] = filters.status
  if (filters?.startDate) query['startDate'] = filters.startDate
  if (filters?.endDate) query['endDate'] = filters.endDate
  const result = await apiClient.GET(
    '/collections/{collectionId}/review-history',
    { params: { path: { collectionId }, query: query as never } },
  )
  if (!result.response.ok || !result.data) {
    throw new Error(`review history: HTTP ${result.response.status}`)
  }
  return result.data as unknown as ReviewHistoryAsset[]
}

export async function fetchReviewHistoryStats(
  collectionId: string,
  params?: { projection?: 'asset' } & ReviewHistoryFilters,
): Promise<ReviewHistoryStats> {
  const query: Record<string, string> = {}
  if (params?.projection) query['projection'] = params.projection
  if (params?.assetId) query['assetId'] = params.assetId
  if (params?.ruleId) query['ruleId'] = params.ruleId
  if (params?.status) query['status'] = params.status
  if (params?.startDate) query['startDate'] = params.startDate
  if (params?.endDate) query['endDate'] = params.endDate
  const result = await apiClient.GET(
    '/collections/{collectionId}/review-history/stats',
    { params: { path: { collectionId }, query: query as never } },
  )
  if (!result.response.ok || !result.data) {
    throw new Error(`review history stats: HTTP ${result.response.status}`)
  }
  return result.data as unknown as ReviewHistoryStats
}

export type DeleteReviewHistoryInput = {
  retentionDate?: string
  assetId?: string
}

export async function deleteReviewHistory(
  collectionId: string,
  input: DeleteReviewHistoryInput,
): Promise<{ HistoryEntriesDeleted: number }> {
  const query: Record<string, string> = {}
  if (input.retentionDate) query['retentionDate'] = input.retentionDate
  if (input.assetId) query['assetId'] = input.assetId
  const result = await apiClient.DELETE(
    '/collections/{collectionId}/review-history',
    { params: { path: { collectionId }, query: query as never } },
  )
  if (!result.response.ok || !result.data) {
    throw new Error(`delete review history: HTTP ${result.response.status}`)
  }
  return result.data as unknown as { HistoryEntriesDeleted: number }
}

// ---- Exports (M18e) -----------------------------------------------------
//
// Archive + POAM endpoints return raw binary streams (application/zip
// or .xlsx). openapi-fetch is configured for JSON, so we drop down to
// plain fetch() with the bearer token and trigger a browser download
// from the resulting Blob. Errors are surfaced as JSON if the server
// produced an envelope; otherwise we surface the status text.

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api'

async function fetchBlob(
  method: 'GET' | 'POST',
  path: string,
  init: { body?: unknown; query?: Record<string, string> } = {},
): Promise<{ blob: Blob; filename: string }> {
  const url = new URL(`${API_BASE}${path}`, window.location.origin)
  for (const [k, v] of Object.entries(init.query ?? {})) {
    if (v !== '' && v !== undefined && v !== null) url.searchParams.set(k, v)
  }
  const headers: Record<string, string> = {}
  const token = getAccessTokenForClient()
  if (token) headers['Authorization'] = `Bearer ${token}`
  let body: BodyInit | undefined
  if (init.body !== undefined) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(init.body)
  }
  const resp = await fetch(url.toString(), {
    method,
    headers,
    body,
    credentials: 'include',
  })
  if (!resp.ok) {
    let detail = ''
    try {
      detail = (await resp.text()).slice(0, 256)
    } catch {
      // ignore body-read failure; we still have status
    }
    throw new Error(
      `${method} ${path}: HTTP ${resp.status}${detail ? ` — ${detail}` : ''}`,
    )
  }
  const filename = extractFilename(resp.headers.get('Content-Disposition'))
  const blob = await resp.blob()
  return { blob, filename }
}

function extractFilename(disposition: string | null): string {
  if (!disposition) return 'download'
  // RFC 6266 — prefer filename* (UTF-8 encoded) when present.
  const star = /filename\*=UTF-8''([^;]+)/i.exec(disposition)
  if (star && star[1]) {
    try {
      return decodeURIComponent(star[1])
    } catch {
      return star[1]
    }
  }
  const plain = /filename="?([^";]+)"?/i.exec(disposition)
  if (plain && plain[1]) return plain[1]
  return 'download'
}

/** Trigger a browser download for a Blob from JS without leaving the SPA. */
export function downloadBlob(blob: Blob, filename: string): void {
  const href = URL.createObjectURL(blob)
  try {
    const a = document.createElement('a')
    a.href = href
    a.download = filename
    document.body.appendChild(a)
    a.click()
    a.remove()
  } finally {
    // Browsers hold the blob until revocation; defer slightly so the
    // download has a chance to start.
    setTimeout(() => URL.revokeObjectURL(href), 30_000)
  }
}

/**
 * Body of POST /archive/ckl|cklb|xccdf. Each entry pins an Asset and
 * (optionally) the list of STIG benchmarks to include for that asset.
 * Per the schema, omitting `stigs` requests the default revisions of
 * every benchmark mapped to the asset and visible to the caller.
 */
export type AssetStigSelection = {
  assetId: string
  stigs?: Array<string | { benchmarkId: string; revisionStr: string }>
}

export type CklMode = 'mono' | 'multi'

export async function downloadCklArchive(
  collectionId: string,
  selections: AssetStigSelection[],
  mode: CklMode = 'mono',
): Promise<void> {
  const { blob, filename } = await fetchBlob(
    'POST',
    `/collections/${encodeURIComponent(collectionId)}/archive/ckl`,
    { body: selections, query: { mode } },
  )
  downloadBlob(blob, filename || `collection-${collectionId}-ckl.zip`)
}

export async function downloadCklbArchive(
  collectionId: string,
  selections: AssetStigSelection[],
  mode: CklMode = 'mono',
): Promise<void> {
  const { blob, filename } = await fetchBlob(
    'POST',
    `/collections/${encodeURIComponent(collectionId)}/archive/cklb`,
    { body: selections, query: { mode } },
  )
  downloadBlob(blob, filename || `collection-${collectionId}-cklb.zip`)
}

export async function downloadXccdfArchive(
  collectionId: string,
  selections: AssetStigSelection[],
): Promise<void> {
  const { blob, filename } = await fetchBlob(
    'POST',
    `/collections/${encodeURIComponent(collectionId)}/archive/xccdf`,
    { body: selections },
  )
  downloadBlob(blob, filename || `collection-${collectionId}-xccdf.zip`)
}

// ---- POAM (M18e) --------------------------------------------------------

export type PoamAggregator = 'groupId' | 'ruleId'
export type PoamFormat = 'emass' | 'mccast'

export type PoamInput = {
  aggregator?: PoamAggregator
  format?: PoamFormat
  acceptedOnly?: boolean
  benchmarkId?: string
  assetId?: string
  date?: string
  office?: string
  status?: string
  mccastPackageId?: string
  mccastAuthName?: string
}

export async function downloadPoam(
  collectionId: string,
  input: PoamInput = {},
): Promise<void> {
  const query: Record<string, string> = {}
  if (input.aggregator) query['aggregator'] = input.aggregator
  if (input.format) query['format'] = input.format
  if (input.acceptedOnly) query['acceptedOnly'] = 'true'
  if (input.benchmarkId) query['benchmarkId'] = input.benchmarkId
  if (input.assetId) query['assetId'] = input.assetId
  if (input.date) query['date'] = input.date
  if (input.office) query['office'] = input.office
  if (input.status) query['status'] = input.status
  if (input.mccastPackageId)
    query['mccastPackageId'] = input.mccastPackageId
  if (input.mccastAuthName) query['mccastAuthName'] = input.mccastAuthName
  const { blob, filename } = await fetchBlob(
    'GET',
    `/collections/${encodeURIComponent(collectionId)}/poam`,
    { query },
  )
  downloadBlob(blob, filename || `poam-${collectionId}.xlsx`)
}

// ---- Users / User Groups (M18f) ----------------------------------------

export type UserStatus = 'available' | 'unavailable'

export type CollectionGrantInput = {
  collectionId: string
  roleId: 1 | 2 | 3 | 4
}

export type UserSummary = {
  userId: string
  username: string
  displayName?: string
  email?: string | null
  status?: UserStatus
  statusDate?: string
  statusUser?: { userId?: string; username?: string } | null
  lastAccess?: number | null
  privileges?: {
    admin?: boolean
    create_collection?: boolean
  }
  collectionGrants?: Array<{
    roleId?: number
    collection?: { collectionId?: string; name?: string }
  }>
  userGroups?: Array<{
    userGroupId?: string
    name?: string
  }>
}

export type UsersFilter = {
  username?: string
  usernameMatch?: 'exact' | 'startsWith' | 'endsWith' | 'contains'
  status?: UserStatus
  privilege?: 'admin' | 'create_collection'
}

/**
 * Lists Users visible to the requester. Requires the
 * `stig-manager:user:read` scope.
 */
export async function fetchUsers(
  filter?: UsersFilter,
): Promise<UserSummary[]> {
  const query: Record<string, string> = {
    elevate: 'true',
    projection: 'collectionGrants',
  }
  if (filter?.username) query['username'] = filter.username
  if (filter?.usernameMatch) query['username-match'] = filter.usernameMatch
  if (filter?.status) query['status'] = filter.status
  if (filter?.privilege) query['privilege'] = filter.privilege
  const result = await apiClient.GET('/users', {
    params: { query: query as never },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`users: HTTP ${result.response.status}`)
  }
  return result.data as unknown as UserSummary[]
}

export async function fetchUser(userId: string): Promise<UserSummary> {
  const query: Record<string, string> = {
    elevate: 'true',
    projection: 'collectionGrants',
  }
  const result = await apiClient.GET('/users/{userId}', {
    params: { path: { userId }, query: query as never },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`user ${userId}: HTTP ${result.response.status}`)
  }
  return result.data as unknown as UserSummary
}

export type UserCreateInput = {
  username: string
  collectionGrants: CollectionGrantInput[]
  userGroups?: string[]
}

export async function createUser(input: UserCreateInput): Promise<UserSummary> {
  const result = await apiClient.POST('/users', {
    params: { query: { elevate: true } as never },
    body: input as never,
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`create user: HTTP ${result.response.status}`)
  }
  return result.data as unknown as UserSummary
}

export type UserPatchInput = {
  username?: string
  status?: UserStatus
  collectionGrants?: CollectionGrantInput[]
  userGroups?: string[]
}

export async function updateUser(
  userId: string,
  input: UserPatchInput,
): Promise<UserSummary> {
  const result = await apiClient.PATCH('/users/{userId}', {
    params: { path: { userId }, query: { elevate: true } as never },
    body: input as never,
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`update user: HTTP ${result.response.status}`)
  }
  return result.data as unknown as UserSummary
}

export async function deleteUser(userId: string): Promise<UserSummary> {
  const result = await apiClient.DELETE('/users/{userId}', {
    params: { path: { userId }, query: { elevate: true } as never },
  })
  if (!result.response.ok) {
    throw new Error(`delete user: HTTP ${result.response.status}`)
  }
  return result.data as unknown as UserSummary
}

export type UserGroupSummary = {
  userGroupId: string
  name: string
  description?: string | null
  users?: Array<{ userId?: string; username?: string; displayName?: string }>
  collectionGrants?: Array<{
    roleId?: number
    collection?: { collectionId?: string; name?: string }
  }>
}

export async function fetchUserGroups(): Promise<UserGroupSummary[]> {
  const query: Record<string, string> = {
    elevate: 'true',
    projection: 'users',
  }
  const result = await apiClient.GET('/user-groups', {
    params: { query: query as never },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`user groups: HTTP ${result.response.status}`)
  }
  return result.data as unknown as UserGroupSummary[]
}

export async function fetchUserGroup(
  userGroupId: string,
): Promise<UserGroupSummary> {
  const query: Record<string, string> = {
    elevate: 'true',
    projection: 'users',
  }
  const result = await apiClient.GET('/user-groups/{userGroupId}', {
    params: { path: { userGroupId }, query: query as never },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`user group ${userGroupId}: HTTP ${result.response.status}`)
  }
  return result.data as unknown as UserGroupSummary
}

export type UserGroupCreateInput = {
  name: string
  description?: string | null
  userIds?: string[]
  collectionGrants?: CollectionGrantInput[]
}

export async function createUserGroup(
  input: UserGroupCreateInput,
): Promise<UserGroupSummary> {
  const result = await apiClient.POST('/user-groups', {
    params: { query: { elevate: true } as never },
    body: input as never,
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`create user group: HTTP ${result.response.status}`)
  }
  return result.data as unknown as UserGroupSummary
}

export type UserGroupPatchInput = {
  name?: string
  description?: string | null
  userIds?: string[]
  collectionGrants?: CollectionGrantInput[]
}

export async function updateUserGroup(
  userGroupId: string,
  input: UserGroupPatchInput,
): Promise<UserGroupSummary> {
  const result = await apiClient.PATCH('/user-groups/{userGroupId}', {
    params: { path: { userGroupId }, query: { elevate: true } as never },
    body: input as never,
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`update user group: HTTP ${result.response.status}`)
  }
  return result.data as unknown as UserGroupSummary
}

export async function deleteUserGroup(
  userGroupId: string,
): Promise<UserGroupSummary> {
  const result = await apiClient.DELETE('/user-groups/{userGroupId}', {
    params: { path: { userGroupId }, query: { elevate: true } as never },
  })
  if (!result.response.ok) {
    throw new Error(`delete user group: HTTP ${result.response.status}`)
  }
  return result.data as unknown as UserGroupSummary
}

// ---- Jobs (M18f) -------------------------------------------------------

export type JobTask = {
  taskId: string
  name: string
  description?: string | null
  command?: string
}

export type JobRunState = 'running' | 'completed' | 'failed' | null

export type JobRun = {
  runId: string
  jobId?: string
  created: string
  updated?: string | null
  state: JobRunState
}

export type JobRunOutput = {
  seq?: number
  type: string
  message: string
  task: string
  taskId?: string
  ts: string
}

export type JobEvent =
  | number
  | null
  | {
      type: 'once'
      eventId?: string
      starts: string
      enabled?: boolean
    }
  | {
      type: 'recurring'
      eventId?: string
      interval: { value: string; field: 'minute' | 'hour' | 'day' | 'week' | 'month' }
      starts?: string | null
      ends?: string | null
      enabled?: boolean
    }

export type Job = {
  jobId: string
  name: string
  description?: string | null
  createdBy?: { userId?: string; username?: string } | null
  created: string
  updatedBy?: { userId?: string; username?: string } | null
  updated?: string | null
  tasks: JobTask[]
  event?: JobEvent
  runCount?: number
  lastRun?: JobRun | null
}

export async function fetchJobs(): Promise<Job[]> {
  const result = await apiClient.GET('/jobs', {
    params: { query: { elevate: true } as never },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`jobs: HTTP ${result.response.status}`)
  }
  return result.data as unknown as Job[]
}

export async function fetchJob(jobId: string): Promise<Job> {
  const result = await apiClient.GET('/jobs/{jobId}', {
    params: { path: { jobId }, query: { elevate: true } as never },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`job ${jobId}: HTTP ${result.response.status}`)
  }
  return result.data as unknown as Job
}

export async function fetchJobTasks(): Promise<JobTask[]> {
  const result = await apiClient.GET('/jobs/tasks', {
    params: { query: { elevate: true } as never },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`job tasks: HTTP ${result.response.status}`)
  }
  return result.data as unknown as JobTask[]
}

export type JobCreateInput = {
  name: string
  description?: string | null
  tasks: string[]
  event?: JobEvent
}

export async function createJob(input: JobCreateInput): Promise<Job> {
  const result = await apiClient.POST('/jobs', {
    params: { query: { elevate: true } as never },
    body: input as never,
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`create job: HTTP ${result.response.status}`)
  }
  return result.data as unknown as Job
}

export async function deleteJob(jobId: string): Promise<void> {
  const result = await apiClient.DELETE('/jobs/{jobId}', {
    params: { path: { jobId }, query: { elevate: true } as never },
  })
  if (!result.response.ok) {
    throw new Error(`delete job: HTTP ${result.response.status}`)
  }
}

export async function fetchJobRuns(jobId: string): Promise<JobRun[]> {
  const result = await apiClient.GET('/jobs/{jobId}/runs', {
    params: { path: { jobId }, query: { elevate: true } as never },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`job runs: HTTP ${result.response.status}`)
  }
  return result.data as unknown as JobRun[]
}

export async function startJobRun(
  jobId: string,
): Promise<{ runId: string }> {
  const result = await apiClient.POST('/jobs/{jobId}/runs', {
    params: { path: { jobId }, query: { elevate: true } as never },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`start job run: HTTP ${result.response.status}`)
  }
  return result.data as unknown as { runId: string }
}

export async function fetchJobRunOutput(
  runId: string,
  afterSeq?: number,
): Promise<JobRunOutput[]> {
  const query: Record<string, string> = { elevate: 'true' }
  if (afterSeq !== undefined && afterSeq > 0) {
    query['after-seq'] = String(afterSeq)
  }
  const result = await apiClient.GET('/jobs/runs/{runId}/output', {
    params: { path: { runId }, query: query as never },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`job run output: HTTP ${result.response.status}`)
  }
  return result.data as unknown as JobRunOutput[]
}

// ---- AppInfo / AppData tables (M18f) -----------------------------------

/**
 * Rich shape returned by `GET /op/appinfo`. The API populates the
 * structurally-stable subset; this type captures what the SPA renders.
 * Unknown sub-fields are tolerated via index access in the page.
 */
export type AppInfoDetail = {
  version: string
  commit?: string
  buildDate?: string
  date?: string
  schema?: string
  counts?: {
    users?: number
    userGroups?: number
    collections?: number
    assets?: number
    stigs?: number
    reviews?: number
    jobs?: number
    runs?: number
  }
  postgres?: {
    version?: string
    startTime?: string
    uptime?: number
    connections?: number
    variables?: Record<string, string | number | null>
  }
  runtime?: {
    goroutines?: number
    cpus?: number
    goVersion?: string
    os?: string
    arch?: string
    uptime?: number
    memory?: {
      heapAlloc?: number
      heapInuse?: number
      stackSys?: number
      sys?: number
    }
  }
  requests?: {
    totalRequests?: number
    totalApiRequests?: number
    totalRequestDuration?: number
    totalErrors?: number
    operationIds?: Record<
      string,
      {
        totalRequests?: number
        totalDuration?: number
        minDuration?: number
        maxDuration?: number
        errors?: number
      }
    >
  }
}

export async function fetchAppInfoDetail(): Promise<AppInfoDetail> {
  const result = await apiClient.GET('/op/appinfo', {})
  if (!result.response.ok || !result.data) {
    throw new Error(`appinfo: HTTP ${result.response.status}`)
  }
  return result.data as unknown as AppInfoDetail
}

export type AppDataTable = {
  name?: string
  rows?: number
  dataLength?: number
}

export async function fetchAppDataTables(): Promise<AppDataTable[]> {
  const result = await apiClient.GET('/op/appdata/tables', {})
  if (!result.response.ok || !result.data) {
    throw new Error(`appdata tables: HTTP ${result.response.status}`)
  }
  return result.data as unknown as AppDataTable[]
}

// ---- Collection Grants (M18f) ------------------------------------------

export type GrantSubject = {
  user?: { userId?: string; username?: string; displayName?: string }
  userGroup?: { userGroupId?: string; name?: string }
}

export type CollectionGrant = GrantSubject & {
  grantId?: string
  roleId: 1 | 2 | 3 | 4
}

export async function fetchCollectionGrants(
  collectionId: string,
): Promise<CollectionGrant[]> {
  const query: Record<string, string> = { elevate: 'true' }
  const result = await apiClient.GET('/collections/{collectionId}/grants', {
    params: { path: { collectionId }, query: query as never },
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`grants: HTTP ${result.response.status}`)
  }
  return result.data as unknown as CollectionGrant[]
}

export type GrantPostInput =
  | { userId: string; roleId: 1 | 2 | 3 | 4 }
  | { userGroupId: string; roleId: 1 | 2 | 3 | 4 }

export async function postCollectionGrants(
  collectionId: string,
  body: GrantPostInput[],
): Promise<CollectionGrant[]> {
  const query: Record<string, string> = { elevate: 'true' }
  const result = await apiClient.POST('/collections/{collectionId}/grants', {
    params: { path: { collectionId }, query: query as never },
    body: body as never,
  })
  if (!result.response.ok || !result.data) {
    throw new Error(`post grants: HTTP ${result.response.status}`)
  }
  return result.data as unknown as CollectionGrant[]
}

export async function putCollectionGrant(
  collectionId: string,
  grantId: string,
  body: GrantPostInput,
): Promise<CollectionGrant> {
  const query: Record<string, string> = { elevate: 'true' }
  const result = await apiClient.PUT(
    '/collections/{collectionId}/grants/{grantId}',
    {
      params: { path: { collectionId, grantId }, query: query as never },
      body: body as never,
    },
  )
  if (!result.response.ok || !result.data) {
    throw new Error(`put grant: HTTP ${result.response.status}`)
  }
  return result.data as unknown as CollectionGrant
}

export async function deleteCollectionGrant(
  collectionId: string,
  grantId: string,
): Promise<CollectionGrant> {
  const query: Record<string, string> = { elevate: 'true' }
  const result = await apiClient.DELETE(
    '/collections/{collectionId}/grants/{grantId}',
    {
      params: { path: { collectionId, grantId }, query: query as never },
    },
  )
  if (!result.response.ok) {
    throw new Error(`delete grant: HTTP ${result.response.status}`)
  }
  return result.data as unknown as CollectionGrant
}
