// Create / Edit a User. POSTs to /users for a fresh record; PATCHes
// /users/{userId} otherwise. Username is required on create; on edit
// it can be changed (the API supports rename via PATCH). Status is
// editable on edit only.

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
import {
  useCreateUser,
  useUpdateUser,
  type UserPatchInput,
  type UserSummary,
} from '@/lib/api/hooks'

interface UserEditDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: UserSummary | null
}

export function UserEditDialog({ open, onOpenChange, user }: UserEditDialogProps) {
  const create = useCreateUser()
  const update = useUpdateUser()

  const [username, setUsername] = React.useState('')
  const [status, setStatus] = React.useState<'available' | 'unavailable'>(
    'available',
  )
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    if (open) {
      setUsername(user?.username ?? '')
      setStatus((user?.status ?? 'available') as 'available' | 'unavailable')
      setError(null)
      create.reset()
      update.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, user])

  const editing = Boolean(user)
  const busy = create.isPending || update.isPending

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    const trimmed = username.trim()
    if (!trimmed) {
      setError('Username is required.')
      return
    }
    try {
      if (editing && user) {
        const patch: UserPatchInput = {}
        if (trimmed !== user.username) patch.username = trimmed
        if (status !== user.status) patch.status = status
        if (Object.keys(patch).length === 0) {
          onOpenChange(false)
          return
        }
        await update.mutateAsync({ userId: user.userId, input: patch })
      } else {
        await create.mutateAsync({
          username: trimmed,
          collectionGrants: [],
        })
      }
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="user-edit-dialog">
        <DialogHeader>
          <DialogTitle>{editing ? 'Edit User' : 'New User'}</DialogTitle>
          <DialogDescription>
            {editing
              ? "Update the user's username or status. Status changes take effect immediately."
              : 'Pre-creates a user record. The user signs in via OIDC; on first login the existing record is matched by username.'}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="user-username">Username</Label>
            <Input
              id="user-username"
              autoFocus
              required
              maxLength={255}
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              data-testid="user-username-input"
            />
          </div>
          {editing && (
            <div className="space-y-1.5">
              <Label htmlFor="user-status">Status</Label>
              <select
                id="user-status"
                className="block w-full rounded-md border border-[var(--color-border)] bg-[var(--color-background)] px-2 py-1 text-sm"
                value={status}
                onChange={(e) =>
                  setStatus(e.target.value as 'available' | 'unavailable')
                }
                data-testid="user-status-select"
              >
                <option value="available">available</option>
                <option value="unavailable">unavailable</option>
              </select>
            </div>
          )}
          {error && (
            <p className="text-sm text-red-500" data-testid="user-edit-error">
              {error}
            </p>
          )}
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="ghost" disabled={busy}>
                Cancel
              </Button>
            </DialogClose>
            <Button type="submit" disabled={busy} data-testid="user-edit-submit">
              {busy ? (
                <>
                  <Loader2 className="size-4 animate-spin" /> Saving…
                </>
              ) : editing ? (
                'Save'
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
