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
import { describe, test } from 'node:test'
import { fileURLToPath } from 'node:url'
import {
  createUserGroupOptions,
  includeSelectedGroupOptions,
} from '../../lib/group-options.ts'

const root = dirname(fileURLToPath(import.meta.url))

describe('独立令牌分组选项', () => {
  test('保留用户可用分组的独立属性', () => {
    const [option] = createUserGroupOptions({
      hack: {
        id: 7,
        code: 'hack',
        name: 'Hack',
        exclusive: true,
      },
    })

    assert.equal(option?.exclusive, true)
  })

  test('编辑历史令牌时保留分组引用的独立属性', () => {
    const [option] = includeSelectedGroupOptions(
      [],
      ['hack'],
      [{ id: 7, code: 'hack', name: 'Hack', exclusive: true }]
    )

    assert.equal(option?.exclusive, true)
  })

  test('令牌多选器同时限制 auto 和独立分组', () => {
    const source = readFileSync(
      resolve(root, 'components/api-key-group-multi-select.tsx'),
      'utf8'
    )

    assert.match(source, /isExclusiveSelected/)
    assert.match(source, /option\.exclusive && value\.length > 0/)
    assert.match(source, /onValueChange\(\[selectedValue\]\)/)
  })
})
