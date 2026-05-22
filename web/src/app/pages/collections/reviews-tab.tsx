// Reviews tab inside the Collection detail page. Wraps the bulk
// review-mutation endpoint (POST /collections/{cid}/reviews) in a
// form so Manage-or-Owner users can apply a single source review to
// every (asset, rule) pair resolved from their selection.
//
// The form lets the caller pick:
//   * an asset criterion — assetIds (multi-select from the
//     collection's asset list) OR benchmarkIds (multi-select from
//     the collection's STIG list).
//   * a rule criterion — ruleIds (comma- or whitespace-separated
//     free text) OR benchmarkIds (same multi-select).
//   * an action — insert / update / merge.
//   * a source review (result, status, detail, comment).
//   * a dryRun toggle — submits the same request with `dryRun=true`
//     so users can preview the counts before committing.
//
// Per the spec (see #/components/schemas/ReviewBatch), `assets` and
// `rules` are each oneOf — the UI lets the user toggle between the
// two shapes per field. The actual updateFilters slot (status/result
// gating beyond the asset+rule cross-product) is intentionally
// deferred for now; see the M18d PR for the rationale.
//
// Imports — CKL / CKLB / XCCDF upload + dry-run preview — are
// deferred to a follow-up because they require either a client-side
// parser bundle or a new server-side multipart endpoint (the
// existing /collections/{cid}/reviews accepts JSON only). The "Imports"
// sub-card here renders an explainer with that context.

import { Loader2, Send } from 'lucide-react'
import * as React from 'react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  useAssets,
  useCollectionStigs,
  useReviewBatch,
  type ReviewBatchInput,
  type ReviewBatchResponse,
  type ReviewBatchResponseDryRun,
  type ReviewResult,
  type ReviewStatusLabel,
} from '@/lib/api/hooks'

const RESULT_OPTIONS: ReviewResult[] = [
  'fail',
  'pass',
  'notapplicable',
  'notchecked',
  'unknown',
  'error',
  'notselected',
  'informational',
  'fixed',
]

const STATUS_OPTIONS: { value: ReviewStatusLabel | ''; label: string }[] = [
  { value: '', label: 'Keep current (default)' },
  { value: 'saved', label: 'Saved' },
  { value: 'submitted', label: 'Submitted' },
]

type AssetMode = 'assetIds' | 'benchmarkIds'
type RuleMode = 'ruleIds' | 'benchmarkIds'

interface ReviewsTabProps {
  collectionId: string
  /** Current user's roleId on the Collection (null = no grant). */
  role: number | null
}

