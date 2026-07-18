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

export const MODEL_PRICE_UNITS = {
  REQUEST: 'request',
  SECOND: 'second',
} as const

export type ModelPriceUnit =
  (typeof MODEL_PRICE_UNITS)[keyof typeof MODEL_PRICE_UNITS]

/** 缺失或未知单位按后端兼容语义回退为按次。 */
export function normalizeModelPriceUnit(value: unknown): ModelPriceUnit {
  return value === MODEL_PRICE_UNITS.SECOND
    ? MODEL_PRICE_UNITS.SECOND
    : MODEL_PRICE_UNITS.REQUEST
}

export function isModelPriceUnitMap(
  value: unknown
): value is Record<string, ModelPriceUnit> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false

  return Object.values(value).every(
    (unit) =>
      unit === MODEL_PRICE_UNITS.REQUEST || unit === MODEL_PRICE_UNITS.SECOND
  )
}
