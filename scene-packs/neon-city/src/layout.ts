import * as THREE from 'three'

/**
 * The city plan: a road grid with one wide avenue, blocks split into lots,
 * a downtown of towers, a distant skyline ring and two elevated highways
 * that cross in a stacked interchange. Everything is seeded, so the city is
 * the same on every load (and in the poster).
 */
export const PITCH = 90
export const ROAD = 20
/** The avenue the street camera stands in runs along z at this x. */
export const AVENUE_X = 45
export const AVENUE = 34
export const GRID = 11
/** Roads run between -EXTENT and EXTENT. */
export const EXTENT = (GRID + 0.5) * PITCH
export const DOWNTOWN = new THREE.Vector2(0, -260)

export interface Building {
  x: number
  y: number
  z: number
  w: number
  h: number
  d: number
  seed: number
  /** 0 = plain, otherwise 1 + index into the neon palette for edge lights and crowns. */
  neon: number
  /** Street-level buildings get shopfronts and signs; setback tiers and far towers do not. */
  street: boolean
}

export interface Highway {
  curve: THREE.CatmullRomCurve3
  width: number
  lanes: number
  rail: THREE.Color
}

export function mulberry32(seed: number): () => number {
  let state = seed >>> 0
  return () => {
    state = (state + 0x6d2b79f5) >>> 0
    let value = state
    value = Math.imul(value ^ (value >>> 15), value | 1)
    value ^= value + Math.imul(value ^ (value >>> 7), value | 61)
    return ((value ^ (value >>> 14)) >>> 0) / 4294967296
  }
}

/** Centre lines of the roads; the same list serves both axes. */
export const ROAD_CENTERS: readonly number[] = Array.from({ length: 2 * GRID + 2 }, (_, index) => (index - GRID - 0.5) * PITCH)

/** Width of a road running along z (centre x) or along x (centre z). */
export function roadWidth(center: number, alongZ: boolean): number {
  return alongZ && Math.abs(center - AVENUE_X) < 1 ? AVENUE : ROAD
}

export const HIGHWAYS: readonly Highway[] = [
  {
    // East-west, crossing over the avenue in front of the street camera.
    curve: new THREE.CatmullRomCurve3([
      new THREE.Vector3(-1500, 30, 70), new THREE.Vector3(-900, 32, 30), new THREE.Vector3(-480, 34, -30),
      new THREE.Vector3(-150, 36, -70), new THREE.Vector3(180, 36, -45), new THREE.Vector3(520, 34, 30),
      new THREE.Vector3(950, 32, 10), new THREE.Vector3(1500, 30, -30),
    ], false, 'centripetal'),
    width: 26,
    lanes: 3,
    rail: new THREE.Color().setRGB(1, 0.72, 0.42),
  },
  {
    // North-south, climbing over the first one in a stacked interchange.
    curve: new THREE.CatmullRomCurve3([
      new THREE.Vector3(-330, 26, 1500), new THREE.Vector3(-270, 30, 800), new THREE.Vector3(-200, 40, 330),
      new THREE.Vector3(-235, 54, -30), new THREE.Vector3(-330, 44, -400), new THREE.Vector3(-430, 36, -850),
      new THREE.Vector3(-470, 30, -1500),
    ], false, 'centripetal'),
    width: 22,
    lanes: 2,
    rail: new THREE.Color().setRGB(0.18, 0.78, 1),
  },
]

function heightAt(x: number, z: number, random: () => number): number {
  const distance = Math.hypot(x - DOWNTOWN.x, (z - DOWNTOWN.y) * 0.85)
  const core = Math.exp(-((distance / 430) ** 2))
  let height = 9 + random() * 20 + core * (35 + random() ** 1.6 * 190)
  if (core > 0.55 && random() < 0.1) height = 230 + random() * 150
  return height
}

function nearHighway(x: number, z: number, radius: number, samples: THREE.Vector3[][]): boolean {
  for (let index = 0; index < HIGHWAYS.length; index++) {
    const clearance = radius + HIGHWAYS[index]!.width / 2 + 6
    for (const point of samples[index]!) {
      if (Math.abs(point.x - x) < clearance && Math.abs(point.z - z) < clearance && Math.hypot(point.x - x, point.z - z) < clearance) return true
    }
  }
  return false
}

