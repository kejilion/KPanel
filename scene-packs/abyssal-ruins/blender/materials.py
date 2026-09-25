"""Deterministic tiled PBR materials for the Blender / glTF abyssal ruins.

All surface detail is baked into ordinary PNG maps. The render and exported GLB
therefore share the same base color, roughness and tangent-space normal textures.
Call ``create_materials(path)`` from Blender; no scene or render state is changed.
"""

from pathlib import Path
import struct
import zlib

import bpy
import numpy as np


TEXTURE_VERSION = "abyss-pbr-v2"


def _noise(size, cells, rng):
    """Smooth periodic value noise with no seam at UV repeat boundaries."""
    lattice = rng.random((cells, cells), dtype=np.float32)
    coord = np.arange(size, dtype=np.float32) * (cells / size)
    index = np.floor(coord).astype(np.int32)
    t = coord - index
    t = t * t * t * (t * (t * 6.0 - 15.0) + 10.0)
    index %= cells
    following = (index + 1) % cells
    a = lattice[index[:, None], index[None, :]]
    b = lattice[index[:, None], following[None, :]]
    c = lattice[following[:, None], index[None, :]]
    d = lattice[following[:, None], following[None, :]]
    return (a * (1 - t[None, :]) + b * t[None, :]) * (1 - t[:, None]) + (
        c * (1 - t[None, :]) + d * t[None, :]
    ) * t[:, None]


def _fbm(size, rng, octaves=(4, 9, 19, 43, 91), weights=(0.40, 0.26, 0.17, 0.11, 0.06)):
    result = np.zeros((size, size), dtype=np.float32)
    for cells, weight in zip(octaves, weights):
        result += (_noise(size, cells, rng) - 0.5) * weight
    return result


def _pores(size, rng, count):
    pits = np.zeros((size, size), dtype=np.float32)
    for _ in range(count):
        x, y = rng.integers(0, size, 2)
        radius = rng.uniform(0.6, 2.8)
        extent = int(np.ceil(radius * 2.3))
        offsets = np.arange(-extent, extent + 1)
        xx, yy = np.meshgrid(offsets, offsets)
        pit = np.exp(-(xx * xx + yy * yy) / (radius * radius)) * rng.uniform(0.25, 1.0)
        indices = np.ix_((y + offsets) % size, (x + offsets) % size)
        pits[indices] += pit
    return np.clip(pits, 0.0, 1.0)


def _normal(height, scale):
    dx = (np.roll(height, -1, axis=1) - np.roll(height, 1, axis=1)) * scale
    # Image row 0 is the top row; tangent +Y points toward increasing UV V.
    dy = (np.roll(height, -1, axis=0) - np.roll(height, 1, axis=0)) * scale
    normal = np.stack((-dx, dy, np.ones_like(dx)), axis=-1)
    normal /= np.linalg.norm(normal, axis=-1, keepdims=True)
    return np.clip((normal * 0.5 + 0.5) * 255.0, 0, 255).astype(np.uint8)


def _rgb(base, variation, tint=None):
    values = np.asarray(base, dtype=np.float32)[None, None, :] + variation[:, :, None]
    if tint is not None:
        values += tint
    # Keep smooth gradients, with restrained sub-byte grain for compact PNGs.
    return np.clip(np.round(values), 0, 255).astype(np.uint8)


def _write_png(path, pixels):
    """Lossless PNG using row subtraction; no image library or color transform."""
    if pixels.ndim == 2:
        pixels = np.repeat(pixels[:, :, None], 3, axis=2)
    height, width, _ = pixels.shape
    filtered = np.empty((height, width * 3 + 1), dtype=np.uint8)
    filtered[:, 0] = 1  # PNG Sub filter.
    filtered[:, 1:4] = pixels[:, 0, :]
    filtered[:, 4:] = (pixels[:, 1:, :] - pixels[:, :-1, :]).reshape(height, -1)
    def chunk(kind, content):
        return struct.pack(">I", len(content)) + kind + content + struct.pack(">I", zlib.crc32(kind + content))
    data = b"\x89PNG\r\n\x1a\n"
    data += chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
    data += chunk(b"IDAT", zlib.compress(filtered.tobytes(), 9))
    data += chunk(b"IEND", b"")
    path.write_bytes(data)


