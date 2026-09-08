import assert from 'node:assert/strict';
import { test } from 'node:test';
import { build } from '../frontend/node_modules/esbuild/lib/main.js';

const bundle = await build({ entryPoints: [new URL('../frontend/src/core/game.ts', import.meta.url).pathname], bundle: true, format: 'esm', write: false });
const G = await import(`data:text/javascript;base64,${Buffer.from(bundle.outputFiles[0].text).toString('base64')}`);

test('placement collects its fish immediately', () => {
  const before = G.createGame(['Black', 'Orange']);
  const to = G.legalPlacements(before)[0];
  const next = G.applyMove(before, { kind: 'place', player: 0, from: -1, to });
  assert.equal(next.players[0].score, 1);
  assert.equal(before.players[0].score, 0);
});

test('arrival scores destination fish; departure and isolated removal never score again', () => {
  const before = {
    tiles: [1, 3, 2, 1].map((fish, q) => ({ q, r: 0, fish, gone: false, owner: q === 0 ? 0 : -1 })),
    players: [{ id: 0, name: 'Black', score: 1, penguins: [0], stuck: false }, { id: 1, name: 'Orange', score: 0, penguins: [-1], stuck: true }],
    phase: 'moving', turn: 0, winner: -2, ply: 2,
  };
  let next = before;
  for (const [from, to, score] of [[0, 1, 4], [1, 2, 6], [2, 3, 7]]) {
    next = G.applyMove(next, { kind: 'move', player: 0, from, to });
    assert.equal(next.players[0].score, score);
  }
  assert.equal(next.phase, 'finished');
  assert.deepEqual(next.players[0].penguins, [-1]);
  assert.equal(before.players[0].score, 1);
});

function isolated(phase = 'moving') {
  return {
    tiles: [0, 5, 6, 7].map((q, i) => ({ q, r: 0, fish: 1, gone: false, owner: i === 0 ? 0 : i === 1 && phase === 'moving' ? 1 : -1 })),
    players: [
      { id: 0, name: 'Black', score: 1, penguins: [0], stuck: false },
      { id: 1, name: 'Orange', score: phase === 'placing' ? 0 : 1, penguins: [phase === 'placing' ? -1 : 1], stuck: false },
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
  assert.equal(before.players[0].score, 1);
});

test('opponent continues alone, terminal removal does not add fish again', () => {
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
  state.tiles[2].fish = 3;
  state.tiles.push({ q: 8, r: 0, fish: 2, gone: false, owner: -1 });
  const next = G.applyMove(state, { kind: 'move', player: 1, from: 1, to: 3 });
  assert.equal(next.tiles[1].gone, true);
  assert.equal(next.tiles[2].gone, false);
  assert.equal(next.tiles[3].gone, false);
  assert.equal(next.players[1].score, 2); // Initial placement + destination; intermediate fish stay on the board.
  state.tiles[2].gone = true;
  assert.equal(G.applyMove(state, { kind: 'move', player: 1, from: 1, to: 3 }), null);
});

test('scores equal visited ice throughout two to four player games', () => {
  for (const count of [2, 3, 4]) {
    for (let round = 0; round < 5; round++) {
      let state = G.createGame(Array.from({ length: count }, (_, i) => `Player ${i}`));
      for (let steps = 0; state.phase !== 'finished'; steps++) {
        assert.ok(steps < 100);
        const moves = G.allMoves(state, state.turn);
        assert.ok(moves.length > 0);
        state = G.applyMove(state, moves[(steps * 7 + round) % moves.length]);
        const visitedFish = state.tiles.filter(t => t.gone || t.owner >= 0).reduce((sum, t) => sum + t.fish, 0);
        assert.equal(state.players.reduce((sum, p) => sum + p.score, 0), visitedFish);
      }
    }
  }
});

for (const mover of [0, 1]) {
  test(`single ice penguin falls immediately after player ${mover} moves`, () => {
    const before = {
      tiles: [0, 1, 2, 3, 10, 11, 12].map((q, i) => ({ q, r: 0, fish: [3, 2, 1, 1, 1, 1, 1][i], gone: false, owner: [0, 1, -1, -1, 0, -1, 1][i] })),
      players: [{ id: 0, name: 'Black', score: 4, penguins: [0, 4], stuck: false }, { id: 1, name: 'Orange', score: 3, penguins: [1, 6], stuck: false }],
      phase: 'moving', turn: mover, winner: -2, ply: 4,
    };
    if (mover === 0) {
      before.tiles[0].owner = -1;
      before.tiles[1].owner = 0;
      before.players[0].penguins[0] = 1;
      before.players[1].penguins[0] = -1;
      before.players[0].score = 3;
      before.players[1].score = 1;
    }
    const next = G.applyMove(before, { kind: 'move', player: mover, from: 1, to: mover === 0 ? 0 : 2 });
    assert.deepEqual(next.players[0].penguins, [-1, 4]);
    assert.equal(next.tiles[0].gone, true);
    assert.equal(next.tiles[0].owner, -1);
    assert.equal(next.players[0].score, mover === 0 ? 6 : 4);
    assert.equal(next.players[0].stuck, false);
    assert.equal(G.playerCanMove(next, 0), true);
    assert.equal(next.phase, 'moving');
    assert.equal(before.players[0].score, mover === 0 ? 3 : 4);
    assert.equal(before.tiles[0].gone, false);
  });
}

test('occupied adjacent ice does not count as single ice', () => {
  const before = {
    tiles: [0, 1, 2, 5, 6, 10, 11, 12].map((q, i) => ({ q, r: 0, fish: 1, gone: false, owner: [0, 1, -1, 0, -1, 1, -1, -1][i] })),
    players: [{ id: 0, score: 0, penguins: [0, 3], stuck: false }, { id: 1, score: 0, penguins: [1, 5], stuck: false }],
    phase: 'moving', turn: 1, winner: -2, ply: 4,
  };
  const next = G.applyMove(before, { kind: 'move', player: 1, from: 5, to: 6 });
  assert.equal(next.tiles[0].gone, false);
  assert.equal(next.players[0].penguins[0], 0);
  assert.equal(next.players[0].score, 0);
});
