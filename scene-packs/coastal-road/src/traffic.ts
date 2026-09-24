import * as THREE from 'three'
import { mergeGeometries } from 'three/examples/jsm/utils/BufferGeometryUtils.js'
import { glowPoints, ROAD_HALF_WIDTH } from './road'
import type { LightingUniforms } from './shading'
import { colored, paintedMaterial } from './terrain'

/**
 * Life on the road: a car now and then in either direction (lights on after
 * dusk) and cyclists pedalling along the shoulder. Each one runs the length of
 * the road and then waits a while off-screen, so the road is never busy.
 */
interface Traveller {
  mesh: THREE.Object3D
  speed: number
  /** +1 heading south (towards +z), -1 heading north. */
  direction: number
  lane: number
  phase: number
  /** Seconds spent off the road between runs. */
  rest: number
  legs?: THREE.Object3D[]
  lights: number
}

function carGeometry(paint: THREE.Color): THREE.BufferGeometry {
  const glass = new THREE.Color().setRGB(0.05, 0.07, 0.09)
  const tyre = new THREE.Color().setRGB(0.03, 0.03, 0.03)
  const body = new THREE.BoxGeometry(4.3, 0.75, 1.82)
  body.translate(0, 0.62, 0)
  const cabin = new THREE.BoxGeometry(2.3, 0.62, 1.62)
  cabin.translate(-0.25, 1.3, 0)
  const parts = [colored(body, paint), colored(cabin, glass)]
  for (const x of [-1.35, 1.35]) {
    for (const z of [-0.85, 0.85]) {
      const wheel = new THREE.CylinderGeometry(0.34, 0.34, 0.24, 10)
      wheel.rotateX(Math.PI / 2)
      wheel.translate(x, 0.34, z)
      parts.push(colored(wheel, tyre))
    }
  }
  return mergeGeometries(parts)!
}

function bikeAndRider(jersey: THREE.Color, rider: THREE.Group): THREE.Object3D[] {
  const frame = new THREE.Color().setRGB(0.12, 0.12, 0.13)
  const skin = new THREE.Color().setRGB(0.62, 0.45, 0.34)
  const shorts = new THREE.Color().setRGB(0.05, 0.05, 0.06)
  const parts: THREE.BufferGeometry[] = []
  for (const x of [-0.52, 0.52]) {
    const wheel = new THREE.TorusGeometry(0.34, 0.035, 6, 20)
    wheel.translate(x, 0.34, 0)
    parts.push(colored(wheel, frame))
  }
  const tube = new THREE.BoxGeometry(1.0, 0.05, 0.05)
  tube.rotateZ(0.12)
  tube.translate(0, 0.62, 0)
  parts.push(colored(tube, frame))
  const seatPost = new THREE.BoxGeometry(0.05, 0.5, 0.05)
  seatPost.translate(-0.28, 0.66, 0)
  parts.push(colored(seatPost, frame))
  const fork = new THREE.BoxGeometry(0.05, 0.62, 0.05)
  fork.rotateZ(-0.3)
  fork.translate(0.43, 0.62, 0)
  parts.push(colored(fork, frame))
  // Rider leaning over the bars.
  const torso = new THREE.BoxGeometry(0.62, 0.22, 0.34)
  torso.rotateZ(0.55)
  torso.translate(-0.05, 1.18, 0)
  parts.push(colored(torso, jersey))
  const head = new THREE.SphereGeometry(0.12, 10, 8)
  head.translate(0.28, 1.42, 0)
  parts.push(colored(head, jersey))
  for (const z of [-0.16, 0.16]) {
    const arm = new THREE.BoxGeometry(0.46, 0.07, 0.07)
    arm.rotateZ(-0.55)
    arm.translate(0.25, 1.12, z)
    parts.push(colored(arm, skin))
  }
  rider.add(new THREE.Mesh(mergeGeometries(parts)!))
  // Legs: one bar from hip to pedal each, turned every frame with the crank.
  return [-0.14, 0.14].map((z) => {
    const leg = new THREE.Mesh(colored(new THREE.BoxGeometry(0.1, 1, 0.1).translate(0, -0.5, 0), shorts))
    leg.position.set(-0.24, 1.0, z)
    rider.add(leg)
    return leg
  })
}

export interface Traffic {
  group: THREE.Group
  update(time: number, night: number): void
  /**
   * For the road camera: puts a southbound cyclist and a northbound car at these distances along
   * the road right now, so the shot always has someone riding ahead and a car coming the other way.
   */
  stage(time: number, cyclistAt: number, carAt: number): void
}

