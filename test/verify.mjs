// 企鹅棋规则引擎 + WebSocket 协议端到端验证
// 用法: node test/verify.mjs（先 esbuild 编译核心逻辑）
import { execSync } from 'node:child_process';
import { createServer } from 'node:http';

const root = new URL('..', import.meta.url).pathname;
// 1. 编译核心逻辑
execSync(
  `npx esbuild src/core/game.ts --bundle --format=esm --outfile=/tmp/penguin-game.mjs --log-level=error`,
  { cwd: root + 'frontend' },
);
const G = await import('/tmp/penguin-game.mjs');

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

console.log('== 规则引擎测试 ==');
// 棋盘
{
  const st = G.createGame(['A', 'B']);
  ok(st.tiles.length === 60, '棋盘 60 格');
  const fish = st.tiles.reduce((a, t) => a + t.fish, 0);
  ok(fish === 100, `鱼总量 100 (实际 ${fish})`);
  const one = st.tiles.filter((t) => t.fish === 1).length;
  const two = st.tiles.filter((t) => t.fish === 2).length;
  const three = st.tiles.filter((t) => t.fish === 3).length;
  ok(one === 30 && two === 20 && three === 10, `鱼分布 30/20/10 (实际 ${one}/${two}/${three})`);
  ok(st.players[0].penguins.length === 4 && st.players[1].penguins.length === 4, '每人 4 只企鹅');
  ok(st.phase === 'placing' && st.turn === 0, '初始为放置阶段');
}

// 放置规则
{
  const st = G.createGame(['A', 'B']);
  const legal = G.legalPlacements(st);
  ok(legal.length === 30, `放置阶段 30 个合法格 (实际 ${legal.length})`);
  const twoFish = st.tiles.findIndex((t) => t.fish === 2);
  ok(!legal.includes(twoFish), '2 鱼格不可放置');
  const bad = G.applyMove(st, { kind: 'place', player: 0, from: -1, to: twoFish });
  ok(bad === null, '2 鱼格放置被拒绝');
  // 错误回合
  const badTurn = G.applyMove(st, { kind: 'place', player: 1, from: -1, to: legal[0] });
  ok(badTurn === null, '非当前玩家走子被拒绝');
  // 正常放置 x8
  let s = st;
  let player = 0;
  let placed = 0;
  while (s.phase === 'placing') {
    const mv = { kind: 'place', player, from: -1, to: G.legalPlacements(s)[0] };
    const next = G.applyMove(s, mv);
    ok(next !== null, `第 ${placed + 1} 次放置成功`);
    s = next;
    player = 1 - player;
    placed++;
  }
  ok(placed === 8, `8 次放置后进入移动阶段 (实际 ${placed})`);
  ok(s.phase === 'moving', '阶段切换为 moving');
}

// 移动规则 + 计分
{
  // 构造确定性棋盘：全部 1 鱼
  const st = G.createGame(['A', 'B']);
  for (const t of st.tiles) t.fish = 1;
  let s = st;
  let player = 0;
  while (s.phase === 'placing') {
    s = G.applyMove(s, { kind: 'place', player, from: -1, to: G.legalPlacements(s)[0] });
    player = 1 - player;
  }
  // 玩家 0 移动第一只企鹅
  const peng = s.players[s.turn].penguins[0];
  const targets = G.legalMovesFrom(s, peng);
  ok(targets.length > 0, '有合法移动目标');
  const before = s.players[s.turn].score;
  s = G.applyMove(s, { kind: 'move', player: s.turn, from: peng, to: targets[0] });
  ok(s !== null, '移动成功');
  ok(s.players[1 - s.turn === 1 ? 0 : 1].score === before, '未行动玩家分数不变');
  ok(s.tiles[peng].gone, '起点格沉没');
  ok(s.tiles[peng].owner === -1, '起点格无企鹅');
  ok(s.players[(s.turn + 1) % 2].score === before + 1, '行动玩家 +1 鱼');
  // 非法：跳过企鹅
  const t2 = G.legalMovesFrom(s, s.players[s.turn].penguins[0]);
  if (t2.length > 1) {
    const skip = G.applyMove(s, { kind: 'move', player: s.turn, from: t2[0], to: t2[1] });
    ok(skip === null, '不能从非己方格出发');
  }
}

