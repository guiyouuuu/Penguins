/** Canvas 渲染器：卡通清新典雅风格（冰原棋盘 + 碎裂冰洞 + 圆润企鹅 + 小蓝鱼） */
import { axialToPixel, hexCorners } from '../core/hex';
import type { GameState, Move } from '../core/types';
import { legalMovesFrom } from '../core/game';
import { iceArtwork, penguinArtwork } from './artwork';

export const PALETTE = {
  bgTop: '#DFF2FA',
  bgBottom: '#EAF7F0',
  tileA: '#F4FBFE',
  tileB: '#DCF0FA',
  tileEdge: '#FFFFFF',
  tileShadow: 'rgba(125, 175, 205, 0.4)',
  crack: 'rgba(130, 185, 215, 0.55)',
  water: '#9CCBE9',
  waterDeep: '#7FB8DE',
  waterEdge: 'rgba(130, 185, 220, 0.85)',
  shard: '#F6FCFF',
  shardEdge: 'rgba(140, 195, 225, 0.9)',
  highlight: '#FFF3C2',
  highlightEdge: '#F2C94C',
  fish: '#5A9BD4',
  fishDark: '#2F5E8C',
  pengBody: ['#2F3B45', '#F2A65A'],
  pengBelly: ['#FFFFFF', '#FFF6E8'],
  pengBeak: ['#F2A65A', '#C2602F'],
  glow: '#E8A13C',
  ring: 'rgba(232, 161, 60, 0.5)',
} as const;

/** 稳定的格子伪随机种子（装饰裂纹 / 碎冰形状用，同一格子每次一致） */
function tileSeed(q: number, r: number): number {
  let h = (q * 374761393 + r * 668265263) | 0;
  h = Math.imul(h ^ (h >>> 13), 1274126177);
  return Math.abs(h ^ (h >>> 16));
}

export interface RenderUI {
  /** 选中的企鹅格索引，-1 无 */
  selected: number;
  /** 可落子格集合 */
  reachable: Set<number>;
  /** 悬停格索引，-1 无 */
  hover: number;
  /** 我方席位（联机），本地双人传 0 */
  mySeat: number;
}

const MOVE_MS = 420;
const SINK_MS = 960;

export class BoardRenderer {
  private ctx: CanvasRenderingContext2D;
  private size = 40;
  private offX = 0;
  private offY = 0;
  private moveAnim: { player: number; from: number; to: number; t0: number } | null = null;
  private sinkAnims: Array<{ idx: number; t0: number }> = [];
  private placeAnim: { to: number; t0: number } | null = null;
  /** 待播放的走子动画（延迟到 render 时才换算像素，此时 resize 已完成） */
  private pendingMove: Move | null = null;
  /** 待播放的碎裂动画格索引 */
  private pendingSinks: number[] = [];
  private pendingDepartures: Array<{ idx: number; player: number }> = [];
  private departures: Array<{ idx: number; player: number; t0: number }> = [];
  private penguins: CanvasImageSource[] = [penguinArtwork(0), penguinArtwork(1)];
  private ice: CanvasImageSource = iceArtwork();
  private reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
  private mobilityState: GameState | null = null;
  private movable = new Set<number>();

  get isAnimating(): boolean {
    return !!(this.pendingMove || this.pendingSinks.length || this.moveAnim || this.placeAnim || this.sinkAnims.length || this.pendingDepartures.length || this.departures.length);
  }

  resetAnimations(): void {
    this.pendingMove = null;
    this.pendingSinks = [];
    this.moveAnim = null;
    this.placeAnim = null;
    this.sinkAnims = [];
    this.pendingDepartures = [];
    this.departures = [];
  }

  playDepartures(penguins: Array<{ idx: number; player: number }>): void {
    if (!this.reducedMotion.matches) this.pendingDepartures.push(...penguins);
  }

  constructor(private canvas: HTMLCanvasElement) {
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new Error('Canvas 2D 不可用');
    this.ctx = ctx;
    for (const [i, name] of ['penguin-black', 'penguin-orange', 'ice-floe'].entries()) {
      const img = new Image();
      img.onload = () => { if (i === 2) this.ice = img; else this.penguins[i] = img; };
      img.src = `/artwork/${name}.png`;
    }
  }

  /** 触发一步棋的企鹅跳跃动画（像素换算延迟到下一帧 render 时执行） */
  playMove(mv: Move): void {
    if (this.reducedMotion.matches) return;
    this.pendingMove = mv;
  }

