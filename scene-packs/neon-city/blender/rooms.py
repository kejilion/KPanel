"""Renders the rooms seen through the neon city's windows.

Each lit window in the city shows a room behind it, drawn with "interior
mapping": the shader follows the view ray into a box behind the pane and looks
up where it lands in a picture of that box, taken from a fixed camera. This
script builds eight such rooms (a living room, an open-plan office, a bedroom,
a kitchen, a meeting room, a room behind blinds, one behind curtains, and a
stairwell), lights them as they would be at night, renders each with Cycles
from that camera and packs them into one atlas:

    rooms.webp   4 x 2 rooms, 384 px each

The room is the pane's own size (2.2 m square) and 2.8 m deep; the camera sits
1.32 m outside the pane, framing it exactly. The shader uses the same numbers
(see ROOM in src/buildings.ts).

    blender -b --factory-startup --python blender/rooms.py -- <out dir>
"""
import math
import os
import sys

import bpy
import numpy as np
from mathutils import Vector

OUT = os.path.abspath(sys.argv[sys.argv.index('--') + 1]) if '--' in sys.argv else os.path.join(os.path.dirname(__file__), 'out')
SIZE = 384
PANE = 2.2        # width and height of the pane, and of the room behind it (m)
DEPTH = 2.8       # depth of the room (m)
CAMERA = 1.32     # camera distance outside the pane (m): 1.2 half-panes
RNG = np.random.default_rng(20260925)
H = PANE / 2


def material(name, colour, rough=0.6, emit=0.0, emit_colour=None, metal=0.0):
    mat = bpy.data.materials.new(name)
    mat.use_nodes = True
    bsdf = mat.node_tree.nodes['Principled BSDF']
    bsdf.inputs['Base Color'].default_value = (*colour, 1.0)
    bsdf.inputs['Roughness'].default_value = rough
    bsdf.inputs['Metallic'].default_value = metal
    if emit > 0:
        bsdf.inputs['Emission Color'].default_value = (*(emit_colour or colour), 1.0)
        bsdf.inputs['Emission Strength'].default_value = emit
    return mat


def box(centre, size, mat, rotation=0.0):
    bpy.ops.mesh.primitive_cube_add(size=1, location=centre, rotation=(0, 0, rotation))
    obj = bpy.context.object
    obj.scale = size
    obj.data.materials.append(mat)
    return obj


def cylinder(centre, radius, depth, mat, vertices=24, rotation=(0, 0, 0)):
    bpy.ops.mesh.primitive_cylinder_add(radius=radius, depth=depth, location=centre, vertices=vertices, rotation=rotation)
    obj = bpy.context.object
    obj.data.materials.append(mat)
    bpy.ops.object.shade_smooth()
    return obj


def sphere(centre, radius, mat):
    bpy.ops.mesh.primitive_uv_sphere_add(radius=radius, location=centre, segments=16, ring_count=10)
    obj = bpy.context.object
    obj.data.materials.append(mat)
    bpy.ops.object.shade_smooth()
    return obj


def area_light(centre, size, power, colour, rotation=(0, 0, 0)):
    bpy.ops.object.light_add(type='AREA', location=centre, rotation=rotation)
    light = bpy.context.object
    light.data.size = size
    light.data.energy = power
    light.data.color = colour
    return light


def point_light(centre, power, colour, radius=0.05):
    bpy.ops.object.light_add(type='POINT', location=centre)
    light = bpy.context.object
    light.data.energy = power
    light.data.color = colour
    light.data.shadow_soft_size = radius
    return light


