// Rule lookup card on the Library landing page.
//
// Hits GET /stigs/rules/{ruleId} and renders the projection inline.
// The user-facing flow is "paste the SV-… or V-… ID and see its
// definition / check / fix / CCIs without leaving the Library".

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
import { useRuleByRuleId } from '@/lib/api/hooks'

export function RuleLookup() {
  const [input, setInput] = React.useState('')
  const [target, setTarget] = React.useState<string | undefined>(undefined)
  const q = useRuleByRuleId(target)

  function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    const trimmed = input.trim()
    setTarget(trimmed || undefined)
  }

  return (
    <Card data-testid="rule-lookup-card">
      <CardHeader>
        <CardTitle>Rule lookup</CardTitle>
        <CardDescription>
          Look up a Rule by its ID (e.g. <span className="font-mono text-xs">SV-1234r1_rule</span>).
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <form onSubmit={onSubmit} className="flex items-center gap-2">
          <Input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="SV-… or V-…"
            data-testid="rule-lookup-input"
            className="font-mono text-xs"
          />
          <Button
            type="submit"
            size="sm"
            disabled={!input.trim()}
            data-testid="rule-lookup-submit"
          >
            <Search className="size-4" /> Look up
          </Button>
        </form>

        {target && q.isLoading && (
          <p className="text-sm text-[var(--color-muted-foreground)]">Loading…</p>
        )}
        {target && q.isError && (
          <p className="text-sm text-red-500" data-testid="rule-lookup-error">
            {q.error instanceof Error ? q.error.message : 'Lookup failed.'}
          </p>
        )}
        {target && q.data && (
          <div className="space-y-2 rounded-md border border-[var(--color-border)] bg-[var(--color-card)]/40 p-3 text-sm" data-testid="rule-lookup-result">
            <div className="flex flex-wrap items-baseline gap-2">
              <span className="font-mono text-xs">{q.data.ruleId}</span>
              {q.data.severity && (
                <span className="rounded-sm bg-[var(--color-accent)]/40 px-1.5 py-0.5 text-xs uppercase tracking-wider">
                  {q.data.severity}
                </span>
              )}
            </div>
            <p className="font-medium">{q.data.title}</p>
            {q.data.version && (
              <p className="text-xs text-[var(--color-muted-foreground)]">
                <span className="font-mono">Version:</span> {q.data.version}
              </p>
            )}
            {q.data.detail?.vulnDiscussion && (
              <details>
                <summary className="cursor-pointer text-xs font-medium text-[var(--color-muted-foreground)]">
                  Vulnerability Discussion
                </summary>
                <pre className="mt-2 whitespace-pre-wrap break-words text-xs">
                  {q.data.detail.vulnDiscussion}
                </pre>
              </details>
            )}
            {q.data.check?.content && (
              <details>
                <summary className="cursor-pointer text-xs font-medium text-[var(--color-muted-foreground)]">
                  Check
                </summary>
                <pre className="mt-2 whitespace-pre-wrap break-words text-xs">
                  {q.data.check.content}
                </pre>
              </details>
            )}
            {q.data.fix?.text && (
              <details>
                <summary className="cursor-pointer text-xs font-medium text-[var(--color-muted-foreground)]">
                  Fix
                </summary>
                <pre className="mt-2 whitespace-pre-wrap break-words text-xs">
                  {q.data.fix.text}
                </pre>
              </details>
            )}
            {q.data.ccis && q.data.ccis.length > 0 && (
              <div className="text-xs">
                <span className="font-medium text-[var(--color-muted-foreground)]">CCIs:</span>{' '}
                {q.data.ccis.map((c) => c.cci).filter(Boolean).join(', ')}
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
