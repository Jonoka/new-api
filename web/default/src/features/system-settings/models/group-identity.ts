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

/** 为新分组生成仅供内部使用、且不与现有分组冲突的稳定标识。 */
export function createUniqueGroupCode(existingCodes: Iterable<string>): string {
  const normalizedCodes = new Set(
    Array.from(existingCodes, (code) => code.trim())
  )
  let suffix = 1

  while (normalizedCodes.has(`group_${suffix}`)) {
    suffix += 1
  }

  return `group_${suffix}`
}

/** 只追加本次页面已经占用过的标识，删除分组时也不会释放。 */
export function reserveGroupCodes(
  reservedCodes: ReadonlySet<string>,
  occupiedCodes: Iterable<string>
): Set<string> {
  const nextReservedCodes = new Set(reservedCodes)
  for (const code of occupiedCodes) {
    const normalizedCode = code.trim()
    if (normalizedCode) nextReservedCodes.add(normalizedCode)
  }
  return nextReservedCodes
}

/** 按内部值解析当前显示名称，找不到时不把内部值回显给管理员。 */
export function getGroupNameByCode(
  groups: Iterable<{ code: string; name: string }>,
  code: string
): string | undefined {
  for (const group of groups) {
    if (group.code === code) return group.name.trim() || undefined
  }
  return undefined
}

/** 返回只读 ID 列的展示值，尚未持久化的分组统一显示 New。 */
export function getGroupIdDisplayValue(id?: number): number | 'New' {
  return id && id > 0 ? id : 'New'
}
