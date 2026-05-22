// POAM tab inside the Collection detail page. Wraps the
// `GET /collections/{cid}/poam` endpoint which streams an xlsx
// spreadsheet for either the EMASS or MCCAST template. The form
// mirrors the spec's query parameters: aggregator, format,
// acceptedOnly, benchmarkId, assetId, date, office, status, and the
// mccast-specific mccastPackageId / mccastAuthName.

import { Download, Loader2 } from 'lucide-react'
import * as React from 'react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useAssets, useCollectionStigs } from '@/lib/api/hooks'
import {
  downloadPoam,
  type PoamAggregator,
  type PoamFormat,
} from '@/lib/api'

interface PoamTabProps {
  collectionId: string
}

export function PoamTab({ collectionId }: PoamTabProps) {
  const assets = useAssets({ collectionId })
  const stigs = useCollectionStigs(collectionId)

  const [aggregator, setAggregator] = React.useState<PoamAggregator>('groupId')
  const [format, setFormat] = React.useState<PoamFormat>('emass')
  const [acceptedOnly, setAcceptedOnly] = React.useState(false)
  const [benchmarkId, setBenchmarkId] = React.useState('')
  const [assetId, setAssetId] = React.useState('')
  const [date, setDate] = React.useState('')
  const [office, setOffice] = React.useState('')
  const [status, setStatus] = React.useState('')
  const [mccastPackageId, setMccastPackageId] = React.useState('')
  const [mccastAuthName, setMccastAuthName] = React.useState('')

  const [pending, setPending] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)
  const [success, setSuccess] = React.useState<string | null>(null)

  async function onDownload() {
    setError(null)
    setSuccess(null)
    setPending(true)
    try {
      await downloadPoam(collectionId, {
        aggregator,
        format,
        acceptedOnly: acceptedOnly || undefined,
        benchmarkId: benchmarkId || undefined,
        assetId: assetId || undefined,
        date: date || undefined,
        office: office || undefined,
        status: status || undefined,
        mccastPackageId: mccastPackageId || undefined,
        mccastAuthName: mccastAuthName || undefined,
      })
      setSuccess(
        `Downloaded ${format.toUpperCase()} POAM (${aggregator} aggregation).`,
      )
    } catch (err) {
      setError(err instanceof Error ? err.message : 'POAM download failed.')
    } finally {
      setPending(false)
    }
  }

  return (
    <div className="space-y-4" data-testid="collection-poam-tab">
      <Card>
        <CardHeader>
          <CardTitle>POA&amp;M</CardTitle>
          <p className="text-sm text-[var(--color-muted-foreground)]">
            Generates a Plan of Action &amp; Milestones spreadsheet in xlsx
            format. Findings are aggregated by either{' '}
            <span className="font-medium">groupId</span> (V-…) or{' '}
            <span className="font-medium">ruleId</span> (SV-…). The
            downloaded file uses either the EMASS or MCCAST template.
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <div>
              <Label htmlFor="poam-aggregator">Aggregator</Label>
              <select
                id="poam-aggregator"
                value={aggregator}
                onChange={(e) =>
                  setAggregator(e.target.value as PoamAggregator)
                }
                className="mt-1 w-full rounded border border-[var(--color-border)] bg-transparent px-2 py-1 text-sm"
                data-testid="poam-aggregator"
              >
                <option value="groupId">Group ID (V-…)</option>
                <option value="ruleId">Rule ID (SV-…)</option>
              </select>
            </div>
            <div>
              <Label htmlFor="poam-format">Template</Label>
              <select
                id="poam-format"
                value={format}
                onChange={(e) => setFormat(e.target.value as PoamFormat)}
                className="mt-1 w-full rounded border border-[var(--color-border)] bg-transparent px-2 py-1 text-sm"
                data-testid="poam-format"
              >
                <option value="emass">EMASS</option>
                <option value="mccast">MCCAST</option>
              </select>
            </div>
            <div>
              <Label htmlFor="poam-benchmark">Benchmark</Label>
              <select
                id="poam-benchmark"
                value={benchmarkId}
                onChange={(e) => setBenchmarkId(e.target.value)}
                className="mt-1 w-full rounded border border-[var(--color-border)] bg-transparent px-2 py-1 text-sm"
                data-testid="poam-benchmark"
              >
                <option value="">All benchmarks</option>
                {(stigs.data ?? []).map((s) => (
                  <option key={s.benchmarkId} value={s.benchmarkId}>
                    {s.benchmarkId}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <Label htmlFor="poam-asset">Asset</Label>
              <select
                id="poam-asset"
                value={assetId}
                onChange={(e) => setAssetId(e.target.value)}
                className="mt-1 w-full rounded border border-[var(--color-border)] bg-transparent px-2 py-1 text-sm"
                data-testid="poam-asset"
              >
                <option value="">All assets</option>
                {(assets.data ?? []).map((a) => (
                  <option key={a.assetId} value={a.assetId}>
                    {a.name}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <div>
              <Label htmlFor="poam-date">Date (MM/DD/YYYY)</Label>
              <Input
                id="poam-date"
                value={date}
                onChange={(e) => setDate(e.target.value)}
                placeholder="01/15/2026"
                data-testid="poam-date"
              />
            </div>
            <div>
              <Label htmlFor="poam-office">Office / Org</Label>
              <Input
                id="poam-office"
                value={office}
                onChange={(e) => setOffice(e.target.value)}
                data-testid="poam-office"
              />
            </div>
            <div>
              <Label htmlFor="poam-status">Status</Label>
              <Input
                id="poam-status"
                value={status}
                onChange={(e) => setStatus(e.target.value)}
                placeholder="Ongoing"
                data-testid="poam-status"
              />
            </div>
            <div className="flex items-end gap-2 pb-1">
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={acceptedOnly}
                  onChange={(e) => setAcceptedOnly(e.target.checked)}
                  data-testid="poam-accepted-only"
                />
                <span>Accepted only</span>
              </label>
            </div>
          </div>

          {format === 'mccast' && (
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
                <Label htmlFor="poam-mccast-pkg">MCCAST Package ID</Label>
                <Input
                  id="poam-mccast-pkg"
                  value={mccastPackageId}
                  onChange={(e) => setMccastPackageId(e.target.value)}
                  data-testid="poam-mccast-package-id"
                />
              </div>
              <div>
                <Label htmlFor="poam-mccast-auth">MCCAST Auth Name</Label>
                <Input
                  id="poam-mccast-auth"
                  value={mccastAuthName}
                  onChange={(e) => setMccastAuthName(e.target.value)}
                  data-testid="poam-mccast-auth-name"
                />
              </div>
            </div>
          )}

          {error && (
            <p className="text-sm text-red-500" data-testid="poam-error">
              {error}
            </p>
          )}
          {success && (
            <p
              className="text-sm text-emerald-500"
              data-testid="poam-success"
            >
              {success}
            </p>
          )}

          <Button
            type="button"
            onClick={onDownload}
            disabled={pending}
            data-testid="poam-download-button"
          >
            {pending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Download className="size-4" />
            )}
            <span className="ml-2">Download POAM</span>
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
