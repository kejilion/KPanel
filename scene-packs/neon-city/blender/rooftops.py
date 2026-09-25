"""Models the rooftop clutter of the neon city: what real roofs carry.

Seven props, each its own mesh in one glTF binary, each 1 unit = 1 m with its
base at the origin, coloured per vertex (no textures):

    hvac      a packaged air-conditioning unit with fan grilles
    chiller   a row of cooling towers with round fans on top
    tank      a timber water tank on a steel stand, conical roof
    housing   a stair and lift housing with a door and a vent
    antenna   a lattice mast with dishes and small aerials
    dish      a satellite dish on a stand
    vents     a cluster of vent pipes and small boxes

The scene scatters them over the roofs (see src/rooftops.ts).

    blender -b --factory-startup --python blender/rooftops.py -- <out dir>
"""
import math
import os
import sys

import bmesh
import bpy
from mathutils import Matrix, Vector

OUT = os.path.abspath(sys.argv[sys.argv.index('--') + 1]) if '--' in sys.argv else os.path.join(os.path.dirname(__file__), 'out')

GREY = (0.42, 0.43, 0.44)
LIGHT = (0.62, 0.63, 0.62)
DARK = (0.08, 0.085, 0.09)
STEEL = (0.3, 0.31, 0.33)
RUST = (0.36, 0.2, 0.12)
TIMBER = (0.3, 0.2, 0.12)
WHITE = (0.75, 0.76, 0.75)
DOOR = (0.2, 0.24, 0.3)


class Prop:
    def __init__(self, name):
        self.name = name
        self.bm = bmesh.new()
        self.colour = self.bm.loops.layers.color.new('Col')

    def _paint(self, verts, colour):
        faces = {face for vert in verts for face in vert.link_faces}
        for face in faces:
            for loop in face.loops:
                loop[self.colour] = (*colour, 1.0)
        return faces

    def box(self, centre, size, colour, rotation=0.0):
        result = bmesh.ops.create_cube(self.bm, size=1.0)
        matrix = Matrix.Translation(centre) @ Matrix.Rotation(rotation, 4, 'Z') @ Matrix.Diagonal((*size, 1.0))
        bmesh.ops.transform(self.bm, matrix=matrix, verts=result['verts'])
        self._paint(result['verts'], colour)

    def cylinder(self, centre, radius, height, colour, segments=16, top=None, axis='Z'):
        result = bmesh.ops.create_cone(self.bm, cap_ends=True, cap_tris=False, segments=segments, radius1=radius,
                                       radius2=radius if top is None else top, depth=height)
        rotation = {'Z': Matrix.Identity(4), 'X': Matrix.Rotation(math.pi / 2, 4, 'Y'), 'Y': Matrix.Rotation(math.pi / 2, 4, 'X')}[axis]
        bmesh.ops.transform(self.bm, matrix=Matrix.Translation(centre) @ rotation, verts=result['verts'])
        self._paint(result['verts'], colour)

    def build(self):
        mesh = bpy.data.meshes.new(self.name)
        self.bm.to_mesh(mesh)
        self.bm.free()
        # The exporter writes the active colour attribute (as COLOR_0) when there is no material.
        mesh.color_attributes.active_color = mesh.color_attributes['Col']
        mesh.color_attributes.render_color_index = 0
        obj = bpy.data.objects.new(self.name, mesh)
        bpy.context.scene.collection.objects.link(obj)
        return obj


def hvac():
    p = Prop('hvac')
    p.box((0, 0, 0.8), (3.2, 1.8, 1.6), GREY)
    p.box((0, 0, 1.62), (3.3, 1.9, 0.06), LIGHT)
    for x in (-0.8, 0.8):
        p.cylinder((x, 0, 1.68), 0.62, 0.08, DARK, 24)
        p.cylinder((x, 0, 1.73), 0.12, 0.06, STEEL, 12)
    for side in (-1, 1):
        for k in range(6):
            p.box((-1.2 + k * 0.48, side * 0.92, 0.8), (0.3, 0.04, 1.1), DARK)
    p.box((0, 0, 0.06), (3.4, 2.0, 0.12), STEEL)
    return p.build()


