// Create a new Job. Pick one task from the registry and give it a
// name. Schedule (event) configuration is intentionally not exposed
// in this milestone — manual `POST /jobs/{jobId}/runs` only.

import { Loader2 } from 'lucide-react'
import * as React from 'react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { useCreateJob, type JobTask } from '@/lib/api/hooks'

interface JobCreateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tasks: JobTask[]
}

export function JobCreateDialog({
  open,
  onOpenChange,
  tasks,
}: JobCreateDialogProps) {
  const create = useCreateJob()
  const [name, setName] = React.useState('')
  const [description, setDescription] = React.useState('')
  const [taskName, setTaskName] = React.useState('')
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    if (open) {
      setName('')
      setDescription('')
      setTaskName(tasks[0]?.name ?? '')
      setError(null)
      create.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const busy = create.isPending

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    const trimmedName = name.trim()
    if (!trimmedName) {
      setError('Job name is required.')
      return
    }
    if (!taskName) {
      setError('Pick at least one task.')
      return
    }
    try {
      await create.mutateAsync({
        name: trimmedName,
        description: description || null,
        tasks: [taskName],
      })
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Create failed.')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="job-create-dialog">
        <DialogHeader>
          <DialogTitle>New Job</DialogTitle>
          <DialogDescription>
            Pick a task from the registry and give the job a name. You can run
            it ad-hoc from the Jobs table.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="job-name">Name</Label>
            <Input
              id="job-name"
              autoFocus
              required
              maxLength={255}
              value={name}
              onChange={(e) => setName(e.target.value)}
              data-testid="job-name-input"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="job-description">Description</Label>
            <Textarea
              id="job-description"
              maxLength={1024}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              data-testid="job-description-input"
              placeholder="What this job is for (optional)."
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="job-task">Task</Label>
            <select
              id="job-task"
              className="block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-background)] px-2 py-1 text-sm"
              value={taskName}
              onChange={(e) => setTaskName(e.target.value)}
              data-testid="job-task-select"
            >
              {tasks.map((t) => (
                <option key={t.taskId} value={t.name}>
                  {t.name}
                  {t.description ? ` — ${t.description}` : ''}
                </option>
              ))}
            </select>
          </div>
          {error && (
            <p className="text-sm text-red-500" data-testid="job-create-error">
              {error}
            </p>
          )}
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="ghost" disabled={busy}>
                Cancel
              </Button>
            </DialogClose>
            <Button
              type="submit"
              disabled={busy || tasks.length === 0}
              data-testid="job-create-submit"
            >
              {busy ? (
                <>
                  <Loader2 className="size-4 animate-spin" /> Creating…
                </>
              ) : (
                'Create'
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
