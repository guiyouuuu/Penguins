import assert from 'node:assert/strict';

const base = process.env.PENGUIN_BASE_URL || 'http://127.0.0.1:8081';
const cases = [
  ['/healthz', {}, 200, 0],
  ['/readyz', {}, 200, 0],
  ['/api/leaderboard', {}, 200, 0],
  ['/api/profile', {}, 401, 20001],
  ['/api/missing', {}, 404, 10002],
  ['/api/login', {}, 405, 10003],
  ['/ws?token=invalid-test-token', {}, 401, 20001],
  ['/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{"unknown":true}' }, 400, 10001],
];

for (const [path, options, status, code] of cases) {
  const response = await fetch(base + path, options);
  const body = await response.json();
  assert.equal(response.status, status, path);
  assert.equal(body.code, code, path);
  assert.equal(body.requestId, response.headers.get('x-request-id'), path);
  assert.ok(body.requestId && body.message, path);
  assert.deepEqual(Object.keys(body).sort(), ['code', 'data', 'message', 'requestId']);
  if (status >= 400) assert.equal(body.data, null, path);
  if (status === 405) assert.ok(response.headers.get('allow'), path);
}
console.log(`HTTP contract: ${cases.length} cases passed`);
