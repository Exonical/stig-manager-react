import { ShieldCheck } from 'lucide-react'
import { NavLink } from 'react-router-dom'

import { ADMIN_NAV, PRIMARY_NAV, type NavItem } from './nav-items'
import { useAuth } from '@/lib/auth/auth-context'
import { hasScope } from '@/lib/auth/scopes'
import { cn } from '@/lib/utils'

export function Sidebar() {
  const { status } = useAuth()
  const user = status.kind === 'signed-in' ? status.user : null
  const isVisible = (item: NavItem) => !item.scope || hasScope(user, item.scope)
  const primary = PRIMARY_NAV.filter(isVisible)
  const admin = ADMIN_NAV.filter(isVisible)

  return (
    <aside
      className="hidden w-60 shrink-0 border-r border-[var(--color-border)] bg-[var(--color-card)]/40 md:flex md:flex-col"
      data-testid="app-sidebar"
    >
      <div className="flex items-center gap-2 px-5 py-4">
        <ShieldCheck className="size-6 text-sky-500" />
        <span className="text-base font-semibold tracking-tight">
          STIG Manager
        </span>
      </div>
      <nav className="flex flex-1 flex-col gap-1 px-3 pb-6 pt-2">
        <NavGroup label="Workspace" items={primary} />
        {admin.length > 0 && <NavGroup label="Administration" items={admin} />}
      </nav>
    </aside>
  )
}

function NavGroup({
  label,
  items,
}: {
  label: string
  items: readonly NavItem[]
}) {
  if (items.length === 0) return null
  return (
    <div className="mt-3 first:mt-0">
      <p className="px-3 pb-1 text-xs font-medium uppercase tracking-wider text-[var(--color-muted-foreground)]">
        {label}
      </p>
      <ul className="space-y-0.5">
        {items.map((item) => (
          <li key={item.to}>
            <NavLink
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
                  isActive
                    ? 'bg-[var(--color-accent)] text-[var(--color-accent-foreground)]'
                    : 'text-[var(--color-muted-foreground)] hover:bg-[var(--color-accent)]/40 hover:text-[var(--color-accent-foreground)]',
                )
              }
              data-testid={`nav-${item.label.toLowerCase().replace(/\s+/g, '-')}`}
            >
              <item.icon className="size-4" />
              <span>{item.label}</span>
            </NavLink>
          </li>
        ))}
      </ul>
    </div>
  )
}
