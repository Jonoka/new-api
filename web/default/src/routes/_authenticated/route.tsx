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
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { getSelf } from '@/lib/api'
import { AuthenticatedLayout } from '@/components/layout'
import { rememberCanvasReturn } from '@/features/canvas/sso'

// 内存中的验证标记，避免同一会话中重复验证
let sessionVerified = false
let sessionVerifiedAt = 0

const AUTH_SESSION_REFRESH_TTL_MS = 30_000
const ADMIN_ROUTE_PREFIXES = [
  '/channels',
  '/channel-observability',
  '/models',
  '/system-settings',
  '/users',
  '/redemption-codes',
  '/subscriptions',
  '/game-management',
  '/invoice-management',
  '/notification-center',
  '/extensions',
]

function isAdminRoute(pathname: string): boolean {
  return ADMIN_ROUTE_PREFIXES.some(
    (prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`)
  )
}

export const Route = createFileRoute('/_authenticated')({
  beforeLoad: async ({ location }) => {
	rememberCanvasReturn(location.href)
    const { auth } = useAuthStore.getState()
    const pathname = location.pathname || ''

    // 如果本地没有用户信息，直接跳转登录页
    if (!auth.user) {
      throw redirect({
        to: '/sign-in',
        search: { redirect: location.href },
      })
    }

    const shouldRefreshSession =
      !sessionVerified ||
      Date.now() - sessionVerifiedAt > AUTH_SESSION_REFRESH_TTL_MS ||
      isAdminRoute(pathname)

    // 本地有用户信息，也需要定期刷新，保证角色/权限变更无需重新登录。
    if (shouldRefreshSession) {
      const res = await getSelf().catch(() => null)
      if (res?.success && res.data) {
        // 验证成功，更新用户信息（可能有变化）
        auth.setUser(res.data)
        sessionVerified = true
        sessionVerifiedAt = Date.now()
      } else {
        // 验证失败或 API 调用失败，清除本地缓存并跳转登录页
        auth.reset()
        sessionVerified = false
        sessionVerifiedAt = 0
        throw redirect({
          to: '/sign-in',
          search: { redirect: location.href },
        })
      }
    }
  },
  component: AuthenticatedLayout,
})
