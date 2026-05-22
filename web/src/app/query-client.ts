// Singleton TanStack Query client used by every query/mutation in the
// SPA. Defaults are tuned for an internal admin app: aggressive cache
// reuse for catalog data (collections, library) and a short stale time
// so background polling doesn't drift.

import { QueryClient } from '@tanstack/react-query'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 5 * 60_000,
      refetchOnWindowFocus: false,
      retry: (failureCount, error) => {
        // 401/403 won't fix themselves on retry — surface them
        // immediately so route guards can redirect to sign-in.
        const status = (error as { status?: number } | null)?.status
        if (status === 401 || status === 403) return false
        return failureCount < 2
      },
    },
  },
})
