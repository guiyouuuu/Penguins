/** 企鹅棋共享类型定义（前端本地 / 网络同步共用） */

export type Phase = 'placing' | 'moving' | 'finished';

export interface Tile {
  /** axial 坐标 q */
  q: number;
  /** axial 坐标 r */
  r: number;
  /** 鱼数量 1~3 */
  fish: number;
  /** 格子是否已沉没 */
  gone: boolean;
  /** 占据该格的企鹅所属玩家，-1 表示无 */
  owner: number;
}

export interface Player {
  /** 席位 0/1（联机时由服务器分配） */
  id: number;
  name: string;
  /** 已收集的鱼 */
  score: number;
  /** 各企鹅所在格索引，-1 表示未放置 */
  penguins: number[];
  /** 是否已被困（无法行动） */
  stuck: boolean;
}

export interface GameState {
  tiles: Tile[];
  players: Player[];
  phase: Phase;
  /** 当前行动玩家索引 */
  turn: number;
  /** 胜者：玩家 id；-1 平局；-2 未结束 */
  winner: number;
  /** 已进行回合数（含放置） */
  ply: number;
}

/** 一步棋（放置或移动），用于同步与动画 */
export interface Move {
  kind: 'place' | 'move';
  player: number;
  from: number;
  to: number;
}

/** 消息协议：客户端 → 服务器 */
export type ClientMsg =
  | { type: 'create_room' }
  | { type: 'join_room'; room: string }
  | { type: 'quick_match' }
  | { type: 'play_ai' }
  | { type: 'ready'; ready: boolean }
  | { type: 'start_game' }
  | { type: 'place'; tile: number }
  | { type: 'move'; from: number; to: number }
  | { type: 'resign' }
  | { type: 'rematch' }
  | { type: 'leave' };

/** 消息协议：服务器 → 客户端 */
export type ServerMsg =
  | { type: 'room'; room: string; you: number }
  | { type: 'waiting' }
  | { type: 'lobby'; room: string; names: [string, string]; ready: boolean }
  | { type: 'start'; state: GameState; names: [string, string] }
  | { type: 'state'; state: GameState; last: Move | null }
  | { type: 'over'; state: GameState; winner: number; last: Move | null }
  | { type: 'opponent_left' }
  | { type: 'rematch_ask' }
  | { type: 'error'; msg: string; code: number; message: string; requestId: string };
