// Create / Edit a User Group. Membership is edited via a multi-select
// over the existing User list. Collection grants on the group are not
// edited from this dialog (use the per-collection Grants tab instead).

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
import {
  useCreateUserGroup,
  useUpdateUserGroup,
  useUsers,
  type UserGroupPatchInput,
  type UserGroupSummary,
} from '@/lib/api/hooks'

interface UserGroupEditDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  group: UserGroupSummary | null
}

export function UserGroupEditDialog({
  open,
  onOpenChange,
  group,
}: UserGroupEditDialogProps) {
  const create = useCreateUserGroup()
  const update = useUpdateUserGroup()
  const users = useUsers()

  const [name, setName] = React.useState('')
  const [description, setDescription] = React.useState('')
  const [memberIds, setMemberIds] = React.useState<Set<string>>(new Set())
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    if (open) {
      setName(group?.name ?? '')
      setDescription(group?.description ?? '')
      const ids = new Set<string>()
      for (const u of group?.users ?? []) {
        if (u.userId) ids.add(u.userId)
      }
      setMemberIds(ids)
      setError(null)
      create.reset()
      update.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, group])

  const editing = Boolean(group)
  const busy = create.isPending || update.isPending

  function toggleMember(userId: string) {
    setMemberIds((prev) => {
      const next = new Set(prev)
      if (next.has(userId)) {
        next.delete(userId)
      } else {
        next.add(userId)
      }
      return next
    })
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    const trimmed = name.trim()
    if (!trimmed) {
      setError('Name is required.')
      return
    }
    const userIds = Array.from(memberIds)
    try {
      if (editing && group) {
        const patch: UserGroupPatchInput = {}
        if (trimmed !== group.name) patch.name = trimmed
        if ((description || null) !== (group.description ?? null)) {
          patch.description = description || null
        }
        // Always send the membership snapshot — the API replaces it.
        patch.userIds = userIds
        await update.mutateAsync({ userGroupId: group.userGroupId, input: patch })
      } else {
        await create.mutateAsync({
          name: trimmed,
          description: description || null,
          userIds,
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
      <DialogContent
        data-testid="user-group-edit-dialog"
        className="max-h-[80vh] overflow-y-auto"
      >
        <DialogHeader>
          <DialogTitle>{editing ? 'Edit Group' : 'New Group'}</DialogTitle>
          <DialogDescription>
            Bundle users for bulk collection grants. Update grant assignments
            from the per-collection Grants tab.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="ug-name">Name</Label>
            <Input
              id="ug-name"
              autoFocus
              required
              maxLength={255}
              value={name}
              onChange={(e) => setName(e.target.value)}
              data-testid="user-group-name-input"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="ug-description">Description</Label>
            <Textarea
              id="ug-description"
              maxLength={255}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              data-testid="user-group-description-input"
              placeholder="What this group is for (optional)."
            />
          </div>
          <div className="space-y-1.5">
            <Label>Members</Label>
            <div className="max-h-48 overflow-y-auto rounded-md border border-[var(--color-border)] bg-[var(--color-background)]">
              {users.isLoading && (
                <p className="px-3 py-2 text-xs text-[var(--color-muted-foreground)]">
                  Loading users…
                </p>
              )}
              {!users.isLoading && (users.data?.length ?? 0) === 0 && (
                <p className="px-3 py-2 text-xs text-[var(--color-muted-foreground)]">
                  No users available.
                </p>
              )}
              <ul className="divide-y divide-[var(--color-border)]">
                {(users.data ?? []).map((u) => (
                  <li key={u.userId} className="flex items-center gap-2 px-3 py-1.5">
                    <input
                      id={`ug-member-${u.userId}`}
                      type="checkbox"
                      checked={memberIds.has(u.userId)}
                      onChange={() => toggleMember(u.userId)}
                      data-testid={`user-group-member-${u.userId}`}
                    />
                    <Label
                      htmlFor={`ug-member-${u.userId}`}
                      className="flex-1 cursor-pointer text-sm font-normal"
                    >
                      <span className="font-medium">{u.username}</span>
                      {u.displayName && (
                        <span className="ml-2 text-[var(--color-muted-foreground)]">
                          {u.displayName}
                        </span>
                      )}
                    </Label>
                  </li>
                ))}
              </ul>
            </div>
          </div>
          {error && (
            <p className="text-sm text-red-500" data-testid="user-group-edit-error">
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
              disabled={busy}
              data-testid="user-group-edit-submit"
            >
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
