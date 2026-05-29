// /collections/:collectionId/workspace
//
// Collection management workspace. Loosely modeled after the upstream
// stig-manager "Manage Collection" view: a single info-dense page with
// three regions — Manage panel on the left (name + description +
// Grants/Users/Settings/Metadata/Labels tab strip), Assets table top
// right, STIGs table bottom right.
//
// Both right-side tables drive off /collections/{cid}/metrics/summary/*
// so the same Assessed / Submitted / Accepted / Rejected percentages
// surface here that the Metrics tab uses, but in a one-screen layout
// designed to replace the per-tab click-through.
//
// Routing decision: this is a *new* route alongside the existing
// /collections/:cid tabbed detail. The Collections list points here by
// default; direct links to /collections/:cid keep working.

import {
  ArrowLeft,
  ArrowUpDown,
  Loader2,
  Plus,
  RefreshCw,
  Search,
  Settings,
  Tag,
  Trash2,
  Upload,
  UserPlus,
  Users,
} from 'lucide-react'
import * as React from 'react'
import { Link, useParams } from 'react-router-dom'

import { GrantsTab } from './grants-tab'
import { NewAssetDialog } from '../assets/new-asset-dialog'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  useCollection,
  useCurrentUser,
  useDeleteAsset,
  useMetricsByAsset,
  useMetricsByStig,
  type MetricsSummary,
  type MetricsSummaryAggAsset,
  type MetricsSummaryAggStig,
} from '@/lib/api/hooks'
import {
  atLeast,
  roleForCollection,
  ROLE_LABELS,
  type CollectionRoleId,
} from '@/lib/auth/roles'
import { cn } from '@/lib/utils'

export function CollectionWorkspacePage() {
  const { collectionId } = useParams<{ collectionId: string }>()
  const collection = useCollection(collectionId)
  const me = useCurrentUser()
  const role = roleForCollection(me.data, collectionId ?? '')

  if (collection.isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
        <Loader2 className="size-4 animate-spin" /> Loading collection…
      </div>
    )
  }

  if (collection.isError || !collection.data || !collectionId) {
    return (
      <div className="space-y-3" data-testid="collection-workspace-not-found">
        <Link
          to="/collections"
          className="inline-flex items-center gap-1 text-sm text-sky-500 hover:underline"
        >
          <ArrowLeft className="size-4" /> Back to Collections
        </Link>
        <p className="text-sm text-red-500">
          Failed to load collection {collectionId}:{' '}
          {(collection.error as Error | null)?.message ?? 'Not found.'}
        </p>
      </div>
    )
  }

  const c = collection.data
  return (
    <div
      className="flex flex-col gap-3"
      data-testid="collection-workspace"
    >
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Link
            to="/collections"
            className="inline-flex items-center gap-1 text-sm text-sky-500 hover:underline"
            data-testid="workspace-back-link"
          >
            <ArrowLeft className="size-4" /> Collections
          </Link>
          <span className="text-sm text-[var(--color-muted-foreground)]">/</span>
          <h1 className="text-xl font-semibold tracking-tight" data-testid="workspace-collection-name">
            {c.name}
          </h1>
          <RoleBadge role={role} />
        </div>
        <div className="text-xs text-[var(--color-muted-foreground)]">
          Need the legacy tabbed view?{' '}
          <Link
            to={`/collections/${collectionId}`}
            className="text-sky-500 hover:underline"
            data-testid="workspace-legacy-link"
          >
            open detail
          </Link>
        </div>
      </header>

      {/* Body: 1 column on mobile, 2 columns (manage | tables) on lg+. */}
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-[28rem_minmax(0,1fr)]">
        <ManagePanel collectionId={collectionId} collection={c} role={role} />
        <div className="flex min-w-0 flex-col gap-3">
          <AssetsMetricsTable collectionId={collectionId} role={role} />
          <StigsMetricsTable collectionId={collectionId} role={role} />
        </div>
      </div>
    </div>
  )
}

// ---- Manage panel (left) ------------------------------------------------

