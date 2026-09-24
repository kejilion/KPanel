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
  /** Called when the camera sets off for this shot (and for the first shot at start). */
  enter?: () => void
  /** A moving shot: position and target from the seconds since the camera set off for it. */
  track?: (elapsed: number, position: THREE.Vector3, target: THREE.Vector3) => void
}

interface Pose {
  position: THREE.Vector3
  target: THREE.Vector3
  fov: number
}

export interface DirectorFrame {
  /** 0 = black, 1 = full exposure; the entrance fades up from black. */
  fade: number
}

export const ENTRANCE_SECONDS = 6.5
const FADE_SECONDS = 3.2
/** Moves are short: the shots sit close together around the headland, a few hundred metres apart. */
const TRANSITION_MIN_SECONDS = 5
const TRANSITION_MAX_SECONDS = 8
const TRANSITION_METERS_PER_SECOND = 80
const HOLD_SECONDS = 26
/** The camera's path between shots arcs up just enough to stay this far above the ground. */
const CLEARANCE = 14

const easeOut = (t: number) => 1 - (1 - t) ** 3
const smoothstep = (t: number) => t * t * (3 - 2 * t)
const smootherstep = (t: number) => t * t * t * (t * (t * 6 - 15) + 10)
const clamp01 = (t: number) => Math.min(1, Math.max(0, t))

function copyPose(pose: Pose): Pose {
  return { position: pose.position.clone(), target: pose.target.clone(), fov: pose.fov }
}

function viewDirection(pose: Pose): THREE.Vector3 {
  return pose.target.clone().sub(pose.position).normalize()
}

/** Turns the view from one direction to another (t in 0-1). */
function turn(from: THREE.Vector3, to: THREE.Vector3, t: number, out: THREE.Vector3): THREE.Vector3 {
  const rotation = new THREE.Quaternion().setFromUnitVectors(from, to)
  return out.copy(from).applyQuaternion(new THREE.Quaternion().slerp(rotation, t))
}

function bezier(a: THREE.Vector3, control: THREE.Vector3, b: THREE.Vector3, t: number, out: THREE.Vector3): THREE.Vector3 {
  const u = 1 - t
  return out.set(0, 0, 0).addScaledVector(a, u * u).addScaledVector(control, 2 * u * t).addScaledVector(b, t * t)
}

/**
 * Runs the entrance, the idle camera and the moves between shots. A move is one
 * short, direct arc from shot to shot, like the orbital station's: it rises only
 * as far as it must to clear the cliffs, and turns the view as it goes.
 */
export class Director {
  readonly shots: readonly Shot[]
  private readonly camera: THREE.PerspectiveCamera
  private readonly ground: (x: number, z: number) => number
  private current = 0
  private entranceTime = 0
  private readonly entranceStart: Pose
  private transition?: { from: Pose, control: THREE.Vector3, to: number, time: number, duration: number }
  private holdTime = 0
  private time = 0
  private readonly startedAt: number[] = []
  private readonly pose: Pose = { position: new THREE.Vector3(), target: new THREE.Vector3(), fov: 45 }
  private readonly look = new THREE.Vector3()
  private readonly point = new THREE.Vector3()
  onShotChange?: (index: number) => void

