import * as THREE from 'three'

/**
 * Time of day along the camera tour: 0 = dusk, 1 = blue hour, 2 = rainy night.
 * Every shader in the city reads the same uniform objects, so one update per
 * frame moves sky, fog, windows, neon, rain and wet streets together.
 */
interface Look {
  skyTop: [number, number, number]
  skyMid: [number, number, number]
  horizon: [number, number, number]
  glow: [number, number, number]
  fog: [number, number, number]
  sunColor: [number, number, number]
  fogDensity: number
  sun: number
  ambient: number
  lit: number
  neon: number
  lamps: number
  rain: number
  wet: number
  stars: number
  moon: number
  clouds: number
  search: number
  exposure: number
  bloom: number
}

const LOOKS: readonly Look[] = [
  { // Dusk: afterglow behind the skyline, the city switching on.
    skyTop: [0.022, 0.035, 0.12], skyMid: [0.17, 0.06, 0.17], horizon: [0.9, 0.3, 0.11], glow: [0.12, 0.05, 0.07],
    fog: [0.1, 0.05, 0.075], sunColor: [1.3, 0.48, 0.2], fogDensity: 0.0005,
    sun: 1, ambient: 0.45, lit: 0.14, neon: 0.7, lamps: 0.7, rain: 0, wet: 0.3, stars: 0, moon: 0, clouds: 0.45, search: 0, exposure: 0.85, bloom: 0.45,
  },
  { // Blue hour: deep blue sky, traffic and highway rails take over.
    skyTop: [0.005, 0.012, 0.05], skyMid: [0.013, 0.028, 0.09], horizon: [0.045, 0.065, 0.16], glow: [0.1, 0.05, 0.13],
    fog: [0.011, 0.018, 0.045], sunColor: [0.3, 0.26, 0.5], fogDensity: 0.00065,
    sun: 0.15, ambient: 0.16, lit: 0.26, neon: 1, lamps: 1, rain: 0.12, wet: 0.5, stars: 0.7, moon: 0.4, clouds: 0.3, search: 0.55, exposure: 0.9, bloom: 0.5,
  },
  { // Rainy night: low clouds lit from below, haze, soaked streets.
    skyTop: [0.004, 0.004, 0.012], skyMid: [0.018, 0.01, 0.028], horizon: [0.09, 0.035, 0.08], glow: [0.36, 0.1, 0.3],
    fog: [0.035, 0.018, 0.045], sunColor: [0, 0, 0], fogDensity: 0.0011,
    sun: 0, ambient: 0.06, lit: 0.32, neon: 1.1, lamps: 1.05, rain: 1, wet: 1, stars: 0.1, moon: 0, clouds: 0.9, search: 1, exposure: 0.95, bloom: 0.52,
  },
]

// The sun has just set behind the skyline the dusk shot looks at (towards -z).
export const SUN_DIRECTION = new THREE.Vector3(0.12, -0.02, -1).normalize()
export const MOON_DIRECTION = new THREE.Vector3(0.45, 0.42, -0.79).normalize()

/** Neon palette shared by signs, shopfronts, tower edges and screens (linear HDR-ready). */
export const NEON_COLORS = [
  new THREE.Color().setRGB(1, 0.12, 0.55),
  new THREE.Color().setRGB(0.08, 0.8, 1),
  new THREE.Color().setRGB(1, 0.5, 0.1),
  new THREE.Color().setRGB(0.55, 0.25, 1),
  new THREE.Color().setRGB(0.25, 1, 0.5),
  new THREE.Color().setRGB(1, 0.16, 0.12),
]

/** Hash without sine, Dave Hoskins (MIT), https://www.shadertoy.com/view/4djSRW */
export const HASH_GLSL = /* glsl */ `
float hash12(vec2 p) {
  vec3 p3 = fract(vec3(p.xyx) * 0.1031);
  p3 += dot(p3, p3.yzx + 33.33);
  return fract((p3.x + p3.y) * p3.z);
}
float hash13(vec3 p3) {
  p3 = fract(p3 * 0.1031);
  p3 += dot(p3, p3.zyx + 31.32);
  return fract((p3.x + p3.y) * p3.z);
}
`

