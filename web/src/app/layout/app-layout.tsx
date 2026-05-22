import { Outlet } from 'react-router-dom'

import { Sidebar } from './sidebar'
import { Topbar } from './topbar'

export function AppLayout() {
  return (
    <div className="flex min-h-screen bg-[var(--color-background)]">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar />
        <main className="flex-1 overflow-x-hidden">
          <div className="mx-auto max-w-7xl px-6 py-8">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}