  constructor(camera: THREE.PerspectiveCamera, shots: readonly Shot[], options: { entrance: boolean, shot?: number, ground: (x: number, z: number) => number }) {
    this.camera = camera
    this.shots = shots
    this.ground = options.ground
    this.current = options.shot ?? 0
    this.startedAt[this.current] = 0
    shots[this.current]!.enter?.()
    // The entrance glides in from further back and higher, with a slightly wider lens.
    const first = this.idlePose(this.current, 0)
    const back = first.position.clone().sub(first.target).setLength(Math.min(first.position.distanceTo(first.target), 400))
    this.entranceStart = {
      position: first.position.clone().addScaledVector(back, 0.3).add(new THREE.Vector3(0, back.length() * 0.1, 0)),
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
    this.startedAt[index] = this.time
    this.shots[index]!.enter?.()
    const from = copyPose(this.pose)
    const next = this.idlePose(index, this.time)
    const distance = from.position.distanceTo(next.position)
    const duration = THREE.MathUtils.clamp(3.5 + distance / TRANSITION_METERS_PER_SECOND, TRANSITION_MIN_SECONDS, TRANSITION_MAX_SECONDS)
    // Raise the arc's control point until the whole path clears the ground; near either end it
    // may be as low as that shot itself. (A tracked shot moves on a little during the move.)
    const fromClear = Math.max(0, from.position.y - this.ground(from.position.x, from.position.z))
    const toClear = Math.max(0, next.position.y - this.ground(next.position.x, next.position.z))
    const control = from.position.clone().lerp(next.position, 0.5)
    control.y = Math.max(control.y, from.position.y, next.position.y)
    for (let lift = 0; lift < 400; lift += 5) {
      let clear = true
      for (let step = 1; step < 32 && clear; step++) {
        const t = step / 32
        const needed = Math.min(CLEARANCE, THREE.MathUtils.lerp(fromClear, toClear, t) + CLEARANCE * Math.sin(Math.PI * t))
        bezier(from.position, control, next.position, t, this.point)
        clear = this.point.y >= this.ground(this.point.x, this.point.z) + needed
      }
      if (clear) break
      control.y += 5
    }
    this.transition = { from, control, to: index, time: 0, duration }
    this.holdTime = 0
    this.onShotChange?.(index)
  }

  private idlePose(index: number, time: number): Pose {
    const shot = this.shots[index]!
    if (shot.track) {
      const pose = { position: new THREE.Vector3(), target: new THREE.Vector3(), fov: shot.fov }
      shot.track(Math.max(0, time - (this.startedAt[index] ?? 0)), pose.position, pose.target)
      return pose
    }
    const offset = shot.position.clone().sub(shot.target)
    offset.applyAxisAngle(new THREE.Vector3(0, 1, 0), Math.sin(time * 0.04) * shot.orbit)
    const position = shot.target.clone().add(offset)
    position.y += Math.sin(time * 0.11) * shot.float
    return { position, target: shot.target.clone(), fov: shot.fov }
  }

  update(dt: number, time: number): DirectorFrame {
    this.time = time
    const frame: DirectorFrame = { fade: 1 }
    if (this.entranceTime < ENTRANCE_SECONDS) {
      this.entranceTime = Math.min(ENTRANCE_SECONDS, this.entranceTime + dt)
      const t = this.entranceTime
      const final = this.idlePose(this.current, time)
      const settle = easeOut(clamp01(t / ENTRANCE_SECONDS))
      this.pose.position.copy(this.entranceStart.position).lerp(final.position, settle)
      this.pose.target.copy(final.target)
      this.pose.fov = THREE.MathUtils.lerp(this.entranceStart.fov, final.fov, settle)
      frame.fade = smoothstep(clamp01(t / FADE_SECONDS))
    } else if (this.transition) {
      this.transition.time += dt
      const progress = clamp01(this.transition.time / this.transition.duration)
      const to = this.idlePose(this.transition.to, time)
      const eased = smootherstep(progress)
      const from = this.transition.from
      bezier(from.position, this.transition.control, to.position, eased, this.pose.position)
      turn(viewDirection(from), viewDirection(to), eased, this.look)
      this.pose.target.copy(this.pose.position).addScaledVector(this.look, 400)
      this.pose.fov = THREE.MathUtils.lerp(from.fov, to.fov, eased)
      if (progress >= 1) {
        this.current = this.transition.to
        this.transition = undefined
      }
    } else {
      Object.assign(this.pose, this.idlePose(this.current, time))
      this.holdTime += dt
      if (this.holdTime >= HOLD_SECONDS) this.cut()
    }
    this.camera.position.copy(this.pose.position)
    this.camera.lookAt(this.pose.target)
    if (Math.abs(this.camera.fov - this.pose.fov) > 0.01) {
      this.camera.fov = this.pose.fov
      this.camera.updateProjectionMatrix()
    }
    return frame
  }
}
