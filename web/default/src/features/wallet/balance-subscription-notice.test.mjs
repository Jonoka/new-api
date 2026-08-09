import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const root = dirname(fileURLToPath(import.meta.url))

test('Default 仅在余额购买订阅关闭时显示充值用途提示', () => {
  const source = readFileSync(
    resolve(root, 'components/recharge-form-card.tsx'),
    'utf8'
  )
  const noticeStart = source.indexOf(
    'topupInfo?.enable_balance_subscription === false'
  )
  const noticeEnd = source.indexOf('{/* Online Topup Section */}', noticeStart)
  const noticeSource = source.slice(noticeStart, noticeEnd)

  assert.notEqual(noticeStart, -1)
  assert.match(noticeSource, /<Alert>/)
  assert.match(
    noticeSource,
    /Top-up balance can only be used for API calls and cannot be used to purchase subscription plans\./
  )
  assert.match(
    noticeSource,
    /Please purchase subscription plans separately on the "Subscription Plans" page\./
  )
})
