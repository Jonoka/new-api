import * as assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { buildCanvasLaunchUrl } from './lib'

describe('buildCanvasLaunchUrl', () => {
  test('builds a New API session launch URL without apiKey', () => {
    const url = buildCanvasLaunchUrl({
      canvasOrigin: 'https://canvas.jo2api.com',
      newApiOrigin: 'https://api.jo2api.com',
      group: 'vip group',
    })

    assert.equal(
      url,
      'https://canvas.jo2api.com/?mode=newapi&baseUrl=https%3A%2F%2Fapi.jo2api.com%2Fcanvas&group=vip+group'
    )
    assert.equal(url.includes('apiKey'), false)
  })
})
