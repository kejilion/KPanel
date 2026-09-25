"""Builds the orbital station in Blender and exports it for the web.

The station keeps the layout the scene was framed around (a 78 m spine, a hub,
two modules, a 30 m habitat ring on six spokes, a docking ring near the nose,
a 104 m truss carrying four solar wings, radiators and a dish) but every part
is modelled the way real stations are built: pressurised modules wrapped in
gold and silver insulation blankets, ribbed radiators, lattice trusses, solar
blankets of individual cells, a habitat ring of segmented modules with rows of
windows, docking ports, tanks, antennas and handrails. Materials are PBR with
generated, tileable textures (painted panels with seams and rivets, crinkled
foil, solar cells, radiator ribs, dark metal), and the moving parts are
separate nodes the scene turns: habitat, dock and wings.

    blender -b --factory-startup --python blender/station.py -- <out dir>

Everything is seeded, so a rerun gives the same station.
"""
import math
import os
import sys

import bmesh
import bpy
import numpy as np
from mathutils import Matrix, Vector

OUT = os.path.abspath(sys.argv[sys.argv.index('--') + 1]) if '--' in sys.argv else os.path.join(os.path.dirname(__file__), 'out')
RNG = np.random.default_rng(20260925)


# Coordinates ---------------------------------------------------------------------------------
# The scene is laid out in three.js axes (y up). Blender is z up; the glTF exporter turns Blender's
# (x, y, z) into (x, z, -y). So a three.js point (x, y, z) is authored here as (x, -z, y).

def T(x, y, z):
    return Vector((x, -z, y))


def S(x, y, z):
    """A box size given in three.js axes, for a box that is not turned."""
    return (x, z, y)


AXES = {
    'x': Matrix.Rotation(math.pi / 2, 4, 'Y'),   # local z -> three x
    'y': Matrix.Identity(4),                      # local z -> three y
    'z': Matrix.Rotation(-math.pi / 2, 4, 'X'),  # local z -> three z (Blender -y)
}


# Textures ------------------------------------------------------------------------------------

def tile_noise(size, cells, seed):
    """Smooth, tileable value noise in [0, 1]."""
    rng = np.random.default_rng(seed)
    lattice = rng.random((cells, cells))
    coords = np.arange(size) * cells / size
    i0 = np.floor(coords).astype(int)
    f = coords - i0
    f = f * f * (3 - 2 * f)
    i1 = (i0 + 1) % cells
    rows0 = lattice[i0][:, i0] * (1 - f)[None, :] + lattice[i0][:, i1] * f[None, :]
    rows1 = lattice[i1][:, i0] * (1 - f)[None, :] + lattice[i1][:, i1] * f[None, :]
    return rows0 * (1 - f)[:, None] + rows1 * f[:, None]


def tile_fbm(size, cells, octaves, seed):
    total = np.zeros((size, size))
    amplitude = 0.5
    for octave in range(octaves):
        total += amplitude * tile_noise(size, min(cells << octave, size), seed + octave)
        amplitude *= 0.5
    return total / (1 - 0.5 ** octaves)


def normal_from_height(height, strength):
    dx = (np.roll(height, -1, axis=1) - np.roll(height, 1, axis=1)) * 0.5 * strength
    dy = (np.roll(height, -1, axis=0) - np.roll(height, 1, axis=0)) * 0.5 * strength
    normal = np.stack([-dx, dy, np.ones_like(height)], axis=2)
    normal /= np.linalg.norm(normal, axis=2, keepdims=True)
    return normal * 0.5 + 0.5


