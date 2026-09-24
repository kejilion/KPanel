"""Build the editable sanctuary, portable GLB, and three real Blender frames.

Blender: Z up; glTF / cameras.json: Y up. All lengths are metres.
Usage: blender --background --python build_scene.py -- --render 0 1 2
"""
import argparse
import json
import math
import random
import sys
from pathlib import Path

import bpy
from mathutils import Vector

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT / 'blender'))
from materials import create_materials

parser = argparse.ArgumentParser()
parser.add_argument('--render', nargs='*', type=int, default=[])
parser.add_argument('--width', type=int, default=1280)
parser.add_argument('--samples', type=int, default=64)
args = parser.parse_args(sys.argv[sys.argv.index('--') + 1:] if '--' in sys.argv else [])
rng = random.Random(91427)
bpy.ops.object.select_all(action='SELECT')
bpy.ops.object.delete(use_global=False)
M = create_materials(ROOT / 'assets' / 'textures')
asset_objects = []


def finish(obj, name, mat, bevel=0.0, smooth=False):
    obj.name = name
    obj.data.materials.append(M[mat])
    if bevel:
        modifier = obj.modifiers.new('Eroded stone edges', 'BEVEL')
        modifier.width = bevel
        modifier.segments = 2
        bpy.context.view_layer.objects.active = obj
        bpy.ops.object.modifier_apply(modifier=modifier.name)
    if smooth:
        for polygon in obj.data.polygons:
            polygon.use_smooth = True
    asset_objects.append(obj)
    return obj


def planar_uv(obj, repeat=3.5):
    """World-scale box projection, with per-face axes to preserve grain scale."""
    uv = obj.data.uv_layers.active or obj.data.uv_layers.new(name='UVMap')
    for polygon in obj.data.polygons:
        axis = max(range(3), key=lambda index: abs(polygon.normal[index]))
        axes = ((1, 2), (0, 2), (0, 1))[axis]
        for loop_index in polygon.loop_indices:
            position = obj.data.vertices[obj.data.loops[loop_index].vertex_index].co
            uv.data[loop_index].uv = (position[axes[0]] / repeat, position[axes[1]] / repeat)


def block(name, pos, size, mat='stone', bevel=.07, rot=(0, 0, 0)):
    bpy.ops.mesh.primitive_cube_add(size=1, location=pos)
    obj = bpy.context.object
    obj.scale = size
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    obj.rotation_euler = rot
    finish(obj, name, mat, bevel)
    planar_uv(obj)
    return obj


def cylinder(name, pos, radius, depth, mat='stone', vertices=48):
    bpy.ops.mesh.primitive_cylinder_add(vertices=vertices, radius=radius, depth=depth, location=pos)
    obj = finish(bpy.context.object, name, mat, .045, True)
    planar_uv(obj)
    return obj


def mesh(name, verts, faces, mat, smooth=False):
    data = bpy.data.meshes.new(name)
    data.from_pydata(verts, [], faces)
    data.update()
    obj = bpy.data.objects.new(name, data)
    bpy.context.collection.objects.link(obj)
    finish(obj, name, mat, smooth=smooth)
    planar_uv(obj)
    return obj


def rock(name, pos, scale, mat='stone', detail=1):
    bpy.ops.mesh.primitive_ico_sphere_add(subdivisions=detail, radius=1, location=pos)
    obj = bpy.context.object
    for vert in obj.data.vertices:
        vert.co *= rng.uniform(.80, 1.18)
    obj.scale = scale
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    obj.rotation_euler = (rng.random(), rng.random(), rng.random() * 6.28)
    finish(obj, name, mat, smooth=True)
    planar_uv(obj)
    return obj


def tube(name, points, radius, mat, sides=7):
    vertices, faces = [], []
    for i, p in enumerate(points):
        tangent = Vector(points[min(i + 1, len(points) - 1)]) - Vector(points[max(0, i - 1)])
        tangent.normalize()
        u = tangent.cross(Vector((0, 0, 1)))
        if u.length < .01:
            u = tangent.cross(Vector((0, 1, 0)))
        u.normalize()
        v = tangent.cross(u).normalized()
        r = radius * (1 - .55 * i / len(points))
        for j in range(sides):
            a = j * 2 * math.pi / sides
            vertices.append(Vector(p) + r * (math.cos(a) * u + math.sin(a) * v))
    for i in range(len(points) - 1):
        for j in range(sides):
            k = i * sides + j
            n = i * sides + (j + 1) % sides
            faces.append((k, n, n + sides, k + sides))
    return mesh(name, vertices, faces, mat, True)


