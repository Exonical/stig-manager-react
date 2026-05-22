// Helpers for working with Collection-level role IDs. The numbers
// match upstream's CollectionRoleId enum: 1=Restricted, 2=Full,
// 3=Manage, 4=Owner.

import type { CurrentUser } from '@/lib/api'

export type CollectionRoleId = 1 | 2 | 3 | 4

export const ROLE_LABELS: Record<CollectionRoleId, string> = {
  1: 'Restricted',
  2: 'Full',
  3: 'Manage',
  4: 'Owner',
}

/**
 * Resolves the signed-in user's role in the given Collection, or
 * `null` if the user has no grant. Owners (and admins) implicitly hold
 * every lower role.
 */
export function roleForCollection(
  user: CurrentUser | undefined,
  collectionId: string,
): CollectionRoleId | null {
  if (!user) return null
  if (user.privileges?.admin) return 4
  for (const g of user.collectionGrants ?? []) {
    if (g.collection?.collectionId === collectionId && g.roleId) {
      return g.roleId as CollectionRoleId
    }
  }
  return null
}

/**
 * Returns true when the role is at least `min`. Used by detail-tab
 * gating (e.g. `Grants` tab requires Manage or Owner).
 */
export function atLeast(
  role: CollectionRoleId | null,
  min: CollectionRoleId,
): boolean {
  return role !== null && role >= min
}
