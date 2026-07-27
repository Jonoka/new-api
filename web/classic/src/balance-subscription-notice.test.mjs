import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));

test('Classic 仅在余额购买订阅关闭时显示充值用途提示', () => {
  const source = readFileSync(
    resolve(root, 'components/topup/RechargeCard.jsx'),
    'utf8',
  );
  const noticeStart = source.indexOf(
    'topupInfo?.enable_balance_subscription === false',
  );
  const noticeEnd = source.indexOf('{/* 统计数据 */}', noticeStart);
  const noticeSource = source.slice(noticeStart, noticeEnd);

  assert.notEqual(noticeStart, -1);
  assert.match(noticeSource, /<Banner/);
  assert.match(
    noticeSource,
    /充值余额仅用于 API 调用消耗，不可用于购买订阅套餐。/,
  );
  assert.match(noticeSource, /订阅套餐请在「订阅套餐」页面单独购买。/);
});
