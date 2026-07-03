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
*/
import {
  Activity,
  BookOpen,
  Bot,
  Box,
  Brush,
  ChartNoAxesColumn,
  CircleHelp,
  CreditCard,
  ExternalLink,
  FileText,
  FlaskConical,
  Gamepad2,
  Globe,
  HandCoins,
  Home,
  Key,
  LayoutDashboard,
  Link as LinkIcon,
  ListTodo,
  MessageSquare,
  Puzzle,
  Radio,
  Settings,
  Sparkles,
  Ticket,
  User,
  Users,
  Wallet,
  type LucideIcon,
} from 'lucide-react'

export type CustomNavSection = 'chat' | 'console' | 'personal' | 'admin'
export type CustomNavTarget = 'same' | 'blank'

export type CustomMenuItemConfig = {
  id: string
  title: string
  url: string
  enabled: boolean
  icon?: string
  order: number
  requireAuth?: boolean
  openInNewTab?: boolean
  section?: CustomNavSection | string
}

export type CustomNavItem = Omit<CustomMenuItemConfig, 'section'> & {
  section?: CustomNavSection
  external: boolean
}

export type CustomNavItemInput = Partial<CustomMenuItemConfig> & {
  href?: string
  target?: CustomNavTarget
}

type ParseCustomNavItemsOptions = {
  includeDisabled?: boolean
}

const CUSTOM_NAV_ICONS = {
  Activity,
  BookOpen,
  Bot,
  Box,
  Brush,
  ChartNoAxesColumn,
  CircleHelp,
  CreditCard,
  ExternalLink,
  FileText,
  FlaskConical,
  Gamepad2,
  Globe,
  HandCoins,
  Home,
  Key,
  LayoutDashboard,
  Link: LinkIcon,
  ListTodo,
  MessageSquare,
  Puzzle,
  Radio,
  Settings,
  Sparkles,
  Ticket,
  User,
  Users,
  Wallet,
} satisfies Record<string, LucideIcon>

export type CustomNavIconName = keyof typeof CUSTOM_NAV_ICONS

export const CUSTOM_NAV_ICON_OPTIONS = Object.keys(
  CUSTOM_NAV_ICONS
) as CustomNavIconName[]

const SIDEBAR_SECTIONS = new Set<CustomNavSection>([
  'chat',
  'console',
  'personal',
  'admin',
])

function normalizeId(value: unknown, fallback: string): string {
  const raw = typeof value === 'string' ? value.trim() : ''
  const normalized = raw
    .replace(/\s+/g, '-')
    .replace(/[^\p{L}\p{N}:_.-]+/gu, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
  return normalized || fallback
}

function normalizeUrl(
  value: unknown
): { url: string; external: boolean } | null {
  if (typeof value !== 'string') return null
  const raw = value.trim()
  if (!raw) return null

  if (raw.startsWith('/')) {
    if (raw.startsWith('//')) return null
    return { url: raw, external: false }
  }

  try {
    const url = new URL(raw)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return null
    return { url: url.toString(), external: true }
  } catch {
    return null
  }
}

function toBoolean(value: unknown, fallback: boolean): boolean {
  if (typeof value === 'boolean') return value
  if (typeof value === 'number') return value === 1
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase()
    if (normalized === 'true' || normalized === '1') return true
    if (normalized === 'false' || normalized === '0') return false
  }
  return fallback
}

function toOrder(value: unknown, fallback: number): number {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return fallback
}

export function getCustomNavIcon(name: unknown): LucideIcon | undefined {
  if (typeof name !== 'string') return undefined
  return CUSTOM_NAV_ICONS[name as CustomNavIconName]
}

export function getSidebarCustomModuleKey(id: string): string {
  return `custom:${normalizeId(id, 'item')}`
}

export function parseCustomNavItems(
  value: unknown,
  options: ParseCustomNavItemsOptions = {}
): CustomNavItem[] {
  if (!Array.isArray(value)) return []

  return value
    .map((raw, index): CustomNavItem | null => {
      if (!raw || typeof raw !== 'object') return null
      const record = raw as CustomNavItemInput
      const normalizedUrl = normalizeUrl(record.url ?? record.href)
      if (!normalizedUrl) return null

      const title = typeof record.title === 'string' ? record.title.trim() : ''
      if (!title) return null

      const id = normalizeId(record.id, `custom-${index + 1}`)
      const section =
        typeof record.section === 'string' &&
        SIDEBAR_SECTIONS.has(record.section as CustomNavSection)
          ? (record.section as CustomNavSection)
          : undefined
      const icon = getCustomNavIcon(record.icon) ? record.icon : undefined
      const openInNewTab =
        record.target === 'blank'
          ? true
          : toBoolean(record.openInNewTab, normalizedUrl.external)

      return {
        id,
        title,
        url: normalizedUrl.url,
        enabled: toBoolean(record.enabled, true),
        icon,
        order: toOrder(record.order, index),
        requireAuth: toBoolean(record.requireAuth, false),
        openInNewTab,
        section,
        external: normalizedUrl.external || openInNewTab,
      }
    })
    .filter(
      (item): item is CustomNavItem =>
        item !== null && (options.includeDisabled || item.enabled)
    )
    .sort((a, b) => a.order - b.order || a.title.localeCompare(b.title))
}
