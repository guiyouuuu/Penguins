// 最小化调试：单房间创建/加入/走子消息流追踪
const BASE = 'http://localhost:8080';
const stamp = Date.now().toString(36);

async function reg(u) {
  const r = await fetch(`${BASE}/api/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: u, password: '123456' }),
  });
  const d = (await r.json()).data;
  if (!d.token) throw new Error('注册失败: ' + JSON.stringify(d));
  return d.token;
}

function client(token, label) {
  const ws = new WebSocket(`ws://localhost:8080/ws?token=${encodeURIComponent(token)}`);
  ws.onmessage = (ev) => console.log(`[${label}] <-`, ev.data.slice(0, 120));
  ws.onclose = (ev) => console.log(`[${label}] closed`, ev.code, ev.reason);
  ws.onerror = () => console.log(`[${label}] error`);
  return new Promise((r) => (ws.onopen = () => r(ws)));
}

const t0 = await reg(`dbgA_${stamp}`);
const t1 = await reg(`dbgB_${stamp}`);
console.log('tokens ok');
const wsA = await client(t0, 'A');
const wsB = await client(t1, 'B');
console.log('sockets open');

let roomCode = '';
let startState = null;
wsA.addEventListener('message', (ev) => {
  const m = JSON.parse(ev.data);
  if (m.type === 'room') roomCode = m.room;
  if (m.type === 'lobby' && m.ready) wsA.send(JSON.stringify({ type: 'start_game' }));
  if (m.type === 'start') startState = m.state;
});
wsB.addEventListener('message', (ev) => {
  const m = JSON.parse(ev.data);
  if (m.type === 'lobby' && !m.ready) wsB.send(JSON.stringify({ type: 'ready', ready: true }));
});
wsA.send(JSON.stringify({ type: 'create_room' }));
await new Promise((r) => setTimeout(r, 300));
console.log('roomCode =', roomCode);

wsB.send(JSON.stringify({ type: 'join_room', room: roomCode }));
await new Promise((r) => setTimeout(r, 500));

// A 尝试放置（用编译后的引擎选合法格）
import { execSync } from 'node:child_process';
execSync(
  `npx esbuild src/core/game.ts --bundle --format=esm --outfile=/tmp/penguin-game.mjs --log-level=error`,
  { cwd: new URL('..', import.meta.url).pathname + 'frontend' },
);
const G = await import('/tmp/penguin-game.mjs');
await new Promise((r) => setTimeout(r, 300));
const tile = startState ? G.legalPlacements(startState)[0] : 0;
console.log('placing tile =', tile);
wsA.send(JSON.stringify({ type: 'place', tile }));
await new Promise((r) => setTimeout(r, 500));
wsA.close();
wsB.close();
process.exit(0);