def shell(wall, floor, ceiling, back=None):
    """The room: floor, ceiling, side walls and back wall (no front wall: that is the window)."""
    box((0, DEPTH / 2, -H - 0.05), (PANE + 0.4, DEPTH + 0.2, 0.1), floor)
    box((0, DEPTH / 2, H + 0.05), (PANE + 0.4, DEPTH + 0.2, 0.1), ceiling)
    box((-H - 0.05, DEPTH / 2, 0), (0.1, DEPTH + 0.2, PANE + 0.2), wall)
    box((H + 0.05, DEPTH / 2, 0), (0.1, DEPTH + 0.2, PANE + 0.2), wall)
    box((0, DEPTH + 0.05, 0), (PANE + 0.2, 0.1, PANE + 0.2), back or wall)
    # A skirting board and a sill inside the window.
    skirting = material('skirting', (0.85, 0.84, 0.8), 0.4)
    box((0, DEPTH - 0.01, -H + 0.05), (PANE, 0.02, 0.1), skirting)
    for side in (-1, 1):
        box((side * (H - 0.01), DEPTH / 2, -H + 0.05), (0.02, DEPTH, 0.1), skirting)


def books(x0, x1, z, y, depth=0.22):
    x = x0
    while x < x1 - 0.03:
        width = RNG.uniform(0.025, 0.06)
        height = RNG.uniform(0.18, 0.28)
        colour = RNG.choice([(0.5, 0.1, 0.08), (0.1, 0.2, 0.45), (0.7, 0.6, 0.35), (0.15, 0.3, 0.15), (0.8, 0.78, 0.7), (0.2, 0.2, 0.22)])
        box((x + width / 2, y, z + height / 2), (width * 0.95, depth * RNG.uniform(0.7, 1.0), height), material('book', tuple(c * RNG.uniform(0.7, 1.1) for c in colour), 0.7))
        x += width


def plant(centre, height):
    pot = material('pot', (0.55, 0.3, 0.2), 0.8)
    leaf = material('leaf', (0.08, 0.22, 0.06), 0.6)
    cylinder((centre[0], centre[1], centre[2] + 0.12), 0.13, 0.24, pot)
    for _ in range(9):
        offset = Vector((RNG.uniform(-0.18, 0.18), RNG.uniform(-0.18, 0.18), RNG.uniform(0.3, height)))
        sphere((centre[0] + offset.x, centre[1] + offset.y, centre[2] + offset.z), RNG.uniform(0.09, 0.17), leaf)


def living_room():
    shell(material('wall', (0.72, 0.66, 0.56), 0.9), material('floor', (0.32, 0.2, 0.12), 0.45), material('ceiling', (0.85, 0.84, 0.8), 0.9))
    sofa = material('sofa', (0.18, 0.28, 0.35), 0.9)
    box((0, DEPTH - 0.45, -H + 0.22), (1.7, 0.8, 0.44), sofa)
    box((0, DEPTH - 0.12, -H + 0.52), (1.7, 0.22, 0.6), sofa)
    for side in (-1, 1):
        box((side * 0.8, DEPTH - 0.45, -H + 0.36), (0.18, 0.8, 0.28), sofa)
    box((0, DEPTH - 1.35, -H + 0.2), (0.9, 0.5, 0.05), material('table', (0.35, 0.22, 0.12), 0.35))
    box((0, DEPTH - 1.3, -H + 0.005), (1.8, 1.6, 0.01), material('rug', (0.55, 0.25, 0.18), 1.0))
    # A picture over the sofa and a floor lamp in the corner.
    box((0.1, DEPTH - 0.005, 0.25), (0.8, 0.02, 0.55), material('art', (0.2, 0.35, 0.5), 0.5))
    cylinder((-0.85, DEPTH - 0.35, -H + 0.8), 0.02, 1.6, material('steel', (0.2, 0.2, 0.2), 0.3, metal=1.0))
    shade = material('shade', (1.0, 0.9, 0.75), 0.9, emit=6.0, emit_colour=(1.0, 0.72, 0.45))
    cylinder((-0.85, DEPTH - 0.35, 0.55), 0.18, 0.28, shade, rotation=(0, 0, 0))
    point_light((-0.85, DEPTH - 0.35, 0.5), 60, (1.0, 0.68, 0.4), 0.15)
    # A television on the side wall, its glow on the room.
    box((H - 0.03, DEPTH - 1.4, 0.05), (0.04, 1.0, 0.58), material('tv', (0.02, 0.02, 0.02), 0.2, emit=3.0, emit_colour=(0.35, 0.55, 1.0)))
    area_light((H - 0.08, DEPTH - 1.4, 0.05), 0.8, 25, (0.4, 0.6, 1.0), rotation=(0, -math.pi / 2, 0))
    plant((0.8, DEPTH - 0.3, -H), 0.9)


