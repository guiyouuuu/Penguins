// 深度调试：对比前端 TS 引擎与服务器对走子的判定
import { execSync } from 'node:child_process';
const root = new URL('..', import.meta.url).pathname;
execSync(`npx esbuild src/core/game.ts --bundle --format=esm --outfile=/tmp/penguin-game.mjs --log-level=error`, { cwd: root + 'frontend' });
const G = await import('/tmp/penguin-game.mjs');

const ws = new WebSocket('ws://localhost:5173/ws');
let state = null;

ws.onopen = () => ws.send(JSON.stringify({ type: 'play_ai' }));
ws.onmessage = (ev) => {
  const m = JSON.parse(ev.data);
  if (m.type === 'start' || m.type === 'state') {
    state = m.state;
    if (state.phase === 'placing') {
      // 轮到玩家才放
      if (state.turn === 0) {
        const tile = G.legalPlacements(state)[0];
        ws.send(JSON.stringify({ type: 'place', tile }));
      }
    } else if (state.phase === 'moving' && state.turn === 0) {
      tryMove();
    }
  } else if (m.type === 'error') {
    console.log('ERROR:', m.msg);
    dumpDetail();
    process.exit(1);
  } else if (m.type === 'over') {
    console.log('over');
    process.exit(0);
  }
};

let tried = false;
function tryMove() {
  if (tried) return;
  tried = true;
  const peng = state.players[0].penguins.find((p) => p >= 0 && G.legalMovesFrom(state, p).length > 0);
  const moves = G.legalMovesFrom(state, peng);
  const to = moves[0];
  console.log('企鹅索引:', peng, '坐标:', state.tiles[peng].q, state.tiles[peng].r);
  console.log('TS 引擎合法目标:', moves, '对应坐标:', moves.map((i) => `(${state.tiles[i].q},${state.tiles[i].r})`).join(' '));
  // 模拟服务器校验：从 from 沿 6 方向滑行能到的格
  const DIRS = [[1,0],[1,-1],[0,-1],[-1,0],[-1,1],[0,1]];
  const serverReach = [];
  for (const [dq, dr] of DIRS) {
    let q = state.tiles[peng].q + dq, r = state.tiles[peng].r + dr;
    for (;;) {
      const n = state.tiles.findIndex((t) => t.q === q && t.r === r);
      if (n === -1) break;
      const t = state.tiles[n];
      if (t.gone || t.owner !== -1) break;
      serverReach.push(n);
      q += dq; r += dr;
    }
  }
  console.log('滑行复算目标:', serverReach);
  console.log('发送: from=', peng, 'to=', to);
  ws.send(JSON.stringify({ type: 'move', from: peng, to }));
}

function dumpDetail() {
  if (!state) return;
  console.log('当前 turn:', state.turn, 'phase:', state.phase);
  console.log('玩家0企鹅:', state.players[0].penguins, '玩家1企鹅:', state.players[1].penguins);
}

setTimeout(() => { console.log('TIMEOUT'); dumpDetail(); process.exit(1); }, 20000);
