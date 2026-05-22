import { createBrowserRouter, Navigate } from 'react-router-dom'

import { RequireAuth } from './auth/require-auth'
import { RequireScope } from './auth/require-scope'
import { AppLayout } from './layout/app-layout'
import { AdminLayout } from './pages/admin/admin-layout'
import { AppInfoPage } from './pages/admin/app-info-page'
import { JobsPage } from './pages/admin/jobs-page'
import { UserGroupsPage } from './pages/admin/user-groups-page'
import { UsersPage } from './pages/admin/users-page'
import { AssetDetailPage } from './pages/assets/detail'
import { ReviewEditorPage } from './pages/assets/review-editor'
import { CollectionDetailPage } from './pages/collections/detail'
import { CollectionsListPage } from './pages/collections/list'
import { DashboardPage } from './pages/dashboard'
import { LibraryDetailPage } from './pages/library/detail'
import { LibraryListPage } from './pages/library/list'
import { NotFoundPage } from './pages/not-found'
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
        path: 'collections/:collectionId/assets/:assetId',
        element: (
          <RequireScope scope="stig-manager:collection:read">
            <AssetDetailPage />
          </RequireScope>
        ),
      },
      {
        path: 'collections/:collectionId/assets/:assetId/rules/:ruleId',
        element: (
          <RequireScope scope="stig-manager:collection:read">
            <ReviewEditorPage />
          </RequireScope>
        ),
      },
      {
        path: 'library',
        element: (
          <RequireScope scope="stig-manager:stig:read">
            <LibraryListPage />
          </RequireScope>
        ),
      },
      {
        path: 'library/:benchmarkId',
        element: (
          <RequireScope scope="stig-manager:stig:read">
            <LibraryDetailPage />
          </RequireScope>
        ),
      },
      {
        path: 'admin',
        element: <AdminLayout />,
        children: [
          { index: true, element: <Navigate to="users" replace /> },
          {
            path: 'users',
            element: (
              <RequireScope
                scope={['stig-manager:user:read', 'stig-manager:user']}
              >
                <UsersPage />
              </RequireScope>
            ),
          },
          {
            path: 'user-groups',
            element: (
              <RequireScope
                scope={['stig-manager:user:read', 'stig-manager:user']}
              >
                <UserGroupsPage />
              </RequireScope>
            ),
          },
          {
            path: 'jobs',
            element: (
              <RequireScope scope="stig-manager:op:read">
                <JobsPage />
              </RequireScope>
            ),
          },
          {
            path: 'app-info',
            element: (
              <RequireScope scope="stig-manager:op:read">
                <AppInfoPage />
              </RequireScope>
            ),
          },
        ],
      },
      { path: '*', element: <NotFoundPage /> },
    ],
  },
])
