import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
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
  for (const origin of ['', 'http://canvas.test', 'https://user@canvas.test', 'https://canvas.test/', 'https://canvas.test/path', 'https://canvas.test?q=1', 'https://canvas.test:', 'https://canvas.test:443', 'https://canvas.test:0443', 'https://canvas.test:09443', 'https://canvas.test:0', 'https://canvas.test:65536']) {
    assert.equal(buildCanvasSSOLaunchUrl(origin, 'default'), '');
  }
});

test('relative destinations reject external URLs, encoded controls and backslashes', () => {
  assert.equal(safeCanvasDestination('/project/1?tab=assets#image'), '/project/1?tab=assets#image');
  for (const next of ['https://evil.test', '//evil.test', '/%2fevil.test', '/%5cevil.test', '/%0d%0aX', '/%00', 'javascript:alert(1)']) {
    assert.equal(safeCanvasDestination(next), '/');
  }
});

test('dot-segment normalization cannot turn a canvas destination into an external launch', () => {
  for (const next of ['/a/..//evil.test', '/a/%2e%2e//evil.test', '/a/.%2E//evil.test', '/a/%2E.//evil.test', '/%2e//evil.test']) {
    assert.equal(safeCanvasDestination(next), '/');
    const launch = new URL(buildCanvasSSOLaunchUrl('https://canvas.test', 'default', next, true));
    assert.equal(launch.origin, 'https://canvas.test');
    assert.equal(launch.pathname, '/');
  }
});

test('hidden-launcher mode retains configured-origin resume wiring', () => {
  const status = { canvas_sso_origin: 'https://canvas.test', canvas_sso_launch_enabled: false };
  const launch = new URL(buildCanvasSSOLaunchUrl(status.canvas_sso_origin, 'default', '/projects/1', true));
  assert.equal(launch.origin, status.canvas_sso_origin);
  assert.equal(launch.searchParams.get('newapi_returned'), '1');
  const source = readFileSync(new URL('./pages/Canvas/index.jsx', import.meta.url), 'utf8');
  const resume = source.slice(source.indexOf('if (resumed.current'), source.indexOf('window.location.replace(target)'));
  assert.ok(resume.includes('buildCanvasSSOLaunchUrl'));
  assert.ok(resume.includes('canvas_sso_origin'));
  assert.equal(resume.includes('canvas_sso_launch_enabled'), false);
  assert.match(source, /canvas_sso_launch_enabled && \(/);
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
