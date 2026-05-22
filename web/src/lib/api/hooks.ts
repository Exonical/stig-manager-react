// TanStack Query wrappers around the typed openapi-fetch client. Each
// hook is the entry point pages use to read API data; centralising the
// query keys here keeps invalidation predictable.

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query'

import { apiClient } from './client'
import {
  createAsset,
  createCollection,
  deleteAsset,
  deleteReviewHistory,
  fetchAppInfo,
  fetchAsset,
  fetchAssets,
  fetchAssetStigs,
  fetchCollection,
  fetchCollections,
  fetchCollectionStigs,
  fetchCurrentUser,
  fetchMetricsSummaryByAsset,
  fetchMetricsSummaryByStig,
  fetchMetricsSummaryCollection,
  fetchReviewByAssetRule,
  fetchReviewHistory,
  fetchReviewHistoryStats,
  fetchReviewsByAsset,
  fetchRulesByRevision,
  postReviewBatch,
  putReviewByAssetRule,
  updateAsset,
  type AppInfo,
  type Asset,
  type AssetForm,
  type AssetStig,
  type AssetUpdateInput,
  type CollectionStig,
  type CollectionSummary,
  type CreateCollectionInput,
  type CurrentUser,
  type DeleteReviewHistoryInput,
  type MetricsSummaryAggAsset,
  type MetricsSummaryAggCollection,
  type MetricsSummaryAggStig,
  type Review,
  type ReviewBatchInput,
  type ReviewBatchResponse,
  type ReviewBatchResponseDryRun,
  type ReviewHistoryAsset,
  type ReviewHistoryFilters,
  type ReviewHistoryStats,
  type ReviewPutInput,
  type ReviewResult,
  type ReviewStatusLabel,
  type Rule,
} from './index'

// Re-export commonly-used types for consumers that already import
// from this module.
export type {
  Asset,
  AssetForm,
  AssetStig,
  AssetUpdateInput,
  CollectionStig,
  DeleteReviewHistoryInput,
  MetricsSummaryAggAsset,
  MetricsSummaryAggCollection,
  MetricsSummaryAggStig,
  Review,
  ReviewBatchInput,
  ReviewBatchResponse,
  ReviewBatchResponseDryRun,
  ReviewHistoryAsset,
  ReviewHistoryFilters,
  ReviewHistoryStats,
  ReviewPutInput,
  ReviewResult,
  ReviewStatusLabel,
  Rule,
}

export const QUERY_KEYS = {
  appInfo: ['op', 'appinfo'] as const,
  opState: ['op', 'state'] as const,
  user: ['user'] as const,
  collections: ['collections'] as const,
  collection: (id: string) => ['collection', id] as const,
  collectionStigs: (id: string) => ['collection', id, 'stigs'] as const,
  assets: (collectionId: string) => ['assets', collectionId] as const,
  asset: (id: string) => ['asset', id] as const,
  assetStigs: (id: string) => ['asset', id, 'stigs'] as const,
  rulesByRevision: (b: string, r: string) =>
    ['rules', b, r] as const,
  reviewsByAsset: (cid: string, aid: string) =>
    ['reviews', cid, aid] as const,
  review: (cid: string, aid: string, ruleId: string) =>
    ['review', cid, aid, ruleId] as const,
  metricsCollection: (cid: string) =>
    ['collection', cid, 'metrics', 'collection'] as const,
  metricsAsset: (cid: string) =>
    ['collection', cid, 'metrics', 'asset'] as const,
  metricsStig: (cid: string) =>
    ['collection', cid, 'metrics', 'stig'] as const,
  reviewHistory: (cid: string, filters?: ReviewHistoryFilters) =>
    ['collection', cid, 'review-history', filters ?? {}] as const,
  reviewHistoryStats: (
    cid: string,
    params?: { projection?: 'asset' } & ReviewHistoryFilters,
  ) =>
    ['collection', cid, 'review-history-stats', params ?? {}] as const,
} as const

export function useAppInfo(): UseQueryResult<AppInfo> {
  return useQuery({
    queryKey: QUERY_KEYS.appInfo,
    queryFn: fetchAppInfo,
  })
}

export function useCollections(params?: {
  name?: string
}): UseQueryResult<CollectionSummary[]> {
  const name = params?.name?.trim() ?? ''
  return useQuery({
    queryKey: name ? [...QUERY_KEYS.collections, { name }] : QUERY_KEYS.collections,
    queryFn: () => fetchCollections(name ? { name } : undefined),
  })
}

export function useCollection(
  collectionId: string | undefined,
): UseQueryResult<CollectionSummary> {
  return useQuery({
    queryKey: collectionId ? QUERY_KEYS.collection(collectionId) : ['collection', 'noop'],
    queryFn: () => fetchCollection(collectionId as string),
    enabled: Boolean(collectionId),
  })
}

export function useCurrentUser(): UseQueryResult<CurrentUser> {
  return useQuery({
    queryKey: QUERY_KEYS.user,
    queryFn: fetchCurrentUser,
    // Identity is sticky; cache for the full gc window.
    staleTime: 5 * 60_000,
  })
}

