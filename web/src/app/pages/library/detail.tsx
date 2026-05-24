// /library/:benchmarkId — STIG detail page.
//
// Shows the metadata projection for a single benchmark, a revision
// picker, and a paginated table of rules for the selected revision.
// Clicking a rule row opens a side panel with the full lookup
// projection (Vulnerability Discussion / Check / Fix / CCIs).

import { ArrowLeft, Loader2, Search } from 'lucide-react'
import * as React from 'react'
import { Link, useParams } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  useRuleByRuleId,
  useRulesByRevision,
  useSTIG,
} from '@/lib/api/hooks'

function formatDate(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.valueOf())) return '—'
  return d.toLocaleDateString()
}

const PAGE_SIZE = 50

function severityClass(sev: string | undefined): string {
  switch ((sev ?? '').toLowerCase()) {
    case 'high':
      return 'bg-red-500/15 text-red-500'
    case 'medium':
      return 'bg-amber-500/15 text-amber-500'
    case 'low':
      return 'bg-emerald-500/15 text-emerald-500'
    default:
      return 'bg-[var(--color-accent)]/40'
  }
}

export function LibraryDetailPage() {
  const params = useParams<{ benchmarkId: string }>()
  const benchmarkId = params.benchmarkId ?? ''
  const stigQ = useSTIG(benchmarkId)

  const revisionStrs = stigQ.data?.revisionStrs ?? []
  const latest = stigQ.data?.lastRevisionStr ?? revisionStrs[0]

  const [selectedRevision, setSelectedRevision] = React.useState<string | undefined>(undefined)
  // Default the picker to the latest revision the first time data arrives.
  React.useEffect(() => {
    if (!selectedRevision && latest) setSelectedRevision(latest)
  }, [latest, selectedRevision])

  const rulesQ = useRulesByRevision(
    benchmarkId || undefined,
    selectedRevision,
  )

  const [filter, setFilter] = React.useState('')
  const [page, setPage] = React.useState(0)
  // Reset paging when the revision or filter changes.
  React.useEffect(() => {
    setPage(0)
  }, [selectedRevision, filter])

  const filtered = React.useMemo(() => {
    const rows = rulesQ.data ?? []
    const f = filter.trim().toLowerCase()
    if (!f) return rows
    return rows.filter(
      (r) =>
        r.ruleId.toLowerCase().includes(f) ||
        r.title.toLowerCase().includes(f) ||
        (r.version ?? '').toLowerCase().includes(f) ||
        (r.groupId ?? '').toLowerCase().includes(f),
    )
  }, [rulesQ.data, filter])

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const pageRows = filtered.slice(page * PAGE_SIZE, page * PAGE_SIZE + PAGE_SIZE)

  const [selectedRuleId, setSelectedRuleId] = React.useState<string | undefined>(undefined)
  const detailQ = useRuleByRuleId(selectedRuleId)

  return (
    <div className="space-y-6" data-testid="library-detail-page">
      <header className="flex items-center justify-between gap-3">
        <div>
          <Link
            to="/library"
            className="inline-flex items-center gap-1 text-sm text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]"
          >
            <ArrowLeft className="size-4" /> Back to Library
          </Link>
          <h1
            className="mt-1 text-3xl font-semibold tracking-tight"
            data-testid="library-detail-heading"
          >
            {benchmarkId}
          </h1>
        </div>
      </header>

      {stigQ.isLoading && (
        <div className="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
          <Loader2 className="size-4 animate-spin" /> Loading benchmark…
        </div>
      )}
      {stigQ.isError && (
        <Card>
          <CardContent className="pt-6">
            <p className="text-sm text-red-500">
              {stigQ.error instanceof Error
                ? stigQ.error.message
                : 'Failed to load benchmark.'}
            </p>
            <Button asChild variant="link" size="sm">
              <Link to="/library">Back to Library</Link>
            </Button>
          </CardContent>
        </Card>
      )}

      {stigQ.data && (
        <>
          <div className="grid gap-6 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>{stigQ.data.title}</CardTitle>
                <CardDescription>Benchmark metadata.</CardDescription>
              </CardHeader>
              <CardContent>
                <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-sm">
                  <dt className="text-[var(--color-muted-foreground)]">Benchmark ID</dt>
                  <dd className="font-mono text-xs">{stigQ.data.benchmarkId}</dd>
                  <dt className="text-[var(--color-muted-foreground)]">Latest revision</dt>
                  <dd className="font-mono text-xs">{stigQ.data.lastRevisionStr ?? '—'}</dd>
                  <dt className="text-[var(--color-muted-foreground)]">Revision date</dt>
                  <dd>{formatDate(stigQ.data.lastRevisionDate)}</dd>
                  <dt className="text-[var(--color-muted-foreground)]">Rule count</dt>
                  <dd className="tabular-nums">{stigQ.data.ruleCount ?? '—'}</dd>
                  {stigQ.data.marking && (
                    <>
                      <dt className="text-[var(--color-muted-foreground)]">Marking</dt>
                      <dd>{stigQ.data.marking}</dd>
                    </>
                  )}
                  {stigQ.data.status && (
                    <>
                      <dt className="text-[var(--color-muted-foreground)]">Status</dt>
                      <dd>{stigQ.data.status}</dd>
                    </>
                  )}
                </dl>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Revisions</CardTitle>
                <CardDescription>
                  {revisionStrs.length > 0
                    ? `${revisionStrs.length} revision${revisionStrs.length === 1 ? '' : 's'} imported.`
                    : 'No revisions imported yet.'}
                </CardDescription>
              </CardHeader>
              <CardContent>
                {revisionStrs.length > 0 ? (
                  <ul
                    className="space-y-1 text-sm"
                    data-testid="library-detail-revisions"
                  >
                    {revisionStrs.map((r) => (
                      <li key={r} className="font-mono text-xs">
                        {r}
                        {r === latest && (
                          <span className="ml-2 rounded-sm bg-[var(--color-accent)]/40 px-1.5 py-0.5 text-[10px] uppercase tracking-wider">
                            latest
                          </span>
                        )}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-sm text-[var(--color-muted-foreground)]">
                    Revisions appear here once an XCCDF bundle is imported.
                  </p>
                )}
              </CardContent>
            </Card>
          </div>

          <Card data-testid="library-rules-card">
            <CardHeader>
              <CardTitle>Rules</CardTitle>
              <CardDescription>
                Rules in the selected revision. Click a row for the full
                Vulnerability Discussion / Check / Fix projection.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-3 sm:grid-cols-[max-content_1fr]">
                <div className="space-y-1.5">
                  <Label htmlFor="library-revision-picker">Revision</Label>
                  <select
                    id="library-revision-picker"
                    value={selectedRevision ?? ''}
                    onChange={(e) => setSelectedRevision(e.target.value)}
                    className="block w-44 rounded-md border border-[var(--color-border)] bg-[var(--color-background)] px-2 py-1 font-mono text-xs"
                    data-testid="library-revision-select"
                    disabled={revisionStrs.length === 0}
                  >
                    {revisionStrs.length === 0 && <option value="">—</option>}
                    {revisionStrs.map((r) => (
                      <option key={r} value={r}>
                        {r}
                        {r === latest ? ' (latest)' : ''}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="library-rule-filter">Filter</Label>
                  <div className="relative">
                    <Search className="absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-[var(--color-muted-foreground)]" />
                    <Input
                      id="library-rule-filter"
                      value={filter}
                      onChange={(e) => setFilter(e.target.value)}
                      placeholder="ruleId / title / version / groupId"
                      className="pl-7 text-xs"
                      data-testid="library-rule-filter"
                    />
                  </div>
                </div>
              </div>

              {rulesQ.isLoading && (
                <p className="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
                  <Loader2 className="size-4 animate-spin" /> Loading rules…
                </p>
              )}
              {rulesQ.isError && (
                <p className="text-sm text-red-500" data-testid="library-rules-error">
                  {rulesQ.error instanceof Error
                    ? rulesQ.error.message
                    : 'Failed to load rules.'}
                </p>
              )}
              {rulesQ.data && rulesQ.data.length === 0 && (
                <p className="text-sm text-[var(--color-muted-foreground)]">
                  No rules found for this revision.
                </p>
              )}
              {rulesQ.data && rulesQ.data.length > 0 && (
                <>
                  <div className="overflow-x-auto rounded-md border border-[var(--color-border)]">
                    <table
                      className="w-full border-collapse text-sm"
                      data-testid="library-rules-table"
                    >
                      <thead className="bg-[var(--color-muted)]/40 text-left text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
                        <tr>
                          <th className="px-3 py-2">Rule ID</th>
                          <th className="px-3 py-2">Group</th>
                          <th className="px-3 py-2">Version</th>
                          <th className="px-3 py-2">Severity</th>
                          <th className="px-3 py-2">Title</th>
                        </tr>
                      </thead>
                      <tbody>
                        {pageRows.map((row) => {
                          const isSelected = row.ruleId === selectedRuleId
                          return (
                            <tr
                              key={row.ruleId}
                              data-testid="library-rule-row"
                              className={`cursor-pointer border-t border-[var(--color-border)] transition-colors ${
                                isSelected
                                  ? 'bg-[var(--color-accent)]/30'
                                  : 'hover:bg-[var(--color-muted)]/30'
                              }`}
                              onClick={() => setSelectedRuleId(row.ruleId)}
                            >
                              <td className="px-3 py-2 font-mono text-xs">
                                {row.ruleId}
                              </td>
                              <td className="px-3 py-2 font-mono text-xs">
                                {row.groupId ?? '—'}
                              </td>
                              <td className="px-3 py-2 font-mono text-xs">
                                {row.version ?? '—'}
                              </td>
                              <td className="px-3 py-2">
                                <span
                                  className={`rounded-sm px-1.5 py-0.5 text-[10px] uppercase tracking-wider ${severityClass(row.severity)}`}
                                >
                                  {row.severity || '—'}
                                </span>
                              </td>
                              <td className="px-3 py-2">{row.title}</td>
                            </tr>
                          )
                        })}
                      </tbody>
                    </table>
                  </div>
                  <div className="flex items-center justify-between gap-2 text-xs text-[var(--color-muted-foreground)]">
                    <span data-testid="library-rules-page-status">
                      Page {page + 1} of {pageCount} ({filtered.length}{' '}
                      rule{filtered.length === 1 ? '' : 's'})
                    </span>
                    <div className="flex items-center gap-2">
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => setPage((p) => Math.max(0, p - 1))}
                        disabled={page === 0}
                        data-testid="library-rules-prev"
                      >
                        Prev
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() =>
                          setPage((p) => Math.min(pageCount - 1, p + 1))
                        }
                        disabled={page >= pageCount - 1}
                        data-testid="library-rules-next"
                      >
                        Next
                      </Button>
                    </div>
                  </div>
                </>
              )}

              {selectedRuleId && (
                <div
                  className="space-y-2 rounded-md border border-[var(--color-border)] bg-[var(--color-card)]/40 p-3 text-sm"
                  data-testid="library-rule-detail-panel"
                >
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="font-mono text-xs">{selectedRuleId}</span>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => setSelectedRuleId(undefined)}
                    >
                      Close
                    </Button>
                  </div>
                  {detailQ.isLoading && (
                    <p className="text-xs text-[var(--color-muted-foreground)]">
                      Loading rule…
                    </p>
                  )}
                  {detailQ.isError && (
                    <p className="text-xs text-red-500">
                      {detailQ.error instanceof Error
                        ? detailQ.error.message
                        : 'Failed to load rule.'}
                    </p>
                  )}
                  {detailQ.data && (
                    <>
                      <p className="font-medium">{detailQ.data.title}</p>
                      {detailQ.data.severity && (
                        <span
                          className={`inline-block rounded-sm px-1.5 py-0.5 text-[10px] uppercase tracking-wider ${severityClass(detailQ.data.severity)}`}
                        >
                          {detailQ.data.severity}
                        </span>
                      )}
                      {detailQ.data.version && (
                        <p className="text-xs text-[var(--color-muted-foreground)]">
                          <span className="font-mono">Version:</span>{' '}
                          {detailQ.data.version}
                        </p>
                      )}
                      {detailQ.data.detail?.vulnDiscussion && (
                        <details open>
                          <summary className="cursor-pointer text-xs font-medium text-[var(--color-muted-foreground)]">
                            Vulnerability Discussion
                          </summary>
                          <pre className="mt-2 whitespace-pre-wrap break-words text-xs">
                            {detailQ.data.detail.vulnDiscussion}
                          </pre>
                        </details>
                      )}
                      {detailQ.data.check?.content && (
                        <details>
                          <summary className="cursor-pointer text-xs font-medium text-[var(--color-muted-foreground)]">
                            Check
                          </summary>
                          <pre className="mt-2 whitespace-pre-wrap break-words text-xs">
                            {detailQ.data.check.content}
                          </pre>
                        </details>
                      )}
                      {detailQ.data.fix?.text && (
                        <details>
                          <summary className="cursor-pointer text-xs font-medium text-[var(--color-muted-foreground)]">
                            Fix
                          </summary>
                          <pre className="mt-2 whitespace-pre-wrap break-words text-xs">
                            {detailQ.data.fix.text}
                          </pre>
                        </details>
                      )}
                      {detailQ.data.ccis && detailQ.data.ccis.length > 0 && (
                        <div className="text-xs">
                          <span className="font-medium text-[var(--color-muted-foreground)]">
                            CCIs:
                          </span>{' '}
                          {detailQ.data.ccis
                            .map((c) => c.cci)
                            .filter(Boolean)
                            .join(', ')}
                        </div>
                      )}
                    </>
                  )}
                </div>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}
