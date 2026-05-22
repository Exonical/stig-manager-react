// Exports tab inside the Collection detail page. Wraps the three
// `/collections/{cid}/archive/{ckl|cklb|xccdf}` endpoints in one
// form. The user picks an asset subset and a format; the API streams
// a ZIP that we trigger as a browser download. The default selection
// is "all assets in the Collection"; omitting per-asset `stigs`
// requests the default revisions of every benchmark mapped to the
// asset (and visible to the caller).

import { Download, Loader2 } from 'lucide-react'
import * as React from 'react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { useAssets } from '@/lib/api/hooks'
import {
  downloadCklArchive,
  downloadCklbArchive,
  downloadXccdfArchive,
  type AssetStigSelection,
  type CklMode,
} from '@/lib/api'

interface ExportsTabProps {
  collectionId: string
}

type Format = 'ckl' | 'cklb' | 'xccdf'

export function ExportsTab({ collectionId }: ExportsTabProps) {
  const assets = useAssets({ collectionId })

  const [selected, setSelected] = React.useState<Set<string>>(new Set())
  const [format, setFormat] = React.useState<Format>('ckl')
  const [mode, setMode] = React.useState<CklMode>('mono')
  const [pending, setPending] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)
  const [lastDownload, setLastDownload] = React.useState<string | null>(null)

  function toggleAll(checked: boolean) {
    if (!checked) {
      setSelected(new Set())
      return
    }
    setSelected(new Set((assets.data ?? []).map((a) => a.assetId)))
  }

  function toggleOne(assetId: string, checked: boolean) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (checked) next.add(assetId)
      else next.delete(assetId)
      return next
    })
  }

  async function onDownload() {
    setError(null)
    setLastDownload(null)
    const list = assets.data ?? []
    const effective = selected.size === 0 ? list.map((a) => a.assetId) : Array.from(selected)
    if (effective.length === 0) {
      setError('No assets in this Collection to export.')
      return
    }
    const selections: AssetStigSelection[] = effective.map((assetId) => ({
      assetId,
    }))
    setPending(true)
    try {
      if (format === 'ckl') {
        await downloadCklArchive(collectionId, selections, mode)
      } else if (format === 'cklb') {
        await downloadCklbArchive(collectionId, selections, mode)
      } else {
        await downloadXccdfArchive(collectionId, selections)
      }
      setLastDownload(
        `Downloaded ${format.toUpperCase()} archive for ${effective.length} asset${effective.length === 1 ? '' : 's'}.`,
      )
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Download failed.')
    } finally {
      setPending(false)
    }
  }

  const allChecked =
    selected.size > 0 && selected.size === (assets.data ?? []).length

  return (
    <div className="space-y-4" data-testid="collection-exports-tab">
      <Card>
        <CardHeader>
          <CardTitle>Checklist exports</CardTitle>
          <p className="text-sm text-[var(--color-muted-foreground)]">
            Generates a ZIP containing one file per (asset, STIG) by default.
            If no assets are selected, every asset in the Collection is
            included. Switch to <span className="font-medium">mono</span>{' '}
            mode for a single combined file per asset (CKL / CKLB only).
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <Label>Format</Label>
              <div
                className="mt-1 inline-flex rounded-md border border-[var(--color-border)] p-0.5 text-xs"
                role="radiogroup"
                data-testid="export-format-toggle"
              >
                {(['ckl', 'cklb', 'xccdf'] as Format[]).map((f) => (
                  <button
                    key={f}
                    type="button"
                    role="radio"
                    aria-checked={format === f}
                    onClick={() => setFormat(f)}
                    className={`rounded px-3 py-1 uppercase tracking-wider ${
                      format === f
                        ? 'bg-[var(--color-primary)] text-[var(--color-primary-foreground)]'
                        : 'text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]'
                    }`}
                    data-testid={`export-format-${f}`}
                  >
                    {f}
                  </button>
                ))}
              </div>
            </div>

            {format !== 'xccdf' && (
              <div>
                <Label>Mode</Label>
                <div
                  className="mt-1 inline-flex rounded-md border border-[var(--color-border)] p-0.5 text-xs"
                  role="radiogroup"
                  data-testid="export-mode-toggle"
                >
                  {(['mono', 'multi'] as CklMode[]).map((m) => (
                    <button
                      key={m}
                      type="button"
                      role="radio"
                      aria-checked={mode === m}
                      onClick={() => setMode(m)}
                      className={`rounded px-3 py-1 uppercase tracking-wider ${
                        mode === m
                          ? 'bg-[var(--color-primary)] text-[var(--color-primary-foreground)]'
                          : 'text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]'
                      }`}
                    >
                      {m}
                    </button>
                  ))}
                </div>
              </div>
            )}
          </div>

          <div>
            <div className="flex items-center justify-between">
              <Label>Assets</Label>
              <label className="flex items-center gap-2 text-xs text-[var(--color-muted-foreground)]">
                <input
                  type="checkbox"
                  checked={allChecked}
                  onChange={(e) => toggleAll(e.target.checked)}
                  data-testid="export-select-all"
                />
                Select all
              </label>
            </div>
            <div className="mt-2 max-h-64 overflow-y-auto rounded border border-[var(--color-border)] p-2">
              {assets.isLoading ? (
                <Spinner label="Loading assets…" />
              ) : (assets.data ?? []).length === 0 ? (
                <p
                  className="text-sm text-[var(--color-muted-foreground)]"
                  data-testid="export-no-assets"
                >
                  No assets in this Collection yet.
                </p>
              ) : (
                <ul className="space-y-1">
                  {(assets.data ?? []).map((a) => (
                    <li key={a.assetId}>
                      <label className="flex items-center gap-2 text-sm">
                        <input
                          type="checkbox"
                          checked={selected.has(a.assetId)}
                          onChange={(e) =>
                            toggleOne(a.assetId, e.target.checked)
                          }
                          data-testid={`export-asset-${a.assetId}`}
                        />
                        <span>{a.name}</span>
                        <span className="text-xs text-[var(--color-muted-foreground)]">
                          {a.stigs?.length ?? 0} STIG
                          {(a.stigs?.length ?? 0) === 1 ? '' : 's'}
                        </span>
                      </label>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            <p className="mt-1 text-xs text-[var(--color-muted-foreground)]">
              Leave empty to export every asset.
            </p>
          </div>

          {error && (
            <p
              className="text-sm text-red-500"
              data-testid="export-error"
            >
              {error}
            </p>
          )}
          {lastDownload && (
            <p
              className="text-sm text-emerald-500"
              data-testid="export-success"
            >
              {lastDownload}
            </p>
          )}

          <Button
            type="button"
            onClick={onDownload}
            disabled={pending || (assets.data ?? []).length === 0}
            data-testid="export-download-button"
          >
            {pending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Download className="size-4" />
            )}
            <span className="ml-2">Download archive</span>
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}

function Spinner({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
      <Loader2 className="size-4 animate-spin" /> {label}
    </div>
  )
}
