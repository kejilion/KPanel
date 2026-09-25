"""Sculpts the sea stacks of the sea-and-sky scene and bakes them for the web.

For every rock in ../src/rocks.json (the same list the scene places them from)
this builds a finely detailed stack, a few hundred thousand faces, with the
features of a real sandstone stack: lumpy mass, stepped layers that weather
back at different rates, vertical joints, a wave-cut notch at the waterline,
fine erosion; and colours it (layered sandstone, dark weed and wet rock where
the waves wash, salt on the ledges). A light mesh of the same shape carries
all of that as baked textures: colour, an object-space normal map and ambient
occlusion. The light meshes are exported as one glTF binary; the baked maps are
written as raw float arrays (.npy, linear, top row first) for pack_textures.py
to encode as WebP, so no colour management touches the data maps.

    blender -b --factory-startup --python blender/rocks.py -- <out dir>

Everything is seeded from the rock list, so a rerun gives the same stacks.
"""
import json
import math
import os
import sys

import bpy
import numpy as np

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.abspath(sys.argv[sys.argv.index('--') + 1]) if '--' in sys.argv else os.path.join(HERE, 'out')
ROCKS = json.load(open(os.path.join(HERE, '..', 'src', 'rocks.json'), encoding='utf-8'))


# Noise ---------------------------------------------------------------------------------------

def hash3(ix, iy, iz, seed):
    h = (ix.astype(np.uint64) * np.uint64(73856093)) ^ (iy.astype(np.uint64) * np.uint64(19349663)) \
        ^ (iz.astype(np.uint64) * np.uint64(83492791)) ^ np.uint64(int(seed * 1000) & 0xffffffff)
    h = (h ^ (h >> np.uint64(13))) * np.uint64(1274126177)
    h = h ^ (h >> np.uint64(16))
    return (h & np.uint64(0xffff)).astype(np.float64) / 65535.0


def value_noise(p, seed):
    """Smooth value noise in [-1, 1] at points p (N x 3)."""
    base = np.floor(p)
    f = p - base
    i = base.astype(np.int64)
    u = f * f * (3.0 - 2.0 * f)
    out = 0.0
    for dx in (0, 1):
        for dy in (0, 1):
            for dz in (0, 1):
                w = (u[:, 0] if dx else 1 - u[:, 0]) * (u[:, 1] if dy else 1 - u[:, 1]) * (u[:, 2] if dz else 1 - u[:, 2])
                out = out + w * hash3(i[:, 0] + dx, i[:, 1] + dy, i[:, 2] + dz, seed)
    return out * 2.0 - 1.0


def fbm(p, octaves, seed, gain=0.5, lacunarity=2.03):
    total = np.zeros(len(p))
    amplitude = 0.5
    for octave in range(octaves):
        total += amplitude * value_noise(p, seed + octave * 17.0)
        p = p * lacunarity + np.array([3.1, 7.7, 1.3])
        amplitude *= gain
    return total


def ridged(p, octaves, seed):
    """Sharp creases where the noise crosses zero: joints and cracks, in [0, 1]."""
    total = np.zeros(len(p))
    amplitude = 0.6
    for octave in range(octaves):
        total += amplitude * (1.0 - np.abs(value_noise(p, seed + octave * 31.0)))
        p = p * 2.1 + np.array([5.3, 1.9, 8.2])
        amplitude *= 0.45
    return np.clip(total, 0.0, 1.0)


def smoothstep(a, b, x):
    t = np.clip((x - a) / (b - a), 0.0, 1.0)
    return t * t * (3.0 - 2.0 * t)


# Shape ---------------------------------------------------------------------------------------

