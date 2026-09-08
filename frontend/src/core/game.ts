/** 企鹅棋核心规则引擎（纯函数式，不碰 DOM；与 Go 后端引擎保持同一套规则） */
import { boardCoords, HEX_DIRS } from './hex';
import type { GameState, Move, Player, Tile } from './types';

/** 按人数返回企鹅数：2 人 4 只、3 人 3 只、4 人 2 只 */
export function penguinCount(playerCount: number): number {
  return playerCount === 2 ? 4 : playerCount === 3 ? 3 : 2;
}

/** 生成随机棋盘：30 格 1 鱼、20 格 2 鱼、10 格 3 鱼（官方配置） */
function shuffledFish(): number[] {
  const bag: number[] = [];
  for (let i = 0; i < 30; i++) bag.push(1);
  for (let i = 0; i < 20; i++) bag.push(2);
  for (let i = 0; i < 10; i++) bag.push(3);
  for (let i = bag.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [bag[i], bag[j]] = [bag[j], bag[i]];
  }
  return bag;
}

/** 创建新对局 */
export function createGame(names: string[]): GameState {
  const coords = boardCoords();
  const fish = shuffledFish();
  const tiles: Tile[] = coords.map((c, i) => ({
    q: c.q,
    r: c.r,
    fish: fish[i],
    gone: false,
    owner: -1,
  }));
  const players: Player[] = names.map((name, i) => ({
    id: i,
    name,
    score: 0,
    penguins: new Array(penguinCount(names.length)).fill(-1),
    stuck: false,
  }));
  return { tiles, players, phase: 'placing', turn: 0, winner: -2, ply: 0 };
}

/** 邻居格索引（未沉没且存在），方向序与 HEX_DIRS 一致 */
export function neighbors(state: GameState, idx: number): number[] {
  const t = state.tiles[idx];
  const res: number[] = [];
  for (const [dq, dr] of HEX_DIRS) {
    const n = state.tiles.findIndex((x) => x.q === t.q + dq && x.r === t.r + dr);
    res.push(n);
  }
  return res;
}

/** 放置阶段的合法格：未沉没、无企鹅、恰好 1 条鱼 */
export function legalPlacements(state: GameState): number[] {
  return state.tiles
    .map((t, i) => ({ t, i }))
    .filter(({ t }) => !t.gone && t.owner === -1 && t.fish === 1)
    .map(({ i }) => i);
}

/** 某格企鹅的合法移动目标：六方向直线滑行，遇洞 / 企鹅 / 边界停止 */
export function legalMovesFrom(state: GameState, idx: number): number[] {
  const start = state.tiles[idx];
  const res: number[] = [];
  for (const [dq, dr] of HEX_DIRS) {
    let q = start.q + dq;
    let r = start.r + dr;
    for (;;) {
      const n = state.tiles.findIndex((x) => x.q === q && x.r === r);
      if (n === -1) break; // 出界
      const tile = state.tiles[n];
      if (tile.gone || tile.owner !== -1) break; // 洞或企鹅阻挡
      res.push(n);
      q += dq;
      r += dr;
    }
  }
  return res;
}

/** 玩家当前是否还有任何可行动的企鹅 */
export function playerCanMove(state: GameState, player: number): boolean {
  const p = state.players[player];
  for (const idx of p.penguins) {
    if (idx >= 0 && legalMovesFrom(state, idx).length > 0) return true;
  }
  return false;
}

/** 淘汰玩家（官方规则）：全部企鹅被困 → 企鹅离场，带走脚下格子的鱼，格子沉没 */
function eliminatePlayer(state: GameState, player: number): void {
  const p = state.players[player];
  for (let i = 0; i < p.penguins.length; i++) {
    const idx = p.penguins[i];
    if (idx >= 0) {
      state.tiles[idx].owner = -1;
      state.tiles[idx].gone = true;
      p.score += state.tiles[idx].fish;
      p.penguins[i] = -1;
    }
  }
  p.stuck = true;
}

