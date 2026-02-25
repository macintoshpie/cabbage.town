import { CABBAGE_SPRITE, SPRITE_W, SPRITE_H } from './sprites';

export interface Sprite {
  id: string;
  name: string;
  targetX: number;   // normalized 0–1
  targetY: number;
  currentX: number;  // lerped normalized position
  currentY: number;
  opacity: number;   // 0–1, used for fade-in
  isLocal: boolean;
  chatText: string | null;
  chatExpiry: number;   // timestamp when bubble starts fading
  chatOpacity: number;  // 1.0 → 0.0 fade
}

const LERP_FACTOR = 0.12;
const FADE_SPEED = 0.05;
const PIXEL_SCALE = 3; // each sprite pixel = 3 canvas pixels
const NAME_FONT = '11px monospace';
const NAME_COLOR = 'rgba(255,255,255,0.9)';
const NAME_SHADOW = 'rgba(0,0,0,0.5)';
const BUBBLE_FONT = '11px monospace';
const BUBBLE_LINE_CHARS = 20;
const BUBBLE_PAD_X = 6;
const BUBBLE_PAD_Y = 4;
const BUBBLE_LINE_H = 14;
const BUBBLE_RADIUS = 4;
const BUBBLE_POINTER_H = 5;
const BUBBLE_HOLD_MS = 10_000;
const BUBBLE_FADE_SPEED = 0.03;

export class Renderer {
  private canvas: HTMLCanvasElement;
  private ctx: CanvasRenderingContext2D;
  private sprites = new Map<string, Sprite>();
  private animId = 0;
  private running = false;

