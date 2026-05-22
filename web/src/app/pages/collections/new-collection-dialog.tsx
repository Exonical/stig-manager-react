// New Collection dialog. Posts to /collections with a single grant for
// the current user as Owner. The signed-in user's `userId` comes from
// `GET /user`, which auto-upserts the app_user row on first hit.

import { Loader2 } from 'lucide-react'
import * as React from 'react'
import { useNavigate } from 'react-router-dom'

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
import { useCreateCollection, useCurrentUser } from '@/lib/api/hooks'

interface NewCollectionDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function NewCollectionDialog({
  open,
  onOpenChange,
}: NewCollectionDialogProps) {
  const navigate = useNavigate()
  const create = useCreateCollection()
  const me = useCurrentUser()

  const [name, setName] = React.useState('')
  const [description, setDescription] = React.useState('')
  const [error, setError] = React.useState<string | null>(null)

  // Reset on close so reopening doesn't show stale text. `create` is a
  // TanStack-Query useMutation result whose object reference is unstable
  // across renders; including it in the deps causes the effect to run
  // every render and starves React's navigation transitions.
  React.useEffect(() => {
    if (!open) {
      setName('')
      setDescription('')
      setError(null)
      create.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!me.data?.userId) {
      setError("Couldn't determine your user record. Try reloading the page.")
      return
    }
    if (!name.trim()) {
      setError('Name is required.')
      return
    }
    try {
      const created = await create.mutateAsync({
        name: name.trim(),
        description: description.trim() || undefined,
        grants: [{ userId: me.data.userId, roleId: 4 }],
      })
      navigate(`/collections/${created.collectionId}`)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create collection.')
    }
  }

  const busy = create.isPending || me.isLoading

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="new-collection-dialog">
        <DialogHeader>
          <DialogTitle>New Collection</DialogTitle>
          <DialogDescription>
            Creates a Collection with you as the Owner. You can add other users,
            grants, and STIG assignments after the Collection is created.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="new-collection-name">Name</Label>
            <Input
              id="new-collection-name"
              autoFocus
              required
              maxLength={255}
              value={name}
              onChange={(e) => setName(e.target.value)}
              data-testid="new-collection-name-input"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="new-collection-description">Description</Label>
            <Textarea
              id="new-collection-description"
              maxLength={255}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              data-testid="new-collection-description-input"
              placeholder="A short summary visible on the collection list."
            />
          </div>
          {error && (
            <p className="text-sm text-red-500" data-testid="new-collection-error">
              {error}
            </p>
          )}
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="ghost" disabled={busy}>
                Cancel
              </Button>
            </DialogClose>
            <Button type="submit" disabled={busy} data-testid="new-collection-submit">
              {busy ? (
                <>
                  <Loader2 className="size-4 animate-spin" /> Creating…
                </>
              ) : (
                'Create Collection'
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
