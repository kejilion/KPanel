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

/**
 * How a facade is built; the building shader draws it from these (buildings.ts).
 * punched: windows set in a masonry or rendered wall. curtain: a glass wall on thin mullions,
 * an opaque band at each floor. ribbon: continuous bands of glass between solid spandrels.
 * fins: tall narrow windows between deep piers. grid: square windows in a precast frame.
 * blank: plant floors and the like, clad in louvres and panels. lantern: a glass crown lit
 * from within.
 */
export const FACADE = { punched: 0, curtain: 1, ribbon: 2, fins: 3, grid: 4, blank: 5, lantern: 6 } as const

export interface Facade {
  style: number
  /** Storey height and the bay width it aims for, in metres (bays are fitted to each face). */
  floor: number
  bay: number
  /** The wall's colour, or the glass's for a curtain wall; linear. */
  tint: readonly [number, number, number]
  /** How busy it is (0-1): how many of its windows are lit when the city is. */
  occupancy: number
}

/** One box of a building: a whole low building, or a podium, shaft, setback or crown. */
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
  facade: Facade
  /** The top of the whole building this box belongs to (a spire not counted). */
  top: number
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

type Colour = readonly [number, number, number]
const BRICK_AND_STONE: readonly Colour[] = [
  [0.27, 0.12, 0.08], [0.2, 0.11, 0.07], [0.42, 0.32, 0.21], [0.32, 0.31, 0.29],
  [0.5, 0.48, 0.45], [0.44, 0.37, 0.27], [0.13, 0.09, 0.075], [0.36, 0.2, 0.14],
]
const PRECAST: readonly Colour[] = [[0.5, 0.5, 0.48], [0.4, 0.4, 0.39], [0.48, 0.43, 0.34], [0.3, 0.31, 0.33]]
const SPANDREL: readonly Colour[] = [[0.52, 0.52, 0.5], [0.28, 0.29, 0.3], [0.07, 0.075, 0.08], [0.34, 0.17, 0.1], [0.45, 0.4, 0.33]]
const PIERS: readonly Colour[] = [[0.45, 0.42, 0.37], [0.06, 0.065, 0.07], [0.2, 0.13, 0.07], [0.52, 0.52, 0.5]]
const GLASS: readonly Colour[] = [[0.05, 0.09, 0.14], [0.04, 0.1, 0.095], [0.11, 0.08, 0.05], [0.07, 0.075, 0.08], [0.12, 0.15, 0.18]]
const PANELS: readonly Colour[] = [[0.22, 0.23, 0.24], [0.4, 0.4, 0.39], [0.08, 0.085, 0.09]]

/** Low buildings are mostly brick and precast, towers mostly glass and piers. */
function styleFor(height: number, random: () => number): number {
  const r = random()
  if (height < 35) return r < 0.45 ? FACADE.punched : r < 0.62 ? FACADE.grid : r < 0.8 ? FACADE.ribbon : r < 0.9 ? FACADE.fins : FACADE.curtain
  if (height < 110) return r < 0.25 ? FACADE.punched : r < 0.48 ? FACADE.ribbon : r < 0.63 ? FACADE.grid : r < 0.8 ? FACADE.fins : FACADE.curtain
  return r < 0.45 ? FACADE.curtain : r < 0.7 ? FACADE.fins : r < 0.88 ? FACADE.ribbon : FACADE.punched
}

function facadeFor(style: number, random: () => number): Facade {
  const pick = (list: readonly Colour[]) => list[Math.floor(random() * list.length)]!
  const office = style === FACADE.curtain || style === FACADE.ribbon || style === FACADE.fins
  const floor = office ? 3.6 + random() * 0.6 : 3 + random() * 0.4
  const bay = {
    [FACADE.punched]: 2.6 + random() * 1.2,
    [FACADE.curtain]: 1.5 + random() * 0.4,
    [FACADE.ribbon]: 1.5 + random() * 0.6,
    [FACADE.fins]: 1.2 + random() * 0.6,
    [FACADE.grid]: 1.8 + random() * 0.8,
  }[style] ?? 3
  const tint = {
    [FACADE.punched]: BRICK_AND_STONE,
    [FACADE.curtain]: GLASS,
    [FACADE.ribbon]: SPANDREL,
    [FACADE.fins]: PIERS,
    [FACADE.grid]: PRECAST,
  }[style] ?? PANELS
  // Some offices are nearly dark by evening, some blocks of flats nearly all lit.
  return { style, floor, bay, tint: pick(tint), occupancy: office ? random() : 0.3 + random() * 0.7 }
}

/** Rounds a height to whole storeys, plus the roof slab. */
function storeys(height: number, facade: Facade, least = 1): number {
  return Math.max(least, Math.round(height / facade.floor)) * facade.floor + 0.6
}

interface Lot { x: number, z: number, w: number, d: number }

/** What the city plan fixes for a lot: its height, neon, seed, and a tower's setbacks and spire. */
interface Plan {
  height: number
  neon: number
  seed: number
  /** Each setback: its width and depth as a share of the floor below, and its height. */
  tiers: { w: number, d: number, h: number }[]
  spire: number
}

