import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  parseHeaderNavModules,
  parseSidebarModulesFromStatus,
} from './nav-modules'

describe('navigation module configuration', () => {
  test('preserves custom header items from status', () => {
    const modules = parseHeaderNavModules(
      JSON.stringify({
        home: false,
        customItems: [
          {
            id: 'canvas',
            title: '无限画布',
            url: '/canvas',
            enabled: true,
            icon: 'Brush',
            order: 10,
          },
        ],
      })
    )

    assert.equal(modules.home, false)
    assert.equal(modules.customItems.length, 1)
    assert.equal(modules.customItems[0].id, 'canvas')
  })

  test('preserves custom sidebar items from status', () => {
    const modules = parseSidebarModulesFromStatus({
      SidebarModulesAdmin: JSON.stringify({
        chat: { enabled: true, canvas: true },
        customItems: [
          {
            id: 'canvas-docs',
            title: 'Canvas Docs',
            url: 'https://docs.canvas.best',
            enabled: true,
            icon: 'BookOpen',
            order: 20,
            section: 'chat',
          },
        ],
      }),
    })

    assert.equal(modules.customItems.length, 1)
    assert.equal(modules.customItems[0].id, 'canvas-docs')
    assert.deepEqual(modules.chat, { enabled: true, canvas: true })
  })
})
