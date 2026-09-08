// 并发压测：多用户注册 → 多房间同时开局对决 → 战绩落库验证
// 用法: node test/load.mjs [房间数]（默认 6）
import { execSync } from 'node:child_process';

const root = new URL('..', import.meta.url).pathname;
const BASE = process.env.PENGUIN_BASE_URL || 'http://localhost:8080';
const WS_BASE = BASE.replace(/^http/, 'ws') + '/ws';

execSync(
  `npx esbuild src/core/game.ts --bundle --format=esm --outfile=/tmp/penguin-game.mjs --log-level=error`,
  { cwd: root + 'frontend' },
);
const G = await import('/tmp/penguin-game.mjs');

const ROOMS = parseInt(process.argv[2] ?? '6', 10);
let pass = 0;
let fail = 0;
const ok = (cond, name) => {
  if (cond) {
    pass++;
    console.log(`  ✓ ${name}`);
  } else {
    fail++;
    console.error(`  ✗ ${name}`);
  }
};

// ---------- HTTP 基础 ----------
async function http(path, init) {
  const res = await fetch(BASE + path, init);
  const data = await res.json().catch(() => ({}));
  return { status: res.status, data: data.data, code: data.code, requestId: data.requestId };
}

console.log('== 认证 API 测试 ==');
const stamp = Date.now().toString(36);
const testUser = `测试企鹅_${stamp}`;

{
  const r = await http('/api/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: testUser, password: '123456' }),
  });
  ok(r.status === 200 && r.data.token && r.data.user.elo === 1000, `注册成功 (${testUser})`);
  const dup = await http('/api/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: testUser, password: '123456' }),
  });
  ok(dup.status === 409, `重复注册被拒绝 (${dup.status})`);
  const bad = await http('/api/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: testUser, password: 'wrong!' }),
  });
  ok(bad.status === 401, `错误密码被拒绝 (${bad.status})`);
  const login = await http('/api/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: testUser, password: '123456' }),
  });
  ok(login.status === 200 && login.data.token, '登录成功');
  const noAuth = await http('/api/profile');
  ok(noAuth.status === 401, `无 token 拉档案被拒绝 (${noAuth.status})`);
  const prof = await http('/api/profile', { headers: { Authorization: `Bearer ${login.data.token}` } });
  ok(prof.status === 200 && prof.data.user.username === testUser, '档案接口正常');
}

// ---------- 并发对局 ----------
console.log(`\n== ${ROOMS} 房间并发对决 ==`);

/** 一个自动对弈的客户端 */
function autoPlayer(token, name) {
  const ws = new WebSocket(`${WS_BASE}?token=${encodeURIComponent(token)}`);
  let state = null;
  let seat = -1;
  let finished = false;
  let winner = null;
  const errors = [];
  const pending = [];
  ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.type === 'room') {
      seat = msg.you;
    } else if (msg.type === 'start') {
      state = msg.state;
      if (state.turn === seat) playTurn();
    } else if (msg.type === 'state' || msg.type === 'over') {
      state = msg.state;
      if (msg.type === 'over') {
        finished = true;
        winner = msg.winner;
      }
      if (!finished) playTurn();
    } else if (msg.type === 'error') {
      errors.push(msg.msg);
      if (process.env.VERBOSE) console.log(`   [${name}] error: ${msg.msg}`);
      // 走子被拒（如锁冲突）时若仍轮到我则重试
      if (state && state.turn === seat && state.phase !== 'finished' && !finished) {
        setTimeout(playTurn, 50);
      }
    } else if (msg.type === 'opponent_left') {
      errors.push('opponent_left');
    }
  };
  function playTurn() {
    if (state.turn !== seat) return;
    setTimeout(() => {
      if (state.turn !== seat || state.phase === 'finished') return;
      if (state.phase === 'placing') {
        const tile = G.legalPlacements(state)[0];
        ws.send(JSON.stringify({ type: 'place', tile }));
      } else {
        const peng = state.players[state.turn].penguins.find((p) => p >= 0 && G.legalMovesFrom(state, p).length > 0);
        if (peng === undefined) return; // 困毙由引擎处理
        const to = G.legalMovesFrom(state, peng)[0];
        ws.send(JSON.stringify({ type: 'move', from: peng, to }));
      }
    }, 5 + Math.random() * 20);
  }
  return {
    ws,
    name,
    get finished() {
      return finished;
    },
    get winner() {
      return winner;
    },
    get state() {
      return state;
    },
    get errors() {
      return errors;
    },
    close: () => ws.close(),
  };
}

