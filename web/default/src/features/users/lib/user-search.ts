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
import type { User } from '../types'

type SearchableUser = Pick<
  User,
  'id' | 'username' | 'display_name' | 'email'
>

export function matchesUserSearchFilter(
  user: SearchableUser,
  filterValue: unknown
): boolean {
  const searchValue = String(filterValue).trim().toLowerCase()
  if (!searchValue) return true

  return [user.id, user.username, user.display_name, user.email].some((field) =>
    String(field || '')
      .toLowerCase()
      .includes(searchValue)
  )
}
