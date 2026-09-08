/** WebSocket 网络客户端：联机 / 人机共用（服务器权威） */
import type { ClientMsg, ServerMsg } from '../core/types';

export class NetClient {
  private ws: WebSocket | null = null;
  /** 收到服务器消息 */
  onMessage: (msg: ServerMsg) => void = () => {};
  /** 连接断开 */
  onClose: () => void = () => {};

  get connected(): boolean {
    return this.ws !== null && this.ws.readyState === WebSocket.OPEN;
  }

  /** 建立连接（携带登录 token，游客可省略） */
  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      try {
        const proto = location.protocol === 'https:' ? 'wss' : 'ws';
        const token = localStorage.getItem('penguin_token') ?? '';
        const qs = token ? `?token=${encodeURIComponent(token)}` : '';
        const ws = new WebSocket(`${proto}://${location.host}/ws${qs}`);
        this.ws = ws;
        ws.onopen = () => resolve();
        ws.onerror = () => reject(new Error('无法连接服务器'));
        ws.onclose = () => {
          this.ws = null;
          this.onClose();
        };
        ws.onmessage = (ev) => {
          try {
            const msg = JSON.parse(ev.data as string) as ServerMsg;
            this.onMessage(msg);
          } catch {
            // 忽略无法解析的消息
          }
        };
      } catch (e) {
        reject(e as Error);
      }
    });
  }

  send(msg: ClientMsg): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg));
    }
  }

  close(): void {
    this.onClose = () => {};
    this.ws?.close();
    this.ws = null;
  }
}