/**
 * One building on a lot, as boxes from the ground up. Low streets are rows of narrow buildings
 * of their own heights and fronts; mid-rise blocks are slabs, stepped tops and pairs of wings;
 * towers stand on podiums or on their own (now and then two to a lot), step back as they rise
 * (round the middle, or to one side) and finish in a lit glass lantern, stepped crown, plant
 * floor or spire.
 */
function addBuilding(buildings: Building[], lot: Lot, plan: Plan): void {
  const { height, neon } = plan
  // The plan fixes where the building stands and how tall it is; its shape and facade come from its own seed.
  const random = mulberry32(Math.floor(plan.seed * 4294967296))
  const parts: Building[] = []
  const add = (x: number, y: number, z: number, w: number, h: number, d: number, facade: Facade, street: boolean, glow = neon) => {
    parts.push({ x, y, z, w, h, d, seed: random(), neon: glow, street, facade, top: 0 })
  }
  const side = () => Math.floor(random() * 3) - 1
  const long = lot.w >= lot.d
  const length = long ? lot.w : lot.d
  const along = (offset: number, size: number) => long
    ? { x: lot.x + offset, z: lot.z, w: size, d: lot.d }
    : { x: lot.x, z: lot.z + offset, w: lot.w, d: size }

  if (height < 35) {
    const count = length > 34 && random() < 0.55 ? (length > 52 && random() < 0.5 ? 3 : 2) : 1
    const shares = Array.from({ length: count }, () => 0.7 + random() * 0.6)
    const total = shares.reduce((sum, share) => sum + share, 0)
    let cursor = -length / 2
    for (const share of shares) {
      const size = (length * share) / total
      const piece = along(cursor + size / 2, size)
      cursor += size
      const facade = facadeFor(styleFor(height, random), random)
      add(piece.x, 0, piece.z, piece.w, storeys(height * (0.6 + random() * 0.8), facade, 3), piece.d, facade, true, 0)
    }
  } else if (height < 110) {
    const facade = facadeFor(styleFor(height, random), random)
    const shape = random()
    if (shape < 0.3) {
      // The top floors stepped back from one or two sides.
      const lower = storeys(height * (0.68 + random() * 0.14), facade)
      add(lot.x, 0, lot.z, lot.w, lower, lot.d, facade, true)
      const w = lot.w * (0.55 + random() * 0.25)
      const d = lot.d * (0.55 + random() * 0.3)
      add(lot.x + (side() * (lot.w - w)) / 2, lower, lot.z + (side() * (lot.d - d)) / 2, w, storeys(height - lower, facade), d, facade, false)
    } else if (shape < 0.52 && length > 30) {
      // Two wings of different heights.
      const split = 0.45 + random() * 0.15
      const first = along(-length / 2 + (length * split) / 2, length * split)
      const second = along(length / 2 - (length * (1 - split)) / 2, length * (1 - split))
      const flip = random() < 0.5
      add(first.x, 0, first.z, first.w, storeys(flip ? height : height * (0.5 + random() * 0.3), facade), first.d, facade, true)
      add(second.x, 0, second.z, second.w, storeys(flip ? height * (0.5 + random() * 0.3) : height, facade), second.d, facade, true)
    } else {
      add(lot.x, 0, lot.z, lot.w, storeys(height, facade), lot.d, facade, true)
    }
    if (random() < 0.35) {
      // A plant room or penthouse, off to one side of the roof.
      const roof = parts.reduce((best, part) => (part.y + part.h > best.y + best.h ? part : best))
      const w = roof.w * (0.3 + random() * 0.2)
      const d = roof.d * (0.3 + random() * 0.2)
      const penthouse = random() < 0.5 ? facade : facadeFor(FACADE.blank, random)
      add(roof.x + (side() * (roof.w - w)) / 2 * 0.8, roof.y + roof.h, roof.z + (side() * (roof.d - d)) / 2 * 0.8, w, 4 + random() * 3, d, penthouse, false, 0)
    }
  } else {
    const facade = facadeFor(styleFor(height, random), random)
    let base = 0
    let shafts: Lot[] = [lot]
    if (random() < 0.6 && Math.min(lot.w, lot.d) > 26) {
      // A podium over the whole lot, the tower rising from it, often off-centre.
      const podium = facadeFor([FACADE.curtain, FACADE.ribbon, FACADE.punched][Math.floor(random() * 3)]!, random)
      base = storeys(12 + random() * 16, podium, 3)
      add(lot.x, 0, lot.z, lot.w, base, lot.d, podium, true, 0)
      const w = lot.w * (0.55 + random() * 0.2)
      const d = lot.d * (0.55 + random() * 0.2)
      const shift = random() < 0.5 ? 0 : 0.6 + random() * 0.4
      shafts = [{ x: lot.x + (side() * (lot.w - w) * shift) / 2, z: lot.z + (side() * (lot.d - d) * shift) / 2, w, d }]
    } else if (length > 44 && random() < 0.25) {
      // Twin towers.
      const gap = 6 + random() * 4
      const size = (length - gap) / 2
      shafts = [along(-(size + gap) / 2, size), along((size + gap) / 2, size)]
    }
    // The plan's setbacks, round the middle as first laid out or stepped to one side; a tower the
    // plan left straight may still step in near the top, within the same height.
    const toSide = random() < 0.5
    let tiers = plan.tiers.map((tier) => ({ ...tier }))
    let shaftHeight = height - base
    if (!tiers.length && random() < 0.4) {
      const steps = 1 + Math.floor(random() * 2)
      const kept = shaftHeight * (0.7 + random() * 0.12)
      tiers = Array.from({ length: steps }, () => ({ w: 0.7 + random() * 0.15, d: 0.7 + random() * 0.15, h: (shaftHeight - kept) / steps }))
      shaftHeight = kept
    }
    shafts.forEach((shaft, index) => {
      const scale = index ? 0.8 + random() * 0.12 : 1
      let { x, z, w, d } = shaft
      let bottom = base
      const rise = (stretch: number) => {
        const h = storeys(stretch * scale, facade)
        add(x, bottom, z, w, h, d, facade, bottom === 0)
        bottom += h
      }
      rise(shaftHeight)
      tiers.forEach((tier, step) => {
        const nw = w * tier.w
        const nd = d * tier.d
        if (toSide) {
          x += ((w - nw) / 2) * (step % 2 ? 1 : -1) * (long ? 1 : 0.4)
          z += ((d - nd) / 2) * (step % 2 ? 0.4 : 1)
        }
        w = nw
        d = nd
        rise(tier.h)
      })
      const top = bottom
      const crown = random()
      if (plan.spire) {
        add(x, top, z, 1.6, plan.spire, 1.6, facadeFor(FACADE.blank, random), false, 0)
      } else if (crown < 0.25) {
        // A glass lantern lit from within, in the tower's neon colour if it has one.
        add(x, top, z, w * 0.78, 8 + random() * 10, d * 0.78, facadeFor(FACADE.lantern, random), false)
      } else if (crown < 0.45) {
        // A stepped crown.
        let cw = w
        let cd = d
        let cy = top
        for (let step = 0; step < 2 + Math.floor(random() * 2); step++) {
          cw *= 0.78
          cd *= 0.78
          const ch = 4 + random() * 4
          add(x, cy, z, cw, ch, cd, facade, false)
          cy += ch
        }
      } else if (crown < 0.7) {
        // Plant floors behind louvres.
        add(x, top, z, w * 0.62, 6 + random() * 4, d * 0.62, facadeFor(FACADE.blank, random), false, 0)
      }
    })
  }

  const top = Math.max(...parts.filter((part) => part.w > 4).map((part) => part.y + part.h))
  for (const part of parts) {
    part.top = top
    buildings.push(part)
  }
}

