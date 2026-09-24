"""Paints the planet of the orbital-station scene, offline and in detail.

The scene used to compute the planet's surface in its shader, with a handful of
noise octaves; here it is painted once, with many more, into equirectangular
maps the size of the sphere's UVs (three.js SphereGeometry: the top row is the
north pole, u runs round from -x through +z):

    planet-surface.webp  sRGB colour
    planet-relief.webp   object-space normal (relief of mountains and coasts)
    planet-masks.webp    red: water (for the sun's glint), green: city lights
                         for the night side (lossless: sharp coastlines)
    planet-clouds.webp   cloud cover (grey)

No map has an alpha channel: WebP (and browsers) may throw away the colour of
fully transparent pixels, so data is never kept under alpha.

Continents are domain-warped noise with ridged mountain ranges; the land is
coloured by latitude, altitude and rainfall (rainforest, grassland, savanna,
desert, tundra, rock, snow and ice caps), the sea by depth, with pale shelves
along the coasts. Cities gather on temperate and tropical coasts and lowlands.
Clouds form bands and swirls. Everything is seeded.

    py tools/planet.py <assets dir> [width]
"""
import os
import sys

import numpy as np
from PIL import Image

OUT = sys.argv[1] if len(sys.argv) > 1 else 'assets'
WIDTH = int(sys.argv[2]) if len(sys.argv) > 2 else 4096
HEIGHT = WIDTH // 2
CHUNK = 128

rng = np.random.default_rng(20260925)
PERM = np.tile(rng.permutation(256), 2).astype(np.int32)
GRADIENTS = np.array([[1, 1, 0], [-1, 1, 0], [1, -1, 0], [-1, -1, 0], [1, 0, 1], [-1, 0, 1], [1, 0, -1], [-1, 0, -1],
                      [0, 1, 1], [0, -1, 1], [0, 1, -1], [0, -1, -1], [1, 1, 0], [0, -1, 1], [-1, 1, 0], [0, -1, -1]], dtype=np.float32)


def perlin(p):
    """Improved Perlin noise at points p (N x 3), roughly in [-1, 1]."""
    base = np.floor(p)
    f = (p - base).astype(np.float32)
    i = base.astype(np.int64) & 255
    u = f * f * f * (f * (f * 6 - 15) + 10)
    total = np.zeros(len(p), dtype=np.float32)
    for dx in (0, 1):
        for dy in (0, 1):
            for dz in (0, 1):
                h = PERM[PERM[PERM[i[:, 0] + dx] + i[:, 1] + dy] + i[:, 2] + dz] & 15
                g = GRADIENTS[h]
                dot = g[:, 0] * (f[:, 0] - dx) + g[:, 1] * (f[:, 1] - dy) + g[:, 2] * (f[:, 2] - dz)
                w = (u[:, 0] if dx else 1 - u[:, 0]) * (u[:, 1] if dy else 1 - u[:, 1]) * (u[:, 2] if dz else 1 - u[:, 2])
                total += w * dot
    return total


def fbm(p, octaves, gain=0.5, lacunarity=2.03, offset=0.0):
    total = np.zeros(len(p), dtype=np.float32)
    amplitude = 0.5
    for octave in range(octaves):
        total += amplitude * perlin(p * (lacunarity ** octave) + offset + octave * 17.31)
        amplitude *= gain
    return total


def ridged(p, octaves, offset=0.0):
    """Sharp crests: mountain ranges."""
    total = np.zeros(len(p), dtype=np.float32)
    amplitude = 0.5
    weight = np.ones(len(p), dtype=np.float32)
    for octave in range(octaves):
        n = 1.0 - np.abs(perlin(p * (2.1 ** octave) + offset + octave * 7.7))
        n = n * n * weight
        weight = np.clip(n * 1.6, 0, 1)
        total += amplitude * n
        amplitude *= 0.5
    return total


def smoothstep(a, b, x):
    t = np.clip((x - a) / (b - a), 0.0, 1.0)
    return t * t * (3.0 - 2.0 * t)


def mix(a, b, t):
    return a + (b - a) * t[..., None]


def sphere_points(rows):
    """Unit-sphere positions of the pixel centres in these rows (three.js SphereGeometry layout)."""
    theta = (rows + 0.5) / HEIGHT * np.pi
    phi = (np.arange(WIDTH) + 0.5) / WIDTH * 2 * np.pi
    theta, phi = np.meshgrid(theta, phi, indexing='ij')
    return np.stack([-np.cos(phi) * np.sin(theta), np.cos(theta), np.sin(phi) * np.sin(theta)], axis=-1).reshape(-1, 3).astype(np.float32)


