import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const root = dirname(fileURLToPath(import.meta.url));
const modalSource = readFileSync(
  resolve(root, 'components/invoice/InvoiceBatchRequestModal.jsx'),
  'utf8',
);
const pageSource = readFileSync(
  resolve(root, 'pages/Invoice/index.jsx'),
  'utf8',
);

test('classic invoice center exposes the batch invoice workflow to users only', () => {
  assert.match(pageSource, /!adminView && \(/);
  assert.match(pageSource, /<InvoiceBatchRequestModal/);
  assert.match(pageSource, /t\('申请开票'\)/);
});

test('classic batch invoice uses server preview and request endpoints', () => {
  assert.match(modalSource, /API\.get\('\/api\/user\/invoice\/orders'\)/);
  assert.match(modalSource, /API\.post\('\/api\/user\/invoice\/preview'/);
  assert.match(modalSource, /API\.post\('\/api\/user\/invoice\/request'/);
});

test('classic batch invoice prevents selecting previously invoiced orders', () => {
  assert.match(
    modalSource,
    /Boolean\(order\?\.invoice_eligible\) && !order\?\.invoiced/,
  );
  assert.match(modalSource, /disabled: !isOrderSelectable\(record\)/);
  assert.match(modalSource, /filter\(isOrderSelectable\)/);
  assert.match(modalSource, /showRequiredToggle=\{false\}/);
});

test('classic batch invoice clears stale previews as soon as selection changes', () => {
  assert.match(
    modalSource,
    /selectedOrderRefs\.length === 0[\s\S]*setPreview\(null\);[\s\S]*setPreviewLoading\(false\);/,
  );
  assert.match(
    modalSource,
    /let cancelled = false;\s+setPreview\(null\);\s+setPreviewError\(''\);\s+setPreviewLoading\(true\);\s+const timer/,
  );
});
