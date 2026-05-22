// /admin/* shell. Renders a sub-nav for the admin pages above the
// per-page content. Visibility of each link respects the same scope
// rules as the sidebar so we don't show links the user can't follow.

import { Briefcase, ClipboardList, Users, UsersRound } from 'lucide-react'
import { NavLink, Outlet } from 'react-router-dom'

import { useAuth } from '@/lib/auth/auth-context'
import { hasScope, type ScopeName } from '@/lib/auth/scopes'
import { cn } from '@/lib/utils'

interface AdminNavItem {
  to: string
  label: string
  icon: typeof Users
  scope: ScopeName
}

const ADMIN_TABS: readonly AdminNavItem[] = [
  {
    to: '/admin/users',
    label: 'Users',
    icon: Users,
    scope: 'stig-manager:user:read',
  },
  {
    to: '/admin/user-groups',
    label: 'User Groups',
    icon: UsersRound,
    scope: 'stig-manager:user:read',
  },
  {
    to: '/admin/jobs',
    label: 'Jobs',
    icon: Briefcase,
    scope: 'stig-manager:op:read',
  },
  {
    to: '/admin/app-info',
    label: 'App info',
    icon: ClipboardList,
    scope: 'stig-manager:op:read',
  },
]

export function AdminLayout() {
  const { status } = useAuth()
  const user = status.kind === 'signed-in' ? status.user : null
  const visible = ADMIN_TABS.filter((t) => hasScope(user, t.scope))

  return (
    <div className="space-y-6" data-testid="admin-layout">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight">Administration</h1>
        <p className="mt-1 text-sm text-[var(--color-muted-foreground)]">
          Manage users, groups, background jobs, and runtime state.
        </p>
      </header>
      <nav
        className="flex flex-wrap gap-1 rounded-md border border-[var(--color-border)] bg-[var(--color-card)]/40 p-1"
        data-testid="admin-tabs"
      >
        {visible.map((tab) => (
          <NavLink
            key={tab.to}
            to={tab.to}
            className={({ isActive }) =>
              cn(
                'inline-flex items-center gap-2 rounded-md px-3 py-1.5 text-sm transition-colors',
                isActive
                  ? 'bg-[var(--color-accent)] text-[var(--color-accent-foreground)]'
                  : 'text-[var(--color-muted-foreground)] hover:bg-[var(--color-accent)]/40 hover:text-[var(--color-accent-foreground)]',
              )
            }
            data-testid={`admin-tab-${tab.label.toLowerCase().replace(/\s+/g, '-')}`}
          >
            <tab.icon className="size-4" />
            <span>{tab.label}</span>
          </NavLink>
        ))}
      </nav>
      <Outlet />
    </div>
  )
}
