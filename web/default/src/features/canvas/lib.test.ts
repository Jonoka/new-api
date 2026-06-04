import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { buildCanvasLaunchUrl } from './lib'

describe('buildCanvasLaunchUrl', () => {
  test('builds a New API session launch URL without apiKey', () => {
    const url = buildCanvasLaunchUrl({
      canvasOrigin: 'https://canvas.maolaoapi.com',
      newApiOrigin: 'https://maolaoapi.com',
      group: 'vip group',
    })

    assert.equal(
      url,
      'https://canvas.maolaoapi.com/?mode=newapi&baseUrl=https%3A%2F%2Fmaolaoapi.com%2Fcanvas&group=vip+group'
    )
    assert.equal(url.includes('apiKey'), false)
  })
})
