import * as THREE from 'three'
import { PLANET_RADIUS } from './planet'

export interface Shot {
  id: string
  position: THREE.Vector3
  target: THREE.Vector3
  fov: number
  /** Idle orbit around the target, in radians either side. */
  orbit: number
  /** Idle vertical float, in world units. */
  float: number
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

export const ENTRANCE_SECONDS = 6
/** The picture comes up at once and settles, rather than lingering near black for its first second. */
const FADE_SECONDS = 2.2
const fadeIn = (t: number) => 1 - (1 - t) ** 2
const TRANSITION_SECONDS = 5
const HOLD_SECONDS = 26

const easeInOut = (t: number) => (t < 0.5 ? 4 * t * t * t : 1 - (-2 * t + 2) ** 3 / 2)
const easeOut = (t: number) => 1 - (1 - t) ** 3
const clamp01 = (t: number) => Math.min(1, Math.max(0, t))

function copyPose(pose: Pose): Pose {
  return { position: pose.position.clone(), target: pose.target.clone(), fov: pose.fov }
}

/**
 * Runs the entrance, the idle camera and the transitions between shots. The
 * camera arcs away from the planet between shots so it never cuts through it.
 */
export class Director {
  readonly shots: readonly Shot[]
  private readonly camera: THREE.PerspectiveCamera
  private current = 0
  private entranceTime = 0
  private entranceStart: Pose
  private transition?: { from: Pose, to: number, time: number }
  private holdTime = 0
  private readonly pose: Pose = { position: new THREE.Vector3(), target: new THREE.Vector3(), fov: 40 }
  onShotChange?: (index: number) => void

  constructor(camera: THREE.PerspectiveCamera, shots: readonly Shot[], options: { entrance: boolean, shot?: number }) {
    this.camera = camera
    this.shots = shots
    this.current = options.shot ?? 0
    // The entrance glides in from a little further back and higher up, with a slightly wider lens.
    const first = this.idlePose(this.current, 0)
    const back = first.position.clone().sub(first.target)
    this.entranceStart = {
      position: first.position.clone().addScaledVector(back, 0.32).add(new THREE.Vector3(0, back.length() * 0.08, 0)),
      target: first.target.clone(),
      fov: first.fov + 6,
    }
    this.entranceTime = options.entrance ? 0 : ENTRANCE_SECONDS
  }

  get shotIndex(): number {
    return this.transition?.to ?? this.current
  }

  /** Moves to the next shot (or a given one) with an arcing camera move. */
  cut(index = (this.shotIndex + 1) % this.shots.length): void {
    if (this.entranceTime < ENTRANCE_SECONDS || index === this.shotIndex) return
    this.transition = { from: copyPose(this.pose), to: index, time: 0 }
    this.holdTime = 0
    this.onShotChange?.(index)
  }

  private idlePose(index: number, time: number): Pose {
    const shot = this.shots[index]!
    const offset = shot.position.clone().sub(shot.target)
    offset.applyAxisAngle(new THREE.Vector3(0, 1, 0), Math.sin(time * 0.045) * shot.orbit)
    offset.multiplyScalar(1 + Math.sin(time * 0.083) * 0.035)
    const position = shot.target.clone().add(offset)
    position.y += Math.sin(time * 0.13) * shot.float
    return { position, target: shot.target.clone(), fov: shot.fov }
  }

  update(dt: number, time: number): DirectorFrame {
    const frame: DirectorFrame = { fade: 1 }
    if (this.entranceTime < ENTRANCE_SECONDS) {
      this.entranceTime = Math.min(ENTRANCE_SECONDS, this.entranceTime + dt)
      this.entrancePose(time, frame)
    } else if (this.transition) {
      this.transition.time += dt
      const progress = Math.min(1, this.transition.time / TRANSITION_SECONDS)
      const to = this.idlePose(this.transition.to, time)
      const eased = easeInOut(progress)
      // Lift the midpoint away from the planet so the path arcs around it.
      const middle = this.transition.from.position.clone().lerp(to.position, 0.5)
      const lifted = middle.clone().setLength(Math.max(middle.length(), PLANET_RADIUS * 2.4)).add(new THREE.Vector3(0, 40, 0))
      const a = this.transition.from.position.clone().lerp(lifted, eased)
      const b = lifted.clone().lerp(to.position, eased)
      this.pose.position.copy(a.lerp(b, eased))
      this.pose.target.copy(this.transition.from.target).lerp(to.target, eased)
      this.pose.fov = THREE.MathUtils.lerp(this.transition.from.fov, to.fov, eased)
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

  /** One continuous move: fade up from black while the camera eases into the first shot. */
  private entrancePose(time: number, frame: DirectorFrame): void {
    const t = this.entranceTime
    const final = this.idlePose(this.current, time)
    const settle = easeOut(clamp01(t / ENTRANCE_SECONDS))
    this.pose.position.copy(this.entranceStart.position).lerp(final.position, settle)
    this.pose.target.copy(final.target)
    this.pose.fov = THREE.MathUtils.lerp(this.entranceStart.fov, final.fov, settle)
    frame.fade = fadeIn(clamp01(t / FADE_SECONDS))
  }
}
