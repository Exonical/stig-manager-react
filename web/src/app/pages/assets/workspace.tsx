// /collections/:collectionId/assets/:assetId/workspace
//
// Three-pane review workspace for an Asset.
//
//   Left   ~30%   Rule list for the selected STIG (filter + sort + row select)
//   Middle ~40%   Selected rule's RuleDetail (vuln discussion, check, fix, ccis)
//   Right  ~30%   Review Resources tabs (History / Other Assets / Status text)
//                 stacked above the Evaluation form
//
// State that survives reload is encoded in the URL as
// ?stig=<benchmarkId>&rule=<ruleId>.  Selecting a row pushes the
// `rule` param; selecting a STIG pushes both.
//
// Keyboard shortcuts (only when the rule list has focus or no input is
// focused — text inputs/textareas swallow the keys naturally):
//
//   j / ArrowDown   next rule
//   k / ArrowUp     prev rule
//   f               result -> fail
//   p               result -> pass
//   n               result -> notapplicable
//   u               result -> notchecked
//   Ctrl+Enter      save the current evaluation
//   Ctrl+Shift+Enter save + advance to next unreviewed rule
//   /               focus the search box

import {
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Loader2,
} from 'lucide-react'
import * as React from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  useAsset,
  useAssetStigs,
  useCurrentUser,
  usePutReview,
  useReview,
  useReviewHistory,
  useReviewsByAsset,
  useRuleByRuleId,
  useRulesByRevision,
  type Review,
  type ReviewResult,
  type ReviewStatusLabel,
  type Rule,
} from '@/lib/api/hooks'
import { atLeast, roleForCollection } from '@/lib/auth/roles'

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

type StatusFilter = '' | 'unreviewed' | ReviewStatusLabel