# A broad seabed with windlike sand ripples and larger burial dunes.
N = 131
vertices, faces = [], []
for iy in range(N):
    y = (iy / (N - 1) - .5) * 165
    for ix in range(N):
        x = (ix / (N - 1) - .5) * 160
        dunes = .75 * math.sin(x * .14 + .5 * math.sin(y * .11)) + .48 * math.cos(y * .16 - x * .08)
        sanctuary = math.exp(-((x / 14)**4 + ((y - 1) / 25)**4))
        z = -.30 + sanctuary * .27 + dunes * (1 - sanctuary * .92) + .055 * math.sin(x * 3 + y * .5)
        vertices.append((x, y, z))
        if ix < N - 1 and iy < N - 1:
            a = iy * N + ix
            faces.append((a, a + 1, a + N + 1, a + N))
mesh('Sand dunes / buried sanctuary', vertices, faces, 'sand', True)

# Partially buried paving. Uneven edges / missing tiles prevent a clean showroom floor.
for ix in range(-4, 4):
    for iy in range(-8, 10):
        if rng.random() < .12:
            continue
        x, y = ix * 1.94 + .97, iy * 1.94
        z = .04 + rng.uniform(-.065, .05)
        block('Uneven limestone paving', (x, y, z), (1.90, 1.89, .32), bevel=.055,
              rot=(rng.uniform(-.012, .012), rng.uniform(-.012, .012), rng.uniform(-.013, .013)))


def column(x, y, height=9, broken=False):
    block('Column foot / buried lower plinth', (x, y, .36), (2.18, 2.18, .58), bevel=.1)
    block('Column stepped plinth', (x, y, .76), (1.8, 1.8, .25), bevel=.045)
    cylinder('Column torus base', (x, y, 1.03), .87, .27)
    # Actual fluted geometry, irregular broken crown, 16 carved flutes.
    seg, rings = 96, 13
    verts, quads = [], []
    for iz in range(rings):
        fraction = iz / (rings - 1)
        h = 1.12 + fraction * (height - 1.55)
        for j in range(seg):
            angle = j * 2 * math.pi / seg
            radius = .67 * (1 - .13 * fraction) + .029 * math.cos(angle * 16)
            crown = rng.uniform(-.42, .32) if broken and iz == rings - 1 else 0
            verts.append((x + radius * math.cos(angle), y + radius * math.sin(angle), h + crown))
            if iz < rings - 1:
                a = iz * seg + j
                b = iz * seg + (j + 1) % seg
                quads.append((a, b, b + seg, a + seg))
    quads.append(tuple(range((rings - 1) * seg, rings * seg)))
    mesh('Fluted column / fractured' if broken else 'Fluted limestone column', verts, quads, 'stone', True)
    if not broken:
        cylinder('Capital neck', (x, y, height - .35), .73, .24)
        cylinder('Capital spreading echinus', (x, y, height - .12), .92, .26)
        block('Capital abacus', (x, y, height + .15), (2.1, 2.1, .32), bevel=.07)
    # Scalloped erosion at base; naturally blends pillars into sand.
    for _ in range(5):
        angle = rng.random() * 6.28
        rock('Marine encrustation', (x + .73 * math.cos(angle), y + .73 * math.sin(angle), rng.uniform(.2, .8)),
             (rng.uniform(.16, .32), .17, .16), 'algae')


for y in [-12, -4, 5, 14]:
    for x in [-7, 7]:
        damaged = (x == 7 and y == -12) or (x == -7 and y == 5)
        column(x, y, 4.7 if damaged else 9, damaged)

