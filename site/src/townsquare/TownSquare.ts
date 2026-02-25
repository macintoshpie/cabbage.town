import { TownSquareWS, type ServerMessage } from './ws';
import { Renderer } from './renderer';

const MOVE_THROTTLE_MS = 100; // max 10 messages/sec

export class TownSquare {
  private ws: TownSquareWS;
  private renderer: Renderer;
  private canvas: HTMLCanvasElement;
  private localId: string | null = null;
  private lastMoveSent = 0;
  private clickHandler: (e: MouseEvent) => void;
  private resizeHandler: () => void;
  private keyHandler: (e: KeyboardEvent) => void;
  private chatSendHandler: (e: Event) => void;

  constructor(wsUrl: string) {
    // Persistent identity token
    const TOKEN_KEY = 'ts-token';
    let token = localStorage.getItem(TOKEN_KEY);
    if (!token) {
      token = crypto.randomUUID();
      localStorage.setItem(TOKEN_KEY, token);
    }
    const sep = wsUrl.includes('?') ? '&' : '?';
    wsUrl = `${wsUrl}${sep}token=${encodeURIComponent(token)}`;
    // Create full-viewport canvas overlay
    this.canvas = document.createElement('canvas');
    this.canvas.style.position = 'fixed';
    this.canvas.style.top = '0';
    this.canvas.style.left = '0';
    this.canvas.style.width = '100vw';
    this.canvas.style.height = '100vh';
    this.canvas.style.pointerEvents = 'none';
    this.canvas.style.zIndex = '1';

    document.body.appendChild(this.canvas);

    this.renderer = new Renderer(this.canvas);

    // Size canvas to viewport
    this.resizeHandler = () => this.handleResize();
    window.addEventListener('resize', this.resizeHandler);
    this.handleResize();

    // Click-to-move on the whole page
    this.clickHandler = (e: MouseEvent) => this.handleClick(e);
    window.addEventListener('click', this.clickHandler);

    // Chat: listen for sends from ChatBar component, T key opens it
    this.chatSendHandler = (e: Event) => {
      this.ws.sendChat((e as CustomEvent).detail);
    };
    window.addEventListener('ts-send-chat', this.chatSendHandler);

    this.keyHandler = (e: KeyboardEvent) => {
      if (e.key === 't' || e.key === 'T') {
        const tag = (e.target as HTMLElement).tagName;
        if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
        e.preventDefault();
        window.dispatchEvent(new Event('ts-open-chat'));
      }
    };
    window.addEventListener('keydown', this.keyHandler);

    // Connect WebSocket
    this.ws = new TownSquareWS(wsUrl, {
      onMessage: (msg) => this.handleMessage(msg),
      onOpen: () => {},
      onClose: () => {},
    });
    this.ws.connect();
    this.renderer.start();
  }

  private handleResize() {
    this.renderer.resize(window.innerWidth, window.innerHeight);
  }

  private handleClick(e: MouseEvent) {
    // Don't intercept clicks on interactive elements
    const target = e.target as HTMLElement;
    if (target.closest('#playButton') || target.closest('#eye') || target.closest('a') || target.closest('button') || target.closest('[data-chat-ui]')) return;

    if (!this.localId) return;

    const nx = Math.max(0, Math.min(1, e.clientX / window.innerWidth));
    const ny = Math.max(0, Math.min(1, e.clientY / window.innerHeight));

    this.renderer.setLocalTarget(this.localId, nx, ny);
    this.throttledSendMove(nx, ny);
  }

  private throttledSendMove(x: number, y: number) {
    const now = Date.now();
    if (now - this.lastMoveSent < MOVE_THROTTLE_MS) return;
    this.lastMoveSent = now;
    this.ws.sendMove(x, y);
  }

  private handleMessage(msg: ServerMessage) {
    switch (msg.type) {
      case 'welcome':
        this.localId = msg.id;
        this.renderer.addSprite(msg.id, msg.name, msg.x, msg.y, true);
        for (const u of msg.users) {
          this.renderer.addSprite(u.id, u.name, u.x, u.y, false);
        }
        break;

      case 'join':
        this.renderer.addSprite(msg.id, msg.name, msg.x, msg.y, false);
        break;

      case 'moved':
        this.renderer.updateTarget(msg.id, msg.x, msg.y);
        break;

      case 'leave':
        this.renderer.removeSprite(msg.id);
        break;

      case 'chatted':
        this.renderer.showChat(msg.id, msg.text);
        break;
    }
  }

  stop() {
    this.ws.stop();
    this.renderer.stop();
    window.removeEventListener('resize', this.resizeHandler);
    window.removeEventListener('click', this.clickHandler);
    window.removeEventListener('keydown', this.keyHandler);
    window.removeEventListener('ts-send-chat', this.chatSendHandler);
    this.canvas.remove();
  }
}
