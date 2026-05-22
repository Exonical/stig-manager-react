// Per-STIG rule list for an asset. Renders an expandable card with the
// rule list for the given (benchmark, revision); each row shows the
// current review result (if any) and links to the single-rule review
// editor.

import { ChevronDown, ChevronRight, Loader2 } from 'lucide-react'
import * as React from 'react'
import { Link } from 'react-router-dom'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useRulesByRevision } from '@/lib/api/hooks'

interface RuleListProps {
  benchmarkId: string
  revisionStr: string
  collectionId: string
  assetId: string
  reviewsByRule: Map<string, { result: string; status?: string }>
}

export function RuleList({
  benchmarkId,
  revisionStr,
  collectionId,
  assetId,
  reviewsByRule,
}: RuleListProps) {
  const [open, setOpen] = React.useState(false)
  const rules = useRulesByRevision(open ? benchmarkId : undefined, open ? revisionStr : undefined)

  const reviewedCount = React.useMemo(() => {
    if (!rules.data) return null
    let n = 0
    for (const r of rules.data) if (reviewsByRule.has(r.ruleId)) n++
    return n
  }, [rules.data, reviewsByRule])

  return (
    <Card data-testid={`rule-list-${benchmarkId}`}>
      <CardHeader>
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          className="flex w-full items-center justify-between gap-3 text-left"
          data-testid={`rule-list-toggle-${benchmarkId}`}
        >
          <div className="flex items-center gap-2">
            {open ? (
              <ChevronDown className="size-4" />
            ) : (
              <ChevronRight className="size-4" />
            )}
            <CardTitle className="font-mono text-sm">{benchmarkId}</CardTitle>
            <span className="text-xs text-[var(--color-muted-foreground)]">
              {revisionStr}
            </span>
          </div>
          <span className="text-xs text-[var(--color-muted-foreground)]">
            {reviewedCount === null
              ? ''
              : `${reviewedCount} / ${rules.data?.length ?? 0} reviewed`}
          </span>
        </button>
      </CardHeader>

      {open && (
        <CardContent>
          {rules.isLoading && (
            <p className="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
              <Loader2 className="size-4 animate-spin" /> Loading rules…
            </p>
          )}
          {rules.isError && (
            <p className="text-sm text-red-500">
              Failed to load rules:{' '}
              {(rules.error as Error | null)?.message ?? 'Unknown error.'}
            </p>
          )}
          {rules.data && (
            <div className="overflow-x-auto rounded-md border border-[var(--color-border)]">
              <table className="w-full text-sm" data-testid={`rule-table-${benchmarkId}`}>
                <thead className="bg-[var(--color-muted)]/30 text-left text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
                  <tr>
                    <th className="px-3 py-2">Rule</th>
                    <th className="px-3 py-2">Severity</th>
                    <th className="px-3 py-2">Title</th>
                    <th className="px-3 py-2 text-right">Result</th>
                  </tr>
                </thead>
                <tbody>
                  {rules.data.map((r) => {
                    const rev = reviewsByRule.get(r.ruleId)
                    return (
                      <tr
                        key={r.ruleId}
                        className="border-t border-[var(--color-border)] hover:bg-[var(--color-muted)]/20"
                        data-testid={`rule-row-${r.ruleId}`}
                      >
                        <td className="px-3 py-2">
                          <Link
                            to={`/collections/${collectionId}/assets/${assetId}/rules/${r.ruleId}`}
                            className="font-mono text-xs text-sky-500 hover:underline"
                            data-testid={`rule-link-${r.ruleId}`}
                          >
                            {r.ruleId}
                          </Link>
                        </td>
                        <td className="px-3 py-2 text-xs uppercase">{r.severity}</td>
                        <td className="px-3 py-2">{r.title}</td>
                        <td className="px-3 py-2 text-right">
                          <ResultPill result={rev?.result} status={rev?.status} />
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      )}
    </Card>
  )
}

function ResultPill({ result, status }: { result?: string; status?: string }) {
  if (!result) {
    return (
      <span className="rounded-full border border-[var(--color-border)] px-2 py-0.5 text-[10px] uppercase tracking-wider text-[var(--color-muted-foreground)]">
        Unreviewed
      </span>
    )
  }
  const tone =
    result === 'pass'
      ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-500'
      : result === 'fail'
        ? 'border-red-500/40 bg-red-500/10 text-red-500'
        : 'border-amber-500/40 bg-amber-500/10 text-amber-500'
  return (
    <span
      className={`rounded-full border px-2 py-0.5 text-[10px] uppercase tracking-wider ${tone}`}
      title={status ? `status: ${status}` : undefined}
    >
      {result}
    </span>
  )
}
