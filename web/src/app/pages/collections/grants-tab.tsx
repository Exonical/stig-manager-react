// Grants tab on Collection detail. Manage-role and above sees a
// list of every user / user-group grant on the collection plus an
// "Add grant" form. Owners (role 4) can also delete grants.

import { Loader2, Plus, Trash2 } from 'lucide-react'
import * as React from 'react'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  useCollectionGrants,
  useDeleteCollectionGrant,
  usePostCollectionGrants,
  useUserGroups,
  useUsers,
  type CollectionGrant,
  type GrantPostInput,
} from '@/lib/api/hooks'
import { atLeast, ROLE_LABELS, type CollectionRoleId } from '@/lib/auth/roles'

interface GrantsTabProps {
  collectionId: string
  role: CollectionRoleId | null
}

export function GrantsTab({ collectionId, role }: GrantsTabProps) {
  const canEdit = atLeast(role, 3) // Manage or Owner
  const grants = useCollectionGrants(collectionId)
  const users = useUsers()
  const groups = useUserGroups()

  const post = usePostCollectionGrants()
  const del = useDeleteCollectionGrant()

  const [subjectKind, setSubjectKind] = React.useState<'user' | 'group'>('user')
  const [subjectId, setSubjectId] = React.useState<string>('')
  const [roleId, setRoleId] = React.useState<CollectionRoleId>(1)
  const [error, setError] = React.useState<string | null>(null)

  // Default the subject pickers as data lands.
  React.useEffect(() => {
    if (subjectKind === 'user' && !subjectId && users.data?.length) {
      setSubjectId(users.data[0]!.userId)
    } else if (subjectKind === 'group' && !subjectId && groups.data?.length) {
      setSubjectId(groups.data[0]!.userGroupId)
    }
  }, [subjectKind, subjectId, users.data, groups.data])

  async function onAdd(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!subjectId) {
      setError('Pick a user or group.')
      return
    }
    const body: GrantPostInput =
      subjectKind === 'user'
        ? { userId: subjectId, roleId }
        : { userGroupId: subjectId, roleId }
    try {
      await post.mutateAsync({ collectionId, body: [body] })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add grant.')
    }
  }

  async function onDelete(g: CollectionGrant) {
    if (!g.grantId) return
    const subject =
      g.user?.username ?? g.userGroup?.name ?? g.user?.userId ?? g.grantId
    const ok = window.confirm(`Remove grant for "${subject}"?`)
    if (!ok) return
    try {
      await del.mutateAsync({ collectionId, grantId: g.grantId })
    } catch (err) {
      window.alert(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <div className="space-y-4" data-testid="collection-grants-tab">
      <Card>
        <CardHeader>
          <CardTitle>Grants</CardTitle>
          <CardDescription>
            Users and groups with access to this Collection. Role 4 = Owner.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {canEdit && (
            <form
              onSubmit={onAdd}
              className="flex flex-wrap items-end gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-card)]/40 p-3"
              data-testid="add-grant-form"
            >
              <div className="flex flex-col gap-1">
                <label className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
                  Subject
                </label>
                <select
                  className="rounded-md border border-[var(--color-border)] bg-[var(--color-background)] px-2 py-1 text-sm"
                  value={subjectKind}
                  onChange={(e) => {
                    setSubjectKind(e.target.value as 'user' | 'group')
                    setSubjectId('')
                  }}
                  data-testid="grant-subject-kind"
                >
                  <option value="user">User</option>
                  <option value="group">User Group</option>
                </select>
              </div>
              <div className="flex flex-col gap-1">
                <label className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
                  {subjectKind === 'user' ? 'User' : 'Group'}
                </label>
                <select
                  className="min-w-[12rem] rounded-md border border-[var(--color-border)] bg-[var(--color-background)] px-2 py-1 text-sm"
                  value={subjectId}
                  onChange={(e) => setSubjectId(e.target.value)}
                  data-testid="grant-subject-id"
                >
                  {subjectKind === 'user'
                    ? (users.data ?? []).map((u) => (
                        <option key={u.userId} value={u.userId}>
                          {u.username}
                        </option>
                      ))
                    : (groups.data ?? []).map((g) => (
                        <option key={g.userGroupId} value={g.userGroupId}>
                          {g.name}
                        </option>
                      ))}
                </select>
              </div>
              <div className="flex flex-col gap-1">
                <label className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
                  Role
                </label>
                <select
                  className="rounded-md border border-[var(--color-border)] bg-[var(--color-background)] px-2 py-1 text-sm"
                  value={roleId}
                  onChange={(e) =>
                    setRoleId(Number(e.target.value) as CollectionRoleId)
                  }
                  data-testid="grant-role"
                >
                  <option value={1}>1 — Restricted</option>
                  <option value={2}>2 — Full</option>
                  <option value={3}>3 — Manage</option>
                  <option value={4}>4 — Owner</option>
                </select>
              </div>
              <Button
                type="submit"
                size="sm"
                disabled={post.isPending}
                data-testid="grant-add-button"
              >
                {post.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Plus className="size-4" />
                )}
                Add grant
              </Button>
              {error && (
                <p className="basis-full text-xs text-red-500" data-testid="grant-add-error">
                  {error}
                </p>
              )}
            </form>
          )}

          <div className="overflow-x-auto rounded-md border border-[var(--color-border)]">
            <table className="w-full text-sm" data-testid="grants-table">
              <thead className="bg-[var(--color-muted)]/30 text-left text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]">
                <tr>
                  <th className="px-3 py-2">Kind</th>
                  <th className="px-3 py-2">Subject</th>
                  <th className="px-3 py-2">Role</th>
                  <th className="w-16 px-3 py-2 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {grants.isLoading && (
                  <tr>
                    <td
                      colSpan={4}
                      className="px-3 py-6 text-center text-[var(--color-muted-foreground)]"
                    >
                      Loading grants…
                    </td>
                  </tr>
                )}
                {grants.isError && (
                  <tr>
                    <td colSpan={4} className="px-3 py-6 text-center text-red-500">
                      Failed to load grants: {(grants.error as Error).message}
                    </td>
                  </tr>
                )}
                {!grants.isLoading && (grants.data?.length ?? 0) === 0 && (
                  <tr>
                    <td
                      colSpan={4}
                      className="px-3 py-6 text-center text-[var(--color-muted-foreground)]"
                      data-testid="grants-empty"
                    >
                      No grants on this collection yet.
                    </td>
                  </tr>
                )}
                {(grants.data ?? []).map((g) => {
                  const kind = g.user ? 'user' : g.userGroup ? 'group' : '—'
                  const subject =
                    g.user?.username ??
                    g.userGroup?.name ??
                    g.user?.userId ??
                    g.userGroup?.userGroupId ??
                    '—'
                  return (
                    <tr
                      key={g.grantId ?? subject}
                      className="border-t border-[var(--color-border)]"
                      data-testid={`grant-row-${g.grantId ?? subject}`}
                    >
                      <td className="px-3 py-2 capitalize">{kind}</td>
                      <td className="px-3 py-2">{subject}</td>
                      <td className="px-3 py-2">
                        {ROLE_LABELS[g.roleId as 1 | 2 | 3 | 4] ?? g.roleId}
                      </td>
                      <td className="px-3 py-2 text-right">
                        {canEdit && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => onDelete(g)}
                            disabled={del.isPending}
                            data-testid={`grant-delete-${g.grantId ?? subject}`}
                            aria-label={`Delete grant for ${subject}`}
                          >
                            <Trash2 className="size-3.5 text-red-500" />
                          </Button>
                        )}
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
  )
}
