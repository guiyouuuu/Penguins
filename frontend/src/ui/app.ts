/** 应用层：菜单 / 本地对弈 / 人机 / 联机的状态管理与交互 */
import { applyMove, createGame, legalMovesFrom, legalPlacements } from '../core/game';
import type { GameState, Move, ServerMsg } from '../core/types';
import { NetClient } from '../net/client';
import { BoardRenderer } from '../render/renderer';
import { AuthUI } from './auth';

type Mode = 'menu' | 'local' | 'ai' | 'online';

const $ = <T extends HTMLElement>(id: string): T => {
  const el = document.getElementById(id);
  if (!el) throw new Error(`缺少元素 #${id}`);
  return el as T;
};

export class App {
  private mode: Mode = 'menu';
  private state: GameState | null = null;
  private mySeat = 0;
  private selected = -1;
  private reachable = new Set<number>();
  private hover = -1;
  private busy = false; // 网络等待 / 动画锁
  private history: GameState[] = [];
  private rematchReady = false;
  private queuedStates: ServerMsg[] = [];
  private gameOverTimer = 0;

  private renderer: BoardRenderer;
  private net = new NetClient();
  private canvas: HTMLCanvasElement;
  private auth = new AuthUI();

  constructor() {
    this.canvas = $('board');
    this.renderer = new BoardRenderer(this.canvas);
    this.bindNet();
    this.bindMenu();
    this.bindBoard();
    this.bindTopActions();
    // 登录/退出后重连，让 WebSocket 携带最新身份
    this.auth.onAuthChange = () => this.net.close();
    window.addEventListener('resize', () => {
      if (this.state) this.renderer.resize(this.state);
    });
    requestAnimationFrame(this.loop);
  }

  private loop = (now: number): void => {
    if (!this.renderer.isAnimating && this.queuedStates.length > 0) {
      this.handleServerMsg(this.queuedStates.shift()!, true);
    }
    if (this.state && this.mode !== 'menu') {
      this.renderer.render(this.state, { selected: this.selected, reachable: this.reachable, hover: this.hover, mySeat: this.mySeat }, now);
    }
    requestAnimationFrame(this.loop);
  };

  // ---------- 菜单 ----------

  private bindMenu(): void {
    $('btn-local').addEventListener('click', () => this.startLocal());
    $('btn-ai').addEventListener('click', () => this.startNet('ai'));
    $('btn-create').addEventListener('click', () => this.startNet('create'));
    $('btn-match').addEventListener('click', () => this.startNet('match'));
    $('btn-join').addEventListener('click', () => {
      $('join-row').classList.toggle('hidden');
      $('room-input').focus();
    });
    $('btn-join-confirm').addEventListener('click', () => {
      const room = $<HTMLInputElement>('room-input').value.trim().toUpperCase();
      if (room.length < 4) {
        this.menuTip('房间号至少 4 位');
        return;
      }
      this.startNet('join', room);
    });
    $('room-input').addEventListener('keydown', (e) => {
      if (e.key === 'Enter') $('btn-join-confirm').click();
    });
  }

  private menuTip(text: string): void {
    const tip = $('menu-tip');
    tip.textContent = text;
    tip.classList.remove('hidden');
  }

  private async startNet(action: 'ai' | 'create' | 'match' | 'join', room = ''): Promise<void> {
    this.menuTip('');
    try {
      if (!this.net.connected) {
        this.menuTip('正在连接服务器…');
        await this.net.connect();
      }
    } catch {
      this.menuTip('无法连接服务器，请确认后端已启动');
      return;
    }
    if (action === 'ai') {
      this.setNetMode('ai');
      this.net.send({ type: 'play_ai' });
    } else {
      this.setNetMode('online');
      if (action === 'create') this.net.send({ type: 'create_room' });
      else if (action === 'match') this.net.send({ type: 'quick_match' });
      else this.net.send({ type: 'join_room', room });
    }
  }

  // ---------- 对局生命周期 ----------

  private startLocal(): void {
    this.mode = 'local';
    this.mySeat = 0;
    this.history = [];
    this.busy = false;
    this.state = createGame(['玩家一', '玩家二']);
    this.resetSelection();
    this.enterGame('本地对弈', false);
  }

