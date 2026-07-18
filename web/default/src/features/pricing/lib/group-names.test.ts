import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { getGroupDisplayName } from './group-names.ts'

describe('模型广场分组显示名称', () => {
  test('使用接口返回的名称展示', () => {
    assert.equal(
      getGroupDisplayName('group_1', { group_1: '高级分组' }),
      '高级分组'
    )
  })

  test('缺少映射或名称为空时回退到内部值', () => {
    assert.equal(getGroupDisplayName('legacy', {}), 'legacy')
    assert.equal(getGroupDisplayName('legacy', { legacy: '  ' }), 'legacy')
  })
})
