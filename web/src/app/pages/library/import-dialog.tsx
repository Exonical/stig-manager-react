// Import a STIG XCCDF Benchmark via multipart upload.

import { Loader2 } from 'lucide-react'
import * as React from 'react'

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
import { Label } from '@/components/ui/label'
import {
  useImportBenchmark,
  type ImportBenchmarkResult,
} from '@/lib/api/hooks'

interface ImportStigDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ImportStigDialog({
  open,
  onOpenChange,
}: ImportStigDialogProps) {
  const importer = useImportBenchmark()
  const [file, setFile] = React.useState<File | null>(null)
  const [clobber, setClobber] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)
  const [result, setResult] = React.useState<ImportBenchmarkResult | null>(null)

  React.useEffect(() => {
    if (open) {
      setFile(null)
      setClobber(false)
      setError(null)
      setResult(null)
      importer.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const busy = importer.isPending

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setResult(null)
    if (!file) {
      setError('Choose an XCCDF file to import.')
      return
    }
    try {
      const r = await importer.mutateAsync({ file, clobber })
      setResult(r)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed.')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="import-stig-dialog">
        <DialogHeader>
          <DialogTitle>Import XCCDF Benchmark</DialogTitle>
          <DialogDescription>
            Upload a DISA XCCDF Benchmark — a raw{' '}
            <code>.xml</code> file, a DISA STIG{' '}
            <code>.zip</code> bundle (e.g.{' '}
            <code>U_RHEL_10_V1R1_STIG.zip</code>), or a STIG Library
            quarterly zip. Existing (benchmark, revision) pairs are
            rejected unless you enable clobber.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="import-file">XCCDF file or DISA zip</Label>
            <input
              id="import-file"
              type="file"
              accept=".xml,.xccdf,.zip,application/xml,text/xml,application/zip"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              className="block w-full text-sm text-[var(--color-foreground)] file:mr-3 file:rounded-md file:border file:border-[var(--color-border)] file:bg-[var(--color-card)] file:px-2 file:py-1 file:text-sm file:font-medium hover:file:bg-[var(--color-accent)]/40"
              data-testid="import-file-input"
            />
            {file && (
              <p className="text-xs text-[var(--color-muted-foreground)]">
                {file.name} · {(file.size / 1024).toFixed(1)} KiB
              </p>
            )}
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={clobber}
              onChange={(e) => setClobber(e.target.checked)}
              data-testid="import-clobber-checkbox"
            />
            Overwrite existing revision if present
          </label>
          {error && (
            <p className="text-sm text-red-500" data-testid="import-stig-error">
              {error}
            </p>
          )}
          {result && (
            <div
              className="rounded-md border border-[var(--color-border)] bg-[var(--color-card)]/40 p-3 text-sm"
              data-testid="import-stig-result"
            >
              <p className="font-semibold">
                Import succeeded
                {result.revisions.length > 1 && (
                  <span className="ml-2 text-xs text-[var(--color-muted-foreground)]">
                    ({result.revisions.length} revisions)
                  </span>
                )}
              </p>
              <ul className="space-y-1">
                {result.revisions.map((r, i) => (
                  <li key={`${r.benchmarkId}-${r.revisionStr}-${i}`} className="font-mono text-xs">
                    {r.benchmarkId} · revision {r.revisionStr}
                    {r.action ? ` · ${r.action}` : ''}
                  </li>
                ))}
              </ul>
            </div>
          )}
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="ghost" disabled={busy}>
                Close
              </Button>
            </DialogClose>
            <Button
              type="submit"
              disabled={busy || !file}
              data-testid="import-stig-submit"
            >
              {busy ? (
                <>
                  <Loader2 className="size-4 animate-spin" /> Importing…
                </>
              ) : (
                'Import'
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
