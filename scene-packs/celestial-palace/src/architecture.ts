import * as T from 'three'
import { mergeGeometries } from 'three/examples/jsm/utils/BufferGeometryUtils.js'
import { box, line, random } from './math'

const marble = new T.MeshStandardMaterial({ color: '#a6acc0', roughness: 0.72, metalness: 0.16 })
const edge = new T.MeshStandardMaterial({ color: '#d5c9cb', roughness: 0.44, metalness: 0.32 })
const timber = new T.MeshStandardMaterial({ color: '#362a47', roughness: 0.55 })
const roofMat = new T.MeshStandardMaterial({ color: '#24414e', roughness: 0.35, metalness: 0.55, side: T.DoubleSide })
const gold = new T.MeshStandardMaterial({ color: '#c7a468', roughness: 0.32, metalness: 0.68, emissive: '#cc8536', emissiveIntensity: 0.15 })
const light = new T.MeshBasicMaterial({ color: new T.Color('#ffad55').multiplyScalar(1.35) })
const rootMat = new T.MeshStandardMaterial({ color: '#262338', roughness: 1, flatShading: true })
const foliage = [ '#aa6b9c', '#d491ae', '#cc87b4', '#e4acbf' ].map(color => new T.MeshStandardMaterial({ color, roughness: 0.85, flatShading: true }))
const rand = random(2783)

function pillar(parent: T.Object3D, x: number, y: number, z: number, h: number, r = 0.22): void {
  const column = new T.Mesh(new T.CylinderGeometry(r, r * 1.2, h, 8), timber)
  column.position.set(x, y + h / 2, z)
  parent.add(column)
  for (const dy of [0.12, h - 0.16]) {
    const cap = new T.Mesh(new T.CylinderGeometry(r * 1.45, r * 1.45, 0.25, 8), gold)
    cap.position.set(x, y + dy, z)
    parent.add(cap)
  }
}

// Four swept roof faces: the outer corners rise into the characteristic eaves.
function roof(parent: T.Object3D, y: number, half: number, height: number): void {
  const n = 20
  const vertices: number[] = [], indices: number[] = [], tiles: T.Vector3[] = []
  function point(u: number, t: number, side: number): T.Vector3 {
    const width = half * (0.13 + 0.87 * t)
    const yy = y + height * Math.pow(1 - t, 1.65) + 0.62 * Math.pow(Math.abs(u), 5) * Math.pow(t, 3)
    return new T.Vector3(u * width, yy, width).applyAxisAngle(new T.Vector3(0, 1, 0), side * Math.PI / 2)
  }
  for (let side = 0; side < 4; side++) {
    const offset = vertices.length / 3
    for (let j = 0; j <= n; j++) for (let i = 0; i <= n; i++) {
      const p = point(i / n * 2 - 1, j / n, side)
      vertices.push(p.x, p.y, p.z)
    }
    for (let j = 0; j < n; j++) for (let i = 0; i < n; i++) {
      const a = offset + j * (n + 1) + i, b = a + n + 1
      indices.push(a, b, a + 1, b, b + 1, a + 1)
    }
    for (let i = 0; i <= 24; i++) {
      for (let j = 0; j < 10; j++) {
        tiles.push(point(i / 12 - 1, j / 10, side).add(new T.Vector3(0, 0.024, 0)), point(i / 12 - 1, (j + 1) / 10, side).add(new T.Vector3(0, 0.024, 0)))
      }
    }
    line(Array.from({ length: 21 }, (_, i) => point(i / 10 - 1, 1, side)), 0.065, gold, parent)
    line(Array.from({ length: 12 }, (_, i) => point(1, i / 11, side)), 0.075, gold, parent)
  }
  const geo = new T.BufferGeometry()
  geo.setAttribute('position', new T.Float32BufferAttribute(vertices, 3)); geo.setIndex(indices); geo.computeVertexNormals()
  parent.add(new T.Mesh(geo, roofMat))
  parent.add(new T.LineSegments(new T.BufferGeometry().setFromPoints(tiles), new T.LineBasicMaterial({ color: '#64828c', transparent: true, opacity: 0.3 })))
}