def office():
    shell(material('wall', (0.8, 0.8, 0.78), 0.9), material('carpet', (0.25, 0.27, 0.3), 1.0), material('tiles', (0.9, 0.9, 0.9), 0.8))
    desk = material('desk', (0.85, 0.85, 0.82), 0.4)
    black = material('black', (0.03, 0.03, 0.035), 0.4)
    screen = material('screen', (0.02, 0.02, 0.02), 0.3, emit=4.0, emit_colour=(0.55, 0.75, 1.0))
    for row, y in enumerate((DEPTH - 2.1, DEPTH - 1.0)):
        box((0, y, -H + 0.73), (2.0, 0.7, 0.04), desk)
        for x in (-0.55, 0.55):
            box((x, y + 0.2, -H + 0.98), (0.55, 0.03, 0.34), screen if RNG.random() < 0.7 else black)
            box((x, y + 0.22, -H + 0.8), (0.05, 0.05, 0.12), black)
            if row == 1:
                box((x, y - 0.5, -H + 0.45), (0.45, 0.45, 0.08), black)
                box((x, y - 0.72, -H + 0.72), (0.45, 0.06, 0.5), black)
    # Fluorescent panels in the ceiling: the main light of an office at night.
    panel = material('panel', (1.0, 1.0, 1.0), 0.5, emit=3.0, emit_colour=(0.9, 0.95, 1.0))
    for y in (DEPTH - 2.1, DEPTH - 0.7):
        box((0, y, H - 0.005), (1.2, 0.6, 0.01), panel)
        area_light((0, y, H - 0.02), 1.0, 22, (0.9, 0.95, 1.0))
    box((0, DEPTH - 0.02, 0.3), (1.4, 0.02, 0.8), material('whiteboard', (0.95, 0.95, 0.95), 0.2))


def bedroom():
    shell(material('wall', (0.55, 0.62, 0.68), 0.9), material('floor', (0.45, 0.32, 0.2), 0.5), material('ceiling', (0.88, 0.87, 0.85), 0.9))
    bed = material('bed', (0.88, 0.86, 0.82), 0.95)
    box((0.1, DEPTH - 1.0, -H + 0.25), (1.5, 1.9, 0.5), bed)
    box((0.1, DEPTH - 0.08, -H + 0.55), (1.6, 0.12, 1.1), material('headboard', (0.3, 0.2, 0.15), 0.6))
    box((0.1, DEPTH - 1.35, -H + 0.52), (1.52, 1.1, 0.06), material('blanket', (0.5, 0.18, 0.2), 1.0))
    table = material('nightstand', (0.35, 0.24, 0.15), 0.5)
    box((-0.9, DEPTH - 0.3, -H + 0.25), (0.4, 0.4, 0.5), table)
    shade = material('shade', (1.0, 0.9, 0.75), 0.9, emit=5.0, emit_colour=(1.0, 0.7, 0.4))
    cylinder((-0.9, DEPTH - 0.3, -H + 0.75), 0.12, 0.2, shade)
    point_light((-0.9, DEPTH - 0.3, -H + 0.72), 35, (1.0, 0.65, 0.35), 0.1)
    box((H - 0.25, DEPTH - 0.45, -H + 0.9), (0.5, 0.8, 1.8), material('wardrobe', (0.6, 0.5, 0.4), 0.5))
    area_light((0, DEPTH - 1.2, H - 0.02), 0.5, 8, (1.0, 0.85, 0.7))


