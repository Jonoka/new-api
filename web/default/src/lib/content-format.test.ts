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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  embeddedContentIframeSandbox,
  isHttpUrl,
  isLikelyHtml,
  trustedContentIframeSandbox,
} from './content-format'

describe('富文本格式识别', () => {
  test('只把 HTTP 和 HTTPS 地址识别为可嵌入外链', () => {
    assert.equal(isHttpUrl('https://example.com/path?q=1'), true)
    assert.equal(isHttpUrl('http://example.com'), true)
    assert.equal(isHttpUrl('javascript:alert(1)'), false)
    assert.equal(isHttpUrl('data:text/html,<script>alert(1)</script>'), false)
    assert.equal(isHttpUrl('file:///etc/passwd'), false)
    assert.equal(isHttpUrl('/relative/path'), false)
  })

  test('识别完整页面、样式和常规 HTML 标签', () => {
    assert.equal(isLikelyHtml('<!doctype html><html></html>'), true)
    assert.equal(isLikelyHtml('<style>body { color: red; }</style>'), true)
    assert.equal(isLikelyHtml('<script>alert(1)</script>'), true)
    assert.equal(isLikelyHtml('<section>内容</section>'), true)
    assert.equal(isLikelyHtml('## Markdown\n\n纯文本'), false)
  })
})

describe('iframe 沙箱策略', () => {
  test('整页可信外链不授予同源权限', () => {
    assert.match(trustedContentIframeSandbox, /allow-scripts/)
    assert.doesNotMatch(trustedContentIframeSandbox, /allow-same-origin/)
  })

  test('富文本内部 iframe 禁止脚本和同源权限', () => {
    assert.doesNotMatch(embeddedContentIframeSandbox, /allow-scripts/)
    assert.doesNotMatch(embeddedContentIframeSandbox, /allow-same-origin/)
  })
})