function railing(parent: T.Object3D, a: T.Vector3, b: T.Vector3, count: number): void {
  for (let i = 0; i <= count; i++) {
    const p = a.clone().lerp(b, i / count)
    box(parent, [0.16, 1, 0.16], [p.x, p.y + 0.5, p.z], edge)
    const bead = new T.Mesh(new T.SphereGeometry(0.14, 6, 4), gold)
    bead.position.set(p.x, p.y + 1.04, p.z); parent.add(bead)
  }
  for (const dy of [0.3, 0.8]) line([a.clone().add(new T.Vector3(0, dy, 0)), b.clone().add(new T.Vector3(0, dy, 0))], 0.052, edge, parent)
}

function level(parent: T.Object3D, y: number, half: number, h: number): void {
  box(parent, [half * 2.15, 0.32, half * 2.15], [0, y, 0], edge)
  box(parent, [half * 1.48, h * 0.85, half * 1.48], [0, y + h * 0.43, 0], timber)
  for (let side = 0; side < 4; side++) {
    const wall = new T.Group(); wall.rotation.y = side * Math.PI / 2; parent.add(wall)
    for (let i = -2; i <= 2; i++) {
      const x = i * half * 0.31
      box(wall, [half * 0.235, h * 0.57, 0.06], [x, y + h * 0.48, half * 0.75], light)
      for (const sign of [-1, 1]) box(wall, [0.065, h * 0.6, 0.075], [x + sign * half * 0.075, y + h * 0.48, half * 0.79], timber)
      for (const yy of [0.27, 0.53, 0.71]) box(wall, [half * 0.235, 0.07, 0.075], [x, y + h * yy, half * 0.79], timber)
    }
    for (let i = -2; i <= 2; i++) pillar(wall, i * half * 0.44, y, half * 0.91, h, 0.14 + half * 0.016)
    box(wall, [half * 1.96, 0.38, 0.28], [0, y + h - 0.15, half * 0.91], timber)
    box(wall, [half * 1.96, 0.08, 0.31], [0, y + h - 0.12, half * 0.91], gold)
    railing(wall, new T.Vector3(-half, y + 0.2, half), new T.Vector3(half, y + 0.2, half), 12)
  }
  roof(parent, y + h, half * 1.3, half * 0.47)
}

function palace(parent: T.Object3D, x: number, y: number, z: number, scale: number): void {
  const g = new T.Group(); g.position.set(x, y, z); g.scale.setScalar(scale); parent.add(g)
  for (let i = 0; i < 3; i++) box(g, [17 - i * 1.4, 0.45, 14 - i * 0.8], [0, i * 0.45, 0], marble)
  level(g, 1.5, 6.2, 4.9)
  level(g, 8.4, 4.7, 3.7)
  level(g, 13.8, 3.3, 3.5)
  roof(g, 19.2, 2.4, 2.2)
  const spire = new T.Mesh(new T.ConeGeometry(0.26, 3.6, 8), gold); spire.position.y = 23; g.add(spire)
  for (const y0 of [21, 21.5, 22.0]) {
    const ring = new T.Mesh(new T.TorusGeometry(0.6 - (y0 - 21) * 0.23, 0.07, 6, 24), gold)
    ring.rotation.x = Math.PI / 2; ring.position.y = y0; g.add(ring)
  }
  const pearl = new T.Mesh(new T.SphereGeometry(0.24, 12, 8), light); pearl.position.y = 24.8; g.add(pearl)
  for (let i = 0; i < 8; i++) box(g, [4.6, 0.22, 0.65], [0, 1.5 - i * 0.19, 7 + i * 0.52], edge)
}