  /** 触发格子碎裂沉没动画（走子起点 / 出局企鹅脚下） */
  playSinks(idxs: number[]): void {
    if (this.reducedMotion.matches) return;
    this.pendingSinks.push(...idxs);
  }

  /** 将 pending 动画转换为像素动画（在 render 内调用，保证 resize 已完成） */
  private flushPending(now: number): void {
    const mv = this.pendingMove;
    if (mv) {
      this.pendingMove = null;
      if (mv.kind === 'move') {
        this.moveAnim = { player: mv.player, from: mv.from, to: mv.to, t0: now };
      } else {
        this.placeAnim = { to: mv.to, t0: now };
      }
    }
    if (this.pendingSinks.length > 0) {
      for (const idx of new Set(this.pendingSinks)) {
        this.sinkAnims.push({ idx, t0: now + (mv?.kind === 'move' && mv.to === idx ? MOVE_MS : 0) });
      }
      this.pendingSinks = [];
    }
    for (const p of this.pendingDepartures) {
      this.departures.push({ ...p, t0: now + (mv?.kind === 'move' && mv.to === p.idx ? MOVE_MS : 0) });
    }
    this.pendingDepartures = [];
  }

  /** 点击命中：画布内坐标 → 格索引（-1 无） */
  cellAt(x: number, y: number): number {
    const cells = this.cellsCache;
    if (cells.length === 0) return -1;
    const qx = x - this.offX;
    const qy = y - this.offY;
    // 像素 → axial（cube rounding）
    const rf = ((2 / 3) * qy) / this.size;
    const qf = (Math.sqrt(3) / 3) * (qx / this.size) - rf / 2;
    const xf = qf;
    const zf = rf;
    const yf = -xf - zf;
    let rx = Math.round(xf);
    let ry = Math.round(yf);
    let rz = Math.round(zf);
    const dx = Math.abs(rx - xf);
    const dy = Math.abs(ry - yf);
    const dz = Math.abs(rz - zf);
    if (dx > dy && dx > dz) rx = -ry - rz;
    else if (dy <= dz) rz = -rx - ry;
    const exact = cells.findIndex((c) => c.q === rx && c.r === rz);
    if (exact >= 0) return exact;
    // 边缘容差：最近格中心
    let best = -1;
    let bestD = this.size * this.size;
    for (let i = 0; i < cells.length; i++) {
      const p = axialToPixel(cells[i].q, cells[i].r, this.size);
      const d = (p.x - qx) ** 2 + (p.y - qy) ** 2;
      if (d < bestD) {
        bestD = d;
        best = i;
      }
    }
    return best;
  }

  /** 计算格中心像素 */
  centerOf(idx: number): { x: number; y: number } {
    const cells = this.cellsCache;
    if (!cells[idx]) return { x: 0, y: 0 };
    const p = axialToPixel(cells[idx].q, cells[idx].r, this.size);
    return { x: p.x + this.offX, y: p.y + this.offY };
  }

  private cellsCache: Array<{ q: number; r: number }> = [];
  /** 缓存的画布 CSS 尺寸 */
  private w = 0;
  private h = 0;

  /** 适配画布尺寸（resize 事件 / 状态更新时调用） */
  resize(state: GameState): void {
    const dpr = window.devicePixelRatio || 1;
    const rect = this.canvas.getBoundingClientRect();
    const w = rect.width;
    const h = rect.height;
    if (w === 0 || h === 0) return;
    this.w = w;
    this.h = h;
    this.canvas.width = Math.round(w * dpr);
    this.canvas.height = Math.round(h * dpr);
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

    // 计算棋盘包围盒
    const cells = state.tiles.map((t) => ({ q: t.q, r: t.r }));
    this.cellsCache = cells;
    let minX = Infinity;
    let maxX = -Infinity;
    let minY = Infinity;
    let maxY = -Infinity;
    for (const c of cells) {
      const p = axialToPixel(c.q, c.r, 1);
      minX = Math.min(minX, p.x - 1);
      maxX = Math.max(maxX, p.x + 1);
      minY = Math.min(minY, p.y - 1);
      maxY = Math.max(maxY, p.y + 1);
    }
    const pad = Math.min(w, h) * 0.045;
    const sizeX = (w - pad * 2) / (maxX - minX);
    const sizeY = (h - pad * 2) / (maxY - minY + .25);
    this.size = Math.min(sizeX, sizeY);
    // 居中
    const cx = (minX + maxX) / 2;
    const cy = (minY + maxY) / 2;
    this.offX = w / 2 - cx * this.size;
    this.offY = h / 2 - (cy + .08) * this.size;
  }

