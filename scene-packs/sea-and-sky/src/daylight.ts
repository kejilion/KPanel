import * as THREE from 'three'

/**
 * Automatic day and night from the local clock: the sun rises in the east,
 * crosses the south and sets in the west (north is -z, east is +x). Sky, light
 * and clouds follow its elevation; at night the moon takes over as the key
 * light and the lamps come on along the coast. The moon keeps its real phase
 * and rises later each day, so it is sometimes up by day as a pale disc and
 * some nights are moonless. Nothing depends on the camera.
 */
const LATITUDE = THREE.MathUtils.degToRad(32)
const DECLINATION = THREE.MathUtils.degToRad(8)
/** Local hours of sunrise and sunset at this latitude and declination. */
const DAY_HALF = THREE.MathUtils.radToDeg(Math.acos(-Math.tan(LATITUDE) * Math.tan(DECLINATION))) / 15
export const SUNRISE = 12 - DAY_HALF
export const SUNSET = 12 + DAY_HALF
const SYNODIC_DAYS = 29.530588853
/** A new moon: 2000-01-06 18:14 UTC. */
const NEW_MOON_EPOCH = Date.UTC(2000, 0, 6, 18, 14)

type RGB = [number, number, number]

interface Stop {
  elevation: number
  skyTop: RGB
  skyHorizon: RGB
  /** Key light colour (sun or moon) times intensity. */
  light: RGB
  /** Ambient light from the sky above and from the cloud sea below. */
  ambientTop: RGB
  ambientBottom: RGB
  /** Glow around the sun on the horizon. */
  glow: RGB
}

// Keyed by sun elevation in degrees; colours are linear and meant for ACES tone mapping.
const STOPS: readonly Stop[] = [
  { elevation: -18, skyTop: [0.004, 0.007, 0.02], skyHorizon: [0.012, 0.02, 0.045], light: [0.22, 0.28, 0.42], ambientTop: [0.018, 0.026, 0.05], ambientBottom: [0.012, 0.016, 0.028], glow: [0, 0, 0] },
  { elevation: -8, skyTop: [0.012, 0.022, 0.07], skyHorizon: [0.06, 0.06, 0.12], light: [0.18, 0.22, 0.34], ambientTop: [0.04, 0.05, 0.1], ambientBottom: [0.03, 0.03, 0.05], glow: [0.18, 0.06, 0.06] },
  // The key light hands over from moon to sun here, so it is at its faintest.
  { elevation: -5, skyTop: [0.025, 0.04, 0.12], skyHorizon: [0.2, 0.12, 0.15], light: [0.05, 0.05, 0.07], ambientTop: [0.06, 0.07, 0.14], ambientBottom: [0.05, 0.045, 0.06], glow: [0.55, 0.18, 0.1] },
  { elevation: -2, skyTop: [0.05, 0.08, 0.2], skyHorizon: [0.5, 0.26, 0.2], light: [1.1, 0.42, 0.2], ambientTop: [0.1, 0.12, 0.22], ambientBottom: [0.1, 0.08, 0.09], glow: [1.2, 0.4, 0.15] },
  { elevation: 5, skyTop: [0.1, 0.2, 0.45], skyHorizon: [0.85, 0.55, 0.36], light: [2.2, 1.25, 0.65], ambientTop: [0.2, 0.26, 0.42], ambientBottom: [0.22, 0.2, 0.2], glow: [1.1, 0.55, 0.25] },
  // Golden hour: warm light and a honeyed horizon before the sky turns blue.
  { elevation: 10, skyTop: [0.12, 0.24, 0.52], skyHorizon: [0.82, 0.6, 0.42], light: [2.5, 1.7, 1.0], ambientTop: [0.26, 0.32, 0.5], ambientBottom: [0.3, 0.26, 0.24], glow: [0.85, 0.48, 0.2] },
  { elevation: 18, skyTop: [0.1, 0.27, 0.62], skyHorizon: [0.55, 0.68, 0.85], light: [2.3, 2.05, 1.75], ambientTop: [0.3, 0.42, 0.66], ambientBottom: [0.36, 0.38, 0.42], glow: [0.35, 0.3, 0.22] },
  { elevation: 60, skyTop: [0.08, 0.25, 0.62], skyHorizon: [0.5, 0.66, 0.86], light: [2.4, 2.3, 2.1], ambientTop: [0.32, 0.46, 0.72], ambientBottom: [0.4, 0.42, 0.46], glow: [0.2, 0.2, 0.18] },
]

