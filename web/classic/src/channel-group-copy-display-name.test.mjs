import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import { getGroupDisplayName } from './helpers/groupDetails.js';

const root = dirname(fileURLToPath(import.meta.url));

test('分组复制内容优先使用当前显示名称并回退稳定标识', () => {
  assert.equal(
    getGroupDisplayName('legacy-code', {
      'legacy-code': '当前显示名称',
    }),
    '当前显示名称',
  );
  assert.equal(getGroupDisplayName('legacy-code', {}), 'legacy-code');
});

test('Classic 分组标签的复制、提示和失败内容共用显示名称', () => {
  const renderSource = readFileSync(
    resolve(root, 'helpers/render.jsx'),
    'utf8',
  );
  const renderGroupSource = renderSource.slice(
    renderSource.indexOf('export function renderGroup'),
    renderSource.indexOf('export function renderRatio'),
  );

  assert.match(renderGroupSource, /getGroupDisplayName\(group, labels\)/);
  assert.match(renderGroupSource, /copy\(displayName\)/);
  assert.match(renderGroupSource, /t\('已复制：'\) \+ displayName/);
  assert.match(renderGroupSource, /content:\s*displayName/);
  assert.doesNotMatch(renderGroupSource, /copy\(group\)/);
});
