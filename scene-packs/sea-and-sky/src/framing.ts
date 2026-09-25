import * as THREE from 'three'
import type { Daylight } from './daylight'

/**
 * What the camera looks at, and how it follows it. The subject is whatever
 * lights the scene, with the moments worth seeing first: a sunrise or sunset,
 * then a moonrise or moonset (by day too), then the sun, the moon, and on a
 * moonless night the Milky Way. When the sun and moon are both at the horizon
 * (a full moon rising as the sun sets), the second one is kept for the wide
 * shot, so both get seen.
 *
 * Everything that moves the camera goes through two critically damped springs
 * in a row, so not only its speed but its acceleration changes smoothly: no
 * sudden starts, stops or jolts, whether the sun is creeping across the sky or
 * the subject changes from one side of it to the other.
 */
export interface SpringState {
  value: number
  velocity: number
}

export function spring(state: SpringState, goal: number, omega: number, dt: number): void {
  const offset = state.value - goal
  const decay = Math.exp(-omega * dt)
  const push = (state.velocity + omega * offset) * dt
  state.value = goal + (offset + push) * decay
  state.velocity = (state.velocity - omega * push) * decay
}

const degrees = THREE.MathUtils.degToRad
/** The galactic pole and centre on the celestial sphere (equatorial unit vectors, as in the sky shader). */
const GALACTIC_POLE = new THREE.Vector3(-0.8677, -0.1981, 0.456)
const GALACTIC_CENTRE = new THREE.Vector3(-0.0549, -0.8734, -0.4839)

/** Two springs in a row: the second follows the first, so the output's acceleration is continuous. */
class SmoothValue {
  private readonly lead: SpringState = { value: 0, velocity: 0 }
  private readonly trail: SpringState = { value: 0, velocity: 0 }
  constructor(private readonly omega: number) {}
  get value(): number {
    return this.trail.value
  }
  update(goal: number, dt: number): void {
    spring(this.lead, goal, this.omega, dt)
    spring(this.trail, this.lead.value, this.omega, dt)
  }
  snap(value: number): void {
    Object.assign(this.lead, { value, velocity: 0 })
    Object.assign(this.trail, { value, velocity: 0 })
  }
}

/** A followed direction, with the lens and tilt that keep it in frame above the horizon. */
export class Framing {
  readonly heading = new THREE.Vector3(0, 0, -1)
  // Swinging round to a new subject takes a good ten to fifteen seconds; the lens and tilt follow
  // the (already smooth) elevation, and only need smoothing over their corners.
  private readonly yaw = new SmoothValue(0.55)
  private readonly rise = new SmoothValue(0.55)
  private readonly lens = new SmoothValue(1.2)
  private readonly tilt = new SmoothValue(1.2)

  /** Elevation of the subject, radians. */
  get elevation(): number {
    return this.rise.value
  }

  /** Vertical field of view, degrees: wider for a subject high in the sky. */
  get fov(): number {
    return this.lens.value
  }

  /** Camera pitch, radians: the subject in the upper part of the frame, the horizon kept in. */
  get pitch(): number {
    return this.tilt.value
  }

  follow(direction: THREE.Vector3, dt: number, snap = false): void {
    const goalYaw = Math.atan2(direction.x, direction.z)
    const goalRise = Math.asin(THREE.MathUtils.clamp(direction.y, -1, 1))
    // The short way round from where the camera is now.
    const yaw = this.yaw.value + Math.atan2(Math.sin(goalYaw - this.yaw.value), Math.cos(goalYaw - this.yaw.value))
    if (snap) {
      this.yaw.snap(yaw)
      this.rise.snap(goalRise)
    } else {
      this.yaw.update(yaw, dt)
      this.rise.update(goalRise, dt)
    }
    const up = THREE.MathUtils.radToDeg(this.rise.value)
    const lens = THREE.MathUtils.clamp(up + 14, 48, 76)
    const tilt = degrees(up < 34 ? THREE.MathUtils.clamp(up - 11, 1.5, 19) : up / 2 - 1)
    if (snap) {
      this.lens.snap(lens)
      this.tilt.snap(tilt)
    } else {
      this.lens.update(lens, dt)
      this.tilt.update(tilt, dt)
    }
    this.heading.set(Math.sin(this.yaw.value), 0, Math.cos(this.yaw.value))
  }
}

/** Picks the subjects: the main one, and a second one for the wide shot (often the same). */
export function createSubjects(daylight: Daylight) {
  const primary = new THREE.Vector3()
  const secondary = new THREE.Vector3()
  const toLocal = new THREE.Matrix3()
  const core = new THREE.Vector3()
  const pole = new THREE.Vector3()
  return {
    primary,
    secondary,
    update(): void {
      const sunEvent = daylight.elevation > -4 && daylight.elevation < 14
      const moonVisible = daylight.moonIllumination > 0.08
      const moonEvent = moonVisible && daylight.moonElevation > -1 && daylight.moonElevation < 14
      if (sunEvent) {
        primary.copy(daylight.sunDir)
      } else if (moonEvent) {
        primary.copy(daylight.moonDir)
      } else if (daylight.elevation > -10) {
        primary.copy(daylight.sunDir)
      } else if (moonVisible && daylight.moonElevation > 2) {
        primary.copy(daylight.moonDir)
      } else {
        // A moonless night: the core of the Milky Way if it is up, otherwise where the band
        // rises from the horizon on the side nearer the core.
        toLocal.copy(daylight.celestial).transpose()
        core.copy(GALACTIC_CENTRE).applyMatrix3(toLocal)
        if (core.y > 0.1) {
          primary.copy(core)
        } else {
          pole.copy(GALACTIC_POLE).applyMatrix3(toLocal)
          primary.crossVectors(pole, new THREE.Vector3(0, 1, 0)).normalize()
          if (primary.dot(core) < 0) primary.negate()
          primary.setY(0.35).normalize()
        }
      }
      const apart = daylight.sunDir.angleTo(daylight.moonDir) > degrees(40)
      secondary.copy(sunEvent && moonEvent && apart ? daylight.moonDir : primary)
    },
  }
}