export interface Daylight {
  /** Direction towards the key light (the sun by day, the moon by night). */
  lightDir: THREE.Vector3
  /** 1 while the key light's source is in the sky (always by day; at night, while the moon is up). */
  keyVisible: number
  sunDir: THREE.Vector3
  moonDir: THREE.Vector3
  light: THREE.Color
  skyTop: THREE.Color
  skyHorizon: THREE.Color
  ambientTop: THREE.Color
  ambientBottom: THREE.Color
  glow: THREE.Color
  /** 0 by day, 1 at full night: lamps, fireflies and stars. */
  night: number
  /** Sun elevation in degrees. */
  elevation: number
  /** Turns a direction in the local sky (east +x, up +y, south +z) into equatorial coordinates. */
  celestial: THREE.Matrix3
  /** Moon elevation in degrees, and the lit fraction of its disc (0 new, 1 full). */
  moonElevation: number
  moonIllumination: number
  /** Camera exposure: lower in bright daylight, higher at night. */
  exposure: number
}

/** Local solar time in hours (0-24), from the clock or a ?hour= override for previews. */
export function localHour(date: Date, override: number | undefined): number {
  if (override !== undefined) return override
  return date.getHours() + date.getMinutes() / 60 + date.getSeconds() / 3600
}

/** Days since the last new moon, from the date or a ?moon= override for previews. */
export function moonAge(date: Date, override: number | undefined): number {
  const days = override ?? (date.getTime() - NEW_MOON_EPOCH) / 86_400_000
  return ((days % SYNODIC_DAYS) + SYNODIC_DAYS) % SYNODIC_DAYS
}

function sunDirection(hour: number, out: THREE.Vector3): THREE.Vector3 {
  const hourAngle = THREE.MathUtils.degToRad((hour - 12) * 15)
  const sinElevation = Math.sin(LATITUDE) * Math.sin(DECLINATION) + Math.cos(LATITUDE) * Math.cos(DECLINATION) * Math.cos(hourAngle)
  const elevation = Math.asin(sinElevation)
  const cosAzimuth = (Math.sin(DECLINATION) - Math.sin(elevation) * Math.sin(LATITUDE)) / (Math.cos(elevation) * Math.cos(LATITUDE))
  let azimuth = Math.acos(THREE.MathUtils.clamp(cosAzimuth, -1, 1)) // from north, towards east
  if (hourAngle > 0) azimuth = Math.PI * 2 - azimuth
  return out.set(Math.sin(azimuth) * Math.cos(elevation), Math.sin(elevation), -Math.cos(azimuth) * Math.cos(elevation))
}

function blend(target: THREE.Color, key: keyof Omit<Stop, 'elevation'>, elevation: number): void {
  const last = STOPS[STOPS.length - 1]!
  if (elevation <= STOPS[0]!.elevation) { target.setRGB(...STOPS[0]![key]); return }
  if (elevation >= last.elevation) { target.setRGB(...last[key]); return }
  for (let index = 0; index < STOPS.length - 1; index++) {
    const a = STOPS[index]!
    const b = STOPS[index + 1]!
    if (elevation <= b.elevation) {
      const t = (elevation - a.elevation) / (b.elevation - a.elevation)
      const s = t * t * (3 - 2 * t)
      target.setRGB(
        a[key][0] + (b[key][0] - a[key][0]) * s,
        a[key][1] + (b[key][1] - a[key][1]) * s,
        a[key][2] + (b[key][2] - a[key][2]) * s,
      )
      return
    }
  }
}

