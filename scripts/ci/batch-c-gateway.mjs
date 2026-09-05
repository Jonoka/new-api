import assert from 'node:assert/strict';
import http from 'node:http';

assert.equal(process.env.GITHUB_ACTIONS, 'true');
const bodies = [];
const failures = [];
const output = '{"output":"","encrypted_output":"ci-fixture","opaque":9007199254740993}';
const gateway = http.createServer(async (req, res) => {
  try {
    const chunks = [];
    for await (const chunk of req) chunks.push(chunk);
    const body = Buffer.concat(chunks).toString('utf8');
    bodies.push(body);
    assert.equal(req.url, '/v1/alpha/search');
    assert.equal(req.headers.authorization, 'Bearer ci-gateway-key');
    assert.equal(req.headers['chatgpt-account-id'], undefined);
    assert.equal(Number(req.headers['content-length']), Buffer.byteLength(body));
    assert.ok(body.includes('9007199254740993'));
    const parsed = JSON.parse(body);
    assert.equal(parsed.model, 'c-alpha-mapped');
    assert.equal(parsed.zero, 0);
    assert.equal(parsed.flag, false);
    assert.equal(parsed.null, null);
    assert.equal(parsed.instructions, undefined);
    assert.equal(parsed.store, undefined);
    res.writeHead(parsed.fixture === 'fail' ? 400 : 200, { 'Content-Type': 'application/json' });
    res.end(parsed.fixture === 'fail' ? '{"error":{"message":"CI rejection","type":"invalid_request_error"}}' : output);
  } catch (error) {
    failures.push(String(error));
    res.writeHead(500, { 'Content-Type': 'application/json' });
    res.end('{"error":{"message":"fixture assertion failed"}}');
  }
});
await new Promise((resolve, reject) => {
  gateway.once('error', reject);
  gateway.listen(38080, '127.0.0.1', resolve);
});
try {
  const body = (extra = '') => '{"model":"c-alpha-public","input":"fixture","opaque":9007199254740993,"zero":0,"flag":false,"null":null' + extra + '}';
  const request = (raw, authenticated = true) => fetch('http://127.0.0.1:3000/v1/alpha/search', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...(authenticated ? { Authorization: 'Bearer sk-calphasynthetictoken' } : {}) },
    body: raw,
    signal: AbortSignal.timeout(20000),
  });
  let response = await request(body(), false);
  assert.equal(response.status, 401);
  response = await request(body(',"stream":true'));
  assert.equal(response.status, 400, await response.text());
  response = await request('{"model":"not-authorized"}');
  assert.ok(response.status >= 400);
  assert.equal(bodies.length, 0, 'invalid requests must never reach the gateway');
  response = await request(body());
  const success = await response.text();
  assert.equal(response.status, 200, success);
  assert.equal(success, output);
  response = await request(body(',"fixture":"fail"'));
  assert.equal(response.status, 400, await response.text());
  assert.equal(bodies.length, 2, 'one success and one failed attempt, with no application replay');
  assert.deepEqual(failures, []);
  console.log('Candidate Alpha auth, validation, mapping, gateway bytes and responses passed.');
} finally {
  await new Promise((resolve) => gateway.close(resolve));
}