// 困毙出局：鱼已在落脚时计分，离场不再加分，对方继续行动。
{
  // 用真实随机对局模拟直到结束
  let s = G.createGame(['A', 'B']);
  let player = 0;
  let guard = 0;
  while (s.phase !== 'finished' && guard < 500) {
    guard++;
    const moves = G.allMoves(s, s.turn);
    if (moves.length === 0) {
      // 引擎应保证 turn 玩家必有着法；否则死锁
      ok(false, `第 ${s.ply} 步轮到玩家 ${s.turn} 但无着法（死锁）`);
      break;
    }
    s = G.applyMove(s, moves[0]);
  }
  ok(s.phase === 'finished', `对局正常终局 (${s.ply} 步)`);
  ok(s.winner >= -1, `产生结果: 胜者=${s.winner}`);
  const totalFish = s.players[0].score + s.players[1].score;
  ok(totalFish <= 100, `总鱼数不超标 (${totalFish})`);
  // 新规则不变量：终局全员出局，企鹅全部离场，场上无遗留企鹅
  ok(s.players.every((p) => p.stuck), '终局时双方均已出局');
  ok(s.players.every((p) => p.penguins.every((x) => x === -1)), '出局企鹅全部离场（penguins=-1）');
  const alive = s.tiles.filter((t) => !t.gone && t.owner !== -1).length;
  ok(alive === 0, `场上无遗留企鹅 (${alive})`);
}

console.log(`\n规则测试: ${pass} 通过, ${fail} 失败\n`);

// ================= WebSocket 协议测试 =================
console.log('== WebSocket 协议测试 (人机对战) ==');

const results = [];
const waiters = [];
function expect(ws, pred, name, timeout = 8000) {
  return new Promise((resolve) => {
    const timer = setTimeout(() => {
      ok(false, `${name} (超时)`);
      resolve(null);
    }, timeout);
    waiters.push((msg) => {
      if (msg.type === 'error' && !pred(msg)) {
        // 服务器报错：快速失败
        clearTimeout(timer);
        ok(false, `${name} (服务器返回错误: ${msg.msg})`);
        resolve(null);
        return true;
      }
      if (pred(msg)) {
        clearTimeout(timer);
        ok(true, name);
        resolve(msg);
        return true;
      }
      return false;
    });
  });
}

function makeClient() {
  const ws = new WebSocket('ws://localhost:5173/ws');
  ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (process.env.VERBOSE) console.log('   [msg]', msg.type, 'ply=', msg.state?.ply ?? '', 'phase=', msg.state?.phase ?? '', msg.msg ?? '');
    for (let i = 0; i < waiters.length; i++) {
      if (waiters[i](msg)) {
        waiters.splice(i, 1);
        break;
      }
    }
  };
  ws.onclose = () => {
    if (process.env.VERBOSE) console.log('   [ws closed]');
  };
  return new Promise((res) => {
    ws.onopen = () => res(ws);
  });
}

const ws = await makeClient();
const startP = expect(ws, (m) => m.type === 'start', '收到 start（人机开局）');
ws.send(JSON.stringify({ type: 'play_ai' }));
const start = await startP;
if (start) {
  let state = start.state;
  ok(state.players.length === 2 && state.players[1].name === '冰原企鹅', 'AI 席位就位');
  ok(state.turn === 0, '玩家先手');

  // 放置 8 只企鹅（玩家放 → AI 应答交替）
  let done = null;
  for (let i = 0; i < 4; i++) {
    const p1 = expect(ws, (m) => m.type === 'state' && m.state.ply === state.ply + 2, `第 ${i + 1} 轮放置：AI 回应`);
    ws.send(JSON.stringify({ type: 'place', tile: G.legalPlacements(state)[0] }));
    done = await p1;
    if (!done) break;
    state = done.state;
  }
  if (done) {
    ok(state.phase === 'moving' || state.ply >= 8, `放置完成 (phase=${state.phase}, ply=${state.ply})`);

    // 走一步移动（选一只有路可走的企鹅）
    const myPeng = state.players[0].penguins.find((p) => p >= 0 && G.legalMovesFrom(state, p).length > 0);
    ok(myPeng !== undefined, '玩家有可行动的企鹅');
    if (myPeng === undefined) {
      ws.close();
      process.exit(1);
    }
    const mv = G.legalMovesFrom(state, myPeng)[0];
    const p2 = expect(ws, (m) => m.type === 'state' || m.type === 'over', '移动后收到 AI 回应');
    ws.send(JSON.stringify({ type: 'move', from: myPeng, to: mv }));
    const resp = await p2;
    if (resp) {
      ok(resp.state.players[0].score >= 1, `玩家得分 (实际 ${resp.state.players[0].score})`);
      // 非法走子应报错（从对手的企鹅出发必被拒绝）
      const pErr = expect(ws, (m) => m.type === 'error', '非法走子被服务器拒绝');
      ws.send(JSON.stringify({ type: 'move', from: resp.state.players[1].penguins[0], to: resp.state.players[1].penguins[0] }));
      await pErr;
      // 认输
      const pOver = expect(ws, (m) => m.type === 'over', '认输后收到 over');
      ws.send(JSON.stringify({ type: 'resign' }));
      const over = await pOver;
      if (over) ok(over.winner === 1, `AI 获胜 (winner=${over.winner})`);
    }
  }
}
ws.close();

