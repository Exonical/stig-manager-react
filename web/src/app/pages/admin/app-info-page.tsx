// /admin/app-info — runtime / counts / postgres / request metrics +
// AppData tables list.
//
// Auto-refreshes every 15s while the page is open.

import { Loader2 } from 'lucide-react'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useAppDataTables, useAppInfoDetail } from '@/lib/api/hooks'

export function AppInfoPage() {
  const info = useAppInfoDetail()
  const tables = useAppDataTables()

  return (
    <div className="space-y-6" data-testid="admin-app-info-page">
      {info.isLoading && (
        <p className="inline-flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
          <Loader2 className="size-4 animate-spin" /> Loading app info…
        </p>
      )}
      {info.isError && (
        <p className="text-sm text-red-500">
          Failed to load appinfo: {(info.error as Error).message}
        </p>
      )}

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        <Card data-testid="appinfo-build">
          <CardHeader>
            <CardTitle>Build</CardTitle>
            <CardDescription>API binary metadata.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <KV label="Version" value={info.data?.version ?? '—'} mono />
            <KV label="Commit" value={info.data?.commit ?? '—'} mono />
            <KV label="Build date" value={info.data?.buildDate ?? '—'} />
            <KV label="Schema" value={info.data?.schema ?? '—'} mono />
          </CardContent>
        </Card>

        <Card data-testid="appinfo-counts">
          <CardHeader>
            <CardTitle>Row counts</CardTitle>
            <CardDescription>Top-level data totals.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <KV label="Users" value={fmt(info.data?.counts?.users)} />
            <KV label="User groups" value={fmt(info.data?.counts?.userGroups)} />
            <KV label="Collections" value={fmt(info.data?.counts?.collections)} />
            <KV label="Assets" value={fmt(info.data?.counts?.assets)} />
            <KV label="STIGs" value={fmt(info.data?.counts?.stigs)} />
            <KV label="Reviews" value={fmt(info.data?.counts?.reviews)} />
            <KV label="Jobs" value={fmt(info.data?.counts?.jobs)} />
            <KV label="Runs" value={fmt(info.data?.counts?.runs)} />
          </CardContent>
        </Card>

        <Card data-testid="appinfo-runtime">
          <CardHeader>
            <CardTitle>Runtime</CardTitle>
            <CardDescription>Go runtime stats.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <KV label="Go version" value={info.data?.runtime?.goVersion ?? '—'} mono />
            <KV label="OS / Arch" value={`${info.data?.runtime?.os ?? '?'} / ${info.data?.runtime?.arch ?? '?'}`} />
            <KV label="CPUs" value={fmt(info.data?.runtime?.cpus)} />
            <KV label="Goroutines" value={fmt(info.data?.runtime?.goroutines)} />
            <KV label="Uptime" value={fmtDuration(info.data?.runtime?.uptime)} />
            <KV
              label="Heap alloc"
              value={fmtBytes(info.data?.runtime?.memory?.heapAlloc)}
            />
            <KV label="Sys" value={fmtBytes(info.data?.runtime?.memory?.sys)} />
          </CardContent>
        </Card>

        <Card data-testid="appinfo-postgres">
          <CardHeader>
            <CardTitle>Postgres</CardTitle>
            <CardDescription>Database state.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <KV label="Version" value={info.data?.postgres?.version ?? '—'} />
            <KV label="Start time" value={info.data?.postgres?.startTime ?? '—'} />
            <KV
              label="Uptime"
              value={fmtDuration(info.data?.postgres?.uptime)}
            />
            <KV
              label="Connections"
              value={fmt(info.data?.postgres?.connections)}
            />
          </CardContent>
        </Card>

        <Card data-testid="appinfo-requests" className="md:col-span-2">
          <CardHeader>
            <CardTitle>Requests</CardTitle>
            <CardDescription>
              Per-route counters since process start.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <KV
              label="Total requests"
              value={fmt(info.data?.requests?.totalRequests)}
            />
            <KV
              label="API requests"
              value={fmt(info.data?.requests?.totalApiRequests)}
            />
            <KV
              label="Total errors"
              value={fmt(info.data?.requests?.totalErrors)}
            />
            <KV
              label="Total duration (s)"
              value={fmt(info.data?.requests?.totalRequestDuration)}
            />
          </CardContent>
        </Card>
      </div>

      {/* AppData tables list */}
      <Card data-testid="appinfo-tables">
        <CardHeader>
          <CardTitle>AppData tables</CardTitle>
          <CardDescription>
            Tables visible to the AppData export, with row + byte totals.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {tables.isLoading && (
            <p className="text-sm text-[var(--color-muted-foreground)]">
              Loading tables…
            </p>
          )}
          {tables.isError && (
            <p className="text-sm text-red-500">
              Failed to load tables: {(tables.error as Error).message}
            </p>
          )}
          {!tables.isLoading && (tables.data?.length ?? 0) === 0 && (
            <p
              className="text-sm text-[var(--color-muted-foreground)]"
              data-testid="appdata-tables-empty"
            >
              No tables reported.
            </p>
          )}
          {(tables.data?.length ?? 0) > 0 && (
            <div className="overflow-x-auto rounded-md border border-[var(--color-border)]">
              <table className="w-full text-sm" data-testid="appdata-tables-table">
                <thead className="bg-[var(--color-muted)]/30 text-left text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
                  <tr>
                    <th className="px-3 py-2">Table</th>
                    <th className="px-3 py-2 text-right">Rows</th>
                    <th className="px-3 py-2 text-right">Data length</th>
                  </tr>
                </thead>
                <tbody>
                  {(tables.data ?? []).map((t, i) => (
                    <tr
                      key={`${t.name ?? 'unknown'}-${i}`}
                      className="border-t border-[var(--color-border)]"
                    >
                      <td className="px-3 py-2 font-mono">{t.name ?? '?'}</td>
                      <td className="px-3 py-2 text-right">{fmt(t.rows)}</td>
                      <td className="px-3 py-2 text-right">
                        {fmtBytes(t.dataLength)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function KV({
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

function fmt(v: number | undefined | null): string {
  if (v === undefined || v === null) return '—'
  return Number(v).toLocaleString()
}

function fmtBytes(v: number | undefined | null): string {
  if (v === undefined || v === null) return '—'
  if (v < 1024) return `${v} B`
  if (v < 1024 * 1024) return `${(v / 1024).toFixed(1)} KiB`
  if (v < 1024 * 1024 * 1024) return `${(v / 1024 / 1024).toFixed(1)} MiB`
  return `${(v / 1024 / 1024 / 1024).toFixed(2)} GiB`
}

function fmtDuration(seconds: number | undefined | null): string {
  if (seconds === undefined || seconds === null) return '—'
  if (seconds < 60) return `${seconds.toFixed(0)}s`
  if (seconds < 3600) return `${(seconds / 60).toFixed(1)}m`
  if (seconds < 86400) return `${(seconds / 3600).toFixed(1)}h`
  return `${(seconds / 86400).toFixed(1)}d`
}
