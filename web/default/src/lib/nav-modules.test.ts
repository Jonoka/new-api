import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  getCanvasSettingsFromSidebarModules,
  normalizeCanvasOrigin,
} from './canvas-settings'
import {
  parseHeaderNavModules,
  parseSidebarModulesFromStatus,
} from './nav-modules'

function asRecord(value: unknown): Record<string, unknown> {
  assert.equal(typeof value, 'object')
  assert.equal(Array.isArray(value), false)
  assert.notEqual(value, null)
  return value as Record<string, unknown>
}

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
    const chat = asRecord(modules.chat)
    assert.equal(chat.enabled, true)
    assert.equal(chat.canvas, true)
  })

  test('reads configurable canvas launcher settings from sidebar modules', () => {
    const raw = JSON.stringify({
      chat: {
        enabled: true,
        canvas: true,
        canvasOrigin: 'canvas.example.com/path',
        canvasIcon: 'Sparkles',
      },
    })

    const modules = parseSidebarModulesFromStatus({
      SidebarModulesAdmin: raw,
    })
    const settings = getCanvasSettingsFromSidebarModules(raw)

    const chat = asRecord(modules.chat)
    assert.equal(chat.canvasOrigin, 'https://canvas.example.com')
    assert.equal(chat.canvasIcon, 'Sparkles')
    assert.equal(settings.canvasOrigin, 'https://canvas.example.com')
    assert.equal(settings.canvasIcon, 'Sparkles')
  })

  test('falls back when canvas origin is invalid', () => {
    assert.equal(
      normalizeCanvasOrigin('javascript:alert(1)'),
      'https://canvas.maolaoapi.com'
    )
  })

  test('preserves header placement on sidebar-managed custom items', () => {
    const modules = parseSidebarModulesFromStatus({
      SidebarModulesAdmin: JSON.stringify({
        customItems: [
          {
            id: 'api-docs',
            title: 'API Docs',
            url: '/docs',
            enabled: true,
            order: 10,
            section: 'header',
          },
        ],
      }),
    })

    assert.equal(modules.customItems[0].section, 'header')
  })
})
