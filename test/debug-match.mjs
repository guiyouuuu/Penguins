// 快速匹配流程验证：两个登录用户同时排队 → 自动配对开局
const BASE = 'http://localhost:8080';
const stamp = Date.now().toString(36);

async function reg(u) {
  const r = await fetch(`${BASE}/api/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: u, password: '123456' }),
  });
  return (await r.json()).data.token;
}

function client(token, label) {
  const ws = new WebSocket(`ws://localhost:8080/ws?token=${encodeURIComponent(token)}`);
  const log = [];
  ws.onmessage = (ev) => {
    const m = JSON.parse(ev.data);
    log.push(m.type);
    console.log(`[${label}] <-`, m.type, m.room ?? '', m.you ?? '', m.msg ?? '');
  };
  return new Promise((r) => (ws.onopen = () => r(ws)));
}

const t0 = await reg(`匹配A_${stamp}`);
const t1 = await reg(`匹配B_${stamp}`);
const wsA = await client(t0, 'A');
const wsB = await client(t1, 'B');

wsA.send(JSON.stringify({ type: 'quick_match' }));
setTimeout(() => wsB.send(JSON.stringify({ type: 'quick_match' })), 100);

await new Promise((r) => setTimeout(r, 2000));
const aStart = wsA._seenStart;
// 收集消息类型判断
const seenA = [], seenB = [];
wsA.onmessage = (ev) => seenA.push(JSON.parse(ev.data).type);
wsB.onmessage = (ev) => seenB.push(JSON.parse(ev.data).type);
await new Promise((r) => setTimeout(r, 300));
wsA.close();
wsB.close();
process.exit(0);
