// Metrics tab inside the Collection detail page. Surfaces the
// `/collections/{cid}/metrics/summary*` endpoint family in three
// stacked panels: a collection-wide KPI strip, a per-asset table, and
// a per-STIG table. Everything is read-only, scoped to
// `stig-manager:collection:read`. Lower roles still see the metrics
// because the API allows it; the form controls don't exist here.

import { Loader2 } from 'lucide-react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  useMetricsByAsset,
  useMetricsByStig,
  useMetricsCollection,
  type MetricsSummaryAggAsset,
  type MetricsSummaryAggStig,
} from '@/lib/api/hooks'
import type { MetricsSummary } from '@/lib/api'

interface MetricsTabProps {
  collectionId: string
}

export function MetricsTab({ collectionId }: MetricsTabProps) {
  const total = useMetricsCollection(collectionId)
  const byAsset = useMetricsByAsset(collectionId)
  const byStig = useMetricsByStig(collectionId)

  return (
    <div className="space-y-4" data-testid="collection-metrics-tab">
      <Card>
        <CardHeader>
          <CardTitle>Collection metrics</CardTitle>
          <p className="text-sm text-[var(--color-muted-foreground)]">
            Aggregate counts across all assets and STIGs in this Collection.
            Severity counts are independent of result counts — for example, an
            unassessed High-severity rule contributes to both
            <em>{' '}assessmentsBySeverity.high{' '}</em>
            (denominator) and <em>findings.high</em> when failed.
          </p>
        </CardHeader>
        <CardContent>
          {total.isLoading ? (
            <Spinner label="Loading totals…" />
          ) : total.isError ? (
            <p className="text-sm text-red-500" data-testid="metrics-error">
              {(total.error as Error).message}
            </p>
          ) : total.data ? (
            <div
              className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4"
              data-testid="metrics-kpi-grid"
            >
              <Kpi label="Assets" value={total.data.assets} />
              <Kpi label="STIGs" value={total.data.stigs} />
              <Kpi label="Checklists" value={total.data.checklists} />
              <Kpi
                label="Assessments"
                value={total.data.metrics.assessments}
              />
              <Kpi label="Assessed" value={total.data.metrics.assessed} />
              <Kpi
                label="Findings (high)"
                value={total.data.metrics.findings.high}
              />
              <Kpi
                label="Findings (medium)"
                value={total.data.metrics.findings.medium}
              />
              <Kpi
                label="Findings (low)"
                value={total.data.metrics.findings.low}
              />
              <ResultCounts metrics={total.data.metrics} />
              <StatusCounts metrics={total.data.metrics} />
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>By asset</CardTitle>
          <p className="text-sm text-[var(--color-muted-foreground)]">
            One row per asset. Findings shown as high / medium / low.
          </p>
        </CardHeader>
        <CardContent>
          {byAsset.isLoading ? (
            <Spinner label="Loading per-asset metrics…" />
          ) : byAsset.isError ? (
            <p className="text-sm text-red-500">
              {(byAsset.error as Error).message}
            </p>
          ) : (byAsset.data ?? []).length === 0 ? (
            <p
              className="text-sm text-[var(--color-muted-foreground)]"
              data-testid="metrics-by-asset-empty"
            >
              No assets mapped to STIGs yet.
            </p>
          ) : (
            <AssetTable rows={byAsset.data ?? []} />
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>By STIG</CardTitle>
          <p className="text-sm text-[var(--color-muted-foreground)]">
            One row per benchmark mapped to at least one asset.
          </p>
        </CardHeader>
        <CardContent>
          {byStig.isLoading ? (
            <Spinner label="Loading per-STIG metrics…" />
          ) : byStig.isError ? (
            <p className="text-sm text-red-500">
              {(byStig.error as Error).message}
            </p>
          ) : (byStig.data ?? []).length === 0 ? (
            <p
              className="text-sm text-[var(--color-muted-foreground)]"
              data-testid="metrics-by-stig-empty"
            >
              No STIGs mapped to assets in this Collection.
            </p>
          ) : (
            <StigTable rows={byStig.data ?? []} />
          )}
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

function Kpi({ label, value }: { label: string; value: number }) {
  return (
    <div
      className="rounded border border-[var(--color-border)] p-3"
      data-testid={`metrics-kpi-${slug(label)}`}
    >
      <div className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
        {label}
      </div>
      <div className="mt-1 text-2xl font-semibold">{value}</div>
    </div>
  )
}

function ResultCounts({ metrics }: { metrics: MetricsSummary['metrics'] }) {
  return (
    <div
      className="col-span-2 rounded border border-[var(--color-border)] p-3 sm:col-span-3 md:col-span-2"
      data-testid="metrics-result-counts"
    >
      <div className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
        Results
      </div>
      <dl className="mt-1 grid grid-cols-4 gap-2 text-xs">
        <Stat label="pass" value={metrics.results.pass} />
        <Stat label="fail" value={metrics.results.fail} />
        <Stat label="n/a" value={metrics.results.notapplicable} />
        <Stat label="other" value={metrics.results.other} />
      </dl>
    </div>
  )
}

function StatusCounts({ metrics }: { metrics: MetricsSummary['metrics'] }) {
  return (
    <div
      className="col-span-2 rounded border border-[var(--color-border)] p-3 sm:col-span-3 md:col-span-2"
      data-testid="metrics-status-counts"
    >
      <div className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
        Statuses
      </div>
      <dl className="mt-1 grid grid-cols-4 gap-2 text-xs">
        <Stat label="saved" value={metrics.statuses.saved} />
        <Stat label="submitted" value={metrics.statuses.submitted} />
        <Stat label="accepted" value={metrics.statuses.accepted} />
        <Stat label="rejected" value={metrics.statuses.rejected} />
      </dl>
    </div>
  )
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <dt className="text-[var(--color-muted-foreground)]">{label}</dt>
      <dd className="font-mono text-sm">{value}</dd>
    </div>
  )
}

function AssetTable({ rows }: { rows: MetricsSummaryAggAsset[] }) {
  return (
    <div className="overflow-x-auto">
      <table
        className="min-w-full text-sm"
        data-testid="metrics-by-asset-table"
      >
        <thead className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
          <tr className="border-b border-[var(--color-border)]">
            <th className="px-2 py-1 text-left">Asset</th>
            <th className="px-2 py-1 text-right">STIGs</th>
            <th className="px-2 py-1 text-right">Assessed</th>
            <th className="px-2 py-1 text-right">Findings (H/M/L)</th>
            <th className="px-2 py-1 text-right">Pass</th>
            <th className="px-2 py-1 text-right">Fail</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr
              key={r.assetId}
              className="border-b border-[var(--color-border)]/60"
              data-testid={`metrics-by-asset-row-${r.assetId}`}
            >
              <td className="px-2 py-1">{r.name}</td>
              <td className="px-2 py-1 text-right font-mono">
                {r.benchmarkIds.length}
              </td>
              <td className="px-2 py-1 text-right font-mono">
                {r.metrics.assessed}
              </td>
              <td className="px-2 py-1 text-right font-mono">
                {r.metrics.findings.high}/{r.metrics.findings.medium}/
                {r.metrics.findings.low}
              </td>
              <td className="px-2 py-1 text-right font-mono">
                {r.metrics.results.pass}
              </td>
              <td className="px-2 py-1 text-right font-mono">
                {r.metrics.results.fail}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function StigTable({ rows }: { rows: MetricsSummaryAggStig[] }) {
  return (
    <div className="overflow-x-auto">
      <table className="min-w-full text-sm" data-testid="metrics-by-stig-table">
        <thead className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
          <tr className="border-b border-[var(--color-border)]">
            <th className="px-2 py-1 text-left">Benchmark</th>
            <th className="px-2 py-1 text-right">Assets</th>
            <th className="px-2 py-1 text-right">Rules</th>
            <th className="px-2 py-1 text-right">Findings (H/M/L)</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr
              key={r.benchmarkId}
              className="border-b border-[var(--color-border)]/60"
              data-testid={`metrics-by-stig-row-${r.benchmarkId}`}
            >
              <td className="px-2 py-1">
                <span className="font-mono text-xs">{r.benchmarkId}</span>
                {r.title && (
                  <span className="ml-2 text-[var(--color-muted-foreground)]">
                    {r.title}
                  </span>
                )}
              </td>
              <td className="px-2 py-1 text-right font-mono">{r.assets}</td>
              <td className="px-2 py-1 text-right font-mono">
                {r.ruleCount ?? '\u2014'}
              </td>
              <td className="px-2 py-1 text-right font-mono">
                {r.metrics.findings.high}/{r.metrics.findings.medium}/
                {r.metrics.findings.low}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function slug(s: string): string {
  return s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '')
}
