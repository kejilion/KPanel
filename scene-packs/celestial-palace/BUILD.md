# 云海天宫 / Palace Above the Clouds

An original procedural Three.js scene. All geometry, materials and animation are
generated from the readable TypeScript source; no downloaded models, textures,
fonts or network requests are used at runtime. Scene code and generated artwork
are MIT licensed. The bundled Three.js dependency is MIT licensed and declared
in `manifest.json`; the complete license notice ships as `dist/license.txt`.

## Build

Build this package, then refresh the shared catalog once the KPanel scene-pack
host is available:

```sh
node scene-packs/celestial-palace/build.mjs
npm --prefix web run scene-packs:build
node scripts/check-scene-packs.mjs
```

This independent scene branch can also build with an existing KPanel `web/`
toolchain (Vite 8.1.5, Three.js 0.186.0). The optional argument is the path to
that `web` directory; it defaults to the current repository's `web/`:

```sh
node scene-packs/celestial-palace/build.mjs /path/to/kpanel/web
node scene-packs/celestial-palace/build.mjs --check /path/to/kpanel/web
node web/node_modules/typescript/bin/tsc -p scene-packs/celestial-palace/tsconfig.json
```

`catalog-entry.json` is an exact size/SHA-256 record of the standalone package.
When integrating into the scene repository, run its standard builder to merge
this entry into the shared `scene-packs/catalog.json`. This is a custom-toolchain
pack (entry `src/palace.ts`), so the repository builder preserves its published
files, including license notices. The `--check` command rebuilds in a temporary
directory and checks every published byte and catalog hash.

Serve `dist/` over HTTP. `index.html` plays the entrance and cycles cameras every
29 seconds. `?entrance=off&shot=0`, `shot=1` and `shot=2` select an immediate shot
for artwork capture. Normal camera changes blend over 5.5 seconds.

The three views share one violet/slate/gold theme. Rendering pauses on parent
pause commands or document hiding and resumes without a time jump. Only parent
window messages are accepted; camera indices are bounded to the three shots.
