import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { describe, test } from 'node:test'
import { fileURLToPath } from 'node:url'
import { createGroupOptions } from '../../lib/group-options.ts'

const root = dirname(fileURLToPath(import.meta.url))
const readSource = (...parts: string[]) =>
  readFileSync(resolve(root, ...parts), 'utf8')

describe('用户编辑器分组显示名称', () => {
  test('显示当前名称但保留内部 code 作为提交值', () => {
    assert.deepEqual(
      createGroupOptions([
        {
          id: 42,
          code: 'Codex-Team',
          name: 'Codex-福利组',
        },
      ]),
      [
        {
          id: 42,
          code: 'Codex-Team',
          name: 'Codex-福利组',
          value: 'Codex-Team',
          label: 'Codex-福利组',
          description: undefined,
          ratio: undefined,
          exclusive: false,
        },
      ]
    )
  })

  test('用户编辑器复用结构化分组接口和统一选项 helper', () => {
    const source = readSource('components/users-mutate-drawer.tsx')

    assert.match(source, /queryFn:\s*getGroupDetails/)
    assert.match(source, /createGroupOptions\(groupsData\?\.data\)/)
    assert.match(source, /value=\{group\.value\}/)
    assert.match(source, /\{group\.label\}/)
    assert.doesNotMatch(source, /queryFn:\s*getGroups/)
  })
})
