import * as T from 'three'
import { box, line } from './math'

const marble = new T.MeshStandardMaterial({ color: '#b9c6bc', roughness: 0.72, metalness: 0.08 })
const edge = new T.MeshStandardMaterial({ color: '#c8c7b2', roughness: 0.52, metalness: 0.12 })
const timber = new T.MeshStandardMaterial({ color: '#20363a', roughness: 0.55 })
const roofMat = new T.MeshStandardMaterial({ color: '#123840', roughness: 0.39, metalness: 0.38, side: T.DoubleSide })
const gold = new T.MeshStandardMaterial({ color: '#c7a468', roughness: 0.32, metalness: 0.68, emissive: '#cc8536', emissiveIntensity: 0.15 })
const light = new T.MeshBasicMaterial({ color: new T.Color('#ffad55').multiplyScalar(1.35) })

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

export function palace(parent: T.Object3D, x: number, y: number, z: number, scale: number): T.Group {
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
  return g
}
