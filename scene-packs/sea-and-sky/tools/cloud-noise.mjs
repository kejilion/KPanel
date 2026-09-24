// Generates the two tileable 3D noise volumes the clouds are built from, as raw bytes:
//   assets/cloud-shape.bin  96^3: Perlin-Worley, the billowing mass of the clouds
//   assets/cloud-detail.bin 32^3: Worley fbm, the wisps eaten out of their edges
// Both wrap on every axis, so the sky can scroll through them forever.
//
//   node tools/cloud-noise.mjs
import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const assets = join(dirname(fileURLToPath(import.meta.url)), '..', 'assets')

function hash(x, y, z, seed) {
  let h = (x * 374761393 + y * 668265263 + z * 2147483647 + seed * 144269504) | 0
  h = Math.imul(h ^ (h >>> 13), 1274126177)
  return ((h ^ (h >>> 16)) >>> 0) / 4294967296
}

/** Gradient noise with period `period` lattice cells on every axis, in about [-1, 1]. */
function perlin(x, y, z, period, seed) {
  const x0 = Math.floor(x)
  const y0 = Math.floor(y)
  const z0 = Math.floor(z)
  const fade = (t) => t * t * t * (t * (t * 6 - 15) + 10)
  const u = fade(x - x0)
  const v = fade(y - y0)
  const w = fade(z - z0)
  let total = 0
  for (let dz = 0; dz < 2; dz++) {
    for (let dy = 0; dy < 2; dy++) {
      for (let dx = 0; dx < 2; dx++) {
        const cx = (((x0 + dx) % period) + period) % period
        const cy = (((y0 + dy) % period) + period) % period
        const cz = (((z0 + dz) % period) + period) % period
        const theta = hash(cx, cy, cz, seed) * Math.PI * 2
        const phi = Math.acos(hash(cx, cy, cz, seed + 1) * 2 - 1)
        const gx = Math.sin(phi) * Math.cos(theta)
        const gy = Math.sin(phi) * Math.sin(theta)
        const gz = Math.cos(phi)
        const dot = gx * (x - x0 - dx) + gy * (y - y0 - dy) + gz * (z - z0 - dz)
        total += dot * (dx ? u : 1 - u) * (dy ? v : 1 - v) * (dz ? w : 1 - w)
      }
    }
  }
  return total * 1.6
}

/** Distance to the nearest of one random point per cell, `cells` cells per axis, wrapped; in [0, 1]. */
function worley(x, y, z, cells, seed) {
  const cx = Math.floor(x)
  const cy = Math.floor(y)
  const cz = Math.floor(z)
  let nearest = 9
  for (let dz = -1; dz <= 1; dz++) {
    for (let dy = -1; dy <= 1; dy++) {
      for (let dx = -1; dx <= 1; dx++) {
        const ix = cx + dx
        const iy = cy + dy
        const iz = cz + dz
        const wx = ((ix % cells) + cells) % cells
        const wy = ((iy % cells) + cells) % cells
        const wz = ((iz % cells) + cells) % cells
        const px = ix + hash(wx, wy, wz, seed)
        const py = iy + hash(wx, wy, wz, seed + 7)
        const pz = iz + hash(wx, wy, wz, seed + 13)
        nearest = Math.min(nearest, (px - x) ** 2 + (py - y) ** 2 + (pz - z) ** 2)
      }
    }
  }
  return Math.min(1, Math.sqrt(nearest))
}

function volume(size, sample) {
  const bytes = new Uint8Array(size ** 3)
  for (let z = 0; z < size; z++) {
    for (let y = 0; y < size; y++) {
      for (let x = 0; x < size; x++) {
        const value = sample((x + 0.5) / size, (y + 0.5) / size, (z + 0.5) / size)
        bytes[(z * size + y) * size + x] = Math.round(Math.min(1, Math.max(0, value)) * 255)
      }
    }
  }
  return bytes
}

const remap = (value, low, high, toLow, toHigh) => toLow + ((value - low) / (high - low)) * (toHigh - toLow)
const worleyFbm = (x, y, z, cells, seed) =>
  (1 - worley(x * cells, y * cells, z * cells, cells, seed)) * 0.625
  + (1 - worley(x * cells * 2, y * cells * 2, z * cells * 2, cells * 2, seed + 1)) * 0.25
  + (1 - worley(x * cells * 4, y * cells * 4, z * cells * 4, cells * 4, seed + 2)) * 0.125

// The mass: low-frequency Perlin fbm, dilated by Worley so it billows in rounded lobes.
const shape = volume(96, (x, y, z) => {
  let fbm = 0
  let amplitude = 0.5
  for (let octave = 0; octave < 4; octave++) {
    const period = 4 << octave
    fbm += amplitude * perlin(x * period, y * period, z * period, period, 11 + octave)
    amplitude *= 0.5
  }
  const cells = worleyFbm(x, y, z, 5, 31)
  return remap(fbm * 0.5 + 0.5, cells - 1, 1, 0, 1)
})

// The wisps: higher-frequency Worley fbm.
const detail = volume(32, (x, y, z) => worleyFbm(x, y, z, 4, 71))

mkdirSync(assets, { recursive: true })
writeFileSync(join(assets, 'cloud-shape.bin'), shape)
writeFileSync(join(assets, 'cloud-detail.bin'), detail)
console.log(`cloud noise: shape ${shape.length} bytes, detail ${detail.length} bytes`)
