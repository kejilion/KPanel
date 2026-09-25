import * as THREE from 'three'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'

export interface Station {
  group: THREE.Group
  /** 0 = dark, 1 = every window and beacon powered. */
  setPower(value: number): void
  update(time: number, dt: number): void
}

/**
 * The station, modelled in Blender (see blender/station.py): modules wrapped in
 * gold and silver insulation, a hub of painted panels, a habitat ring of
 * segmented modules with rows of windows, a docking ring, ribbed radiators, a
 * dish, and a lattice truss carrying four solar wings. The habitat and the
 * docking ring turn, the wings track the sun, and the navigation beacons blink.
 */
export async function createStation(sunDirection: THREE.Vector3, renderer: THREE.WebGLRenderer): Promise<Station> {
  const gltf = await new GLTFLoader().loadAsync('assets/station.gltf')
  const group = new THREE.Group()
  const model = gltf.scene
  group.add(model)
  const node = (name: string) => {
    const found = model.getObjectByName(name)
    if (!found) throw new Error(`station.gltf: no ${name}`)
    return found
  }
  const habitat = node('habitat')
  const dock = node('dock')
  const arrays = node('arrays')
  const wings = node('wings')
  let windows: THREE.MeshStandardMaterial | undefined
  model.traverse((object) => {
    const mesh = object as THREE.Mesh
    if (!mesh.isMesh) return
    mesh.castShadow = true
    mesh.receiveShadow = true
    const materials = Array.isArray(mesh.material) ? mesh.material : [mesh.material]
    for (const material of materials as THREE.MeshStandardMaterial[]) {
      material.envMapIntensity = 1
      for (const map of [material.map, material.normalMap, material.roughnessMap, material.metalnessMap, material.emissiveMap]) {
        if (!map) continue
        map.anisotropy = 8
        // On the graphics card now, rather than on the first frame.
        renderer.initTexture(map)
      }
      if (material.name === 'window') windows = material
    }
  })

  // Navigation beacons: red and green on the ring, white strobes on the array tips.
  const beacons: { mesh: THREE.Mesh, material: THREE.MeshBasicMaterial, phase: number, rate: number, color: THREE.Color }[] = []
  const addBeacon = (parent: THREE.Object3D, position: THREE.Vector3, hex: number, rate: number, phase: number) => {
    const color = new THREE.Color(hex).multiplyScalar(6)
    const material = new THREE.MeshBasicMaterial({ color, toneMapped: false })
    const mesh = new THREE.Mesh(new THREE.SphereGeometry(0.4, 12, 8), material)
    mesh.position.copy(position)
    parent.add(mesh)
    beacons.push({ mesh, material, phase, rate, color })
  }
  for (let index = 0; index < 8; index++) {
    const angle = (index / 8) * Math.PI * 2
    addBeacon(habitat, new THREE.Vector3(Math.cos(angle) * 30, 2.9, Math.sin(angle) * 30), index % 2 ? 0xff2a2a : 0x33ff77, 1.1, index * 0.4)
  }
  addBeacon(arrays, new THREE.Vector3(-52.5, 0.8, 0), 0xffffff, 0.8, 0)
  addBeacon(arrays, new THREE.Vector3(52.5, 0.8, 0), 0xffffff, 0.8, 0.5)
  addBeacon(model, new THREE.Vector3(0, 43.6, 0), 0xff3030, 0.6, 0.2)

  let power = 0
  const localSun = new THREE.Vector3()
  const inverse = new THREE.Quaternion()

  return {
    group,
    setPower(value) {
      power = value
      if (windows) windows.emissiveIntensity = value * 1.2
    },
    update(time, dt) {
      habitat.rotation.y += dt * 0.07
      dock.rotation.y += dt * 0.12
      // Rotating about the truss (X) by atan2(z, y) turns the wing normal (+Y) towards the sun.
      arrays.getWorldQuaternion(inverse).invert()
      localSun.copy(sunDirection).applyQuaternion(inverse)
      wings.rotation.x = Math.atan2(localSun.z, localSun.y)
      for (const beacon of beacons) {
        const cycle = (time * beacon.rate + beacon.phase) % 1
        const on = cycle < 0.12 ? 1 : 0.06
        beacon.material.color.copy(beacon.color).multiplyScalar(on * power)
        beacon.mesh.visible = power > 0.02
      }
    },
  }
}

