import * as THREE from 'three'

export interface Shot {
  id: string
  position: THREE.Vector3
  target: THREE.Vector3
  fov: number
  /** Idle orbit around the target, in radians either side. */
  orbit: number
  /** Idle vertical float, in world units. */
  float: number
  /** Time of day for this shot: 0 dusk, 1 blue hour, 2 rainy night. */
  tod: number
}

interface Pose {
  position: THREE.Vector3
  target: THREE.Vector3
  fov: number
}

export interface DirectorFrame {
  /** 0 = black, 1 = full exposure; the entrance fades up from black. */
  fade: number
  tod: number
}

export const ENTRANCE_SECONDS = 6.5
/** The picture comes up at once and settles, rather than lingering near black for its first second. */
const FADE_SECONDS = 2.2
const fadeIn = (t: number) => 1 - (1 - t) ** 2
/** Moves take longer the further they fly and the more they turn, so the camera never rushes. */
const TRANSITION_MIN_SECONDS = 10
const TRANSITION_MAX_SECONDS = 24
const TRANSITION_METERS_PER_SECOND = 55
const TRANSITION_DEGREES_PER_SECOND = 15
const HOLD_SECONDS = 28
/** Camera moves between shots cross the city at this height, above every rooftop on their way. */
const CLEARANCE = 195

const easeOut = (t: number) => 1 - (1 - t) ** 3
const smoothstep = (t: number) => t * t * (3 - 2 * t)
const clamp01 = (t: number) => Math.min(1, Math.max(0, t))

function copyPose(pose: Pose): Pose {
  return { position: pose.position.clone(), target: pose.target.clone(), fov: pose.fov }
}

function viewDirection(pose: Pose): THREE.Vector3 {
  return pose.target.clone().sub(pose.position).normalize()
}

/** Turns the view from one direction to another at a steady rate (t in 0-1). */
function turn(from: THREE.Vector3, to: THREE.Vector3, t: number, out: THREE.Vector3): THREE.Vector3 {
  const rotation = new THREE.Quaternion().setFromUnitVectors(from, to)
  return out.copy(from).applyQuaternion(new THREE.Quaternion().slerp(rotation, t))
}

/**
 * Runs the entrance, the idle camera and the moves between shots. A move is a
 * drone flight: it climbs straight up to cruising height, crosses the city above
 * the rooftops and comes straight down onto the next shot (the three phases
 * overlap, so the motion stays smooth), while the time of day blends along.
 */
export class Director {
  readonly shots: readonly Shot[]
  private readonly camera: THREE.PerspectiveCamera
  private current = 0
  private entranceTime = 0
  private readonly entranceStart: Pose
  private transition?: { from: Pose, fromTod: number, to: number, time: number, duration: number }
  private holdTime = 0
  private tod: number
  private readonly pose: Pose = { position: new THREE.Vector3(), target: new THREE.Vector3(), fov: 45 }
  private readonly look = new THREE.Vector3()
  onShotChange?: (index: number) => void

  constructor(camera: THREE.PerspectiveCamera, shots: readonly Shot[], options: { entrance: boolean, shot?: number }) {
    this.camera = camera
    this.shots = shots
    this.current = options.shot ?? 0
    this.tod = shots[this.current]!.tod
    // The entrance glides in from further back and higher, with a slightly wider lens.
    const first = this.idlePose(this.current, 0)
    const back = first.position.clone().sub(first.target)
    this.entranceStart = {
      position: first.position.clone().addScaledVector(back, 0.35).add(new THREE.Vector3(0, back.length() * 0.12, 0)),
      target: first.target.clone(),
      fov: first.fov + 6,
    }
    this.entranceTime = options.entrance ? 0 : ENTRANCE_SECONDS
  }

  get shotIndex(): number {
    return this.transition?.to ?? this.current
  }

