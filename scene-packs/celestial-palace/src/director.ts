import * as T from 'three'
import { smooth } from './math'

export const cameraIds = ['celestial-gate', 'lantern-bridge', 'silver-falls'] as const
const shots = [
  { position: new T.Vector3(52, 23, 86), target: new T.Vector3(0, 11, -5), fov: 42 },
  { position: new T.Vector3(14, 10, 52), target: new T.Vector3(0, 12, -4), fov: 48 },
  { position: new T.Vector3(-52, 19, 47), target: new T.Vector3(1, 8, -8), fov: 47 },
]

export class Director {
  index = 0
  private lastSwitch = 0
  private switchTime = -100
  private fromPosition = new T.Vector3()
  private fromTarget = new T.Vector3()
  private target = new T.Vector3()
  private fromFov = 42
  constructor(private camera: T.PerspectiveCamera, private entrance: boolean, initial: number, private announce: (index: number) => void) {
    this.index = Number.isInteger(initial) && initial >= 0 && initial < shots.length ? initial : 0
    const shot = shots[this.index]!
    camera.position.copy(shot.position); this.target.copy(shot.target); camera.fov = shot.fov
  }
  switch(t: number, index?: number): void {
    const next = index === undefined ? (this.index + 1) % shots.length : index
    if (!Number.isInteger(next) || next < 0 || next >= shots.length || next === this.index) return
    this.fromPosition.copy(this.camera.position); this.fromTarget.copy(this.target); this.fromFov = this.camera.fov
    this.index = next; this.switchTime = t; this.lastSwitch = t; this.announce(next)
  }
  update(t: number): void {
    if (t - this.lastSwitch > 29) this.switch(t)
    const shot = shots[this.index]!, blend = smooth((t - this.switchTime) / 5.5)
    const position = shot.position.clone(); const target = shot.target.clone()
    position.x += Math.sin(t * .065) * 1.2; position.y += Math.sin(t * .11) * .45
    position.z += Math.cos(t * .07) * .8
    if (this.entrance && t < 5.8 && this.switchTime < 0) {
      const e = 1 - smooth(t / 5.8)
      position.add(new T.Vector3(7 * e, 3 * e, 18 * e)); target.y -= 2 * e
    }
    this.camera.position.copy(this.fromPosition).lerp(position, blend)
    this.target.copy(this.fromTarget).lerp(target, blend)
    this.camera.fov = T.MathUtils.lerp(this.fromFov, shot.fov, blend)
    this.camera.lookAt(this.target); this.camera.updateProjectionMatrix()
  }
}
