import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { applyAutoGroupOrder } from './auto-group-order.ts'

describe('自动分组排序', () => {
  test('按拖拽后的稳定 key 更新启用状态和连续顺序', () => {
    const groups = [
      { _key: 'a', auto_enabled: true, auto_order: 0, name: 'A' },
      { _key: 'b', auto_enabled: true, auto_order: 1, name: 'B' },
      { _key: 'c', auto_enabled: false, auto_order: 0, name: 'C' },
    ]

    const reordered = applyAutoGroupOrder(groups, ['c', 'a'])

    assert.deepEqual(
      reordered.map(({ _key, auto_enabled, auto_order }) => ({
        _key,
        auto_enabled,
        auto_order,
      })),
      [
        { _key: 'a', auto_enabled: true, auto_order: 1 },
        { _key: 'b', auto_enabled: false, auto_order: 0 },
        { _key: 'c', auto_enabled: true, auto_order: 0 },
      ]
    )
    assert.equal(groups[0].auto_order, 0)
  })

  test('空顺序会停用全部自动分组', () => {
    const groups = [
      { _key: 'a', auto_enabled: true, auto_order: 3 },
      { _key: 'b', auto_enabled: true, auto_order: 7 },
    ]

    assert.deepEqual(
      applyAutoGroupOrder(groups, []).map(
        ({ _key, auto_enabled, auto_order }) => ({
          _key,
          auto_enabled,
          auto_order,
        })
      ),
      [
        { _key: 'a', auto_enabled: false, auto_order: 0 },
        { _key: 'b', auto_enabled: false, auto_order: 0 },
      ]
    )
  })
})
