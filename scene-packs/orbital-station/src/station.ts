import * as THREE from 'three'

function canvasTexture(width: number, height: number, draw: (context: CanvasRenderingContext2D) => void, color = true): THREE.CanvasTexture {
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  draw(canvas.getContext('2d')!)
  const texture = new THREE.CanvasTexture(canvas)
  texture.wrapS = texture.wrapT = THREE.RepeatWrapping
  texture.anisotropy = 8
  if (color) texture.colorSpace = THREE.SRGBColorSpace
  return texture
}

function hullTexture(random: () => number): THREE.CanvasTexture {
  return canvasTexture(512, 512, (context) => {
    context.fillStyle = '#c9ced6'
    context.fillRect(0, 0, 512, 512)
    for (let index = 0; index < 90; index++) {
      const shade = 190 + Math.floor(random() * 40)
      context.fillStyle = `rgb(${shade},${shade + 3},${shade + 8})`
      context.fillRect(Math.floor(random() * 16) * 32, Math.floor(random() * 16) * 32, 32 * (1 + Math.floor(random() * 3)), 32)
    }
    context.strokeStyle = 'rgba(70,76,88,0.55)'
    context.lineWidth = 2
    for (let line = 0; line <= 512; line += 64) {
      context.beginPath(); context.moveTo(line, 0); context.lineTo(line, 512); context.stroke()
      context.beginPath(); context.moveTo(0, line); context.lineTo(512, line); context.stroke()
    }
  })
}

function windowTexture(random: () => number): THREE.CanvasTexture {
  return canvasTexture(1024, 64, (context) => {
    context.fillStyle = '#000'
    context.fillRect(0, 0, 1024, 64)
    // Two window rows on the outer face of the tube (v near 0 and 1 on a torus).
    for (const row of [3, 54]) {
      for (let x = 8; x < 1024; x += 24) {
        if (random() < 0.25) continue
        context.fillStyle = random() < 0.8 ? '#ffd9a0' : '#a8d8ff'
        context.fillRect(x, row, 9, 7)
      }
    }
  })
}

function solarTexture(): THREE.CanvasTexture {
  return canvasTexture(256, 256, (context) => {
    context.fillStyle = '#0c1a3d'
    context.fillRect(0, 0, 256, 256)
    for (let x = 0; x < 256; x += 32) {
      for (let y = 0; y < 256; y += 32) {
        const gradient = context.createLinearGradient(x, y, x + 32, y + 32)
        gradient.addColorStop(0, '#1d3a7a')
        gradient.addColorStop(1, '#102556')
        context.fillStyle = gradient
        context.fillRect(x + 2, y + 2, 28, 28)
      }
    }
  })
}

export interface Station {
  group: THREE.Group
  /** 0 = dark, 1 = every window and beacon powered. */
  setPower(value: number): void
  update(time: number, dt: number): void
}