interface ManagePanelProps {
  collectionId: string
  collection: { name: string; description?: string | null; collectionId: string }
  role: CollectionRoleId | null
}

function ManagePanel({ collectionId, collection, role }: ManagePanelProps) {
  // The five Manage tabs match the upstream layout. Grants is the only
  // tab with a full implementation today (re-uses the existing
  // GrantsTab component); the others are intentional placeholders
  // pointing to where their content will land.
  return (
    <Card className="self-start" data-testid="workspace-manage-panel">
      <CardHeader className="space-y-1 pb-3">
        <CardTitle className="text-base">Manage Collection</CardTitle>
        <CardDescription className="text-xs">
          Properties, grants, and labels for this Collection.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-3">
          <Field label="Name" value={collection.name} />
          <Field
            label="Description"
            value={collection.description?.trim() || '—'}
            multiline
          />
        </div>

        <Tabs defaultValue="grants" data-testid="manage-tabs">
          <TabsList className="w-full justify-start gap-1 overflow-x-auto">
            <TabsTrigger value="grants" data-testid="manage-tab-grants">
              <UserPlus className="mr-1 size-3.5" /> Grants
            </TabsTrigger>
            <TabsTrigger value="users" data-testid="manage-tab-users">
              <Users className="mr-1 size-3.5" /> Users
            </TabsTrigger>
            <TabsTrigger value="settings" data-testid="manage-tab-settings">
              <Settings className="mr-1 size-3.5" /> Settings
            </TabsTrigger>
            <TabsTrigger value="metadata" data-testid="manage-tab-metadata">
              Metadata
            </TabsTrigger>
            <TabsTrigger value="labels" data-testid="manage-tab-labels">
              <Tag className="mr-1 size-3.5" /> Labels
            </TabsTrigger>
          </TabsList>

          <TabsContent value="grants" className="mt-3">
            <GrantsTab collectionId={collectionId} role={role} />
          </TabsContent>

          <TabsContent value="users" className="mt-3">
            <Placeholder
              title="Users"
              blurb="Direct user list for this Collection (projection of Grants by subject)."
              link={`/admin/users`}
              linkLabel="Open Users admin →"
            />
          </TabsContent>

          <TabsContent value="settings" className="mt-3">
            <Placeholder
              title="Settings"
              blurb="Per-Collection settings: field visibility, history retention, status workflow."
            />
          </TabsContent>

          <TabsContent value="metadata" className="mt-3">
            <Placeholder
              title="Metadata"
              blurb="Free-form key/value metadata on the Collection."
            />
          </TabsContent>

          <TabsContent value="labels" className="mt-3">
            <Placeholder
              title="Labels"
              blurb="Define color-coded Labels and tag Assets with them."
              link={`/collections/${collectionId}`}
              linkLabel="Open legacy Labels tab →"
            />
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  )
}

function Field({
  label,
  value,
  multiline = false,
}: {
  label: string
  value: string
  multiline?: boolean
}) {
  return (
    <div className="space-y-1">
      <div className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
        {label}
      </div>
      <div
        className={cn(
          'rounded-md border border-[var(--color-border)] bg-[var(--color-muted)]/20 px-3 py-2 text-sm',
          multiline && 'whitespace-pre-wrap',
        )}
      >
        {value}
      </div>
    </div>
  )
}

function Placeholder({
  title,
  blurb,
  link,
  linkLabel,
}: {
  title: string
  blurb: string
  link?: string
  linkLabel?: string
}) {
  return (
    <Card className="border-dashed">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 text-xs text-[var(--color-muted-foreground)]">
        <p>{blurb}</p>
        {link && linkLabel && (
          <Link to={link} className="text-sky-500 hover:underline">
            {linkLabel}
          </Link>
        )}
      </CardContent>
    </Card>
  )
}

// ---- Assets metrics table (top right) -----------------------------------

interface TableProps {
  collectionId: string
  role: CollectionRoleId | null
}

