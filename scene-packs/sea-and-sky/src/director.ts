import * as THREE from 'three'

/**
 * A shot is a camera that keeps moving on its own: every frame it says where it is, what it
 * looks at and with what lens, from the seconds since the camera set off for it. Its lens may
 * change from frame to frame (the director reads it every frame), smoothly, as the shot keeps
 * a moving sun or moon in frame.
 */
export interface Shot {
  id: string
  fov: number
  /** Called when the camera sets off for this shot (and for the first shot at start). */
  enter?: () => void
  track: (elapsed: number, position: THREE.Vector3, target: THREE.Vector3) => void
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
/** A move takes longer the further it goes and the more it turns, within these bounds. */
const TRANSITION_MIN_SECONDS = 5
const TRANSITION_MAX_SECONDS = 12
const TRANSITION_METERS_PER_SECOND = 50
const TRANSITION_DEGREES_PER_SECOND = 30
/** Each shot holds a minute and a half: the time-lapse itself keeps the picture moving. */
const HOLD_SECONDS = 90
/** A move rises just enough to stay this far above the sea and the stacks. */
const CLEARANCE = 20

const easeOut = (t: number) => 1 - (1 - t) ** 3
const smoothstep = (t: number) => t * t * (3 - 2 * t)
const smootherstep = (t: number) => t * t * t * (t * (t * 6 - 15) + 10)
const clamp01 = (t: number) => Math.min(1, Math.max(0, t))

function viewDirection(pose: Pose): THREE.Vector3 {
  return pose.target.clone().sub(pose.position).normalize()
}

/** How far the heading turns from one view direction to another, the short way round. */
function headingTurn(from: THREE.Vector3, to: THREE.Vector3): number {
  const turn = Math.atan2(to.x, to.z) - Math.atan2(from.x, from.z)
  return Math.atan2(Math.sin(turn), Math.cos(turn))
}

/**
 * Turns the view from one direction to another (t in 0-1): heading and pitch separately, so even
 * a wide turn swings along the horizon instead of through the zenith. The heading turns by
 * `amount`, which the caller keeps continuous, so a turn of nearly half a circle never flips
 * sides midway when its ends move.
 */
function turn(from: THREE.Vector3, to: THREE.Vector3, amount: number, t: number, out: THREE.Vector3): THREE.Vector3 {
  const heading = Math.atan2(from.x, from.z) + amount * t
  const pitch = THREE.MathUtils.lerp(Math.asin(THREE.MathUtils.clamp(from.y, -1, 1)), Math.asin(THREE.MathUtils.clamp(to.y, -1, 1)), t)
  return out.set(Math.sin(heading) * Math.cos(pitch), Math.sin(pitch), Math.cos(heading) * Math.cos(pitch))
}

/**
 * Runs the entrance, the shots and the moves between them. During a move both
 * shots keep running, and the camera eases from one to the other along a
 * gentle arc (raised only as far as it must be to clear the stacks), turning as
 * it goes; so nothing ever stops dead or jumps, however the shots are moving.
 */
export class Director {
  readonly shots: readonly Shot[]
  private readonly camera: THREE.PerspectiveCamera
  private readonly ground: (x: number, z: number) => number
  private current = 0
  private entranceTime = 0
  private readonly entranceOffset: THREE.Vector3
  /** from: the shot being left, or -1 if the camera was itself mid-move (then `still` holds its pose). */
  private transition?: { from: number, still: Pose, to: number, time: number, duration: number, lift: number, turn: number }
  private holdTime = 0
  private time = 0
  private readonly startedAt: number[] = []
  private readonly pose: Pose = { position: new THREE.Vector3(), target: new THREE.Vector3(), fov: 45 }
  private readonly fromPose: Pose = { position: new THREE.Vector3(), target: new THREE.Vector3(), fov: 45 }
  private readonly toPose: Pose = { position: new THREE.Vector3(), target: new THREE.Vector3(), fov: 45 }
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
    // The entrance glides in from further back and a little higher, with a slightly wider lens.
    this.shotPose(this.current, 0, this.toPose)
    const back = this.toPose.position.clone().sub(this.toPose.target).setY(0).setLength(60)
    this.entranceOffset = back.add(new THREE.Vector3(0, 8, 0))
    this.entranceTime = options.entrance ? 0 : ENTRANCE_SECONDS
  }

  get shotIndex(): number {
    return this.transition?.to ?? this.current
  }

