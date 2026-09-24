/**
 * The world is the open sea and one small group of sea stacks, rising from a
 * shoal so the water turns clear around them and breaks into foam. Everything
 * here is plain functions of position, shared by the rocks, the ocean (depth
 * and foam) and the camera (so its moves clear the stacks).
 */
import rocks from './rocks.json'

export const SEA_DEPTH = 40

export interface Rock {
  x: number
  z: number
  radius: number
  height: number
  seed: number
}

// A tall stack, two companions, a broken arch-like pair and a few low reefs awash. The list is
// shared with blender/rocks.py, which sculpts the stacks from it.
export const ROCKS: readonly Rock[] = rocks

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
    if (Math.hypot(x - rock.x, z - rock.z) < rock.radius * 1.4) h = Math.max(h, rock.height * 1.1 + 2)
  }
  return h
}