# Paired side arcades, genuine stone voussoirs with staggered gaps.
def arch(name, center, radius, spring, depth=.85, thickness=.68, turn=0, missing=()):
    cx, cy = center
    for i in range(17):
        if i in missing:
            continue
        a0, a1 = i * math.pi / 17 + .008, (i + 1) * math.pi / 17 - .008
        verts = []
        for d in [-depth / 2, depth / 2]:
            for r, a in [(radius, a0), (radius + thickness, a0), (radius + thickness, a1), (radius, a1)]:
                local_x = r * math.cos(a)
                verts.append((cx + local_x * math.cos(turn) - d * math.sin(turn),
                              cy + local_x * math.sin(turn) + d * math.cos(turn), spring + r * math.sin(a)))
        obj = mesh(name, verts, [(0, 3, 2, 1), (4, 5, 6, 7), (0, 1, 5, 4), (1, 2, 6, 5), (2, 3, 7, 6), (3, 0, 4, 7)], 'stone')
        bevel = obj.modifiers.new('Worn arch edges', 'BEVEL')
        bevel.width = .045
        bevel.segments = 2
        bpy.context.view_layer.objects.active = obj
        bpy.ops.object.modifier_apply(modifier=bevel.name)


for x in [-7, 7]:
    for idx, y in enumerate([-8, .5, 9.5]):
        missing = ()
        if x == 7 and idx == 0:
            missing = tuple(range(6, 17))
        if x == -7 and idx == 1:
            missing = tuple(range(0, 11))
        if x == -7 and idx == 2:
            missing = tuple(range(6, 17))
        arch('Arcade arch / individual limestone blocks', (x, y), 3.5 if idx == 0 else 4, 9.3,
             turn=math.pi / 2, missing=missing)

# Monumental rear apse gives a distinct, readable cathedral silhouette.
for x in [-6.25, 6.25]:
    block('Apse pier', (x, 19, 4.7), (2, 2.4, 9.2), bevel=.12)
    block('Apse crown', (x, 19, 9.35), (2.5, 2.9, .45), bevel=.08)
arch('Apse broken double arch', (0, 19), 5.25, 9.5, depth=1.6, thickness=1.1, missing=(1,))
arch('Apse inner archivolt', (0, 18.10), 5.1, 9.5, depth=.25, thickness=.2, missing=(1, 2))
for x in [-10, 10]:
    for y in [21, 27]:
        column(x, y, 5.5 if x > 0 else 7, True)

# Broken colonnade fragments. Cylinders laid into sand show the human scale.
for x, y, length, angle in [(10, -12, 5, .8), (-10, -2, 4.5, -.6), (-5, 5, 3, 1.9)]:
    fallen = cylinder('Fallen column drum', (x, y, .52), .68, length)
    fallen.rotation_euler = (math.pi / 2, .04, angle)
for _ in range(85):
    side = rng.choice([-1, 1])
    x = side * rng.uniform(8.5, 19)
    y = rng.uniform(-23, 30)
    s = rng.uniform(.22, 1.05)
    rock('Collapsed stone rubble', (x, y, .12), (s, s * rng.uniform(.5, 1.5), s * .55), detail=1)

# Raised sanctuary, brass inlays and the suspended basalt seal.
for level in range(4):
    block('Sanctuary step', (0, 10, .28 + level * .40), (12.8 - level * 1.25, 10.6 - level * 1.08, .45), bevel=.09)
for x in [-4.6, 4.6]:
    block('Bronze stair inlay', (x, 9.6, 1.72), (.065, 5.2, .025), 'bronze', .01)

# Three fractured pieces assembled into a slowly levitating rectangular seal.
# Named parent is the single runtime animation pivot; the architecture stays static.
seal = bpy.data.objects.new('Abyss_Seal', None)
bpy.context.collection.objects.link(seal)
seal.location = (0, 10, 7.8)
seal_objects = []
for index, (height, z, width, offset) in enumerate([(3.1, 4.67, 5.6, -.1), (2.7, 7.73, 5.75, .10), (2.65, 10.53, 5.5, -.04)]):
    shard = block('Floating basalt seal shard', (offset, 10, z), (width, 1.2, height), 'basalt', .12,
                  rot=(0, -.008 if index == 1 else .012, .014 * (index - 1)))
    seal_objects.append(shard)
    for x in [-2.3, 2.3]:
        seal_objects.append(block('Ancient bronze edge inlay', (x + offset, 9.375, z), (.055, .03, height - .32), 'bronze', .008))

# Concentric carved bronze / faintly emissive ring: no giant neon portal.
for radius, minor, mat in [(1.67, .055, 'bronze'), (1.50, .025, 'glow'), (1.25, .025, 'bronze')]:
    bpy.ops.mesh.primitive_torus_add(major_radius=radius, minor_radius=minor, major_segments=96, minor_segments=8,
                                   location=(0, 9.32, 7.8), rotation=(math.pi / 2, 0, 0))
    seal_objects.append(finish(bpy.context.object, 'Astronomical seal / carved ring', mat, smooth=True))
