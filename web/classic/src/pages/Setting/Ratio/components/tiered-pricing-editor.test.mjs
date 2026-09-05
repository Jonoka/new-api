import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { test } from 'node:test';

const requireDefault = createRequire(
  new URL('../../../../../../default/package.json', import.meta.url),
);
const { JSDOM } = requireDefault('jsdom');
const browser = new JSDOM('<!doctype html><html><body></body></html>', {
  url: 'https://pricing.example.test/',
});
// Semi's animation module initializes a 1px canvas even without visible animation.
browser.window.HTMLCanvasElement.prototype.getContext = function () {
  return {
    canvas: this,
    fillStyle: '',
    fillRect() {},
    clearRect() {},
    drawImage() {},
    measureText() {
      return { width: 0 };
    },
    getImageData() {
      return { data: new Uint8ClampedArray(4) };
    },
  };
};
for (const name of [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLInputElement',
  'Element',
  'Node',
  'ShadowRoot',
  'MutationObserver',
  'localStorage',
]) {
  const value = name === 'window' ? browser.window : browser.window[name];
  Object.defineProperty(globalThis, name, { configurable: true, value });
}
Object.defineProperty(globalThis, 'getComputedStyle', {
  configurable: true,
  value: browser.window.getComputedStyle.bind(browser.window),
});
Object.defineProperty(globalThis, 'IS_REACT_ACT_ENVIRONMENT', {
  configurable: true,
  value: true,
});
browser.window.matchMedia = () => ({
  matches: false,
  media: '',
  onchange: null,
  addListener() {},
  removeListener() {},
  addEventListener() {},
  removeEventListener() {},
  dispatchEvent() {
    return true;
  },
});
browser.window.ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
};
globalThis.ResizeObserver = browser.window.ResizeObserver;
globalThis.requestAnimationFrame = (callback) =>
  setTimeout(() => callback(Date.now()), 0);
globalThis.cancelAnimationFrame = clearTimeout;

const React = await import('react');
const { act } = await import('react-dom/test-utils');
const { createRoot } = await import('react-dom/client');
const { default: ModelPricingEditor } =
  await import('./ModelPricingEditor.jsx');

function buttonWithText(container, text) {
  const button = [...container.querySelectorAll('button')].find(
    (candidate) => candidate.textContent.trim() === text,
  );
  assert.ok(button, `missing button: ${text}`);
  return button;
}

test('model selection preserves each tariff and invalid drafts block switching and apply', async () => {
  const base = 'tier("base", p * 2.5 + c * 15)';
  const legacy = `(${base}) * (hour("UTC") >= 9 || hour("UTC") < 12 ? 2 : 1)`;
  const valid = `(${base}) * (hour("UTC") >= 9 && hour("UTC") < 12 ? 3 : 1)`;
  const options = {
    'billing_setting.billing_mode': JSON.stringify({
      'A-model': 'tiered_expr',
      'B-model': 'tiered_expr',
    }),
    'billing_setting.billing_expr': JSON.stringify({
      'A-model': legacy,
      'B-model': valid,
    }),
  };
  const container = document.createElement('div');
  document.body.append(container);
  const root = createRoot(container);
  try {
    await act(async () => {
      root.render(
        React.createElement(ModelPricingEditor, { options, refresh() {} }),
      );
    });
    assert.ok(container.textContent.includes(legacy));
    for (const [name, expression] of [
      ['B-model', valid],
      ['A-model', legacy],
      ['B-model', valid],
    ]) {
      await act(async () => {
        buttonWithText(container, name).click();
      });
      assert.ok(
        container.textContent.includes(expression),
        `${name} tariff preserved`,
      );
    }
    await act(async () => {
      buttonWithText(container, '添加条件组').click();
    });
    assert.equal(buttonWithText(container, '应用更改').disabled, true);
    const invalidGroupsBefore = container.querySelectorAll('input').length;
    const perTokenRadio = [
      ...container.querySelectorAll('input[type="radio"]'),
    ].find((input) => input.value === 'per-token');
    assert.ok(perTokenRadio);
    assert.equal(perTokenRadio.disabled, true);
    await act(async () => {
      perTokenRadio.click();
      buttonWithText(container, 'A-model').click();
      buttonWithText(container, '应用更改').click();
    });
    assert.equal(buttonWithText(container, '应用更改').disabled, true);
    assert.equal(
      container.querySelectorAll('input').length,
      invalidGroupsBefore,
    );
    assert.ok(container.textContent.includes(valid));
  } finally {
    await act(async () => {
      root.unmount();
    });
    container.remove();
  }
});
