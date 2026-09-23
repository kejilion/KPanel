import type { SceneContext2D, SceneEnvironment, SceneSurface } from '../types'

export type RGB = readonly [number, number, number]

export const TAU = Math.PI * 2

export function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

export function lerp(from: number, to: number, amount: number): number {
  return from + (to - from) * amount
}

export function smoothstep(amount: number): number {
  const t = clamp(amount, 0, 1)
  return t * t * (3 - 2 * t)
}

export function hex(value: string): RGB {
  const parsed = Number.parseInt(value.slice(1), 16)
  return [(parsed >> 16) & 255, (parsed >> 8) & 255, parsed & 255]
}

export function mix(from: RGB, to: RGB, amount: number): RGB {
  return [lerp(from[0], to[0], amount), lerp(from[1], to[1], amount), lerp(from[2], to[2], amount)]
}

export function rgba(color: RGB, alpha = 1): string {
  return `rgb(${Math.round(color[0])} ${Math.round(color[1])} ${Math.round(color[2])} / ${Math.round(clamp(alpha, 0, 1) * 1000) / 10}%)`
}

/** Deterministic PRNG so every visit to a scene composes the same landscape. */
export function random(seed: number): () => number {
  let state = seed >>> 0
  return () => {
    state = (state + 0x6d2b79f5) >>> 0
    let t = state
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

export interface Layer {
  surface: SceneSurface
  context: SceneContext2D
}

export function createLayer(environment: SceneEnvironment, width: number, height: number): Layer {
  const surface = environment.createSurface(Math.max(1, Math.round(width)), Math.max(1, Math.round(height)))
  const context = surface.getContext('2d') as SceneContext2D | null
  if (!context) throw new Error('scene_layer_unavailable')
  return { surface, context }
}

/** Pre-rendered radial glow; drawing sprites is far cheaper than per-frame gradients. */
export function glowSprite(environment: SceneEnvironment, radius: number, color: RGB, core = 0.2): SceneSurface {
  const size = Math.max(2, Math.ceil(radius * 2))
  const { surface, context } = createLayer(environment, size, size)
  const gradient = context.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2)
  gradient.addColorStop(0, rgba(color, 1))
  gradient.addColorStop(core, rgba(color, 0.6))
  gradient.addColorStop(0.55, rgba(color, 0.14))
  gradient.addColorStop(1, rgba(color, 0))
  context.fillStyle = gradient
  context.fillRect(0, 0, size, size)
  return surface
}

/** Midpoint-displacement ridge heights in [0, 1] across `count` samples. */
export function ridge(next: () => number, count: number, roughness: number): number[] {
  let size = 1
  while (size < count - 1) size *= 2
  const points = new Array<number>(size + 1).fill(0)
  points[0] = next()
  points[size] = next()
  let amplitude = 1
  for (let step = size; step > 1; step /= 2) {
    const half = step / 2
    for (let index = half; index < size; index += step) {
      points[index] = ((points[index - half]! + points[index + half]!) / 2) + (next() - 0.5) * amplitude
    }
    amplitude *= roughness
  }
  const min = Math.min(...points)
  const max = Math.max(...points)
  return points.slice(0, count).map((value) => (value - min) / Math.max(1e-6, max - min))
}

export function fillRidge(
  context: SceneContext2D,
  heights: readonly number[],
  width: number,
  baseline: number,
  amplitude: number,
  bottom: number,
): void {
  context.beginPath()
  context.moveTo(0, bottom)
  const step = width / Math.max(1, heights.length - 1)
  heights.forEach((value, index) => context.lineTo(index * step, baseline - value * amplitude))
  context.lineTo(width, bottom)
  context.closePath()
  context.fill()
}

export interface Star {
  x: number
  y: number
  size: number
  phase: number
  speed: number
  brightness: number
}

export function createStars(next: () => number, count: number): Star[] {
  return Array.from({ length: count }, () => ({
    x: next(),
    y: next() ** 1.6,
    size: next() < 0.9 ? 1 : 2,
    phase: next() * TAU,
    speed: 0.6 + next() * 1.8,
    brightness: 0.35 + next() * 0.65,
  }))
}

export function drawStars(
  context: SceneContext2D,
  stars: readonly Star[],
  width: number,
  height: number,
  time: number,
  alpha: number,
  scale: number,
): void {
  if (alpha <= 0.01) return
  context.fillStyle = '#fff'
  for (const star of stars) {
    const twinkle = 0.55 + 0.45 * Math.sin(time * star.speed + star.phase)
    context.globalAlpha = alpha * star.brightness * twinkle
    const size = star.size * scale
    context.fillRect(star.x * width, star.y * height, size, size)
  }
  context.globalAlpha = 1
}

// Local time of day ----------------------------------------------------------

export interface SkyKey {
  hour: number
  top: RGB
  middle: RGB
  horizon: RGB
  sun: RGB
  stars: number
  light: number
}

const key = (hour: number, top: string, middle: string, horizon: string, sun: string, stars: number, light: number): SkyKey => ({
  hour, top: hex(top), middle: hex(middle), horizon: hex(horizon), sun: hex(sun), stars, light,
})

export const SKY_KEYS: readonly SkyKey[] = [
  key(0, '#050919', '#0b1531', '#1a2447', '#dfe7ff', 1, 0.06),
  key(4.6, '#0a1230', '#1d2a56', '#3b3a68', '#ffd2b0', 0.85, 0.12),
  key(5.9, '#24356f', '#7c5d92', '#f29c7c', '#ffb27a', 0.2, 0.42),
  key(7.2, '#3a72c4', '#8fb6e2', '#f7d6ae', '#fff0d0', 0, 0.82),
  key(12, '#2a78d4', '#69abea', '#cfe7f7', '#fffaf0', 0, 1),
  key(16.4, '#3674c6', '#86b3e0', '#f3ddb9', '#fff0cf', 0, 0.9),
  key(18.1, '#2c3f7c', '#b2638b', '#ff9b5c', '#ffb36b', 0.05, 0.55),
  key(19.3, '#121b45', '#3d2d63', '#a2527b', '#ff9d7a', 0.55, 0.24),
  key(20.6, '#070c20', '#10193b', '#23315b', '#dfe7ff', 1, 0.08),
  key(24, '#050919', '#0b1531', '#1a2447', '#dfe7ff', 1, 0.06),
]

export interface SkyColors {
  top: RGB
  middle: RGB
  horizon: RGB
  sun: RGB
  stars: number
  light: number
}

export function skyAt(hour: number): SkyColors {
  const normalized = ((hour % 24) + 24) % 24
  let index = 0
  while (index < SKY_KEYS.length - 2 && SKY_KEYS[index + 1]!.hour <= normalized) index++
  const from = SKY_KEYS[index]!
  const to = SKY_KEYS[index + 1]!
  const amount = smoothstep((normalized - from.hour) / (to.hour - from.hour))
  return {
    top: mix(from.top, to.top, amount),
    middle: mix(from.middle, to.middle, amount),
    horizon: mix(from.horizon, to.horizon, amount),
    sun: mix(from.sun, to.sun, amount),
    stars: lerp(from.stars, to.stars, amount),
    light: lerp(from.light, to.light, amount),
  }
}

export const SUNRISE = 6
export const SUNSET = 18.6

export interface CelestialPosition {
  /** Horizontal position in [0, 1]. */
  x: number
  /** Height above the horizon in [-1, 1]; negative values are below it. */
  altitude: number
  kind: 'sun' | 'moon'
}

export function celestialAt(hour: number): CelestialPosition {
  const normalized = ((hour % 24) + 24) % 24
  const day = SUNSET - SUNRISE
  if (normalized >= SUNRISE && normalized <= SUNSET) {
    const progress = (normalized - SUNRISE) / day
    return { kind: 'sun', x: lerp(0.12, 0.88, progress), altitude: Math.sin(progress * Math.PI) }
  }
  const night = 24 - day
  const progress = (((normalized - SUNSET) % 24) + 24) % 24 / night
  return { kind: 'moon', x: lerp(0.14, 0.86, progress), altitude: Math.sin(progress * Math.PI) * 0.85 }
}

export function paintSky(context: SceneContext2D, sky: SkyColors, width: number, horizonY: number): void {
  const gradient = context.createLinearGradient(0, 0, 0, horizonY)
  gradient.addColorStop(0, rgba(sky.top))
  gradient.addColorStop(0.62, rgba(sky.middle))
  gradient.addColorStop(1, rgba(sky.horizon))
  context.fillStyle = gradient
  context.fillRect(0, 0, width, horizonY + 2)
}
