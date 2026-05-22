// CCI lookup card on the Library landing page.
//
// Hits GET /stigs/ccis/{cci}. The API accepts either "CCI-000366" or
// the six-digit form ("000366") and normalises both, so the user can
// paste whichever form they have in front of them.

import { Search } from 'lucide-react'
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
import { useCci } from '@/lib/api/hooks'

function normaliseInput(raw: string): string {
  const s = raw.trim()
  if (!s) return ''
  // Strip a "CCI-" prefix; the OpenAPI path param matches /^\d{6}$/.
  return s.replace(/^cci-/i, '')
}

export function CciLookup() {
  const [input, setInput] = React.useState('')
  const [target, setTarget] = React.useState<string | undefined>(undefined)
  const q = useCci(target)

  function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    const trimmed = normaliseInput(input)
    setTarget(trimmed || undefined)
  }

  return (
    <Card data-testid="cci-lookup-card">
      <CardHeader>
        <CardTitle>CCI lookup</CardTitle>
        <CardDescription>
          Look up a CCI by its six-digit ID (e.g. <span className="font-mono text-xs">000366</span> or <span className="font-mono text-xs">CCI-000366</span>).
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <form onSubmit={onSubmit} className="flex items-center gap-2">
          <Input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="000366 or CCI-000366"
            data-testid="cci-lookup-input"
            className="font-mono text-xs"
          />
          <Button
            type="submit"
            size="sm"
            disabled={!input.trim()}
            data-testid="cci-lookup-submit"
          >
            <Search className="size-4" /> Look up
          </Button>
        </form>

        {target && q.isLoading && (
          <p className="text-sm text-[var(--color-muted-foreground)]">Loading…</p>
        )}
        {target && q.isError && (
          <p className="text-sm text-red-500" data-testid="cci-lookup-error">
            {q.error instanceof Error ? q.error.message : 'Lookup failed.'}
          </p>
        )}
        {target && q.data && (
          <div className="space-y-2 rounded-md border border-[var(--color-border)] bg-[var(--color-card)]/40 p-3 text-sm" data-testid="cci-lookup-result">
            <div className="flex flex-wrap items-baseline gap-2">
              <span className="font-mono text-xs">{q.data.cci}</span>
              {q.data.status && (
                <span className="rounded-sm bg-[var(--color-accent)]/40 px-1.5 py-0.5 text-xs uppercase tracking-wider">
                  {q.data.status}
                </span>
              )}
              {q.data.type && (
                <span className="text-xs text-[var(--color-muted-foreground)]">
                  {q.data.type}
                </span>
              )}
            </div>
            {q.data.definition && (
              <p className="text-sm">{q.data.definition}</p>
            )}
            {q.data.publishdate && (
              <p className="text-xs text-[var(--color-muted-foreground)]">
                Published: {new Date(q.data.publishdate).toLocaleDateString()}
              </p>
            )}
            {q.data.stigs && q.data.stigs.length > 0 && (
              <details>
                <summary className="cursor-pointer text-xs font-medium text-[var(--color-muted-foreground)]">
                  Referenced by {q.data.stigs.length} STIG revision{q.data.stigs.length === 1 ? '' : 's'}
                </summary>
                <ul className="mt-2 space-y-0.5 text-xs">
                  {q.data.stigs.map((s, idx) => (
                    <li key={`${s.benchmarkId}-${s.revisionStr}-${idx}`} className="font-mono">
                      {s.benchmarkId} · {s.revisionStr}
                    </li>
                  ))}
                </ul>
              </details>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
