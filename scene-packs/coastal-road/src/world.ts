import * as THREE from 'three'

/**
 * The coast: land to the east (+x), open sea to the west, so the sun sets over
 * the water. A headland with a lighthouse, a sandy cove, sea stacks offshore,
 * and a road winding along the cliff tops. Everything here is plain functions
 * of position, shared by the terrain, the ocean (for depth and foam), the road
 * and the traffic.
 */
export const SEA_LEVEL = 0
export const HEADLAND_Z = 380
export const COVE_Z = -260
export const WORLD_Z = 2600

function hash(n: number): number {
  const h = Math.sin(n * 127.1) * 43758.5453
  return h - Math.floor(h)
}
function noise1(x: number): number {
  const i = Math.floor(x)
  const f = x - i
  const s = f * f * (3 - 2 * f)
  return (hash(i) * (1 - s) + hash(i + 1) * s) * 2 - 1
}
function noise2(x: number, z: number): number {
  const ix = Math.floor(x)
  const iz = Math.floor(z)
  const fx = x - ix
  const fz = z - iz
  const u = fx * fx * (3 - 2 * fx)
  const v = fz * fz * (3 - 2 * fz)
  const h = (a: number, b: number) => hash(a * 57 + b * 113)
  return ((h(ix, iz) * (1 - u) + h(ix + 1, iz) * u) * (1 - v) + (h(ix, iz + 1) * (1 - u) + h(ix + 1, iz + 1) * u) * v) * 2 - 1
}
const bump = (z: number, centre: number, width: number) => Math.exp(-(((z - centre) / width) ** 2))

/** x of the waterline at z. */
export function coastX(z: number): number {
  return 40 * Math.sin(z * 0.0031 + 0.6) + 22 * Math.sin(z * 0.0093 + 2.1) + 8 * noise1(z * 0.03) + 4 * noise1(z * 0.11) + 1.6 * noise1(z * 0.4)
    - 160 * bump(z, HEADLAND_Z, 120)
    + 115 * bump(z, COVE_Z, 150)
}

/** Height of the cliff top at z: high on the headland, down to a beach in the cove. */
export function cliffHeight(z: number): number {
  const base = 50 + 16 * noise1(z * 0.004) + 9 * Math.sin(z * 0.011)
  return base * (1 - 0.86 * bump(z, COVE_Z, 115)) + 26 * bump(z, HEADLAND_Z, 140)
}

/** The road keeps back from the edge and cuts across the neck of the headland. */
export function roadX(z: number): number {
  const smooth = 40 * Math.sin(z * 0.0031 + 0.6) + 22 * Math.sin(z * 0.0093 + 2.1)
    - 160 * 0.3 * bump(z, HEADLAND_Z, 160)
    + 115 * bump(z, COVE_Z, 170)
  return smooth + 62
}

export function roadY(z: number): number {
  // A running average of the cliff tops keeps the grades gentle; the road dips to the cove.
  let sum = 0
  for (let k = -6; k <= 6; k++) sum += cliffHeight(z + k * 24)
  return Math.max(9, sum / 13) + 4
}

export interface Rock {
  x: number
  z: number
  radius: number
  height: number
  seed: number
}

function makeRocks(): Rock[] {
  const rocks: Rock[] = []
  let seed = 1
  const random = () => hash(seed++ * 1.37)
  // Sea stacks off the headland.
  for (let index = 0; index < 9; index++) {
    const z = HEADLAND_Z + (random() - 0.5) * 320
    const offshore = 40 + random() * 190
    rocks.push({ x: coastX(z) - offshore, z, radius: 9 + random() * 16, height: 12 + random() * 34, seed: random() * 100 })
  }
  // Scattered reefs and boulders along the coast, more of them near the cove.
  for (let index = 0; index < 46; index++) {
    const z = (random() - 0.5) * 2400
    const offshore = 8 + random() ** 1.6 * 230
    rocks.push({ x: coastX(z) - offshore, z, radius: 3 + random() * 9, height: 1.5 + random() * 9, seed: random() * 100 })
  }
  return rocks
}
export const ROCKS: readonly Rock[] = makeRocks()

/** Ground height at (x, z), including the seabed; rocks are separate meshes (see rockBase). */
export function groundHeight(x: number, z: number): number {
  const d = x - coastX(z)
  const top = cliffHeight(z)
  let h: number
  if (d < 0) {
    h = Math.max(-46, d * 0.11 - 1.2) + noise2(x * 0.02, z * 0.02) * 1.5
  } else {
    // A scree apron at the foot, then the cliff face (its width varies along the coast),
    // notched by erosion, rolling over into the grassy top.
    const width = 20 + 22 * (noise1(z * 0.009 + 4) * 0.5 + 0.5)
    const apron = 5 + 4 * noise1(z * 0.05)
    if (d < width) {
      const t = d / width
      const face = Math.pow(t * t * (3 - 2 * t), 0.45)
      const erosion = (noise2(x * 0.09, z * 0.05) * 0.6 + noise2(x * 0.3, z * 0.2) * 0.25) * 6 * Math.sin(Math.PI * t)
      h = Math.min(apron * t * 3, apron) + (top - apron) * face + erosion
    } else {
      h = top + (d - width) * 0.16 + (noise2(x * 0.004, z * 0.004) * 60 + noise2(x * 0.013, z * 0.013) * 14) * Math.min(1, (d - width) / 220)
    }
  }
  // A bench for the road.
  const dr = Math.abs(x - roadX(z))
  if (dr < 30) {
    const t = Math.max(0, (dr - 8) / 22)
    h = THREE.MathUtils.lerp(roadY(z) - 0.25, h, t * t * (3 - 2 * t))
  }
  return h
}

/** Rock footprints as extra height, for the ocean's shallows and foam around them. */
export function rockBase(x: number, z: number): number {
  let h = -Infinity
  for (const rock of ROCKS) {
    const r = Math.hypot(x - rock.x, z - rock.z)
    if (r < rock.radius * 1.3) h = Math.max(h, rock.height * (1 - r / (rock.radius * 1.3)))
  }
  return h
}

export const LIGHTHOUSE = new THREE.Vector3()
{
  const x = coastX(HEADLAND_Z) + 34
  LIGHTHOUSE.set(x, groundHeight(x, HEADLAND_Z), HEADLAND_Z)
}

/** What the camera must fly over at (x, z): land or sea, the sea stacks and the lighthouse. */
export function obstacleHeight(x: number, z: number): number {
  let h = Math.max(groundHeight(x, z), 1.5)
  for (const rock of ROCKS) {
    if (Math.hypot(x - rock.x, z - rock.z) < rock.radius * 1.4) h = Math.max(h, rock.height + 2)
  }
  if (Math.hypot(x - LIGHTHOUSE.x, z - LIGHTHOUSE.z) < 16) h = Math.max(h, LIGHTHOUSE.y + 32)
  return h
}

export function roadCurve(): THREE.CatmullRomCurve3 {
  const points: THREE.Vector3[] = []
  for (let z = -WORLD_Z; z <= WORLD_Z; z += 20) points.push(new THREE.Vector3(roadX(z), roadY(z), z))
  return new THREE.CatmullRomCurve3(points, false, 'centripetal')
}
