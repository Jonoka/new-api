import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const root = dirname(fileURLToPath(import.meta.url));
const invoicePageSource = readFileSync(
  resolve(root, 'pages/Invoice/index.jsx'),
  'utf8',
);

test('classic invoice cost column displays the stored invoice total amount', () => {
  assert.match(
    invoicePageSource,
    /title: t\('费用'\),\s*dataIndex: 'total_amount',\s*key: 'total_amount'/,
  );
  assert.doesNotMatch(
    invoicePageSource,
    /title: t\('费用'\),\s*dataIndex: 'fee_amount'/,
  );
  assert.match(invoicePageSource, /record\.base_amount/);
});