def kitchen():
    shell(material('wall', (0.9, 0.88, 0.82), 0.8), material('tiles', (0.4, 0.4, 0.42), 0.3), material('ceiling', (0.9, 0.9, 0.9), 0.9),
          back=material('splash', (0.75, 0.82, 0.8), 0.2))
    cabinet = material('cabinet', (0.2, 0.3, 0.26), 0.5)
    top = material('top', (0.15, 0.15, 0.16), 0.25)
    box((0, DEPTH - 0.3, -H + 0.45), (PANE, 0.6, 0.9), cabinet)
    box((0, DEPTH - 0.3, -H + 0.92), (PANE, 0.62, 0.04), top)
    box((0, DEPTH - 0.18, 0.55), (PANE, 0.35, 0.7), cabinet)
    box((-H + 0.35, DEPTH - 1.2, -H + 0.9), (0.7, 0.7, 1.8), material('fridge', (0.8, 0.8, 0.82), 0.25, metal=0.6))
    box((0.3, DEPTH - 1.5, -H + 0.45), (1.0, 0.6, 0.9), cabinet)
    pendant = material('pendant', (1.0, 0.85, 0.6), 0.5, emit=10.0, emit_colour=(1.0, 0.75, 0.45))
    for x in (0.0, 0.6):
        sphere((x, DEPTH - 1.5, H - 0.55), 0.09, pendant)
        cylinder((x, DEPTH - 1.5, H - 0.25), 0.005, 0.5, material('cord', (0.05, 0.05, 0.05)))
        point_light((x, DEPTH - 1.5, H - 0.6), 25, (1.0, 0.72, 0.42), 0.08)
    area_light((0, DEPTH - 1.0, H - 0.02), 1.4, 18, (1.0, 0.9, 0.8))


def meeting_room():
    shell(material('wall', (0.3, 0.32, 0.36), 0.8), material('carpet', (0.15, 0.16, 0.2), 1.0), material('ceiling', (0.85, 0.85, 0.85), 0.9),
          back=material('wood', (0.4, 0.28, 0.18), 0.5))
    box((0, DEPTH - 1.3, -H + 0.75), (1.2, 2.0, 0.05), material('table', (0.9, 0.9, 0.88), 0.3))
    chair = material('chair', (0.1, 0.1, 0.12), 0.5)
    for y in (DEPTH - 2.0, DEPTH - 1.3, DEPTH - 0.6):
        for side in (-1, 1):
            box((side * 0.8, y, -H + 0.5), (0.4, 0.4, 0.1), chair)
            box((side * 1.0, y, -H + 0.8), (0.06, 0.4, 0.55), chair)
    box((0, DEPTH - 0.02, 0.2), (1.5, 0.02, 0.85), material('display', (0.02, 0.02, 0.02), 0.2, emit=3.0, emit_colour=(0.2, 0.5, 0.9)))
    for y in (DEPTH - 2.0, DEPTH - 1.0):
        cylinder((0, y, H - 0.02), 0.12, 0.02, material('downlight', (1, 1, 1), emit=6.0, emit_colour=(1.0, 0.9, 0.75)))
        point_light((0, y, H - 0.1), 22, (1.0, 0.88, 0.7), 0.1)


def blinds():
    living_room()
    slat = material('slat', (0.9, 0.88, 0.82), 0.6)
    for z in np.arange(-H + 0.05, H, 0.09):
        box((0, 0.12, z), (PANE, 0.07, 0.012), slat, 0)
        bpy.context.object.rotation_euler = (math.radians(60), 0, 0)


def curtains():
    bedroom()
    fabric = material('curtain', (0.75, 0.55, 0.35), 1.0)
    # Two curtains drawn most of the way, folds as a row of rounded pleats.
    for side, reach in ((-1, 0.72), (1, 0.62)):
        x0 = side * H
        x1 = side * (H - reach)
        for k in range(10):
            t = (k + 0.5) / 10
            x = x0 + (x1 - x0) * t
            cylinder((x, 0.2 + 0.03 * math.sin(k * 2.1), 0), 0.045, PANE, fabric, 12)


