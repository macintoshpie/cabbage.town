export type ServerMessage =
  | { type: 'welcome'; id: string; name: string; x: number; y: number; users: UserInfo[] }
  | { type: 'join'; id: string; name: string; x: number; y: number }
  | { type: 'moved'; id: string; x: number; y: number }
  | { type: 'leave'; id: string }
  | { type: 'chatted'; id: string; text: string };

export interface UserInfo {
  id: string;
  name: string;
  x: number;
  y: number;
}

export interface WSCallbacks {
  onMessage: (msg: ServerMessage) => void;
  onOpen: () => void;
  onClose: () => void;
}

const MAX_BACKOFF = 30_000;
const BASE_BACKOFF = 1_000;

export class TownSquareWS {
  private ws: WebSocket | null = null;
  private url: string;
  private callbacks: WSCallbacks;
  private backoff = BASE_BACKOFF;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private stopped = false;

  constructor(url: string, callbacks: WSCallbacks) {
    this.url = url;
    this.callbacks = callbacks;
  }

  connect() {
    this.stopped = false;
    this.doConnect();
  }

  private doConnect() {
    if (this.stopped) return;

    try {
      this.ws = new WebSocket(this.url);
    } catch {
      this.scheduleReconnect();
      return;
    }

    this.ws.onopen = () => {
      this.backoff = BASE_BACKOFF;
      this.callbacks.onOpen();
    };

    this.ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data) as ServerMessage;
        this.callbacks.onMessage(msg);
      } catch {
        // ignore malformed messages
      }
    };

    this.ws.onclose = () => {
      this.callbacks.onClose();
      this.scheduleReconnect();
    };

    this.ws.onerror = () => {
      // onclose will fire after onerror
    };
  }

  private scheduleReconnect() {
    if (this.stopped) return;
    const jitter = Math.random() * 0.5 + 0.75; // 0.75x – 1.25x
    const delay = Math.min(this.backoff * jitter, MAX_BACKOFF);
    this.backoff = Math.min(this.backoff * 2, MAX_BACKOFF);
    this.reconnectTimer = setTimeout(() => this.doConnect(), delay);
  }

  sendMove(x: number, y: number) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: 'move', x, y }));
    }
  }

  sendChat(text: string) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: 'chat', text }));
    }
  }

  stop() {
    this.stopped = true;
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.onclose = null;
      this.ws.close();
      this.ws = null;
    }
  }
}