const phantom = new THREE.Vector3()

/** Day of the year (1-366), for where the stars stand at a given hour. */
export function dayOfYear(date: Date): number {
  return Math.floor((Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()) - Date.UTC(date.getFullYear(), 0, 0)) / 86_400_000)
}

export function createDaylight(): Daylight & { update(hour: number, age: number, day: number): void } {
  const state = {
    lightDir: new THREE.Vector3(),
    keyVisible: 1,
    celestial: new THREE.Matrix3(),
    sunDir: new THREE.Vector3(),
    moonDir: new THREE.Vector3(),
    light: new THREE.Color(),
    skyTop: new THREE.Color(),
    skyHorizon: new THREE.Color(),
    ambientTop: new THREE.Color(),
    ambientBottom: new THREE.Color(),
    glow: new THREE.Color(),
    night: 0,
    elevation: 0,
    moonElevation: 0,
    moonIllumination: 1,
    exposure: 1,
    update(hour: number, age: number, day: number): void {
      sunDirection(hour, state.sunDir)
      // Local sidereal time: the sun's right ascension (0h at the March equinox) plus its hour angle.
      const sidereal = THREE.MathUtils.degToRad(((hour - 12 + (24 * (day - 80)) / 365.25) % 24) * 15)
      const sl = Math.sin(sidereal)
      const cl = Math.cos(sidereal)
      const sp = Math.sin(LATITUDE)
      const cp = Math.cos(LATITUDE)
      state.celestial.set(
        -sl, cl * cp, cl * sp,
        cl, sl * cp, sl * sp,
        0, sp, -cp,
      )
      // The moon trails the sun across the sky by its age: 12 hours at full moon.
      sunDirection((((hour - ((age % SYNODIC_DAYS) / SYNODIC_DAYS) * 24) % 24) + 24) % 24, state.moonDir)
      state.moonElevation = THREE.MathUtils.radToDeg(Math.asin(state.moonDir.y))
      state.moonIllumination = (1 - Math.cos((age / SYNODIC_DAYS) * Math.PI * 2)) / 2
      const elevation = THREE.MathUtils.radToDeg(Math.asin(state.sunDir.y))
      state.elevation = elevation
      state.night = 1 - THREE.MathUtils.smoothstep(elevation, -10, -3)
      for (const key of ['skyTop', 'skyHorizon', 'light', 'ambientTop', 'ambientBottom', 'glow'] as const) blend(state[key], key, elevation)
      // Hand the key light from the sun to the moon deep in twilight, where both are faint.
      if (elevation > -5) {
        state.lightDir.copy(state.sunDir)
        // Its glitter on the sea fades out as the sun goes down.
        state.keyVisible = THREE.MathUtils.smoothstep(elevation, -5, 0.5)
        if (state.lightDir.y < 0.02) {
          // Keep the grazing sun just above the horizon so clouds still catch it at sunrise and sunset.
          state.lightDir.y = 0.02
          state.lightDir.normalize()
        }
      } else {
        // Moonlight follows the moon's phase and height; with the moon down, a faint light from
        // high in the same quarter of the sky stands in for the starlit sky, so land stays legible.
        const up = THREE.MathUtils.smoothstep(state.moonElevation, -2, 8)
        state.keyVisible = up
        phantom.set(state.moonDir.x, 0, state.moonDir.z).normalize().setY(1.2).normalize()
        state.lightDir.copy(phantom).lerp(state.moonDir, up).normalize()
        if (state.lightDir.y < 0.03) { state.lightDir.y = 0.03; state.lightDir.normalize() }
        state.light.multiplyScalar(0.3 + 0.7 * up * Math.sqrt(state.moonIllumination))
      }
      const daytime = THREE.MathUtils.smoothstep(elevation, 2, 25)
      // Kept a little low by day, so the scene sits calmly behind the desktop in either theme.
      state.exposure = THREE.MathUtils.lerp(THREE.MathUtils.lerp(0.9, 1.25, state.night), 0.54, daytime)
    },
  }
  return state
}
