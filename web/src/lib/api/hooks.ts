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
  createCollection,
  fetchAppInfo,
  fetchCollection,
  fetchCollections,
  fetchCurrentUser,
  type AppInfo,
  type CollectionSummary,
  type CreateCollectionInput,
  type CurrentUser,
} from './index'

export const QUERY_KEYS = {
  appInfo: ['op', 'appinfo'] as const,
  opState: ['op', 'state'] as const,
  user: ['user'] as const,
  collections: ['collections'] as const,
  collection: (id: string) => ['collection', id] as const,
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
