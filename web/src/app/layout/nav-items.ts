// Single source of truth for primary navigation entries. Each entry
// declares the path, the icon, and the OIDC scope (if any) needed to
// see the link at all. Pages further enforce scopes on the route
// itself.

import {
  BookOpen,
  Briefcase,
  ClipboardList,
  Layers,
  ScrollText,
  ShieldCheck,
  Users,
  UsersRound,
} from 'lucide-react'

import type { ScopeName } from '@/lib/auth/scopes'

export interface NavItem {
  to: string
  label: string
  icon: typeof ShieldCheck
  scope?: ScopeName
  end?: boolean
}

export const PRIMARY_NAV: readonly NavItem[] = [
  { to: '/', label: 'Dashboard', icon: ShieldCheck, end: true },
  {
    to: '/collections',
    label: 'Collections',
    icon: Layers,
    scope: 'stig-manager:collection:read',
  },
  {
    to: '/library',
    label: 'STIG Library',
    icon: BookOpen,
    scope: 'stig-manager:stig:read',
  },
] as const

export const ADMIN_NAV: readonly NavItem[] = [
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
  {
    to: '/admin/audit-log',
    label: 'Audit log',
    icon: ScrollText,
    scope: 'stig-manager:op:read',
  },
] as const
