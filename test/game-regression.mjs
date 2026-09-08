import assert from 'node:assert/strict';
import { test } from 'node:test';
import { build } from '../frontend/node_modules/esbuild/lib/main.js';

const bundle = await build({ entryPoints: [new URL('../frontend/src/core/game.ts', import.meta.url).pathname], bundle: true, format: 'esm', write: false });
const G = await import(`data:text/javascript;base64,${Buffer.from(bundle.outputFiles[0].text).toString('base64')}`);

function isolated(phase = 'moving') {
  return {
    tiles: [0, 5, 6, 7].map((q, i) => ({ q, r: 0, fish: 1, gone: false, owner: i === 0 ? 0 : i === 1 && phase === 'moving' ? 1 : -1 })),
    players: [
      { id: 0, name: 'Black', score: 0, penguins: [0], stuck: false },
      { id: 1, name: 'Orange', score: 0, penguins: [phase === 'placing' ? -1 : 1], stuck: false },
    ],
    phase, turn: 1, winner: -2, ply: phase === 'placing' ? 1 : 2,
  };
}

test('blocked first player is settled immediately after placement', () => {
  const before = isolated('placing');
  const next = G.applyMove(before, { kind: 'place', player: 1, from: -1, to: 1 });
  assert.equal(next.phase, 'moving');
  assert.equal(next.turn, 1);
  assert.equal(next.players[0].stuck, true);
  assert.equal(next.players[0].score, 1);
  assert.deepEqual(next.players[0].penguins, [-1]);
  assert.equal(next.tiles[0].gone, true);
  assert.equal(before.players[0].score, 0);
});

test('opponent continues alone, terminal fish are counted exactly once', () => {
  const next = G.applyMove(isolated(), { kind: 'move', player: 1, from: 1, to: 2 });
  assert.equal(next.phase, 'moving');
  assert.equal(next.turn, 1);
  assert.equal(next.players[0].stuck, true);
  assert.equal(next.players[0].score, 1);
  const end = G.applyMove(next, { kind: 'move', player: 1, from: 2, to: 3 });
  assert.equal(end.phase, 'finished');
  assert.equal(end.winner, 1);
  assert.deepEqual(end.players.map(p => p.score), [1, 3]);
  assert.ok(end.players.every(p => p.stuck && p.penguins.every(i => i === -1)));
  assert.equal(G.applyMove(end, { kind: 'move', player: 1, from: 2, to: 3 }), null);
});

test('sliding preserves intermediate ice and holes block the route', () => {
  const state = isolated();
  state.tiles.push({ q: 8, r: 0, fish: 2, gone: false, owner: -1 });
  const next = G.applyMove(state, { kind: 'move', player: 1, from: 1, to: 3 });
  assert.equal(next.tiles[1].gone, true);
  assert.equal(next.tiles[2].gone, false);
  assert.equal(next.tiles[3].gone, false);
  state.tiles[2].gone = true;
  assert.equal(G.applyMove(state, { kind: 'move', player: 1, from: 1, to: 3 }), null);
});
