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
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, test } from 'node:test'

const root = dirname(fileURLToPath(import.meta.url))
const readSource = (...parts: string[]) =>
  readFileSync(resolve(root, ...parts), 'utf8')

describe('root role management wiring', () => {
  test('only a root user receives the full create-role options', () => {
    const source = readSource('components/users-mutate-drawer.tsx')

    assert.match(source, /currentUserRole === USER_ROLE\.ROOT/)
    assert.match(source, /getUserRoleOptions\(t\)/)
  })

  test('root promotion and revocation use explicit actions', () => {
    const source = readSource('components/data-table-row-actions.tsx')

    assert.match(source, /'promote_root'/)
    assert.match(source, /'demote_root'/)
    assert.match(source, /user\.id !== currentUser\?\.id/)
    assert.match(source, /t\('Super Admin'\)/)
  })

  test('the action contract includes both root-role operations', () => {
    const source = readSource('types.ts')

    assert.match(source, /\| 'promote_root'/)
    assert.match(source, /\| 'demote_root'/)
  })
})
