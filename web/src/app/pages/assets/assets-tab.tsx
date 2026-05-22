// Assets tab inside the Collection detail page. Lists every Asset in
// the Collection with name + filtering, and links each row to the
// per-Asset detail page at /collections/:cid/assets/:aid.
//
// A "New Asset" button is gated on the Collection role: anyone with
// Full (2) or higher can create Assets in their Collection.

import { Loader2, Plus, Search } from 'lucide-react'
import * as React from 'react'
import { Link } from 'react-router-dom'

import { NewAssetDialog } from './new-asset-dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { useAssets } from '@/lib/api/hooks'

interface AssetsTabProps {
  collectionId: string
  /** Current user's roleId on the Collection (null = no grant). */
  role: number | null
}

export function AssetsTab({ collectionId, role }: AssetsTabProps) {
  const [query, setQuery] = React.useState('')
  const [debounced, setDebounced] = React.useState('')
  React.useEffect(() => {
    const id = window.setTimeout(() => setDebounced(query), 200)
    return () => window.clearTimeout(id)
  }, [query])

  const assets = useAssets({
    collectionId,
    name: debounced || undefined,
  })

  const [dialogOpen, setDialogOpen] = React.useState(false)
  const canCreate = role !== null && role >= 2

  return (
    <Card data-testid="collection-assets-tab">
      <CardHeader className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <CardTitle>Assets</CardTitle>
          <p className="text-sm text-[var(--color-muted-foreground)]">
            The hardware/software targets in this Collection. Pick one to
            review or edit.
          </p>
        </div>
        {canCreate && (
          <Button
            size="sm"
            onClick={() => setDialogOpen(true)}
            data-testid="new-asset-button"
          >
            <Plus className="size-4" /> New Asset
          </Button>
        )}
      </CardHeader>

      <CardContent className="space-y-4">
        <div className="flex items-center gap-2">
          <div className="relative w-full max-w-md">
            <Search className="pointer-events-none absolute left-2 top-1/2 size-4 -translate-y-1/2 text-[var(--color-muted-foreground)]" />
            <Input
              placeholder="Search by name…"
              className="pl-8"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              data-testid="assets-search"
            />
          </div>
          {assets.isFetching && !assets.isLoading && (
            <Loader2 className="size-4 animate-spin text-[var(--color-muted-foreground)]" />
          )}
        </div>

        <div className="overflow-x-auto rounded-md border border-[var(--color-border)]">
          <table className="w-full text-sm" data-testid="assets-table">
            <thead className="bg-[var(--color-muted)]/30 text-left text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
              <tr>
                <th className="px-4 py-2">Name</th>
                <th className="px-4 py-2">FQDN</th>
                <th className="px-4 py-2">IP</th>
                <th className="px-4 py-2 text-right">STIGs</th>
              </tr>
            </thead>
            <tbody>
              {assets.isLoading && (
                <tr>
                  <td
                    className="px-4 py-6 text-center text-[var(--color-muted-foreground)]"
                    colSpan={4}
                  >
                    Loading assets…
                  </td>
                </tr>
              )}
              {assets.isError && (
                <tr>
                  <td className="px-4 py-6 text-center text-red-500" colSpan={4}>
                    Failed to load assets:{' '}
                    {(assets.error as Error | null)?.message ?? 'Unknown error.'}
                  </td>
                </tr>
              )}
              {assets.data && assets.data.length === 0 && (
                <tr>
                  <td
                    className="px-4 py-6 text-center text-[var(--color-muted-foreground)]"
                    colSpan={4}
                  >
                    {debounced
                      ? 'No assets match that filter.'
                      : 'No assets yet. Create one to get started.'}
                  </td>
                </tr>
              )}
              {assets.data?.map((a) => (
                <tr
                  key={a.assetId}
                  className="border-t border-[var(--color-border)] hover:bg-[var(--color-muted)]/20"
                  data-testid={`asset-row-${a.assetId}`}
                >
                  <td className="px-4 py-2">
                    <Link
                      to={`/collections/${collectionId}/assets/${a.assetId}`}
                      className="text-sky-500 hover:underline"
                      data-testid={`asset-link-${a.assetId}`}
                    >
                      {a.name}
                    </Link>
                  </td>
                  <td className="px-4 py-2 text-[var(--color-muted-foreground)]">
                    {a.fqdn ?? '\u2014'}
                  </td>
                  <td className="px-4 py-2 font-mono text-xs text-[var(--color-muted-foreground)]">
                    {a.ip ?? '\u2014'}
                  </td>
                  <td className="px-4 py-2 text-right text-[var(--color-muted-foreground)]">
                    {a.stigs?.length ?? 0}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </CardContent>

      <NewAssetDialog
        collectionId={collectionId}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
      />
    </Card>
  )
}