  private enterGame(subtitle: string, _online: boolean): void {
    this.renderer.resetAnimations();
    this.queuedStates = [];
    window.clearTimeout(this.gameOverTimer);
    $('toast').classList.add('hidden');
    $('user-bar').classList.add('hidden');
    $('menu-screen').classList.add('hidden');
    $('game-screen').classList.remove('hidden');
    $('overlay').classList.add('hidden');
    $('game-subtitle').textContent = subtitle;
    $('btn-resign').classList.toggle('hidden', this.mode === 'local');
    $('btn-undo').classList.toggle('hidden', this.mode !== 'local');
    this.renderer.resize(this.state!);
    this.refreshHUD();
    requestAnimationFrame(() => this.renderer.resize(this.state!));
  }

  private backToMenu(): void {
    this.renderer.resetAnimations();
    this.queuedStates = [];
    window.clearTimeout(this.gameOverTimer);
    $('toast').classList.add('hidden');
    $('user-bar').classList.remove('hidden');
    this.net.send({ type: 'leave' });
    this.net.close();
    this.mode = 'menu';
    this.state = null;
    this.resetSelection();
    $('game-screen').classList.add('hidden');
    $('overlay').classList.add('hidden');
    $('menu-screen').classList.remove('hidden');
    $('join-row').classList.add('hidden');
    this.menuTip('');
    void this.auth.refresh(); // 对局结束，刷新战绩与 Elo
  }

  private resetSelection(): void {
    this.selected = -1;
    this.reachable = new Set(this.state?.phase === 'placing' && this.myTurn ? legalPlacements(this.state) : []);
    this.hover = -1;
  }

  // ---------- 棋盘交互 ----------

  private bindBoard(): void {
    this.canvas.addEventListener('click', (e) => {
      if (!this.state || this.busy || this.renderer.isAnimating || this.state.phase === 'finished') return;
      const rect = this.canvas.getBoundingClientRect();
      const idx = this.renderer.cellAt(e.clientX - rect.left, e.clientY - rect.top);
      if (idx < 0) return;
      this.onCellClick(idx);
    });
    this.canvas.addEventListener('mousemove', (e) => {
      if (!this.state) return;
      const rect = this.canvas.getBoundingClientRect();
      this.hover = this.renderer.cellAt(e.clientX - rect.left, e.clientY - rect.top);
    });
    this.canvas.addEventListener('mouseleave', () => {
      this.hover = -1;
    });
  }

  /** 是否轮到本地操作（本地双人始终可操作） */
  private get myTurn(): boolean {
    if (!this.state) return false;
    if (this.mode === 'local') return true;
    return this.state.turn === this.mySeat;
  }

  private onCellClick(idx: number): void {
    const st = this.state!;
    if (!this.myTurn) return;
    if (st.phase === 'placing') {
      if (!legalPlacements(st).includes(idx)) return;
      this.commit({ kind: 'place', player: st.turn, from: -1, to: idx });
      return;
    }
    if (st.phase !== 'moving') return;
    const tile = st.tiles[idx];
    // 点击自己（当前玩家）的企鹅：选中
    if (tile.owner === st.turn) {
      const targets = legalMovesFrom(st, idx);
      if (targets.length === 0) {
        this.resetSelection();
        this.showToast('这只企鹅已被困，当前玩家的其他企鹅仍可行动');
        return;
      }
      if (this.selected === idx) {
        this.resetSelection();
      } else {
        this.selected = idx;
        this.reachable = new Set(targets);
      }
      return;
    }
    // 点击可达格：移动
    if (this.reachable.has(idx)) {
      this.commit({ kind: 'move', player: st.turn, from: this.selected, to: idx });
      return;
    }
    this.resetSelection();
  }

  /** 提交一步棋：本地直接应用；联机/人机发给服务器 */
  private commit(mv: Move): void {
    const st = this.state!;
    if (this.mode === 'local') {
      const next = applyMove(st, mv);
      if (!next) return;
      this.history.push(st);
      this.state = next;
      this.renderer.playMove(mv);
      this.onStateArrived(st, next, mv);
      this.resetSelection();
      this.refreshHUD();
      if (next.phase === 'finished') this.scheduleGameOver(next.winner);
      return;
    }
    // 网络 / 人机
    this.busy = true;
    this.resetSelection();
    if (mv.kind === 'place') this.net.send({ type: 'place', tile: mv.to });
    else this.net.send({ type: 'move', from: mv.from, to: mv.to });
    this.refreshHUD();
  }