export function createTraffic(uniforms: LightingUniforms, curve: THREE.CatmullRomCurve3, random: () => number): Traffic {
  const group = new THREE.Group()
  const length = curve.getLength()
  const material = paintedMaterial(uniforms, 0.3)
  const travellers: Traveller[] = []
  const paints = [[0.55, 0.06, 0.05], [0.85, 0.85, 0.82], [0.06, 0.16, 0.38], [0.4, 0.42, 0.44], [0.75, 0.55, 0.12], [0.08, 0.26, 0.18]]
  paints.forEach(([r, g, b], index) => {
    const mesh = new THREE.Mesh(carGeometry(new THREE.Color().setRGB(r!, g!, b!)), material)
    group.add(mesh)
    const direction = index % 2 === 0 ? 1 : -1
    travellers.push({ mesh, speed: 15 + random() * 6, direction, lane: direction * 1.9, phase: random() * 600, rest: 60 + random() * 160, lights: 4 })
  })
  const jerseys = [[0.8, 0.12, 0.1], [0.95, 0.75, 0.1], [0.1, 0.35, 0.8], [0.9, 0.9, 0.9]]
  jerseys.forEach(([r, g, b], index) => {
    const rider = new THREE.Group()
    const legs = bikeAndRider(new THREE.Color().setRGB(r!, g!, b!), rider)
    rider.traverse((object) => { if ((object as THREE.Mesh).isMesh) (object as THREE.Mesh).material = material })
    group.add(rider)
    const direction = index % 2 === 0 ? 1 : -1
    travellers.push({ mesh: rider, speed: 5.2 + random() * 2, direction, lane: direction * (ROAD_HALF_WIDTH - 0.9), phase: random() * 900, rest: 40 + random() * 90, legs, lights: 1 })
  })

  const lightCount = travellers.reduce((sum, traveller) => sum + traveller.lights, 0)
  const glow = glowPoints(uniforms, lightCount)
  group.add(glow.points)
  const up = new THREE.Vector3(0, 1, 0)
  const point = new THREE.Vector3()
  const tangent = new THREE.Vector3()
  const side = new THREE.Vector3()
  const heading = new THREE.Vector3()
  const local = new THREE.Vector3()
  const quaternion = new THREE.Quaternion()
  const x = new THREE.Vector3(1, 0, 0)

  const place = (traveller: Traveller | undefined, time: number, s: number) => {
    if (!traveller) return
    const period = (length + traveller.speed * traveller.rest) / traveller.speed
    const distance = traveller.direction > 0 ? s : length - s
    traveller.phase = (((distance / traveller.speed - time) % period) + period) % period
  }

  return {
    group,
    stage(time, cyclistAt, carAt) {
      place(travellers.find((traveller) => traveller.legs && traveller.direction > 0), time, cyclistAt)
      place(travellers.find((traveller) => !traveller.legs && traveller.direction < 0), time, carAt)
    },
    update(time, night) {
      let light = 0
      for (const traveller of travellers) {
        const cycle = length + traveller.speed * traveller.rest
        const distance = (time * traveller.speed + traveller.phase * traveller.speed) % cycle
        const onRoad = distance < length
        traveller.mesh.visible = onRoad
        const s = traveller.direction > 0 ? distance : length - distance
        const u = THREE.MathUtils.clamp(s / length, 0, 1)
        curve.getPointAt(u, point)
        curve.getTangentAt(u, tangent)
        side.crossVectors(tangent, up).normalize()
        heading.copy(tangent).multiplyScalar(traveller.direction)
        traveller.mesh.position.copy(point).addScaledVector(side, traveller.lane).add(new THREE.Vector3(0, 0.1, 0))
        quaternion.setFromUnitVectors(x, new THREE.Vector3(heading.x, 0, heading.z).normalize())
        traveller.mesh.quaternion.copy(quaternion)
        if (traveller.legs) {
          const crank = time * traveller.speed * 1.4
          traveller.legs.forEach((leg, index) => {
            const angle = crank + index * Math.PI
            // Hip at (-0.24, 1.0), pedal circling the crank at (0, 0.34).
            const pedalX = Math.cos(angle) * 0.17
            const pedalY = 0.34 + Math.sin(angle) * 0.17
            const dx = pedalX + 0.24
            const dy = pedalY - 1.0
            leg.rotation.z = Math.atan2(dx, -dy)
            leg.scale.y = Math.hypot(dx, dy)
          })
        }
        // Lights: headlights on after dusk, tail lights always faintly on.
        const lit = onRoad ? 1 : 0
        const lamps = traveller.lights === 4
          ? [[2.2, 0.62, 0.62, 1, 0.92, 0.75, night * 3], [2.2, 0.62, -0.62, 1, 0.92, 0.75, night * 3], [-2.18, 0.7, 0.66, 1, 0.08, 0.04, 0.4 + night * 1.6], [-2.18, 0.7, -0.66, 1, 0.08, 0.04, 0.4 + night * 1.6]]
          : [[-0.6, 0.62, 0, 1, 0.08, 0.04, 0.2 + night * 1.4]]
        for (const [lx, ly, lz, r, g, b, strength] of lamps) {
          local.set(lx!, ly!, lz!).applyQuaternion(quaternion).add(traveller.mesh.position)
          glow.positions.set([local.x, local.y, local.z], light * 3)
          glow.colors.set([r! * strength! * lit, g! * strength! * lit, b! * strength! * lit], light * 3)
          glow.sizes[light] = traveller.lights === 4 ? 0.9 : 0.35
          light++
        }
      }
      glow.update()
    },
  }
}
