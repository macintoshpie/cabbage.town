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
}

const LERP_FACTOR = 0.12;
const FADE_SPEED = 0.05;
const PIXEL_SCALE = 3; // each sprite pixel = 3 canvas pixels
const NAME_FONT = '11px monospace';
const NAME_COLOR = 'rgba(255,255,255,0.9)';
const NAME_SHADOW = 'rgba(0,0,0,0.5)';

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
}