function island(parent: T.Object3D, x: number, y: number, z: number, radius: number, depth: number): void {
  const geometry = new T.CylinderGeometry(radius, radius * 0.09, depth, 17, 7)
  const pos = geometry.attributes.position!
  for (let i = 0; i < pos.count; i++) {
    const yy = pos.getY(i), rough = 0.84 + rand() * 0.3
    pos.setXYZ(i, pos.getX(i) * rough, yy + (Math.abs(yy) < depth * 0.49 ? (rand() - 0.5) * 3 : 0), pos.getZ(i) * rough)
  }
  geometry.computeVertexNormals()
  const rock = new T.Mesh(geometry, rootMat); rock.position.set(x, y - depth / 2, z); parent.add(rock)
  const rim = new T.Mesh(new T.CylinderGeometry(radius, radius * 0.95, 0.7, 48), marble); rim.position.set(x, y, z); parent.add(rim)
  const floor = new T.Mesh(new T.CylinderGeometry(radius * 0.95, radius * 0.98, 0.3, 48), new T.MeshStandardMaterial({ color: '#78878e', roughness: 0.88 }))
  floor.position.set(x, y + 0.48, z); parent.add(floor)
  // Stepped shards extend below the platform, breaking up the island silhouette.
  for (let i = 0; i < 9; i++) {
    const shard = new T.Mesh(new T.ConeGeometry(radius * (0.12 + rand() * 0.17), depth * (0.4 + rand() * 0.6), 5), rootMat)
    const a = i / 9 * Math.PI * 2
    shard.rotation.z = Math.PI + (rand() - 0.5) * 0.3
    shard.position.set(x + Math.cos(a) * radius * 0.7, y - depth * 0.5, z + Math.sin(a) * radius * 0.7); parent.add(shard)
  }
}

function cherry(parent: T.Object3D, x: number, y: number, z: number, scale: number): void {
  const g = new T.Group(); g.position.set(x, y, z); g.scale.setScalar(scale); parent.add(g)
  line([new T.Vector3(), new T.Vector3(-0.2, 2, 0), new T.Vector3(0.7, 4.7, 0.2), new T.Vector3(0.4, 6.9, 0)], 0.24, timber, g)
  for (let i = 0; i < 6; i++) {
    const angle = i * 2.4, end = new T.Vector3(Math.cos(angle) * (2.2 + rand()), 4 + rand() * 3.3, Math.sin(angle) * 2.8)
    line([new T.Vector3(0, 2.8, 0), end.clone().multiplyScalar(0.75), end], 0.12, timber, g)
    for (let j = 0; j < 5; j++) {
      const leaves = new T.Mesh(new T.IcosahedronGeometry(1.2 + rand() * 0.6, 1), foliage[(i + j) % foliage.length])
      leaves.position.copy(end).add(new T.Vector3((rand() - 0.5) * 3, (rand() - 0.5) * 1.4, (rand() - 0.5) * 3))
      leaves.scale.set(1, 0.62, 1); g.add(leaves)
    }
  }
}

function bridge(parent: T.Object3D, from: T.Vector3, to: T.Vector3, width: number): void {
  const n = 42, side = new T.Vector3().subVectors(to, from).normalize().cross(new T.Vector3(0, 1, 0))
  const paths: T.Vector3[][] = [[], []]
  for (let i = 0; i <= n; i++) {
    const p = from.clone().lerp(to, i / n); p.y += Math.sin(i / n * Math.PI) * 2.7
    const step = box(parent, [width, 0.28, from.distanceTo(to) / n + 0.03], [p.x, p.y, p.z], marble)
    step.rotation.y = Math.atan2(to.x - from.x, to.z - from.z)
    for (let j = 0; j < 2; j++) {
      const q = p.clone().addScaledVector(side, (j ? 1 : -1) * (width / 2 - 0.1))
      paths[j]!.push(q.clone().add(new T.Vector3(0, 1.3, 0)))
      if (i % 3 === 0) {
        box(parent, [0.18, 1.4, 0.18], [q.x, q.y + 0.6, q.z], edge)
        const jewel = new T.Mesh(new T.SphereGeometry(0.15, 8, 6), light); jewel.position.copy(q).y += 1.42; parent.add(jewel)
      }
    }
  }
  for (const points of paths) line(points, 0.07, gold, parent)
}