export function AssetReviewWorkspacePage() {
  const { collectionId, assetId } = useParams<{
    collectionId: string
    assetId: string
  }>()
  const [params, setParams] = useSearchParams()
  const me = useCurrentUser()
  const role = roleForCollection(me.data, collectionId ?? '')
  const canEdit = atLeast(role, 2)

  const asset = useAsset(assetId)
  const stigs = useAssetStigs(assetId)
  const reviews = useReviewsByAsset(collectionId, assetId)

  // STIG selection. Default to the first assigned STIG if the URL
  // has none.
  const stigParam = params.get('stig') ?? ''
  const selectedStig =
    stigs.data?.find((s) => s.benchmarkId === stigParam) ??
    stigs.data?.[0]

  React.useEffect(() => {
    if (selectedStig && !stigParam) {
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev)
          next.set('stig', selectedStig.benchmarkId)
          return next
        },
        { replace: true },
      )
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedStig?.benchmarkId, stigParam])

  // Rules for the selected STIG (basic projection — full detail is
  // loaded per-rule on selection).
  const rules = useRulesByRevision(
    selectedStig?.benchmarkId,
    selectedStig?.revisionStr ?? (selectedStig ? 'latest' : undefined),
  )

  const reviewsByRule = React.useMemo(() => {
    const m = new Map<string, Review>()
    for (const r of reviews.data ?? []) {
      const id = r.ruleId ?? r.ruleIds?.[0]
      if (id) m.set(id, r)
    }
    return m
  }, [reviews.data])

  // ---- Rule list filters ------------------------------------------
  const [search, setSearch] = React.useState('')
  const [severity, setSeverity] = React.useState<'' | 'high' | 'medium' | 'low'>('')
  const [statusFilter, setStatusFilter] = React.useState<StatusFilter>('')
  const [resultFilter, setResultFilter] = React.useState<'' | ReviewResult>('')
  const searchRef = React.useRef<HTMLInputElement | null>(null)

  const filteredRules = React.useMemo(() => {
    if (!rules.data) return [] as Rule[]
    const needle = search.trim().toLowerCase()
    return rules.data.filter((r) => {
      if (severity && r.severity !== severity) return false
      const rev = reviewsByRule.get(r.ruleId)
      if (statusFilter === 'unreviewed' && rev) return false
      if (
        statusFilter !== '' &&
        statusFilter !== 'unreviewed' &&
        rev?.status?.label !== statusFilter
      )
        return false
      if (resultFilter && rev?.result !== resultFilter) return false
      if (!needle) return true
      return (
        r.ruleId.toLowerCase().includes(needle) ||
        r.title.toLowerCase().includes(needle) ||
        (r.groupId?.toLowerCase().includes(needle) ?? false)
      )
    })
  }, [rules.data, search, severity, statusFilter, resultFilter, reviewsByRule])

  // ---- Selected rule ----------------------------------------------
  const ruleParam = params.get('rule') ?? ''
  const selectedRuleId = React.useMemo(() => {
    if (ruleParam && filteredRules.some((r) => r.ruleId === ruleParam)) {
      return ruleParam
    }
    return filteredRules[0]?.ruleId ?? ''
  }, [ruleParam, filteredRules])

  // Whenever the URL `rule` param drifts out of the filtered set,
  // fall back to the first row but don't loop on it.
  React.useEffect(() => {
    if (!selectedRuleId) return
    if (selectedRuleId !== ruleParam) {
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev)
          next.set('rule', selectedRuleId)
          return next
        },
        { replace: true },
      )
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedRuleId])

  function selectRule(ruleId: string) {
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        next.set('rule', ruleId)
        return next
      },
      { replace: false },
    )
  }

  // Auto-scroll the selected row into view.
  React.useEffect(() => {
    const el = document.querySelector(
      `[data-testid="ws-rule-row-${selectedRuleId}"]`,
    )
    if (el) (el as HTMLElement).scrollIntoView({ block: 'nearest' })
  }, [selectedRuleId])

  const ruleDetail = useRuleByRuleId(selectedRuleId || undefined)
  const review = useReview(collectionId, assetId, selectedRuleId || undefined)
  const history = useReviewHistory(collectionId, {
    assetId,
    ruleId: selectedRuleId || undefined,
  })

  // ---- Evaluation form state --------------------------------------
  const [result, setResult] = React.useState<ReviewResult>('notchecked')
  const [detail, setDetail] = React.useState('')
  const [comment, setComment] = React.useState('')
  const [status, setStatus] = React.useState<ReviewStatusLabel | ''>('')
  const [savedAt, setSavedAt] = React.useState<string | null>(null)
  const [error, setError] = React.useState<string | null>(null)
  const put = usePutReview()

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
    setSavedAt(null)
    setError(null)
  }, [review.isPending, review.data, selectedRuleId])

  async function save(advance: boolean) {
    if (!collectionId || !assetId || !selectedRuleId) return
    setError(null)
    setSavedAt(null)
    try {
      await put.mutateAsync({
        collectionId,
        assetId,
        ruleId: selectedRuleId,
        body: {
          result,
          detail,
          comment,
          ...(status ? { status } : {}),
        },
      })
      setSavedAt(new Date().toLocaleTimeString())
      if (advance) {
        const idx = filteredRules.findIndex(
          (r) => r.ruleId === selectedRuleId,
        )
        const nextRule =
          filteredRules.slice(idx + 1).find((r) => !reviewsByRule.has(r.ruleId)) ??
          filteredRules[idx + 1]
        if (nextRule) selectRule(nextRule.ruleId)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save review.')
    }
  }

  // ---- Keyboard shortcuts -----------------------------------------
  React.useEffect(() => {
    if (!canEdit) return
    function onKey(e: KeyboardEvent) {
      const tgt = e.target as HTMLElement | null
      const tag = tgt?.tagName ?? ''
      const isText =
        tag === 'INPUT' ||
        tag === 'TEXTAREA' ||
        tag === 'SELECT' ||
        (tgt?.isContentEditable ?? false)

      if (e.ctrlKey && e.key === 'Enter') {
        e.preventDefault()
        void save(e.shiftKey)
        return
      }
      if (isText) return

      if (e.key === '/') {
        e.preventDefault()
        searchRef.current?.focus()
        return
      }
      const arrow = e.key === 'ArrowDown' || e.key === 'ArrowUp'
      if (e.key === 'j' || e.key === 'k' || arrow) {
        e.preventDefault()
        const idx = filteredRules.findIndex((r) => r.ruleId === selectedRuleId)
        const dir = e.key === 'j' || e.key === 'ArrowDown' ? 1 : -1
        const nxt = filteredRules[Math.min(
          Math.max(idx + dir, 0),
          filteredRules.length - 1,
        )]
        if (nxt) selectRule(nxt.ruleId)
        return
      }
      if (e.key === 'f') setResult('fail')
      else if (e.key === 'p') setResult('pass')
      else if (e.key === 'n') setResult('notapplicable')
      else if (e.key === 'u') setResult('notchecked')
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filteredRules, selectedRuleId, canEdit, result, detail, comment, status])

  // ---- Render -----------------------------------------------------
  if (!collectionId || !assetId) return null

  if (asset.isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
        <Loader2 className="size-4 animate-spin" /> Loading workspace…
      </div>
    )
  }

  if (asset.isError || !asset.data) {
    return (
      <div className="space-y-3" data-testid="workspace-not-found">
        <Link
          to={`/collections/${collectionId}`}
          className="inline-flex items-center gap-1 text-sm text-sky-500 hover:underline"
        >
          <ArrowLeft className="size-4" /> Back to Collection
        </Link>
        <p className="text-sm">Asset not found.</p>
      </div>
    )
  }

  const counts = countByResult(filteredRules, reviewsByRule)

  return (
    <div className="flex h-[calc(100vh-7rem)] flex-col gap-3" data-testid="review-workspace">
      {/* Breadcrumb + STIG picker -------------------------------- */}
      <header className="flex flex-wrap items-center gap-3">
        <Link
          to={`/collections/${collectionId}/assets/${assetId}`}
          className="inline-flex items-center gap-1 text-sm text-sky-500 hover:underline"
          data-testid="workspace-back-link"
        >
          <ArrowLeft className="size-4" /> {asset.data.name}
        </Link>
        <span className="text-sm text-[var(--color-muted-foreground)]">/</span>
        <label className="text-sm">
          <span className="mr-2 text-[var(--color-muted-foreground)]">STIG</span>
          <select
            value={selectedStig?.benchmarkId ?? ''}
            onChange={(e) => {
              const next = new URLSearchParams(params)
              next.set('stig', e.target.value)
              next.delete('rule')
              setParams(next)
            }}
            className="rounded-md border border-[var(--color-border)] bg-transparent px-2 py-1 text-sm"
            data-testid="workspace-stig-select"
            disabled={stigs.isLoading}
          >
            {(stigs.data ?? []).map((s) => (
              <option key={s.benchmarkId} value={s.benchmarkId}>
                {s.benchmarkId} ({s.revisionStr ?? 'latest'})
              </option>
            ))}
            {(stigs.data?.length ?? 0) === 0 && <option value="">no STIGs assigned</option>}
          </select>
        </label>
        {selectedStig && (
          <span className="text-xs text-[var(--color-muted-foreground)]" data-testid="workspace-counts">
            {filteredRules.length} of {rules.data?.length ?? 0} rules ·{' '}
            <span className="text-rose-500">{counts.fail} fail</span> ·{' '}
            <span className="text-emerald-500">{counts.pass} pass</span> ·{' '}
            <span className="text-amber-500">{counts.na} N/A</span> ·{' '}
            <span>{counts.unreviewed} unreviewed</span>
          </span>
        )}
      </header>

      {/* Three-pane body ----------------------------------------- */}
      <div className="grid flex-1 grid-cols-12 gap-3 overflow-hidden">
        <RuleListPane
          rules={filteredRules}
          all={rules.data ?? null}
          loading={rules.isLoading || stigs.isLoading}
          selectedRuleId={selectedRuleId}
          reviewsByRule={reviewsByRule}
          onSelect={selectRule}
          search={search}
          setSearch={setSearch}
          searchRef={searchRef}
          severity={severity}
          setSeverity={setSeverity}
          statusFilter={statusFilter}
          setStatusFilter={setStatusFilter}
          resultFilter={resultFilter}
          setResultFilter={setResultFilter}
        />

        <RuleDetailPane
          ruleId={selectedRuleId}
          loading={ruleDetail.isLoading}
          detail={ruleDetail.data ?? null}
        />

        <ReviewPane
          ruleId={selectedRuleId}
          canEdit={canEdit}
          review={review.data ?? null}
          history={history.data ?? []}
          historyLoading={history.isLoading}
          result={result}
          setResult={setResult}
          detail={detail}
          setDetail={setDetail}
          comment={comment}
          setComment={setComment}
          status={status}
          setStatus={setStatus}
          savedAt={savedAt}
          error={error}
          pending={put.isPending}
          onSave={() => save(false)}
          onSaveAdvance={() => save(true)}
          onPrev={() => {
            const i = filteredRules.findIndex((r) => r.ruleId === selectedRuleId)
            if (i > 0) selectRule(filteredRules[i - 1].ruleId)
          }}
          onNext={() => {
            const i = filteredRules.findIndex((r) => r.ruleId === selectedRuleId)
            if (i >= 0 && i < filteredRules.length - 1)
              selectRule(filteredRules[i + 1].ruleId)
          }}
        />
      </div>
    </div>
  )
}