  constructor(canvas: HTMLCanvasElement) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d')!;
    this.ctx.imageSmoothingEnabled = false;
  }

  addSprite(id: string, name: string, x: number, y: number, isLocal: boolean) {
    this.sprites.set(id, {
      id, name,
      targetX: x, targetY: y,
      currentX: x, currentY: y,
      opacity: 0,
      isLocal,
      chatText: null,
      chatExpiry: 0,
      chatOpacity: 0,
    });
  }

  updateTarget(id: string, x: number, y: number) {
    const s = this.sprites.get(id);
    if (s) {
      s.targetX = x;
      s.targetY = y;
    }
  }

  setLocalTarget(id: string, x: number, y: number) {
    this.updateTarget(id, x, y);
  }

  removeSprite(id: string) {
    this.sprites.delete(id);
  }

  showChat(id: string, text: string) {
    const s = this.sprites.get(id);
    if (s) {
      s.chatText = text;
      s.chatExpiry = Date.now() + BUBBLE_HOLD_MS;
      s.chatOpacity = 1;
    }
  }

  start() {
    if (this.running) return;
    this.running = true;
    this.tick();
  }

  stop() {
    this.running = false;
    if (this.animId) {
      cancelAnimationFrame(this.animId);
      this.animId = 0;
    }
  }

  resize(width: number, height: number) {
    const dpr = window.devicePixelRatio || 1;
    this.canvas.width = width * dpr;
    this.canvas.height = height * dpr;
    this.canvas.style.width = width + 'px';
    this.canvas.style.height = height + 'px';
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    this.ctx.imageSmoothingEnabled = false;
  }

  private tick = () => {
    if (!this.running) return;

    const w = this.canvas.clientWidth;
    const h = this.canvas.clientHeight;

    this.ctx.clearRect(0, 0, w, h);

    // Sort by Y for depth (lower Y = behind)
    const sorted = [...this.sprites.values()].sort((a, b) => a.currentY - b.currentY);

    for (const s of sorted) {
      // Lerp position
      s.currentX += (s.targetX - s.currentX) * LERP_FACTOR;
      s.currentY += (s.targetY - s.currentY) * LERP_FACTOR;

      // Fade in
      if (s.opacity < 1) {
        s.opacity = Math.min(1, s.opacity + FADE_SPEED);
      }

      const px = s.currentX * w;
      const py = s.currentY * h;

      this.ctx.globalAlpha = s.opacity;
      this.drawCabbage(px, py);
      this.drawName(s.name, px, py, s.isLocal);

      // Chat bubble
      if (s.chatText) {
        const now = Date.now();
        if (now > s.chatExpiry) {
          s.chatOpacity -= BUBBLE_FADE_SPEED;
          if (s.chatOpacity <= 0) {
            s.chatText = null;
            s.chatOpacity = 0;
          }
        }
        if (s.chatText) {
          const spriteH = SPRITE_H * PIXEL_SCALE;
          const bubbleY = py - spriteH - 22;
          this.drawBubble(s.chatText, px, bubbleY, s.chatOpacity * s.opacity);
        }
      }

      this.ctx.globalAlpha = 1;
    }

    this.animId = requestAnimationFrame(this.tick);
  };

  private drawCabbage(cx: number, cy: number) {
    const spriteW = SPRITE_W * PIXEL_SCALE;
    const spriteH = SPRITE_H * PIXEL_SCALE;
    const startX = cx - spriteW / 2;
    const startY = cy - spriteH;

    for (let row = 0; row < SPRITE_H; row++) {
      for (let col = 0; col < SPRITE_W; col++) {
        const color = CABBAGE_SPRITE[row][col];
        if (!color) continue;
        this.ctx.fillStyle = color;
        this.ctx.fillRect(
          Math.round(startX + col * PIXEL_SCALE),
          Math.round(startY + row * PIXEL_SCALE),
          PIXEL_SCALE,
          PIXEL_SCALE,
        );
      }
    }
  }

  private drawName(name: string, cx: number, cy: number, isLocal: boolean) {
    const spriteH = SPRITE_H * PIXEL_SCALE;
    const textY = cy - spriteH - 6;

    this.ctx.font = NAME_FONT;
    this.ctx.textAlign = 'center';
    this.ctx.textBaseline = 'bottom';

    // Shadow
    this.ctx.fillStyle = NAME_SHADOW;
    this.ctx.fillText(name, cx + 1, textY + 1);

    // Text (highlight local player)
    this.ctx.fillStyle = isLocal ? '#ffd700' : NAME_COLOR;
    this.ctx.fillText(name, cx, textY);
  }

  private drawBubble(text: string, cx: number, cy: number, opacity: number) {
    if (opacity <= 0) return;

    // Word-wrap text into lines, hard-breaking long words with hyphens
    const lines: string[] = [];
    let line = '';
    for (const word of text.split(' ')) {
      // Word fits on current line
      if (line.length + word.length + (line ? 1 : 0) <= BUBBLE_LINE_CHARS) {
        line = line ? line + ' ' + word : word;
        continue;
      }
      // Word fits on a new line by itself
      if (word.length <= BUBBLE_LINE_CHARS) {
        if (line) lines.push(line);
        line = word;
        continue;
      }
      // Word is too long — hard-break with hyphens
      let remaining = word;
      while (remaining.length > 0) {
        const space = BUBBLE_LINE_CHARS - (line ? line.length + 1 : 0);
        if (remaining.length <= space) {
          line = line ? line + ' ' + remaining : remaining;
          remaining = '';
        } else if (space > 1) {
          const chunk = remaining.slice(0, space - 1) + '-';
          line = line ? line + ' ' + chunk : chunk;
          remaining = remaining.slice(space - 1);
          lines.push(line);
          line = '';
        } else {
          if (line) lines.push(line);
          line = '';
        }
      }
    }
    if (line) lines.push(line);

    this.ctx.font = BUBBLE_FONT;

    // Measure max line width
    let maxW = 0;
    for (const l of lines) {
      const w = this.ctx.measureText(l).width;
      if (w > maxW) maxW = w;
    }

    const boxW = maxW + BUBBLE_PAD_X * 2;
    const boxH = lines.length * BUBBLE_LINE_H + BUBBLE_PAD_Y * 2;
    const boxX = cx - boxW / 2;
    const boxY = cy - boxH;

    const prevAlpha = this.ctx.globalAlpha;
    this.ctx.globalAlpha = opacity;

    // Background rounded rect
    this.ctx.fillStyle = 'rgba(0,0,0,0.7)';
    this.ctx.beginPath();
    this.ctx.moveTo(boxX + BUBBLE_RADIUS, boxY);
    this.ctx.lineTo(boxX + boxW - BUBBLE_RADIUS, boxY);
    this.ctx.quadraticCurveTo(boxX + boxW, boxY, boxX + boxW, boxY + BUBBLE_RADIUS);
    this.ctx.lineTo(boxX + boxW, boxY + boxH - BUBBLE_RADIUS);
    this.ctx.quadraticCurveTo(boxX + boxW, boxY + boxH, boxX + boxW - BUBBLE_RADIUS, boxY + boxH);
    this.ctx.lineTo(boxX + BUBBLE_RADIUS, boxY + boxH);
    this.ctx.quadraticCurveTo(boxX, boxY + boxH, boxX, boxY + boxH - BUBBLE_RADIUS);
    this.ctx.lineTo(boxX, boxY + BUBBLE_RADIUS);
    this.ctx.quadraticCurveTo(boxX, boxY, boxX + BUBBLE_RADIUS, boxY);
    this.ctx.closePath();
    this.ctx.fill();

    // Pointer triangle
    this.ctx.beginPath();
    this.ctx.moveTo(cx - 4, boxY + boxH);
    this.ctx.lineTo(cx, boxY + boxH + BUBBLE_POINTER_H);
    this.ctx.lineTo(cx + 4, boxY + boxH);
    this.ctx.closePath();
    this.ctx.fill();

    // Text
    this.ctx.fillStyle = '#fff';
    this.ctx.textAlign = 'center';
    this.ctx.textBaseline = 'top';
    for (let i = 0; i < lines.length; i++) {
      this.ctx.fillText(lines[i], cx, boxY + BUBBLE_PAD_Y + i * BUBBLE_LINE_H);
    }

    this.ctx.globalAlpha = prevAlpha;
  }
}
