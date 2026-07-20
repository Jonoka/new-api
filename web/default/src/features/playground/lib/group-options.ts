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
import type { GroupOption } from '../types'

export interface PlaygroundUserGroupInfo {
  name?: string
  desc?: string
  ratio: number
}

export type PlaygroundUserGroupMap = Record<string, PlaygroundUserGroupInfo>

function getGroupDescription(
  description: string | undefined,
  code: string,
  name: string
): string | undefined {
  const normalized = description?.trim()
  if (!normalized || normalized === code || normalized === name)
    return undefined
  return normalized
}

export function createPlaygroundGroupOptions(
  groups: PlaygroundUserGroupMap
): GroupOption[] {
  return Object.entries(groups).map(([code, info]) => {
    const name = info.name?.trim() || code
    return {
      label: name,
      value: code,
      ratio: info.ratio,
      desc: getGroupDescription(info.desc, code, name),
    }
  })
}