export function useCreateCollection(): UseMutationResult<
  CollectionSummary,
  Error,
  CreateCollectionInput
> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createCollection,
    onSuccess: () => {
      // Any cached list view (with or without a name filter) is stale
      // after the create; QUERY_KEYS.collections is the prefix.
      void qc.invalidateQueries({ queryKey: QUERY_KEYS.collections })
      // The caller is automatically granted Owner on the new
      // collection, so /user (which carries collectionGrants) is stale.
      // Without this, the detail page renders with role=null and hides
      // the tabs that the new owner is supposed to see.
      void qc.invalidateQueries({ queryKey: QUERY_KEYS.user })
    },
  })
}

export interface OpState {
  status: string
  dependencies?: Record<string, { status: string; detail?: string }>
}

export function useOpState(): UseQueryResult<OpState> {
  return useQuery({
    queryKey: QUERY_KEYS.opState,
    queryFn: async (): Promise<OpState> => {
      const result = await apiClient.GET('/op/state', {})
      if (!result.response.ok || !result.data) {
        throw new Error(`op/state: HTTP ${result.response.status}`)
      }
      return result.data as unknown as OpState
    },
    // Short stale time so the dashboard's "is the backend up" tile
    // refreshes on revisit without spamming the endpoint.
    staleTime: 5_000,
  })
}

// ---- Assets -------------------------------------------------------------

export function useAssets(params: {
  collectionId: string | undefined
  name?: string
  labelId?: string
}): UseQueryResult<Asset[]> {
  const { collectionId, name, labelId } = params
  const key: readonly unknown[] = collectionId
    ? [...QUERY_KEYS.assets(collectionId), { name: name ?? '', labelId: labelId ?? '' }]
    : ['assets', 'noop']
  return useQuery({
    queryKey: key,
    queryFn: () =>
      fetchAssets({
        collectionId: collectionId as string,
        name: name || undefined,
        labelId: labelId || undefined,
      }),
    enabled: Boolean(collectionId),
  })
}

export function useAsset(assetId: string | undefined): UseQueryResult<Asset> {
  return useQuery({
    queryKey: assetId ? QUERY_KEYS.asset(assetId) : ['asset', 'noop'],
    queryFn: () => fetchAsset(assetId as string),
    enabled: Boolean(assetId),
  })
}

export function useAssetStigs(
  assetId: string | undefined,
): UseQueryResult<AssetStig[]> {
  return useQuery({
    queryKey: assetId ? QUERY_KEYS.assetStigs(assetId) : ['asset', 'noop', 'stigs'],
    queryFn: () => fetchAssetStigs(assetId as string),
    enabled: Boolean(assetId),
  })
}

export function useCollectionStigs(
  collectionId: string | undefined,
): UseQueryResult<CollectionStig[]> {
  return useQuery({
    queryKey: collectionId ? QUERY_KEYS.collectionStigs(collectionId) : ['collection', 'noop', 'stigs'],
    queryFn: () => fetchCollectionStigs(collectionId as string),
    enabled: Boolean(collectionId),
  })
}

export function useCreateAsset(): UseMutationResult<Asset, Error, AssetForm> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createAsset,
    onSuccess: (asset) => {
      const cid = asset.collection?.collectionId
      if (cid) {
        void qc.invalidateQueries({ queryKey: QUERY_KEYS.assets(cid) })
        void qc.invalidateQueries({ queryKey: QUERY_KEYS.collectionStigs(cid) })
      }
    },
  })
}

export function useUpdateAsset(): UseMutationResult<
  Asset,
  Error,
  { assetId: string; input: AssetUpdateInput }
> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ assetId, input }) => updateAsset(assetId, input),
    onSuccess: (asset) => {
      void qc.invalidateQueries({ queryKey: QUERY_KEYS.asset(asset.assetId) })
      void qc.invalidateQueries({ queryKey: QUERY_KEYS.assetStigs(asset.assetId) })
      const cid = asset.collection?.collectionId
      if (cid) {
        void qc.invalidateQueries({ queryKey: QUERY_KEYS.assets(cid) })
      }
    },
  })
}

export function useDeleteAsset(): UseMutationResult<
  Asset,
  Error,
  { assetId: string; collectionId?: string }
> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ assetId }) => deleteAsset(assetId),
    onSuccess: (_data, { collectionId }) => {
      if (collectionId) {
        void qc.invalidateQueries({ queryKey: QUERY_KEYS.assets(collectionId) })
      }
    },
  })
}

// ---- Rules & Reviews ----------------------------------------------------

export function useRulesByRevision(
  benchmarkId: string | undefined,
  revisionStr: string | undefined,
): UseQueryResult<Rule[]> {
  return useQuery({
    queryKey:
      benchmarkId && revisionStr
        ? QUERY_KEYS.rulesByRevision(benchmarkId, revisionStr)
        : ['rules', 'noop'],
    queryFn: () =>
      fetchRulesByRevision(benchmarkId as string, revisionStr as string),
    enabled: Boolean(benchmarkId && revisionStr),
    // Rules are revision-static; cache aggressively.
    staleTime: 5 * 60_000,
  })
}

