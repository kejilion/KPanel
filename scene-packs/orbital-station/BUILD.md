# Orbital Station: how the assets are made

The scene code in `src/` is built by the standard toolchain (`npm --prefix web run scene-packs:build`).
The files in `assets/` are generated, reproducibly, and committed:

- **The station** (`station.gltf`, `station.bin`, `station-textures/`): modelled by Blender 5.2 from `blender/station.py`: modules wrapped
  in gold and silver insulation, a hub of painted panels, a habitat ring of segmented modules with rows
  of windows, a docking ring, ribbed radiators, a dish, a lattice truss and four solar wings, with PBR
  materials and generated textures (WebP files beside the model, not embedded: embedded images load
  through blob: URLs, which the scene sandbox does not allow). The habitat, dock and wings are separate nodes
  the scene turns.

  ```bash
  blender -b --factory-startup --python blender/station.py -- blender/out
  cp -r blender/out/station.gltf blender/out/station.bin blender/out/station-textures assets/
  ```

- **The planet** (`planet-surface.webp`, `planet-relief.webp`, `planet-masks.webp`,
  `planet-clouds.webp`): painted at 4096 x 2048 by `tools/planet.py` (numpy and Pillow; about five
  minutes): continents with mountain ranges, biomes by latitude, height and rainfall, ice caps, the
  relief as an object-space normal map, water and city-light masks, and the cloud cover.

  ```bash
  py tools/planet.py assets 4096
  ```