// ---- 联机双客户端测试（登录用户） ----
console.log('\n== WebSocket 协议测试 (联机对战) ==');
const stamp = Date.now().toString(36);
async function regUser(u) {
  const r = await fetch('http://localhost:8080/api/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: u, password: '123456' }),
  });
  return (await r.json()).data.token;
}
const tokenA = await regUser(`验证A_${stamp}`);
const tokenB = await regUser(`验证B_${stamp}`);
ok(!!tokenA && !!tokenB, '双用户注册获取 token');

function makeAuthClient(token) {
  const ws = new WebSocket(`ws://localhost:5173/ws?token=${encodeURIComponent(token)}`);
  ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (process.env.VERBOSE) console.log('   [msg]', msg.type, 'ply=', msg.state?.ply ?? '', 'phase=', msg.state?.phase ?? '', msg.msg ?? '');
    for (let i = 0; i < waiters.length; i++) {
      if (waiters[i](msg)) {
        waiters.splice(i, 1);
        break;
      }
    }
  };
  return new Promise((res) => {
    ws.onopen = () => res(ws);
  });
}

const wsA = await makeAuthClient(tokenA);
const roomP = expect(wsA, (m) => m.type === 'room', 'A 创建房间');
wsA.send(JSON.stringify({ type: 'create_room' }));
const roomMsg = await roomP;
ok(roomMsg && roomMsg.you === 0, 'A 坐席 0');

const wsB = await makeAuthClient(tokenB);
const startAB = expect(wsA, (m) => m.type === 'start', 'A 收到开局');
const startB = expect(wsB, (m) => m.type === 'start', 'B 收到开局');
const joinedB = expect(wsB, (m) => m.type === 'lobby' && m.names[1], 'B 进入等待房间');
wsB.send(JSON.stringify({ type: 'join_room', room: roomMsg.room }));
await joinedB;
const readyA = expect(wsA, (m) => m.type === 'lobby' && m.ready, 'A 收到好友准备');
wsB.send(JSON.stringify({ type: 'ready', ready: true }));
await readyA;
wsA.send(JSON.stringify({ type: 'start_game' }));
const [sA, sB] = await Promise.all([startAB, startB]);
if (sA && sB) {
  ok(sA.state.tiles.length === 60 && sB.state.tiles.length === 60, '双方同步初始局面');
  // A 放置 → B 收到 state
  const bRec = expect(wsB, (m) => m.type === 'state', 'B 收到 A 的走子广播');
  wsA.send(JSON.stringify({ type: 'place', tile: G.legalPlacements(sA.state)[0] }));
  const bState = await bRec;
  ok(bState && bState.state.ply === 1, '广播状态同步 (ply=1)');
  // B 断线 → A 收到 opponent_left
  const aLeft = expect(wsA, (m) => m.type === 'opponent_left', 'B 离开后 A 收到通知');
  wsB.close();
  await aLeft;
}
wsA.close();

console.log(`\n总计: ${pass} 通过, ${fail} 失败`);
process.exit(fail > 0 ? 1 : 0);