export function createLayout(seed = 20260924): { buildings: Building[], landmarks: Building[] } {
  const random = mulberry32(seed)
  const buildings: Building[] = []
  const samples = HIGHWAYS.map((highway) => highway.curve.getSpacedPoints(420))

  const half = (center: number, alongZ: boolean) => roadWidth(center, alongZ) / 2
  for (let i = -GRID; i <= GRID; i++) {
    for (let j = -GRID; j <= GRID; j++) {
      const left = (i - 0.5) * PITCH
      const right = (i + 0.5) * PITCH
      const x0 = left + half(left, true)
      const x1 = right - half(right, true)
      const z0 = (j - 0.5) * PITCH + ROAD / 2
      const z1 = (j + 0.5) * PITCH - ROAD / 2
      if (random() < 0.04) continue // a plaza
      const blockX = (x0 + x1) / 2
      const blockZ = (z0 + z1) / 2
      const downtown = Math.exp(-((Math.hypot(blockX - DOWNTOWN.x, blockZ - DOWNTOWN.y) / 380) ** 2))
      const splitX = random() < 0.6 - downtown * 0.4 ? 2 : 1
      const splitZ = random() < 0.6 - downtown * 0.4 ? 2 : 1
      for (let a = 0; a < splitX; a++) {
        for (let b = 0; b < splitZ; b++) {
          const lx0 = x0 + ((x1 - x0) * a) / splitX + (a > 0 ? 1.5 : 0)
          const lx1 = x0 + ((x1 - x0) * (a + 1)) / splitX - (a < splitX - 1 ? 1.5 : 0)
          const lz0 = z0 + ((z1 - z0) * b) / splitZ + (b > 0 ? 1.5 : 0)
          const lz1 = z0 + ((z1 - z0) * (b + 1)) / splitZ - (b < splitZ - 1 ? 1.5 : 0)
          const setback = 1 + random() * 2.5
          const w = lx1 - lx0 - setback * 2
          const d = lz1 - lz0 - setback * 2
          if (w < 8 || d < 8) continue
          const x = (lx0 + lx1) / 2
          const z = (lz0 + lz1) / 2
          if (nearHighway(x, z, Math.max(w, d) * 0.6, samples)) continue
          const h = heightAt(x, z, random)
          const neon = h > 90 && random() < 0.28 ? 1 + Math.floor(random() * 4) : 0
          const building: Building = { x, y: 0, z, w, h, d, seed: random(), neon, street: true }
          buildings.push(building)
          // Setback tiers on tall towers.
          let base = h
          let tierW = w
          let tierD = d
          for (let tier = 0; tier < 2 && base > 110 && random() < 0.55; tier++) {
            tierW *= 0.62 + random() * 0.18
            tierD *= 0.62 + random() * 0.18
            const tierH = base * (0.12 + random() * 0.2)
            buildings.push({ x, y: base, z, w: tierW, h: tierH, d: tierD, seed: random(), neon, street: false })
            base += tierH
          }
          if (base > 200 && random() < 0.5) {
            buildings.push({ x, y: base, z, w: 1.6, h: 24 + random() * 30, d: 1.6, seed: random(), neon: 0, street: false })
          }
        }
      }
    }
  }

  // A ring of lower city around the grid, taller behind downtown so the dusk skyline has depth.
  for (let index = 0; index < 1300; index++) {
    const angle = random() * Math.PI * 2
    const radius = 1120 + random() ** 0.7 * 1800
    const x = Math.cos(angle) * radius
    const z = Math.sin(angle) * radius - 150
    if (Math.abs(x) < EXTENT + 30 && Math.abs(z) < EXTENT + 30) continue
    const behind = Math.max(0, -Math.sin(angle)) ** 2
    const w = 16 + random() * 40
    const d = 16 + random() * 40
    const h = 10 + random() * 40 + behind * random() ** 2 * 170
    buildings.push({ x, y: 0, z, w, h, d, seed: random(), neon: 0, street: false })
  }

  const landmarks = buildings
    .filter((building) => building.street && building.h > 150)
    .sort((a, b) => b.h - a.h)
  return { buildings, landmarks }
}
