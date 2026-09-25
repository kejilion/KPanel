"""Encodes the maps baked by rocks.py as the WebP textures the scene loads.

    py blender/pack_textures.py <baked dir> <pack assets dir>

For each rock: rock-N-colour.webp (sRGB colour) and rock-N-surface.webp (the
object-space normal in RGB, ambient occlusion in alpha). Copies rocks.glb too.
"""
import os
import shutil
import sys

import numpy as np
from PIL import Image

source, target = sys.argv[1], sys.argv[2]
os.makedirs(target, exist_ok=True)


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
    surface = np.concatenate([normal[:, :, :3], occlusion[:, :, :1]], axis=2)
    Image.fromarray(byte(surface), 'RGBA').save(os.path.join(target, f'rock-{index}-surface.webp'), 'WEBP', quality=86, alpha_quality=70, method=6)
    index += 1
shutil.copyfile(os.path.join(source, 'rocks.glb'), os.path.join(target, 'rocks.glb'))
print(f'packed {index} rocks into {target}')
