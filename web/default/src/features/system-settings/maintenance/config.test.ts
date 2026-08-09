import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { parseSidebarModulesAdmin } from './config'

function asRecord(value: unknown): Record<string, unknown> {
  assert.equal(typeof value, 'object')
  assert.equal(Array.isArray(value), false)
  assert.notEqual(value, null)
  return value as Record<string, unknown>
}

describe('sidebar modules admin configuration', () => {
  test('preserves canvas domain and icon settings', () => {
    const config = parseSidebarModulesAdmin(
      JSON.stringify({
        chat: {
          enabled: true,
          canvas: true,
          canvasOrigin: 'canvas.example.com/app',
          canvasIcon: 'Sparkles',
        },
      })
    )

    const chat = asRecord(config.chat)
    assert.equal(chat.canvasOrigin, 'https://canvas.example.com')
    assert.equal(chat.canvasIcon, 'Sparkles')
  })
})