def outline(rock, rows):
    """The stack's outline, top centre to under water: (radius factor, height, outline position s).

    A sea stack stands on steep walls, a little wider at the foot, with a rounded edge and a gently
    domed top; a low reef is a broad, low slab of rock the sea has worn flat-ish, awash at its edges.
    """
    height = rock['height']
    s = np.linspace(0.0, 1.0, rows)
    if height <= 6:
        phi = s * math.acos(-0.3)
        up = np.cos(phi)
        # A flattened top falling away in rough shoulders, rather than a dome.
        return np.sin(phi) ** 0.7, np.where(up > 0, up ** 0.45, up) * height * 0.85, s
    # Each stack its own: some broad and flat-topped, some tapering to a narrower crown.
    rng = np.random.default_rng(int(rock['seed'] * 7))
    crown, taper = 0.5 + 0.3 * rng.random(), 0.1 + 0.3 * rng.random()
    top, edge = 0.14, 0.26
    across = np.empty_like(s)
    z = np.empty_like(s)
    on_top = s < top
    t = s[on_top] / top
    across[on_top] = crown * t
    z[on_top] = height * (0.96 + 0.04 * (1.0 - t * t))
    on_edge = (s >= top) & (s < edge)
    angle = (s[on_edge] - top) / (edge - top) * math.pi / 2
    across[on_edge] = crown + 0.12 * np.sin(angle)
    z[on_edge] = height * (0.84 + 0.12 * np.cos(angle))
    on_wall = s >= edge
    t = (s[on_wall] - edge) / (1.0 - edge)
    across[on_wall] = crown + 0.12 + taper * t ** 1.3
    z[on_wall] = height * (0.84 - 1.04 * t)
    return across, z, s


def stack_surface(rock, rows, columns, detail):
    """Points of a stack on a (row, column) grid, from the top centre down to under water.

    Returns positions (rows x (columns+1) x 3, Blender axes: z up), uv and the fields the colours
    follow. The last column repeats the first, so the texture has a clean seam. detail=False keeps
    only what the light mesh should carry in its silhouette (the mass, the big bites and layers);
    the rest goes into the maps.
    """
    radius, height, seed = rock['radius'], rock['height'], rock['seed']
    stack = height > 6
    across, z, along = outline(rock, rows)
    across = across[:, None] * np.ones((1, columns + 1))
    z = z[:, None] * np.ones((1, columns + 1))
    along = along[:, None] * np.ones((1, columns + 1))
    theta = np.linspace(0.0, 2 * math.pi, columns + 1)[None, :] * np.ones((rows, 1))
    x0 = np.cos(theta) * across * radius
    y0 = np.sin(theta) * across * radius
    flat = np.stack([x0.ravel(), y0.ravel(), z.ravel()], axis=1)
    count = len(flat)
    zeros = np.zeros(count)

    # The mass: big lumps, and the stack leans a little, so no two are alike.
    lumps = 1.0 + (0.22 if stack else 0.3) * fbm(flat / (radius * 0.8), 4, seed)
    rng = np.random.default_rng(int(seed * 1000))
    lean = rng.normal(0.0, 0.06, 2) if stack else np.zeros(2)
    inset = zeros.copy()
    joints = zeros.copy()
    within = zeros.copy()
    hardness = zeros.copy()
    top_rows = along.ravel() < 0.26
    if stack:
        # Big bites out of the walls, where blocks have fallen away.
        wall = smoothstep(0.1, 0.4, along.ravel())
        inset += 0.3 * radius * smoothstep(0.35, 0.75, fbm(flat / (radius * 1.1), 3, seed + 3) * 0.5 + 0.5) * wall
        # Layers of sandstone from half a metre to four metres thick, warped a little; each weathers
        # back at its own rate in places, the soft top of a layer cut back under the harder one above.
        thickness = 0.5 + 3.5 * rng.random(200) ** 1.6
        bounds = np.concatenate([[-0.3 * height], -0.3 * height + np.cumsum(thickness)])
        warped = flat[:, 2] + 1.2 * fbm(flat / 18.0, 3, seed + 5)
        index = np.clip(np.searchsorted(bounds, warped) - 1, 0, len(thickness) - 1)
        within = (warped - bounds[index]) / thickness[index]
        hardness = hash3(index.astype(np.int64), np.zeros(count, np.int64), np.zeros(count, np.int64), seed)
        patchy = smoothstep(0.35, 0.7, fbm(flat / 7.0, 3, seed + 7) * 0.5 + 0.5)
        inset += (0.04 * radius * smoothstep(0.55, 0.98, within) + 0.03 * radius * hardness) * patchy * wall
        # Big vertical joints split the walls into blocks and columns.
        tall = flat * np.array([1.0 / 9.0, 1.0 / 9.0, 1.0 / 60.0])
        inset += 0.07 * radius * smoothstep(0.8, 0.97, ridged(tall, 2, seed + 13)) * wall
        # The crown is broken, not flat: on some stacks a shattered peak, on others a jagged edge.
        broken = 0.08 + 0.2 * rng.random()
        crown = broken * height * fbm(flat / (radius * 0.7), 3, seed + 17) * (1.0 - wall)
    else:
        crown = zeros
    if not stack:
        # Reefs: broken into blocks and gullies, so the silhouette is ragged, not round.
        inset += 0.22 * radius * (fbm(flat / (radius * 0.35), 3, seed + 19) * 0.5 + 0.5)
        crown = crown + 0.35 * height * fbm(flat / (radius * 0.45), 3, seed + 21) * np.maximum(flat[:, 2], 0.0) / height
    # A notch cut by the waves at the waterline.
    inset += 0.07 * radius * np.exp(-((flat[:, 2] - 0.6) / 1.3) ** 2)
    if detail:
        # Fine joints, widened by the weather, and erosion over everything.
        stretched = flat * np.array([1.0 / 5.0, 1.0 / 5.0, 1.0 / 26.0])
        joints = smoothstep(0.78, 0.97, ridged(stretched, 3, seed + 11)) * (1.0 if stack else 0.4)
        inset += 0.05 * radius * joints
        inset -= 0.018 * radius * fbm(flat / 1.1, 4, seed + 23) + 0.006 * radius * fbm(flat / 0.3, 3, seed + 29)
    reach = across.ravel() * radius
    scale = np.where(reach > 1e-6, np.maximum(lumps * reach - inset, 0.02 * radius) / np.maximum(reach, 1e-6), 1.0)
    height_here = flat[:, 2] + crown
    points = np.stack([
        x0.ravel() * scale + lean[0] * np.maximum(height_here, 0.0),
        y0.ravel() * scale + lean[1] * np.maximum(height_here, 0.0),
        height_here,
    ], axis=1)
    uv = np.stack([theta.ravel() / (2 * math.pi), 1.0 - along.ravel()], axis=1)
    shape = (rows, columns + 1)
    return points.reshape(rows, columns + 1, 3), uv.reshape(rows, columns + 1, 2), joints.reshape(shape), within.reshape(shape), hardness.reshape(shape), top_rows.reshape(shape)


