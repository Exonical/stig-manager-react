// /admin/user-groups — list and manage user groups.
//
// User groups bundle multiple users together for the purpose of
// granting collection access. A user group can be a grantee on a
// collection just like a user can.

import { Loader2, Pencil, Plus, Trash2 } from 'lucide-react'
import * as React from 'react'

import { UserGroupEditDialog } from './user-group-edit-dialog'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  useDeleteUserGroup,
  useUserGroups,
  type UserGroupSummary,
} from '@/lib/api/hooks'
import { useAuth } from '@/lib/auth/auth-context'
import { hasScope } from '@/lib/auth/scopes'

export function UserGroupsPage() {
  const { status } = useAuth()
  const user = status.kind === 'signed-in' ? status.user : null
  const canWrite = hasScope(user, 'stig-manager:user')

  const groups = useUserGroups()

  const [editing, setEditing] = React.useState<UserGroupSummary | null>(null)
  const [dialogOpen, setDialogOpen] = React.useState(false)

  function openCreate() {
    setEditing(null)
    setDialogOpen(true)
  }
  function openEdit(g: UserGroupSummary) {
    setEditing(g)
    setDialogOpen(true)
  }

  const del = useDeleteUserGroup()
  async function onDelete(g: UserGroupSummary) {
    const ok = window.confirm(
      `Delete group "${g.name}"? This removes the group and all of its grants.`,
    )
    if (!ok) return
    try {
      await del.mutateAsync(g.userGroupId)
    } catch (err) {
      window.alert(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <div className="space-y-6" data-testid="admin-user-groups-page">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div>
              <CardTitle>User Groups</CardTitle>
              <CardDescription>
                Bundle users into groups for bulk collection grants.
              </CardDescription>
            </div>
            {canWrite && (
              <Button
                size="sm"
                onClick={openCreate}
                data-testid="new-user-group-button"
              >
                <Plus className="size-4" /> New Group
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto rounded-md border border-[var(--color-border)]">
            <table className="w-full text-sm" data-testid="user-groups-table">
              <thead className="bg-[var(--color-muted)]/30 text-left text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
                <tr>
                  <th className="px-3 py-2">Name</th>
                  <th className="px-3 py-2">Description</th>
                  <th className="px-3 py-2 text-right">Members</th>
                  <th className="px-3 py-2 text-right">Grants</th>
                  <th className="w-24 px-3 py-2 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {groups.isLoading && (
                  <tr>
                    <td
                      className="px-3 py-6 text-center text-[var(--color-muted-foreground)]"
                      colSpan={5}
                    >
                      Loading user groups…
                    </td>
                  </tr>
                )}
                {groups.isError && (
                  <tr>
                    <td className="px-3 py-6 text-center text-red-500" colSpan={5}>
                      Failed to load user groups: {(groups.error as Error).message}
                    </td>
                  </tr>
                )}
                {!groups.isLoading && (groups.data?.length ?? 0) === 0 && (
                  <tr>
                    <td
                      className="px-3 py-6 text-center text-[var(--color-muted-foreground)]"
                      colSpan={5}
                      data-testid="user-groups-empty"
                    >
                      No user groups yet.
                    </td>
                  </tr>
                )}
                {(groups.data ?? []).map((g) => (
                  <tr
                    key={g.userGroupId}
                    className="border-t border-[var(--color-border)]"
                    data-testid={`user-group-row-${g.userGroupId}`}
                  >
                    <td className="px-3 py-2 font-medium">{g.name}</td>
                    <td className="px-3 py-2 text-[var(--color-muted-foreground)]">
                      {g.description ?? '—'}
                    </td>
                    <td className="px-3 py-2 text-right">
                      {g.users?.length ?? 0}
                    </td>
                    <td className="px-3 py-2 text-right">
                      {g.collectionGrants?.length ?? 0}
                    </td>
                    <td className="px-3 py-2 text-right">
                      <div className="flex justify-end gap-1">
                        {canWrite && (
                          <>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => openEdit(g)}
                              data-testid={`user-group-edit-${g.userGroupId}`}
                              aria-label={`Edit ${g.name}`}
                            >
                              <Pencil className="size-3.5" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => onDelete(g)}
                              data-testid={`user-group-delete-${g.userGroupId}`}
                              aria-label={`Delete ${g.name}`}
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
          {groups.isFetching && !groups.isLoading && (
            <p className="mt-2 inline-flex items-center gap-2 text-xs text-[var(--color-muted-foreground)]">
              <Loader2 className="size-3 animate-spin" /> refreshing…
            </p>
          )}
        </CardContent>
      </Card>

      {canWrite && (
        <UserGroupEditDialog
          open={dialogOpen}
          onOpenChange={setDialogOpen}
          group={editing}
        />
      )}
    </div>
  )
}
