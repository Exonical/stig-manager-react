// /admin/audit-log — surface the audit_log mutations captured by the
// audit middleware. Lists rows in reverse-chronological order with
// filters (method, path substring, userId, since/until) and a
// per-row expand to inspect the JSON payload + metadata.

import { Loader2, RefreshCw, Search } from 'lucide-react'
import * as React from 'react'

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
  useAuditLog,
  type AuditLogEntry,
  type AuditLogFilter,
} from '@/lib/api/hooks'

const HTTP_METHODS = ['', 'POST', 'PUT', 'PATCH', 'DELETE'] as const

function formatTs(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.valueOf())) return iso
  return d.toLocaleString()
}

function statusClasses(status: number): string {
  if (status >= 500) return 'bg-red-500/20 text-red-700 dark:text-red-300'
  if (status >= 400) return 'bg-amber-500/20 text-amber-700 dark:text-amber-300'
  if (status >= 300) return 'bg-blue-500/20 text-blue-700 dark:text-blue-300'
  return 'bg-emerald-500/20 text-emerald-700 dark:text-emerald-300'
}

function methodClasses(method: string): string {
  switch (method) {
    case 'POST':
      return 'bg-emerald-500/20 text-emerald-700 dark:text-emerald-300'
    case 'PUT':
      return 'bg-blue-500/20 text-blue-700 dark:text-blue-300'
    case 'PATCH':
      return 'bg-purple-500/20 text-purple-700 dark:text-purple-300'
    case 'DELETE':
      return 'bg-red-500/20 text-red-700 dark:text-red-300'
    default:
      return 'bg-[var(--color-accent)]/40 text-[var(--color-accent-foreground)]'
  }
}

function toRfc3339(local: string): string | undefined {
  // <input type="datetime-local"> returns "YYYY-MM-DDTHH:mm"; we parse
  // it as local time and emit a UTC RFC3339 string for the API.
  if (!local) return undefined
  const d = new Date(local)
  if (Number.isNaN(d.valueOf())) return undefined
  return d.toISOString()
}

function ExpandedRow({ row }: { row: AuditLogEntry }) {
  return (
    <tr className="border-t border-[var(--color-border)] bg-[var(--color-muted)]/20">
      <td colSpan={7} className="px-3 py-3">
        <div className="grid gap-3 lg:grid-cols-2">
          <div>
            <p className="mb-1 text-xs font-medium uppercase tracking-wider text-[var(--color-muted-foreground)]">
              Payload
            </p>
            {row.payload !== undefined && row.payload !== null ? (
              <pre
                className="max-h-96 overflow-auto rounded-md border border-[var(--color-border)] bg-[var(--color-card)] p-2 text-xs"
                data-testid={`audit-row-payload-${row.auditId}`}
              >
                {JSON.stringify(row.payload, null, 2)}
              </pre>
            ) : (
              <p className="text-xs text-[var(--color-muted-foreground)]">
                No payload recorded.
              </p>
            )}
          </div>
          <div>
            <p className="mb-1 text-xs font-medium uppercase tracking-wider text-[var(--color-muted-foreground)]">
              Metadata
            </p>
            {row.metadata !== undefined && row.metadata !== null ? (
              <pre
                className="max-h-96 overflow-auto rounded-md border border-[var(--color-border)] bg-[var(--color-card)] p-2 text-xs"
                data-testid={`audit-row-metadata-${row.auditId}`}
              >
                {JSON.stringify(row.metadata, null, 2)}
              </pre>
            ) : (
              <p className="text-xs text-[var(--color-muted-foreground)]">
                No metadata recorded.
              </p>
            )}
          </div>
        </div>
        <dl className="mt-3 grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-xs">
          {row.route && (
            <>
              <dt className="text-[var(--color-muted-foreground)]">Route</dt>
              <dd className="font-mono">{row.route}</dd>
            </>
          )}
          {row.requestId && (
            <>
              <dt className="text-[var(--color-muted-foreground)]">Request ID</dt>
              <dd className="font-mono">{row.requestId}</dd>
            </>
          )}
          {row.subject && (
            <>
              <dt className="text-[var(--color-muted-foreground)]">OIDC subject</dt>
              <dd className="font-mono">{row.subject}</dd>
            </>
          )}
          {row.ip && (
            <>
              <dt className="text-[var(--color-muted-foreground)]">IP</dt>
              <dd className="font-mono">{row.ip}</dd>
            </>
          )}
        </dl>
      </td>
    </tr>
  )
}

