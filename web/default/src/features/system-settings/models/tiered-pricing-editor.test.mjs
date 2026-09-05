import { JSDOM } from 'jsdom'
import assert from 'node:assert/strict'
import { test } from 'node:test'

const browser = new JSDOM('<!doctype html><html><body></body></html>', {
  url: 'https://pricing.example.test/',
  pretendToBeVisual: true,
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
  requestAnimationFrame: {
    configurable: true,
    value: browser.window.requestAnimationFrame.bind(browser.window),
  },
  cancelAnimationFrame: {
    configurable: true,
    value: browser.window.cancelAnimationFrame.bind(browser.window),
  },
  ShadowRoot: { configurable: true, value: browser.window.ShadowRoot },
  MutationObserver: {
    configurable: true,
    value: browser.window.MutationObserver,
  },
  ResizeObserver: {
    configurable: true,
    value: class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  },
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
const { ModelPricingEditorPanel } = await import('./model-pricing-sheet.tsx')

function buttonWithText(container, text) {
  const button = [...container.querySelectorAll('button')].find(
    (candidate) => candidate.textContent.trim() === text
  )
  assert.ok(button, `missing button: ${text}`)
  return button
}

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

test('invalid rule survives a base-price edit and blocks outer mode and submit', async () => {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  const saved = []
  try {
    await React.act(async () => {
      root.render(
        React.createElement(ModelPricingEditorPanel, {
          editData: {
            name: 'draft-model',
            billingMode: 'tiered_expr',
            billingExpr: 'tier("base", p * 2.5 + c * 15)',
            requestRuleExpr: '',
          },
          onSave: (data) => saved.push(data),
          onCancel() {},
        })
      )
    })
    await React.act(async () => {
      buttonWithText(container, 'Add rule group').click()
    })
    assert.equal(
      container.querySelector('button[type="submit"]').disabled,
      true
    )
    const priceInput = container.querySelector('input[type="number"]')
    assert.ok(priceInput)
    await React.act(async () => {
      Object.getOwnPropertyDescriptor(
        browser.window.HTMLInputElement.prototype,
        'value'
      ).set.call(priceInput, '3.5')
      priceInput.dispatchEvent(
        new browser.window.Event('input', { bubbles: true })
      )
      priceInput.dispatchEvent(
        new browser.window.Event('change', { bubbles: true })
      )
    })
    assert.ok(
      container.querySelector('[aria-label="Remove rule group"]'),
      'invalid group must remain'
    )
    assert.equal(
      container.querySelector('button[type="submit"]').disabled,
      true
    )
    const tokenTab = buttonWithText(container, 'Per-token')
    assert.ok(
      tokenTab.disabled || tokenTab.getAttribute('aria-disabled') === 'true'
    )
    await React.act(async () => {
      tokenTab.click()
      container.querySelector('form').dispatchEvent(
        new browser.window.Event('submit', {
          bubbles: true,
          cancelable: true,
        })
      )
    })
    assert.equal(saved.length, 0)
    assert.ok(container.querySelector('[aria-label="Remove rule group"]'))
    await React.act(async () => {
      container.querySelector('[aria-label="Remove rule group"]').click()
    })
    assert.equal(
      container.querySelector('button[type="submit"]').disabled,
      false
    )
  } finally {
    await React.act(async () => {
      root.unmount()
    })
    container.remove()
  }
})
