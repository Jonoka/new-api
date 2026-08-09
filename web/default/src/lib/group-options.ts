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

export type GroupMode = 'inherit' | 'explicit' | 'auto'

export interface GroupIdentity {
  id: number
  code: string
  name: string
  exclusive?: boolean
}

export interface GroupOption {
  id: number | null
  code: string
  name: string
  value: string
  label: string
  description?: string
  ratio?: number | string
  exclusive: boolean
}

export interface UserGroupInfo {
  id?: number | null
  code?: string
  name?: string
  desc?: string
  description?: string
  ratio?: number | string
  exclusive?: boolean
}

export type UserGroupMap = Record<string, UserGroupInfo>

export interface GroupSelectionRecord {
  group?: string | null
  group_ids?: readonly number[] | null
  group_details?: readonly GroupIdentity[] | null
  group_mode?: GroupMode | null
}

export interface GroupSelectionPayload {
  group: string
  /** ID 无法完整解析时省略，交给后端按兼容 code 回退。 */
  group_ids?: number[]
  group_mode: GroupMode
}

interface GroupOptionSource {
  id?: number | null
  code?: string | null
  name?: string | null
  description?: string | null
  ratio?: number | string | null
  exclusive?: boolean | null
}

function normalizeGroupId(value: unknown): number | null {
  const id = Number(value)
  return Number.isInteger(id) && id > 0 ? id : null
}

function normalizeText(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function deduplicateCodes(codes: readonly string[]): string[] {
  const result: string[] = []
  const seen = new Set<string>()

  for (const rawCode of codes) {
    const code = rawCode.trim()
    if (!code || seen.has(code)) continue
    seen.add(code)
    result.push(code)
  }

  return result
}

export function parseGroupCodes(value: string | null | undefined): string[] {
  if (!value) return []
  return deduplicateCodes(value.split(','))
}

function createGroupOption(
  source: GroupOptionSource,
  fallbackCode = ''
): GroupOption | null {
  const code = normalizeText(source.code) || fallbackCode.trim()
  if (!code) return null

  const name = normalizeText(source.name) || code
  const description = normalizeText(source.description)
  const ratio = source.ratio ?? undefined

  return {
    id: normalizeGroupId(source.id),
    code,
    name,
    value: code,
    label: name,
    description: description || undefined,
    ratio,
    exclusive: source.exclusive === true,
  }
}

export function createGroupOptions(
  groups: readonly GroupOptionSource[] | null | undefined
): GroupOption[] {
  if (!groups) return []

  const result: GroupOption[] = []
  const seen = new Set<string>()

  for (const group of groups) {
    const option = createGroupOption(group)
    if (!option || seen.has(option.code)) continue
    seen.add(option.code)
    result.push(option)
  }

  return result
}

function buildUserGroupDescription(
  code: string,
  name: string,
  description: string
): string | undefined {
  if (!description || description === code || description === name) {
    return undefined
  }
  return description
}

export function createUserGroupOptions(groups: UserGroupMap): GroupOption[] {
  const result: GroupOption[] = []
  const seen = new Set<string>()

  for (const [mapCode, info] of Object.entries(groups)) {
    const code = normalizeText(info.code) || mapCode.trim()
    if (!code || seen.has(code)) continue

    const name = normalizeText(info.name) || code
    const rawDescription =
      normalizeText(info.description) || normalizeText(info.desc)
    const option = createGroupOption(
      {
        id: code === 'auto' ? null : info.id,
        code,
        name,
        description: buildUserGroupDescription(code, name, rawDescription),
        ratio: info.ratio,
        exclusive: info.exclusive === true,
      },
      mapCode
    )

    if (!option) continue
    seen.add(option.code)
    result.push(option)
  }

  return result
}

export function includeSelectedGroupOptions(
  options: readonly GroupOption[],
  selectedCodes: readonly string[],
  references: readonly GroupIdentity[] = []
): GroupOption[] {
  const result = [...options]
  const byCode = new Map<string, GroupOption>(
    result.map((option) => [option.code, option])
  )
  const indexByCode = new Map<string, number>(
    result.map((option, index) => [option.code, index])
  )

  for (const reference of references) {
    const option = createGroupOption(reference)
    if (!option) continue

    const existing = byCode.get(option.code)
    if (existing) {
      const mergedOption: GroupOption = {
        ...existing,
        id: existing.id ?? option.id,
        exclusive: existing.exclusive || option.exclusive,
      }
      const existingIndex = indexByCode.get(option.code)
      if (existingIndex !== undefined) result[existingIndex] = mergedOption
      byCode.set(option.code, mergedOption)
      continue
    }

    byCode.set(option.code, option)
    indexByCode.set(option.code, result.length)
    result.push(option)
  }

  for (const code of deduplicateCodes(selectedCodes)) {
    if (byCode.has(code)) continue
    const option = createGroupOption({ code, name: code })
    if (!option) continue
    byCode.set(code, option)
    indexByCode.set(code, result.length)
    result.push(option)
  }

  return result
}

export function resolveGroupSelectionCodes(
  record: GroupSelectionRecord,
  options: readonly GroupOption[] = []
): string[] {
  if (record.group_mode === 'auto') return ['auto']
  if (record.group_mode === 'inherit') return []

  const legacyCodes = parseGroupCodes(record.group)
  if (
    record.group_mode == null &&
    legacyCodes.length === 1 &&
    legacyCodes[0]?.toLowerCase() === 'auto'
  ) {
    return ['auto']
  }

  const codeById = new Map<number, string>()
  for (const reference of record.group_details ?? []) {
    const id = normalizeGroupId(reference.id)
    const code = normalizeText(reference.code)
    if (id && code) codeById.set(id, code)
  }
  for (const option of options) {
    if (option.id) codeById.set(option.id, option.code)
  }

  const groupIds = Array.from(
    new Set(
      (record.group_ids ?? [])
        .map((id) => normalizeGroupId(id))
        .filter((id): id is number => id !== null)
    )
  )
  if (groupIds.length > 0) {
    const idCodes = groupIds.map((id) => codeById.get(id))
    if (idCodes.every((code): code is string => Boolean(code))) {
      return deduplicateCodes(idCodes)
    }
  }

  const referenceCodes = deduplicateCodes(
    (record.group_details ?? []).map((reference) => reference.code)
  )
  if (referenceCodes.length > 0) return referenceCodes

  return legacyCodes
}

export function buildGroupSelectionPayload(
  selectedCodes: readonly string[],
  options: readonly GroupOption[]
): GroupSelectionPayload {
  const codes = deduplicateCodes(selectedCodes)

  if (codes.length === 1 && codes[0]?.toLowerCase() === 'auto') {
    return { group: 'auto', group_ids: [], group_mode: 'auto' }
  }

  if (codes.length === 0) {
    return { group: '', group_ids: [], group_mode: 'inherit' }
  }

  const idByCode = new Map<string, number>()
  for (const option of options) {
    if (option.id) idByCode.set(option.code, option.id)
  }

  const resolvedIds = codes.map((code) => idByCode.get(code))
  const groupIds = resolvedIds.every((id): id is number => id !== undefined)
    ? resolvedIds
    : undefined

  return {
    group: codes.join(','),
    group_ids: groupIds,
    group_mode: 'explicit',
  }
}
