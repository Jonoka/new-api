/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { JSDOM } from 'jsdom'
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

const browser = new JSDOM(
  '<!doctype html><html><head></head><body></body></html>',
  {
    url: 'https://app.example.com/',
  }
)

Object.defineProperties(globalThis, {
  document: { configurable: true, value: browser.window.document },
  Element: { configurable: true, value: browser.window.Element },
  HTMLElement: { configurable: true, value: browser.window.HTMLElement },
  Node: { configurable: true, value: browser.window.Node },
  window: { configurable: true, value: browser.window },
})

const { sanitizeHtmlContent } = await import('./html-content.tsx')
const { embeddedContentIframeSandbox } =
  await import('../lib/content-format.ts')

describe('HTML 内容清理', () => {
  test('移除脚本、事件属性和 javascript URL', () => {
    const html = sanitizeHtmlContent(
      `
        <p>安全内容</p>
        <script>globalThis.__xss = true</script>
        <img src="/safe.png" onerror="globalThis.__xss = true">
        <a href="javascript:alert(1)">危险链接</a>
        <iframe src="javascript:alert(2)" srcdoc="<script>alert(3)</script>"></iframe>
      `,
      'isolated'
    )
    const fragment = JSDOM.fragment(html)

    assert.equal(fragment.querySelector('script'), null)
    assert.equal(fragment.querySelector('img')?.hasAttribute('onerror'), false)
    assert.equal(fragment.querySelector('a')?.hasAttribute('href'), false)
    assert.equal(fragment.querySelector('iframe')?.hasAttribute('src'), false)
    assert.equal(
      fragment.querySelector('iframe')?.hasAttribute('srcdoc'),
      false
    )
    assert.doesNotMatch(html, /__xss|javascript:/i)
  })

  test('保留可信 iframe，并覆盖为低权限沙箱', () => {
    const html = sanitizeHtmlContent(
      `
        <iframe
          src="https://video.example.com/embed/1"
          sandbox="allow-scripts allow-same-origin"
        ></iframe>
      `,
      'isolated'
    )
    const frame = JSDOM.fragment(html).querySelector('iframe')

    assert.ok(frame)
    assert.equal(frame.getAttribute('sandbox'), embeddedContentIframeSandbox)
    assert.equal(frame.getAttribute('referrerpolicy'), 'no-referrer')
    assert.equal(frame.getAttribute('loading'), 'lazy')
    assert.doesNotMatch(frame.getAttribute('sandbox'), /allow-scripts/)
    assert.doesNotMatch(frame.getAttribute('sandbox'), /allow-same-origin/)
  })

  test('为新窗口链接补充 opener 防护', () => {
    const html = sanitizeHtmlContent(
      '<a href="https://example.com" target="_blank" rel="author">链接</a>',
      'isolated'
    )
    const link = JSDOM.fragment(html).querySelector('a')
    const rel = new Set(link?.getAttribute('rel')?.split(/\s+/))

    assert.ok(rel.has('author'))
    assert.ok(rel.has('noopener'))
    assert.ok(rel.has('noreferrer'))
  })
})
