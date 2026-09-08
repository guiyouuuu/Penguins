import assert from 'node:assert/strict';
import { build } from '../frontend/node_modules/esbuild/lib/main.js';
const bundle = await build({ entryPoints: [new URL('../frontend/src/core/game.ts', import.meta.url).pathname], bundle: true, format: 'esm', write: false });
const G = await import(`data:text/javascript;base64,${Buffer.from(bundle.outputFiles[0].text).toString('base64')}`);
const ws = new WebSocket(process.env.PENGUIN_WS_URL || 'ws://127.0.0.1:8081/ws');
let messages = 0, consecutiveAI = 0, previousAI = false;
try {
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('Full game timed out')), 90000);
    const fail = error => { clearTimeout(timer); reject(error); };
    ws.onerror = () => fail(new Error('WebSocket connection failed'));
    ws.onopen = () => ws.send(JSON.stringify({ type: 'play_ai' }));
    ws.onmessage = event => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'error') throw new Error(msg.msg);
        if (!msg.state) return;
        messages++;
        const state = msg.state;
        if (previousAI && msg.last?.player === 1) consecutiveAI++;
        previousAI = msg.last?.player === 1;
        if (msg.type === 'over') {
          assert.equal(state.phase, 'finished');
          assert.ok(state.players.every(p => p.stuck && p.penguins.every(i => i === -1)));
          assert.ok(state.players.reduce((sum, p) => sum + p.score, 0) <= 100);
          clearTimeout(timer);
          console.log(JSON.stringify({ result: 'passed', messages, consecutiveAI, scores: state.players.map(p => p.score), winner: state.winner }));
          resolve();
          return;
        }
        const moves = G.allMoves(state, state.turn);
        assert.ok(moves.length > 0, `Deadlock at ply ${state.ply}`);
        if (state.turn === 0) {
          const move = moves[0];
          ws.send(JSON.stringify(move.kind === 'place' ? { type: 'place', tile: move.to } : { type: 'move', from: move.from, to: move.to }));
        }
      } catch (error) { fail(error); }
    };
  });
} finally {
  if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'leave' }));
  ws.close();
}