  /** Moves to the next shot (or a given one). */
  cut(index = (this.shotIndex + 1) % this.shots.length): void {
    if (this.entranceTime < ENTRANCE_SECONDS || index === this.shotIndex || !this.shots[index]) return
    const next = this.shots[index]!
    const distance = Math.hypot(next.position.x - this.pose.position.x, next.position.z - this.pose.position.z)
    const angle = THREE.MathUtils.radToDeg(viewDirection(this.pose).angleTo(next.target.clone().sub(next.position).normalize()))
    const duration = THREE.MathUtils.clamp(
      5 + distance / TRANSITION_METERS_PER_SECOND + angle / TRANSITION_DEGREES_PER_SECOND,
      TRANSITION_MIN_SECONDS,
      TRANSITION_MAX_SECONDS,
    )
    this.transition = { from: copyPose(this.pose), fromTod: this.tod, to: index, time: 0, duration }
    this.holdTime = 0
    this.onShotChange?.(index)
  }

  private idlePose(index: number, time: number): Pose {
    const shot = this.shots[index]!
    const offset = shot.position.clone().sub(shot.target)
    offset.applyAxisAngle(new THREE.Vector3(0, 1, 0), Math.sin(time * 0.04) * shot.orbit)
    const position = shot.target.clone().add(offset)
    position.y += Math.sin(time * 0.11) * shot.float
    return { position, target: shot.target.clone(), fov: shot.fov }
  }

  update(dt: number, time: number): DirectorFrame {
    const frame: DirectorFrame = { fade: 1, tod: this.tod }
    if (this.entranceTime < ENTRANCE_SECONDS) {
      this.entranceTime = Math.min(ENTRANCE_SECONDS, this.entranceTime + dt)
      const t = this.entranceTime
      const final = this.idlePose(this.current, time)
      const settle = easeOut(clamp01(t / ENTRANCE_SECONDS))
      this.pose.position.copy(this.entranceStart.position).lerp(final.position, settle)
      this.pose.target.copy(final.target)
      this.pose.fov = THREE.MathUtils.lerp(this.entranceStart.fov, final.fov, settle)
      frame.fade = fadeIn(clamp01(t / FADE_SECONDS))
    } else if (this.transition) {
      this.transition.time += dt
      const progress = clamp01(this.transition.time / this.transition.duration)
      const to = this.idlePose(this.transition.to, time)
      const eased = smoothstep(progress)
      const from = this.transition.from
      const cruise = Math.max(from.position.y, to.position.y, CLEARANCE)
      // Each phase eases on its own over plain time; easing the whole move as well would
      // squeeze the flight across the city into a second or two.
      const rise = smoothstep(clamp01(progress / 0.35))
      const across = smoothstep(clamp01((progress - 0.18) / 0.64))
      const fall = smoothstep(clamp01((progress - 0.65) / 0.35))
      this.pose.position.set(
        THREE.MathUtils.lerp(from.position.x, to.position.x, across),
        THREE.MathUtils.lerp(THREE.MathUtils.lerp(from.position.y, cruise, rise), to.position.y, fall),
        THREE.MathUtils.lerp(from.position.z, to.position.z, across),
      )
      turn(viewDirection(from), viewDirection(to), eased, this.look)
      this.pose.target.copy(this.pose.position).addScaledVector(this.look, 400)
      this.pose.fov = THREE.MathUtils.lerp(from.fov, to.fov, eased)
      this.tod = THREE.MathUtils.lerp(this.transition.fromTod, this.shots[this.transition.to]!.tod, eased)
      if (progress >= 1) {
        this.current = this.transition.to
        this.transition = undefined
      }
    } else {
      Object.assign(this.pose, this.idlePose(this.current, time))
      this.holdTime += dt
      if (this.holdTime >= HOLD_SECONDS) this.cut()
    }
    frame.tod = this.tod
    this.camera.position.copy(this.pose.position)
    this.camera.lookAt(this.pose.target)
    if (Math.abs(this.camera.fov - this.pose.fov) > 0.01) {
      this.camera.fov = this.pose.fov
      this.camera.updateProjectionMatrix()
    }
    return frame
  }
}
