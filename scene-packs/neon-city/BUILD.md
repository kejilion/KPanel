# Neon City: how the assets are made

The scene code in `src/` is built by the standard toolchain (`npm --prefix web run scene-packs:build`).
The files in `assets/` are generated, reproducibly, with Blender 5.2, and committed:

- **Rooms behind the windows** (`rooms.webp`): eight rooms (living room, open-plan office,
  bedroom, kitchen, meeting room, one behind blinds, one behind curtains, a stairwell), lit for
  the night and rendered with Cycles from a fixed camera by `blender/rooms.py`. The buildings'
  shader follows each view ray into the room behind its pane and looks up where it lands in these
  pictures (interior mapping), so every lit window shows a room with the right parallax.

  ```bash
  blender -b --factory-startup --python blender/rooms.py -- blender/out
  py -c "from PIL import Image; Image.open('blender/out/rooms.png').convert('RGB').save('assets/rooms.webp', 'WEBP', quality=90, method=6)"
  ```

- **Rooftop clutter** (`rooftops.glb`): air-conditioning units, cooling towers, water tanks, stair
  housings, masts, dishes and vents, modelled by `blender/rooftops.py` (vertex colours, no
  textures) and scattered over the roofs by the scene.

  ```bash
  blender -b --factory-startup --python blender/rooftops.py -- blender/out
  cp blender/out/rooftops.glb assets/
  ```
