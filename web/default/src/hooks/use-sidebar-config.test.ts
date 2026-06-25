import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { getSidebarCustomModuleKey } from '@/lib/custom-nav'
import type { NavGroup } from '@/components/layout/types'
import { filterSidebarNavGroups } from './use-sidebar-config'

describe('sidebar config filtering', () => {
  const customKey = getSidebarCustomModuleKey('工具 1')
  const groups: NavGroup[] = [
    {
      title: 'Chat',
      items: [
        {
          title: 'Custom Tool',
          url: '/tools',
          configUrls: [customKey],
        },
      ],
    },
  ]

  test('lets users hide administrator custom sidebar items', () => {
    const filtered = filterSidebarNavGroups(
      groups,
      { custom: { enabled: true, [customKey]: true } },
      { custom: { enabled: true, [customKey]: false } },
      undefined
    )

    assert.equal(filtered.length, 0)
  })

  test('does not let users show administrator-disabled custom sidebar items', () => {
    const filtered = filterSidebarNavGroups(
      groups,
      { custom: { enabled: true, [customKey]: false } },
      { custom: { enabled: true, [customKey]: true } },
      undefined
    )

    assert.equal(filtered.length, 0)
  })

  test('keeps custom sidebar item visible when both layers allow it', () => {
    const filtered = filterSidebarNavGroups(
      groups,
      { custom: { enabled: true, [customKey]: true } },
      { custom: { enabled: true, [customKey]: true } },
      undefined
    )

    assert.equal(filtered.length, 1)
    assert.equal(filtered[0].items[0].title, 'Custom Tool')
  })
})
