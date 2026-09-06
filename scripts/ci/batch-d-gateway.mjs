import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHmac } from 'node:crypto';
import http from 'node:http';

assert.equal(process.env.GITHUB_ACTIONS, 'true');
const postgres = process.env.TEST_POSTGRES_CONTAINER;
assert.ok(postgres);
const calls = new Map();
const pending = [];
const failures = [];
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const query = (sql) => execFileSync('docker', ['exec', postgres, 'psql', '-XAt', '-v', 'ON_ERROR_STOP=1', '-U', 'postgres', '-d', 'newapi_candidate_smoke', '-c', sql], { encoding: 'utf8' }).trim();
const state = (name) => JSON.parse(query(`SELECT json_build_object(
  'wallet',(SELECT quota FROM users WHERE username='d-wallet-${name}'),
  'used',(SELECT used_quota FROM users WHERE username='d-wallet-${name}'),
  'requests',(SELECT request_count FROM users WHERE username='d-wallet-${name}'),
  'remain',(SELECT remain_quota FROM tokens WHERE name='d-wallet-${name}'),
  'tokenUsed',(SELECT used_quota FROM tokens WHERE name='d-wallet-${name}'),
  'channelUsed',(SELECT used_quota FROM channels WHERE name='d-wallet-${name}'))`));
const waitFor = async (check, description, timeout = 90000) => {
  const until = Date.now() + timeout;
  do { if (await check()) return; await sleep(1000); } while (Date.now() < until);
  throw new Error(`Timed out: ${description}`);
};
const gateway = http.createServer(async (req, res) => {
  try {
    const chunks = [];
    for await (const chunk of req) chunks.push(chunk);
    const body = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    assert.equal(req.url, '/v1/chat/completions');
    assert.equal(req.headers.authorization, 'Bearer d-ci-gateway');
    const name = body.model.replace(/^d-wallet-/, '');
    assert.ok(['wallet', 'token', 'trust', 'stale', 'refund', 'restart'].includes(name));
    calls.set(name, (calls.get(name) ?? 0) + 1);
    if (name === 'restart') { pending.push(res); return; }
    res.setHeader('Content-Type', 'application/json');
    if (name === 'refund') {
      res.writeHead(400);
      res.end('{"error":{"message":"CI rejection","type":"invalid_request_error"}}');
      return;
    }
    res.end(JSON.stringify({ id: 'd-ci-response', object: 'chat.completion', model: body.model,
      choices: [{ index: 0, message: { role: 'assistant', content: 'ok' }, finish_reason: 'stop' }],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 } }));
  } catch (error) {
    failures.push(String(error)); res.writeHead(500); res.end('fixture failure');
  }
});
await new Promise((resolve, reject) => { gateway.once('error', reject); gateway.listen(38080, '127.0.0.2', resolve); });
const request = async (name, index, timeout = 30000) => {
  const response = await fetch(`http://127.0.0.1:${38001 + index % 2}/v1/chat/completions`, {
    method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer sk-dwallet${name}` },
    body: JSON.stringify({ model: `d-wallet-${name}`, messages: [{ role: 'user', content: 'fixture' }] }),
    signal: AbortSignal.timeout(timeout),
  });
  const body = await response.text();
  return { status: response.status, body };
};
const ready = async (port) => {
  try { return (await fetch(`http://127.0.0.1:${port}/api/status`, { signal: AbortSignal.timeout(2000) })).ok; }
  catch { return false; }
};
try {
  await waitFor(() => query("SELECT count(*) FROM balance_cache_repairs WHERE repaired_at=0") === '0', 'durable Redis repair in fresh candidate processes');
  assert.deepEqual(state('cache'), { wallet: 11000, remain: 11000, tokenUsed: 1000, used: 0, channelUsed: 0, requests: 0 });
  const cacheUserID = query("SELECT id FROM users WHERE username='d-wallet-cache'");
  const cacheTokenHash = createHmac('sha256', 'd-ci-crypto-only').update('dwalletcache').digest('hex');
  const cacheExists = execFileSync('docker', ['exec', process.env.TEST_REDIS_CONTAINER, 'redis-cli', '-n', '13', 'EXISTS', `user:${cacheUserID}`, `token:${cacheTokenHash}`], { encoding: 'utf8' }).trim();
  assert.equal(cacheExists, '0', 'fresh processes must invalidate both stale projections without replaying money');
  console.log('PostgreSQL and Redis outage/restart convergence passed with unchanged committed balances.');
  for (const [name, count, expected] of [['wallet', 16, 3], ['token', 16, 3], ['stale', 4, 0], ['trust', 8, 8], ['refund', 4, 0]]) {
    const results = await Promise.all(Array.from({ length: count }, (_, index) => request(name, index)));
    assert.equal(results.filter((r) => r.status === 200).length, expected, JSON.stringify({ name, results }));
    assert.ok(results.every((r) => r.status === 200 || r.status >= 400), name);
    assert.equal(calls.get(name) ?? 0, name === 'refund' ? count : expected, `upstream count for ${name}`);
    const expectedState = {
      wallet: { wallet: 0, remain: 54000, tokenUsed: 6000, used: 6000, channelUsed: 6000, requests: 3 },
      token: { wallet: 54000, remain: 0, tokenUsed: 6000, used: 6000, channelUsed: 6000, requests: 3 },
      stale: { wallet: 100, remain: 100, tokenUsed: 0, used: 0, channelUsed: 0, requests: 0 },
      trust: { wallet: 5984000, remain: 5984000, tokenUsed: 16000, used: 16000, channelUsed: 16000, requests: 8 },
      refund: { wallet: 12000, remain: 12000, tokenUsed: 0, used: 0, channelUsed: 0, requests: 0 },
    }[name];
    await waitFor(() => {
      const current = state(name);
      return Object.entries(expectedState).every(([key, value]) => current[key] === value);
    }, `financial state ${name}`, 30000);
    assert.deepEqual(state(name), expectedState);
    console.log(`Two-process ${name}: ${expected} successes, ${calls.get(name) ?? 0} upstream calls; balances verified.`);
  }
  const abandoned = [request('restart', 0, 90000), request('restart', 1, 90000)].map((p) => p.catch(() => ({ status: 0 })));
  await waitFor(() => calls.get('restart') === 2, 'two reserved requests reached upstream', 30000);
  assert.equal(state('restart').wallet, 8000);
  assert.equal(state('restart').remain, 8000);
  execFileSync('docker', ['kill', 'newapi-batch-d-a', 'newapi-batch-d-b']);
  await Promise.all(abandoned);
  for (const response of pending) response.destroy();
  execFileSync('docker', ['start', 'newapi-batch-d-a']);
  await waitFor(() => ready(38001), 'candidate restart');
  await waitFor(() => state('restart').wallet === 12000 && state('restart').remain === 12000, 'expired ordinary reservation recovery');
  assert.deepEqual(state('restart'), { wallet: 12000, remain: 12000, tokenUsed: 0, used: 0, channelUsed: 0, requests: 0 });
  assert.equal(calls.get('restart'), 2, 'recovery must never contact upstream');
  await waitFor(() => query("SELECT count(*) FROM task_submissions WHERE model_name LIKE 'd-wallet-%' AND (state IN ('active','settlement_pending') OR cache_pending)") === '0', 'D journals and cache markers drain');
  assert.deepEqual(failures, []);
  console.log('Ordinary reservations recovered after two process exits without duplicate refund or provider retry.');
} finally {
  for (const response of pending) response.destroy();
  await new Promise((resolve) => gateway.close(resolve));
}