SEA = 0.0


def terrain(p):
    """Height above sea level (continents, ranges, detail) and rainfall, at points p."""
    warp = np.stack([fbm(p * 1.3, 4, offset=11.0), fbm(p * 1.3, 4, offset=23.0), fbm(p * 1.3, 4, offset=37.0)], axis=1)
    q = p + warp * 0.55
    # About a third land, as on Earth; heights in rough thousands of metres / 10 (0.5 = 5 km).
    continents = fbm(q * 1.15, 8) - 0.04
    ranges = ridged(q * 2.6, 7, offset=51.0)
    inland = smoothstep(0.02, 0.2, continents)
    hills = ridged(q * 14.0, 5, offset=61.0)
    height = continents * 0.6 + ranges * 0.22 * inland + hills * 0.035 * inland + fbm(q * 9.0, 6, offset=71.0) * 0.03
    rain = fbm(q * 2.0, 6, offset=91.0) + 0.25 * (1 - smoothstep(0.0, 0.25, continents))
    return height - SEA, rain


def main():
    os.makedirs(OUT, exist_ok=True)
    surface = np.zeros((HEIGHT, WIDTH, 4), dtype=np.float32)
    height_map = np.zeros((HEIGHT, WIDTH), dtype=np.float32)
    lights = np.zeros((HEIGHT, WIDTH), dtype=np.float32)
    clouds = np.zeros((HEIGHT, WIDTH), dtype=np.float32)
    for start in range(0, HEIGHT, CHUNK):
        rows = np.arange(start, min(start + CHUNK, HEIGHT))
        p = sphere_points(rows)
        latitude = np.abs(p[:, 1])
        height, rain = terrain(p)
        land = height > 0
        # Temperature: warm at the equator, cold at the poles and up high; rainfall shapes the rest.
        temperature = 1.0 - latitude * 1.1 - np.maximum(height, 0) * 0.9 + fbm(p * 3.0, 3, offset=101.0) * 0.15
        # Vegetation in patches, drier on the ridges: fine detail the eye reads as terrain.
        patches = fbm(p * 28.0, 5, offset=105.0)
        wet = rain + 0.3 * (1 - np.abs(latitude - 0.05) * 3).clip(0, 1) - 0.35 * np.exp(-((latitude - 0.42) / 0.12) ** 2) + patches * 0.18
        grain = fbm(p * 40.0, 3, offset=111.0) * 0.6 + fbm(p * 150.0, 3, offset=113.0) * 0.4
        rainforest = np.array([0.08, 0.2, 0.06])
        temperate = np.array([0.18, 0.28, 0.1])
        grass = np.array([0.36, 0.38, 0.17])
        savanna = np.array([0.5, 0.42, 0.22])
        desert = np.array([0.76, 0.6, 0.4])
        tundra = np.array([0.38, 0.36, 0.3])
        rock = np.array([0.42, 0.38, 0.34])
        snow = np.array([0.93, 0.95, 0.98])
        warm_land = mix(mix(desert, savanna, smoothstep(-0.25, -0.05, wet)), mix(grass, rainforest, smoothstep(0.0, 0.2, wet)), smoothstep(-0.12, 0.02, wet))
        cool_land = mix(mix(tundra, grass, smoothstep(-0.2, 0.05, wet)), temperate, smoothstep(0.0, 0.2, wet))
        ground = mix(cool_land, warm_land, smoothstep(0.35, 0.6, temperature))
        ground = mix(ground, rock, smoothstep(0.14, 0.26, height))
        ground *= (0.84 + 0.32 * grain)[:, None]
        ground = mix(ground, snow, np.clip(smoothstep(0.08, -0.05, temperature) + smoothstep(0.3, 0.38, height) * 0.9, 0, 1))
        # Sea: deep blue, lighter over the shelves near the coast.
        deep = np.array([0.01, 0.045, 0.13])
        shelf = np.array([0.04, 0.2, 0.3])
        sea = mix(deep, shelf, smoothstep(-0.06, 0.0, height))
        ice = smoothstep(0.9, 0.95, latitude + fbm(p * 6.0, 4, offset=121.0) * 0.06)
        colour = np.where(land[:, None], ground, sea)
        colour = mix(colour, snow, ice)
        water = (~land) & (ice < 0.5)
        r0, r1 = rows[0], rows[-1] + 1
        surface[r0:r1] = np.concatenate([colour, water[:, None].astype(np.float32)], axis=1).reshape(len(rows), WIDTH, 4)
        height_map[r0:r1] = np.where(land, height, 0.0).reshape(len(rows), WIDTH)
        # Cities: temperate and tropical lowlands near the coasts, in clusters and along lines.
        # Settled regions (broad), cities within them (clusters), and towns scattered between.
        habitable = land & (temperature > 0.3) & (ice < 0.2)
        lowland = smoothstep(0.16, 0.02, height)
        region = smoothstep(-0.15, 0.15, fbm(p * 4.0, 4, offset=125.0)) * smoothstep(-0.35, 0.05, wet)
        cities = smoothstep(0.02, 0.3, fbm(p * 16.0, 4, offset=131.0)) * smoothstep(-0.15, 0.3, fbm(p * 70.0, 3, offset=141.0))
        towns = smoothstep(0.25, 0.45, fbm(p * 220.0, 2, offset=151.0)) * 0.5
        lights[r0:r1] = (habitable * lowland * region * (cities + towns)).reshape(len(rows), WIDTH).clip(0, 1)
        # Clouds: bands (the tropics and the storm tracks of the middle latitudes) and swirls.
        swirl = np.stack([fbm(p * 2.2, 4, offset=161.0), fbm(p * 2.2, 4, offset=171.0), fbm(p * 2.2, 4, offset=181.0)], axis=1)
        cq = p + swirl * 0.8
        cq = cq * np.array([1.0, 2.2, 1.0], dtype=np.float32)
        cover = fbm(cq * 2.4, 8, offset=191.0) + 0.14 * np.exp(-((latitude - 0.05) / 0.08) ** 2) + 0.12 * np.exp(-((latitude - 0.55) / 0.12) ** 2) - 0.1 * np.exp(-((latitude - 0.3) / 0.08) ** 2)
        clouds[r0:r1] = smoothstep(0.02, 0.38, cover).reshape(len(rows), WIDTH)
        print(f'planet: rows {r0}-{r1}', flush=True)

    # Relief: the object-space normal of the height field.
    phi = (np.arange(WIDTH) + 0.5) / WIDTH * 2 * np.pi
    theta = (np.arange(HEIGHT) + 0.5) / HEIGHT * np.pi
    theta, phi = np.meshgrid(theta, phi, indexing='ij')
    sin_t = np.maximum(np.sin(theta), 1e-3)
    d_phi = (np.roll(height_map, -1, axis=1) - np.roll(height_map, 1, axis=1)) / (2 * 2 * np.pi / WIDTH)
    d_theta = (np.vstack([height_map[1:], height_map[-1:]]) - np.vstack([height_map[:1], height_map[:-1]])) / (2 * np.pi / HEIGHT)
    position = np.stack([-np.cos(phi) * np.sin(theta), np.cos(theta), np.sin(phi) * np.sin(theta)], axis=-1)
    e_phi = np.stack([np.sin(phi), np.zeros_like(phi), np.cos(phi)], axis=-1)
    e_theta = np.stack([-np.cos(phi) * np.cos(theta), -np.sin(theta), np.sin(phi) * np.cos(theta)], axis=-1)
    # Relief exaggerated a few times over, so the ranges show in low sun, but no more.
    strength = 0.12
    normal = position - strength * ((d_phi / sin_t)[..., None] * e_phi + d_theta[..., None] * e_theta)
    normal /= np.linalg.norm(normal, axis=-1, keepdims=True)

    def to_bytes(values):
        return np.clip(np.round(values * 255), 0, 255).astype(np.uint8)

    Image.fromarray(to_bytes(surface[..., :3]), 'RGB').save(os.path.join(OUT, 'planet-surface.webp'), 'WEBP', quality=88, method=6)
    Image.fromarray(to_bytes(normal * 0.5 + 0.5), 'RGB').save(os.path.join(OUT, 'planet-relief.webp'), 'WEBP', quality=90, method=6)
    masks = np.stack([surface[..., 3], lights, np.zeros_like(lights)], axis=-1)
    # Lossless: lossy WebP would let the sharp coastlines of the water mask bleed into the lights.
    Image.fromarray(to_bytes(masks), 'RGB').save(os.path.join(OUT, 'planet-masks.webp'), 'WEBP', lossless=True, method=6)
    Image.fromarray(to_bytes(clouds), 'L').save(os.path.join(OUT, 'planet-clouds.webp'), 'WEBP', quality=85, method=6)
    for name in ('planet-surface.webp', 'planet-relief.webp', 'planet-masks.webp', 'planet-clouds.webp'):
        print(name, os.path.getsize(os.path.join(OUT, name)))


main()
