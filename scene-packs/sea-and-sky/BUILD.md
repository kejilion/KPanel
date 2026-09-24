# Sea and Sky: how the assets are made

The scene code in `src/` is built by the standard toolchain (`npm --prefix web run scene-packs:build`).
The files in `assets/` are generated, reproducibly, and committed:

- **Sea stacks** (`rocks.glb`, `rock-N-colour.webp`, `rock-N-surface.webp`): sculpted and baked by
  Blender 5.2 from the rock list in `src/rocks.json` (the same list the scene places them from).

  ```bash
  blender -b --factory-startup --python blender/rocks.py -- blender/out
  py blender/pack_textures.py blender/out assets
  ```

  `rocks.py` builds a detailed stack per rock (about a million points for the big ones) and a light
  mesh of the same shape, and bakes colour, an object-space normal map and ambient occlusion from one
  to the other with Cycles. `pack_textures.py` encodes them as WebP: colour, and normal plus occlusion
  in one RGBA texture.

- **Cloud noise** (`cloud-shape.bin`, `cloud-detail.bin`): tileable 3D noise volumes the volumetric
  clouds are marched through.

  ```bash
  node tools/cloud-noise.mjs
  ```
