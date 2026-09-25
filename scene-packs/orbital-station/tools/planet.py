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

    py tools/planet.py <assets dir> [width] [clouds]

With "clouds" only the cloud map is painted again (it takes a minute, not five).
"""
import os
import sys

import numpy as np
from PIL import Image

OUT = sys.argv[1] if len(sys.argv) > 1 else 'assets'
WIDTH = int(sys.argv[2]) if len(sys.argv) > 2 else 4096
CLOUDS_ONLY = len(sys.argv) > 3 and sys.argv[3] == 'clouds'
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


def billow(p, octaves, offset=0.0):
    """Rounded lumps with creases between them, in about [0, 0.9]: heaped cloud."""
    total = np.zeros(len(p), dtype=np.float32)
    amplitude = 0.5
    for octave in range(octaves):
        total += amplitude * np.abs(perlin(p * (2.03 ** octave) + offset + octave * 13.1))
        amplitude *= 0.5
    return total


def worley(p, frequency, seed):
    """Distance to the nearest of one random point per cell (in cells), at points p: round cells."""
    q = p * frequency
    base = np.floor(q).astype(np.int64)
    frac = (q - base).astype(np.float32)
    nearest = np.full(len(p), 9.0, dtype=np.float32)
    for dx in (-1, 0, 1):
        for dy in (-1, 0, 1):
            for dz in (-1, 0, 1):
                cell = base + np.array([dx, dy, dz])
                h = (cell[:, 0] * 73856093) ^ (cell[:, 1] * 19349663) ^ (cell[:, 2] * 83492791) ^ seed
                h = (h ^ (h >> 13)) * 1274126177
                jitter = np.stack([((h >> shift) & 1023) / 1023.0 for shift in (4, 14, 24)], axis=1).astype(np.float32)
                offset = np.array([dx, dy, dz], dtype=np.float32) + jitter - frac
                nearest = np.minimum(nearest, np.einsum('ij,ij->i', offset, offset))
    return np.sqrt(nearest)


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


def cloud_cover(p, latitude):
    """How much cloud there is, 0..1, at points p: soft, translucent at the edges.

    Weather systems decide where cloud gathers (the tropics, the storm tracks of the middle
    latitudes, big swirling fronts; the subtropics stay clear); within them the cloud is heaped
    into clusters and cells of puffs (billowed noise at a few scales, stretched a little along the
    latitude as the winds do), with thin veils of cirrus drawn out east to west.
    """
    # The weather systems swirl; the cloud within them is not smeared by the swirl.
    swirl = np.stack([fbm(p * 2.2, 4, offset=161.0), fbm(p * 2.2, 4, offset=171.0), fbm(p * 2.2, 4, offset=181.0)], axis=1)
    q = p + swirl * 0.7
    weather = fbm(q * 1.8, 5, offset=191.0) + 0.16 * np.exp(-((latitude - 0.06) / 0.09) ** 2)         + 0.12 * np.exp(-((latitude - 0.55) / 0.12) ** 2) - 0.14 * np.exp(-((latitude - 0.3) / 0.08) ** 2)
    # Clusters of cumulus cells, and the cells within them: rounded puffs.
    # The cells are nudged by noise so they are irregular, of mixed sizes, and run together.
    nudge = np.stack([fbm(p * 30.0, 3, offset=241.0), fbm(p * 30.0, 3, offset=251.0), fbm(p * 30.0, 3, offset=261.0)], axis=1)
    light_warp = p + swirl * 0.08 + nudge * 0.012
    clusters = 1.0 - smoothstep(0.1, 1.0, worley(light_warp, 14.0, 7))
    cells = 1.0 - smoothstep(0.0, 1.1, worley(light_warp, 48.0, 11))
    grain = fbm(p * 160.0, 3, offset=231.0)
    density = weather + 0.2 * (clusters - 0.45) + 0.12 * (cells - 0.45) + 0.06 * grain + 0.02
    # A wide, soft rise from nothing to thick cloud: most cloud is somewhere in between.
    heaped = smoothstep(0.04, 0.5, density) ** 1.5
    # Thin veils of cirrus drawn out east to west, only where there is weather.
    veil = p + swirl * 0.2
    cirrus = smoothstep(0.1, 0.5, fbm(veil * np.array([2.0, 9.0, 2.0], dtype=np.float32), 5, offset=221.0)) * 0.18
    cirrus *= smoothstep(-0.1, 0.1, weather)
    return np.clip(np.maximum(heaped, cirrus), 0.0, 1.0)


def clouds_only():
    clouds = np.zeros((HEIGHT, WIDTH), dtype=np.float32)
    for start in range(0, HEIGHT, CHUNK):
        rows = np.arange(start, min(start + CHUNK, HEIGHT))
        p = sphere_points(rows)
        clouds[rows[0]:rows[-1] + 1] = cloud_cover(p, np.abs(p[:, 1])).reshape(len(rows), WIDTH)
        print(f'planet: cloud rows {rows[0]}-{rows[-1] + 1}', flush=True)
    save_clouds(clouds)


def save_clouds(clouds):
    image = np.clip(np.round(clouds * 255), 0, 255).astype(np.uint8)
    Image.fromarray(image, 'L').save(os.path.join(OUT, 'planet-clouds.webp'), 'WEBP', quality=85, method=6)
    print('planet-clouds.webp', os.path.getsize(os.path.join(OUT, 'planet-clouds.webp')))


def main():
    os.makedirs(OUT, exist_ok=True)
    if CLOUDS_ONLY:
        clouds_only()
        return
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
        clouds[r0:r1] = cloud_cover(p, latitude).reshape(len(rows), WIDTH)
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
    save_clouds(clouds)
    for name in ('planet-surface.webp', 'planet-relief.webp', 'planet-masks.webp'):
        print(name, os.path.getsize(os.path.join(OUT, name)))


main()
