// /admin/users — list and manage users.
//
// Lists every User visible to the requester with status, last-access,
// and grant counts. The `stig-manager:user` (write) scope unlocks the
// create / edit / delete affordances; without it the page is read-only.

import { Loader2, Pencil, Plus, Search, Trash2 } from 'lucide-react'
import * as React from 'react'

import { UserEditDialog } from './user-edit-dialog'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  useDeleteUser,
  useUsers,
  type UserSummary,
  type UsersFilter,
} from '@/lib/api/hooks'
import { useAuth } from '@/lib/auth/auth-context'
import { hasScope } from '@/lib/auth/scopes'

export function UsersPage() {
  const { status } = useAuth()
  const user = status.kind === 'signed-in' ? status.user : null
  const canWrite = hasScope(user, 'stig-manager:user')

  const [query, setQuery] = React.useState('')
  const [debounced, setDebounced] = React.useState('')
  React.useEffect(() => {
    const id = window.setTimeout(() => setDebounced(query), 200)
    return () => window.clearTimeout(id)
  }, [query])

  const filter: UsersFilter | undefined = debounced
    ? { username: debounced, usernameMatch: 'contains' }
    : undefined
  const users = useUsers(filter)

  const [editing, setEditing] = React.useState<UserSummary | null>(null)
  const [dialogOpen, setDialogOpen] = React.useState(false)

  function openCreate() {
    setEditing(null)
    setDialogOpen(true)
  }
  function openEdit(u: UserSummary) {
    setEditing(u)
    setDialogOpen(true)
  }

  const del = useDeleteUser()

  async function onDelete(u: UserSummary) {
    const ok = window.confirm(
      `Delete user "${u.username}"? Only users who have never accessed the system can be deleted.`,
    )
    if (!ok) return
    try {
      await del.mutateAsync(u.userId)
    } catch (err) {
      window.alert(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <div className="space-y-6" data-testid="admin-users-page">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div>
              <CardTitle>Users</CardTitle>
              <CardDescription>
                Manage user records, grants, and group memberships.
              </CardDescription>
            </div>
            {canWrite && (
              <Button
                size="sm"
                onClick={openCreate}
                data-testid="new-user-button"
              >
                <Plus className="size-4" /> New User
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <div className="relative w-full max-w-md">
              <Search className="pointer-events-none absolute left-2 top-1/2 size-4 -translate-y-1/2 text-[var(--color-muted-foreground)]" />
              <Input
                placeholder="Search by username (contains)…"
                className="pl-8"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                data-testid="users-search"
              />
            </div>
            {users.isFetching && !users.isLoading && (
              <Loader2 className="size-4 animate-spin text-[var(--color-muted-foreground)]" />
            )}
          </div>

          <div className="overflow-x-auto rounded-md border border-[var(--color-border)]">
            <table className="w-full text-sm" data-testid="users-table">
              <thead className="bg-[var(--color-muted)]/30 text-left text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
                <tr>
                  <th className="px-3 py-2">Username</th>
                  <th className="px-3 py-2">Display name</th>
                  <th className="px-3 py-2">Status</th>
                  <th className="px-3 py-2">Last access</th>
                  <th className="px-3 py-2 text-right">Grants</th>
                  <th className="px-3 py-2 text-right">Groups</th>
                  <th className="w-24 px-3 py-2 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {users.isLoading && (
                  <tr>
                    <td
                      className="px-3 py-6 text-center text-[var(--color-muted-foreground)]"
                      colSpan={7}
                    >
                      Loading users…
                    </td>
                  </tr>
                )}
                {users.isError && (
                  <tr>
                    <td
                      className="px-3 py-6 text-center text-red-500"
                      colSpan={7}
                    >
                      Failed to load users: {(users.error as Error).message}
                    </td>
                  </tr>
                )}
                {!users.isLoading && (users.data?.length ?? 0) === 0 && (
                  <tr>
                    <td
                      className="px-3 py-6 text-center text-[var(--color-muted-foreground)]"
                      colSpan={7}
                      data-testid="users-empty"
                    >
                      {debounced ? `No users match "${debounced}".` : 'No users yet.'}
                    </td>
                  </tr>
                )}
                {(users.data ?? []).map((u) => (
                  <tr
                    key={u.userId}
                    className="border-t border-[var(--color-border)]"
                    data-testid={`user-row-${u.userId}`}
                  >
                    <td className="px-3 py-2 font-medium" data-testid={`user-username-${u.userId}`}>
                      {u.username}
                    </td>
                    <td className="px-3 py-2 text-[var(--color-muted-foreground)]">
                      {u.displayName ?? '—'}
                    </td>
                    <td className="px-3 py-2">
                      <span
                        className={
                          u.status === 'available'
                            ? 'rounded-full border border-emerald-500/40 bg-emerald-500/10 px-2 py-0.5 text-xs uppercase tracking-wider text-emerald-500'
                            : 'rounded-full border border-[var(--color-border)] px-2 py-0.5 text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]'
                        }
                      >
                        {u.status ?? 'unknown'}
                      </span>
                    </td>
                    <td className="px-3 py-2 font-mono text-xs text-[var(--color-muted-foreground)]">
                      {formatLastAccess(u.lastAccess)}
                    </td>
                    <td className="px-3 py-2 text-right">
                      {u.collectionGrants?.length ?? 0}
                    </td>
                    <td className="px-3 py-2 text-right">
                      {u.userGroups?.length ?? 0}
                    </td>
                    <td className="px-3 py-2 text-right">
                      <div className="flex justify-end gap-1">
                        {canWrite && (
                          <>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => openEdit(u)}
                              data-testid={`user-edit-${u.userId}`}
                              aria-label={`Edit ${u.username}`}
                            >
                              <Pencil className="size-3.5" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => onDelete(u)}
                              data-testid={`user-delete-${u.userId}`}
                              aria-label={`Delete ${u.username}`}
                              disabled={del.isPending}
                            >
                              <Trash2 className="size-3.5 text-red-500" />
                            </Button>
                          </>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {canWrite && (
        <UserEditDialog
          open={dialogOpen}
          onOpenChange={setDialogOpen}
          user={editing}
        />
      )}
    </div>
  )
}

function formatLastAccess(unix: number | null | undefined): string {
  if (!unix) return 'never'
  return new Date(unix * 1000).toLocaleString()
}
