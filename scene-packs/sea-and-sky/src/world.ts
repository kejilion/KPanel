/**
 * The world is the open sea and one small group of sea stacks, rising from a
 * shoal so the water turns clear around them and breaks into foam. Everything
 * here is plain functions of position, shared by the rocks, the ocean (depth
 * and foam) and the camera (so its moves clear the stacks).
 */
export const SEA_DEPTH = 40

export interface Rock {
  x: number
  z: number
  radius: number
  height: number
  seed: number
}

// A tall stack, two companions, a broken arch-like pair and a few low reefs awash.
export const ROCKS: readonly Rock[] = [
  { x: 0, z: 0, radius: 17, height: 44, seed: 12.3 },
  { x: 38, z: -26, radius: 12, height: 29, seed: 47.1 },
  { x: -34, z: 22, radius: 10, height: 21, seed: 8.6 },
  { x: 70, z: 14, radius: 9, height: 15, seed: 63.9 },
  { x: 84, z: 30, radius: 7, height: 11, seed: 29.4 },
  { x: -70, z: -18, radius: 6, height: 4, seed: 71.2 },
  { x: 22, z: 52, radius: 5, height: 3, seed: 5.5 },
  { x: -18, z: -58, radius: 7, height: 3.5, seed: 91.8 },
  { x: 118, z: -40, radius: 5, height: 2.5, seed: 38.2 },
]

/** The seabed: deep water, shoaling towards the rocks. */
export function groundHeight(x: number, z: number): number {
  let h = -SEA_DEPTH
  for (const rock of ROCKS) {
    const r = Math.hypot(x - rock.x, z - rock.z)
    h = Math.max(h, -1.5 - Math.max(0, r - rock.radius) * 0.32)
  }
  return h
}

/** Rock footprints as extra height, for the ocean's shallows and foam around them. */
export function rockBase(x: number, z: number): number {
  let h = -Infinity
  for (const rock of ROCKS) {
    const r = Math.hypot(x - rock.x, z - rock.z)
    if (r < rock.radius * 1.12) h = Math.max(h, rock.height * (1 - r / (rock.radius * 1.12)))
  }
  return h
}

/** What the camera must fly over at (x, z): the sea, and the stacks. */
export function obstacleHeight(x: number, z: number): number {
  let h = 1.5
  for (const rock of ROCKS) {
    if (Math.hypot(x - rock.x, z - rock.z) < rock.radius * 1.4) h = Math.max(h, rock.height + 2)
  }
  return h
}