// ---- Rule list pane --------------------------------------------------

function RuleListPane({
  rules,
  all,
  loading,
  selectedRuleId,
  reviewsByRule,
  onSelect,
  search,
  setSearch,
  searchRef,
  severity,
  setSeverity,
  statusFilter,
  setStatusFilter,
  resultFilter,
  setResultFilter,
}: {
  rules: Rule[]
  all: Rule[] | null
  loading: boolean
  selectedRuleId: string
  reviewsByRule: Map<string, Review>
  onSelect: (ruleId: string) => void
  search: string
  setSearch: (s: string) => void
  searchRef: React.RefObject<HTMLInputElement | null>
  severity: '' | 'high' | 'medium' | 'low'
  setSeverity: (s: '' | 'high' | 'medium' | 'low') => void
  statusFilter: StatusFilter
  setStatusFilter: (s: StatusFilter) => void
  resultFilter: '' | ReviewResult
  setResultFilter: (s: '' | ReviewResult) => void
}) {
  return (
    <Card
      className="col-span-12 flex min-h-0 flex-col overflow-hidden lg:col-span-4"
      data-testid="workspace-rule-list"
    >
      <CardHeader className="space-y-2 pb-2">
        <div className="flex items-center gap-2">
          <Input
            ref={searchRef}
            placeholder="Filter rules (press / to focus)"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            data-testid="workspace-search"
          />
        </div>
        <div className="flex flex-wrap gap-1 text-xs">
          <FilterPill active={severity === ''} onClick={() => setSeverity('')}>All sev</FilterPill>
          <FilterPill active={severity === 'high'} onClick={() => setSeverity('high')}>CAT I</FilterPill>
          <FilterPill active={severity === 'medium'} onClick={() => setSeverity('medium')}>CAT II</FilterPill>
          <FilterPill active={severity === 'low'} onClick={() => setSeverity('low')}>CAT III</FilterPill>
        </div>
        <div className="flex flex-wrap gap-1 text-xs">
          <FilterPill active={statusFilter === ''} onClick={() => setStatusFilter('')}>Any status</FilterPill>
          <FilterPill active={statusFilter === 'unreviewed'} onClick={() => setStatusFilter('unreviewed')}>Unreviewed</FilterPill>
          <FilterPill active={statusFilter === 'saved'} onClick={() => setStatusFilter('saved')}>Saved</FilterPill>
          <FilterPill active={statusFilter === 'submitted'} onClick={() => setStatusFilter('submitted')}>Submitted</FilterPill>
        </div>
        <div className="flex flex-wrap gap-1 text-xs">
          <FilterPill active={resultFilter === ''} onClick={() => setResultFilter('')}>Any result</FilterPill>
          <FilterPill active={resultFilter === 'fail'} onClick={() => setResultFilter('fail')}>Fail</FilterPill>
          <FilterPill active={resultFilter === 'pass'} onClick={() => setResultFilter('pass')}>Pass</FilterPill>
          <FilterPill active={resultFilter === 'notapplicable'} onClick={() => setResultFilter('notapplicable')}>N/A</FilterPill>
        </div>
      </CardHeader>
      <CardContent className="flex-1 overflow-y-auto p-0">
        {loading && (
          <p className="flex items-center gap-2 p-4 text-sm text-[var(--color-muted-foreground)]">
            <Loader2 className="size-4 animate-spin" /> Loading rules…
          </p>
        )}
        {!loading && all && all.length === 0 && (
          <p className="p-4 text-sm text-[var(--color-muted-foreground)]">
            This STIG has no rules.
          </p>
        )}
        {!loading && all && all.length > 0 && rules.length === 0 && (
          <p className="p-4 text-sm text-[var(--color-muted-foreground)]">
            No rules match the current filter.
          </p>
        )}
        {!loading && rules.length > 0 && (
          <ul className="divide-y divide-[var(--color-border)] text-sm">
            {rules.map((r) => {
              const rev = reviewsByRule.get(r.ruleId)
              const selected = r.ruleId === selectedRuleId
              return (
                <li
                  key={r.ruleId}
                  data-testid={`ws-rule-row-${r.ruleId}`}
                  className={
                    'cursor-pointer px-3 py-2 ' +
                    (selected
                      ? 'bg-sky-500/10'
                      : 'hover:bg-[var(--color-muted)]/30')
                  }
                  onClick={() => onSelect(r.ruleId)}
                >
                  <div className="flex items-center gap-2">
                    <SeverityPill severity={r.severity} />
                    <span className="font-mono text-xs text-[var(--color-muted-foreground)]">
                      {r.groupId ?? r.ruleId}
                    </span>
                    <span className="ml-auto">
                      <ResultPill rev={rev} />
                    </span>
                  </div>
                  <p className="mt-1 line-clamp-2 text-xs">{r.title}</p>
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

// ---- Rule detail pane ------------------------------------------------

function RuleDetailPane({
  ruleId,
  loading,
  detail,
}: {
  ruleId: string
  loading: boolean
  detail: import('@/lib/api').RuleDetail | null
}) {
  return (
    <Card
      className="col-span-12 flex min-h-0 flex-col overflow-hidden lg:col-span-5"
      data-testid="workspace-rule-detail"
    >
      <CardHeader className="space-y-1 pb-2">
        <CardTitle className="font-mono text-base" data-testid="workspace-rule-id">
          {ruleId || 'No rule selected'}
        </CardTitle>
        {detail?.title && (
          <p className="text-sm text-[var(--color-muted-foreground)]">
            {detail.title}
          </p>
        )}
      </CardHeader>
      <CardContent className="flex-1 space-y-4 overflow-y-auto text-sm">
        {loading && (
          <p className="flex items-center gap-2 text-[var(--color-muted-foreground)]">
            <Loader2 className="size-4 animate-spin" /> Loading rule detail…
          </p>
        )}
        {!loading && detail && (
          <>
            {detail.detail?.vulnDiscussion && (
              <Section title="Vulnerability Discussion">
                <p className="whitespace-pre-wrap">{detail.detail.vulnDiscussion}</p>
              </Section>
            )}
            {detail.check?.content && (
              <Section title="Check">
                <pre className="whitespace-pre-wrap font-mono text-xs">
                  {detail.check.content}
                </pre>
              </Section>
            )}
            {detail.fix?.text && (
              <Section title="Fix">
                <pre className="whitespace-pre-wrap font-mono text-xs">
                  {detail.fix.text}
                </pre>
              </Section>
            )}
            {detail.ccis && detail.ccis.length > 0 && (
              <Section title="CCIs">
                <ul className="space-y-1">
                  {detail.ccis.map((c) => (
                    <li key={c.cci ?? '?'} className="font-mono text-xs">
                      {c.cci}
                    </li>
                  ))}
                </ul>
              </Section>
            )}
          </>
        )}
        {!loading && !detail && ruleId && (
          <p className="text-[var(--color-muted-foreground)]">
            No detail available for this rule.
          </p>
        )}
      </CardContent>
    </Card>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section>
      <h3 className="mb-1 text-xs font-semibold uppercase tracking-wider text-[var(--color-muted-foreground)]">
        {title}
      </h3>
      {children}
    </section>
  )
}

// ---- Review pane (history + evaluation form) ------------------------

type ReviewPaneTab = 'history' | 'other' | 'status'

function ReviewPane({
  ruleId,
  canEdit,
  review,
  history,
  historyLoading,
  result,
  setResult,
  detail,
  setDetail,
  comment,
  setComment,
  status,
  setStatus,
  savedAt,
  error,
  pending,
  onSave,
  onSaveAdvance,
  onPrev,
  onNext,
}: {
  ruleId: string
  canEdit: boolean
  review: Review | null
  history: import('@/lib/api').ReviewHistoryAsset[]
  historyLoading: boolean
  result: ReviewResult
  setResult: (r: ReviewResult) => void
  detail: string
  setDetail: (s: string) => void
  comment: string
  setComment: (s: string) => void
  status: ReviewStatusLabel | ''
  setStatus: (s: ReviewStatusLabel | '') => void
  savedAt: string | null
  error: string | null
  pending: boolean
  onSave: () => void
  onSaveAdvance: () => void
  onPrev: () => void
  onNext: () => void
}) {
  const [tab, setTab] = React.useState<ReviewPaneTab>('history')
  // Reset to history when rule changes; keeps the panel showing the
  // most useful info first.
  React.useEffect(() => {
    setTab('history')
  }, [ruleId])

  return (
    <div
      className="col-span-12 flex min-h-0 flex-col gap-3 overflow-hidden lg:col-span-3"
      data-testid="workspace-review-pane"
    >
      <Card className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <CardHeader className="pb-2">
          <div className="flex flex-wrap gap-1 text-xs" data-testid="workspace-tabs">
            <FilterPill active={tab === 'history'} onClick={() => setTab('history')}>History</FilterPill>
            <FilterPill active={tab === 'other'} onClick={() => setTab('other')}>Other Assets</FilterPill>
            <FilterPill active={tab === 'status'} onClick={() => setTab('status')}>Status Text</FilterPill>
          </div>
        </CardHeader>
        <CardContent className="flex-1 overflow-y-auto text-sm">
          {tab === 'history' && (
            <HistoryList loading={historyLoading} history={history} />
          )}
          {tab === 'other' && (
            <p className="text-[var(--color-muted-foreground)]">
              Cross-asset view for this rule is coming soon (M23b). Use the
              Reviews tab on the Collection for now.
            </p>
          )}
          {tab === 'status' && (
            <div className="space-y-2">
              <p className="text-[var(--color-muted-foreground)]">
                Status reviewer note (if any) on the current review.
              </p>
              <p className="whitespace-pre-wrap">
                {review?.status?.text || '— none —'}
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="flex flex-col" data-testid="workspace-eval-form">
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="text-base">Evaluation</CardTitle>
          <div className="flex gap-1">
            <button
              type="button"
              onClick={onPrev}
              className="rounded border border-[var(--color-border)] p-1 text-xs hover:bg-[var(--color-muted)]/30"
              data-testid="workspace-prev"
              aria-label="Previous rule"
            >
              <ChevronLeft className="size-4" />
            </button>
            <button
              type="button"
              onClick={onNext}
              className="rounded border border-[var(--color-border)] p-1 text-xs hover:bg-[var(--color-muted)]/30"
              data-testid="workspace-next"
              aria-label="Next rule"
            >
              <ChevronRight className="size-4" />
            </button>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="ws-result">Result</Label>
            <select
              id="ws-result"
              disabled={!canEdit || !ruleId}
              className="block w-full rounded-md border border-[var(--color-border)] bg-transparent px-3 py-2 text-sm"
              value={result}
              onChange={(e) => setResult(e.target.value as ReviewResult)}
              data-testid="workspace-result-select"
            >
              {RESULT_OPTIONS.map((opt) => (
                <option key={opt} value={opt}>
                  {opt}
                </option>
              ))}
            </select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="ws-detail">Detail</Label>
            <Textarea
              id="ws-detail"
              rows={3}
              value={detail}
              onChange={(e) => setDetail(e.target.value)}
              disabled={!canEdit || !ruleId}
              data-testid="workspace-detail-input"
              placeholder="What did you observe?"
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="ws-comment">Comment</Label>
            <Textarea
              id="ws-comment"
              rows={2}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              disabled={!canEdit || !ruleId}
              data-testid="workspace-comment-input"
              placeholder="Mitigations, follow-ups."
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="ws-status">Status</Label>
            <select
              id="ws-status"
              disabled={!canEdit || !ruleId}
              className="block w-full rounded-md border border-[var(--color-border)] bg-transparent px-3 py-2 text-sm"
              value={status}
              onChange={(e) => setStatus(e.target.value as ReviewStatusLabel | '')}
              data-testid="workspace-status-select"
            >
              {STATUS_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label}
                </option>
              ))}
            </select>
          </div>

          {error && (
            <p className="text-sm text-red-500" data-testid="workspace-error">
              {error}
            </p>
          )}
          {savedAt && (
            <p className="text-sm text-emerald-500" data-testid="workspace-saved">
              Saved at {savedAt}.
            </p>
          )}

          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              onClick={onSave}
              disabled={!canEdit || !ruleId || pending}
              data-testid="workspace-save"
              size="sm"
            >
              {pending ? <Loader2 className="size-4 animate-spin" /> : null}
              Save
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={onSaveAdvance}
              disabled={!canEdit || !ruleId || pending}
              data-testid="workspace-save-advance"
              size="sm"
            >
              Save + next
            </Button>
            <span className="text-[10px] text-[var(--color-muted-foreground)]">
              Ctrl+Enter to save · Ctrl+Shift+Enter to advance
            </span>
          </div>

          {review && (
            <p className="border-t border-[var(--color-border)] pt-2 text-[10px] text-[var(--color-muted-foreground)]">
              Last touched by{' '}
              <span className="font-medium">{review.username ?? '—'}</span> ·{' '}
              {review.ts ? new Date(review.ts).toLocaleString() : '—'} ·{' '}
              status {review.status?.label ?? '—'}
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function HistoryList({
  loading,
  history,
}: {
  loading: boolean
  history: import('@/lib/api').ReviewHistoryAsset[]
}) {
  const entries = React.useMemo(() => {
    const rows: import('@/lib/api').ReviewHistoryEntry[] = []
    for (const a of history) {
      for (const r of a.reviewHistories) {
        for (const e of r.history) rows.push(e)
      }
    }
    rows.sort((a, b) => (a.ts < b.ts ? 1 : -1))
    return rows.slice(0, 20)
  }, [history])
  if (loading) {
    return (
      <p className="flex items-center gap-2 text-[var(--color-muted-foreground)]">
        <Loader2 className="size-4 animate-spin" /> Loading history…
      </p>
    )
  }
  if (entries.length === 0) {
    return (
      <p className="text-[var(--color-muted-foreground)]">
        No prior reviews for this rule on this asset.
      </p>
    )
  }
  return (
    <ul className="space-y-2" data-testid="workspace-history">
      {entries.map((e, idx) => (
        <li
          key={`${e.ts}-${idx}`}
          className="rounded border border-[var(--color-border)] p-2 text-xs"
        >
          <div className="flex items-center justify-between gap-2">
            <span className="font-mono text-[10px] text-[var(--color-muted-foreground)]">
              {new Date(e.ts).toLocaleString()}
            </span>
            <ResultPill rev={{ result: e.result, status: e.status }} />
          </div>
          <p className="mt-1 text-[var(--color-muted-foreground)]">
            {e.username ?? 'unknown'} · {e.status?.label ?? '—'}
          </p>
          {e.detail && (
            <p className="mt-1 line-clamp-3 whitespace-pre-wrap">{e.detail}</p>
          )}
        </li>
      ))}
    </ul>
  )
}

// ---- Bits ------------------------------------------------------------

function FilterPill({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        'rounded-full border px-2 py-0.5 text-[11px] ' +
        (active
          ? 'border-sky-500 bg-sky-500/10 text-sky-500'
          : 'border-[var(--color-border)] text-[var(--color-muted-foreground)] hover:bg-[var(--color-muted)]/30')
      }
    >
      {children}
    </button>
  )
}

function SeverityPill({ severity }: { severity: string }) {
  const cls =
    severity === 'high'
      ? 'border-rose-500 bg-rose-500/10 text-rose-500'
      : severity === 'medium'
        ? 'border-amber-500 bg-amber-500/10 text-amber-500'
        : 'border-sky-500 bg-sky-500/10 text-sky-500'
  const label =
    severity === 'high' ? 'I' : severity === 'medium' ? 'II' : 'III'
  return (
    <span
      className={
        'inline-block rounded-full border px-1.5 py-0 text-[10px] font-semibold ' +
        cls
      }
    >
      CAT {label}
    </span>
  )
}

function ResultPill({
  rev,
}: {
  rev: { result?: string; status?: { label?: string } } | undefined
}) {
  if (!rev?.result) {
    return (
      <span className="rounded-full border border-[var(--color-border)] px-1.5 py-0 text-[10px] uppercase tracking-wider text-[var(--color-muted-foreground)]">
        unreviewed
      </span>
    )
  }
  const cls =
    rev.result === 'fail'
      ? 'border-rose-500 bg-rose-500/10 text-rose-500'
      : rev.result === 'pass'
        ? 'border-emerald-500 bg-emerald-500/10 text-emerald-500'
        : 'border-amber-500 bg-amber-500/10 text-amber-500'
  return (
    <span
      className={
        'rounded-full border px-1.5 py-0 text-[10px] font-medium ' + cls
      }
      data-testid={`ws-result-${rev.result}`}
    >
      {rev.result}
      {rev.status?.label ? ` · ${rev.status.label}` : ''}
    </span>
  )
}

function countByResult(
  rules: Rule[],
  reviewsByRule: Map<string, Review>,
): { fail: number; pass: number; na: number; unreviewed: number } {
  let fail = 0,
    pass = 0,
    na = 0,
    unreviewed = 0
  for (const r of rules) {
    const rev = reviewsByRule.get(r.ruleId)
    if (!rev) {
      unreviewed++
      continue
    }
    if (rev.result === 'fail') fail++
    else if (rev.result === 'pass') pass++
    else if (rev.result === 'notapplicable') na++
  }
  return { fail, pass, na, unreviewed }
}
