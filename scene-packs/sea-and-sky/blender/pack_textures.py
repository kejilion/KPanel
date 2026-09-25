"""Encodes the maps baked by rocks.py as the WebP textures the scene loads.

    py blender/pack_textures.py <baked dir> <pack assets dir>

For each rock: rock-N-colour.webp (sRGB colour) and rock-N-surface.webp (the
object-space normal in RGB, ambient occlusion in alpha). Copies rocks.glb too.

The surface maps are kept to SURFACE_MAX: even the closest shot sees a stack at a
few hundred pixels, so the 2048 bake's top level is never sampled, and it is most
of the download.
"""
import os
import shutil
import sys

import numpy as np
from PIL import Image

source, target = sys.argv[1], sys.argv[2]
os.makedirs(target, exist_ok=True)
SURFACE_MAX = 1536


def to_srgb(linear):
    linear = np.clip(linear, 0.0, 1.0)
    return np.where(linear <= 0.0031308, linear * 12.92, 1.055 * np.power(linear, 1 / 2.4) - 0.055)


def byte(values):
    return np.clip(np.round(values * 255.0), 0, 255).astype(np.uint8)


index = 0
while os.path.exists(os.path.join(source, f'rock-{index}-colour.npy')):
    colour = np.load(os.path.join(source, f'rock-{index}-colour.npy'))
    normal = np.load(os.path.join(source, f'rock-{index}-normal.npy'))
    occlusion = np.load(os.path.join(source, f'rock-{index}-occlusion.npy'))
    Image.fromarray(byte(to_srgb(colour[:, :, :3])), 'RGB').save(os.path.join(target, f'rock-{index}-colour.webp'), 'WEBP', quality=86, method=6)
    normal_image = Image.fromarray(byte(normal[:, :, :3]), 'RGB')
    occlusion_image = Image.fromarray(byte(occlusion[:, :, 0]), 'L')
    if normal_image.width > SURFACE_MAX:
        # Resized apart: an RGBA resize premultiplies by alpha, which would spoil the normals in deep crevices.
        size = (SURFACE_MAX, SURFACE_MAX * normal_image.height // normal_image.width)
        normal_image = normal_image.resize(size, Image.LANCZOS)
        occlusion_image = occlusion_image.resize(size, Image.LANCZOS)
    surface = Image.merge('RGBA', (*normal_image.split(), occlusion_image))
    surface.save(os.path.join(target, f'rock-{index}-surface.webp'), 'WEBP', quality=86, alpha_quality=70, method=6)
    index += 1
shutil.copyfile(os.path.join(source, 'rocks.glb'), os.path.join(target, 'rocks.glb'))
print(f'packed {index} rocks into {target}')