  render(state: GameState, ui: RenderUI, now: number): void {
    const ctx = this.ctx;
    const w = this.w;
    const h = this.h;
    if (w === 0 || h === 0) return;
    ctx.clearRect(0, 0, w, h);

    // 背景渐变
    const bg = ctx.createLinearGradient(0, 0, 0, h);
    bg.addColorStop(0, '#66B7BB');
    bg.addColorStop(.5, '#4199A6');
    bg.addColorStop(1, '#287C91');
    ctx.fillStyle = bg;
    ctx.fillRect(0, 0, w, h);
    const waterTime = this.reducedMotion.matches ? 0 : now;
    ctx.strokeStyle = 'rgba(213,249,246,.13)';
    ctx.lineWidth = 1;
    for (let row = 0; row < h / 36; row++) {
      ctx.beginPath();
      for (let x = -20; x < w + 20; x += 8) {
        const y = row * 36 + Math.sin(x / 67 + row * 1.7 + waterTime / 3200) * 4;
        if (x === -20) ctx.moveTo(x, y); else ctx.lineTo(x, y);
      }
      ctx.stroke();
    }

    // 清理过期动画
    if (this.moveAnim && now - this.moveAnim.t0 > MOVE_MS) this.moveAnim = null;
    if (this.placeAnim && now - this.placeAnim.t0 > SINK_MS) this.placeAnim = null;
    this.sinkAnims = this.sinkAnims.filter((a) => now - a.t0 < SINK_MS);
    this.departures = this.departures.filter((a) => now - a.t0 < 620);
    this.flushPending(now);

    if (state !== this.mobilityState) {
      this.mobilityState = state;
      this.movable = new Set(state.tiles.flatMap((t, i) => !t.gone && t.owner >= 0 && legalMovesFrom(state, i).length > 0 ? [i] : []));
    }

    const breathe = this.reducedMotion.matches ? .5 : 0.5 + 0.5 * Math.sin(now / 620);

    // 1. 先画碎裂沉没格（冰洞水面）
    state.tiles.forEach((t, i) => {
      if (!t.gone) return;
      const c = this.centerOf(i);
      const anim = this.sinkAnims.find((a) => a.idx === i);
      if (anim && now < anim.t0) this.drawTile(c.x, c.y, tileSeed(t.q, t.r), false, false, breathe);
      else this.drawBrokenIce(c.x, c.y, anim ? now - anim.t0 : -1, tileSeed(t.q, t.r), breathe);
    });

    // 2. 画冰面格
    state.tiles.forEach((t, i) => {
      if (t.gone) return;
      const c = this.centerOf(i);
      const isReach = ui.reachable.has(i);
      const isHover = ui.hover === i && isReach;
      this.drawTile(c.x, c.y, tileSeed(t.q, t.r), isReach, isHover, breathe);
      // 鱼图案
      if (t.owner === -1) this.drawFishes(c.x, c.y, t.fish);
    });

    // 3. 画企鹅
    state.tiles.forEach((t, i) => {
      if (t.gone || t.owner === -1) return;
      // 动画中的企鹅跳过静态绘制
      if (this.moveAnim && this.moveAnim.to === i) return;
      if (this.placeAnim && this.placeAnim.to === i) return;
      const c = this.centerOf(i);
      const isTurn = state.phase !== 'finished' && state.turn === t.owner;
      if (i === ui.selected) this.drawSelectionRing(c.x, c.y, breathe);
      else if (isTurn && this.movable.has(i)) this.drawTurnRing(c.x, c.y, breathe, t.owner);
      const bob = !this.reducedMotion.matches && this.movable.has(i) ? Math.sin(now / 850 + i) * this.size * .015 : 0;
      this.drawPenguin(c.x, c.y + bob, t.owner);
      if (state.phase === 'moving' && !this.movable.has(i)) {
        ctx.fillStyle = '#527A87';
        ctx.beginPath(); ctx.arc(c.x + this.size * .48, c.y + this.size * .43, this.size * .16, 0, Math.PI * 2); ctx.fill();
        ctx.fillStyle = '#FFFFFF';
        ctx.fillRect(c.x + this.size * .42, c.y + this.size * .35, this.size * .04, this.size * .15);
        ctx.fillRect(c.x + this.size * .50, c.y + this.size * .35, this.size * .04, this.size * .15);
      }
    });

    for (const departure of this.departures) {
      if (now < departure.t0) continue;
      const t = Math.min(1, (now - departure.t0) / 620);
      const c = this.centerOf(departure.idx);
      ctx.save();
      ctx.globalAlpha = 1 - t * t;
      ctx.translate(c.x, c.y + t * this.size * .4);
      ctx.scale(1 - t * .4, 1 - t * .4);
      this.drawPenguin(0, 0, departure.player);
      ctx.restore();
    }

    // 4. 动画中的企鹅
    if (this.moveAnim) {
      const a = this.moveAnim;
      const t = Math.min(1, (now - a.t0) / MOVE_MS);
      const ease = t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2;
      // 轻微跳跃弧线
      const from = this.centerOf(a.from);
      const to = this.centerOf(a.to);
      const x = from.x + (to.x - from.x) * ease;
      const y = from.y + (to.y - from.y) * ease - Math.sin(t * Math.PI) * this.size * 0.48;
      ctx.save();
      ctx.translate(x, y);
      ctx.rotate(Math.sin(t * Math.PI * 2) * .08 * Math.sign(to.x - from.x));
      ctx.translate(-x, -y);
      this.drawPenguin(x, y, a.player);
      ctx.restore();
    }
    if (this.placeAnim) {
      const a = this.placeAnim;
      const t = Math.min(1, (now - a.t0) / SINK_MS);
      const scale = 0.3 + 0.7 * this.backOut(t);
      const c = this.centerOf(a.to);
      this.ctx.save();
      this.ctx.translate(c.x, c.y);
      this.ctx.scale(scale, scale);
      this.ctx.translate(-c.x, -c.y);
      if (state.tiles[a.to].owner >= 0) this.drawPenguin(c.x, c.y, state.tiles[a.to].owner);
      this.ctx.restore();
    }
  }