export function useReviewsByAsset(
  collectionId: string | undefined,
  assetId: string | undefined,
): UseQueryResult<Review[]> {
  return useQuery({
    queryKey:
      collectionId && assetId
        ? QUERY_KEYS.reviewsByAsset(collectionId, assetId)
        : ['reviews', 'noop'],
    queryFn: () =>
      fetchReviewsByAsset(collectionId as string, assetId as string),
    enabled: Boolean(collectionId && assetId),
  })
}

export function useReview(
  collectionId: string | undefined,
  assetId: string | undefined,
  ruleId: string | undefined,
): UseQueryResult<Review | null> {
  return useQuery({
    queryKey:
      collectionId && assetId && ruleId
        ? QUERY_KEYS.review(collectionId, assetId, ruleId)
        : ['review', 'noop'],
    queryFn: () =>
      fetchReviewByAssetRule(
        collectionId as string,
        assetId as string,
        ruleId as string,
      ),
    enabled: Boolean(collectionId && assetId && ruleId),
  })
}

export function usePutReview(): UseMutationResult<
  Review,
  Error,
  {
    collectionId: string
    assetId: string
    ruleId: string
    body: ReviewPutInput
  }
> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ collectionId, assetId, ruleId, body }) =>
      putReviewByAssetRule(collectionId, assetId, ruleId, body),
    onSuccess: (_data, { collectionId, assetId, ruleId }) => {
      void qc.invalidateQueries({
        queryKey: QUERY_KEYS.review(collectionId, assetId, ruleId),
      })
      void qc.invalidateQueries({
        queryKey: QUERY_KEYS.reviewsByAsset(collectionId, assetId),
      })
    },
  })
}

export function useReviewBatch(): UseMutationResult<
  ReviewBatchResponse | ReviewBatchResponseDryRun,
  Error,
  { collectionId: string; body: ReviewBatchInput }
> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ collectionId, body }) => postReviewBatch(collectionId, body),
    onSuccess: (_data, { collectionId, body }) => {
      // Real (non-dry-run) writes will have changed reviews under one
      // or more assets — invalidate the list view conservatively.
      if (body.dryRun) return
      void qc.invalidateQueries({ queryKey: ['reviews', collectionId] })
    },
  })
}

// ---- Metrics (M18e) -----------------------------------------------------

export function useMetricsCollection(
  collectionId: string | undefined,
): UseQueryResult<MetricsSummaryAggCollection> {
  return useQuery({
    queryKey: collectionId
      ? QUERY_KEYS.metricsCollection(collectionId)
      : ['metrics', 'noop'],
    queryFn: () => fetchMetricsSummaryCollection(collectionId as string),
    enabled: Boolean(collectionId),
  })
}

export function useMetricsByAsset(
  collectionId: string | undefined,
): UseQueryResult<MetricsSummaryAggAsset[]> {
  return useQuery({
    queryKey: collectionId
      ? QUERY_KEYS.metricsAsset(collectionId)
      : ['metrics', 'asset', 'noop'],
    queryFn: () => fetchMetricsSummaryByAsset(collectionId as string),
    enabled: Boolean(collectionId),
  })
}

export function useMetricsByStig(
  collectionId: string | undefined,
): UseQueryResult<MetricsSummaryAggStig[]> {
  return useQuery({
    queryKey: collectionId
      ? QUERY_KEYS.metricsStig(collectionId)
      : ['metrics', 'stig', 'noop'],
    queryFn: () => fetchMetricsSummaryByStig(collectionId as string),
    enabled: Boolean(collectionId),
  })
}

// ---- Review History (M18e) ----------------------------------------------

export function useReviewHistory(
  collectionId: string | undefined,
  filters?: ReviewHistoryFilters,
): UseQueryResult<ReviewHistoryAsset[]> {
  return useQuery({
    queryKey: collectionId
      ? QUERY_KEYS.reviewHistory(collectionId, filters)
      : ['review-history', 'noop'],
    queryFn: () => fetchReviewHistory(collectionId as string, filters),
    enabled: Boolean(collectionId),
  })
}

export function useReviewHistoryStats(
  collectionId: string | undefined,
  params?: { projection?: 'asset' } & ReviewHistoryFilters,
): UseQueryResult<ReviewHistoryStats> {
  return useQuery({
    queryKey: collectionId
      ? QUERY_KEYS.reviewHistoryStats(collectionId, params)
      : ['review-history-stats', 'noop'],
    queryFn: () => fetchReviewHistoryStats(collectionId as string, params),
    enabled: Boolean(collectionId),
  })
}

export function useDeleteReviewHistory(): UseMutationResult<
  { HistoryEntriesDeleted: number },
  Error,
  { collectionId: string; input: DeleteReviewHistoryInput }
> {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ collectionId, input }) =>
      deleteReviewHistory(collectionId, input),
    onSuccess: (_data, { collectionId }) => {
      void qc.invalidateQueries({
        queryKey: ['collection', collectionId, 'review-history'],
      })
      void qc.invalidateQueries({
        queryKey: ['collection', collectionId, 'review-history-stats'],
      })
    },
  })
}