export function createStation(random: () => number, sunDirection: THREE.Vector3): Station {
  const group = new THREE.Group()
  const hull = new THREE.MeshStandardMaterial({ color: 0xaab1bc, map: hullTexture(random), metalness: 0.45, roughness: 0.5 })
  const dark = new THREE.MeshStandardMaterial({ color: 0x3a4049, metalness: 0.7, roughness: 0.5 })
  const gold = new THREE.MeshStandardMaterial({ color: 0xc9a24a, metalness: 0.9, roughness: 0.3 })
  const windows = windowTexture(random)
  windows.repeat.set(8, 1)
  const ringWindows = new THREE.MeshStandardMaterial({
    color: 0xb4bbc6,
    map: hull.map,
    metalness: 0.5,
    roughness: 0.45,
    emissive: new THREE.Color(0xffe1b0),
    emissiveMap: windows,
    emissiveIntensity: 0,
  })
  const solar = new THREE.MeshStandardMaterial({ color: 0xffffff, map: solarTexture(), metalness: 0.85, roughness: 0.22, emissive: new THREE.Color(0x0a1a44), emissiveIntensity: 0.4 })

  // Spine and modules.
  const spine = new THREE.Mesh(new THREE.CylinderGeometry(2.2, 2.2, 78, 32), hull)
  group.add(spine)
  const hub = new THREE.Mesh(new THREE.CylinderGeometry(7, 7, 14, 48), ringWindows)
  group.add(hub)
  for (const y of [-19, 19]) {
    const module = new THREE.Mesh(new THREE.BoxGeometry(9, 8, 9), hull)
    module.position.y = y
    group.add(module)
    const band = new THREE.Mesh(new THREE.CylinderGeometry(5.4, 5.4, 3, 32), ringWindows)
    band.position.y = y + (y > 0 ? 6 : -6)
    group.add(band)
  }
  const nose = new THREE.Mesh(new THREE.SphereGeometry(4, 32, 16, 0, Math.PI * 2, 0, Math.PI / 2), hull)
  nose.position.y = 39
  group.add(nose)

  // The rotating habitat ring with spokes.
  const habitat = new THREE.Group()
  const ring = new THREE.Mesh(new THREE.TorusGeometry(30, 2.8, 32, 220), ringWindows)
  ring.rotation.x = Math.PI / 2
  habitat.add(ring)
  for (let spoke = 0; spoke < 6; spoke++) {
    const angle = (spoke / 6) * Math.PI * 2
    const arm = new THREE.Mesh(new THREE.CylinderGeometry(0.75, 0.75, 23, 12), dark)
    arm.rotation.z = Math.PI / 2
    arm.position.set(Math.cos(angle) * 18.5, 0, Math.sin(angle) * 18.5)
    arm.rotation.y = -angle
    habitat.add(arm)
  }
  group.add(habitat)

  // Docking ring near the nose.
  const dock = new THREE.Mesh(new THREE.TorusGeometry(11, 1.1, 16, 120), hull)
  dock.rotation.x = Math.PI / 2
  dock.position.y = 28
  group.add(dock)

  // Solar arrays on a truss below the hub; the wings turn to track the sun.
  const arrays = new THREE.Group()
  const truss = new THREE.Mesh(new THREE.BoxGeometry(104, 1.2, 1.2), dark)
  arrays.add(truss)
  const wings = new THREE.Group()
  for (const side of [-1, 1]) {
    for (const offset of [0, 1]) {
      const panel = new THREE.Mesh(new THREE.BoxGeometry(22, 0.3, 12), solar)
      panel.position.set(side * (17 + offset * 25), 0, 0)
      wings.add(panel)
    }
  }
  arrays.add(wings)
  arrays.position.y = -31
  group.add(arrays)

  // Radiators and a high-gain dish.
  for (const side of [-1, 1]) {
    const radiator = new THREE.Mesh(new THREE.BoxGeometry(0.3, 12, 7), new THREE.MeshStandardMaterial({ color: 0x8d949e, map: hull.map, metalness: 0.2, roughness: 0.65 }))
    radiator.position.set(side * 8, 10, 0)
    group.add(radiator)
  }
  const dish = new THREE.Mesh(new THREE.SphereGeometry(5.5, 32, 16, 0, Math.PI * 2, 0, Math.PI / 3), gold)
  dish.material.side = THREE.DoubleSide
  dish.position.set(0, 12, 8)
  dish.rotation.x = Math.PI / 2
  group.add(dish)

  // Navigation beacons: red and green on the ring, white strobes on the array tips.
  const beacons: { mesh: THREE.Mesh, material: THREE.MeshBasicMaterial, phase: number, rate: number, color: THREE.Color }[] = []
  const addBeacon = (parent: THREE.Object3D, position: THREE.Vector3, hex: number, rate: number, phase: number) => {
    const color = new THREE.Color(hex).multiplyScalar(6)
    const material = new THREE.MeshBasicMaterial({ color, toneMapped: false })
    const mesh = new THREE.Mesh(new THREE.SphereGeometry(0.55, 12, 8), material)
    mesh.position.copy(position)
    parent.add(mesh)
    beacons.push({ mesh, material, phase, rate, color })
  }
  for (let index = 0; index < 8; index++) {
    const angle = (index / 8) * Math.PI * 2
    addBeacon(habitat, new THREE.Vector3(Math.cos(angle) * 30, 3.2, Math.sin(angle) * 30), index % 2 ? 0xff2a2a : 0x33ff77, 1.1, index * 0.4)
  }
  addBeacon(arrays, new THREE.Vector3(-52, 0.8, 0), 0xffffff, 0.8, 0)
  addBeacon(arrays, new THREE.Vector3(52, 0.8, 0), 0xffffff, 0.8, 0.5)
  addBeacon(group, new THREE.Vector3(0, 43.5, 0), 0xff3030, 0.6, 0.2)

  let power = 0
  const localSun = new THREE.Vector3()
  const inverse = new THREE.Quaternion()

  return {
    group,
    setPower(value) {
      power = value
      ringWindows.emissiveIntensity = value * 1.5
      solar.emissiveIntensity = 0.1 + value * 0.3
    },
    update(time, dt) {
      habitat.rotation.y += dt * 0.07
      dock.rotation.z += dt * 0.12
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