def stairwell():
    shell(material('wall', (0.55, 0.6, 0.55), 0.7), material('concrete', (0.35, 0.35, 0.34), 0.8), material('ceiling', (0.6, 0.62, 0.6), 0.9))
    step = material('step', (0.4, 0.4, 0.4), 0.7)
    for k in range(9):
        box((0.3, DEPTH - 1.5 + k * 0.17, -H + 0.09 + k * 0.17), (1.0, 0.3, 0.18), step)
    box((-0.2, DEPTH - 0.8, 0), (0.04, 1.6, 0.04), material('rail', (0.6, 0.6, 0.62), 0.3, metal=1.0))
    box((-H + 0.02, DEPTH - 0.8, -H + 1.0), (0.02, 0.9, 2.0), material('door', (0.3, 0.35, 0.4), 0.5))
    tube = material('tube', (1, 1, 1), emit=10.0, emit_colour=(0.85, 1.0, 0.9))
    box((0, DEPTH - 1.2, H - 0.03), (0.08, 1.2, 0.04), tube)
    area_light((0, DEPTH - 1.2, H - 0.06), 1.0, 35, (0.85, 1.0, 0.88))
    box((0.6, DEPTH - 0.01, 0.6), (0.3, 0.02, 0.15), material('exit', (0.1, 1.0, 0.3), emit=6.0))


ROOMS = [living_room, office, bedroom, kitchen, meeting_room, blinds, curtains, stairwell]


def render_room(build, path):
    bpy.ops.wm.read_factory_settings(use_empty=True)
    scene = bpy.context.scene
    scene.render.engine = 'CYCLES'
    prefs = bpy.context.preferences.addons['cycles'].preferences
    for kind in ('OPTIX', 'CUDA'):
        try:
            prefs.compute_device_type = kind
            prefs.get_devices()
            if any(device.type == kind for device in prefs.devices):
                for device in prefs.devices:
                    device.use = device.type == kind
                scene.cycles.device = 'GPU'
                break
        except TypeError:
            continue
    scene.cycles.samples = 256
    scene.cycles.use_denoising = True
    scene.render.resolution_x = SIZE
    scene.render.resolution_y = SIZE
    scene.render.image_settings.file_format = 'PNG'
    scene.view_settings.view_transform = 'Standard'
    scene.view_settings.exposure = 0.0
    world = bpy.data.worlds.new('night')
    world.use_nodes = True
    world.node_tree.nodes['Background'].inputs['Color'].default_value = (0.004, 0.004, 0.008, 1)
    scene.world = world
    build()
    bpy.ops.object.camera_add(location=(0, -CAMERA, 0), rotation=(math.pi / 2, 0, 0))
    camera = bpy.context.object
    camera.data.sensor_fit = 'HORIZONTAL'
    camera.data.angle = 2 * math.atan(H / CAMERA)
    camera.data.clip_start = CAMERA * 0.5
    scene.camera = camera
    scene.render.filepath = path
    bpy.ops.render.render(write_still=True)


def main():
    os.makedirs(OUT, exist_ok=True)
    paths = []
    for index, build in enumerate(ROOMS):
        path = os.path.join(OUT, f'room-{index}.png')
        render_room(build, path)
        paths.append(path)
        print(f'rooms: {build.__name__} rendered', flush=True)
    atlas = np.zeros((SIZE * 2, SIZE * 4, 4), dtype=np.float32)
    for index, path in enumerate(paths):
        image = bpy.data.images.load(path)
        pixels = np.empty(SIZE * SIZE * 4, dtype=np.float32)
        image.pixels.foreach_get(pixels)
        tile = pixels.reshape(SIZE, SIZE, 4)
        row, column = index // 4, index % 4
        # Blender's rows run bottom-up; the atlas is stored bottom-up too, so it saves top-down.
        atlas[(1 - row) * SIZE:(2 - row) * SIZE, column * SIZE:(column + 1) * SIZE] = tile
    out = bpy.data.images.new('rooms', SIZE * 4, SIZE * 2, alpha=False)
    atlas[..., 3] = 1
    out.pixels.foreach_set(atlas.ravel())
    out.filepath_raw = os.path.join(OUT, 'rooms.png')
    out.file_format = 'PNG'
    out.save()
    print('rooms: atlas written', flush=True)


main()