def grid_normals(points):
    du = np.roll(points, -1, axis=1) - np.roll(points, 1, axis=1)
    dv = np.roll(points, -1, axis=0) - np.roll(points, 1, axis=0)
    normals = np.cross(dv, du)  # outward: down the stack, then round it
    length = np.linalg.norm(normals, axis=2, keepdims=True)
    return normals / np.maximum(length, 1e-9)


def stack_colours(rock, points, joints, within, hardness, top_rows):
    """Linear albedo: layered sandstone, dark in the joints and under the ledges, dark and weedy where
    the waves wash, pale with salt on the ledge tops."""
    seed = rock['seed']
    flat = points.reshape(-1, 3)
    n = grid_normals(points).reshape(-1, 3)
    grain = fbm(flat / 0.6, 3, seed + 41) * 0.5 + 0.5
    patches = fbm(flat / 7.0, 3, seed + 43) * 0.5 + 0.5
    light = np.array([0.46, 0.35, 0.24])
    dark = np.array([0.22, 0.16, 0.115])
    # The layers show in the colour, but faintly: most of the variation is in patches and grain.
    mix = np.clip(0.2 + 0.22 * hardness.ravel() + 0.4 * patches + 0.18 * grain, 0, 1)[:, None]
    colour = dark * (1 - mix) + light * mix
    colour *= (1.0 - 0.45 * joints.ravel())[:, None]
    colour *= (1.0 - 0.3 * smoothstep(0.7, 1.0, within.ravel()))[:, None]
    # Dark streaks where rain runs down from the ledges, and grey-green lichen here and there.
    streaks = smoothstep(0.2, 0.8, fbm(flat * np.array([1 / 1.4, 1 / 1.4, 1 / 14.0]), 3, seed + 53) * 0.5 + 0.5)
    colour *= (1.0 - 0.28 * streaks)[:, None]
    # Rusty stains washed down from iron-rich layers.
    rust = smoothstep(0.55, 0.85, fbm(flat * np.array([1 / 3.0, 1 / 3.0, 1 / 22.0]), 3, seed + 57) * 0.5 + 0.5)
    colour = colour * (1 - 0.35 * rust[:, None]) + np.array([0.4, 0.22, 0.11]) * 0.35 * rust[:, None]
    lichen = smoothstep(0.62, 0.8, fbm(flat / 2.2, 3, seed + 59) * 0.5 + 0.5) * (flat[:, 2] > 4.0)
    colour = colour * (1 - 0.4 * lichen[:, None]) + np.array([0.3, 0.32, 0.24]) * 0.4 * lichen[:, None]
    # The tide zone: weed and wet rock up to a ragged line a metre or so above the water, then a
    # splash zone of greyer, barnacled rock fading out above it; no hard edge anywhere.
    ragged = 0.9 * fbm(flat / 2.2, 3, seed + 47) + 0.35 * fbm(flat / 0.6, 2, seed + 49)
    # Low reefs are awash at every tide: wet and weedy almost to the top.
    wet = 1.0 - smoothstep(-0.2, 1.3 if rock['height'] > 6 else 2.6, flat[:, 2] + ragged)
    weed = np.array([0.07, 0.085, 0.05])
    colour = colour * (1 - 0.7 * wet[:, None]) + weed * 0.7 * wet[:, None]
    splash = (1.0 - smoothstep(1.0, 4.0, flat[:, 2] + 1.5 * ragged)) * (1.0 - wet)
    colour = colour * (1 - 0.3 * splash[:, None]) + np.array([0.3, 0.3, 0.28]) * 0.3 * splash[:, None]
    # Salt and guano in patches on the ledge tops, not whole rings.
    patchy = smoothstep(0.55, 0.8, fbm(flat / 1.6, 3, seed + 51) * 0.5 + 0.5)
    salt = smoothstep(0.8, 0.97, n[:, 2]) * (flat[:, 2] > 2.5) * patchy
    colour = colour * (1 - 0.3 * salt[:, None]) + np.array([0.72, 0.7, 0.64]) * 0.3 * salt[:, None]
    if rock['height'] > 6:
        # Wiry grass and thrift on the stack tops, where the gulls leave it alone.
        tuft = smoothstep(0.35, 0.65, fbm(flat / 1.8, 3, seed + 61) * 0.5 + 0.5) * top_rows.ravel() * smoothstep(0.6, 0.9, n[:, 2])
        colour = colour * (1 - 0.75 * tuft[:, None]) + np.array([0.14, 0.17, 0.07]) * 0.75 * tuft[:, None]
    return np.clip(colour, 0, 1)


