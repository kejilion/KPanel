# 深海遗迹 / Abyssal Ruins

An original underwater scene built in Blender and rendered interactively with
Three.js. `assets/scene.glb` contains the actual exported Blender architecture,
geometry and materials. The browser adds water fog, suspended particles, fish,
moving caustics and gentle camera movement. It does not replace the Blender
geometry with a screenshot or a procedural stand-in.

The complete license notice ships in `dist/license.txt`. Three.js 0.186.0 is
declared in the manifest. The Blender source and authoring script are kept in
`blender/`; those authoring files do not ship in the desktop resource pack.

## Rebuild the Blender assets

The authoring script was tested with Blender 5.2.1. From the repository root:

```sh
blender --background --python scene-packs/abyssal-ruins/blender/build_scene.py -- --render 0 1 2 --width 1664 --samples 96
```

This regenerates the geometry and PBR textures, exports `assets/scene.glb` and
`assets/cameras.json`, and saves the editable `blender/abyssal-ruins.blend` with
its textures packed inside. Omitting `--render 0 1 2` skips offline frame rendering
while still exporting the assets and saving the packed Blender file. The script
uses Cycles with an available OptiX GPU; when OptiX is unavailable it selects CPU
rendering.

`renders/camera-1.png` through `camera-3.png` are offline Blender reference frames.
Cycles volume scattering, lighting and tone mapping differ from the interactive
WebGL renderer, so these files do not represent browser output. The published
`poster.webp` and `thumb.webp` are captured from the actual WebGL scene; regenerate
them after browser visual review, then rebuild the pack and its catalog hashes.

## Build the browser pack

The pack uses the existing KPanel web dependencies (Vite 8.1.5, Three.js 0.186.0):

```sh
node scene-packs/abyssal-ruins/build.mjs
node scene-packs/abyssal-ruins/build.mjs --check
node web/node_modules/typescript/bin/tsc -p scene-packs/abyssal-ruins/tsconfig.json
```

An optional positional argument points to another compatible KPanel `web/`
directory when building this independent scene branch before host integration:

```sh
node scene-packs/abyssal-ruins/build.mjs /path/to/kpanel/web
```

`build.mjs` embeds all bytes of `assets/scene.glb` and the three poses from
`assets/cameras.json` into `scene.js`. `GLTFLoader.parseAsync` reads that in-memory
binary. The GLB must embed its resources: the build rejects external buffer or
image URIs. No network fetch is required to load the model, including inside an
opaque-origin `sandbox="allow-scripts"` iframe. glTF embedded images are decoded
using local blob URLs, so the serving CSP must permit `img-src blob:` if the model
contains textures.

Camera positions and look-at targets in `assets/cameras.json` are already in the
Three.js coordinate system. The Blender export uses `(x, y, z) -> (x, z, -y)`;
the runtime must not apply that transform a second time.

This is a custom-toolchain pack with entry `src/runtime.ts`, so the standard
repository scene builder preserves its published assets and license file. After
integration with the scene-pack host branch, rebuild its shared catalog normally:

```sh
npm --prefix web run scene-packs:build
node scripts/check-scene-packs.mjs
```

The standalone `catalog-entry.json` records exact published sizes and SHA-256
hashes. `--check` rebuilds into a temporary directory and compares all bytes.

## Preview behavior

Serve `dist/` over HTTP. `index.html` begins with a black frame and a quiet
6.5-second camera approach. The three viewpoints alternate after 27 seconds of
settled viewing. Manual camera transitions take 8–13 seconds and rise over the
ruined columns. `?entrance=off&shot=0`, `shot=1` and `shot=2` select immediate views
for visual review and artwork capture.

All cameras use the same teal/slate/antique-gold theme. Rendering pauses on host
pause commands or document hiding, and resumes without jumping simulation time.
Only parent-window messages using the existing `kpanel-desktop` source are
accepted. Camera indices are bounded to the three declared views. Context loss
and initialization errors are reported through the existing scene-pack protocol.
