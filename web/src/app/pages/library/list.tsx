// /library — STIG Library landing page.
//
// Lists every benchmark imported into the API with a substring title
// filter. Each row links to the per-STIG detail page; the right-rail
// renders a Rule lookup and a CCI lookup that hit the per-id endpoints
// directly. The `stig-manager:stig` write scope unlocks the XCCDF
// Import button.

import { Loader2, Search, Trash2, Upload } from 'lucide-react'
import * as React from 'react'
import { Link } from 'react-router-dom'

import { ImportStigDialog } from './import-dialog'
import { CciLookup } from './cci-lookup'
import { RuleLookup } from './rule-lookup'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  useDeleteSTIG,
  useSTIGs,
  type STIGSummary,
} from '@/lib/api/hooks'
import { useAuth } from '@/lib/auth/auth-context'
import { hasScope } from '@/lib/auth/scopes'

function formatDate(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.valueOf())) return '—'
  return d.toLocaleDateString()
}

export function LibraryListPage() {
  const { status } = useAuth()
  const user = status.kind === 'signed-in' ? status.user : null
  const canWrite = hasScope(user, 'stig-manager:stig')

  const [query, setQuery] = React.useState('')
  const [debounced, setDebounced] = React.useState('')
  React.useEffect(() => {
    const id = window.setTimeout(() => setDebounced(query), 200)
    return () => window.clearTimeout(id)
  }, [query])

  const stigs = useSTIGs(debounced ? { title: debounced } : undefined)

  const [importOpen, setImportOpen] = React.useState(false)
  const del = useDeleteSTIG()

  async function onDelete(s: STIGSummary) {
    const ok = window.confirm(
      `Delete benchmark "${s.benchmarkId}" and all of its revisions? This cannot be undone.`,
    )
    if (!ok) return
    try {
      await del.mutateAsync(s.benchmarkId)
    } catch (err) {
      window.alert(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <div className="space-y-6" data-testid="library-list-page">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">
            STIG Library
          </h1>
          <p className="mt-1 text-sm text-[var(--color-muted-foreground)]">
            Browse imported benchmarks, rules, and CCIs. Import new XCCDF
            bundles when authorised.
          </p>
        </div>
        {canWrite && (
          <Button
            size="sm"
            onClick={() => setImportOpen(true)}
            data-testid="import-benchmark-button"
          >
            <Upload className="size-4" /> Import XCCDF
          </Button>
        )}
      </header>

      <div className="grid gap-6 lg:grid-cols-[2fr_1fr]">
        <Card>
          <CardHeader>
            <CardTitle>Benchmarks</CardTitle>
            <CardDescription>
              {stigs.data ? `${stigs.data.length} benchmark${stigs.data.length === 1 ? '' : 's'}` : 'Loading…'}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center gap-2">
              <div className="relative w-full max-w-md">
                <Search className="pointer-events-none absolute left-2 top-1/2 size-4 -translate-y-1/2 text-[var(--color-muted-foreground)]" />
                <Input
                  placeholder="Search by title (contains)…"
                  className="pl-8"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  data-testid="library-search"
                />
              </div>
              {stigs.isFetching && !stigs.isLoading && (
                <Loader2 className="size-4 animate-spin text-[var(--color-muted-foreground)]" />
              )}
            </div>

            <div className="overflow-x-auto rounded-md border border-[var(--color-border)]">
              <table className="w-full text-sm" data-testid="library-table">
                <thead className="bg-[var(--color-muted)]/30 text-left text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
                  <tr>
                    <th className="px-3 py-2">Benchmark</th>
                    <th className="px-3 py-2">Title</th>
                    <th className="px-3 py-2">Revision</th>
                    <th className="px-3 py-2">Date</th>
                    <th className="px-3 py-2 text-right">Rules</th>
                    {canWrite && <th className="w-12 px-3 py-2" />}
                  </tr>
                </thead>
                <tbody>
                  {stigs.isLoading && (
                    <tr>
                      <td
                        className="px-3 py-6 text-center text-[var(--color-muted-foreground)]"
                        colSpan={canWrite ? 6 : 5}
                      >
                        Loading benchmarks…
                      </td>
                    </tr>
                  )}
                  {stigs.isError && (
                    <tr>
                      <td
                        className="px-3 py-6 text-center text-red-500"
                        colSpan={canWrite ? 6 : 5}
                      >
                        {stigs.error instanceof Error
                          ? stigs.error.message
                          : 'Failed to load benchmarks.'}
                      </td>
                    </tr>
                  )}
                  {stigs.data && stigs.data.length === 0 && (
                    <tr>
                      <td
                        className="px-3 py-6 text-center text-[var(--color-muted-foreground)]"
                        colSpan={canWrite ? 6 : 5}
                      >
                        {debounced
                          ? 'No benchmarks match that title.'
                          : 'No benchmarks yet. Import an XCCDF bundle to populate the library.'}
                      </td>
                    </tr>
                  )}
                  {stigs.data?.map((s) => (
                    <tr
                      key={s.benchmarkId}
                      className="border-t border-[var(--color-border)] hover:bg-[var(--color-accent)]/30"
                    >
                      <td className="px-3 py-2 font-mono text-xs">
                        <Link
                          to={`/library/${encodeURIComponent(s.benchmarkId)}`}
                          className="font-semibold text-[var(--color-primary)] hover:underline"
                          data-testid={`library-row-${s.benchmarkId}`}
                        >
                          {s.benchmarkId}
                        </Link>
                      </td>
                      <td className="px-3 py-2">{s.title}</td>
                      <td className="px-3 py-2 font-mono text-xs">
                        {s.lastRevisionStr ?? '—'}
                      </td>
                      <td className="px-3 py-2 text-[var(--color-muted-foreground)]">
                        {formatDate(s.lastRevisionDate)}
                      </td>
                      <td className="px-3 py-2 text-right tabular-nums">
                        {s.ruleCount ?? '—'}
                      </td>
                      {canWrite && (
                        <td className="px-3 py-2 text-right">
                          <Button
                            size="icon"
                            variant="ghost"
                            aria-label={`Delete ${s.benchmarkId}`}
                            onClick={() => onDelete(s)}
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>

        <div className="space-y-6">
          <RuleLookup />
          <CciLookup />
        </div>
      </div>

      {canWrite && (
        <ImportStigDialog open={importOpen} onOpenChange={setImportOpen} />
      )}
    </div>
  )
}