def build_mesh(name, points, uv=None, colours=None):
    rows, columns1 = points.shape[:2]
    verts = points.reshape(-1, 3)
    faces = []
    for r in range(rows - 1):
        a = r * columns1 + np.arange(columns1 - 1)
        faces.append(np.stack([a, a + columns1, a + columns1 + 1, a + 1], axis=1))
    faces = np.concatenate(faces)
    mesh = bpy.data.meshes.new(name)
    mesh.vertices.add(len(verts))
    mesh.vertices.foreach_set('co', verts.astype(np.float32).ravel())
    mesh.loops.add(len(faces) * 4)
    mesh.loops.foreach_set('vertex_index', faces.astype(np.int32).ravel())
    mesh.polygons.add(len(faces))
    mesh.polygons.foreach_set('loop_start', np.arange(0, len(faces) * 4, 4, dtype=np.int32))
    mesh.polygons.foreach_set('loop_total', np.full(len(faces), 4, dtype=np.int32))
    if uv is not None:
        layer = mesh.uv_layers.new(name='UVMap')
        layer.data.foreach_set('uv', uv.reshape(-1, 2)[faces.ravel()].astype(np.float32).ravel())
    if colours is not None:
        attribute = mesh.color_attributes.new('Col', 'FLOAT_COLOR', 'POINT')
        rgba = np.concatenate([colours, np.ones((len(colours), 1))], axis=1)
        attribute.data.foreach_set('color', rgba.astype(np.float32).ravel())
    mesh.update()
    mesh.validate()
    mesh.polygons.foreach_set('use_smooth', np.ones(len(mesh.polygons), dtype=bool))
    obj = bpy.data.objects.new(name, mesh)
    bpy.context.scene.collection.objects.link(obj)
    return obj


# Baking --------------------------------------------------------------------------------------

def use_gpu():
    prefs = bpy.context.preferences.addons['cycles'].preferences
    for kind in ('OPTIX', 'CUDA'):
        try:
            prefs.compute_device_type = kind
            prefs.get_devices()
            if any(device.type == kind for device in prefs.devices):
                for device in prefs.devices:
                    device.use = device.type == kind
                bpy.context.scene.cycles.device = 'GPU'
                return kind
        except TypeError:
            continue
    return 'CPU'


def vertex_colour_material():
    material = bpy.data.materials.new('rock-high')
    material.use_nodes = True
    nodes = material.node_tree.nodes
    attribute = nodes.new('ShaderNodeAttribute')
    attribute.attribute_name = 'Col'
    material.node_tree.links.new(attribute.outputs['Color'], nodes['Principled BSDF'].inputs['Base Color'])
    return material