/** Exponential-squared haze that hugs the ground. Needs uFogColor and uFogDensity. */
export const FOG_GLSL = /* glsl */ `
uniform vec3 uFogColor;
uniform float uFogDensity;
float fogAmount(vec3 world) {
  float dist = length(world - cameraPosition);
  float fog = 1.0 - exp(-pow(uFogDensity * dist, 2.0));
  return clamp(fog * mix(0.6, 1.0, exp(-max(world.y, 0.0) * 0.004)), 0.0, 1.0);
}
vec3 applyFog(vec3 color, vec3 world) {
  return mix(color, uFogColor, fogAmount(world));
}
`

function color(value: [number, number, number]): THREE.Color {
  return new THREE.Color().setRGB(value[0], value[1], value[2])
}

export function createAtmosphere() {
  const uniforms = {
    uTime: { value: 0 },
    uSkyTop: { value: new THREE.Color() },
    uSkyMid: { value: new THREE.Color() },
    uHorizon: { value: new THREE.Color() },
    uGlow: { value: new THREE.Color() },
    uFogColor: { value: new THREE.Color() },
    uFogDensity: { value: 0 },
    uSunDir: { value: SUN_DIRECTION },
    uSunColor: { value: new THREE.Color() },
    uSun: { value: 0 },
    uMoonDir: { value: MOON_DIRECTION },
    uMoon: { value: 0 },
    uAmbient: { value: 0 },
    uLit: { value: 0 },
    uNeon: { value: 0 },
    uLamps: { value: 0 },
    uRain: { value: 0 },
    uWet: { value: 0 },
    uStars: { value: 0 },
    uClouds: { value: 0 },
    uSearch: { value: 0 },
    uNeonColors: { value: NEON_COLORS },
    /** World size of one pixel per unit of distance: keeps far lights at least a pixel or two wide. */
    uPixelAngle: { value: 0.001 },
    /** Pixels per unit of tan(angle): turns a world size at a distance into a point size. */
    uPointScale: { value: 800 },
  }
  const grade = { exposure: 1, bloom: 0.6 }
  const scratch = new THREE.Color()

  function blendColor(target: THREE.Color, a: [number, number, number], b: [number, number, number], t: number): void {
    target.copy(color(a)).lerp(scratch.copy(color(b)), t)
  }

  return {
    uniforms,
    grade,
    update(tod: number, time: number): void {
      const clamped = Math.min(2, Math.max(0, tod))
      const index = Math.min(1, Math.floor(clamped))
      const raw = clamped - index
      const t = raw * raw * (3 - 2 * raw)
      const a = LOOKS[index]!
      const b = LOOKS[index + 1]!
      const mix = (x: number, y: number) => x + (y - x) * t
      uniforms.uTime.value = time
      blendColor(uniforms.uSkyTop.value, a.skyTop, b.skyTop, t)
      blendColor(uniforms.uSkyMid.value, a.skyMid, b.skyMid, t)
      blendColor(uniforms.uHorizon.value, a.horizon, b.horizon, t)
      blendColor(uniforms.uGlow.value, a.glow, b.glow, t)
      blendColor(uniforms.uFogColor.value, a.fog, b.fog, t)
      blendColor(uniforms.uSunColor.value, a.sunColor, b.sunColor, t)
      uniforms.uFogDensity.value = mix(a.fogDensity, b.fogDensity)
      uniforms.uSun.value = mix(a.sun, b.sun)
      uniforms.uMoon.value = mix(a.moon, b.moon)
      uniforms.uAmbient.value = mix(a.ambient, b.ambient)
      uniforms.uLit.value = mix(a.lit, b.lit)
      uniforms.uNeon.value = mix(a.neon, b.neon)
      uniforms.uLamps.value = mix(a.lamps, b.lamps)
      uniforms.uRain.value = mix(a.rain, b.rain)
      uniforms.uWet.value = mix(a.wet, b.wet)
      uniforms.uStars.value = mix(a.stars, b.stars)
      uniforms.uClouds.value = mix(a.clouds, b.clouds)
      uniforms.uSearch.value = mix(a.search, b.search)
      grade.exposure = mix(a.exposure, b.exposure)
      grade.bloom = mix(a.bloom, b.bloom)
    },
  }
}

export type Atmosphere = ReturnType<typeof createAtmosphere>
export type AtmosphereUniforms = Atmosphere['uniforms']