for i in range(24):
    a = 2 * math.pi * i / 24
    x, z = 1.88 * math.sin(a), 7.8 + 1.88 * math.cos(a)
    seal_objects.append(block('Carved bronze radial mark', (x, 9.335, z), (.032, .024, .17 if i % 3 else .29), 'bronze', .007, (0, a, 0)))
seal_objects.append(block('Seal central glyph', (0, 9.315, 7.8), (.07, .028, .85), 'glow', .012))
seal_objects.append(block('Seal central glyph', (0, 9.315, 7.8), (.46, .028, .055), 'glow', .012))
for obj in seal_objects:
    world = obj.matrix_world.copy()
    obj.parent = seal
    obj.matrix_world = world
asset_objects.append(seal)

# Weathered ceremonial lamps give a warm scale reference in the blue water.
for x in [-4.5, 4.5]:
    cylinder('Ceremonial plinth', (x, 7, 2.35), .40, 1.25, 'basalt')
    cylinder('Bronze bowl', (x, 7, 3.05), .57, .20, 'bronze')
    cylinder('Sleeping amber coral', (x, 7, 3.19), .32, .13, 'glow')

# Organic foreground sea fans, branching coral and broad kelp ribbons.
for k in range(24):
    x = rng.choice([-1, 1]) * rng.uniform(10, 22)
    y = rng.uniform(-19, 23)
    h = rng.uniform(.75, 2.1)
    tube('Coral trunk', [(x, y, -.1), (x + .06, y, h * .6), (x + .1, y, h)], .065, 'algae')
    for branch in range(5):
        side = -1 if branch % 2 else 1
        base = h * (.22 + branch * .13)
        endx = x + side * h * rng.uniform(.32, .62)
        tube('Coral fan branch', [(x, y, base), ((x + endx) / 2, y + .05, base + h * .20),
                                  (endx, y + .08, min(h + .1, base + h * .36))], .038, 'algae')
    for ribbon in range(3):
        verts, quads = [], []
        offset = rng.uniform(-.5, .5)
        height = rng.uniform(1, 2.8)
        for i in range(12):
            f = i / 11
            width = .13 * math.sin(math.pi * f)**.5 + .016
            cx = x + offset + .23 * math.sin(f * 4 + k)
            cy = y + .25 * f + .12 * math.sin(f * 5)
            verts.extend([(cx - width, cy, f * height - .15), (cx + width, cy, f * height - .15)])
            if i < 11:
                a = i * 2
                quads.append((a, a + 1, a + 3, a + 2))
        mesh('Kelp ribbon', verts, quads, 'algae', True)

# Cameras are exported explicitly after Blender-to-glTF axis conversion.
poses = [
    {'position': [11, 7.3, 31], 'target': [0, 6.5, -6], 'fov': 43},
    {'position': [1, 4.5, 19], 'target': [0, 7, -9], 'fov': 50},
    {'position': [-18, 5, 1], 'target': [0, 7, -10], 'fov': 52},
]


def from_three(p):
    return Vector((p[0], -p[2], p[1]))


cameras = []
for i, pose in enumerate(poses):
    data = bpy.data.cameras.new(f'Camera {i + 1}')
    data.type = 'PERSP'
    data.sensor_fit = 'VERTICAL'
    data.sensor_height = 24
    data.lens = 12 / math.tan(math.radians(pose['fov']) / 2)
    data.clip_end = 300
    obj = bpy.data.objects.new(f'Camera {i + 1}', data)
    bpy.context.collection.objects.link(obj)
    obj.location = from_three(pose['position'])
    obj.rotation_euler = (from_three(pose['target']) - obj.location).to_track_quat('-Z', 'Y').to_euler()
    cameras.append(obj)
(ROOT / 'assets' / 'cameras.json').write_text(json.dumps(poses, indent=2) + '\n', encoding='utf-8')

# Export only real authored assets. Atmosphere and lights stay in the .blend.
bpy.ops.object.select_all(action='DESELECT')
for obj in asset_objects:
    obj.select_set(True)
bpy.ops.export_scene.gltf(filepath=str(ROOT / 'assets' / 'scene.glb'), export_format='GLB',
                          use_selection=True, export_yup=True, export_apply=True,
                          export_cameras=False, export_lights=False, export_animations=False)

