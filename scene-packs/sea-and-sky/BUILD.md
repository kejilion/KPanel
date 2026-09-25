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
  to the other with Cycles, and exports the light meshes Draco-compressed. `pack_textures.py` encodes the
  maps as WebP: colour, and normal plus occlusion in one RGBA texture (at most 1536 pixels square: no
  shot comes near enough to sample more).

- **Draco decoder** (`draco/draco_wasm_wrapper.js`, `draco/draco_decoder.wasm`): Draco 1.5.6
  (Apache-2.0), the glTF build that ships with three.js.

  ```bash
  cp web/node_modules/three/examples/jsm/libs/draco/gltf/draco_wasm_wrapper.js \
     web/node_modules/three/examples/jsm/libs/draco/gltf/draco_decoder.wasm scene-packs/sea-and-sky/assets/draco/
  ```

- **Cloud noise** (`cloud-shape.bin`, `cloud-detail.bin`): tileable 3D noise volumes the volumetric
  clouds are marched through.

  ```bash
  node tools/cloud-noise.mjs
  ```
