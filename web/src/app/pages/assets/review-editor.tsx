// /collections/:collectionId/assets/:assetId/rules/:ruleId
//
// Single-rule review editor. PUTs the entire Review (result + detail +
// comment + optional status). Loads the existing review (or 204 if
// none) and the rule metadata for context.
//
// Status semantics (upstream):
// - omitting `status` from a PUT either keeps the existing value or
//   resets to `saved` per the Collection's resetCriteria. We provide
//   a status picker for the user to explicitly opt-in to "submitted"
//   (or "saved"). Accept/reject are admin actions and live in M18d.

import { ArrowLeft, Loader2 } from 'lucide-react'
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
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  useAsset,
  usePutReview,
  useReview,
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
  { value: '', label: 'Keep current (saved)' },
  { value: 'saved', label: 'Saved' },
  { value: 'submitted', label: 'Submitted' },
]

export function ReviewEditorPage() {
  const { collectionId, assetId, ruleId } = useParams<{
    collectionId: string
    assetId: string
    ruleId: string
  }>()
  const asset = useAsset(assetId)
  const review = useReview(collectionId, assetId, ruleId)
  const put = usePutReview()

  const [result, setResult] = React.useState<ReviewResult>('notchecked')
  const [detail, setDetail] = React.useState('')
  const [comment, setComment] = React.useState('')
  const [status, setStatus] = React.useState<ReviewStatusLabel | ''>('')
  const [error, setError] = React.useState<string | null>(null)
  const [savedAt, setSavedAt] = React.useState<string | null>(null)

  // Seed from the loaded review (if any).
  React.useEffect(() => {
    if (review.isPending) return
    if (review.data) {
      setResult(review.data.result)
      setDetail(review.data.detail ?? '')
      setComment(review.data.comment ?? '')
      setStatus('')
    } else {
      setResult('notchecked')
      setDetail('')
      setComment('')
      setStatus('')
    }
  }, [review.isPending, review.data])

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!collectionId || !assetId || !ruleId) return
    setError(null)
    setSavedAt(null)
    try {
      const body = {
        result,
        detail,
        comment,
        ...(status ? { status } : {}),
      }
      await put.mutateAsync({ collectionId, assetId, ruleId, body })
      setSavedAt(new Date().toLocaleTimeString())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save review.')
    }
  }

  const r = review.data
  const ruleSummary = r?.rule
  return (
    <div className="space-y-6" data-testid="review-editor-page">
      <Link
        to={`/collections/${collectionId}/assets/${assetId}`}
        className="inline-flex items-center gap-1 text-sm text-sky-500 hover:underline"
      >
        <ArrowLeft className="size-4" /> Back to {asset.data?.name ?? 'Asset'}
      </Link>

      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">
          Review{' '}
          <span className="font-mono text-base text-[var(--color-muted-foreground)]">
            {ruleId}
          </span>
        </h1>
        {ruleSummary && (
          <p className="text-sm text-[var(--color-muted-foreground)]">
            {ruleSummary.title}
          </p>
        )}
      </header>

      {review.isLoading && (
        <p className="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
          <Loader2 className="size-4 animate-spin" /> Loading existing review…
        </p>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Finding</CardTitle>
          <CardDescription>
            Record the outcome of this rule against the asset.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="review-result">Result</Label>
              <select
                id="review-result"
                className="block w-full rounded-md border border-[var(--color-border)] bg-transparent px-3 py-2 text-sm"
                value={result}
                onChange={(e) => setResult(e.target.value as ReviewResult)}
                data-testid="review-result-select"
              >
                {RESULT_OPTIONS.map((opt) => (
                  <option key={opt} value={opt}>
                    {opt}
                  </option>
                ))}
              </select>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="review-detail">Detail</Label>
              <Textarea
                id="review-detail"
                rows={4}
                value={detail}
                onChange={(e) => setDetail(e.target.value)}
                placeholder="What did you observe?"
                data-testid="review-detail-input"
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="review-comment">Comment</Label>
              <Textarea
                id="review-comment"
                rows={4}
                value={comment}
                onChange={(e) => setComment(e.target.value)}
                placeholder="Additional context, mitigations, or follow-ups."
                data-testid="review-comment-input"
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="review-status">Status</Label>
              <select
                id="review-status"
                className="block w-full rounded-md border border-[var(--color-border)] bg-transparent px-3 py-2 text-sm"
                value={status}
                onChange={(e) =>
                  setStatus(e.target.value as ReviewStatusLabel | '')
                }
                data-testid="review-status-select"
              >
                {STATUS_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>

            {error && (
              <p className="text-sm text-red-500" data-testid="review-error">
                {error}
              </p>
            )}
            {savedAt && (
              <p className="text-sm text-emerald-500" data-testid="review-saved">
                Saved at {savedAt}.
              </p>
            )}

            <div className="flex justify-end">
              <Button
                type="submit"
                disabled={put.isPending}
                data-testid="review-submit"
              >
                {put.isPending ? (
                  <>
                    <Loader2 className="size-4 animate-spin" /> Saving…
                  </>
                ) : (
                  'Save Review'
                )}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {r && (
        <Card>
          <CardHeader>
            <CardTitle>Last touch</CardTitle>
            <CardDescription>Who edited this review most recently.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-1 text-sm">
            <p>
              <span className="text-[var(--color-muted-foreground)]">User: </span>
              {r.username ?? '\u2014'}
            </p>
            <p>
              <span className="text-[var(--color-muted-foreground)]">When: </span>
              {r.ts ? new Date(r.ts).toLocaleString() : '\u2014'}
            </p>
            <p>
              <span className="text-[var(--color-muted-foreground)]">Status: </span>
              {r.status?.label ?? '\u2014'}
            </p>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