def bake_target_material(image):
    material = bpy.data.materials.new('rock-low-' + image.name)
    material.use_nodes = True
    node = material.node_tree.nodes.new('ShaderNodeTexImage')
    node.image = image
    material.node_tree.nodes.active = node
    return material


def bake(low, high, kind, image, samples, extrusion, **settings):
    scene = bpy.context.scene
    scene.cycles.samples = samples
    low.data.materials.clear()
    low.data.materials.append(bake_target_material(image))
    bpy.ops.object.select_all(action='DESELECT')
    high.select_set(True)
    low.select_set(True)
    bpy.context.view_layer.objects.active = low
    bake = scene.render.bake
    bake.use_selected_to_active = True
    bake.cage_extrusion = extrusion
    bake.max_ray_distance = extrusion * 2.5
    bake.margin = 8
    for key, value in settings.items():
        setattr(bake, key, value)
    bpy.ops.object.bake(type=kind)


def save_pixels(image, path):
    size = image.size[0]
    pixels = np.empty(size * size * 4, dtype=np.float32)
    image.pixels.foreach_get(pixels)
    # Blender's first row is the bottom one; image files start at the top.
    np.save(path, pixels.reshape(size, size, 4)[::-1].copy())


def main():
    os.makedirs(OUT, exist_ok=True)
    bpy.ops.wm.read_factory_settings(use_empty=True)
    scene = bpy.context.scene
    scene.render.engine = 'CYCLES'
    device = use_gpu()
    print('rocks: baking on', device)
    high_material = vertex_colour_material()
    exported = []
    for index, rock in enumerate(ROCKS):
        big = rock['height'] > 10
        size = 2048 if rock['height'] > 20 else 1024 if big else 512
        # Rows and columns in proportion to the stack's size; the fine mesh a few centimetres apart.
        high_rows, high_columns = (900, 1100) if rock['height'] > 20 else (600, 760) if big else (260, 380)
        low_rows, low_columns = (110, 170) if rock['height'] > 20 else (80, 120) if big else (32, 56)
        points, _, joints, within, hardness, top_rows = stack_surface(rock, high_rows, high_columns, True)
        colours = stack_colours(rock, points, joints, within, hardness, top_rows)
        high = build_mesh(f'high-{index}', points, colours=colours)
        high.data.materials.append(high_material)
        low_points, uv, *_ = stack_surface(rock, low_rows, low_columns, False)
        low = build_mesh(f'rock-{index}', low_points, uv=uv)
        extrusion = 0.12 * rock['radius'] + 0.3

        # Every map is baked into a float, non-colour image, so its pixels are the raw linear values.
        maps = {}
        for kind in ('colour', 'normal', 'occlusion'):
            maps[kind] = bpy.data.images.new(f'rock-{index}-{kind}', size, size, float_buffer=True)
            maps[kind].colorspace_settings.name = 'Non-Color'
        bake(low, high, 'DIFFUSE', maps['colour'], 1, extrusion, use_pass_direct=False, use_pass_indirect=False, use_pass_color=True)
        bake(low, high, 'NORMAL', maps['normal'], 1, extrusion, normal_space='OBJECT')
        bake(low, high, 'AO', maps['occlusion'], 48 if big else 24, extrusion)
        for kind, image in maps.items():
            save_pixels(image, os.path.join(OUT, f'rock-{index}-{kind}.npy'))

        # The high mesh is only needed for its own bake.
        bpy.data.objects.remove(high, do_unlink=True)
        low.data.materials.clear()
        # Placed where the scene has it: three.js x = Blender x, three.js z = -Blender y.
        low.location = (rock['x'], -rock['z'], 0.0)
        exported.append(low)
        print(f'rocks: {index} baked ({size}px, {high_rows * high_columns} fine points)')

    bpy.ops.object.select_all(action='DESELECT')
    for obj in exported:
        obj.select_set(True)
    # Draco-compressed, and without vertex normals: the shading comes from the baked normal map.
    bpy.ops.export_scene.gltf(filepath=os.path.join(OUT, 'rocks.glb'), export_format='GLB', use_selection=True,
                              export_materials='NONE', export_normals=False,
                              export_draco_mesh_compression_enable=True, export_draco_mesh_compression_level=7,
                              export_draco_position_quantization=14, export_draco_texcoord_quantization=14)
    print('rocks: exported', len(exported))


main()
