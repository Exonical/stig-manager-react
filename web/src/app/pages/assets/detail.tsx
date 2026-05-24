// /collections/:collectionId/assets/:assetId
//
// Asset detail. Header shows metadata (name, FQDN, IP, MAC, description,
// noncomputing flag). Below: per-STIG rule list. Each STIG row expands
// to show its rules + each rule's current review result. Clicking a
// rule navigates to the single-rule review editor.

import { ArrowLeft, ClipboardCheck, Loader2, Pencil, Trash2 } from 'lucide-react'
import * as React from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'

import { NewAssetDialog } from './new-asset-dialog'
import { RuleList } from './rule-list'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  useAsset,
  useAssetStigs,
  useCollection,
  useCurrentUser,
  useDeleteAsset,
  useReviewsByAsset,
} from '@/lib/api/hooks'
import { atLeast, roleForCollection } from '@/lib/auth/roles'

export function AssetDetailPage() {
  const { collectionId, assetId } = useParams<{
    collectionId: string
    assetId: string
  }>()
  const navigate = useNavigate()
  const asset = useAsset(assetId)
  const stigs = useAssetStigs(assetId)
  const reviews = useReviewsByAsset(collectionId, assetId)
  const me = useCurrentUser()
  const collection = useCollection(collectionId)
  const role = roleForCollection(me.data, collectionId ?? '')
  const canEdit = atLeast(role, 2)

  const deleteAsset = useDeleteAsset()

  const [editOpen, setEditOpen] = React.useState(false)
  const [confirmDelete, setConfirmDelete] = React.useState(false)
  const [deleteError, setDeleteError] = React.useState<string | null>(null)

  const reviewsByRule = React.useMemo(() => {
    const map = new Map<string, { result: string; status?: string }>()
    for (const r of reviews.data ?? []) {
      const ruleId = r.ruleId ?? r.ruleIds?.[0]
      if (!ruleId) continue
      map.set(ruleId, {
        result: r.result,
        status: r.status?.label,
      })
    }
    return map
  }, [reviews.data])

  if (asset.isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
        <Loader2 className="size-4 animate-spin" /> Loading asset…
      </div>
    )
  }
  if (asset.isError || !asset.data) {
    return (
      <div className="space-y-3" data-testid="asset-not-found">
        <Link
          to={`/collections/${collectionId}`}
          className="inline-flex items-center gap-1 text-sm text-sky-500 hover:underline"
        >
          <ArrowLeft className="size-4" /> Back to Collection
        </Link>
        <p className="text-sm text-red-500">
          Failed to load asset {assetId}:{' '}
          {(asset.error as Error | null)?.message ?? 'Not found.'}
        </p>
      </div>
    )
  }

  async function handleDelete() {
    if (!asset.data) return
    setDeleteError(null)
    try {
      await deleteAsset.mutateAsync({
        assetId: asset.data.assetId,
        collectionId,
      })
      navigate(`/collections/${collectionId}`)
    } catch (err) {
      setDeleteError(
        err instanceof Error ? err.message : 'Failed to delete asset.',
      )
    }
  }

  const a = asset.data
  return (
    <div className="space-y-6" data-testid="asset-detail-page">
      <Link
        to={`/collections/${collectionId}`}
        className="inline-flex items-center gap-1 text-sm text-sky-500 hover:underline"
      >
        <ArrowLeft className="size-4" />{' '}
        Back to {collection.data?.name ?? 'Collection'}
      </Link>

      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">{a.name}</h1>
          {a.description && (
            <p className="mt-1 max-w-2xl text-sm text-[var(--color-muted-foreground)]">
              {a.description}
            </p>
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            asChild
            size="sm"
            data-testid="asset-open-workspace"
          >
            <Link
              to={`/collections/${collectionId}/assets/${assetId}/workspace`}
            >
              <ClipboardCheck className="size-4" /> Open Review Workspace
            </Link>
          </Button>
          {canEdit && (
            <>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setEditOpen(true)}
                data-testid="asset-edit-button"
              >
                <Pencil className="size-4" /> Edit
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setConfirmDelete(true)}
                data-testid="asset-delete-button"
              >
                <Trash2 className="size-4" /> Delete
              </Button>
            </>
          )}
        </div>
      </header>

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Identity</CardTitle>
            <CardDescription>Network identifiers for this asset.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <Field label="Asset ID" value={a.assetId} mono />
            <Field label="FQDN" value={a.fqdn ?? '\u2014'} />
            <Field label="IP" value={a.ip ?? '\u2014'} mono />
            <Field label="MAC" value={a.mac ?? '\u2014'} mono />
            <Field
              label="Computing"
              value={a.noncomputing ? 'No (metadata-only)' : 'Yes'}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>STIG mappings</CardTitle>
            <CardDescription>
              The benchmarks assigned to this Asset and their effective revision.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            {stigs.isLoading && (
              <p className="text-[var(--color-muted-foreground)]">Loading…</p>
            )}
            {stigs.data && stigs.data.length === 0 && (
              <p className="text-[var(--color-muted-foreground)]">
                No STIGs mapped. Edit the asset to add one.
              </p>
            )}
            {stigs.data?.map((s) => (
              <div
                key={s.benchmarkId}
                className="flex items-center justify-between gap-2"
              >
                <span className="font-mono text-xs">{s.benchmarkId}</span>
                <span className="text-xs text-[var(--color-muted-foreground)]">
                  {s.revisionStr ?? ''} · {s.ruleCount ?? 0} rules
                  {s.revisionPinned ? ' · pinned' : ''}
                </span>
              </div>
            ))}
          </CardContent>
        </Card>
      </div>

      <section className="space-y-4">
        <h2 className="text-lg font-semibold">Rules</h2>
        <p className="text-sm text-[var(--color-muted-foreground)]">
          Pick a STIG to inspect its rules and review findings.
        </p>
        {stigs.data?.map((s) => (
          <RuleList
            key={s.benchmarkId}
            benchmarkId={s.benchmarkId}
            revisionStr={s.revisionStr ?? 'latest'}
            collectionId={collectionId ?? ''}
            assetId={assetId ?? ''}
            reviewsByRule={reviewsByRule}
          />
        ))}
        {stigs.data && stigs.data.length === 0 && (
          <p className="text-sm text-[var(--color-muted-foreground)]">
            No STIGs mapped — nothing to review yet.
          </p>
        )}
      </section>

      {canEdit && (
        <NewAssetDialog
          collectionId={collectionId ?? ''}
          open={editOpen}
          onOpenChange={setEditOpen}
          asset={a}
        />
      )}

      {confirmDelete && (
        <div className="fixed inset-0 z-50 grid place-items-center bg-black/40">
          <div
            className="w-full max-w-md space-y-3 rounded-md border border-[var(--color-border)] bg-[var(--color-card)] p-6"
            data-testid="asset-delete-confirm"
          >
            <h3 className="text-base font-semibold">Delete asset?</h3>
            <p className="text-sm text-[var(--color-muted-foreground)]">
              This permanently deletes {a.name} and all of its reviews. This
              action cannot be undone.
            </p>
            {deleteError && (
              <p className="text-sm text-red-500">{deleteError}</p>
            )}
            <div className="flex justify-end gap-2">
              <Button
                variant="ghost"
                onClick={() => setConfirmDelete(false)}
                disabled={deleteAsset.isPending}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={handleDelete}
                disabled={deleteAsset.isPending}
                data-testid="asset-delete-confirm-button"
              >
                {deleteAsset.isPending ? (
                  <>
                    <Loader2 className="size-4 animate-spin" /> Deleting…
                  </>
                ) : (
                  'Delete'
                )}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function Field({
  label,
  value,
  mono = false,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="flex justify-between gap-3">
      <span className="text-[var(--color-muted-foreground)]">{label}</span>
      <span className={mono ? 'font-mono text-xs' : 'text-right'}>{value}</span>
    </div>
  )
}
