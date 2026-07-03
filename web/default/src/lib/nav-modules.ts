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
import { getStatus } from '@/lib/api'
import { normalizeCanvasIcon, normalizeCanvasOrigin } from './canvas-settings'
import {
  getSidebarCustomModuleKey,
  type CustomMenuItemConfig,
} from './custom-nav'

export type ModuleAccess = { enabled: boolean; requireAuth: boolean }

export type HeaderNavModule = 'rankings' | 'pricing'

export type HeaderNavModules = {
  home: boolean
  console: boolean
  pricing: ModuleAccess
  rankings: ModuleAccess
  docs: boolean
  about: boolean
  customItems: CustomMenuItemConfig[]
  [key: string]: boolean | ModuleAccess | CustomMenuItemConfig[]
}

type SidebarSectionConfig = {
  enabled: boolean
  [key: string]: boolean | string
}

export type SidebarModules = Record<
  string,
  SidebarSectionConfig | CustomMenuItemConfig[]
> & {
  customItems: CustomMenuItemConfig[]
}

const DEFAULT_HEADER_NAV_MODULES: HeaderNavModules = {
  home: true,
  console: true,
  pricing: { enabled: true, requireAuth: false },
  rankings: { enabled: true, requireAuth: false },
  docs: true,
  about: true,
  customItems: [],
}

const DEFAULTS: Record<HeaderNavModule, ModuleAccess> = {
  pricing: DEFAULT_HEADER_NAV_MODULES.pricing,
  rankings: DEFAULT_HEADER_NAV_MODULES.rankings,
}

function cloneHeaderNavDefaults(): HeaderNavModules {
  return {
    ...DEFAULT_HEADER_NAV_MODULES,
    pricing: { ...DEFAULT_HEADER_NAV_MODULES.pricing },
    rankings: { ...DEFAULT_HEADER_NAV_MODULES.rankings },
    customItems: [...DEFAULT_HEADER_NAV_MODULES.customItems],
  }
}

export function parseHeaderNavBoolean(
  raw: unknown,
  fallback: boolean
): boolean {
  if (typeof raw === 'boolean') return raw
  if (typeof raw === 'number') {
    if (raw === 1) return true
    if (raw === 0) return false
    return fallback
  }
  if (typeof raw === 'string') {
    const normalized = raw.trim().toLowerCase()
    if (normalized === 'true' || normalized === '1') return true
    if (normalized === 'false' || normalized === '0') return false
  }
  return fallback
}

function parseAccess(raw: unknown, fallback: ModuleAccess): ModuleAccess {
  if (
    typeof raw === 'boolean' ||
    typeof raw === 'number' ||
    typeof raw === 'string'
  ) {
    return {
      enabled: parseHeaderNavBoolean(raw, fallback.enabled),
      requireAuth: fallback.requireAuth,
    }
  }
  if (raw && typeof raw === 'object') {
    const r = raw as Record<string, unknown>
    return {
      enabled: parseHeaderNavBoolean(r.enabled, fallback.enabled),
      requireAuth: parseHeaderNavBoolean(r.requireAuth, fallback.requireAuth),
    }
  }
  return { ...fallback }
}

function parseHeaderNavRecord(raw: unknown): Record<string, unknown> | null {
  if (!raw || String(raw).trim() === '') return null
  if (raw && typeof raw === 'object') return raw as Record<string, unknown>

  try {
    return JSON.parse(String(raw)) as Record<string, unknown>
  } catch {
    return null
  }
}

export function parseHeaderNavModules(raw: unknown): HeaderNavModules {
  const result = cloneHeaderNavDefaults()
  const parsed = parseHeaderNavRecord(raw)
  if (!parsed) return result

  Object.entries(parsed).forEach(([key, value]) => {
    if (key === 'customItems' && Array.isArray(value)) {
      result.customItems = value as CustomMenuItemConfig[]
      return
    }

    if (key === 'pricing') {
      result.pricing = parseAccess(value, result.pricing)
      return
    }
    if (key === 'rankings') {
      result.rankings = parseAccess(value, result.rankings)
      return
    }

    const fallback = result[key]
    if (
      typeof fallback === 'boolean' ||
      typeof value === 'boolean' ||
      typeof value === 'number' ||
      typeof value === 'string'
    ) {
      result[key] = parseHeaderNavBoolean(
        value,
        typeof fallback === 'boolean' ? fallback : true
      )
    }
  })

  return result
}

