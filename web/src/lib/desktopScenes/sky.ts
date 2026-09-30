// Local time of day for time-aware scenes. The palette is art-directed per hour
// (in sRGB, converted to linear light for the shaders) so every hour reads as
// intended, while the sun and moon travel a continuous arc across the view.

export type Vec3 = readonly [number, number, number]

export const SUNRISE = 6.1
export const SUNSET = 18.6

interface SkyKey {
  hour: number
  zenith: Vec3
  horizon: Vec3
  /** Horizon tint on the sun's side of the sky. */
  glow: Vec3
  /** Direct sunlight colour before it fades below the horizon. */
  sun: Vec3
  /** Ambient light level: 0 is deep night, 1 is noon. */
  light: number
  stars: number
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

function smoothstep(edge0: number, edge1: number, value: number): number {
  const t = clamp((value - edge0) / (edge1 - edge0), 0, 1)
  return t * t * (3 - 2 * t)
}

function lerp(from: number, to: number, amount: number): number {
  return from + (to - from) * amount
}

function mix3(from: Vec3, to: Vec3, amount: number): Vec3 {
  return [lerp(from[0], to[0], amount), lerp(from[1], to[1], amount), lerp(from[2], to[2], amount)]
}

function scale3(value: Vec3, amount: number): Vec3 {
  return [value[0] * amount, value[1] * amount, value[2] * amount]
}

/** `#rrggbb` in sRGB to linear light. */
export function linear(hex: string): Vec3 {
  const parsed = Number.parseInt(hex.slice(1), 16)
  const channel = (value: number) => {
    const c = value / 255
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  }
  return [channel((parsed >> 16) & 255), channel((parsed >> 8) & 255), channel(parsed & 255)]
}

const key = (hour: number, zenith: string, horizon: string, glow: string, sun: string, light: number, stars: number): SkyKey => ({
  hour, zenith: linear(zenith), horizon: linear(horizon), glow: linear(glow), sun: linear(sun), light, stars,
})

const NIGHT = key(0, '#030817', '#13234a', '#1a2a57', '#000000', 0.05, 1)

export const SKY_KEYS: readonly SkyKey[] = [
  NIGHT,
  key(4.7, '#081231', '#2a3363', '#48386a', '#000000', 0.09, 0.85),
  key(5.7, '#1d3470', '#c98378', '#ff9460', '#ff8a4a', 0.3, 0.3),
  key(6.6, '#3567b2', '#f3c49a', '#ffad6a', '#ffc58a', 0.62, 0),
  key(8.4, '#2c6bc2', '#b6d6ee', '#ffe0b6', '#fff0d4', 0.88, 0),
  key(12, '#2366c6', '#bcdcf3', '#fff2da', '#fffaf0', 1, 0),
  key(16, '#2a6abd', '#c6dbec', '#ffdcaa', '#fff0d0', 0.92, 0),
  key(17.6, '#335fa6', '#efc390', '#ffa35a', '#ffbd74', 0.72, 0),
  key(18.4, '#283d7a', '#ec8560', '#ff7340', '#ff8850', 0.46, 0.05),
  key(19.2, '#121b45', '#83466e', '#b84a66', '#000000', 0.2, 0.5),
  key(20.4, '#050b20', '#18274f', '#1d2b57', '#000000', 0.07, 1),
  { ...NIGHT, hour: 24 },
]

/** Angle along the sun's daily circle: [0, π] by day, (π, 2π) by night. */
export function sunAngle(hour: number): number {
  const h = ((hour % 24) + 24) % 24
  const day = SUNSET - SUNRISE
  if (h >= SUNRISE && h <= SUNSET) return ((h - SUNRISE) / day) * Math.PI
  const night = 24 - day
  const since = (((h - SUNSET) % 24) + 24) % 24
  return Math.PI + (since / night) * Math.PI
}

/** Unit direction on the arc the sun and moon share: rises on the left, sets on the right. */
export function arcDirection(angle: number): Vec3 {
  const x = -Math.cos(angle) * 0.45
  const y = Math.sin(angle) * 0.42
  const length = Math.hypot(x, y, 1)
  return [x / length, y / length, 1 / length]
}

export interface SkyUniforms {
  zenith: Vec3
  horizon: Vec3
  glow: Vec3
  sunColor: Vec3
  moonColor: Vec3
  sunDir: Vec3
  moonDir: Vec3
  light: number
  stars: number
}

export function skyAt(hour: number): SkyUniforms {
  const h = ((hour % 24) + 24) % 24
  let index = 0
  while (index < SKY_KEYS.length - 2 && SKY_KEYS[index + 1]!.hour <= h) index++
  const from = SKY_KEYS[index]!
  const to = SKY_KEYS[index + 1]!
  const amount = smoothstep(0, 1, (h - from.hour) / (to.hour - from.hour))
  const angle = sunAngle(h)
  const sunDir = arcDirection(angle)
  const moonDir = arcDirection(angle - Math.PI)
  const light = lerp(from.light, to.light, amount)
  const sunUp = smoothstep(-0.03, 0.04, sunDir[1])
  const moonUp = smoothstep(-0.02, 0.08, moonDir[1]) * (1 - smoothstep(0.15, 0.45, light))
  return {
    zenith: mix3(from.zenith, to.zenith, amount),
    horizon: mix3(from.horizon, to.horizon, amount),
    glow: mix3(from.glow, to.glow, amount),
    sunColor: scale3(mix3(from.sun, to.sun, amount), sunUp),
    moonColor: scale3(linear('#b7c6f0'), 0.42 * moonUp),
    sunDir,
    moonDir,
    light,
    stars: lerp(from.stars, to.stars, amount),
  }
}