export function AuditLogPage() {
  const [method, setMethod] = React.useState<(typeof HTTP_METHODS)[number]>('')
  const [path, setPath] = React.useState('')
  const [userIdInput, setUserIdInput] = React.useState('')
  const [since, setSince] = React.useState('')
  const [until, setUntil] = React.useState('')
  const [limit, setLimit] = React.useState(100)

  // Stable filter object the page renders + the hook depends on. We
  // bind it on Apply rather than on every keystroke so the table
  // doesn't churn while the user types.
  const [filter, setFilter] = React.useState<AuditLogFilter>({ limit: 100 })

  const log = useAuditLog(filter)

  const [expanded, setExpanded] = React.useState<Set<number>>(new Set())
  function toggleExpand(auditId: number) {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(auditId)) next.delete(auditId)
      else next.add(auditId)
      return next
    })
  }

  function onApply(e: React.FormEvent) {
    e.preventDefault()
    const next: AuditLogFilter = { limit }
    if (method) next.method = method
    if (path.trim()) next.path = path.trim()
    if (userIdInput.trim()) {
      const n = Number.parseInt(userIdInput.trim(), 10)
      if (!Number.isNaN(n)) next.userId = n
    }
    const sinceIso = toRfc3339(since)
    if (sinceIso) next.since = sinceIso
    const untilIso = toRfc3339(until)
    if (untilIso) next.until = untilIso
    setFilter(next)
    setExpanded(new Set())
  }

  function onReset() {
    setMethod('')
    setPath('')
    setUserIdInput('')
    setSince('')
    setUntil('')
    setLimit(100)
    setFilter({ limit: 100 })
    setExpanded(new Set())
  }

  return (
    <div className="space-y-6" data-testid="admin-audit-log-page">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div>
              <CardTitle>Audit log</CardTitle>
              <CardDescription>
                Mutations captured by the audit middleware. Newest first.
              </CardDescription>
            </div>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => log.refetch()}
              disabled={log.isFetching}
              data-testid="audit-refresh"
            >
              <RefreshCw className="size-4" /> Refresh
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <form
            onSubmit={onApply}
            className="grid gap-3 lg:grid-cols-6"
            data-testid="audit-filter-form"
          >
            <div className="space-y-1.5">
              <Label htmlFor="audit-method">Method</Label>
              <select
                id="audit-method"
                value={method}
                onChange={(e) =>
                  setMethod(e.target.value as (typeof HTTP_METHODS)[number])
                }
                className="block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-background)] px-2 py-1 text-sm"
                data-testid="audit-method-select"
              >
                {HTTP_METHODS.map((m) => (
                  <option key={m || 'any'} value={m}>
                    {m || 'Any'}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-1.5 lg:col-span-2">
              <Label htmlFor="audit-path">Path contains</Label>
              <div className="relative">
                <Search className="pointer-events-none absolute left-2 top-1/2 size-4 -translate-y-1/2 text-[var(--color-muted-foreground)]" />
                <Input
                  id="audit-path"
                  className="pl-8"
                  value={path}
                  onChange={(e) => setPath(e.target.value)}
                  placeholder="/api/collections"
                  data-testid="audit-path-input"
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="audit-userid">User ID</Label>
              <Input
                id="audit-userid"
                value={userIdInput}
                onChange={(e) => setUserIdInput(e.target.value)}
                placeholder="numeric"
                inputMode="numeric"
                data-testid="audit-userid-input"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="audit-since">Since</Label>
              <input
                id="audit-since"
                type="datetime-local"
                value={since}
                onChange={(e) => setSince(e.target.value)}
                className="block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-background)] px-2 py-1 text-sm"
                data-testid="audit-since-input"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="audit-until">Until</Label>
              <input
                id="audit-until"
                type="datetime-local"
                value={until}
                onChange={(e) => setUntil(e.target.value)}
                className="block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-background)] px-2 py-1 text-sm"
                data-testid="audit-until-input"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="audit-limit">Limit</Label>
              <Input
                id="audit-limit"
                type="number"
                min={1}
                max={1000}
                value={limit}
                onChange={(e) =>
                  setLimit(
                    Math.min(
                      1000,
                      Math.max(1, Number.parseInt(e.target.value, 10) || 1),
                    ),
                  )
                }
                data-testid="audit-limit-input"
              />
            </div>
            <div className="flex items-end gap-2 lg:col-span-6">
              <Button type="submit" size="sm" data-testid="audit-apply">
                Apply
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={onReset}
                data-testid="audit-reset"
              >
                Reset
              </Button>
              {log.isFetching && (
                <Loader2 className="size-4 animate-spin text-[var(--color-muted-foreground)]" />
              )}
              {log.data && (
                <span className="ml-auto text-xs text-[var(--color-muted-foreground)]">
                  {log.data.length} row{log.data.length === 1 ? '' : 's'}
                </span>
              )}
            </div>
          </form>

          <div className="overflow-x-auto rounded-md border border-[var(--color-border)]">
            <table className="w-full text-sm" data-testid="audit-table">
              <thead className="bg-[var(--color-muted)]/30 text-left text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
                <tr>
                  <th className="w-44 px-3 py-2">Timestamp</th>
                  <th className="w-20 px-3 py-2">Method</th>
                  <th className="px-3 py-2">Path</th>
                  <th className="w-20 px-3 py-2">Status</th>
                  <th className="w-24 px-3 py-2 text-right">Duration</th>
                  <th className="w-44 px-3 py-2">User</th>
                  <th className="w-20 px-3 py-2 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {log.isLoading && (
                  <tr>
                    <td
                      colSpan={7}
                      className="px-3 py-6 text-center text-[var(--color-muted-foreground)]"
                    >
                      Loading audit log…
                    </td>
                  </tr>
                )}
                {log.isError && (
                  <tr>
                    <td
                      colSpan={7}
                      className="px-3 py-6 text-center text-red-500"
                      data-testid="audit-error"
                    >
                      {log.error instanceof Error
                        ? log.error.message
                        : 'Failed to load audit log.'}
                    </td>
                  </tr>
                )}
                {log.data && log.data.length === 0 && (
                  <tr>
                    <td
                      colSpan={7}
                      className="px-3 py-6 text-center text-[var(--color-muted-foreground)]"
                    >
                      No rows match the current filters.
                    </td>
                  </tr>
                )}
                {log.data?.map((row) => {
                  const open = expanded.has(row.auditId)
                  return (
                    <React.Fragment key={row.auditId}>
                      <tr
                        className="border-t border-[var(--color-border)] hover:bg-[var(--color-accent)]/30"
                        data-testid={`audit-row-${row.auditId}`}
                      >
                        <td className="px-3 py-2 text-xs text-[var(--color-muted-foreground)]">
                          {formatTs(row.ts)}
                        </td>
                        <td className="px-3 py-2">
                          <span
                            className={`inline-block rounded-sm px-1.5 py-0.5 text-xs font-medium uppercase tracking-wider ${methodClasses(row.method)}`}
                          >
                            {row.method}
                          </span>
                        </td>
                        <td className="px-3 py-2 font-mono text-xs">{row.path}</td>
                        <td className="px-3 py-2">
                          <span
                            className={`inline-block rounded-sm px-1.5 py-0.5 text-xs font-medium tabular-nums ${statusClasses(row.status)}`}
                          >
                            {row.status}
                          </span>
                        </td>
                        <td className="px-3 py-2 text-right text-xs tabular-nums text-[var(--color-muted-foreground)]">
                          {row.durationMs} ms
                        </td>
                        <td className="px-3 py-2 text-xs">
                          {row.username ? (
                            <span>
                              {row.username}
                              {row.userId ? (
                                <span className="ml-1 text-[var(--color-muted-foreground)]">
                                  #{row.userId}
                                </span>
                              ) : null}
                            </span>
                          ) : (
                            <span className="text-[var(--color-muted-foreground)]">
                              anonymous
                            </span>
                          )}
                        </td>
                        <td className="px-3 py-2 text-right">
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => toggleExpand(row.auditId)}
                            data-testid={`audit-row-toggle-${row.auditId}`}
                          >
                            {open ? 'Hide' : 'View'}
                          </Button>
                        </td>
                      </tr>
                      {open && <ExpandedRow row={row} />}
                    </React.Fragment>
                  )
                })}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