def _generate(texture_dir):
    rng = np.random.default_rng(580021)
    size = 1024
    broad = _fbm(size, rng)
    grains = _noise(size, 257, rng) - 0.5
    pores = _pores(size, rng, 2500)
    mineral = _noise(size, 7, rng) - 0.5
    # Uneven deposits and scattered pits; no Voronoi cells or brick-like grid.
    stain = np.clip((_noise(size, 13, rng) - 0.53) * 3.0, 0, 1)
    color = _rgb((126, 130, 120), broad * 52 + grains * 5 - pores * 21 - stain * 11,
                 mineral[:, :, None] * np.array((7, 3, -4)))
    _write_png(texture_dir / "stone-color.png", color)
    height = broad * 0.65 + grains * 0.025 - pores * 0.09
    _write_png(texture_dir / "stone-normal.png", _normal(height, 2.4))
    rough = np.clip(209 + broad * 37 + pores * 14 + stain * 9, 184, 238).astype(np.uint8)
    _write_png(texture_dir / "stone-roughness.png", rough[::2, ::2])

    sand = _fbm(size, rng, (3, 11, 31, 97), (0.4, 0.27, 0.21, 0.12))
    sand_grain = _noise(size, 313, rng) - 0.5
    flecks = _pores(size, rng, 3800)
    _write_png(texture_dir / "sand-color.png",
               _rgb((112, 123, 116), sand * 34 + sand_grain * 4 - flecks * 7))
    # No repeating sinusoidal ripples: large seabed relief belongs to geometry.
    _write_png(texture_dir / "sand-normal.png", _normal(sand * 0.36 + sand_grain * 0.025 - flecks * 0.016, 2.6))
    _write_png(texture_dir / "sand-roughness.png",
               np.clip(229 + sand[::2, ::2] * 22, 215, 241).astype(np.uint8))

    size = 512
    basalt = _fbm(size, rng)
    fine = _noise(size, 127, rng) - 0.5
    _write_png(texture_dir / "basalt-color.png", _rgb((48, 58, 57), basalt * 34 + fine * 3))
    _write_png(texture_dir / "basalt-roughness.png", np.clip(175 + basalt * 53, 150, 203).astype(np.uint8))
    patina = np.clip((_noise(size, 11, rng) - 0.34) * 2.4, 0, 1)
    wear = _fbm(size, rng)
    bronze = np.array((132, 108, 61)) * (1 - patina[:, :, None]) + np.array((47, 84, 72)) * patina[:, :, None]
    _write_png(texture_dir / "bronze-color.png", np.clip(bronze + wear[:, :, None] * 38, 0, 255).astype(np.uint8))
    _write_png(texture_dir / "bronze-roughness.png", np.clip(133 + patina * 72 + wear * 20, 119, 211).astype(np.uint8))
    growth = _fbm(size, rng)
    _write_png(texture_dir / "algae-color.png", _rgb((43, 61, 43), growth * 33 + fine * 2))
    (texture_dir / ".texture-version").write_text(TEXTURE_VERSION + "\n", encoding="utf-8")


def _image_node(nodes, texture_dir, filename, location, color=False):
    image = bpy.data.images.load(str(texture_dir / filename), check_existing=True)
    image.colorspace_settings.name = "sRGB" if color else "Non-Color"
    node = nodes.new("ShaderNodeTexImage")
    node.name = filename
    node.label = filename
    node.image = image
    node.extension = "REPEAT"
    node.interpolation = "Linear"
    node.location = location
    return node


