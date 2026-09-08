/** 用户会话：token 管理 + HTTP API（注册/登录/档案/天梯） */

export interface UserProfile {
  id: number;
  username: string;
  wins: number;
  losses: number;
  draws: number;
  elo: number;
}

export interface GameRecord {
  id: number;
  room_id: string;
  opponent: string;
  my_score: number;
  opp_score: number;
  winner: number;
  elo_delta: number;
  finished_at: string;
}

const TOKEN_KEY = 'penguin_token';
const USER_KEY = 'penguin_user';

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? '';
}

export function getUser(): UserProfile | null {
  const raw = localStorage.getItem(USER_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as UserProfile;
  } catch {
    return null;
  }
}

function saveSession(token: string, user: UserProfile): void {
  localStorage.setItem(TOKEN_KEY, token);
  localStorage.setItem(USER_KEY, JSON.stringify(user));
}

export function clearSession(): void {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(path, { ...init, headers });
  const result = await res.json() as { code: number; message: string; data: T; requestId: string };
  if (!res.ok || result.code !== 0) {
    throw new ApiError(result.message ?? '请求失败', result.code, result.requestId);
  }
  return result.data;
}

export class ApiError extends Error {
  constructor(message: string, public readonly code: number, public readonly requestId: string) {
    super(message);
    this.name = 'ApiError';
  }
}

interface AuthResp {
  token: string;
  user: UserProfile;
}

export async function register(username: string, password: string): Promise<UserProfile> {
  const r = await api<AuthResp>('/api/register', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  });
  saveSession(r.token, r.user);
  return r.user;
}

export async function login(username: string, password: string): Promise<UserProfile> {
  const r = await api<AuthResp>('/api/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  });
  saveSession(r.token, r.user);
  return r.user;
}

export interface ProfileResp {
  user: UserProfile;
  recent: GameRecord[] | null;
}

export async function fetchProfile(): Promise<ProfileResp> {
  return api<ProfileResp>('/api/profile');
}

export interface LeaderboardResp {
  players: UserProfile[] | null;
}

export async function fetchLeaderboard(): Promise<UserProfile[]> {
  const r = await api<LeaderboardResp>('/api/leaderboard');
  return r.players ?? [];
}
