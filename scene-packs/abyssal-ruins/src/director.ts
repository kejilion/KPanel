import * as T from 'three'

interface CameraShot { position: [number, number, number]; target: [number, number, number]; fov: number }
declare const __ABYSS_CAMERAS__: CameraShot[]

export const cameraIds = ['sunken-threshold', 'silent-sanctuary', 'levitating-relic'] as const
export const cameraShots: readonly CameraShot[] = __ABYSS_CAMERAS__
const HOLD_SECONDS = 27

/** A quintic track keeps camera velocity continuous when a shot is interrupted. */
class Track {
  private coefficients = Array.from({ length: 7 }, () => new T.Vector3())
  private delta = new T.Vector3()
  private duration = 1

  set(from: T.Vector3, velocity: T.Vector3, to: T.Vector3, endVelocity: T.Vector3, duration: number, lift = 0): void {
    const c = this.coefficients
    this.duration = duration
    this.delta.copy(to).sub(from)
    c[0]!.copy(from)
    c[1]!.copy(velocity).multiplyScalar(duration)
    c[2]!.set(0, 0, 0)
    c[3]!.copy(this.delta).multiplyScalar(10).addScaledVector(velocity, -6 * duration).addScaledVector(endVelocity, -4 * duration)
    c[4]!.copy(this.delta).multiplyScalar(-15).addScaledVector(velocity, 8 * duration).addScaledVector(endVelocity, 7 * duration)
    c[5]!.copy(this.delta).multiplyScalar(6).addScaledVector(velocity, -3 * duration).addScaledVector(endVelocity, -3 * duration)
    c[6]!.set(0, 0, 0)
    // 64 u³(1-u)³ rises over the broken columns without an endpoint jump.
    c[3]!.y += 64 * lift
    c[4]!.y -= 192 * lift
    c[5]!.y += 192 * lift
    c[6]!.y -= 64 * lift
  }

  sample(u: number, position: T.Vector3, velocity: T.Vector3): void {
    position.copy(this.coefficients[6]!)
    velocity.copy(this.coefficients[6]!).multiplyScalar(6)
    for (let n = 5; n >= 0; n--) position.multiplyScalar(u).add(this.coefficients[n]!)
    for (let n = 5; n >= 1; n--) velocity.multiplyScalar(u).addScaledVector(this.coefficients[n]!, n)
    velocity.divideScalar(this.duration)
  }
}

export class Director {
  index = 0
  private target = new T.Vector3()
  private velocity = new T.Vector3()
  private targetVelocity = new T.Vector3()
  private fov = new T.Vector3()
  private fovVelocity = new T.Vector3()
  private positionTrack = new Track()
  private targetTrack = new Track()
  private fovTrack = new Track()
  private destination = new T.Vector3()
  private destinationVelocity = new T.Vector3()
  private destinationTarget = new T.Vector3()
  private destinationTargetVelocity = new T.Vector3()
  private destinationFov = new T.Vector3()
  private probe = new T.Vector3()
  private probeVelocity = new T.Vector3()
  private zero = new T.Vector3()
  private start = 0
  private duration = 0
  private settledAt = 0
  private moving = false

  constructor(private camera: T.PerspectiveCamera, entrance: boolean, initial: number, private announce: (index: number) => void) {
    this.index = Number.isInteger(initial) && initial >= 0 && initial < cameraIds.length ? initial : 0
    this.pose(0)
    camera.position.copy(this.destination)
    this.target.copy(this.destinationTarget)
    this.fov.copy(this.destinationFov)
    if (entrance) {
      // The approach remains on the visible side of the entrance arch.
      camera.position.add(this.probe.set(3.5, 1.8, 8))
      this.begin(0, 6.5, false)
    }
    this.sample(0)
  }

  switch(t: number, index?: number): void {
    const next = index === undefined ? (this.index + 1) % cameraIds.length : index
    if (!Number.isInteger(next) || next < 0 || next >= cameraIds.length || next === this.index) return
    this.sample(t)
    this.index = next
    this.pose(t)
    const distance = this.camera.position.distanceTo(this.destination)
    this.begin(t, T.MathUtils.clamp(8 + distance * .075, 8, 13), true)
    this.announce(next)
  }

  update(t: number): void {
    this.sample(t)
    if (!this.moving && t - this.settledAt >= HOLD_SECONDS) this.switch(t)
  }

  private pose(t: number): void {
    const shot = cameraShots[this.index]!
    this.destination.set(...shot.position)
    this.destination.x += Math.sin(t * .065) * .28
    this.destination.y += Math.sin(t * .09) * .14
    this.destination.z += Math.cos(t * .055) * .2
    this.destinationVelocity.set(Math.cos(t * .065) * .0182, Math.cos(t * .09) * .0126, -Math.sin(t * .055) * .011)
    this.destinationTarget.set(...shot.target)
    this.destinationTarget.y += Math.sin(t * .04) * .06
    this.destinationTargetVelocity.set(0, Math.cos(t * .04) * .0024, 0)
    this.destinationFov.set(shot.fov, 0, 0)
  }

  private begin(t: number, duration: number, detour: boolean): void {
    this.start = t
    this.duration = duration
    this.settledAt = t + duration
    this.moving = true
    this.pose(this.settledAt)
    this.positionTrack.set(this.camera.position, this.velocity, this.destination, this.destinationVelocity, duration)
    if (detour) {
      this.positionTrack.sample(.5, this.probe, this.probeVelocity)
      // A fixed world height avoids accumulating elevation after repeated clicks.
      const lift = Math.max(0, 21 - this.probe.y)
      this.positionTrack.set(this.camera.position, this.velocity, this.destination, this.destinationVelocity, duration, lift)
    }
    this.targetTrack.set(this.target, this.targetVelocity, this.destinationTarget, this.destinationTargetVelocity, duration)
    this.fovTrack.set(this.fov, this.fovVelocity, this.destinationFov, this.zero, duration)
  }

  private sample(t: number): void {
    if (this.moving && t < this.settledAt) {
      const u = T.MathUtils.clamp((t - this.start) / this.duration, 0, 1)
      this.positionTrack.sample(u, this.camera.position, this.velocity)
      this.targetTrack.sample(u, this.target, this.targetVelocity)
      this.fovTrack.sample(u, this.fov, this.fovVelocity)
    } else {
      this.moving = false
      this.pose(t)
      this.camera.position.copy(this.destination)
      this.velocity.copy(this.destinationVelocity)
      this.target.copy(this.destinationTarget)
      this.targetVelocity.copy(this.destinationTargetVelocity)
      this.fov.copy(this.destinationFov)
      this.fovVelocity.set(0, 0, 0)
    }
    this.camera.lookAt(this.target)
    if (this.camera.fov !== this.fov.x) {
      this.camera.fov = this.fov.x
      this.camera.updateProjectionMatrix()
    }
  }
}
