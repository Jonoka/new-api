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
import { renderToStaticMarkup } from 'react-dom/server'
import { Markdown } from './markdown'

describe('Markdown 安全渲染', () => {
  test('移除脚本、事件属性、危险 URL 和 iframe', () => {
    const content = `
安全内容

<script>globalThis.__xss = true</script>
<img src="/safe.png" onerror="globalThis.__xss = true">
<a href="javascript:alert(1)">危险链接</a>
[Markdown 危险链接](javascript:alert%282%29)
<iframe src="https://example.com/embed"></iframe>
`

    const html = renderToStaticMarkup(<Markdown>{content}</Markdown>)

    assert.match(html, /安全内容/)
    assert.doesNotMatch(html, /<script/i)
    assert.doesNotMatch(html, /onerror/i)
    assert.doesNotMatch(html, /\b(?:href|src)="javascript:/i)
    assert.doesNotMatch(html, /<iframe/i)
    assert.doesNotMatch(html, /__xss/)
  })

  test('保留安全链接属性并按需渲染软换行', () => {
    const html = renderToStaticMarkup(
      <Markdown breaks>
        {'第一行\n第二行\n\n[文档](https://example.com)'}
      </Markdown>
    )

    assert.match(html, /<br\s*\/?/i)
    assert.match(html, /href="https:\/\/example\.com"/)
    assert.match(html, /target="_blank"/)
    assert.match(html, /rel="noopener noreferrer"/)
  })
})