def _pbr(name, texture_dir, color, roughness, normal, normal_strength=0.5, metallic=0.0):
    material = bpy.data.materials.get(name) or bpy.data.materials.new(name)
    material.use_nodes = True
    material.node_tree.nodes.clear()
    nodes, links = material.node_tree.nodes, material.node_tree.links
    surface = nodes.new("ShaderNodeBsdfPrincipled")
    surface.location = (80, 60)
    surface.inputs["Metallic"].default_value = metallic
    surface.inputs["Roughness"].default_value = 0.82
    surface.inputs["IOR"].default_value = 1.48
    material.diffuse_color = (0.25, 0.28, 0.24, 1)
    output = nodes.new("ShaderNodeOutputMaterial")
    output.location = (430, 60)
    links.new(surface.outputs["BSDF"], output.inputs["Surface"])
    base = _image_node(nodes, texture_dir, color, (-610, 330), color=True)
    links.new(base.outputs["Color"], surface.inputs["Base Color"])
    if roughness:
        texture = _image_node(nodes, texture_dir, roughness, (-610, 30))
        links.new(texture.outputs["Color"], surface.inputs["Roughness"])
    if normal:
        texture = _image_node(nodes, texture_dir, normal, (-610, -270))
        transform = nodes.new("ShaderNodeNormalMap")
        transform.location = (-220, -160)
        transform.inputs["Strength"].default_value = normal_strength
        links.new(texture.outputs["Color"], transform.inputs["Color"])
        links.new(transform.outputs["Normal"], surface.inputs["Normal"])
    return material


def create_materials(texture_dir):
    """Return stone, sand, basalt, bronze, algae and glow bpy materials.

    UVs tile naturally. A stone/sand repeat spans about 3–4 world metres. Calling
    repeatedly reuses the same names and on-disk images without growing the scene.
    """
    texture_dir = Path(texture_dir).resolve()
    texture_dir.mkdir(parents=True, exist_ok=True)
    marker = texture_dir / ".texture-version"
    expected = ("stone-color", "stone-normal", "stone-roughness", "sand-color",
                "sand-normal", "sand-roughness", "basalt-color", "basalt-roughness",
                "bronze-color", "bronze-roughness", "algae-color")
    if not marker.exists() or marker.read_text(encoding="utf-8").strip() != TEXTURE_VERSION or any(
        not (texture_dir / (name + ".png")).exists() for name in expected
    ):
        _generate(texture_dir)
    materials = {
        "stone": _pbr("Abyss | porous limestone", texture_dir, "stone-color.png", "stone-roughness.png", "stone-normal.png", 0.72),
        "sand": _pbr("Abyss | fine seabed sand", texture_dir, "sand-color.png", "sand-roughness.png", "sand-normal.png", 0.48),
        "basalt": _pbr("Abyss | ancient dark basalt", texture_dir, "basalt-color.png", "basalt-roughness.png", "stone-normal.png", 0.34),
        "bronze": _pbr("Abyss | tarnished bronze", texture_dir, "bronze-color.png", "bronze-roughness.png", "stone-normal.png", 0.22, 0.64),
        "algae": _pbr("Abyss | olive marine growth", texture_dir, "algae-color.png", "stone-roughness.png", "stone-normal.png", 0.85),
    }
    glow = bpy.data.materials.get("Abyss | amber inlay") or bpy.data.materials.new("Abyss | amber inlay")
    glow.use_nodes = True
    surface = glow.node_tree.nodes.get("Principled BSDF")
    surface.inputs["Base Color"].default_value = (0.72, 0.41, 0.14, 1.0)
    surface.inputs["Metallic"].default_value = 0.2
    surface.inputs["Roughness"].default_value = 0.4
    surface.inputs["Emission Color"].default_value = (1.0, 0.64, 0.22, 1.0)
    surface.inputs["Emission Strength"].default_value = 2.5
    materials["glow"] = glow
    return materials
