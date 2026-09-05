import assert from 'node:assert/strict';
import test from 'node:test';
import { buildCanvasSSOLaunchUrl, safeCanvasDestination, rememberCanvasReturn, consumeCanvasReturn } from './helpers/canvas-sso.js';

test('Canvas 2 launch uses a strict origin, selected group and bounded return marker', () => {
  const link = new URL(buildCanvasSSOLaunchUrl('https://canvas.test', 'vip group', '/projects/1?apiKey=discard&baseUrl=discard', true));
  assert.equal(link.origin, 'https://canvas.test');
  assert.equal(link.pathname, '/projects/1');
  assert.equal(link.searchParams.get('group'), 'vip group');
  assert.equal(link.searchParams.get('newapi_launch'), '1');
  assert.equal(link.searchParams.get('newapi_returned'), '1');
  assert.equal(link.searchParams.has('apiKey'), false);
  assert.equal(link.searchParams.has('baseUrl'), false);
  const explicit = new URL(buildCanvasSSOLaunchUrl('https://canvas.test', 'default', '/?newapi_returned=1'));
  assert.equal(explicit.searchParams.has('newapi_returned'), false);
  for (const origin of ['', 'http://canvas.test', 'https://user@canvas.test', 'https://canvas.test/', 'https://canvas.test/path', 'https://canvas.test?q=1']) {
    assert.equal(buildCanvasSSOLaunchUrl(origin, 'default'), '');
  }
});

test('relative destinations reject external URLs, encoded controls and backslashes', () => {
  assert.equal(safeCanvasDestination('/project/1?tab=assets#image'), '/project/1?tab=assets#image');
  for (const next of ['https://evil.test', '//evil.test', '/%2fevil.test', '/%5cevil.test', '/%0d%0aX', '/%00', 'javascript:alert(1)']) {
    assert.equal(safeCanvasDestination(next), '/');
  }
});

test('login return survives authentication once, expires and never stores arbitrary navigation', () => {
  const storage = new Map();
  Object.defineProperty(globalThis, 'window', { value: { location: { origin: 'https://api.test' } }, configurable: true });
  Object.defineProperty(globalThis, 'sessionStorage', { value: {
    getItem: (key) => storage.get(key) ?? null,
    setItem: (key, value) => storage.set(key, value),
    removeItem: (key) => storage.delete(key),
  }, configurable: true });
  rememberCanvasReturn('/console/canvas?canvas_resume=1&group=vip&canvas_next=%2Fprojects%2F1&role=100');
  const result = new URL(consumeCanvasReturn(), 'https://api.test');
  assert.equal(result.pathname, '/console/canvas');
  assert.equal(result.searchParams.get('canvas_resume'), '1');
  assert.equal(result.searchParams.get('canvas_next'), '/projects/1');
  assert.equal(result.searchParams.get('group'), 'vip');
  assert.equal(result.searchParams.has('role'), false);
  assert.equal(consumeCanvasReturn(), '');
  rememberCanvasReturn('/console/canvas?canvas_resume=1');
  for (const [key, value] of storage) storage.set(key, JSON.stringify({ ...JSON.parse(value), expires: 0 }));
  assert.equal(consumeCanvasReturn(), '');
  for (const target of ['/dashboard?canvas_resume=1', '//evil.test/canvas?canvas_resume=1', '/console/canvas', 'https://evil.test']) {
    rememberCanvasReturn(target);
    assert.equal(consumeCanvasReturn(), '');
  }
});