def chiller():
    p = Prop('chiller')
    p.box((0, 0, 1.4), (7.5, 2.6, 2.8), LIGHT)
    for k in range(3):
        x = -2.5 + k * 2.5
        p.cylinder((x, 0, 3.05), 1.05, 0.5, GREY, 28)
        p.cylinder((x, 0, 3.32), 0.95, 0.05, DARK, 28)
        p.cylinder((x, 0, 3.36), 0.15, 0.08, STEEL, 12)
    for k in range(12):
        p.box((-3.3 + k * 0.6, 1.31, 1.2), (0.35, 0.04, 1.8), GREY)
    p.box((0, 0, 0.08), (7.8, 2.9, 0.16), STEEL)
    return p.build()


def tank():
    p = Prop('tank')
    for x in (-1.4, 1.4):
        for y in (-1.4, 1.4):
            p.box((x, y, 1.5), (0.2, 0.2, 3.0), STEEL)
    p.box((0, 0, 3.05), (3.4, 3.4, 0.15), STEEL)
    p.cylinder((0, 0, 4.6), 1.8, 3.0, TIMBER, 32)
    for z in (3.5, 4.4, 5.3):
        p.cylinder((0, 0, z), 1.84, 0.08, RUST, 32)
    p.cylinder((0, 0, 6.5), 1.9, 0.8, DARK, 32, top=0.1)
    p.box((1.9, 0, 4.2), (0.06, 0.5, 4.0), RUST)
    return p.build()


def housing():
    p = Prop('housing')
    p.box((0, 0, 1.8), (4.0, 3.2, 3.6), GREY)
    p.box((0, 0, 3.65), (4.2, 3.4, 0.12), LIGHT)
    p.box((0, -1.61, 1.1), (1.0, 0.04, 2.1), DOOR)
    p.box((1.2, -1.61, 2.7), (0.9, 0.04, 0.5), DARK)
    p.box((-1.2, 0, 4.1), (0.9, 0.9, 0.8), STEEL)
    p.cylinder((1.2, 0.8, 4.2), 0.25, 1.0, STEEL, 12)
    return p.build()


def antenna():
    p = Prop('antenna')
    height = 14.0
    for x in (-0.35, 0.35):
        for y in (-0.35, 0.35):
            p.box((x, y, height / 2), (0.08, 0.08, height), RUST)
    for z in range(1, int(height), 1):
        for side in range(4):
            angle = side * math.pi / 2
            p.box((math.cos(angle) * 0.35, math.sin(angle) * 0.35, z), (0.05, 0.75, 0.05), RUST, angle)
    p.cylinder((0, 0, height + 1.5), 0.04, 3.0, STEEL, 6)
    p.cylinder((0.55, 0, 9.0), 0.6, 0.2, WHITE, 20, top=0.2, axis='X')
    p.cylinder((-0.5, 0.2, 11.0), 0.45, 0.16, WHITE, 20, top=0.15, axis='X')
    for z in (6.0, 12.0):
        p.box((0, 0, z), (2.4, 0.06, 0.06), STEEL)
        for x in (-1.2, 1.2):
            p.cylinder((x, 0, z + 0.5), 0.03, 1.0, STEEL, 6)
    return p.build()


def dish():
    p = Prop('dish')
    p.box((0, 0, 0.1), (1.4, 1.4, 0.2), STEEL)
    p.cylinder((0, 0, 0.9), 0.08, 1.6, STEEL, 8)
    p.cylinder((0.3, 0, 1.9), 1.2, 0.35, WHITE, 32, top=0.2, axis='X')
    p.cylinder((1.1, 0, 1.9), 0.05, 1.4, STEEL, 6, axis='X')
    return p.build()


def vents():
    p = Prop('vents')
    for x, y, r, h in ((-0.8, -0.4, 0.25, 1.4), (0.2, 0.5, 0.35, 1.0), (0.9, -0.3, 0.18, 1.8)):
        p.cylinder((x, y, h / 2), r, h, STEEL, 12)
        p.cylinder((x, y, h + 0.1), r * 1.6, 0.2, GREY, 12, top=r * 0.4)
    p.box((0.2, -0.8, 0.4), (1.2, 0.8, 0.8), GREY)
    return p.build()


def main():
    os.makedirs(OUT, exist_ok=True)
    bpy.ops.wm.read_factory_settings(use_empty=True)
    objects = [hvac(), chiller(), tank(), housing(), antenna(), dish(), vents()]
    for obj in objects:
        obj.select_set(True)
    bpy.ops.export_scene.gltf(filepath=os.path.join(OUT, 'rooftops.glb'), export_format='GLB', use_selection=True,
                              export_materials='NONE', export_vertex_color='ACTIVE', export_active_vertex_color_when_no_material=True)
    print('rooftops: exported', [obj.name for obj in objects], flush=True)


main()