type AssetSortKey =
  | 'name'
  | 'stigs'
  | 'assessments'
  | 'assessedPct'
  | 'submittedPct'
  | 'acceptedPct'
  | 'rejectedPct'

function AssetsMetricsTable({ collectionId, role }: TableProps) {
  const metrics = useMetricsByAsset(collectionId)
  const del = useDeleteAsset()
  const canCreate = atLeast(role, 2)
  const canDelete = atLeast(role, 3)
  const [selected, setSelected] = React.useState<Set<string>>(new Set())
  const [query, setQuery] = React.useState('')
  const [sortKey, setSortKey] = React.useState<AssetSortKey>('name')
  const [sortDir, setSortDir] = React.useState<'asc' | 'desc'>('asc')
  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [deleteError, setDeleteError] = React.useState<string | null>(null)

  const rows = React.useMemo(() => {
    const list = metrics.data ?? []
    const filtered = query
      ? list.filter((r) =>
          r.name.toLowerCase().includes(query.trim().toLowerCase()),
        )
      : list
    const cmp = (a: MetricsSummaryAggAsset, b: MetricsSummaryAggAsset) => {
      const av = sortValueAsset(a, sortKey)
      const bv = sortValueAsset(b, sortKey)
      if (typeof av === 'number' && typeof bv === 'number') {
        return sortDir === 'asc' ? av - bv : bv - av
      }
      return sortDir === 'asc'
        ? String(av).localeCompare(String(bv))
        : String(bv).localeCompare(String(av))
    }
    return [...filtered].sort(cmp)
  }, [metrics.data, query, sortKey, sortDir])

  function toggleAll() {
    if (selected.size === rows.length && rows.length > 0) {
      setSelected(new Set())
    } else {
      setSelected(new Set(rows.map((r) => r.assetId)))
    }
  }
  function toggleOne(id: string) {
    setSelected((s) => {
      const next = new Set(s)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }
  function flipSort(key: AssetSortKey) {
    if (key === sortKey) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortKey(key)
      setSortDir(key === 'name' ? 'asc' : 'desc')
    }
  }

  async function bulkDelete() {
    setDeleteError(null)
    const ids = [...selected]
    if (!ids.length) return
    if (
      !window.confirm(
        `Delete ${ids.length} asset${ids.length === 1 ? '' : 's'}? This cannot be undone.`,
      )
    ) {
      return
    }
    try {
      // Serial deletes — bulk endpoint isn't in the API surface.
      for (const id of ids) {
        await del.mutateAsync({ assetId: id })
      }
      setSelected(new Set())
      void metrics.refetch()
    } catch (e) {
      setDeleteError((e as Error).message)
    }
  }

  return (
    <Card data-testid="workspace-assets-card">
      <CardHeader className="flex flex-wrap items-center justify-between gap-2 pb-3">
        <div>
          <CardTitle className="text-base">Assets</CardTitle>
          <CardDescription className="text-xs">
            {metrics.data
              ? `${metrics.data.length} asset${metrics.data.length === 1 ? '' : 's'}`
              : '—'}
          </CardDescription>
        </div>
        <div className="flex flex-wrap items-center gap-1">
          <ToolbarButton
            icon={<Plus className="size-3.5" />}
            label="Create"
            disabled={!canCreate}
            onClick={() => setDialogOpen(true)}
            testid="assets-toolbar-create"
          />
          <ToolbarButton
            icon={<Upload className="size-3.5" />}
            label="Import"
            disabled
            title="File-based imports live on the Reviews tab (M22)."
            testid="assets-toolbar-import"
          />
          <ToolbarButton
            icon={<Trash2 className="size-3.5" />}
            label={`Delete${selected.size ? ` (${selected.size})` : ''}`}
            disabled={!canDelete || selected.size === 0 || del.isPending}
            onClick={bulkDelete}
            testid="assets-toolbar-delete"
            destructive
          />
          <ToolbarButton
            icon={<RefreshCw className="size-3.5" />}
            label="Refresh"
            onClick={() => metrics.refetch()}
            disabled={metrics.isFetching}
            testid="assets-toolbar-refresh"
          />
        </div>
      </CardHeader>

      <CardContent className="space-y-3">
        <div className="flex items-center gap-2">
          <div className="relative w-full max-w-sm">
            <Search className="pointer-events-none absolute left-2 top-1/2 size-4 -translate-y-1/2 text-[var(--color-muted-foreground)]" />
            <Input
              placeholder="Filter by name…"
              className="pl-8"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              data-testid="assets-filter"
            />
          </div>
          {metrics.isFetching && !metrics.isLoading && (
            <Loader2 className="size-4 animate-spin text-[var(--color-muted-foreground)]" />
          )}
          {deleteError && (
            <span className="text-xs text-red-500" data-testid="assets-delete-error">
              {deleteError}
            </span>
          )}
        </div>

        <div className="overflow-x-auto rounded-md border border-[var(--color-border)]">
          <table className="w-full text-xs" data-testid="workspace-assets-table">
            <thead className="bg-[var(--color-muted)]/30 text-left uppercase tracking-wider text-[var(--color-muted-foreground)]">
              <tr>
                <th className="w-8 px-2 py-2">
                  <input
                    type="checkbox"
                    checked={
                      rows.length > 0 && selected.size === rows.length
                    }
                    onChange={toggleAll}
                    aria-label="Select all assets"
                    data-testid="assets-select-all"
                  />
                </th>
                <SortableTh
                  label="Asset"
                  active={sortKey === 'name'}
                  dir={sortDir}
                  onClick={() => flipSort('name')}
                />
                <th className="px-2 py-2">Labels</th>
                <SortableTh
                  label="STIGs"
                  active={sortKey === 'stigs'}
                  dir={sortDir}
                  onClick={() => flipSort('stigs')}
                  className="text-right"
                />
                <SortableTh
                  label="Rules"
                  active={sortKey === 'assessments'}
                  dir={sortDir}
                  onClick={() => flipSort('assessments')}
                  className="text-right"
                />
                <SortableTh
                  label="Assessed"
                  active={sortKey === 'assessedPct'}
                  dir={sortDir}
                  onClick={() => flipSort('assessedPct')}
                />
                <SortableTh
                  label="Submitted"
                  active={sortKey === 'submittedPct'}
                  dir={sortDir}
                  onClick={() => flipSort('submittedPct')}
                />
                <SortableTh
                  label="Accepted"
                  active={sortKey === 'acceptedPct'}
                  dir={sortDir}
                  onClick={() => flipSort('acceptedPct')}
                />
                <SortableTh
                  label="Rejected"
                  active={sortKey === 'rejectedPct'}
                  dir={sortDir}
                  onClick={() => flipSort('rejectedPct')}
                />
              </tr>
            </thead>
            <tbody>
              {metrics.isLoading && (
                <tr>
                  <td
                    colSpan={9}
                    className="px-2 py-6 text-center text-[var(--color-muted-foreground)]"
                  >
                    Loading assets…
                  </td>
                </tr>
              )}
              {metrics.isError && (
                <tr>
                  <td
                    colSpan={9}
                    className="px-2 py-6 text-center text-red-500"
                  >
                    Failed to load:{' '}
                    {(metrics.error as Error | null)?.message ?? 'Unknown error.'}
                  </td>
                </tr>
              )}
              {metrics.data && rows.length === 0 && (
                <tr>
                  <td
                    colSpan={9}
                    className="px-2 py-6 text-center text-[var(--color-muted-foreground)]"
                  >
                    {query
                      ? 'No assets match that filter.'
                      : 'No assets yet. Create one to get started.'}
                  </td>
                </tr>
              )}
              {rows.map((r) => (
                <tr
                  key={r.assetId}
                  className={cn(
                    'border-t border-[var(--color-border)] hover:bg-[var(--color-muted)]/20',
                    selected.has(r.assetId) && 'bg-sky-500/5',
                  )}
                  data-testid={`asset-row-${r.assetId}`}
                >
                  <td className="px-2 py-2">
                    <input
                      type="checkbox"
                      checked={selected.has(r.assetId)}
                      onChange={() => toggleOne(r.assetId)}
                      aria-label={`Select asset ${r.name}`}
                      data-testid={`asset-row-checkbox-${r.assetId}`}
                    />
                  </td>
                  <td className="px-2 py-2">
                    <Link
                      to={`/collections/${collectionId}/assets/${r.assetId}`}
                      className="text-sky-500 hover:underline"
                    >
                      {r.name}
                    </Link>
                  </td>
                  <td className="px-2 py-2">
                    <LabelChips labels={r.labels} />
                  </td>
                  <td className="px-2 py-2 text-right tabular-nums">
                    {r.benchmarkIds?.length ?? 0}
                  </td>
                  <td className="px-2 py-2 text-right tabular-nums">
                    {r.metrics.assessments}
                  </td>
                  <ProgressTd metrics={r.metrics} kind="assessed" />
                  <ProgressTd metrics={r.metrics} kind="submitted" />
                  <ProgressTd metrics={r.metrics} kind="accepted" />
                  <ProgressTd metrics={r.metrics} kind="rejected" />
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </CardContent>

      {canCreate && (
        <NewAssetDialog
          open={dialogOpen}
          onOpenChange={setDialogOpen}
          collectionId={collectionId}
        />
      )}
    </Card>
  )
}

function sortValueAsset(
  r: MetricsSummaryAggAsset,
  key: AssetSortKey,
): string | number {
  switch (key) {
    case 'name':
      return r.name
    case 'stigs':
      return r.benchmarkIds?.length ?? 0
    case 'assessments':
      return r.metrics.assessments
    case 'assessedPct':
      return percent(r.metrics, 'assessed')
    case 'submittedPct':
      return percent(r.metrics, 'submitted')
    case 'acceptedPct':
      return percent(r.metrics, 'accepted')
    case 'rejectedPct':
      return percent(r.metrics, 'rejected')
  }
}

// ---- STIGs metrics table (bottom right) ---------------------------------

type StigSortKey =
  | 'benchmarkId'
  | 'revisionStr'
  | 'assessments'
  | 'assets'
  | 'assessedPct'
  | 'submittedPct'
  | 'acceptedPct'
  | 'rejectedPct'

function StigsMetricsTable({ collectionId }: TableProps) {
  const metrics = useMetricsByStig(collectionId)
  const [query, setQuery] = React.useState('')
  const [sortKey, setSortKey] = React.useState<StigSortKey>('benchmarkId')
  const [sortDir, setSortDir] = React.useState<'asc' | 'desc'>('asc')
  const [selected, setSelected] = React.useState<Set<string>>(new Set())

  const rows = React.useMemo(() => {
    const list = metrics.data ?? []
    const filtered = query
      ? list.filter((r) =>
          r.benchmarkId.toLowerCase().includes(query.trim().toLowerCase()),
        )
      : list
    const cmp = (a: MetricsSummaryAggStig, b: MetricsSummaryAggStig) => {
      const av = sortValueStig(a, sortKey)
      const bv = sortValueStig(b, sortKey)
      if (typeof av === 'number' && typeof bv === 'number') {
        return sortDir === 'asc' ? av - bv : bv - av
      }
      return sortDir === 'asc'
        ? String(av).localeCompare(String(bv))
        : String(bv).localeCompare(String(av))
    }
    return [...filtered].sort(cmp)
  }, [metrics.data, query, sortKey, sortDir])

  function toggleAll() {
    if (selected.size === rows.length && rows.length > 0) {
      setSelected(new Set())
    } else {
      setSelected(new Set(rows.map((r) => r.benchmarkId)))
    }
  }
  function toggleOne(id: string) {
    setSelected((s) => {
      const next = new Set(s)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }
  function flipSort(key: StigSortKey) {
    if (key === sortKey) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortKey(key)
      setSortDir(key === 'benchmarkId' ? 'asc' : 'desc')
    }
  }

  return (
    <Card data-testid="workspace-stigs-card">
      <CardHeader className="flex flex-wrap items-center justify-between gap-2 pb-3">
        <div>
          <CardTitle className="text-base">STIGs</CardTitle>
          <CardDescription className="text-xs">
            {metrics.data
              ? `${metrics.data.length} STIG${metrics.data.length === 1 ? '' : 's'}`
              : '—'}
          </CardDescription>
        </div>
        <div className="flex flex-wrap items-center gap-1">
          <ToolbarButton
            icon={<Plus className="size-3.5" />}
            label="Assign STIG"
            disabled
            title="Attach STIGs from the Asset detail page (M21b)."
            testid="stigs-toolbar-assign"
          />
          <ToolbarButton
            icon={<Trash2 className="size-3.5" />}
            label={`Unassign${selected.size ? ` (${selected.size})` : ''}`}
            disabled
            title="Unassign STIGs from Assets one at a time on the Asset detail page."
            testid="stigs-toolbar-unassign"
          />
          <ToolbarButton
            icon={<RefreshCw className="size-3.5" />}
            label="Refresh"
            onClick={() => metrics.refetch()}
            disabled={metrics.isFetching}
            testid="stigs-toolbar-refresh"
          />
        </div>
      </CardHeader>

      <CardContent className="space-y-3">
        <div className="flex items-center gap-2">
          <div className="relative w-full max-w-sm">
            <Search className="pointer-events-none absolute left-2 top-1/2 size-4 -translate-y-1/2 text-[var(--color-muted-foreground)]" />
            <Input
              placeholder="Filter by benchmarkId…"
              className="pl-8"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              data-testid="stigs-filter"
            />
          </div>
          {metrics.isFetching && !metrics.isLoading && (
            <Loader2 className="size-4 animate-spin text-[var(--color-muted-foreground)]" />
          )}
        </div>

        <div className="overflow-x-auto rounded-md border border-[var(--color-border)]">
          <table className="w-full text-xs" data-testid="workspace-stigs-table">
            <thead className="bg-[var(--color-muted)]/30 text-left uppercase tracking-wider text-[var(--color-muted-foreground)]">
              <tr>
                <th className="w-8 px-2 py-2">
                  <input
                    type="checkbox"
                    checked={
                      rows.length > 0 && selected.size === rows.length
                    }
                    onChange={toggleAll}
                    aria-label="Select all STIGs"
                    data-testid="stigs-select-all"
                  />
                </th>
                <SortableTh
                  label="BenchmarkId"
                  active={sortKey === 'benchmarkId'}
                  dir={sortDir}
                  onClick={() => flipSort('benchmarkId')}
                />
                <SortableTh
                  label="Revision"
                  active={sortKey === 'revisionStr'}
                  dir={sortDir}
                  onClick={() => flipSort('revisionStr')}
                />
                <SortableTh
                  label="Rules"
                  active={sortKey === 'assessments'}
                  dir={sortDir}
                  onClick={() => flipSort('assessments')}
                  className="text-right"
                />
                <SortableTh
                  label="Assets"
                  active={sortKey === 'assets'}
                  dir={sortDir}
                  onClick={() => flipSort('assets')}
                  className="text-right"
                />
                <SortableTh
                  label="Assessed"
                  active={sortKey === 'assessedPct'}
                  dir={sortDir}
                  onClick={() => flipSort('assessedPct')}
                />
                <SortableTh
                  label="Submitted"
                  active={sortKey === 'submittedPct'}
                  dir={sortDir}
                  onClick={() => flipSort('submittedPct')}
                />
                <SortableTh
                  label="Accepted"
                  active={sortKey === 'acceptedPct'}
                  dir={sortDir}
                  onClick={() => flipSort('acceptedPct')}
                />
                <SortableTh
                  label="Rejected"
                  active={sortKey === 'rejectedPct'}
                  dir={sortDir}
                  onClick={() => flipSort('rejectedPct')}
                />
              </tr>
            </thead>
            <tbody>
              {metrics.isLoading && (
                <tr>
                  <td
                    colSpan={9}
                    className="px-2 py-6 text-center text-[var(--color-muted-foreground)]"
                  >
                    Loading STIGs…
                  </td>
                </tr>
              )}
              {metrics.isError && (
                <tr>
                  <td colSpan={9} className="px-2 py-6 text-center text-red-500">
                    Failed to load:{' '}
                    {(metrics.error as Error | null)?.message ?? 'Unknown error.'}
                  </td>
                </tr>
              )}
              {metrics.data && rows.length === 0 && (
                <tr>
                  <td
                    colSpan={9}
                    className="px-2 py-6 text-center text-[var(--color-muted-foreground)]"
                  >
                    {query
                      ? 'No STIGs match that filter.'
                      : 'No STIGs assigned yet. Attach one to an Asset to start.'}
                  </td>
                </tr>
              )}
              {rows.map((r) => (
                <tr
                  key={r.benchmarkId}
                  className={cn(
                    'border-t border-[var(--color-border)] hover:bg-[var(--color-muted)]/20',
                    selected.has(r.benchmarkId) && 'bg-sky-500/5',
                  )}
                  data-testid={`stig-row-${r.benchmarkId}`}
                >
                  <td className="px-2 py-2">
                    <input
                      type="checkbox"
                      checked={selected.has(r.benchmarkId)}
                      onChange={() => toggleOne(r.benchmarkId)}
                      aria-label={`Select STIG ${r.benchmarkId}`}
                      data-testid={`stig-row-checkbox-${r.benchmarkId}`}
                    />
                  </td>
                  <td className="px-2 py-2">
                    <Link
                      to={`/library/${encodeURIComponent(r.benchmarkId)}`}
                      className="text-sky-500 hover:underline"
                    >
                      {r.benchmarkId}
                    </Link>
                  </td>
                  <td className="px-2 py-2 font-mono text-[11px]">
                    {r.revisionStr ?? '—'}
                  </td>
                  <td className="px-2 py-2 text-right tabular-nums">
                    {r.metrics.assessments}
                  </td>
                  <td className="px-2 py-2 text-right tabular-nums">
                    {r.assets}
                  </td>
                  <ProgressTd metrics={r.metrics} kind="assessed" />
                  <ProgressTd metrics={r.metrics} kind="submitted" />
                  <ProgressTd metrics={r.metrics} kind="accepted" />
                  <ProgressTd metrics={r.metrics} kind="rejected" />
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </CardContent>
    </Card>
  )
}

function sortValueStig(
  r: MetricsSummaryAggStig,
  key: StigSortKey,
): string | number {
  switch (key) {
    case 'benchmarkId':
      return r.benchmarkId
    case 'revisionStr':
      return r.revisionStr ?? ''
    case 'assessments':
      return r.metrics.assessments
    case 'assets':
      return r.assets
    case 'assessedPct':
      return percent(r.metrics, 'assessed')
    case 'submittedPct':
      return percent(r.metrics, 'submitted')
    case 'acceptedPct':
      return percent(r.metrics, 'accepted')
    case 'rejectedPct':
      return percent(r.metrics, 'rejected')
  }
}

// ---- Shared bits --------------------------------------------------------

type PctKind = 'assessed' | 'submitted' | 'accepted' | 'rejected'

// Matches upstream's percentage formulas in collectionManager.js
// (assessed / assessments * 100; submitted = (submitted+accepted+rejected)/assessments * 100;
// accepted = accepted/assessments * 100; rejected = rejected/assessments * 100).
function percent(m: MetricsSummary['metrics'], kind: PctKind): number {
  if (!m.assessments) return 0
  const s = m.statuses
  switch (kind) {
    case 'assessed':
      return (m.assessed / m.assessments) * 100
    case 'submitted':
      return ((s.submitted + s.accepted + s.rejected) / m.assessments) * 100
    case 'accepted':
      return (s.accepted / m.assessments) * 100
    case 'rejected':
      return (s.rejected / m.assessments) * 100
  }
}

function ProgressTd({
  metrics,
  kind,
}: {
  metrics: MetricsSummary['metrics']
  kind: PctKind
}) {
  const pct = percent(metrics, kind)
  return (
    <td className="px-2 py-2">
      <ProgressBar pct={pct} kind={kind} />
    </td>
  )
}

function ProgressBar({ pct, kind }: { pct: number; kind: PctKind }) {
  const display = pct === 0 ? '0%' : pct >= 99.95 ? '100%' : `${pct.toFixed(0)}%`
  const colorClass =
    kind === 'rejected'
      ? 'bg-red-500/60'
      : kind === 'accepted'
        ? 'bg-emerald-500/60'
        : kind === 'submitted'
          ? 'bg-sky-500/60'
          : 'bg-emerald-500/40'
  return (
    <div
      className="relative h-5 w-20 overflow-hidden rounded-sm border border-[var(--color-border)] bg-[var(--color-muted)]/30"
      data-testid={`progress-${kind}`}
      data-pct={pct.toFixed(2)}
    >
      <div
        className={cn('absolute inset-y-0 left-0', colorClass)}
        style={{ width: `${Math.min(100, Math.max(0, pct))}%` }}
      />
      <span className="relative z-10 flex h-full items-center justify-center text-[10px] font-medium tabular-nums">
        {display}
      </span>
    </div>
  )
}

function ToolbarButton({
  icon,
  label,
  onClick,
  disabled,
  destructive,
  title,
  testid,
}: {
  icon: React.ReactNode
  label: string
  onClick?: () => void
  disabled?: boolean
  destructive?: boolean
  title?: string
  testid?: string
}) {
  return (
    <Button
      size="sm"
      variant={destructive ? 'destructive' : 'outline'}
      onClick={onClick}
      disabled={disabled}
      title={title}
      data-testid={testid}
    >
      {icon}
      <span className="ml-1">{label}</span>
    </Button>
  )
}

function SortableTh({
  label,
  active,
  dir,
  onClick,
  className,
}: {
  label: string
  active: boolean
  dir: 'asc' | 'desc'
  onClick: () => void
  className?: string
}) {
  return (
    <th
      className={cn(
        'cursor-pointer select-none px-2 py-2 hover:text-[var(--color-foreground)]',
        active && 'text-[var(--color-foreground)]',
        className,
      )}
      onClick={onClick}
    >
      <span className="inline-flex items-center gap-1">
        {label}
        {active ? (
          <span className="text-[10px]">{dir === 'asc' ? '▲' : '▼'}</span>
        ) : (
          <ArrowUpDown className="size-3 opacity-40" />
        )}
      </span>
    </th>
  )
}

function LabelChips({
  labels,
}: {
  labels: MetricsSummaryAggAsset['labels']
}) {
  if (!labels || labels.length === 0) {
    return <span className="text-[var(--color-muted-foreground)]">—</span>
  }
  return (
    <div className="flex flex-wrap gap-1">
      {labels.map((l, i) => (
        <span
          key={l.labelId ?? `${i}-${l.name}`}
          className="rounded-sm border border-[var(--color-border)] px-1.5 py-0.5 text-[10px]"
          style={l.color ? { borderColor: `#${l.color}`, color: `#${l.color}` } : undefined}
        >
          {l.name ?? l.labelId ?? '—'}
        </span>
      ))}
    </div>
  )
}

function RoleBadge({ role }: { role: CollectionRoleId | null }) {
  if (!role) {
    return (
      <span className="rounded-full border border-[var(--color-border)] px-2 py-1 text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
        No grant
      </span>
    )
  }
  return (
    <span
      className="rounded-full border border-sky-500/40 bg-sky-500/10 px-2 py-1 text-xs uppercase tracking-wider text-sky-500"
      data-testid="workspace-role-badge"
    >
      {ROLE_LABELS[role]}
    </span>
  )
}
