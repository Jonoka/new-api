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
import {
  Activity,
  Box,
  Brush,
  CreditCard,
  ExternalLink,
  Gamepad2,
  FileText,
  FlaskConical,
  Key,
  LayoutDashboard,
  ListTodo,
  MessageSquare,
  Radio,
  ReceiptText,
  Settings,
  Ticket,
  User,
  HandCoins,
  Users,
  Wallet,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getCanvasSettingsFromSidebarModules } from '@/lib/canvas-settings'
import {
  getCustomNavIcon,
  getSidebarCustomModuleKey,
  parseCustomNavItems,
} from '@/lib/custom-nav'
import { parseSidebarModulesFromStatus } from '@/lib/nav-modules'
import { useStatus } from '@/hooks/use-status'
import { type SidebarData } from '@/components/layout/types'

/**
 * Root navigation groups for the application sidebar.
 *
 * These are shown when the URL does not match any nested sidebar view
 * registered in `layout/lib/sidebar-view-registry.ts`.
 */
export function useSidebarData(): SidebarData {
  const { t } = useTranslation()
  const { status } = useStatus()
  const sidebarModules = parseSidebarModulesFromStatus(
    status as Record<string, unknown> | null
  )
  const canvasSettings = getCanvasSettingsFromSidebarModules(
    (status as Record<string, unknown> | null)?.SidebarModulesAdmin
  )
  const CanvasIcon = getCustomNavIcon(canvasSettings.canvasIcon) ?? Brush
  const customItems = parseCustomNavItems(sidebarModules.customItems)

  const sidebarData: SidebarData = {
    navGroups: [
      {
        id: 'chat',
        title: t('Chat'),
        items: [
          {
            title: t('Playground'),
            url: '/playground',
            icon: FlaskConical,
          },
          {
            title: t('Infinite Canvas'),
            url: '/canvas',
            icon: CanvasIcon,
          },
          {
            title: t('Chat'),
            icon: MessageSquare,
            type: 'chat-presets',
          },
        ],
      },
      {
        id: 'general',
        title: t('General'),
        items: [
          {
            title: t('Overview'),
            url: '/dashboard/overview',
            icon: Activity,
          },
          {
            title: t('Dashboard'),
            url: '/dashboard/models',
            icon: LayoutDashboard,
          },
          {
            title: t('API Keys'),
            url: '/keys',
            icon: Key,
          },
          {
            title: t('Usage Logs'),
            url: '/usage-logs/common',
            icon: FileText,
          },
          {
            title: t('Task Logs'),
            url: '/usage-logs/task',
            activeUrls: ['/usage-logs/drawing'],
            configUrls: ['/usage-logs/drawing', '/usage-logs/task'],
            icon: ListTodo,
          },
          {
            title: t('Game Center'),
            url: '/game-center',
            activeUrls: ['/game-center'],
            icon: Gamepad2,
          },
        ],
      },
      {
        id: 'personal',
        title: t('Personal'),
        items: [
          {
            title: t('Wallet'),
            url: '/wallet',
            icon: Wallet,
          },
          {
            title: t('Invoice Center'),
            url: '/invoices',
            icon: ReceiptText,
          },
          {
            title: t('Affiliate Commission'),
            url: '/affiliate',
            icon: HandCoins,
          },
          {
            title: t('Profile'),
            url: '/profile',
            icon: User,
          },
        ],
      },
      {
        id: 'admin',
        title: t('Admin'),
        items: [
          {
            title: t('Channels'),
            url: '/channels',
            icon: Radio,
          },
          {
            title: t('Models'),
            url: '/models/metadata',
            icon: Box,
          },
          {
            title: t('Users'),
            url: '/users',
            icon: Users,
          },
          {
            title: t('Marketing Benefits'),
            url: '/redemption-codes',
            icon: Ticket,
          },
          {
            title: t('Subscription Management'),
            url: '/subscriptions',
            icon: CreditCard,
          },
          {
            title: t('Invoice Management'),
            url: '/invoice-management',
            icon: ReceiptText,
          },
          {
            title: t('Affiliate Commission'),
            url: '/system-settings/billing/affiliate',
            icon: HandCoins,
          },
          {
            title: t('Game Management'),
            url: '/game-management',
            icon: Gamepad2,
          },
          {
            title: t('System Settings'),
            url: '/system-settings/site',
            activeUrls: ['/system-settings'],
            icon: Settings,
          },
        ],
      },
    ],
  }

  customItems.forEach((item) => {
    const group = sidebarData.navGroups.find(
      (navGroup) => navGroup.id === item.section
    )
    if (!group) return

    group.items.push({
      title: item.title,
      url: item.url,
      icon: getCustomNavIcon(item.icon) ?? ExternalLink,
      external: item.external,
      configUrls: [getSidebarCustomModuleKey(item.id)],
    })
  })

  return sidebarData
}
