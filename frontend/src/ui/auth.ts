/** 登录 / 注册 / 天梯榜 / 用户卡片 的 UI 逻辑 */
import {
  clearSession,
  fetchLeaderboard,
  fetchProfile,
  getToken,
  getUser,
  login,
  register,
  type UserProfile,
} from '../net/api';

const $ = <T extends HTMLElement>(id: string): T => {
  const el = document.getElementById(id);
  if (!el) throw new Error(`缺少元素 #${id}`);
  return el as T;
};

export class AuthUI {
  private authMode: 'login' | 'register' = 'login';
  private user: UserProfile | null = null;
  private loginResult: ((success: boolean) => void) | null = null;
  /** 登录态变化（登录成功 / 退出登录） */
  onAuthChange: () => void = () => {};

  constructor() {
    this.user = getUser();
    this.bindAuthModal();
    this.bindUserBar();
    this.bindRankModal();
    this.render();
    // 已有 token：静默校验并拉取最新战绩
    if (this.user) void this.refresh();
  }

  get currentUser(): UserProfile | null {
    return this.user;
  }

  requestLogin(resetSession = false): Promise<boolean> {
    if (resetSession) {
      this.user = null;
      clearSession();
      this.render();
    }
    this.openAuthModal();
    return new Promise(resolve => { this.loginResult = resolve; });
  }

  /** 重新拉取档案（对局结束后调用，刷新胜率与 Elo） */
  async refresh(): Promise<void> {
    if (!getToken()) return;
    try {
      const r = await fetchProfile();
      this.user = r.user;
      this.render();
    } catch {
      // token 失效：清除本地会话
      this.user = null;
      clearSession();
      this.render();
    }
  }

  // ---------- 登录 / 注册模态 ----------

  private bindAuthModal(): void {
    $('tab-login').addEventListener('click', () => this.switchAuthMode('login'));
    $('tab-register').addEventListener('click', () => this.switchAuthMode('register'));
    $('auth-cancel').addEventListener('click', () => {
      this.closeAuthModal();
      this.loginResult?.(false);
      this.loginResult = null;
    });
    $('auth-submit').addEventListener('click', () => void this.submitAuth());
    $('auth-password').addEventListener('keydown', (e) => {
      if (e.key === 'Enter') $('auth-submit').click();
    });
    $('auth-username').addEventListener('keydown', (e) => {
      if (e.key === 'Enter') $('auth-password').focus();
    });
  }

  private switchAuthMode(mode: 'login' | 'register'): void {
    this.authMode = mode;
    $('tab-login').classList.toggle('active', mode === 'login');
    $('tab-register').classList.toggle('active', mode === 'register');
    $('auth-submit').textContent = mode === 'login' ? '登录' : '注册并开始';
    this.authError('');
  }

  private openAuthModal(): void {
    this.switchAuthMode('login');
    ($('auth-username') as HTMLInputElement).value = '';
    ($('auth-password') as HTMLInputElement).value = '';
    this.authError('');
    $('auth-modal').classList.remove('hidden');
    $('auth-username').focus();
  }

  private closeAuthModal(): void {
    $('auth-modal').classList.add('hidden');
  }

  private authError(msg: string): void {
    const el = $('auth-error');
    if (msg) {
      el.textContent = msg;
      el.classList.remove('hidden');
    } else {
      el.classList.add('hidden');
    }
  }

  private async submitAuth(): Promise<void> {
    const username = ($('auth-username') as HTMLInputElement).value.trim();
    const password = ($('auth-password') as HTMLInputElement).value;
    if (username.length < 2 || username.length > 16) {
      this.authError('用户名需 2~16 个字符');
      return;
    }
    if (password.length < 6 || password.length > 64) {
      this.authError('密码需 6~64 位');
      return;
    }
    const btn = $<HTMLButtonElement>('auth-submit');
    btn.disabled = true;
    try {
      this.user =
        this.authMode === 'login' ? await login(username, password) : await register(username, password);
      this.closeAuthModal();
      this.render();
      this.onAuthChange();
      this.loginResult?.(true);
      this.loginResult = null;
    } catch (e) {
      this.authError((e as Error).message || '请求失败，请稍后重试');
    } finally {
      btn.disabled = false;
    }
  }

  // ---------- 用户栏 ----------

  private bindUserBar(): void {
    $('btn-login').addEventListener('click', () => this.openAuthModal());
    $('btn-logout').addEventListener('click', () => {
      this.user = null;
      clearSession();
      this.render();
      this.onAuthChange();
    });
  }

  // ---------- 天梯榜 ----------

  private bindRankModal(): void {
    $('btn-rank').addEventListener('click', () => void this.openRankModal());
    $('rank-close').addEventListener('click', () => $('rank-modal').classList.add('hidden'));
  }

  private async openRankModal(): Promise<void> {
    const list = $('rank-list');
    list.innerHTML = '<div class="rank-empty">加载中…</div>';
    $('rank-modal').classList.remove('hidden');
    try {
      const players = await fetchLeaderboard();
      if (players.length === 0) {
        list.innerHTML = '<div class="rank-empty">还没有玩家上榜，快去开一局吧！</div>';
        return;
      }
      list.innerHTML = '';
      players.forEach((p, i) => {
        const row = document.createElement('div');
        row.className = 'rank-row' + (this.user && this.user.id === p.id ? ' me' : '');
        const medal = i < 3 ? ['🥇', '🥈', '🥉'][i] : String(i + 1);
        const rank = document.createElement('span');
        rank.className = 'rank-no';
        rank.textContent = medal;
        const name = document.createElement('span');
        name.className = 'rank-name';
        name.textContent = p.username;
        const record = document.createElement('span');
        record.className = 'rank-record';
        record.textContent = `${p.wins}胜 ${p.losses}负 ${p.draws}平`;
        const elo = document.createElement('strong');
        elo.className = 'rank-elo';
        elo.textContent = String(p.elo);
        row.append(rank, name, record, elo);
        list.appendChild(row);
      });
    } catch {
      list.innerHTML = '<div class="rank-empty">加载失败，请稍后重试</div>';
    }
  }

  // ---------- 渲染 ----------

  private render(): void {
    const logged = this.user !== null;
    $('user-card').classList.toggle('hidden', !logged);
    $('btn-login').classList.toggle('hidden', logged);
    if (logged) {
      $('user-name').textContent = this.user!.username;
      const total = this.user!.wins + this.user!.losses + this.user!.draws;
      const rate = total > 0 ? Math.round((this.user!.wins / total) * 100) : 0;
      $('user-stats').textContent = `Elo ${this.user!.elo} · ${this.user!.wins}胜 ${this.user!.losses}负（${rate}%）`;
    }
  }
}
