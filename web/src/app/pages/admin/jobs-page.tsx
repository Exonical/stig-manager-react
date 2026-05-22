// /admin/jobs — manage background jobs.
//
// Right column lists every Job; left column shows the task registry.
// Selecting a Job loads its run history and tails the latest run's
// output. Operators with `stig-manager:op` (write) can create new
// Jobs, kick off ad-hoc runs, and delete Jobs.

import { Loader2, Play, Plus, Trash2 } from 'lucide-react'
import * as React from 'react'

import { JobCreateDialog } from './job-create-dialog'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  useDeleteJob,
  useJob,
  useJobRunOutput,
  useJobRuns,
  useJobTasks,
  useJobs,
  useStartJobRun,
  type Job,
  type JobRun,
} from '@/lib/api/hooks'
import { useAuth } from '@/lib/auth/auth-context'
import { hasScope } from '@/lib/auth/scopes'

export function JobsPage() {
  const { status } = useAuth()
  const user = status.kind === 'signed-in' ? status.user : null
  const canWrite = hasScope(user, 'stig-manager:op')

  const tasks = useJobTasks()
  const jobs = useJobs()
  const del = useDeleteJob()
  const run = useStartJobRun()

  const [selectedJobId, setSelectedJobId] = React.useState<string | null>(null)
  const [createOpen, setCreateOpen] = React.useState(false)

  // Auto-select the first job once data lands so the right pane has content.
  React.useEffect(() => {
    if (!selectedJobId && (jobs.data?.length ?? 0) > 0) {
      setSelectedJobId(jobs.data![0]!.jobId)
    }
  }, [jobs.data, selectedJobId])

  const selectedJob = useJob(selectedJobId ?? undefined)
  const jobRuns = useJobRuns(selectedJobId ?? undefined)
  const latestRunId = jobRuns.data?.[0]?.runId
  const runOutput = useJobRunOutput(latestRunId, { refetchIntervalMs: 3_000 })

  async function onRun(jobId: string) {
    try {
      await run.mutateAsync(jobId)
    } catch (err) {
      window.alert(err instanceof Error ? err.message : 'Failed to start run.')
    }
  }

  async function onDelete(job: Job) {
    const ok = window.confirm(`Delete job "${job.name}"?`)
    if (!ok) return
    try {
      await del.mutateAsync(job.jobId)
      if (selectedJobId === job.jobId) setSelectedJobId(null)
    } catch (err) {
      window.alert(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <div className="space-y-6" data-testid="admin-jobs-page">
      <div className="grid gap-4 lg:grid-cols-3">
        {/* Task registry */}
        <Card className="lg:col-span-1">
          <CardHeader>
            <CardTitle>Tasks</CardTitle>
            <CardDescription>Built-in task registry.</CardDescription>
          </CardHeader>
          <CardContent>
            {tasks.isLoading && (
              <p className="text-sm text-[var(--color-muted-foreground)]">
                Loading tasks…
              </p>
            )}
            {tasks.isError && (
              <p className="text-sm text-red-500">
                Failed to load tasks: {(tasks.error as Error).message}
              </p>
            )}
            <ul
              className="divide-y divide-[var(--color-border)]"
              data-testid="job-tasks-list"
            >
              {(tasks.data ?? []).map((t) => (
                <li key={t.taskId} className="py-2 text-sm">
                  <div className="font-medium font-mono">{t.name}</div>
                  {t.description && (
                    <p className="mt-0.5 text-xs text-[var(--color-muted-foreground)]">
                      {t.description}
                    </p>
                  )}
                </li>
              ))}
              {!tasks.isLoading && (tasks.data?.length ?? 0) === 0 && (
                <li className="py-2 text-sm text-[var(--color-muted-foreground)]">
                  No tasks registered.
                </li>
              )}
            </ul>
          </CardContent>
        </Card>

        {/* Job list */}
        <Card className="lg:col-span-2">
          <CardHeader>
            <div className="flex flex-wrap items-end justify-between gap-3">
              <div>
                <CardTitle>Jobs</CardTitle>
                <CardDescription>
                  Scheduled and ad-hoc background jobs.
                </CardDescription>
              </div>
              {canWrite && (
                <Button
                  size="sm"
                  onClick={() => setCreateOpen(true)}
                  disabled={(tasks.data?.length ?? 0) === 0}
                  data-testid="new-job-button"
                >
                  <Plus className="size-4" /> New Job
                </Button>
              )}
            </div>
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto rounded-md border border-[var(--color-border)]">
              <table className="w-full text-sm" data-testid="jobs-table">
                <thead className="bg-[var(--color-muted)]/30 text-left text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
                  <tr>
                    <th className="px-3 py-2">Name</th>
                    <th className="px-3 py-2">Tasks</th>
                    <th className="px-3 py-2">Last run</th>
                    <th className="px-3 py-2 text-right">Runs</th>
                    <th className="w-32 px-3 py-2 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {jobs.isLoading && (
                    <tr>
                      <td
                        colSpan={5}
                        className="px-3 py-6 text-center text-[var(--color-muted-foreground)]"
                      >
                        Loading jobs…
                      </td>
                    </tr>
                  )}
                  {jobs.isError && (
                    <tr>
                      <td colSpan={5} className="px-3 py-6 text-center text-red-500">
                        Failed to load jobs: {(jobs.error as Error).message}
                      </td>
                    </tr>
                  )}
                  {!jobs.isLoading && (jobs.data?.length ?? 0) === 0 && (
                    <tr>
                      <td
                        colSpan={5}
                        className="px-3 py-6 text-center text-[var(--color-muted-foreground)]"
                        data-testid="jobs-empty"
                      >
                        No jobs defined yet.
                      </td>
                    </tr>
                  )}
                  {(jobs.data ?? []).map((j) => {
                    const selected = j.jobId === selectedJobId
                    return (
                      <tr
                        key={j.jobId}
                        className={
                          'cursor-pointer border-t border-[var(--color-border)] ' +
                          (selected ? 'bg-[var(--color-accent)]/30' : 'hover:bg-[var(--color-accent)]/10')
                        }
                        onClick={() => setSelectedJobId(j.jobId)}
                        data-testid={`job-row-${j.jobId}`}
                      >
                        <td className="px-3 py-2 font-medium">{j.name}</td>
                        <td className="px-3 py-2 text-[var(--color-muted-foreground)]">
                          {(j.tasks ?? []).map((t) => t.name).join(', ') || '—'}
                        </td>
                        <td className="px-3 py-2 text-[var(--color-muted-foreground)]">
                          <RunBadge run={j.lastRun ?? null} />
                        </td>
                        <td className="px-3 py-2 text-right">
                          {j.runCount ?? 0}
                        </td>
                        <td className="px-3 py-2 text-right">
                          <div className="flex justify-end gap-1">
                            {canWrite && (
                              <>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={(e) => {
                                    e.stopPropagation()
                                    void onRun(j.jobId)
                                  }}
                                  data-testid={`job-run-${j.jobId}`}
                                  disabled={run.isPending}
                                  aria-label={`Run ${j.name}`}
                                >
                                  <Play className="size-3.5" />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={(e) => {
                                    e.stopPropagation()
                                    void onDelete(j)
                                  }}
                                  data-testid={`job-delete-${j.jobId}`}
                                  disabled={del.isPending}
                                  aria-label={`Delete ${j.name}`}
                                >
                                  <Trash2 className="size-3.5 text-red-500" />
                                </Button>
                              </>
                            )}
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Selected job runs + latest run output */}
      {selectedJobId && (
        <div className="grid gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Runs</CardTitle>
              <CardDescription>
                {selectedJob.data?.name
                  ? `History for ${selectedJob.data.name}`
                  : 'Run history.'}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {jobRuns.isLoading && (
                <p className="text-sm text-[var(--color-muted-foreground)]">
                  Loading runs…
                </p>
              )}
              {!jobRuns.isLoading && (jobRuns.data?.length ?? 0) === 0 && (
                <p
                  className="text-sm text-[var(--color-muted-foreground)]"
                  data-testid="job-runs-empty"
                >
                  No runs yet. Click the play icon to start one.
                </p>
              )}
              <ul
                className="divide-y divide-[var(--color-border)]"
                data-testid="job-runs-list"
              >
                {(jobRuns.data ?? []).map((r) => (
                  <li
                    key={r.runId}
                    className="flex items-center justify-between gap-3 py-2 text-sm"
                  >
                    <div>
                      <p className="font-mono text-xs text-[var(--color-muted-foreground)]">
                        run {r.runId}
                      </p>
                      <p className="text-[var(--color-muted-foreground)]">
                        {new Date(r.created).toLocaleString()}
                      </p>
                    </div>
                    <RunBadge run={r} />
                  </li>
                ))}
              </ul>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Latest run output</CardTitle>
              <CardDescription>
                Polls every 3 s while the run is active.
              </CardDescription>
            </CardHeader>
            <CardContent>
              {!latestRunId && (
                <p className="text-sm text-[var(--color-muted-foreground)]">
                  Run the job to see output.
                </p>
              )}
              {latestRunId && runOutput.isLoading && (
                <p className="inline-flex items-center gap-2 text-sm text-[var(--color-muted-foreground)]">
                  <Loader2 className="size-3 animate-spin" /> Loading output…
                </p>
              )}
              {latestRunId && !runOutput.isLoading && (
                <pre
                  className="max-h-72 overflow-auto rounded-md border border-[var(--color-border)] bg-[var(--color-background)] p-3 font-mono text-xs leading-relaxed"
                  data-testid="job-run-output"
                >
                  {(runOutput.data ?? []).length === 0
                    ? '(no output yet)'
                    : (runOutput.data ?? [])
                        .map(
                          (o) =>
                            `[${new Date(o.ts).toLocaleTimeString()}] ${o.type} ${o.task}: ${o.message}`,
                        )
                        .join('\n')}
                </pre>
              )}
            </CardContent>
          </Card>
        </div>
      )}

      {canWrite && (
        <JobCreateDialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          tasks={tasks.data ?? []}
        />
      )}
    </div>
  )
}

function RunBadge({ run }: { run: JobRun | null }) {
  if (!run) {
    return (
      <span className="text-[var(--color-muted-foreground)]">never run</span>
    )
  }
  const state = run.state ?? 'running'
  const cls =
    state === 'completed'
      ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-500'
      : state === 'failed'
        ? 'border-red-500/40 bg-red-500/10 text-red-500'
        : 'border-sky-500/40 bg-sky-500/10 text-sky-500'
  return (
    <span
      className={`rounded-full border px-2 py-0.5 text-xs uppercase tracking-wider ${cls}`}
      data-testid={`run-badge-${run.runId}`}
    >
      {state}
    </span>
  )
}
