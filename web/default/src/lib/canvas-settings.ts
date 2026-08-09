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
import { getCustomNavIcon, type CustomNavIconName } from './custom-nav'

export const DEFAULT_CANVAS_APP_ORIGIN = 'https://canvas.jo2api.com'
export const DEFAULT_CANVAS_ICON: CustomNavIconName = 'Brush'

export type CanvasModuleSettings = {
  canvasOrigin: string
  canvasIcon: CustomNavIconName
}

function parseSidebarModulesRecord(
  raw: unknown
): Record<string, unknown> | null {
  if (!raw || String(raw).trim() === '') return null
  if (raw && typeof raw === 'object') return raw as Record<string, unknown>

  try {
    return JSON.parse(String(raw)) as Record<string, unknown>
  } catch {
    return null
  }
}

export function normalizeCanvasOrigin(
  value: unknown,
  fallback = DEFAULT_CANVAS_APP_ORIGIN
): string {
  const raw = typeof value === 'string' ? value.trim() : ''
  if (!raw) return fallback

  const candidate = /^[a-z][a-z\d+.-]*:\/\//i.test(raw) ? raw : `https://${raw}`

  try {
    const url = new URL(candidate)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return fallback
    return url.origin
  } catch {
    return fallback
  }
}

export function normalizeCanvasIcon(
  value: unknown,
  fallback = DEFAULT_CANVAS_ICON
): CustomNavIconName {
  return getCustomNavIcon(value) ? (value as CustomNavIconName) : fallback
}

export function getCanvasSettingsFromSidebarModules(
  raw: unknown
): CanvasModuleSettings {
  const parsed = parseSidebarModulesRecord(raw)
  const chat = parsed?.chat
  const chatConfig =
    chat && typeof chat === 'object' && !Array.isArray(chat)
      ? (chat as Record<string, unknown>)
      : {}

  return {
    canvasOrigin: normalizeCanvasOrigin(chatConfig.canvasOrigin),
    canvasIcon: normalizeCanvasIcon(chatConfig.canvasIcon),
  }
}