  private backOut(t: number): number {
    const c = 1.70158;
    return 1 + (c + 1) * Math.pow(t - 1, 3) + c * Math.pow(t - 1, 2);
  }

  private drawTile(x: number, y: number, seed: number, reach: boolean, hover: boolean, breathe: number): void {
    const ctx = this.ctx;
    const s = this.size;
    const pts = hexCorners(x, y, s * 0.96);
    ctx.save();
    ctx.drawImage(this.ice, x - s * 1.14, y - s * 1.087, s * 2.28, s * 2.28);
    ctx.beginPath();
    pts.forEach(([px, py], i) => (i === 0 ? ctx.moveTo(px, py) : ctx.lineTo(px, py)));
    ctx.closePath();
    if (reach) {
      ctx.fillStyle = hover ? 'rgba(255,216,117,.55)' : 'rgba(255,237,168,.25)';
      ctx.fill();
      ctx.lineWidth = hover ? 3 : 2;
      ctx.strokeStyle = hover ? '#FFE39A' : `rgba(232,186,82,${.55 + breathe * .3})`;
      ctx.stroke();
    }
    if (seed % 4 === 0) this.drawIceCrack(x, y, s, seed);
    ctx.restore();
  }

  /** 冰面上的天然细裂纹（装饰，折线一条） */
  private drawIceCrack(x: number, y: number, s: number, seed: number): void {
    const ctx = this.ctx;
    const a = ((seed >> 3) % 6) * (Math.PI / 3) + 0.3;
    const pt = (ang: number, d: number): [number, number] => [x + Math.cos(ang) * s * d, y + Math.sin(ang) * s * d];
    ctx.beginPath();
    ctx.moveTo(...pt(a, 0.82));
    ctx.lineTo(...pt(a + 0.4, 0.48));
    ctx.lineTo(...pt(a - 0.2, 0.22));
    ctx.strokeStyle = PALETTE.crack;
    ctx.lineWidth = 1;
    ctx.stroke();
  }