  /** Moves to the next shot (or a given one). */
  cut(index = (this.shotIndex + 1) % this.shots.length): void {
    if (this.entranceTime < ENTRANCE_SECONDS || index === this.shotIndex || !this.shots[index]) return
    const from = this.transition ? -1 : this.current
    const still: Pose = { position: this.pose.position.clone(), target: this.pose.target.clone(), fov: this.pose.fov }
    this.startedAt[index] = this.time
    this.shots[index]!.enter?.()
    this.shotPose(index, this.time, this.toPose)
    const distance = still.position.distanceTo(this.toPose.position)
    const angle = THREE.MathUtils.radToDeg(viewDirection(still).angleTo(viewDirection(this.toPose)))
    const duration = THREE.MathUtils.clamp(
      3 + distance / TRANSITION_METERS_PER_SECOND + angle / TRANSITION_DEGREES_PER_SECOND,
      TRANSITION_MIN_SECONDS,
      TRANSITION_MAX_SECONDS,
    )
    // Raise the arc until the whole path clears the sea and the stacks; near either end it may be
    // as low as that shot itself.
    const fromClear = Math.max(0, still.position.y - this.ground(still.position.x, still.position.z))
    const toClear = Math.max(0, this.toPose.position.y - this.ground(this.toPose.position.x, this.toPose.position.z))
    let lift = 0
    for (; lift < 300; lift += 4) {
      let clear = true
      // Finely enough that even a small reef cannot slip between two samples.
      for (let step = 1; step < 128 && clear; step++) {
        const t = step / 128
        const needed = Math.min(CLEARANCE, THREE.MathUtils.lerp(fromClear, toClear, t) + CLEARANCE * Math.sin(Math.PI * t))
        this.point.lerpVectors(still.position, this.toPose.position, t)
        this.point.y += lift * Math.sin(Math.PI * t)
        clear = this.point.y >= this.ground(this.point.x, this.point.z) + needed
      }
      if (clear) break
    }
    this.transition = { from, still, to: index, time: 0, duration, lift, turn: headingTurn(viewDirection(still), viewDirection(this.toPose)) }
    this.holdTime = 0
    this.onShotChange?.(index)
  }

  private shotPose(index: number, time: number, out: Pose): Pose {
    const shot = this.shots[index]!
    shot.track(Math.max(0, time - (this.startedAt[index] ?? 0)), out.position, out.target)
    out.fov = shot.fov
    return out
  }

  update(dt: number, time: number): DirectorFrame {
    this.time = time
    const frame: DirectorFrame = { fade: 1 }
    if (this.entranceTime < ENTRANCE_SECONDS) {
      this.entranceTime = Math.min(ENTRANCE_SECONDS, this.entranceTime + dt)
      const t = this.entranceTime
      const settle = easeOut(clamp01(t / ENTRANCE_SECONDS))
      this.shotPose(this.current, time, this.pose)
      this.pose.position.addScaledVector(this.entranceOffset, 1 - settle)
      this.pose.fov += 6 * (1 - settle)
      frame.fade = smoothstep(clamp01(t / FADE_SECONDS))
    } else if (this.transition) {
      const move = this.transition
      move.time += dt
      const progress = clamp01(move.time / move.duration)
      const eased = smootherstep(progress)
      const from = move.from >= 0 ? this.shotPose(move.from, time, this.fromPose) : move.still
      const to = this.shotPose(move.to, time, this.toPose)
      this.pose.position.lerpVectors(from.position, to.position, eased)
      this.pose.position.y += move.lift * Math.sin(Math.PI * eased)
      // Keep the heading turn continuous with last frame's, whichever way is now shorter.
      const fromView = viewDirection(from)
      const toView = viewDirection(to)
      let amount = headingTurn(fromView, toView)
      while (amount - move.turn > Math.PI) amount -= Math.PI * 2
      while (amount - move.turn < -Math.PI) amount += Math.PI * 2
      move.turn = amount
      turn(fromView, toView, amount, eased, this.look)
      this.pose.target.copy(this.pose.position).addScaledVector(this.look, 500)
      this.pose.fov = THREE.MathUtils.lerp(from.fov, to.fov, eased)
      if (progress >= 1) {
        this.current = move.to
        this.transition = undefined
      }
    } else {
      this.shotPose(this.current, time, this.pose)
      this.holdTime += dt
      if (this.holdTime >= HOLD_SECONDS) this.cut()
    }
    this.camera.position.copy(this.pose.position)
    this.camera.lookAt(this.pose.target)
    if (Math.abs(this.camera.fov - this.pose.fov) > 0.001) {
      this.camera.fov = this.pose.fov
      this.camera.updateProjectionMatrix()
    }
    return frame
  }
}
