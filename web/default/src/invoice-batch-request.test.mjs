import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import test from 'node:test'

const root = resolve(import.meta.dirname)
const apiSource = readFileSync(
  resolve(root, 'features/invoices/api.ts'),
  'utf8'
)
const componentSource = readFileSync(
  resolve(root, 'features/invoices/components/order-invoice-request.tsx'),
  'utf8'
)

test('default invoice center uses server-side order preview and application APIs', () => {
  assert.match(apiSource, /\/api\/user\/invoice\/orders/)
  assert.match(apiSource, /\/api\/user\/invoice\/preview/)
  assert.match(apiSource, /\/api\/user\/invoice\/request/)
  assert.match(componentSource, /previewOrderInvoice/)
  assert.match(componentSource, /preview\?\.order_amount/)
  assert.doesNotMatch(componentSource, /preview\?\.invoice_total_amount/)
})

test('orders already used for invoices cannot be selected again', () => {
  assert.match(
    componentSource,
    /return order\.invoice_eligible && !order\.invoiced/
  )
  assert.match(componentSource, /disabled=\{!isOrderSelectable\(order\)\}/)
  assert.match(componentSource, /MAX_SELECTED_ORDERS = 100/)
})