function parseSidebarModules(raw: unknown): SidebarModules {
  const result: SidebarModules = { customItems: [] }
  const parsed = parseHeaderNavRecord(raw)
  if (!parsed) return result

  Object.entries(parsed).forEach(([sectionKey, value]) => {
    if (sectionKey === 'customItems' && Array.isArray(value)) {
      result.customItems = value as CustomMenuItemConfig[]
      return
    }
    if (!value || typeof value !== 'object' || Array.isArray(value)) return
    const record = value as Record<string, unknown>
    const section: SidebarSectionConfig = {
      enabled: parseHeaderNavBoolean(record.enabled, true),
    }
    Object.entries(record).forEach(([moduleKey, moduleValue]) => {
      if (moduleKey === 'enabled') return
      if (sectionKey === 'chat' && moduleKey === 'canvasOrigin') {
        section.canvasOrigin = normalizeCanvasOrigin(moduleValue)
        return
      }
      if (sectionKey === 'chat' && moduleKey === 'canvasIcon') {
        section.canvasIcon = normalizeCanvasIcon(moduleValue)
        return
      }
      section[moduleKey] = parseHeaderNavBoolean(moduleValue, true)
    })
    result[sectionKey] = section
  })

  return result
}

export function parseSidebarModulesFromStatus(
  status: Record<string, unknown> | null
): SidebarModules {
  return parseSidebarModules(status?.SidebarModulesAdmin)
}

export function parseHeaderNavModulesFromStatus(
  status: Record<string, unknown> | null
): HeaderNavModules {
  return parseHeaderNavModules(status?.HeaderNavModules)
}

function getCachedStatus(): Record<string, unknown> | null {
  try {
    if (typeof window === 'undefined') return null
    const raw = window.localStorage.getItem('status')
    return raw ? (JSON.parse(raw) as Record<string, unknown>) : null
  } catch {
    return null
  }
}

function cacheStatus(status: Record<string, unknown> | null): void {
  try {
    if (typeof window !== 'undefined' && status) {
      window.localStorage.setItem('status', JSON.stringify(status))
    }
  } catch {
    /* empty */
  }
}

export function getModuleAccessFromStatus(
  status: Record<string, unknown> | null,
  module: HeaderNavModule
): ModuleAccess {
  return parseHeaderNavModulesFromStatus(status)[module] ?? DEFAULTS[module]
}

export function getModuleAccess(module: HeaderNavModule): ModuleAccess {
  return getModuleAccessFromStatus(getCachedStatus(), module)
}

export async function getFreshModuleAccess(
  module: HeaderNavModule
): Promise<ModuleAccess> {
  try {
    const status = (await getStatus()) as Record<string, unknown> | null
    cacheStatus(status)
    return getModuleAccessFromStatus(status, module)
  } catch {
    return { enabled: false, requireAuth: true }
  }
}

export function isSidebarModuleEnabled(
  section: string,
  module: string
): boolean {
  const status = getCachedStatus()
  if (!status) return true

  const raw = status.SidebarModulesAdmin
  if (!raw || String(raw).trim() === '') return true

  try {
    const parsed = parseSidebarModules(raw)
    if (section === '__custom') {
      const customSection = parsed.custom as Record<string, boolean> | undefined
      const customKey = getSidebarCustomModuleKey(module)
      if (customSection?.[customKey] === false) return false
      return true
    }
    const sectionConfig = parsed[section]
    if (!sectionConfig || Array.isArray(sectionConfig)) return true
    if (sectionConfig.enabled === false) return false
    if (sectionConfig[module] === false) return false
    return true
  } catch {
    return true
  }
}
