import * as T from 'three'

export const cameraIds = ['celestial-gate', 'lantern-bridge', 'silver-falls'] as const
export const cameraShots = [
  { position: [60, 35, 145], target: [0, 27, 0], fov: 43 },
  { position: [-8, 15, 48], target: [0, 23, 0], fov: 52 },
  { position: [-40, -16, 72], target: [0, 6, 0], fov: 59 },
] as const

const ENTRANCE_SECONDS = 5.8
const HOLD_SECONDS = 28

/** Quintic interpolation preserves velocity when a running shot is interrupted. */
class Track {
  private coefficients = Array.from({ length: 7 }, () => new T.Vector3())
  private delta = new T.Vector3()
  private duration = 1

  set(from: T.Vector3, velocity: T.Vector3, to: T.Vector3, endVelocity: T.Vector3, duration: number, frontPush = 0): void {
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
    // 64 u³ (1-u)³ moves the middle of the route forward without changing
    // either endpoint's position, velocity or acceleration.
    c[3]!.z += 64 * frontPush
    c[4]!.z -= 192 * frontPush
    c[5]!.z += 192 * frontPush
    c[6]!.z -= 64 * frontPush
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
  private zero = new T.Vector3()
  private probe = new T.Vector3()
  private probeVelocity = new T.Vector3()
  private start = 0
  private duration = 0
  private settledAt = 0
  private moving = false

  constructor(private camera: T.PerspectiveCamera, entrance: boolean, initial: number, private announce: (index: number) => void) {
    this.index = Number.isInteger(initial) && initial >= 0 && initial < cameraShots.length ? initial : 0
    this.pose(0)
    camera.position.copy(this.destination)
    this.target.copy(this.destinationTarget)
    this.fov.copy(this.destinationFov)
    if (entrance) {
      camera.position.add(this.probe.set(16, 7, 35))
      this.target.y -= 2
      this.begin(0, ENTRANCE_SECONDS, false)
    }
    this.sample(0)
  }

  switch(t: number, index?: number): void {
    const next = index === undefined ? (this.index + 1) % cameraShots.length : index
    if (!Number.isInteger(next) || next < 0 || next >= cameraShots.length || next === this.index) return
    this.sample(t)
    this.index = next
    this.pose(t)
    const distance = this.camera.position.distanceTo(this.destination)
    this.begin(t, T.MathUtils.clamp(7 + distance * .035, 7, 13), true)
    this.announce(next)
  }

  update(t: number): void {
    this.sample(t)
    if (!this.moving && t - this.settledAt >= HOLD_SECONDS) this.switch(t)
  }

  private pose(t: number): void {
    const shot = cameraShots[this.index]!
    this.destination.set(shot.position[0], shot.position[1], shot.position[2])
    this.destination.x += Math.sin(t * .065) * 1.05
    this.destination.y += Math.sin(t * .11) * .4
    this.destination.z += Math.cos(t * .07) * .65
    this.destinationVelocity.set(Math.cos(t * .065) * .06825, Math.cos(t * .11) * .044, -Math.sin(t * .07) * .0455)
    this.destinationTarget.set(shot.target[0], shot.target[1], shot.target[2])
    this.destinationTarget.y += Math.sin(t * .045) * .15
    this.destinationTargetVelocity.set(0, Math.cos(t * .045) * .00675, 0)
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
      // Anchor the detour to the destination, not the current detour height:
      // repeated manual switches must not push the route farther out each time.
      const front = Math.max(122, this.destination.z + 12)
      let push = Math.max(0, front - this.probe.z)
      // An interrupted approach retains its incoming velocity. Increase the
      // forward bow if that tangent would otherwise pass behind the safe plane.
      for (let step = 1; step < 64; step++) {
        const u = step / 64
        this.positionTrack.sample(u, this.probe, this.probeVelocity)
        const bump = 64 * u ** 3 * (1 - u) ** 3
        push = Math.max(push, (46 - this.probe.z) / bump)
      }
      this.positionTrack.set(this.camera.position, this.velocity, this.destination, this.destinationVelocity, duration, push)
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