  /** 新状态到达：对比上一状态，触发碎裂动画与出局提示 */
  private onStateArrived(prev: GameState, next: GameState, move?: Move | null): void {
    // 新沉没的格子 → 碎裂动画
    const sinks: number[] = [];
    next.tiles.forEach((t, i) => {
      if (t.gone && !prev.tiles[i].gone) sinks.push(i);
    });
    if (sinks.length > 0) this.renderer.playSinks(sinks);
    // 按单只企鹅比较，玩家尚未出局时也播放孤岛掉落。
    next.players.forEach((p, i) => {
      const positions = prev.players[i].penguins.flatMap((idx, slot) => {
        if (idx < 0 || p.penguins[slot] >= 0) return [];
        return [move?.kind === 'move' && move.player === i && move.from === idx ? move.to : idx];
      });
      if (move?.kind === 'place' && move.player === i && next.tiles[move.to].gone) positions.push(move.to);
      if (positions.length > 0) this.renderer.playDepartures(positions.map(idx => ({ idx, player: i })));
      if (p.stuck && !prev.players[i].stuck) {
        this.showToast(`${p.name} 已无路可走，企鹅全部离场${next.phase === 'finished' ? '' : `，${next.players[next.turn].name} 继续行动`}`);
      } else if (positions.length > 0) {
        this.showToast(`${p.name} 的 ${positions.length} 只企鹅随孤立冰块掉落`);
      }
    });
  }

  private toastTimer = 0;

  private showToast(text: string): void {
    const el = $('toast');
    el.textContent = text;
    el.classList.remove('hidden');
    window.clearTimeout(this.toastTimer);
    this.toastTimer = window.setTimeout(() => el.classList.add('hidden'), 3000);
  }

  // ---------- 网络消息 ----------

  handleServerMsg(msg: ServerMsg, fromQueue = false): void {
    if ((msg.type === 'state' || msg.type === 'over') && (this.renderer.isAnimating || (!fromQueue && this.queuedStates.length > 0))) {
      this.queuedStates.push(msg);
      return;
    }
    switch (msg.type) {
      case 'room':
        this.mySeat = msg.you;
        this.menuTip(`房间已创建：${msg.room}（等待对手加入…）`);
        break;
      case 'waiting':
        this.menuTip('正在等待对手…');
        break;
      case 'start':
        this.mode = this.netMode;
        this.state = msg.state;
        this.history = [];
        this.busy = false;
        this.rematchReady = false;
        this.resetSelection();
        this.enterGame(this.netMode === 'ai' ? '人机对战' : '联机对战', true);
        break;
      case 'state': {
        this.busy = false;
        if (msg.last) this.renderer.playMove(msg.last);
        const prev = this.state;
        this.state = msg.state;
        if (prev) this.onStateArrived(prev, msg.state, msg.last);
        this.resetSelection();
        this.refreshHUD();
        break;
      }
      case 'over': {
        this.busy = false;
        if (msg.last) this.renderer.playMove(msg.last);
        const prev = this.state;
        this.state = msg.state;
        if (prev) this.onStateArrived(prev, msg.state, msg.last);
        this.resetSelection();
        this.refreshHUD();
        this.scheduleGameOver(msg.winner);
        break;
      }
      case 'opponent_left':
        this.showOverlay('🐧💨', '对手离开了', '对方已退出房间，这局算你赢啦～', [
          { label: '返回菜单', action: () => this.backToMenu() },
        ]);
        break;
      case 'rematch_ask':
        this.refreshHUD();
        if (!this.rematchReady) {
          this.showOverlay('🔁', '对手想再来一局', '点击「再来一局」开始新的对局', [
            { label: '再来一局', action: () => this.askRematch() },
            { label: '返回菜单', action: () => this.backToMenu() },
          ]);
        }
        break;
      case 'error':
        this.busy = false;
        if (this.mode === 'menu') this.menuTip(msg.msg);
        else {
          this.showToast(msg.msg);
          this.refreshHUD();
        }
        break;
    }
  }

  private netMode: 'ai' | 'online' = 'online';

  setNetMode(mode: 'ai' | 'online'): void {
    this.netMode = mode;
  }

  bindNet(): void {
    this.net.onMessage = (msg) => this.handleServerMsg(msg);
    this.net.onClose = () => {
      if (this.mode === 'online' || this.mode === 'ai') {
        this.showOverlay('📡', '连接已断开', '与服务器的连接中断了', [
          { label: '返回菜单', action: () => this.backToMenu() },
        ]);
      }
    };
  }

  private askRematch(): void {
    this.rematchReady = true;
    this.net.send({ type: 'rematch' });
    $('overlay').classList.add('hidden');
    if (this.mode === 'ai') return;
    // online：等待对手确认
    this.showOverlay('⏳', '等待对手…', '对手同意后将自动开始新对局', [
      { label: '返回菜单', action: () => this.backToMenu() },
    ]);
  }

  // ---------- HUD ----------

