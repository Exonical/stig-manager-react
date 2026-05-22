// TanStack Query wrappers around the typed openapi-fetch client. Each
// hook is the entry point pages use to read API data; centralising the
// query keys here keeps invalidation predictable.

import { useQuery, type UseQueryResult } from '@tanstack/react-query'

import { apiClient } from './client'
import { fetchAppInfo, fetchCollections, type AppInfo, type CollectionSummary } from './index'

export const QUERY_KEYS = {
  appInfo: ['op', 'appinfo'] as const,
  opState: ['op', 'state'] as const,
  collections: ['collections'] as const,
} as const

export function useAppInfo(): UseQueryResult<AppInfo> {
  return useQuery({
    queryKey: QUERY_KEYS.appInfo,
    queryFn: fetchAppInfo,
  })
}

export function useCollections(): UseQueryResult<CollectionSummary[]> {
  return useQuery({
    queryKey: QUERY_KEYS.collections,
    queryFn: fetchCollections,
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
