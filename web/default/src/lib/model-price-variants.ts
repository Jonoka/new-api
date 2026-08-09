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

export type ModelPriceVariantRule = {
  resolution?: string
  quality?: string
  price: number
}

export type ModelPriceVariantConfig = {
  resolution_enabled: boolean
  quality_enabled: boolean
  rules?: ModelPriceVariantRule[]
  inherited?: boolean
}

export type ModelPriceVariantsMap = Record<string, ModelPriceVariantConfig>

export type ModelPriceVariantRange = {
  minimum: number
  maximum: number
  hasVariants: boolean
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** 与后端匹配规则保持一致，别名归一后也用于重复组合校验。 */
export function normalizeModelPriceVariantResolution(value: string): string {
  const normalized = value.trim().toLowerCase()
  switch (normalized) {
    case '480':
    case '480p':
    case 'sd':
      return '480p'
    case '720':
    case '720p':
    case 'hd':
      return '720p'
    case '1080':
    case '1080p':
    case 'fhd':
    case 'full-hd':
    case 'full_hd':
      return '1080p'
    case '2k':
      return '2k'
    case '4k':
      return '4k'
    default:
      return normalized
  }
}

export function normalizeModelPriceVariantQuality(value: string): string {
  return value.trim().toLowerCase()
}

export function getModelPriceVariantCombinationKey(
  resolution: string,
  quality: string
): string {
  return `${normalizeModelPriceVariantResolution(resolution)}\u0000${normalizeModelPriceVariantQuality(quality)}`
}

function isModelPriceVariantRule(
  value: unknown
): value is ModelPriceVariantRule {
  if (!isRecord(value)) return false
  if (value.resolution !== undefined && typeof value.resolution !== 'string') {
    return false
  }
  if (value.quality !== undefined && typeof value.quality !== 'string') {
    return false
  }
  return (
    typeof value.price === 'number' &&
    Number.isFinite(value.price) &&
    value.price >= 0
  )
}

export function isModelPriceVariantConfig(
  value: unknown
): value is ModelPriceVariantConfig {
  if (!isRecord(value)) return false
  if (typeof value.resolution_enabled !== 'boolean') return false
  if (typeof value.quality_enabled !== 'boolean') return false
  if (value.inherited !== undefined && typeof value.inherited !== 'boolean') {
    return false
  }
  if (value.rules !== undefined && !Array.isArray(value.rules)) return false

  const rules = value.rules ?? []
  if (!rules.every(isModelPriceVariantRule)) return false
  if (!value.resolution_enabled && !value.quality_enabled) return true
  if (rules.length === 0) return false

  const combinations = new Set<string>()
  for (const rule of rules) {
    const resolution = value.resolution_enabled
      ? normalizeModelPriceVariantResolution(rule.resolution ?? '')
      : ''
    const quality = value.quality_enabled
      ? normalizeModelPriceVariantQuality(rule.quality ?? '')
      : ''
    if (value.resolution_enabled && !resolution) return false
    if (value.quality_enabled && !quality) return false

    const combination = `${resolution}\u0000${quality}`
    if (combinations.has(combination)) return false
    combinations.add(combination)
  }

  return true
}

export function isModelPriceVariantsMap(
  value: unknown
): value is ModelPriceVariantsMap {
  if (!isRecord(value)) return false
  return Object.entries(value).every(
    ([modelName, config]) =>
      modelName.trim() !== '' && isModelPriceVariantConfig(config)
  )
}

export function isGrokImagineVideoModel(modelName: string): boolean {
  return modelName.trim().toLowerCase().startsWith('grok-imagine-video')
}

export function getModelPriceVariantRules(
  config: ModelPriceVariantConfig | null | undefined
): ModelPriceVariantRule[] {
  return config?.rules ?? []
}

export function hasActiveModelPriceVariants(
  config: ModelPriceVariantConfig | null | undefined
): boolean {
  return Boolean(
    config &&
    (config.resolution_enabled || config.quality_enabled) &&
    getModelPriceVariantRules(config).length > 0
  )
}

export function cloneModelPriceVariantConfig(
  config: ModelPriceVariantConfig,
  inherited: boolean | undefined = config.inherited
): ModelPriceVariantConfig {
  return {
    resolution_enabled: config.resolution_enabled,
    quality_enabled: config.quality_enabled,
    rules: config.rules?.map((rule) => ({ ...rule })),
    ...(inherited === undefined ? {} : { inherited }),
  }
}

/** 规则价是最终固定单价；范围同时包含未匹配时使用的基础兜底价。 */
export function getModelPriceVariantRange(
  basePrice: number | null | undefined,
  config: ModelPriceVariantConfig | null | undefined
): ModelPriceVariantRange {
  const normalizedBasePrice =
    typeof basePrice === 'number' && Number.isFinite(basePrice)
      ? Math.max(basePrice, 0)
      : 0
  const rules = hasActiveModelPriceVariants(config)
    ? getModelPriceVariantRules(config).filter(
        (rule) => Number.isFinite(rule.price) && rule.price >= 0
      )
    : []
  const prices = [normalizedBasePrice, ...rules.map((rule) => rule.price)]

  return {
    minimum: Math.min(...prices),
    maximum: Math.max(...prices),
    hasVariants: rules.length > 0,
  }
}