/** The boxes standing on this one's roof (setbacks, crowns, plant rooms, spires). */
export function standingOn(buildings: readonly Building[], below: Building): Building[] {
  const roof = below.y + below.h
  return buildings.filter((other) => other !== below && Math.abs(other.y - roof) < 0.01
    && Math.abs(other.x - below.x) < (other.w + below.w) / 2 && Math.abs(other.z - below.z) < (other.d + below.d) / 2)
}

/** The middle of the highest roof of the building a box belongs to, climbing its setbacks and crowns. */
export function summit(buildings: readonly Building[], part: Building): THREE.Vector3 {
  let current = part
  for (;;) {
    const above = standingOn(buildings, current).filter((other) => other.w > 4)
    if (!above.length) return new THREE.Vector3(current.x, current.y + current.h, current.z)
    current = above.reduce((best, other) => (other.w * other.d > best.w * best.d ? other : best))
  }
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
          const seed = random()
          // Setbacks and spires are drawn from the plan's generator just as when the city was first
          // laid out (when each setback was a building with a seed of its own), so every lot after
          // this one keeps its place and height.
          const tiers: Plan['tiers'] = []
          let base = h
          for (let tier = 0; tier < 2 && base > 110 && random() < 0.55; tier++) {
            const tierW = 0.62 + random() * 0.18
            const tierD = 0.62 + random() * 0.18
            const tierH = base * (0.12 + random() * 0.2)
            random()
            tiers.push({ w: tierW, d: tierD, h: tierH })
            base += tierH
          }
          const spire = base > 200 && random() < 0.5 ? 24 + random() * 30 : 0
          if (spire) random()
          addBuilding(buildings, { x, z, w, d }, { height: h, neon, seed, tiers, spire })
        }
      }
    }
  }

  // A ring of lower city around the grid, taller behind downtown so the dusk skyline has depth.
  const looks = mulberry32(seed + 1)
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
    buildings.push({ x, y: 0, z, w, h, d, seed: random(), neon: 0, street: false, facade: facadeFor(styleFor(h, looks), looks), top: h })
  }

  // The tallest towers, by the shaft that rises from the ground or podium (screens go on it,
  // searchlights on the building's top).
  const landmarks = buildings
    .filter((building) => (building.street || building.y > 0) && building.top > 150 && building.w >= 12 && building.y < 30 && building.y + building.h > 90)
    .sort((a, b) => b.top - a.top)
  return { buildings, landmarks }
}
