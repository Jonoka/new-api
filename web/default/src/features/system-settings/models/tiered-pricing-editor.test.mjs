import { JSDOM } from 'jsdom'
import assert from 'node:assert/strict'
import { test } from 'node:test'

const browser = new JSDOM('<!doctype html><html><body></body></html>', {
  url: 'https://pricing.example.test/',
})
Object.defineProperties(globalThis, {
  window: { configurable: true, value: browser.window },
  document: { configurable: true, value: browser.window.document },
  navigator: { configurable: true, value: browser.window.navigator },
  HTMLElement: { configurable: true, value: browser.window.HTMLElement },
  Element: { configurable: true, value: browser.window.Element },
  Node: { configurable: true, value: browser.window.Node },
  localStorage: { configurable: true, value: browser.window.localStorage },
  getComputedStyle: {
    configurable: true,
    value: browser.window.getComputedStyle.bind(browser.window),
  },
  IS_REACT_ACT_ENVIRONMENT: { configurable: true, value: true },
})
browser.window.matchMedia = () => ({
  matches: false,
  media: '',
  onchange: null,
  addListener() {},
  removeListener() {},
  addEventListener() {},
  removeEventListener() {},
  dispatchEvent() {
    return true
  },
})

const React = await import('react')
const { createRoot } = await import('react-dom/client')
const { TieredPricingEditor } = await import('./tiered-pricing-editor.tsx')
const { combineBillingExpr } = await import('../../pricing/lib/billing-expr.ts')

test('mounting a legacy OR rule preserves its effective tariff exactly once', async () => {
  const base = 'tier("base", p * 2.5 + c * 15)'
  const legacy = '(hour("UTC") >= 9 || hour("UTC") < 12 ? 2 : 1)'
  let latest = { base, rule: legacy }
  function Harness() {
    const [billingExpr, setBillingExpr] = React.useState(base)
    const [requestRuleExpr, setRequestRuleExpr] = React.useState(legacy)
    latest = { base: billingExpr, rule: requestRuleExpr }
    return React.createElement(TieredPricingEditor, {
      modelName: 'test',
      billingExpr,
      requestRuleExpr,
      onBillingExprChange: setBillingExpr,
      onRequestRuleExprChange: setRequestRuleExpr,
    })
  }
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  try {
    await React.act(async () => {
      root.render(React.createElement(Harness))
    })
    assert.equal(
      combineBillingExpr(latest.base, latest.rule),
      combineBillingExpr(base, legacy)
    )
    assert.ok(
      container.querySelector('textarea'),
      'unsupported legacy rule stays in raw editor'
    )
  } finally {
    await React.act(async () => {
      root.unmount()
    })
    container.remove()
  }
})