  /** 碎裂沉没格：深水冰洞 + 边缘残冰碎块；sinkAge ≥ 0 时播放碎裂动画 */
  private drawBrokenIce(x: number, y: number, sinkAge: number, seed: number, breathe: number): void {
    const ctx = this.ctx;
    const s = this.size;
    const pts = hexCorners(x, y, s * 0.92);
    ctx.save();
    // 水洞与周围海面连通，不再绘制像完整冰格一样的边框。
    ctx.beginPath();
    pts.forEach(([px, py], i) => (i === 0 ? ctx.moveTo(px, py) : ctx.lineTo(px, py)));
    ctx.closePath();
    const wg = ctx.createRadialGradient(x, y, s * 0.1, x, y, s * 0.95);
    wg.addColorStop(0, 'rgba(15,77,98,.30)');
    wg.addColorStop(1, 'rgba(33,116,139,0)');
    ctx.fillStyle = wg;
    ctx.fill();
    // 2. 洞缘残冰碎块（交替角上的锯齿碎冰，恒定）
    for (let k = 0; k < 6; k++) {
      if ((seed + k) % 2 !== 0) continue;
      const c1 = pts[k];
      const c2 = pts[(k + 1) % 6];
      const c0 = pts[(k + 5) % 6];
      const in1: [number, number] = [c1[0] + (c2[0] - c1[0]) * .10, c1[1] + (c2[1] - c1[1]) * .10];
      const in2: [number, number] = [x + (c1[0] - x) * .85, y + (c1[1] - y) * .85];
      const in3: [number, number] = [c1[0] + (c0[0] - c1[0]) * .12, c1[1] + (c0[1] - c1[1]) * .12];
      ctx.beginPath();
      ctx.moveTo(c1[0], c1[1]);
      ctx.lineTo(in1[0], in1[1]);
      ctx.lineTo(in2[0], in2[1]);
      ctx.lineTo(in3[0], in3[1]);
      ctx.closePath();
      ctx.fillStyle = PALETTE.shard;
      ctx.globalAlpha = 0.45;
      ctx.fill();
      ctx.strokeStyle = PALETTE.shardEdge;
      ctx.lineWidth = 0.8;
      ctx.stroke();
      ctx.globalAlpha = 1;
    }
    // 3. 水面漂浮小冰粒（轻微起伏）
    for (let b = 0; b < 2; b++) {
      const ang = ((seed >> (4 + b * 3)) % 12) / 12 * Math.PI * 2;
      const dist = s * (0.28 + ((seed >> (6 + b * 5)) % 10) / 10 * 0.3);
      const bob = Math.sin(breathe * Math.PI * 2 + b * 2.1 + (seed % 7)) * s * 0.03;
      const bx = x + Math.cos(ang) * dist;
      const by = y + Math.sin(ang) * dist + bob;
      ctx.save();
      ctx.translate(bx, by);
      ctx.rotate(ang);
      ctx.globalAlpha = 0.75;
      ctx.fillStyle = PALETTE.shard;
      ctx.beginPath();
      ctx.moveTo(0, -s * 0.07);
      ctx.lineTo(s * 0.055, 0);
      ctx.lineTo(0, s * 0.07);
      ctx.lineTo(-s * 0.055, 0);
      ctx.closePath();
      ctx.fill();
      ctx.restore();
    }
    // 4. 静态涟漪
    ctx.globalAlpha = 0.12 + 0.06 * breathe;
    ctx.beginPath();
    ctx.arc(x, y, s * (0.34 + 0.05 * breathe), 0, Math.PI * 2);
    ctx.strokeStyle = '#FFFFFF';
    ctx.lineWidth = 1.2;
    ctx.stroke();
    ctx.globalAlpha = 1;
    // 5. 碎裂动画：放射裂缝 → 六块冰楔外漂旋转下沉
    if (sinkAge >= 0 && sinkAge < SINK_MS) {
      const t = sinkAge / SINK_MS;
      const crackA = Math.min(1, t * 4);
      // 先保留完整冰面，裂开后六块冰楔才从原格中心分离。
      if (t < .22) {
        ctx.drawImage(this.ice, x - s * 1.14, y - s * 1.087, s * 2.28, s * 2.28);
      }
      // 放射裂缝
      ctx.strokeStyle = '#418DA5';
      ctx.lineWidth = 1.6;
      ctx.globalAlpha = crackA * 0.9;
      for (const [px, py] of pts) {
        ctx.beginPath();
        ctx.moveTo(x + (px - x) * 0.15, y + (py - y) * 0.15);
        ctx.lineTo(x + (px - x) * 0.92, y + (py - y) * 0.92);
        ctx.stroke();
      }
      // 冰楔分离
      const t2 = Math.max(0, (t - 0.22) / 0.78);
      const ease = 1 - (1 - t2) * (1 - t2);
      if (ease > 0) {
        for (let i = 0; i < 6; i++) {
          const a = pts[i];
          const b = pts[(i + 1) % 6];
          const cx = (a[0] + b[0]) / 2;
          const cy = (a[1] + b[1]) / 2;
          const dx = cx - x;
          const dy = cy - y;
          const len = Math.hypot(dx, dy) || 1;
          const rot = (i % 2 === 0 ? 0.55 : -0.55) * ease;
          ctx.save();
          ctx.translate(cx + (dx / len) * ease * s * .22, cy + (dy / len) * ease * s * .22 + ease * s * .12);
          ctx.rotate(rot);
          ctx.translate(-cx, -cy);
          ctx.globalAlpha = 1 - ease;
          ctx.beginPath();
          ctx.moveTo(x, y);
          ctx.lineTo(a[0], a[1]);
          ctx.lineTo(b[0], b[1]);
          ctx.closePath();
          ctx.fillStyle = i % 2 ? '#C2EAF0' : '#EEFCFE';
          ctx.fill();
          ctx.strokeStyle = '#FFFFFF';
          ctx.lineWidth = 1;
          ctx.stroke();
          ctx.restore();
        }
      }
      // 溅落涟漪
      ctx.globalAlpha = (1 - t) * 0.55;
      ctx.beginPath();
      ctx.ellipse(x, y + s * .12, s * (0.35 + t * .85), s * (.2 + t * .65), 0, 0, Math.PI * 2);
      ctx.strokeStyle = '#FFFFFF';
      ctx.lineWidth = 2;
      ctx.stroke();
      ctx.globalAlpha = 1;
    }
    ctx.restore();
  }

