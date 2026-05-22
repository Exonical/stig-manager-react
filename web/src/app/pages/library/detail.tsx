// /library/:benchmarkId — STIG detail page.
//
// Shows the metadata projection for a single benchmark plus its
// revision list. Per-revision rule/group browsers (which require the
// not-yet-implemented /stigs/{benchmarkId}/revisions/{revisionStr}/rules
// endpoint) are intentionally not surfaced here.

import { ArrowLeft, Loader2 } from 'lucide-react'
import { Link, useParams } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useSTIG } from '@/lib/api/hooks'

function formatDate(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.valueOf())) return '—'
  return d.toLocaleDateString()
}

export function LibraryDetailPage() {
  const params = useParams<{ benchmarkId: string }>()
  const benchmarkId = params.benchmarkId ?? ''
  const q = useSTIG(benchmarkId)

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
          <h1 className="mt-1 text-3xl font-semibold tracking-tight" data-testid="library-detail-heading">
            {benchmarkId}
          </h1>
        </div>
      </header>

      {q.isLoading && (
        <div className="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
          <Loader2 className="size-4 animate-spin" /> Loading benchmark…
        </div>
      )}
      {q.isError && (
        <Card>
          <CardContent className="pt-6">
            <p className="text-sm text-red-500">
              {q.error instanceof Error
                ? q.error.message
                : 'Failed to load benchmark.'}
            </p>
            <Button asChild variant="link" size="sm">
              <Link to="/library">Back to Library</Link>
            </Button>
          </CardContent>
        </Card>
      )}

      {q.data && (
        <div className="grid gap-6 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>{q.data.title}</CardTitle>
              <CardDescription>Benchmark metadata.</CardDescription>
            </CardHeader>
            <CardContent>
              <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-sm">
                <dt className="text-[var(--color-muted-foreground)]">Benchmark ID</dt>
                <dd className="font-mono text-xs">{q.data.benchmarkId}</dd>
                <dt className="text-[var(--color-muted-foreground)]">Latest revision</dt>
                <dd className="font-mono text-xs">{q.data.lastRevisionStr ?? '—'}</dd>
                <dt className="text-[var(--color-muted-foreground)]">Revision date</dt>
                <dd>{formatDate(q.data.lastRevisionDate)}</dd>
                <dt className="text-[var(--color-muted-foreground)]">Rule count</dt>
                <dd className="tabular-nums">{q.data.ruleCount ?? '—'}</dd>
                {q.data.marking && (
                  <>
                    <dt className="text-[var(--color-muted-foreground)]">Marking</dt>
                    <dd>{q.data.marking}</dd>
                  </>
                )}
                {q.data.status && (
                  <>
                    <dt className="text-[var(--color-muted-foreground)]">Status</dt>
                    <dd>{q.data.status}</dd>
                  </>
                )}
              </dl>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Revisions</CardTitle>
              <CardDescription>
                {q.data.revisionStrs && q.data.revisionStrs.length > 0
                  ? `${q.data.revisionStrs.length} revision${q.data.revisionStrs.length === 1 ? '' : 's'} imported.`
                  : 'No revisions imported yet.'}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {q.data.revisionStrs && q.data.revisionStrs.length > 0 ? (
                <ul
                  className="space-y-1 text-sm"
                  data-testid="library-detail-revisions"
                >
                  {q.data.revisionStrs.map((r) => (
                    <li key={r} className="font-mono text-xs">
                      {r}
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
      )}
    </div>
  )
}