  private refreshHUD(): void {
    const st = this.state;
    if (!st) return;
    const board = $('scoreboard');
    board.innerHTML = '';
    st.players.forEach((p) => {
      const el = document.createElement('div');
      el.className =
        'score-chip' +
        (st.phase !== 'finished' && st.turn === p.id ? ' active' : '') +
        (p.id === this.mySeat && this.mode !== 'local' ? ' me' : '') +
        (p.stuck ? ' out' : '');
      const dot = document.createElement('span');
      dot.className = 'chip-dot d' + p.id;
      const name = document.createElement('span');
      name.textContent = p.stuck ? `${p.name} · 已结算` : p.name;
      const score = document.createElement('strong');
      score.textContent = `🐟 ${p.score}`;
      el.append(dot, name, score);
      board.appendChild(el);
    });
    // 状态卡
    const card = $('status-card');
    const lines: string[] = [];
    if (st.phase === 'placing') {
      lines.push(st.players[st.turn].name + ' · 放置企鹅');
    } else if (st.phase === 'moving') {
      lines.push('轮到 ' + st.players[st.turn].name);
      const out = st.players.filter((p) => p.stuck).map((p) => p.name);
      if (out.length > 0) lines.push(`${out.join('、')} 已无路可走 · ${st.players[st.turn].name} 继续行动`);
    } else {
      lines.push('对局结束');
    }
    if (this.mode === 'online' || this.mode === 'ai') {
      if (this.busy) lines.push('走子确认中…');
      else if (st.phase !== 'finished' && !this.myTurn) lines.push('等待对方走子…');
      else if (st.phase !== 'finished') lines.push('你的回合');
    }
    card.replaceChildren(...lines.map((line) => {
      const row = document.createElement('div');
      row.textContent = line;
      return row;
    }));
  }

  // ---------- 结算 / 弹窗 ----------

  private scheduleGameOver(winner: number): void {
    window.clearTimeout(this.gameOverTimer);
    this.gameOverTimer = window.setTimeout(() => {
      if (this.renderer.isAnimating) {
        this.scheduleGameOver(winner);
      } else if (this.state?.phase === 'finished') {
        this.showGameOver(winner);
      }
    }, 120);
  }

  private showGameOver(winner: number): void {
    const st = this.state!;
    let title: string;
    let body: string;
    if (winner === -1) {
      title = '平局！';
      body = `双方同分：${st.players[0].score} 🐟`;
    } else {
      const w = st.players[winner];
      title = w.name + ' 获胜！';
      body = `🐟 ${st.players[0].name} ${st.players[0].score} : ${st.players[1].score} ${st.players[1].name}`;
    }
    const actions = [
      {
        label: '再来一局',
        action: () => {
          if (this.mode === 'local') {
            this.startLocal();
          } else {
            this.askRematch();
          }
        },
      },
      { label: '返回菜单', action: () => this.backToMenu() },
    ];
    this.showOverlay('🏆', title, body, actions);
  }

  private showOverlay(icon: string, title: string, body: string, actions: Array<{ label: string; action: () => void }>): void {
    $('overlay-icon').textContent = icon;
    $('overlay-title').textContent = title;
    $('overlay-body').textContent = body;
    const box = $('overlay-actions');
    box.innerHTML = '';
    actions.forEach(({ label, action }) => {
      const btn = document.createElement('button');
      btn.className = 'btn' + (label.includes('再来') ? ' primary' : '');
      btn.textContent = label;
      btn.addEventListener('click', action);
      box.appendChild(btn);
    });
    $('overlay').classList.remove('hidden');
  }

  // ---------- 顶部操作 ----------

  private bindTopActions(): void {
    $('btn-exit').addEventListener('click', () => {
      if (this.state && this.state.phase !== 'finished' && this.mode !== 'menu') {
        this.showOverlay('🤔', '确定离开吗？', '离开将结束当前对局', [
          { label: '继续对局', action: () => $('overlay').classList.add('hidden') },
          { label: '确定离开', action: () => this.backToMenu() },
        ]);
      } else {
        this.backToMenu();
      }
    });
    $('btn-resign').addEventListener('click', () => {
      if (this.mode === 'local') return;
      this.net.send({ type: 'resign' });
    });
    $('btn-undo').addEventListener('click', () => {
      if (this.mode !== 'local' || this.history.length === 0) return;
      this.state = this.history.pop()!;
      this.renderer.resetAnimations();
      window.clearTimeout(this.gameOverTimer);
      $('overlay').classList.add('hidden');
      this.resetSelection();
      this.refreshHUD();
    });
  }
}