export function ReviewsTab({ collectionId, role }: ReviewsTabProps) {
  const canManage = role !== null && role >= 3
  const assets = useAssets({ collectionId })
  const stigs = useCollectionStigs(collectionId)
  const batch = useReviewBatch()

  const [assetMode, setAssetMode] = React.useState<AssetMode>('assetIds')
  const [ruleMode, setRuleMode] = React.useState<RuleMode>('ruleIds')
  const [selectedAssetIds, setSelectedAssetIds] = React.useState<Set<string>>(
    new Set(),
  )
  const [assetBenchmarkIds, setAssetBenchmarkIds] = React.useState<Set<string>>(
    new Set(),
  )
  const [ruleBenchmarkIds, setRuleBenchmarkIds] = React.useState<Set<string>>(
    new Set(),
  )
  const [ruleIdsRaw, setRuleIdsRaw] = React.useState('')

  const [action, setAction] = React.useState<'insert' | 'update' | 'merge'>(
    'merge',
  )
  const [result, setResult] = React.useState<ReviewResult>('notchecked')
  const [detail, setDetail] = React.useState('')
  const [comment, setComment] = React.useState('')
  const [status, setStatus] = React.useState<ReviewStatusLabel | ''>('')

  const [error, setError] = React.useState<string | null>(null)
  const [lastResponse, setLastResponse] = React.useState<
    | { kind: 'real'; data: ReviewBatchResponse }
    | { kind: 'dry'; data: ReviewBatchResponseDryRun }
    | null
  >(null)

  if (!canManage) {
    return (
      <Card data-testid="collection-reviews-tab">
        <CardHeader>
          <CardTitle>Reviews</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-[var(--color-muted-foreground)]">
            Batch Review requires the <span className="font-medium">Manage</span>{' '}
            role on this Collection. Single-rule review editing is available
            from each asset's detail page.
          </p>
        </CardContent>
      </Card>
    )
  }

  function buildInput(dryRun: boolean): ReviewBatchInput | string {
    const assetCriteria =
      assetMode === 'assetIds'
        ? { assetIds: Array.from(selectedAssetIds) }
        : { benchmarkIds: Array.from(assetBenchmarkIds) }
    if (
      (assetMode === 'assetIds' && selectedAssetIds.size === 0) ||
      (assetMode === 'benchmarkIds' && assetBenchmarkIds.size === 0)
    ) {
      return 'Select at least one asset (or benchmark) to target.'
    }

    const parsedRuleIds = ruleIdsRaw
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter(Boolean)
    const ruleCriteria =
      ruleMode === 'ruleIds'
        ? { ruleIds: parsedRuleIds }
        : { benchmarkIds: Array.from(ruleBenchmarkIds) }
    if (
      (ruleMode === 'ruleIds' && parsedRuleIds.length === 0) ||
      (ruleMode === 'benchmarkIds' && ruleBenchmarkIds.size === 0)
    ) {
      return 'Select at least one rule (or benchmark) to target.'
    }

    return {
      assets: assetCriteria,
      rules: ruleCriteria,
      action,
      dryRun,
      source: {
        review: {
          result,
          detail,
          comment,
          ...(status ? { status: { label: status } } : {}),
        },
      },
    }
  }

  async function onSubmit(dryRun: boolean) {
    setError(null)
    setLastResponse(null)
    const input = buildInput(dryRun)
    if (typeof input === 'string') {
      setError(input)
      return
    }
    try {
      const resp = await batch.mutateAsync({ collectionId, body: input })
      if (dryRun) {
        setLastResponse({
          kind: 'dry',
          data: resp as ReviewBatchResponseDryRun,
        })
      } else {
        setLastResponse({ kind: 'real', data: resp as ReviewBatchResponse })
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Batch failed.')
    }
  }

  return (
    <div className="space-y-4" data-testid="collection-reviews-tab">
      <Card>
        <CardHeader>
          <CardTitle>Batch Review</CardTitle>
          <p className="text-sm text-[var(--color-muted-foreground)]">
            Apply a single review (result / detail / comment / status) to every
            (asset, rule) pair resolved from the selection below. Use{' '}
            <span className="font-medium">Dry run</span> to preview counts
            before committing.
          </p>
        </CardHeader>
        <CardContent className="space-y-6">
          <section className="space-y-3">
            <h3 className="text-sm font-semibold">Assets</h3>
            <ModeToggle
              testId="batch-asset-mode"
              value={assetMode}
              onChange={setAssetMode}
              options={[
                { value: 'assetIds', label: 'Specific assets' },
                { value: 'benchmarkIds', label: 'All assets with benchmark' },
              ]}
            />
            {assetMode === 'assetIds' ? (
              <MultiSelect
                testId="batch-asset-list"
                loading={assets.isLoading}
                empty="No assets in this Collection yet."
                items={(assets.data ?? []).map((a) => ({
                  value: a.assetId,
                  label: a.name,
                }))}
                selected={selectedAssetIds}
                onChange={setSelectedAssetIds}
              />
            ) : (
              <MultiSelect
                testId="batch-asset-benchmark-list"
                loading={stigs.isLoading}
                empty="No STIGs mapped in this Collection."
                items={(stigs.data ?? []).map((s) => ({
                  value: s.benchmarkId,
                  label: s.title
                    ? `${s.benchmarkId} — ${s.title}`
                    : s.benchmarkId,
                }))}
                selected={assetBenchmarkIds}
                onChange={setAssetBenchmarkIds}
              />
            )}
          </section>

          <section className="space-y-3">
            <h3 className="text-sm font-semibold">Rules</h3>
            <ModeToggle
              testId="batch-rule-mode"
              value={ruleMode}
              onChange={setRuleMode}
              options={[
                { value: 'ruleIds', label: 'Specific rule IDs' },
                { value: 'benchmarkIds', label: 'Every rule in benchmark' },
              ]}
            />
            {ruleMode === 'ruleIds' ? (
              <div className="space-y-1">
                <Label htmlFor="batch-rule-ids">
                  Rule IDs (comma or whitespace separated)
                </Label>
                <Textarea
                  id="batch-rule-ids"
                  data-testid="batch-rule-ids"
                  rows={3}
                  placeholder="SV-12345r1_rule, SV-67890r2_rule"
                  value={ruleIdsRaw}
                  onChange={(e) => setRuleIdsRaw(e.target.value)}
                />
              </div>
            ) : (
              <MultiSelect
                testId="batch-rule-benchmark-list"
                loading={stigs.isLoading}
                empty="No STIGs mapped in this Collection."
                items={(stigs.data ?? []).map((s) => ({
                  value: s.benchmarkId,
                  label: s.title
                    ? `${s.benchmarkId} — ${s.title}`
                    : s.benchmarkId,
                }))}
                selected={ruleBenchmarkIds}
                onChange={setRuleBenchmarkIds}
              />
            )}
          </section>

          <section className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="batch-action">Action</Label>
              <select
                id="batch-action"
                data-testid="batch-action"
                className="w-full rounded-md border border-[var(--color-border)] bg-transparent px-3 py-2 text-sm"
                value={action}
                onChange={(e) =>
                  setAction(e.target.value as 'insert' | 'update' | 'merge')
                }
              >
                <option value="merge">merge (insert + update)</option>
                <option value="insert">insert (skip existing)</option>
                <option value="update">update (skip missing)</option>
              </select>
            </div>
            <div className="space-y-1">
              <Label htmlFor="batch-result">Result</Label>
              <select
                id="batch-result"
                data-testid="batch-result"
                className="w-full rounded-md border border-[var(--color-border)] bg-transparent px-3 py-2 text-sm"
                value={result}
                onChange={(e) => setResult(e.target.value as ReviewResult)}
              >
                {RESULT_OPTIONS.map((r) => (
                  <option key={r} value={r}>
                    {r}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-1 sm:col-span-2">
              <Label htmlFor="batch-detail">Detail</Label>
              <Textarea
                id="batch-detail"
                data-testid="batch-detail"
                rows={3}
                value={detail}
                onChange={(e) => setDetail(e.target.value)}
              />
            </div>
            <div className="space-y-1 sm:col-span-2">
              <Label htmlFor="batch-comment">Comment</Label>
              <Textarea
                id="batch-comment"
                data-testid="batch-comment"
                rows={3}
                value={comment}
                onChange={(e) => setComment(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="batch-status">Status</Label>
              <select
                id="batch-status"
                data-testid="batch-status"
                className="w-full rounded-md border border-[var(--color-border)] bg-transparent px-3 py-2 text-sm"
                value={status}
                onChange={(e) =>
                  setStatus(e.target.value as ReviewStatusLabel | '')
                }
              >
                {STATUS_OPTIONS.map((s) => (
                  <option key={s.value} value={s.value}>
                    {s.label}
                  </option>
                ))}
              </select>
            </div>
          </section>

          {error && (
            <p
              className="text-sm text-red-500"
              data-testid="batch-review-error"
            >
              {error}
            </p>
          )}

          <BatchSummary response={lastResponse} />

          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => void onSubmit(true)}
              disabled={batch.isPending}
              data-testid="batch-dry-run"
            >
              {batch.isPending && <Loader2 className="size-4 animate-spin" />}
              Dry run
            </Button>
            <Button
              type="button"
              onClick={() => void onSubmit(false)}
              disabled={batch.isPending}
              data-testid="batch-submit"
            >
              {batch.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Send className="size-4" />
              )}
              Apply
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card data-testid="collection-imports-card">
        <CardHeader>
          <CardTitle>Imports (CKL / CKLB / XCCDF)</CardTitle>
          <p className="text-sm text-[var(--color-muted-foreground)]">
            File-based imports are deferred to a follow-up milestone — they
            require either client-side checklist parsing or a new server
            multipart upload endpoint. The current API accepts JSON-only
            review payloads via{' '}
            <code>POST /collections/{'{'}cid{'}'}/reviews</code> and the
            per-asset bulk endpoint, both of which the Batch Review form above
            exercises.
          </p>
        </CardHeader>
      </Card>
    </div>
  )
}

function ModeToggle<T extends string>({
  value,
  onChange,
  options,
  testId,
}: {
  value: T
  onChange: (next: T) => void
  options: { value: T; label: string }[]
  testId: string
}) {
  return (
    <div className="flex flex-wrap gap-2" data-testid={testId}>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className={
            'rounded-full border px-3 py-1 text-xs ' +
            (value === o.value
              ? 'border-sky-500 bg-sky-500/10 text-sky-500'
              : 'border-[var(--color-border)] text-[var(--color-muted-foreground)] hover:bg-[var(--color-muted)]/30')
          }
          data-testid={`${testId}-${o.value}`}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

function MultiSelect({
  items,
  selected,
  onChange,
  loading,
  empty,
  testId,
}: {
  items: { value: string; label: string }[]
  selected: Set<string>
  onChange: (next: Set<string>) => void
  loading: boolean
  empty: string
  testId: string
}) {
  if (loading) {
    return (
      <p className="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
        <Loader2 className="size-4 animate-spin" /> Loading…
      </p>
    )
  }
  if (items.length === 0) {
    return (
      <p className="text-sm text-[var(--color-muted-foreground)]">{empty}</p>
    )
  }
  return (
    <div
      className="max-h-48 space-y-1 overflow-y-auto rounded-md border border-[var(--color-border)] p-2 text-sm"
      data-testid={testId}
    >
      {items.map((it) => (
        <label
          key={it.value}
          className="flex cursor-pointer items-center gap-2"
        >
          <Input
            type="checkbox"
            className="size-4"
            checked={selected.has(it.value)}
            onChange={(e) => {
              const next = new Set(selected)
              if (e.target.checked) next.add(it.value)
              else next.delete(it.value)
              onChange(next)
            }}
            data-testid={`${testId}-${it.value}`}
          />
          <span>{it.label}</span>
        </label>
      ))}
    </div>
  )
}

function BatchSummary({
  response,
}: {
  response:
    | { kind: 'real'; data: ReviewBatchResponse }
    | { kind: 'dry'; data: ReviewBatchResponseDryRun }
    | null
}) {
  if (!response) return null
  const isDry = response.kind === 'dry'
  const counts = isDry
    ? {
        a: response.data.willInsert,
        b: response.data.willUpdate,
        c: response.data.willFailValidation,
        aLabel: 'Will insert',
        bLabel: 'Will update',
        cLabel: 'Will fail',
      }
    : {
        a: response.data.inserted,
        b: response.data.updated,
        c: response.data.failedValidation,
        aLabel: 'Inserted',
        bLabel: 'Updated',
        cLabel: 'Failed',
      }
  return (
    <div
      className="space-y-2 rounded-md border border-[var(--color-border)] bg-[var(--color-muted)]/20 p-3 text-sm"
      data-testid={isDry ? 'batch-result-dry' : 'batch-result-real'}
    >
      <p className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
        {isDry ? 'Dry-run preview' : 'Applied'}
      </p>
      <div className="flex flex-wrap gap-4">
        <span data-testid="batch-count-insert">
          {counts.aLabel}: <span className="font-semibold">{counts.a}</span>
        </span>
        <span data-testid="batch-count-update">
          {counts.bLabel}: <span className="font-semibold">{counts.b}</span>
        </span>
        <span data-testid="batch-count-fail">
          {counts.cLabel}:{' '}
          <span className="font-semibold text-red-500">{counts.c}</span>
        </span>
      </div>
      {response.data.validationErrors.length > 0 && (
        <ul
          className="space-y-1 text-xs text-[var(--color-muted-foreground)]"
          data-testid="batch-validation-errors"
        >
          {response.data.validationErrors.slice(0, 10).map((e, idx) => (
            <li key={`${e.assetId ?? '?'}-${e.ruleId ?? '?'}-${idx}`}>
              {(e.assetId ?? '?') + ' / ' + (e.ruleId ?? '?') + ' — ' +
                (e.error ?? 'rejected')}
            </li>
          ))}
          {response.data.validationErrors.length > 10 && (
            <li>
              … and {response.data.validationErrors.length - 10} more.
            </li>
          )}
        </ul>
      )}
    </div>
  )
}
