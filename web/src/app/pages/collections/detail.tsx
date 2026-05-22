// /collections/:collectionId — detail view. Header summarises the
// Collection; tab strip exposes role-aware deep links into the
// per-collection surfaces (Assets / Reviews / Labels / Grants /
// Metrics / History / Exports / POAM). Most tab bodies are stubs for
// the M18c+ milestones; the Overview tab renders today.

import { ArrowLeft, Loader2 } from 'lucide-react'
import { Link, useParams } from 'react-router-dom'

import { AssetsTab } from '../assets/assets-tab'
import { ExportsTab } from './exports-tab'
import { HistoryTab } from './history-tab'
import { MetricsTab } from './metrics-tab'
import { PoamTab } from './poam-tab'
import { ReviewsTab } from './reviews-tab'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCollection, useCurrentUser } from '@/lib/api/hooks'
import { atLeast, roleForCollection, ROLE_LABELS } from '@/lib/auth/roles'

interface TabDef {
  value: string
  label: string
  /** Minimum collection role required to see this tab.
   *  null means anyone with a grant on the collection sees it. */
  minRole: 1 | 2 | 3 | 4 | null
  milestone: string
  blurb: string
}

const TABS: readonly TabDef[] = [
  {
    value: 'overview',
    label: 'Overview',
    minRole: null,
    milestone: 'M18b',
    blurb: '',
  },
  {
    value: 'assets',
    // Assets tab is implemented in M18c — see <AssetsTab/>.
    label: 'Assets',
    minRole: 1,
    milestone: 'M18c',
    blurb:
      'Manage the Collection\u2019s Assets (hardware/software targets) and their STIG assignments.',
  },
  {
    value: 'reviews',
    label: 'Reviews',
    minRole: 1,
    milestone: 'M18d',
    blurb:
      'Batch Review workspace \u2014 apply a single review across many (asset, rule) pairs.',
  },
  {
    value: 'labels',
    label: 'Labels',
    minRole: 2,
    milestone: 'M18c',
    blurb: 'Define Collection-scoped Labels and assign them to Assets.',
  },
  {
    value: 'grants',
    label: 'Grants',
    minRole: 3,
    milestone: 'M18f',
    blurb:
      'Grant users (or user groups) access to this Collection with a role.',
  },
  {
    value: 'metrics',
    label: 'Metrics',
    minRole: 1,
    milestone: 'M18e',
    blurb:
      'Dashboard for the metrics-summary API (collection / asset / stig / label).',
  },
  {
    value: 'history',
    label: 'History',
    minRole: 1,
    milestone: 'M18e',
    blurb:
      'Browse review-history entries with filters; admins can prune old entries.',
  },
  {
    value: 'exports',
    label: 'Exports',
    minRole: 1,
    milestone: 'M18e',
    blurb: 'Download CKL / CKLB / XCCDF bundles for the Collection.',
  },
  {
    value: 'poam',
    label: 'POAM',
    minRole: 1,
    milestone: 'M18e',
    blurb: 'Generate POA&M xlsx (EMASS / MCCAST) from open findings.',
  },
] as const

