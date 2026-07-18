import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  createUniqueGroupCode,
  getGroupIdDisplayValue,
  getGroupNameByCode,
  reserveGroupCodes,
} from './group-identity.ts'

describe('分组内部稳定标识', () => {
  test('从 group_1 开始生成', () => {
    assert.equal(createUniqueGroupCode([]), 'group_1')
  })

  test('跳过已有标识并复用最小可用序号', () => {
    assert.equal(
      createUniqueGroupCode(['default', ' group_1 ', 'group_3']),
      'group_2'
    )
  })

  test('删除分组后仍保留本次页面曾占用的标识', () => {
    const loadedCodes = reserveGroupCodes(new Set(), ['group_1', 'group_2'])
    const codesAfterDelete = reserveGroupCodes(loadedCodes, ['group_1'])

    assert.equal(createUniqueGroupCode(codesAfterDelete), 'group_3')
  })

  test('保留持久化 ID，新分组显示 New', () => {
    assert.equal(getGroupIdDisplayValue(12), 12)
    assert.equal(getGroupIdDisplayValue(), 'New')
    assert.equal(getGroupIdDisplayValue(0), 'New')
  })

  test('高级规则只解析当前显示名称，不回显内部值', () => {
    const groups = [{ code: 'group_1', name: '高级分组' }]

    assert.equal(getGroupNameByCode(groups, 'group_1'), '高级分组')
    assert.equal(getGroupNameByCode(groups, 'missing'), undefined)
  })
})