/** 推进行动权：出局玩家跳过；无人可动则终局 */
function advanceTurn(state: GameState): void {
  const n = state.players.length;
  for (let step = 1; step <= n; step++) {
    const next = (state.turn + step) % n;
    if (state.players[next].stuck) continue; // 已出局
    if (playerCanMove(state, next)) {
      state.turn = next;
      return;
    }
    eliminatePlayer(state, next);
  }
  // 无人可动 → 结束
  finishGame(state);
}

/** 终局结算 */
function finishGame(state: GameState): void {
  state.phase = 'finished';
  let best = -1;
  let bestScore = -1;
  let tie = false;
  for (const p of state.players) {
    if (p.score > bestScore) {
      bestScore = p.score;
      best = p.id;
      tie = false;
    } else if (p.score === bestScore) {
      tie = true;
    }
  }
  state.winner = tie ? -1 : best;
}

/** 放置阶段总步数 */
function totalPlacements(state: GameState): number {
  return state.players.reduce((sum, p) => sum + p.penguins.length, 0);
}

/** 执行放置（已验证合法性） */
function doPlace(state: GameState, player: number, tileIdx: number): void {
  const tile = state.tiles[tileIdx];
  tile.owner = player;
  const p = state.players[player];
  const slot = p.penguins.indexOf(-1);
  p.penguins[slot] = tileIdx;
  state.ply++;
  if (state.ply >= totalPlacements(state)) {
    state.phase = 'moving';
    // 移动阶段由第一个放置的玩家（玩家 0）先行动
    state.turn = 0;
    if (!playerCanMove(state, state.turn)) {
      eliminatePlayer(state, state.turn);
      advanceTurn(state);
    }
  } else {
    state.turn = (state.turn + 1) % state.players.length;
  }
}

/** 执行移动（已验证合法性） */
function doMove(state: GameState, player: number, from: number, to: number): void {
  const fromTile = state.tiles[from];
  const toTile = state.tiles[to];
  fromTile.owner = -1;
  fromTile.gone = true; // 离开的格子沉没
  toTile.owner = player;
  state.players[player].score += fromTile.fish; // 起点格的鱼归玩家
  const p = state.players[player];
  p.penguins[p.penguins.indexOf(from)] = to;
  state.ply++;
  // 自己走完即被困 → 立即出局（官方规则：企鹅离场并带走脚下的鱼）
  if (!playerCanMove(state, player)) eliminatePlayer(state, player);
  advanceTurn(state);
}

/** 尝试走子：合法则应用并返回 true（返回新状态，不修改原状态） */
export function applyMove(state: GameState, mv: Move): GameState | null {
  if (state.phase === 'finished') return null;
  if (mv.player !== state.turn) return null;
  const next: GameState = structuredClone(state);
  if (mv.kind === 'place') {
    if (next.phase !== 'placing') return null;
    const t = next.tiles[mv.to];
    if (!t || t.gone || t.owner !== -1 || t.fish !== 1) return null;
    doPlace(next, mv.player, mv.to);
  } else {
    if (next.phase !== 'moving') return null;
    const from = next.tiles[mv.from];
    if (!from || from.owner !== mv.player) return null;
    if (!legalMovesFrom(next, mv.from).includes(mv.to)) return null;
    doMove(next, mv.player, mv.from, mv.to);
  }
  return next;
}

/** 当前局面下某玩家的全部合法着法（AI 与提示用） */
export function allMoves(state: GameState, player: number): Move[] {
  const res: Move[] = [];
  if (state.phase === 'finished') return res;
  if (state.phase === 'placing') {
    if (player === state.turn) {
      for (const i of legalPlacements(state)) res.push({ kind: 'place', player, from: -1, to: i });
    }
    return res;
  }
  if (player !== state.turn) return res;
  for (const idx of state.players[player].penguins) {
    if (idx < 0) continue;
    for (const to of legalMovesFrom(state, idx)) res.push({ kind: 'move', player, from: idx, to });
  }
  return res;
}