async function runRoom(i) {
  const uname0 = `压测A${i}_${stamp}`;
  const uname1 = `压测B${i}_${stamp}`;
  const reg = async (u) =>
    (
      await http('/api/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: u, password: '123456' }),
      })
    ).data.token;
  const [t0, t1] = await Promise.all([reg(uname0), reg(uname1)]);

  const p0 = autoPlayer(t0, uname0);
  const p1 = autoPlayer(t1, uname1);
  await Promise.all([
    new Promise((r) => (p0.ws.onopen = r)),
    new Promise((r) => (p1.ws.onopen = r)),
  ]);

  return await new Promise((resolve) => {
    const roomCode = { value: '' };
    p0.ws.addEventListener('message', (ev) => {
      const msg = JSON.parse(ev.data);
      if (msg.type === 'room') roomCode.value = msg.room;
    });
    p0.ws.send(JSON.stringify({ type: 'create_room' }));
    const joinTimer = setInterval(() => {
      if (roomCode.value) {
        clearInterval(joinTimer);
        p1.ws.send(JSON.stringify({ type: 'join_room', room: roomCode.value }));
      }
    }, 20);
    const done = setInterval(() => {
      if (p0.finished && p1.finished) {
        clearInterval(done);
        clearInterval(joinTimer);
        const s = p0.state;
        resolve({
          room: roomCode.value,
          winner: p0.winner,
          p0Name: uname0,
          p1Name: uname1,
          scores: s ? `${s.players[0].score}:${s.players[1].score}` : '?',
          plies: s ? s.ply : 0,
        });
        p0.close();
        p1.close();
      }
    }, 50);
    setTimeout(() => {
      clearInterval(done);
      clearInterval(joinTimer);
      console.error(`    ✗ 房间 ${roomCode.value || '?'} 超时: A(ply=${p0.state?.ply ?? '-'},errors=${JSON.stringify(p0.errors)}) B(ply=${p1.state?.ply ?? '-'},errors=${JSON.stringify(p1.errors)})`);
      p0.close();
      p1.close();
      resolve(null);
    }, 120000);
  });
}

const t0 = Date.now();
const results = await Promise.all(Array.from({ length: ROOMS }, (_, i) => runRoom(i)));
const elapsed = ((Date.now() - t0) / 1000).toFixed(1);

const finishedRooms = results.filter(Boolean);
ok(finishedRooms.length === ROOMS, `${finishedRooms.length}/${ROOMS} 个房间全部正常终局（${elapsed}s）`);
for (const r of finishedRooms) {
  console.log(`    · 房间 ${r.room} ${r.p0Name} vs ${r.p1Name} → 比分 ${r.scores}（${r.plies} 步，胜者=${r.winner}）`);
}
ok(finishedRooms.every((r) => r.winner >= -1), '所有对局产生合法结果');

// ---------- 战绩落库验证 ----------
console.log('\n== 战绩 / Elo 落库验证 ==');
await new Promise((r) => setTimeout(r, 500)); // 等结算写库
const lb = await http('/api/leaderboard');
ok(lb.status === 200 && Array.isArray(lb.data.players), '天梯榜可访问');
for (const r of finishedRooms.slice(0, 3)) {
  const login = await http('/api/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: r.p0Name, password: '123456' }),
  });
  const prof = await http('/api/profile', { headers: { Authorization: `Bearer ${login.data.token}` } });
  const u = prof.data.user;
  const played = u.wins + u.losses + u.draws;
  ok(played >= 1, `${r.p0Name}: ${u.wins}胜${u.losses}负${u.draws}平 Elo=${u.elo}（已落库）`);
  const recent = prof.data.recent ?? [];
  ok(recent.length >= 1, `${r.p0Name}: 对局历史 ${recent.length} 条`);
  if (recent[0]) ok(recent[0].opponent === r.p1Name, `历史对手正确 (${recent[0].opponent})`);
}

console.log(`\n压测总计: ${pass} 通过, ${fail} 失败（${ROOMS} 房间并发, 耗时 ${elapsed}s）`);
process.exit(fail > 0 ? 1 : 0);
