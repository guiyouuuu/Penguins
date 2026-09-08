/** 应用层：菜单 / 本地对弈 / 人机 / 联机的状态管理与交互 */
import { applyMove, createGame, legalMovesFrom, legalPlacements } from '../core/game';
import type { GameState, Move, ServerMsg } from '../core/types';
import { NetClient } from '../net/client';
import { BoardRenderer } from '../render/renderer';
import { AuthUI } from './auth';

type Mode = 'menu' | 'local' | 'ai' | 'online';
type NetAction = 'ai' | 'create' | 'match' | 'join';
const ROOM_CODE = /^[ABCDEFGHJKMNPQRSTUVWXYZ23456789]{4}$/;

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
  private lobbyAction: NetAction | null = null;
  private lobbyRoom = '';
  private netRequest = 0;
  private lobbyNames: [string, string] = ['', ''];
  private guestReady = false;
  private lobbyPending = false;
  private loginPending = false;

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
    this.auth.onAuthChange = () => {
      if (this.lobbyAction) this.cancelWaiting('登录状态已更新，请重新进入房间');
      else this.net.close();
    };
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
      if (this.lobbyAction) return;
      $('join-row').classList.toggle('hidden');
      $('room-input').focus();
    });
    $('btn-join-confirm').addEventListener('click', () => {
      const room = $<HTMLInputElement>('room-input').value.trim().toUpperCase();
      if (!ROOM_CODE.test(room)) {
        this.menuTip('请输入有效的 4 位房间号');
        return;
      }
      this.startNet('join', room);
    });
    $('room-input').addEventListener('keydown', (e) => {
      if (e.key === 'Enter') $('btn-join-confirm').click();
    });
    $('btn-cancel-wait').addEventListener('click', () => this.cancelWaiting(this.lobbyRoom ? '已退出房间' : '已取消等待'));
    $('btn-copy-room').addEventListener('click', () => this.copyInvitation(false));
    $('btn-copy-invite').addEventListener('click', () => this.copyInvitation(true));
    $('btn-start-game').addEventListener('click', () => {
      if (this.lobbyPending || this.mySeat !== 0 || !this.guestReady) return;
      this.lobbyPending = true;
      this.renderLobby();
      this.net.send({ type: 'start_game' });
    });
    $('guest-ready').addEventListener('change', () => {
      if (this.lobbyPending || this.mySeat !== 1) return;
      const ready = $<HTMLInputElement>('guest-ready').checked;
      this.lobbyPending = true;
      this.renderLobby();
      this.net.send({ type: 'ready', ready });
    });
    const invitedRoom = new URL(location.href).searchParams.get('room')?.trim().toUpperCase();
    if (invitedRoom && ROOM_CODE.test(invitedRoom)) {
      $<HTMLInputElement>('room-input').value = invitedRoom;
      $('join-row').classList.remove('hidden');
    }
  }

  private menuTip(text: string): void {
    const tip = $('menu-tip');
    tip.textContent = text;
    tip.classList.toggle('hidden', !text);
  }

  private renderLobby(): void {
    const waiting = this.lobbyAction !== null;
    for (const id of ['btn-local', 'btn-ai', 'btn-create', 'btn-match', 'btn-join', 'btn-join-confirm', 'room-input']) {
      $<HTMLButtonElement | HTMLInputElement>(id).disabled = waiting;
    }
    // 等待时收起模式入口，移动端也能完整显示房间和邀请操作。
    $('menu-screen').classList.toggle('waiting', waiting);
    $('room-lobby').classList.toggle('hidden', !waiting);
    $('room-share').classList.toggle('hidden', !this.lobbyRoom);
    $<HTMLInputElement>('room-code').value = this.lobbyRoom;
    const inRoom = waiting && !!this.lobbyRoom && !!this.lobbyNames[0];
    $('lobby-players').classList.toggle('hidden', !inRoom);
    $('lobby-host').textContent = this.lobbyNames[0];
    $('lobby-guest').textContent = this.lobbyNames[1] || '空位';
    $('lobby-guest-status').textContent = !this.lobbyNames[1] ? '等待加入' : this.guestReady ? '已准备' : '未准备';
    $('ready-control').classList.toggle('hidden', !inRoom || this.mySeat !== 1);
    const ready = $<HTMLInputElement>('guest-ready');
    if (!this.lobbyPending) ready.checked = this.guestReady;
    ready.disabled = this.lobbyPending;
    const start = $<HTMLButtonElement>('btn-start-game');
    start.classList.toggle('hidden', !inRoom || this.mySeat !== 0);
    start.disabled = !this.lobbyNames[1] || !this.guestReady || this.lobbyPending;
    start.textContent = this.lobbyPending ? '正在开始…' : '开始游戏';
    $('invite-actions').classList.toggle('hidden', this.mySeat !== 0);
    $('btn-cancel-wait').textContent = !this.lobbyRoom ? '取消等待' : this.mySeat === 0 ? '关闭房间' : '退出房间';
    if (!waiting) $('invite-fallback').classList.add('hidden');
  }

  private cancelWaiting(message: string): void {
    ++this.netRequest;
    this.net.close();
    this.lobbyAction = null;
    this.lobbyRoom = '';
    this.lobbyNames = ['', ''];
    this.guestReady = false;
    this.lobbyPending = false;
    this.renderLobby();
    this.menuTip(message);
  }

  private async copyInvitation(link: boolean): Promise<void> {
    if (!this.lobbyRoom) return;
    const request = this.netRequest;
    const url = new URL(location.pathname, location.origin);
    url.searchParams.set('room', this.lobbyRoom);
    const text = link ? url.href : this.lobbyRoom;
    try {
      await navigator.clipboard.writeText(text);
      if (request === this.netRequest) this.menuTip(link ? '邀请链接已复制' : '房间号已复制');
    } catch {
      if (request !== this.netRequest) return;
      const input = $<HTMLInputElement>(link ? 'invite-link' : 'room-code');
      if (link) {
        input.value = text;
        $('invite-fallback').classList.remove('hidden');
      }
      input.focus();
      input.select();
      this.menuTip('自动复制失败，请手动复制已选中的内容');
    }
  }

  private async startNet(action: NetAction, room = '', requireLogin = false): Promise<void> {
    if (this.mode !== 'menu' || this.lobbyAction || this.loginPending) return;
    const request = ++this.netRequest;
    if (action !== 'ai' && (!this.auth.currentUser || requireLogin)) {
      this.loginPending = true;
      const loggedIn = await this.auth.requestLogin(requireLogin);
      this.loginPending = false;
      if (!loggedIn || request !== this.netRequest) return;
    }
    this.lobbyAction = action;
    this.lobbyRoom = '';
    this.lobbyNames = ['', ''];
    this.guestReady = false;
    this.lobbyPending = false;
    this.renderLobby();
    this.menuTip('');
    $('lobby-status').textContent = '正在连接服务器…';
    try {
      if (!this.net.connected) {
        await this.net.connect();
      }
    } catch {
      if (request === this.netRequest) this.cancelWaiting('无法连接服务器，请稍后重试');
      return;
    }
    if (request !== this.netRequest) return;
    $('lobby-status').textContent = action === 'create' ? '正在创建房间…'
      : action === 'join' ? '正在加入房间…' : action === 'match' ? '正在匹配对手…' : '正在准备人机对局…';
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
    if (this.lobbyAction) return;
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
    ++this.netRequest;
    this.lobbyAction = null;
    this.lobbyRoom = '';
    this.renderLobby();
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
        if (this.lobbyAction === 'create' || this.lobbyAction === 'join') {
          this.lobbyRoom = msg.room;
          this.renderLobby();
          $('lobby-status').textContent = msg.you === 0 ? '房间已创建，等待好友加入' : '正在同步房间…';
        }
        break;
      case 'waiting':
        $('lobby-status').textContent = this.lobbyRoom ? '等待好友加入' : '正在匹配对手…';
        break;
      case 'lobby':
        if (!this.lobbyAction || msg.room !== this.lobbyRoom) break;
        this.lobbyNames = msg.names;
        this.guestReady = msg.ready;
        this.lobbyPending = false;
        this.renderLobby();
        $('lobby-status').textContent = !msg.names[1] ? '等待好友加入'
          : this.mySeat === 0 ? (msg.ready ? '好友已准备，可以开始游戏' : '等待好友准备')
          : msg.ready ? '已准备，等待房主开始' : '已加入房间';
        this.menuTip('');
        break;
      case 'start':
        ++this.netRequest;
        this.lobbyAction = null;
        this.lobbyRoom = '';
        this.renderLobby();
        this.menuTip('');
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
        if (this.mode === 'menu') {
          this.cancelWaiting('对方已退出房间，请重新创建或加入');
          break;
        }
        this.showOverlay('🐧💨', '对手离开了', '房间已关闭，可以返回大厅开始新的对局', [
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
        if (this.mode === 'menu') {
          const action = this.lobbyAction;
          const room = this.lobbyRoom || $<HTMLInputElement>('room-input').value.trim().toUpperCase();
          if (this.lobbyRoom && msg.code !== 30001 && msg.code !== 20001) {
            this.lobbyPending = false;
            this.renderLobby();
            this.menuTip(msg.msg);
          } else {
            this.cancelWaiting(msg.msg);
            if (msg.code === 20001 && action) void this.startNet(action, room, true);
          }
        }
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
      if (this.mode === 'menu' && this.lobbyAction) {
        this.cancelWaiting('连接已断开，请重新创建或加入房间');
      }
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
