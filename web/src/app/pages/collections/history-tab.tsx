// Review-history tab inside the Collection detail page. Wraps the
// `/collections/{cid}/review-history` family of endpoints — list,
// stats (with optional per-asset projection), and the Manage-gated
// retention delete.

import { Loader2, Trash2 } from 'lucide-react'
import * as React from 'react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  useAssets,
  useDeleteReviewHistory,
  useReviewHistory,
  useReviewHistoryStats,
  type ReviewHistoryAsset,
  type ReviewHistoryFilters,
} from '@/lib/api/hooks'
import type { ReviewStatusLabel } from '@/lib/api'

interface HistoryTabProps {
  collectionId: string
  role: number | null
}

const STATUS_OPTIONS: { value: ReviewStatusLabel | ''; label: string }[] = [
  { value: '', label: 'Any status' },
  { value: 'saved', label: 'Saved' },
  { value: 'submitted', label: 'Submitted' },
  { value: 'accepted', label: 'Accepted' },
  { value: 'rejected', label: 'Rejected' },
]

export function HistoryTab({ collectionId, role }: HistoryTabProps) {
  const canManage = role !== null && role >= 3
  const assets = useAssets({ collectionId })

  const [assetId, setAssetId] = React.useState('')
  const [ruleId, setRuleId] = React.useState('')
  const [status, setStatus] = React.useState<ReviewStatusLabel | ''>('')
  const [startDate, setStartDate] = React.useState('')
  const [endDate, setEndDate] = React.useState('')

  const filters: ReviewHistoryFilters = React.useMemo(() => {
    const f: ReviewHistoryFilters = {}
    if (assetId) f.assetId = assetId
    if (ruleId) f.ruleId = ruleId.trim()
    if (status) f.status = status
    if (startDate) f.startDate = new Date(startDate).toISOString()
    if (endDate) f.endDate = new Date(endDate).toISOString()
    return f
  }, [assetId, ruleId, status, startDate, endDate])

  const stats = useReviewHistoryStats(collectionId, {
    projection: 'asset',
    ...filters,
  })
  const list = useReviewHistory(collectionId, filters)
  const remove = useDeleteReviewHistory()

  const [retention, setRetention] = React.useState('')
  const [deleteMessage, setDeleteMessage] = React.useState<string | null>(null)

  async function onDelete() {
    setDeleteMessage(null)
    if (!retention) {
      setDeleteMessage(
        'Retention date is required; otherwise the entire history would be erased.',
      )
      return
    }
    if (
      !confirm(
        `Delete history entries older than ${retention}${assetId ? ` for the selected asset` : ''}? This cannot be undone.`,
      )
    ) {
      return
    }
    try {
      const resp = await remove.mutateAsync({
        collectionId,
        input: {
          retentionDate: new Date(retention).toISOString(),
          ...(assetId ? { assetId } : {}),
        },
      })
      setDeleteMessage(
        `Removed ${resp.HistoryEntriesDeleted} history entr${resp.HistoryEntriesDeleted === 1 ? 'y' : 'ies'}.`,
      )
    } catch (err) {
      setDeleteMessage(
        err instanceof Error ? err.message : 'Delete failed.',
      )
    }
  }

  return (
    <div className="space-y-4" data-testid="collection-history-tab">
      <Card>
        <CardHeader>
          <CardTitle>Review history</CardTitle>
          <p className="text-sm text-[var(--color-muted-foreground)]">
            Audit trail of every review state-change. Filters apply to both
            the list and the stats panel below.
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
            <div>
              <Label htmlFor="hist-asset">Asset</Label>
              <select
                id="hist-asset"
                value={assetId}
                onChange={(e) => setAssetId(e.target.value)}
                className="mt-1 w-full rounded border border-[var(--color-border)] bg-transparent px-2 py-1 text-sm"
                data-testid="history-asset-select"
              >
                <option value="">Any asset</option>
                {(assets.data ?? []).map((a) => (
                  <option key={a.assetId} value={a.assetId}>
                    {a.name}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <Label htmlFor="hist-rule">Rule ID</Label>
              <Input
                id="hist-rule"
                value={ruleId}
                onChange={(e) => setRuleId(e.target.value)}
                placeholder="SV-…"
                data-testid="history-rule-input"
              />
            </div>
            <div>
              <Label htmlFor="hist-status">Status</Label>
              <select
                id="hist-status"
                value={status}
                onChange={(e) =>
                  setStatus(e.target.value as ReviewStatusLabel | '')
                }
                className="mt-1 w-full rounded border border-[var(--color-border)] bg-transparent px-2 py-1 text-sm"
                data-testid="history-status-select"
              >
                {STATUS_OPTIONS.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <Label htmlFor="hist-start">Start date</Label>
              <Input
                id="hist-start"
                type="date"
                value={startDate}
                onChange={(e) => setStartDate(e.target.value)}
                data-testid="history-start-input"
              />
            </div>
            <div>
              <Label htmlFor="hist-end">End date</Label>
              <Input
                id="hist-end"
                type="date"
                value={endDate}
                onChange={(e) => setEndDate(e.target.value)}
                data-testid="history-end-input"
              />
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Stats</CardTitle>
        </CardHeader>
        <CardContent>
          {stats.isLoading ? (
            <Spinner label="Loading stats…" />
          ) : stats.isError ? (
            <p className="text-sm text-red-500">
              {(stats.error as Error).message}
            </p>
          ) : stats.data ? (
            <div className="space-y-3" data-testid="history-stats">
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                <Kpi
                  label="Total entries"
                  value={String(stats.data.collectionHistoryEntryCount)}
                  testId="history-total-entries"
                />
                <Kpi
                  label="Oldest entry"
                  value={formatTs(stats.data.oldestHistoryEntryDate)}
                />
                <Kpi
                  label="Assets w/ history"
                  value={String(
                    (stats.data.assetHistoryEntryCounts ?? []).length,
                  )}
                />
              </div>
              {(stats.data.assetHistoryEntryCounts ?? []).length > 0 && (
                <details className="text-sm">
                  <summary className="cursor-pointer text-[var(--color-muted-foreground)]">
                    Per-asset breakdown
                  </summary>
                  <ul className="mt-2 space-y-1">
                    {(stats.data.assetHistoryEntryCounts ?? []).map((a) => (
                      <li
                        key={a.assetId}
                        className="font-mono text-xs"
                        data-testid={`history-asset-stat-${a.assetId}`}
                      >
                        {a.assetId}: {a.historyEntryCount ?? 0} entr
                        {a.historyEntryCount === 1 ? 'y' : 'ies'}
                        {a.oldestHistoryEntry
                          ? ` (oldest ${formatTs(a.oldestHistoryEntry)})`
                          : ''}
                      </li>
                    ))}
                  </ul>
                </details>
              )}
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Entries</CardTitle>
        </CardHeader>
        <CardContent>
          {list.isLoading ? (
            <Spinner label="Loading history…" />
          ) : list.isError ? (
            <p className="text-sm text-red-500">
              {(list.error as Error).message}
            </p>
          ) : (list.data ?? []).length === 0 ? (
            <p
              className="text-sm text-[var(--color-muted-foreground)]"
              data-testid="history-empty"
            >
              No history entries match the current filter.
            </p>
          ) : (
            <HistoryList rows={list.data ?? []} />
          )}
        </CardContent>
      </Card>

      {canManage && (
        <Card>
          <CardHeader>
            <CardTitle>Prune old history</CardTitle>
            <p className="text-sm text-[var(--color-muted-foreground)]">
              Removes review-history rows strictly older than the retention
              date. If an asset is selected above it scopes the delete to
              that asset only.
            </p>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
              <div>
                <Label htmlFor="hist-retention">Retention date</Label>
                <Input
                  id="hist-retention"
                  type="date"
                  value={retention}
                  onChange={(e) => setRetention(e.target.value)}
                  data-testid="history-retention-input"
                />
              </div>
              <div className="flex items-end">
                <Button
                  type="button"
                  variant="destructive"
                  onClick={onDelete}
                  disabled={remove.isPending}
                  data-testid="history-delete-button"
                >
                  {remove.isPending ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <Trash2 className="size-4" />
                  )}
                  <span className="ml-2">Delete older entries</span>
                </Button>
              </div>
            </div>
            {deleteMessage && (
              <p
                className="text-sm text-[var(--color-muted-foreground)]"
                data-testid="history-delete-message"
              >
                {deleteMessage}
              </p>
            )}
          </CardContent>
        </Card>
      )}
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

function Kpi({
  label,
  value,
  testId,
}: {
  label: string
  value: string
  testId?: string
}) {
  return (
    <div
      className="rounded border border-[var(--color-border)] p-3"
      data-testid={testId}
    >
      <div className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
        {label}
      </div>
      <div className="mt-1 text-lg font-semibold">{value}</div>
    </div>
  )
}

function HistoryList({ rows }: { rows: ReviewHistoryAsset[] }) {
  const flat = rows.flatMap((asset) =>
    asset.reviewHistories.flatMap((rule) =>
      rule.history.map((h) => ({
        ...h,
        assetId: asset.assetId,
        ruleId: rule.ruleId,
      })),
    ),
  )
  const sorted = [...flat].sort((a, b) => (a.ts < b.ts ? 1 : -1))
  return (
    <div className="overflow-x-auto">
      <table className="min-w-full text-sm" data-testid="history-table">
        <thead className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
          <tr className="border-b border-[var(--color-border)]">
            <th className="px-2 py-1 text-left">When</th>
            <th className="px-2 py-1 text-left">Asset</th>
            <th className="px-2 py-1 text-left">Rule</th>
            <th className="px-2 py-1 text-left">Result</th>
            <th className="px-2 py-1 text-left">Status</th>
            <th className="px-2 py-1 text-left">User</th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((r, i) => (
            <tr
              key={`${r.assetId}-${r.ruleId}-${r.ts}-${i}`}
              className="border-b border-[var(--color-border)]/60"
            >
              <td className="whitespace-nowrap px-2 py-1 font-mono text-xs">
                {formatTs(r.ts)}
              </td>
              <td className="px-2 py-1 font-mono text-xs">{r.assetId}</td>
              <td className="px-2 py-1 font-mono text-xs">{r.ruleId}</td>
              <td className="px-2 py-1">{r.result}</td>
              <td className="px-2 py-1">{r.status?.label ?? '\u2014'}</td>
              <td className="px-2 py-1">{r.username ?? r.userId ?? '\u2014'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function formatTs(ts: string | null | undefined): string {
  if (!ts) return '\u2014'
  try {
    return new Date(ts).toLocaleString()
  } catch {
    return ts
  }
}
