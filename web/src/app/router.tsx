import { createBrowserRouter, Navigate } from 'react-router-dom'

import { RequireAuth } from './auth/require-auth'
import { RequireScope } from './auth/require-scope'
import { AppLayout } from './layout/app-layout'
import { CollectionDetailPage } from './pages/collections/detail'
import { CollectionsListPage } from './pages/collections/list'
import { DashboardPage } from './pages/dashboard'
import { NotFoundPage } from './pages/not-found'
import { Placeholder } from './pages/placeholder'
import { SignInPage } from './pages/sign-in'

export const router = createBrowserRouter([
  {
    path: '/sign-in',
    element: <SignInPage />,
  },
  {
    path: '/',
    element: (
      <RequireAuth>
        <AppLayout />
      </RequireAuth>
    ),
    children: [
      { index: true, element: <DashboardPage /> },
      {
        path: 'collections',
        element: (
          <RequireScope scope="stig-manager:collection:read">
            <CollectionsListPage />
          </RequireScope>
        ),
      },
      {
        path: 'collections/:collectionId',
        element: (
          <RequireScope scope="stig-manager:collection:read">
            <CollectionDetailPage />
          </RequireScope>
        ),
      },
      {
        path: 'library',
        element: (
          <RequireScope scope="stig-manager:stig:read">
            <Placeholder
              title="STIG Library"
              blurb="Browse benchmarks, rules, and CCIs. Import XCCDF bundles."
              milestone="18g"
            />
          </RequireScope>
        ),
      },
      {
        path: 'jobs',
        element: (
          <RequireScope scope="stig-manager:op:read">
            <Placeholder
              title="Jobs"
              blurb="Background tasks, schedules, and run history."
              milestone="18f"
            />
          </RequireScope>
        ),
      },
      {
        path: 'admin',
        children: [
          { index: true, element: <Navigate to="users" replace /> },
          {
            path: 'users',
            element: (
              <RequireScope
                scope={['stig-manager:user:read', 'stig-manager:user']}
              >
                <Placeholder
                  title="Users"
                  blurb="Manage users, user groups, and collection grants."
                  milestone="18f"
                />
              </RequireScope>
            ),
          },
          {
            path: 'app-info',
            element: (
              <RequireScope scope="stig-manager:op:read">
                <Placeholder
                  title="App info"
                  blurb="Build metadata, request counters, and runtime state."
                  milestone="18f"
                />
              </RequireScope>
            ),
          },
        ],
      },
      { path: '*', element: <NotFoundPage /> },
    ],
  },
])