def panel_ids(size, cells, seed):
    """Panels of a few sizes on a grid: an id per pixel and the distance to the nearest seam."""
    rng = np.random.default_rng(seed)
    ids = -np.ones((cells, cells), dtype=int)
    next_id = 0
    for y in range(cells):
        for x in range(cells):
            if ids[y, x] >= 0:
                continue
            width = 1 + int(rng.random() < 0.45) + int(rng.random() < 0.2)
            height = 1 + int(rng.random() < 0.35)
            for dy in range(height):
                for dx in range(width):
                    yy, xx = (y + dy) % cells, (x + dx) % cells
                    if ids[yy, xx] < 0:
                        ids[yy, xx] = next_id
            next_id += 1
    cell = size // cells
    pixel_ids = np.repeat(np.repeat(ids, cell, axis=0), cell, axis=1)
    edge = (pixel_ids != np.roll(pixel_ids, 1, axis=0)) | (pixel_ids != np.roll(pixel_ids, 1, axis=1)) \
        | (pixel_ids != np.roll(pixel_ids, -1, axis=0)) | (pixel_ids != np.roll(pixel_ids, -1, axis=1))
    return pixel_ids, next_id, edge


def hull_maps(size=1024):
    """Painted panels, 8 m across: seams, rivets, a few darker panels, grime towards the seams."""
    ids, count, edge = panel_ids(size, 8, 11)
    rng = np.random.default_rng(12)
    tint = 0.72 + 0.14 * rng.random(count)
    dark = rng.random(count) < 0.12
    tint = np.where(dark, 0.38 + 0.1 * rng.random(count), tint)
    warm = rng.normal(0.0, 0.015, count)
    base = tint[ids]
    grime = tile_fbm(size, 8, 5, 13)
    near_seam = edge.astype(float)
    for _ in range(6):
        near_seam = np.maximum(near_seam, 0.8 * (np.roll(near_seam, 1, 0) + np.roll(near_seam, -1, 0) + np.roll(near_seam, 1, 1) + np.roll(near_seam, -1, 1)) / 4)
    shade = base * (1 - 0.12 * grime * near_seam - 0.06 * grime)
    colour = np.stack([shade + warm[ids], shade + warm[ids] * 0.5, shade - warm[ids] * 0.3 + 0.01], axis=2)
    height = np.where(edge, -1.0, 0.0) + 0.15 * tile_fbm(size, 16, 3, 14)
    # Rivets along the seams.
    yy, xx = np.mgrid[0:size, 0:size]
    rivet = ((xx % 16 == 8) & (yy % 128 < 6)) | ((yy % 16 == 8) & (xx % 128 < 6))
    height += np.where(rivet & ~edge, 0.6, 0.0)
    colour[edge] *= 0.45
    roughness = 0.42 + 0.2 * rng.random(count)[ids] + 0.1 * grime
    return colour, normal_from_height(height, 2.5), roughness, 0.08


def foil_maps(size=512, gold=True):
    """Multi-layer insulation: crinkled foil, 2 m across."""
    crinkle = tile_fbm(size, 8, 6, 21 if gold else 22)
    ridges = 1.0 - np.abs(tile_fbm(size, 16, 4, 23 if gold else 24) * 2 - 1)
    height = crinkle * 0.6 + ridges ** 3 * 0.8
    base = np.array([0.92, 0.66, 0.25]) if gold else np.array([0.78, 0.8, 0.82])
    colour = base[None, None, :] * (0.75 + 0.35 * crinkle[:, :, None])
    roughness = 0.22 + 0.25 * (1 - ridges)
    return np.clip(colour, 0, 1), normal_from_height(height, 12.0), roughness, 1.0


def metal_maps(size=512):
    """Dark structural metal with scuffs, 2 m across."""
    scuffs = tile_fbm(size, 8, 6, 31)
    streaks = tile_fbm(size, 32, 3, 32)
    grey = 0.3 + 0.08 * scuffs
    colour = np.stack([grey, grey * 1.02, grey * 1.06], axis=2)
    roughness = 0.38 + 0.25 * streaks
    return colour, normal_from_height(scuffs, 1.5), roughness, 0.6


