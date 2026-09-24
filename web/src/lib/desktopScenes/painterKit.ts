/**
 * Small, allocation-free helpers shared by scene painters. Painters draw only
 * pre-rendered sprites and batched paths so a frame stays well under 1 ms.
 */

/** Rectangle of the 16:9 poster after `background-size: cover` in the viewport. */
export interface SceneImageRect {
  x: number
  y: number
  width: number
  height: number
}

export interface SceneViewport {
  width: number
  height: number
  image: SceneImageRect
}

export interface SceneFrame {
  /** Seconds since the previous painted frame, clamped after pauses. */
  dt: number
  /** Seconds since the scene became visible. */
  time: number
  /** Entrance progress from 0 to 1; stays at 1 afterwards. */
  entrance: number
  /** Backing-store pixels per CSS pixel; painters that call setTransform multiply by it. */
  pixelRatio: number
}

export type SceneSurface = HTMLCanvasElement | OffscreenCanvas
export type SceneContext2D = CanvasRenderingContext2D | OffscreenCanvasRenderingContext2D

export interface ScenePainterEnvironment {
  random: () => number
  createSurface: (width: number, height: number) => SceneSurface | undefined
}

export interface ScenePainter {
  resize(viewport: SceneViewport): void
  /** 0.25–1; scales particle counts without restarting the scene. */
  setDensity(density: number): void
  setParams?(params: Readonly<Record<string, number>>): void
  frame(context: CanvasRenderingContext2D, frame: SceneFrame): void
}

export type ScenePainterFactory = (environment: ScenePainterEnvironment) => ScenePainter

export const SCENE_ASPECT = 16 / 9

export function coverImageRect(width: number, height: number, aspect = SCENE_ASPECT): SceneImageRect {
  const imageWidth = Math.max(width, height * aspect)
  const imageHeight = imageWidth / aspect
  return {
    x: (width - imageWidth) / 2,
    y: (height - imageHeight) / 2,
    width: imageWidth,
    height: imageHeight,
  }
}

export function mulberry32(seed: number): () => number {
  let state = seed >>> 0
  return () => {
    state = (state + 0x6d2b79f5) >>> 0
    let value = state
    value = Math.imul(value ^ (value >>> 15), value | 1)
    value ^= value + Math.imul(value ^ (value >>> 7), value | 61)
    return ((value ^ (value >>> 14)) >>> 0) / 4294967296
  }
}

export function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

export function lerp(from: number, to: number, amount: number): number {
  return from + (to - from) * amount
}

export function easeOutCubic(value: number): number {
  const inverse = 1 - clamp(value, 0, 1)
  return 1 - inverse * inverse * inverse
}

/** Box–Muller normal sample, used for light paths that cluster around a sun column. */
export function gaussian(random: () => number): number {
  const u = Math.max(random(), 1e-6)
  return Math.sqrt(-2 * Math.log(u)) * Math.cos(2 * Math.PI * random())
}

/** Draws once into an off-screen surface; returns undefined when canvas is unavailable. */
export function paintSprite(
  environment: ScenePainterEnvironment,
  width: number,
  height: number,
  draw: (context: SceneContext2D, width: number, height: number) => void,
): SceneSurface | undefined {
  const surface = environment.createSurface(Math.ceil(width), Math.ceil(height))
  const context = surface?.getContext('2d') as SceneContext2D | null | undefined
  if (!surface || !context) return undefined
  draw(context, width, height)
  return surface
}

export function glowSprite(
  environment: ScenePainterEnvironment,
  size: number,
  stops: readonly (readonly [number, string])[],
): SceneSurface | undefined {
  return paintSprite(environment, size, size, (context) => {
    const gradient = context.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2)
    for (const [offset, color] of stops) gradient.addColorStop(offset, color)
    context.fillStyle = gradient
    context.fillRect(0, 0, size, size)
  })
}

/** Four-point glint with a soft core, for sun glitter and bright stars. */
export function glintSprite(environment: ScenePainterEnvironment, size: number, tint: string): SceneSurface | undefined {
  return paintSprite(environment, size, size, (context) => {
    const center = size / 2
    const core = context.createRadialGradient(center, center, 0, center, center, center * 0.55)
    core.addColorStop(0, 'rgba(255,255,255,1)')
    core.addColorStop(0.35, tint)
    core.addColorStop(1, 'rgba(255,255,255,0)')
    context.fillStyle = core
    context.fillRect(0, 0, size, size)
    context.globalCompositeOperation = 'lighter'
    for (const [dx, dy] of [[1, 0], [0, 1]] as const) {
      const ray = context.createLinearGradient(center - dx * center, center - dy * center, center + dx * center, center + dy * center)
      ray.addColorStop(0, 'rgba(255,255,255,0)')
      ray.addColorStop(0.5, tint)
      ray.addColorStop(1, 'rgba(255,255,255,0)')
      context.fillStyle = ray
      if (dx) context.fillRect(0, center - size * 0.035, size, size * 0.07)
      else context.fillRect(center - size * 0.035, 0, size * 0.07, size)
    }
  })
}

/** Draws a centered sprite with rotation and a flattened axis in one transform call. */
export function drawSprite(
  context: CanvasRenderingContext2D,
  sprite: SceneSurface,
  x: number,
  y: number,
  size: number,
  pixelRatio: number,
  rotation = 0,
  scaleX = 1,
): void {
  const cos = Math.cos(rotation) * pixelRatio
  const sin = Math.sin(rotation) * pixelRatio
  context.setTransform(cos * scaleX, sin * scaleX, -sin, cos, x * pixelRatio, y * pixelRatio)
  context.drawImage(sprite, -size / 2, -size / 2, size, size)
}

/** Maps normalized poster coordinates to viewport pixels. */
export function imagePoint(viewport: SceneViewport, u: number, v: number): { x: number, y: number } {
  return {
    x: viewport.image.x + u * viewport.image.width,
    y: viewport.image.y + v * viewport.image.height,
  }
}
