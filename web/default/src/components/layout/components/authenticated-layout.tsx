/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useRef } from 'react'
import { useAuthStore } from '@/stores/auth-store'
import { getSelf } from '@/lib/api'
import { getCookie } from '@/lib/cookies'
import { cn } from '@/lib/utils'
import { LayoutProvider } from '@/context/layout-provider'
import { SearchProvider } from '@/context/search-provider'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { AnimatedOutlet } from '@/components/page-transition'
import { SkipToMain } from '@/components/skip-to-main'
import { AppHeader } from './app-header'
import { AppSidebar } from './app-sidebar'

type AuthenticatedLayoutProps = {
  children?: React.ReactNode
}

const CURRENT_USER_REFRESH_INTERVAL_MS = 30_000

function useCurrentUserRefresh() {
  const lastRefreshAt = useRef(0)

  useEffect(() => {
    let disposed = false

    const refresh = async (force = false) => {
      const { auth } = useAuthStore.getState()
      if (!auth.user) return

      const now = Date.now()
      if (
        !force &&
        now - lastRefreshAt.current < CURRENT_USER_REFRESH_INTERVAL_MS
      ) {
        return
      }
      lastRefreshAt.current = now

      const res = await getSelf().catch(() => null)
      if (disposed || !res?.success || !res.data) return
      useAuthStore.getState().auth.setUser(res.data)
    }

    const handleFocus = () => {
      void refresh(true)
    }
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        void refresh(true)
      }
    }

    const intervalId = window.setInterval(() => {
      if (document.visibilityState === 'visible') {
        void refresh(false)
      }
    }, CURRENT_USER_REFRESH_INTERVAL_MS)

    window.addEventListener('focus', handleFocus)
    document.addEventListener('visibilitychange', handleVisibilityChange)

    return () => {
      disposed = true
      window.clearInterval(intervalId)
      window.removeEventListener('focus', handleFocus)
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [])
}

export function AuthenticatedLayout(props: AuthenticatedLayoutProps) {
  useCurrentUserRefresh()
  const defaultOpen = getCookie('sidebar_state') !== 'false'

  return (
    <LayoutProvider>
      <SearchProvider>
        <SidebarProvider defaultOpen={defaultOpen} className='flex-col'>
          <SkipToMain />
          <AppHeader />
          <div className='flex min-h-0 w-full flex-1'>
            <AppSidebar />
            <SidebarInset
              className={cn(
                '@container/content',
                'h-[calc(100svh-var(--app-header-height,0px))]',
                'min-h-0 overflow-hidden',
                'peer-data-[variant=inset]:h-[calc(100svh-var(--app-header-height,0px)-(var(--spacing)*4))]'
              )}
            >
              {props.children ?? <AnimatedOutlet />}
            </SidebarInset>
          </div>
        </SidebarProvider>
      </SearchProvider>
    </LayoutProvider>
  )
}
