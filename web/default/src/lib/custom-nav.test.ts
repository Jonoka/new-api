import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  getCustomNavIcon,
  getSidebarCustomModuleKey,
  parseCustomNavItems,
} from './custom-nav'

describe('custom navigation helpers', () => {
  test('normalizes enabled custom links and sorts them by order', () => {
    const items = parseCustomNavItems([
      {
        id: 'canvas',
        title: '无限画布',
        url: '/canvas',
        icon: 'Brush',
        enabled: true,
        order: 20,
        section: 'chat',
      },
      {
        id: 'bad',
        title: 'Bad',
        url: 'javascript:alert(1)',
        enabled: true,
        order: 10,
      },
      {
        id: 'docs',
        title: 'Docs',
        url: 'https://docs.canvas.best',
        icon: 'BookOpen',
        enabled: true,
        order: 5,
        openInNewTab: true,
      },
    ])

    assert.deepEqual(
      items.map((item) => item.id),
      ['docs', 'canvas']
    )
    assert.equal(items[0].external, true)
    assert.equal(items[1].section, 'chat')
    assert.equal(items[1].icon, 'Brush')
  })

  test('maps custom sidebar items to stable user module keys', () => {
    assert.equal(getSidebarCustomModuleKey('canvas'), 'custom:canvas')
    assert.equal(getSidebarCustomModuleKey('工具 1'), 'custom:工具-1')
  })

  test('keeps disabled custom links only when requested for admin editing', () => {
    const rawItems = [
      {
        id: 'enabled',
        title: 'Enabled',
        url: '/enabled',
        enabled: true,
      },
      {
        id: 'disabled',
        title: 'Disabled',
        url: '/disabled',
        enabled: false,
      },
    ]

    assert.deepEqual(
      parseCustomNavItems(rawItems).map((item) => item.id),
      ['enabled']
    )
    assert.deepEqual(
      parseCustomNavItems(rawItems, { includeDisabled: true }).map(
        (item) => item.id
      ),
      ['enabled', 'disabled']
    )
  })

  test('resolves only whitelisted lucide icons', () => {
    assert.ok(getCustomNavIcon('Brush'))
    assert.equal(getCustomNavIcon('NotReal'), undefined)
  })
})
