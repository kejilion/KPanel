import * as T from 'three'

export function random(seed = 3127): () => number {
  return () => { seed = (Math.imul(1664525, seed) + 1013904223) >>> 0; return seed / 4294967296 }
}
export const smooth = (t: number) => { t = T.MathUtils.clamp(t, 0, 1); return t * t * (3 - 2 * t) }
export function line(points: T.Vector3[], radius: number, material: T.Material, parent: T.Object3D): T.Mesh {
  const mesh = new T.Mesh(new T.TubeGeometry(new T.CatmullRomCurve3(points), Math.max(12, points.length * 3), radius, 5, false), material)
  parent.add(mesh)
  return mesh
}
export function box(parent: T.Object3D, size: number[], position: number[], material: T.Material): T.Mesh {
  const mesh = new T.Mesh(new T.BoxGeometry(size[0], size[1], size[2]), material)
  mesh.position.set(position[0]!, position[1]!, position[2]!)
  parent.add(mesh)
  return mesh
}
