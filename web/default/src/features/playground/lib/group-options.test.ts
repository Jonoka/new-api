import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { createPlaygroundGroupOptions } from './group-options.ts'

describe('操练场分组显示名称', () => {
  test('显示当前名称但保留接口对象键作为选中值', () => {
    assert.deepEqual(
      createPlaygroundGroupOptions({
        'Codex-Team': {
          name: 'Codex-福利组',
          desc: '福利线路',
          ratio: 0.05,
        },
      }),
      [
        {
          label: 'Codex-福利组',
          value: 'Codex-Team',
          ratio: 0.05,
          desc: '福利线路',
        },
      ]
    )
  })

  test('缺少名称时回退到接口对象键', () => {
    const [option] = createPlaygroundGroupOptions({
      default: { ratio: 1 },
    })

    assert.equal(option?.label, 'default')
    assert.equal(option?.value, 'default')
  })

  test('描述与名称或内部值相同时不重复显示', () => {
    const options = createPlaygroundGroupOptions({
      first: { name: '福利组', desc: '福利组', ratio: 1 },
      second: { name: '专线组', desc: 'second', ratio: 1 },
    })

    assert.equal(options[0]?.desc, undefined)
    assert.equal(options[1]?.desc, undefined)
  })
})