export function CollectionDetailPage() {
  const { collectionId } = useParams<{ collectionId: string }>()
  const collection = useCollection(collectionId)
  const me = useCurrentUser()

  const role = roleForCollection(me.data, collectionId ?? '')
  const visibleTabs = TABS.filter(
    (t) => t.minRole === null || atLeast(role, t.minRole),
  )

  if (collection.isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
        <Loader2 className="size-4 animate-spin" /> Loading collection…
      </div>
    )
  }
  if (collection.isError || !collection.data) {
    return (
      <div className="space-y-3" data-testid="collection-not-found">
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
    <div className="space-y-6" data-testid="collection-detail-page">
      <Link
        to="/collections"
        className="inline-flex items-center gap-1 text-sm text-sky-500 hover:underline"
      >
        <ArrowLeft className="size-4" /> Back to Collections
      </Link>

      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">{c.name}</h1>
          {c.description && (
            <p className="mt-1 max-w-2xl text-sm text-[var(--color-muted-foreground)]">
              {c.description}
            </p>
          )}
        </div>
        <RoleBadge role={role} />
      </header>

      <Tabs defaultValue="overview" className="space-y-4">
        <TabsList>
          {visibleTabs.map((tab) => (
            <TabsTrigger
              key={tab.value}
              value={tab.value}
              data-testid={`collection-tab-${tab.value}`}
            >
              {tab.label}
            </TabsTrigger>
          ))}
        </TabsList>

        <TabsContent value="overview">
          <OverviewTab collection={c} />
        </TabsContent>

        {visibleTabs.some((t) => t.value === 'assets') && (
          <TabsContent value="assets">
            <AssetsTab collectionId={c.collectionId} role={role} />
          </TabsContent>
        )}

        {visibleTabs.some((t) => t.value === 'reviews') && (
          <TabsContent value="reviews">
            <ReviewsTab collectionId={c.collectionId} role={role} />
          </TabsContent>
        )}

        {visibleTabs.some((t) => t.value === 'metrics') && (
          <TabsContent value="metrics">
            <MetricsTab collectionId={c.collectionId} />
          </TabsContent>
        )}

        {visibleTabs.some((t) => t.value === 'history') && (
          <TabsContent value="history">
            <HistoryTab collectionId={c.collectionId} role={role} />
          </TabsContent>
        )}

        {visibleTabs.some((t) => t.value === 'exports') && (
          <TabsContent value="exports">
            <ExportsTab collectionId={c.collectionId} />
          </TabsContent>
        )}

        {visibleTabs.some((t) => t.value === 'poam') && (
          <TabsContent value="poam">
            <PoamTab collectionId={c.collectionId} />
          </TabsContent>
        )}

        {visibleTabs
          .filter(
            (t) =>
              t.value !== 'overview' &&
              t.value !== 'assets' &&
              t.value !== 'reviews' &&
              t.value !== 'metrics' &&
              t.value !== 'history' &&
              t.value !== 'exports' &&
              t.value !== 'poam',
          )
          .map((tab) => (
            <TabsContent key={tab.value} value={tab.value}>
              <StubTab
                title={tab.label}
                blurb={tab.blurb}
                milestone={tab.milestone}
              />
            </TabsContent>
          ))}
      </Tabs>
    </div>
  )
}

function RoleBadge({ role }: { role: number | null }) {
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
      data-testid="collection-role-badge"
    >
      {ROLE_LABELS[role as 1 | 2 | 3 | 4]}
    </span>
  )
}

function OverviewTab({
  collection,
}: {
  collection: { collectionId: string; name: string; description?: string | null; created?: string }
}) {
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Identifiers</CardTitle>
          <CardDescription>Stable IDs used by the API.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <Field label="Collection ID" value={collection.collectionId} mono />
          <Field label="Name" value={collection.name} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Provenance</CardTitle>
          <CardDescription>When and where the Collection started.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <Field
            label="Created"
            value={collection.created ? new Date(collection.created).toLocaleString() : '\u2014'}
          />
          <Field
            label="Description"
            value={collection.description ?? '\u2014'}
          />
        </CardContent>
      </Card>
    </div>
  )
}

function Field({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex justify-between gap-3">
      <span className="text-[var(--color-muted-foreground)]">{label}</span>
      <span className={mono ? 'font-mono text-xs' : 'text-right'}>{value}</span>
    </div>
  )
}

function StubTab({
  title,
  blurb,
  milestone,
}: {
  title: string
  blurb: string
  milestone: string
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{blurb}</CardDescription>
      </CardHeader>
      <CardContent className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]/80">
        Coming in milestone {milestone}.
      </CardContent>
    </Card>
  )
}
