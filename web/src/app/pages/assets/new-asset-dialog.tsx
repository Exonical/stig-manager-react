// New Asset dialog. Creates an Asset inside the given Collection and
// optionally maps it to a set of STIGs. The STIG picker shows every
// STIG already mapped into the Collection — assigning a STIG to an
// Asset requires the STIG to first be present in the Collection
// (upstream guards this with a 422).

import { Loader2 } from 'lucide-react'
import * as React from 'react'
import { useNavigate } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  useCollectionStigs,
  useCreateAsset,
  useUpdateAsset,
  type Asset,
  type AssetStig,
} from '@/lib/api/hooks'

interface NewAssetDialogProps {
  collectionId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  /** When provided, the dialog edits the given asset instead of creating a new one. */
  asset?: Asset
}

export function NewAssetDialog({
  collectionId,
  open,
  onOpenChange,
  asset,
}: NewAssetDialogProps) {
  const navigate = useNavigate()
  const create = useCreateAsset()
  const update = useUpdateAsset()
  const stigs = useCollectionStigs(collectionId)

  const editing = Boolean(asset)

  const [name, setName] = React.useState('')
  const [description, setDescription] = React.useState('')
  const [fqdn, setFqdn] = React.useState('')
  const [ip, setIp] = React.useState('')
  const [mac, setMac] = React.useState('')
  const [noncomputing, setNoncomputing] = React.useState(false)
  const [selectedStigs, setSelectedStigs] = React.useState<Set<string>>(
    new Set(),
  )
  const [error, setError] = React.useState<string | null>(null)

  // Re-seed form state whenever the dialog opens (or the target asset changes).
  React.useEffect(() => {
    if (!open) return
    setError(null)
    create.reset()
    update.reset()
    if (asset) {
      setName(asset.name)
      setDescription(asset.description ?? '')
      setFqdn(asset.fqdn ?? '')
      setIp(asset.ip ?? '')
      setMac(asset.mac ?? '')
      setNoncomputing(Boolean(asset.noncomputing))
      setSelectedStigs(
        new Set((asset.stigs ?? []).map((s: AssetStig) => s.benchmarkId)),
      )
    } else {
      setName('')
      setDescription('')
      setFqdn('')
      setIp('')
      setMac('')
      setNoncomputing(false)
      setSelectedStigs(new Set())
    }
    // create/update mutation refs are stable; intentionally omitted.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, asset])

  function toggleStig(id: string) {
    setSelectedStigs((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!name.trim()) {
      setError('Name is required.')
      return
    }
    // Trim and nullify optional/empty fields so we don't trip the
    // API's nullable-string format guards (empty string fails the
    // ip/fqdn/mac patterns).
    const body = {
      collectionId,
      name: name.trim(),
      description: description.trim(),
      fqdn: fqdn.trim() || null,
      ip: ip.trim() || null,
      mac: mac.trim() || null,
      noncomputing,
      metadata: {},
      stigs: Array.from(selectedStigs),
    }
    try {
      if (asset) {
        await update.mutateAsync({ assetId: asset.assetId, input: body })
        onOpenChange(false)
      } else {
        const created = await create.mutateAsync(body)
        navigate(`/collections/${collectionId}/assets/${created.assetId}`)
        onOpenChange(false)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save asset.')
    }
  }

  const busy = create.isPending || update.isPending || stigs.isLoading

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-2xl"
        data-testid={editing ? 'edit-asset-dialog' : 'new-asset-dialog'}
      >
        <DialogHeader>
          <DialogTitle>{editing ? 'Edit Asset' : 'New Asset'}</DialogTitle>
          <DialogDescription>
            {editing
              ? 'Update the Asset\u2019s metadata and STIG mappings.'
              : 'Add a new Asset to this Collection and (optionally) map STIGs to it.'}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="grid gap-3 md:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="asset-name">Name</Label>
              <Input
                id="asset-name"
                autoFocus
                required
                maxLength={255}
                value={name}
                onChange={(e) => setName(e.target.value)}
                data-testid="asset-name-input"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="asset-fqdn">FQDN</Label>
              <Input
                id="asset-fqdn"
                maxLength={255}
                value={fqdn}
                onChange={(e) => setFqdn(e.target.value)}
                placeholder="host.example.com"
                data-testid="asset-fqdn-input"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="asset-ip">IP</Label>
              <Input
                id="asset-ip"
                maxLength={255}
                value={ip}
                onChange={(e) => setIp(e.target.value)}
                placeholder="10.0.0.1"
                data-testid="asset-ip-input"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="asset-mac">MAC</Label>
              <Input
                id="asset-mac"
                maxLength={255}
                value={mac}
                onChange={(e) => setMac(e.target.value)}
                placeholder="aa:bb:cc:dd:ee:ff"
                data-testid="asset-mac-input"
              />
            </div>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="asset-description">Description</Label>
            <Textarea
              id="asset-description"
              maxLength={255}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              data-testid="asset-description-input"
            />
          </div>

          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={noncomputing}
              onChange={(e) => setNoncomputing(e.target.checked)}
              data-testid="asset-noncomputing-input"
            />
            Non-computing asset (no reviews; metadata-only)
          </label>

          <div className="space-y-1.5">
            <Label>STIG assignments</Label>
            <div
              className="max-h-48 overflow-y-auto rounded-md border border-[var(--color-border)] p-2"
              data-testid="asset-stigs-picker"
            >
              {stigs.isLoading && (
                <p className="text-sm text-[var(--color-muted-foreground)]">
                  Loading mapped STIGs…
                </p>
              )}
              {stigs.data && stigs.data.length === 0 && (
                <p className="text-sm text-[var(--color-muted-foreground)]">
                  No STIGs are mapped in this Collection yet. Use the STIG
                  Library (M18g) to import benchmarks.
                </p>
              )}
              {stigs.data?.map((s) => (
                <label
                  key={s.benchmarkId}
                  className="flex items-center gap-2 py-1 text-sm"
                >
                  <input
                    type="checkbox"
                    checked={selectedStigs.has(s.benchmarkId)}
                    onChange={() => toggleStig(s.benchmarkId)}
                    data-testid={`asset-stig-${s.benchmarkId}`}
                  />
                  <span className="font-mono text-xs">{s.benchmarkId}</span>
                  {s.revisionStr && (
                    <span className="text-xs text-[var(--color-muted-foreground)]">
                      {s.revisionStr}
                    </span>
                  )}
                </label>
              ))}
            </div>
          </div>

          {error && (
            <p className="text-sm text-red-500" data-testid="new-asset-error">
              {error}
            </p>
          )}

          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="ghost" disabled={busy}>
                Cancel
              </Button>
            </DialogClose>
            <Button
              type="submit"
              disabled={busy}
              data-testid={editing ? 'edit-asset-submit' : 'new-asset-submit'}
            >
              {busy ? (
                <>
                  <Loader2 className="size-4 animate-spin" />{' '}
                  {editing ? 'Saving\u2026' : 'Creating\u2026'}
                </>
              ) : editing ? (
                'Save changes'
              ) : (
                'Create Asset'
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