def solar_maps(size=1024):
    """Solar blanket: cells a quarter metre across with silver fingers, 2 m across."""
    cells = 8
    cell = size // cells
    yy, xx = np.mgrid[0:size, 0:size]
    u = (xx % cell) / cell
    v = (yy % cell) / cell
    gap = (u < 0.03) | (u > 0.97) | (v < 0.03) | (v > 0.97)
    fingers = (np.abs(((u * 12) % 1) - 0.5) < 0.04) & ~gap
    busbar = (np.abs(v - 0.33) < 0.012) | (np.abs(v - 0.66) < 0.012)
    rng = np.random.default_rng(41)
    per_cell = rng.normal(0, 0.012, (cells, cells))[yy // cell, xx // cell]
    colour = np.stack([0.05 + per_cell, 0.08 + per_cell + 0.03 * v, 0.22 + per_cell + 0.05 * v], axis=2)
    colour[fingers | busbar] = [0.32, 0.34, 0.36]
    colour[gap] = [0.7, 0.72, 0.74]
    height = np.where(gap, -1.0, 0.0) + np.where(fingers | busbar, 0.2, 0.0)
    roughness = np.where(gap, 0.5, 0.12)
    return colour, normal_from_height(height, 1.5), roughness, np.where(gap, 0.2, 0.6)


def radiator_maps(size=512):
    """White radiator panels ribbed every 10 cm, 2 m across."""
    yy, xx = np.mgrid[0:size, 0:size]
    ribs = 0.5 + 0.5 * np.cos((xx / size) * 20 * 2 * math.pi)
    grime = tile_fbm(size, 4, 5, 51)
    shade = 0.82 - 0.08 * grime
    colour = np.stack([shade, shade, shade * 1.01], axis=2)
    return colour, normal_from_height(ribs, 1.2), 0.35 + 0.15 * grime, 0.05


def window_maps():
    """Window panes: 16 x 4 cells, each lit a little differently (the glass itself is dark)."""
    rng = np.random.default_rng(61)
    size_x, size_y = 256, 64
    cells = rng.random((4, 16))
    warm = rng.random((4, 16)) < 0.8
    glow = np.where(cells < 0.18, 0.05, 0.55 + 0.45 * cells)
    colour = np.where(warm[..., None], np.array([1.0, 0.78, 0.48]), np.array([0.62, 0.8, 1.0])) * glow[..., None]
    emissive = np.repeat(np.repeat(colour, size_y // 4, axis=0), size_x // 16, axis=1)
    return emissive


def image_from(name, pixels, colour=True):
    """Saves a (h, w, 3|4) float array as a PNG in OUT and loads it as a Blender image."""
    h, w = pixels.shape[:2]
    if pixels.shape[2] == 3:
        pixels = np.concatenate([pixels, np.ones((h, w, 1))], axis=2)
    image = bpy.data.images.new(name, w, h, alpha=True, float_buffer=False)
    image.pixels.foreach_set(np.clip(pixels[::-1], 0, 1).astype(np.float32).ravel())
    path = os.path.join(OUT, f'{name}.png')
    image.filepath_raw = path
    image.file_format = 'PNG'
    image.save()
    bpy.data.images.remove(image)
    loaded = bpy.data.images.load(path)
    loaded.colorspace_settings.name = 'sRGB' if colour else 'Non-Color'
    return loaded


def pbr_material(name, maps, tile, emission=None):
    colour, normal, roughness, metallic = maps
    size = colour.shape[0]
    roughness = np.broadcast_to(roughness, colour.shape[:2])
    metallic = np.broadcast_to(metallic, colour.shape[:2])
    orm = np.stack([np.ones(colour.shape[:2]), roughness, metallic], axis=2)
    material = bpy.data.materials.new(name)
    material.use_nodes = True
    nodes = material.node_tree.nodes
    links = material.node_tree.links
    bsdf = nodes['Principled BSDF']
    albedo = nodes.new('ShaderNodeTexImage')
    albedo.image = image_from(f'{name}-colour', colour)
    links.new(albedo.outputs['Color'], bsdf.inputs['Base Color'])
    packed = nodes.new('ShaderNodeTexImage')
    packed.image = image_from(f'{name}-orm', orm, colour=False)
    split = nodes.new('ShaderNodeSeparateColor')
    links.new(packed.outputs['Color'], split.inputs['Color'])
    links.new(split.outputs['Green'], bsdf.inputs['Roughness'])
    links.new(split.outputs['Blue'], bsdf.inputs['Metallic'])
    bump = nodes.new('ShaderNodeTexImage')
    bump.image = image_from(f'{name}-normal', normal, colour=False)
    normal_map = nodes.new('ShaderNodeNormalMap')
    links.new(bump.outputs['Color'], normal_map.inputs['Color'])
    links.new(normal_map.outputs['Normal'], bsdf.inputs['Normal'])
    if emission is not None:
        glow = nodes.new('ShaderNodeTexImage')
        glow.image = image_from(f'{name}-emissive', emission)
        links.new(glow.outputs['Color'], bsdf.inputs['Emission Color'])
        bsdf.inputs['Emission Strength'].default_value = 1.0
    material['tile'] = tile
    return material


# Geometry ------------------------------------------------------------------------------------

class Part:
    """One exported node: a mesh built up from pieces, each with its own material."""

    def __init__(self, name, materials, origin=Vector((0, 0, 0))):
        self.name = name
        self.bm = bmesh.new()
        self.uv = self.bm.loops.layers.uv.new('UVMap')
        self.mapped = self.bm.faces.layers.int.new('mapped')
        self.materials = materials
        self.origin = origin

    def slot(self, material):
        return self.materials.index(material)

    def tile(self, material):
        return material['tile']

    def _finish(self, geom, material, matrix, smooth_axis):
        faces = [element for element in geom if isinstance(element, bmesh.types.BMFace)]
        verts = [element for element in geom if isinstance(element, bmesh.types.BMVert)]
        bmesh.ops.transform(self.bm, matrix=Matrix.Translation(-self.origin) @ matrix, verts=verts)
        index = self.slot(material)
        for face in faces:
            face.material_index = index
            face.smooth = smooth_axis
        return faces

    def cylinder(self, centre, axis, radius, length, material, segments=32, radius2=None, caps=True, smooth=True):
        result = bmesh.ops.create_cone(self.bm, cap_ends=caps, cap_tris=False, segments=segments,
                                       radius1=radius, radius2=radius if radius2 is None else radius2, depth=length)
        matrix = Matrix.Translation(centre) @ AXES[axis] if isinstance(axis, str) else Matrix.Translation(centre) @ axis
        faces = self._finish(result['verts'] + list({f for v in result['verts'] for f in v.link_faces}), material, matrix, smooth)
        # Sides unwrapped round the axis, so panels run straight round it; caps are box-mapped later.
        direction = (matrix.to_3x3() @ Vector((0, 0, 1))).normalized()
        inverse = (Matrix.Translation(-self.origin) @ matrix).inverted()
        tile = self.tile(material)
        for face in faces:
            face.normal_update()
            if abs(face.normal.dot(direction)) > 0.9:
                face.smooth = False
                continue
            local = [inverse @ loop.vert.co for loop in face.loops]
            centre = math.atan2(sum(p.y for p in local), sum(p.x for p in local))
            for loop, p in zip(face.loops, local):
                angle = math.atan2(p.y, p.x)
                angle = centre + math.atan2(math.sin(angle - centre), math.cos(angle - centre))
                loop[self.uv].uv = (angle * radius / tile, p.z / tile)
            face[self.mapped] = 1
        return faces

    def box(self, centre, size, material, rotation=Matrix.Identity(4)):
        result = bmesh.ops.create_cube(self.bm, size=1.0)
        matrix = Matrix.Translation(centre) @ rotation @ Matrix.Diagonal((size[0], size[1], size[2], 1.0))
        return self._finish(result['verts'] + list({f for v in result['verts'] for f in v.link_faces}), material, matrix, False)

    def torus(self, centre, major, minor, material, major_segments=160, minor_segments=24, matrix=Matrix.Identity(4)):
        verts = []
        for i in range(major_segments):
            a = i / major_segments * 2 * math.pi
            ring = []
            for j in range(minor_segments):
                b = j / minor_segments * 2 * math.pi
                r = major + minor * math.cos(b)
                ring.append(self.bm.verts.new((r * math.cos(a), r * math.sin(a), minor * math.sin(b))))
            verts.append(ring)
        faces = []
        tile = self.tile(material)
        u_step = 2 * math.pi * major / major_segments / tile
        v_step = 2 * math.pi * minor / minor_segments / tile
        for i in range(major_segments):
            for j in range(minor_segments):
                a, b = verts[i][j], verts[(i + 1) % major_segments][j]
                c, d = verts[(i + 1) % major_segments][(j + 1) % minor_segments], verts[i][(j + 1) % minor_segments]
                face = self.bm.faces.new((a, b, c, d))
                for loop, (di, dj) in zip(face.loops, ((0, 0), (1, 0), (1, 1), (0, 1))):
                    loop[self.uv].uv = ((i + di) * u_step, (j + dj) * v_step)
                face[self.mapped] = 1
                faces.append(face)
        flat = [v for ring in verts for v in ring]
        bmesh.ops.transform(self.bm, matrix=Matrix.Translation(-self.origin) @ Matrix.Translation(centre) @ matrix, verts=flat)
        index = self.slot(material)
        for face in faces:
            face.material_index = index
            face.smooth = True
        return faces

    def build(self, parent=None):
        uv = self.uv
        tiles = [material['tile'] for material in self.materials]
        for face in self.bm.faces:
            if face[self.mapped]:
                continue
            face.normal_update()
            n = face.normal
            axis = max(range(3), key=lambda k: abs(n[k]))
            a, b = [(1, 2), (0, 2), (0, 1)][axis]
            tile = tiles[face.material_index]
            for loop in face.loops:
                co = loop.vert.co + self.origin
                loop[uv].uv = (co[a] / tile, co[b] / tile)
        mesh = bpy.data.meshes.new(self.name)
        self.bm.to_mesh(mesh)
        self.bm.free()
        for material in self.materials:
            mesh.materials.append(material)
        obj = bpy.data.objects.new(self.name, mesh)
        obj.location = self.origin
        bpy.context.scene.collection.objects.link(obj)
        if parent is not None:
            obj.parent = parent
            obj.location = self.origin - parent.location
        return obj


def empty(name, location, parent=None):
    obj = bpy.data.objects.new(name, None)
    obj.location = location
    bpy.context.scene.collection.objects.link(obj)
    if parent is not None:
        obj.parent = parent
        obj.location = location - parent.location
    return obj


def main():
    os.makedirs(OUT, exist_ok=True)
    bpy.ops.wm.read_factory_settings(use_empty=True)
    hull = pbr_material('hull', hull_maps(), 8.0)
    gold = pbr_material('foil-gold', foil_maps(gold=True), 2.0)
    silver = pbr_material('foil-silver', foil_maps(gold=False), 2.0)
    metal = pbr_material('metal', metal_maps(), 2.0)
    solar = pbr_material('solar', solar_maps(), 2.0)
    radiator = pbr_material('radiator', radiator_maps(), 2.0)
    glass = np.full((64, 256, 3), 0.03)
    flat = np.broadcast_to(np.array([0.5, 0.5, 1.0]), (64, 256, 3)).copy()
    windows = pbr_material('window', (glass, flat, 0.08, 0.0), 1.0, emission=window_maps())
    materials = [hull, gold, silver, metal, solar, radiator, windows]

    root = empty('station', Vector((0, 0, 0)))

    # The core: spine, hub, modules, nose, radiators, dish ------------------------------------
    core = Part('core', materials)
    core.cylinder(T(0, 0, 0), 'y', 2.2, 78, hull, 32)
    for y in np.arange(-36, 37, 6):
        core.cylinder(T(0, y, 0), 'y', 2.55, 0.5, metal, 32)
    for k in range(3):
        a = k / 3 * 2 * math.pi + 0.4
        core.cylinder(T(math.cos(a) * 2.55, 0, math.sin(a) * 2.55), 'y', 0.22, 70, metal, 8)
    # The hub: a drum of panels, a gold waist, two rows of windows, ribbed end caps.
    core.cylinder(T(0, 0, 0), 'y', 6.6, 12, hull, 48)
    core.cylinder(T(0, 0, 0), 'y', 6.85, 3.2, gold, 48)
    for y in (-4.2, 4.2):
        for k in range(24):
            a = k / 24 * 2 * math.pi
            core.box(T(math.cos(a) * 6.62, y, math.sin(a) * 6.62), (0.08, 0.9, 0.5), windows,
                     Matrix.Rotation(-a, 4, 'Z'))
    for y in (-6.3, 6.3):
        core.cylinder(T(0, y, 0), 'y', 7.1, 0.6, metal, 48)
        for k in range(12):
            a = k / 12 * 2 * math.pi
            core.box(T(math.cos(a) * 4.6, y + (0.5 if y > 0 else -0.5), math.sin(a) * 4.6), (4.2, 0.35, 0.35), metal, Matrix.Rotation(-a, 4, 'Z'))
    # Two pressurised modules wrapped in insulation, each with a cupola and tanks.
    for y, foil in ((-19, gold), (19, silver)):
        core.cylinder(T(0, y, 0), 'y', 4.6, 9.5, foil, 40)
        for end in (-1, 1):
            core.cylinder(T(0, y + end * 5.35, 0), 'y', 4.6, 1.2, hull, 40, radius2=3.4 if end > 0 else 4.6)
            core.cylinder(T(0, y + end * 4.75, 0), 'y', 4.75, 0.35, metal, 40)
        for k in range(4):
            a = k / 4 * 2 * math.pi + 0.3
            core.cylinder(T(math.cos(a) * 5.4, y + 1.5, math.sin(a) * 5.4), 'y', 0.9, 5.5, hull, 16)
        core.cylinder(T(4.9, y - 2.0, 0), 'x', 1.5, 1.6, hull, 20)
        core.cylinder(T(5.9, y - 2.0, 0), 'x', 1.1, 0.8, windows, 20, radius2=0.8)
    # Window bands.
    for y in (-25, 25):
        core.cylinder(T(0, y, 0), 'y', 5.3, 3.0, hull, 48)
        for k in range(20):
            a = k / 20 * 2 * math.pi
            core.box(T(math.cos(a) * 5.32, y, math.sin(a) * 5.32), (0.08, 1.1, 1.4), windows, Matrix.Rotation(-a, 4, 'Z'))
    # Nose: a docking adapter and port.
    core.cylinder(T(0, 36.5, 0), 'y', 3.4, 5, hull, 32, radius2=2.4)
    core.cylinder(T(0, 40.2, 0), 'y', 1.8, 2.4, metal, 24)
    core.cylinder(T(0, 41.8, 0), 'y', 2.2, 0.8, hull, 24)
    core.cylinder(T(0, 42.6, 0), 'y', 0.25, 1.8, metal, 8)
    # Radiators: ribbed white panels spread out either side on a boom, edge-on to the sun.
    for side in (-1, 1):
        core.box(T(side * 5.0, 10, 0), S(5.2, 0.45, 0.45), metal)
        for k in range(3):
            core.box(T(side * (8.6 + k * 5.9), 10, 0), S(5.6, 7.2, 0.12), radiator)
            core.box(T(side * (8.6 + k * 5.9 + 2.9), 10, 0), S(0.3, 7.4, 0.3), metal)
        for y in (6.3, 13.7):
            core.box(T(side * 14.5, y, 0), S(17.8, 0.25, 0.25), metal)
    # A high-gain dish on a boom, opening outwards (+z), with a feed on three struts.
    core.cylinder(T(0, 12, 4.5), 'z', 0.35, 5, metal, 12)
    for ring, (rim, inner, depth) in enumerate([(5.4, 4.2, 0.8), (4.2, 2.6, 0.6), (2.6, 0.6, 0.4)]):
        core.cylinder(T(0, 12, 8.6 - ring * 0.6), 'z', inner, depth, gold if ring == 0 else hull, 40, radius2=rim, caps=False)
    for k in range(3):
        a = k / 3 * 2 * math.pi
        core.cylinder(T(math.cos(a) * 2.2, 12 + math.sin(a) * 2.2, 10.2), 'z', 0.08, 3.6, metal, 6)
    core.cylinder(T(0, 12, 11.8), 'z', 0.45, 0.8, metal, 12)
    # Antennas and small boxes scattered on the modules and spine.
    for _ in range(40):
        y = RNG.uniform(-34, 34)
        a = RNG.uniform(0, 2 * math.pi)
        r = 2.4 if abs(y) < 14 or abs(abs(y) - 19) > 6 else 4.7
        size = RNG.uniform(0.4, 1.2)
        core.box(T(math.cos(a) * (r + size / 2), y, math.sin(a) * (r + size / 2)), (size, size * RNG.uniform(0.6, 1.4), size * 0.8), hull if RNG.random() < 0.6 else metal, Matrix.Rotation(-a, 4, 'Z'))
    for _ in range(8):
        y = RNG.uniform(-30, 30)
        a = RNG.uniform(0, 2 * math.pi)
        length = RNG.uniform(2, 5)
        core.cylinder(T(math.cos(a) * (2.4 + length / 2), y, math.sin(a) * (2.4 + length / 2)), Matrix.Rotation(-a, 4, 'Z') @ AXES['x'], 0.05, length, metal, 6)
    core.build(root)

    # The habitat ring: 24 segmented modules on six spokes -------------------------------------
    habitat = Part('habitat', materials)
    segments = 24
    ring_matrix = Matrix.Identity(4)
    habitat.torus(T(0, 0, 0), 30, 2.55, hull, 192, 28, ring_matrix)
    for k in range(segments):
        a = k / segments * 2 * math.pi
        # A collar where two modules join.
        tangent = Matrix.Rotation(a, 4, 'Z') @ Matrix.Rotation(math.pi / 2, 4, 'X')
        habitat.cylinder(Vector((math.cos(a) * 30, math.sin(a) * 30, 0)), tangent, 2.95, 0.7, metal, 32)
        # Insulation on alternate modules' inner faces.
        mid = a + math.pi / segments
        if k % 3 == 0:
            habitat.box(Vector((math.cos(mid) * 27.7, math.sin(mid) * 27.7, 0)), (0.3, 4.2, 3.2), gold, Matrix.Rotation(mid, 4, 'Z'))
        # Two rows of windows on the outer face, above and below the equator.
        for row in (-1, 1):
            for w in range(6):
                b = a + (w + 1.5) / (segments * 9) * 2 * math.pi * 1.0
                x, y = math.cos(b), math.sin(b)
                radial = 30 + 2.55 * math.cos(math.radians(38))
                habitat.box(Vector((x * radial, y * radial, row * 2.55 * math.sin(math.radians(38)))), (0.08, 0.7, 0.5), windows,
                            Matrix.Rotation(b, 4, 'Z') @ Matrix.Rotation(-row * math.radians(38), 4, 'Y'))
    for k in range(6):
        a = k / 6 * 2 * math.pi
        spoke = Matrix.Rotation(a, 4, 'Z') @ AXES['x']
        habitat.cylinder(Vector((math.cos(a) * 18.5, math.sin(a) * 18.5, 0)), spoke, 0.85, 23, hull, 16)
        for offset in (-1, 1):
            side = Vector((-math.sin(a), math.cos(a), 0)) * 1.3 * offset
            habitat.cylinder(Vector((math.cos(a) * 18.5, math.sin(a) * 18.5, 0)) + side, spoke, 0.18, 23, metal, 8)
        habitat.cylinder(Vector((math.cos(a) * 27.0, math.sin(a) * 27.0, 0)), spoke, 1.6, 2.4, metal, 20)
    habitat.build(root)

    # The docking ring near the nose, with eight ports ------------------------------------------
    dock_origin = T(0, 28, 0)
    dock = Part('dock', materials, dock_origin)
    dock.torus(dock_origin, 11, 1.05, hull, 128, 20)
    for k in range(8):
        a = k / 8 * 2 * math.pi
        port = Matrix.Rotation(a, 4, 'Z') @ AXES['x']
        centre = dock_origin + Vector((math.cos(a) * 12.4, math.sin(a) * 12.4, 0))
        dock.cylinder(centre, port, 0.9, 1.8, metal, 20)
        dock.cylinder(dock_origin + Vector((math.cos(a) * 13.5, math.sin(a) * 13.5, 0)), port, 1.1, 0.4, hull, 20)
    for k in range(4):
        a = k / 4 * 2 * math.pi + math.pi / 4
        dock.cylinder(dock_origin + Vector((math.cos(a) * 6.5, math.sin(a) * 6.5, 0)), Matrix.Rotation(a, 4, 'Z') @ AXES['x'], 0.3, 9, metal, 8)
    dock.build(root)

    # The solar truss and its four wings ---------------------------------------------------------
    arrays_origin = T(0, -31, 0)
    arrays = empty('arrays', arrays_origin, root)
    truss = Part('truss', materials, arrays_origin)
    half = 52
    for sx in (-0.6, 0.6):
        for sz in (-0.6, 0.6):
            truss.cylinder(arrays_origin + T(0, sx, sz) - T(0, 0, 0), 'x', 0.09, half * 2, metal, 6)
    for x in np.arange(-half, half + 0.01, 2.0):
        c = arrays_origin + T(x, 0, 0)
        truss.box(c, (0.12, 1.3, 0.12), metal)
        truss.box(c, (0.12, 0.12, 1.3), metal)
        if x < half:
            diag = Matrix.Rotation(math.atan2(1.2, 2.0), 4, 'Y')
            truss.box(arrays_origin + T(x + 1, 0, 0.6), (2.35, 0.08, 0.08), metal, diag)
            truss.box(arrays_origin + T(x + 1, 0, -0.6), (2.35, 0.08, 0.08), metal, diag.inverted())
    for side in (-1, 1):
        truss.cylinder(arrays_origin + T(side * 4.5, 0, 0), 'x', 1.1, 2.4, metal, 20)
    truss.build(arrays)

    wings_origin = arrays_origin
    wings = Part('wings', materials, wings_origin)
    for side in (-1, 1):
        for offset in (0, 1):
            cx = side * (17 + offset * 25)
            for blanket in (-1, 1):
                wings.box(wings_origin + T(cx, 0, blanket * 3.25), S(21.4, 0.06, 5.4), solar)
            wings.box(wings_origin + T(cx, 0, 0), S(21.6, 0.3, 0.5), metal)
            for edge in (-1, 1):
                wings.box(wings_origin + T(cx + edge * 10.8, 0, 0), S(0.25, 0.3, 12.2), metal)
                wings.box(wings_origin + T(cx, 0, edge * 6.05), S(21.6, 0.2, 0.15), metal)
    wings.build(arrays)

    bpy.ops.object.select_all(action='SELECT')
    # Textures as separate files, not embedded: embedded images are loaded through blob: URLs,
    # which the scene's sandbox (connect-src 'self') does not allow.
    bpy.ops.export_scene.gltf(
        filepath=os.path.join(OUT, 'station.gltf'),
        export_format='GLTF_SEPARATE',
        export_texture_dir='station-textures',
        export_image_format='WEBP',
        export_image_quality=85,
        export_apply=True,
    )
    triangles = 0
    for obj in bpy.data.objects:
        if obj.type == 'MESH':
            obj.data.calc_loop_triangles()
            triangles += len(obj.data.loop_triangles)
    print('station: exported', triangles, 'triangles')


main()