scene = bpy.context.scene
scene.render.engine = 'CYCLES'
prefs = bpy.context.preferences.addons['cycles'].preferences
gpu = False
try:
    prefs.compute_device_type = 'OPTIX'
    prefs.get_devices()
    gpu = any(device.type == 'OPTIX' for device in prefs.devices)
    for device in prefs.devices:
        device.use = device.type == 'OPTIX'
except (TypeError, RuntimeError):
    pass
scene.cycles.device = 'GPU' if gpu else 'CPU'
scene.cycles.samples = args.samples
scene.cycles.use_denoising = True
scene.cycles.max_bounces = 7
scene.cycles.volume_bounces = 2
scene.world.color = (.008, .016, .02)
scene.world.use_nodes = True
background = scene.world.node_tree.nodes.get('Background')
background.inputs['Color'].default_value = (.035, .105, .15, 1)
background.inputs['Strength'].default_value = .36


def light(name, kind, location, energy, color, size=1, target=(0, 5, 0)):
    data = bpy.data.lights.new(name, kind)
    data.energy = energy
    data.color = color
    if kind == 'AREA':
        data.shape = 'DISK'
        data.size = size
    if kind == 'SPOT':
        data.spot_size = size
        data.spot_blend = .55
        data.shadow_soft_size = .35
    obj = bpy.data.objects.new(name, data)
    bpy.context.collection.objects.link(obj)
    obj.location = location
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat('-Z', 'Y').to_euler()
    return obj


light('Ocean skylight', 'AREA', (-8, -6, 28), 9500, (.48, .81, 1), 16)
light('Soft temple fill', 'AREA', (6, -18, 12), 2000, (.22, .64, .72), 15, (0, 10, 7))
for x, y, power, cone in [(4, 5, 280000, .23), (13, 10, 310000, .24), (-5, 17, 210000, .16)]:
    light('Shaft from distant surface', 'SPOT', (x + 10, y, 34), power, (.63, .89, 1), cone, (x - 4, y - 5, 0))
for x in [-4.5, 4.5]:
    light('Amber coral glow', 'POINT', (x, 7, 3.6), 42, (1, .57, .21))
light('Seal warm bounce', 'AREA', (0, 8.5, 7.5), 65, (1, .64, .28), 3, (0, 0, 6))

# Homogeneous water, kept out of GLB; realtime uses matching depth fog/shafts.
bpy.ops.mesh.primitive_cube_add(size=1, location=(0, 0, 13))
volume = bpy.context.object
volume.name = 'Render only / ocean volume'
volume.scale = (180, 180, 90)
water = bpy.data.materials.new('Render only / underwater scattering')
water.use_nodes = True
nodes = water.node_tree.nodes
nodes.clear()
out = nodes.new('ShaderNodeOutputMaterial')
scatter = nodes.new('ShaderNodeVolumePrincipled')
scatter.inputs['Color'].default_value = (.18, .44, .48, 1)
scatter.inputs['Density'].default_value = .019
scatter.inputs['Anisotropy'].default_value = .38
water.node_tree.links.new(scatter.outputs['Volume'], out.inputs['Volume'])
volume.data.materials.append(water)
scene.render.resolution_x = args.width
scene.render.resolution_y = round(args.width * 9 / 16)
scene.render.resolution_percentage = 100
scene.render.image_settings.file_format = 'PNG'
scene.view_settings.view_transform = 'AgX'
scene.view_settings.look = 'AgX - Medium High Contrast'
scene.view_settings.exposure = .4
scene.camera = cameras[0]
bpy.ops.file.pack_all()
bpy.ops.wm.save_as_mainfile(filepath=str(ROOT / 'blender' / 'abyssal-ruins.blend'), compress=True)
(ROOT / 'renders').mkdir(exist_ok=True)
for index in args.render:
    if index not in range(3):
        raise ValueError('Camera index must be 0, 1 or 2')
    scene.camera = cameras[index]
    scene.render.filepath = str(ROOT / 'renders' / f'camera-{index + 1}.png')
    bpy.ops.render.render(write_still=True)
print('ABYSS_BUILD_COMPLETE', json.dumps({'objects': len(asset_objects), 'glb_bytes': (ROOT / 'assets' / 'scene.glb').stat().st_size}))