/** Shuttles on looping paths around the station, each with an engine glow and trail. */
export interface Traffic {
  group: THREE.Group
  update(time: number, dt: number): void
}

export function createTraffic(anchor: THREE.Vector3, random: () => number): Traffic {
  const group = new THREE.Group()
  const hull = new THREE.MeshStandardMaterial({ color: 0xdfe3ea, metalness: 0.5, roughness: 0.4 })
  const ships = Array.from({ length: 4 }, (_, index) => {
    const ship = new THREE.Group()
    const body = new THREE.Mesh(new THREE.ConeGeometry(1.1, 5, 12), hull)
    body.rotation.x = Math.PI / 2
    ship.add(body)
    const wings = new THREE.Mesh(new THREE.BoxGeometry(4.2, 0.2, 1.6), hull)
    wings.position.z = -1
    ship.add(wings)
    const engine = new THREE.Mesh(new THREE.SphereGeometry(0.55, 12, 8), new THREE.MeshBasicMaterial({ color: new THREE.Color(0.8, 1.6, 3.5), toneMapped: false }))
    engine.position.z = -2.7
    ship.add(engine)
    group.add(ship)

    const trailLength = 48
    const trailPositions = new Float32Array(trailLength * 3)
    const trailGeometry = new THREE.BufferGeometry()
    trailGeometry.setAttribute('position', new THREE.BufferAttribute(trailPositions, 3))
    const trail = new THREE.Points(trailGeometry, new THREE.PointsMaterial({
      color: new THREE.Color(0.3, 0.7, 1.8),
      size: 1.1,
      sizeAttenuation: true,
      transparent: true,
      opacity: 0.4,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
      toneMapped: false,
    }))
    trail.frustumCulled = false
    group.add(trail)
    return {
      ship,
      trail: trailPositions,
      trailGeometry,
      trailLength,
      radius: 55 + index * 22 + random() * 10,
      tilt: (random() - 0.5) * 0.9,
      speed: (0.08 + random() * 0.05) * (index % 2 ? -1 : 1),
      phase: random() * Math.PI * 2,
      primed: false,
    }
  })
  const position = new THREE.Vector3()
  const ahead = new THREE.Vector3()
  const tilt = new THREE.Euler()

  const place = (target: THREE.Vector3, radius: number, angle: number, tiltAngle: number) => {
    target.set(Math.cos(angle) * radius, Math.sin(angle * 2) * 6, Math.sin(angle) * radius)
    tilt.set(tiltAngle, 0, tiltAngle * 0.6)
    target.applyEuler(tilt).add(anchor)
  }

  return {
    group,
    update(time) {
      for (const ship of ships) {
        const angle = ship.phase + time * ship.speed
        place(position, ship.radius, angle, ship.tilt)
        place(ahead, ship.radius, angle + Math.sign(ship.speed) * 0.02, ship.tilt)
        ship.ship.position.copy(position)
        ship.ship.lookAt(ahead)
        if (!ship.primed) {
          for (let index = 0; index < ship.trailLength; index++) ship.trail.set(position.toArray(), index * 3)
          ship.primed = true
        }
        ship.trail.copyWithin(3, 0, (ship.trailLength - 1) * 3)
        const tail = position.clone().addScaledVector(ahead.clone().sub(position).normalize(), -2.8)
        ship.trail.set(tail.toArray(), 0)
        ship.trailGeometry.attributes.position!.needsUpdate = true
      }
    },
  }
}
