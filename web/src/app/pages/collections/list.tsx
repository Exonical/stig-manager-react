// /collections — list view. Renders every Collection the signed-in
// user can see, with a name substring filter, sortable columns, and
// a "New Collection" CTA gated on the `stig-manager:collection` scope.

import { Loader2, Plus, Search } from 'lucide-react'
import * as React from 'react'
import { Link } from 'react-router-dom'

import { NewCollectionDialog } from './new-collection-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useCollections } from '@/lib/api/hooks'
import { useAuth } from '@/lib/auth/auth-context'
import { hasScope } from '@/lib/auth/scopes'

type SortKey = 'name' | 'description'
type SortDir = 'asc' | 'desc'

export function CollectionsListPage() {
  const { status } = useAuth()
  const user = status.kind === 'signed-in' ? status.user : null
  const canCreate = hasScope(user, 'stig-manager:collection')

  const [query, setQuery] = React.useState('')
  const [debounced, setDebounced] = React.useState('')
  React.useEffect(() => {
    const id = window.setTimeout(() => setDebounced(query), 200)
    return () => window.clearTimeout(id)
  }, [query])

  const collections = useCollections(debounced ? { name: debounced } : undefined)

  const [sortKey, setSortKey] = React.useState<SortKey>('name')
  const [sortDir, setSortDir] = React.useState<SortDir>('asc')
  const sorted = React.useMemo(() => {
    const rows = collections.data ?? []
    return [...rows].sort((a, b) => {
      const av = (a[sortKey] ?? '').toString().toLowerCase()
      const bv = (b[sortKey] ?? '').toString().toLowerCase()
      const cmp = av.localeCompare(bv)
      return sortDir === 'asc' ? cmp : -cmp
    })
  }, [collections.data, sortKey, sortDir])

  const [dialogOpen, setDialogOpen] = React.useState(false)

  function flipSort(key: SortKey) {
    if (key === sortKey) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortKey(key)
      setSortDir('asc')
    }
  }

  return (
    <div className="space-y-6" data-testid="collections-list-page">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Collections</h1>
          <p className="mt-1 text-sm text-[var(--color-muted-foreground)]">
            Pick a Collection to review or manage. Filter by name to narrow the list.
          </p>
        </div>
        {canCreate && (
          <Button
            size="sm"
            onClick={() => setDialogOpen(true)}
            data-testid="new-collection-button"
          >
            <Plus className="size-4" /> New Collection
          </Button>
        )}
      </header>

      <div className="flex items-center gap-2">
        <div className="relative w-full max-w-md">
          <Search className="pointer-events-none absolute left-2 top-1/2 size-4 -translate-y-1/2 text-[var(--color-muted-foreground)]" />
          <Input
            placeholder="Search by name…"
            className="pl-8"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            data-testid="collections-search"
          />
        </div>
        {collections.isFetching && !collections.isLoading && (
          <Loader2 className="size-4 animate-spin text-[var(--color-muted-foreground)]" />
        )}
      </div>

      <div className="overflow-x-auto rounded-md border border-[var(--color-border)]">
        <table className="w-full text-sm" data-testid="collections-table">
          <thead className="bg-[var(--color-muted)]/30 text-left text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
            <tr>
              <th className="px-4 py-2">
                <button
                  type="button"
                  className="font-medium uppercase tracking-wider hover:text-[var(--color-foreground)]"
                  onClick={() => flipSort('name')}
                  data-testid="sort-name"
                >
                  Name {sortKey === 'name' ? (sortDir === 'asc' ? '↑' : '↓') : ''}
                </button>
              </th>
              <th className="px-4 py-2">
                <button
                  type="button"
                  className="font-medium uppercase tracking-wider hover:text-[var(--color-foreground)]"
                  onClick={() => flipSort('description')}
                >
                  Description {sortKey === 'description' ? (sortDir === 'asc' ? '↑' : '↓') : ''}
                </button>
              </th>
              <th className="w-24 px-4 py-2 text-right">ID</th>
            </tr>
          </thead>
          <tbody>
            {collections.isLoading && (
              <tr>
                <td className="px-4 py-6 text-center text-[var(--color-muted-foreground)]" colSpan={3}>
                  Loading collections…
                </td>
              </tr>
            )}
            {collections.isError && (
              <tr>
                <td className="px-4 py-6 text-center text-red-500" colSpan={3}>
                  Failed to load collections: {(collections.error as Error).message}
                </td>
              </tr>
            )}
            {!collections.isLoading && sorted.length === 0 && (
              <tr>
                <td className="px-4 py-6 text-center text-[var(--color-muted-foreground)]" colSpan={3}>
                  {debounced
                    ? `No collections match "${debounced}".`
                    : 'No collections yet.'}
                </td>
              </tr>
            )}
            {sorted.map((c) => (
              <tr
                key={c.collectionId}
                className="border-t border-[var(--color-border)] transition-colors hover:bg-[var(--color-muted)]/30"
                data-testid={`collection-row-${c.collectionId}`}
              >
                <td className="px-4 py-2 font-medium">
                  <Link
                    to={`/collections/${c.collectionId}`}
                    className="text-sky-500 hover:underline"
                    data-testid={`collection-link-${c.collectionId}`}
                  >
                    {c.name}
                  </Link>
                </td>
                <td className="px-4 py-2 text-[var(--color-muted-foreground)]">
                  {c.description || '—'}
                </td>
                <td className="px-4 py-2 text-right font-mono text-xs text-[var(--color-muted-foreground)]">
                  {c.collectionId}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {canCreate && (
        <NewCollectionDialog open={dialogOpen} onOpenChange={setDialogOpen} />
      )}
    </div>
  )
}