export function architecture(scene: T.Scene): void {
  const root = new T.Group(); scene.add(root)
  island(root, 0, 1, 0, 13.8, 26); palace(root, 0, 1.65, -1.4, 1)
  island(root, -29, -1, -18, 8.2, 23); palace(root, -29, -0.3, -18, 0.59)
  island(root, 29, 5, -25, 9.2, 30); palace(root, 29, 5.7, -25, 0.66)
  island(root, 1, -1.8, 46, 6.5, 17)
  bridge(root, new T.Vector3(0, 1.65, 12), new T.Vector3(1, -1.1, 45), 3.5)
  bridge(root, new T.Vector3(-11, 1.5, -5), new T.Vector3(-23, -0.5, -16), 2.2)
  bridge(root, new T.Vector3(10, 1.7, -7), new T.Vector3(23, 5.5, -20), 2.2)
  // Processional gate on the near island.
  const gate = new T.Group(); gate.position.set(1, -1.2, 44); root.add(gate)
  pillar(gate, -2.5, 0, 0, 5, 0.28); pillar(gate, 2.5, 0, 0, 5, 0.28)
  box(gate, [6.5, 0.48, 0.5], [0, 4.8, 0], timber); roof(gate, 5.1, 3.8, 1.35)
  cherry(root, -10, 1.7, 2, 0.9); cherry(root, 9.3, 1.7, 4, 0.75)
  cherry(root, -32, -0.4, -15, 0.68); cherry(root, 32, 5.6, -21, 0.68)
  cherry(root, -3, -1, 47, 0.73)
  // Islands recede into the mist without competing with the main palace.
  for (const [x, y, z, r, s] of [[-63, -5, -56, 8, 0.38], [55, 1, -76, 10, 0.45], [-7, 0, -91, 12, 0.53]]) {
    island(root, x!, y!, z!, r!, 26); palace(root, x!, y! + 0.6, z!, s!)
  }
  // Architecture never moves independently. Bake its transforms and combine by
  // material, so thousands of decorative parts require only a few draw calls.
  root.updateMatrixWorld(true)
  const batches = new Map<T.Material, T.BufferGeometry[]>()
  const original: T.Mesh[] = []
  root.traverse(object => {
    if (!(object instanceof T.Mesh) || Array.isArray(object.material)) return
    const geometry = object.geometry.index ? object.geometry.toNonIndexed() : object.geometry.clone()
    geometry.applyMatrix4(object.matrixWorld)
    const batch = batches.get(object.material) || []
    batch.push(geometry); batches.set(object.material, batch); original.push(object)
  })
  for (const [material, geometries] of batches) {
    const combined = mergeGeometries(geometries)
    if (!combined) throw new Error('Unable to batch palace geometry')
    root.add(new T.Mesh(combined, material))
    geometries.forEach(geometry => geometry.dispose())
  }
  original.forEach(mesh => { mesh.removeFromParent(); mesh.geometry.dispose() })
}

export function astrolabe(scene: T.Scene): T.Group {
  const g = new T.Group(); g.position.set(0, 20, -17); scene.add(g)
  const glow = new T.MeshBasicMaterial({ color: new T.Color('#dfb870').multiplyScalar(1.5), transparent: true, opacity: 0.85 })
  for (const r of [16.5, 17.1, 19]) g.add(new T.Mesh(new T.TorusGeometry(r, r === 17.1 ? 0.025 : 0.055, 6, 180), glow))
  const segments: T.Vector3[] = []
  for (let i = 0; i < 96; i++) {
    const a = i / 96 * Math.PI * 2, r = i % 8 === 0 ? 18.25 : 18.7
    segments.push(new T.Vector3(Math.cos(a) * r, Math.sin(a) * r, 0), new T.Vector3(Math.cos(a) * 19, Math.sin(a) * 19, 0))
  }
  g.add(new T.LineSegments(new T.BufferGeometry().setFromPoints(segments), new T.LineBasicMaterial({ color: '#edc992', transparent: true, opacity: 0.62 })))
  for (let i = 0; i < 8; i++) {
    const gem = new T.Mesh(new T.OctahedronGeometry(0.32), glow); const a = i * Math.PI / 4
    gem.position.set(Math.cos(a) * 17.1, Math.sin(a) * 17.1, 0); g.add(gem)
  }
  return g
}