  private drawFishes(x: number, y: number, fish: number): void {
    const s = this.size;
    const layout: Array<[number, number]> =
      fish === 1
        ? [[0, 0]]
        : fish === 2
          ? [[-0.24, 0.06], [0.24, -0.06]]
          : [[0, -0.2], [-0.26, 0.16], [0.26, 0.16]];
    for (const [dx, dy] of layout) {
      this.drawFish(x + dx * s, y + dy * s, s * 0.20);
    }
  }

  private drawFish(x: number, y: number, r: number): void {
    const ctx = this.ctx;
    ctx.save();
    ctx.translate(x, y);
    ctx.rotate(-0.3);
    // 身体
    ctx.beginPath();
    ctx.ellipse(0, 0, r * 1.15, r * 0.6, 0, 0, Math.PI * 2);
    ctx.fillStyle = PALETTE.fish;
    ctx.fill();
    // 尾巴
    ctx.beginPath();
    ctx.moveTo(r * 1.0, 0);
    ctx.lineTo(r * 1.55, -r * 0.5);
    ctx.lineTo(r * 1.55, r * 0.5);
    ctx.closePath();
    ctx.fill();
    // 眼睛
    ctx.beginPath();
    ctx.arc(-r * 0.5, -r * 0.1, r * 0.14, 0, Math.PI * 2);
    ctx.fillStyle = PALETTE.fishDark;
    ctx.fill();
    ctx.restore();
  }

  drawPenguin(x: number, y: number, player: number): void {
    const s = this.size;
    const sprite = this.penguins[player % 2];
    if (sprite) this.ctx.drawImage(sprite, x - s * .94, y - s * 1.04, s * 1.88, s * 1.88);
  }

  private drawSelectionRing(x: number, y: number, breathe: number): void {
    const ctx = this.ctx;
    const s = this.size;
    ctx.save();
    ctx.beginPath();
    ctx.ellipse(x, y + s * 0.1, s * (0.62 + 0.05 * breathe), s * (0.68 + 0.05 * breathe), 0, 0, Math.PI * 2);
    ctx.strokeStyle = PALETTE.glow;
    ctx.lineWidth = 3;
    ctx.globalAlpha = 0.55 + 0.45 * breathe;
    ctx.stroke();
    ctx.restore();
  }

  private drawTurnRing(x: number, y: number, breathe: number, _player: number): void {
    const ctx = this.ctx;
    const s = this.size;
    ctx.save();
    ctx.globalAlpha = 0.25 + 0.3 * breathe;
    ctx.beginPath();
    ctx.ellipse(x, y + s * 0.1, s * 0.58, s * 0.64, 0, 0, Math.PI * 2);
    ctx.strokeStyle = PALETTE.ring;
    ctx.lineWidth = 2.5;
    ctx.stroke();
    ctx.restore();
  }
}
