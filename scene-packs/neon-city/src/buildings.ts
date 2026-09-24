import * as THREE from 'three'
import { FOG_GLSL, HASH_GLSL, type AtmosphereUniforms } from './atmosphere'
import type { Building } from './layout'

/**
 * The rooms behind the windows (rendered in Blender, see blender/rooms.py): each pane opens onto
 * a box the pane's size and ROOM_DEPTH half-panes deep, pictured from ROOM_CAMERA half-panes outside it.
 * The shader follows the view ray into the box and looks up where it lands in that picture, so
 * each room shows its far wall, floor, ceiling and furniture with the right parallax.
 */
// Rooms are rendered with their own lights; these even out their brightness (living room, office,
// bedroom, kitchen, meeting room, blinds, curtains, stairwell).
const ROOM_GAIN = [2.0, 0.45, 3.0, 1.1, 1.4, 3.0, 5.0, 1.2]

const VERTEX = /* glsl */ `
attribute vec4 aStyle; // seed, neon (0 = none, else palette index + 1), warmth, street level
varying vec3 vWorld;
varying vec3 vNormal;
varying vec3 vLocal;
varying vec3 vScale;
// Flat: interpolating even a constant value is not bit-exact on every GPU, and the window hashes
// turn the tiniest difference into a different pane, which showed as a grainy crawl in lit windows.
flat varying vec4 vStyle;
void main() {
  vec4 world = modelMatrix * instanceMatrix * vec4(position, 1.0);
  vWorld = world.xyz;
  vNormal = normalize(mat3(modelMatrix) * mat3(instanceMatrix) * normal);
  vLocal = position;
  vScale = vec3(length(instanceMatrix[0].xyz), length(instanceMatrix[1].xyz), length(instanceMatrix[2].xyz));
  vStyle = aStyle;
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const FRAGMENT = /* glsl */ `
uniform float uTime;
uniform float uLit;
uniform float uNeon;
uniform float uLamps;
uniform float uAmbient;
uniform float uSun;
uniform vec3 uSkyTop;
uniform vec3 uHorizon;
uniform vec3 uSunDir;
uniform vec3 uSunColor;
uniform vec3 uNeonColors[6];
uniform sampler2D uRooms;
uniform float uRoomGain[8];
varying vec3 vWorld;
varying vec3 vNormal;
varying vec3 vLocal;
varying vec3 vScale;
flat varying vec4 vStyle;
${HASH_GLSL}
${FOG_GLSL}

vec3 windowTint(float h) {
  if (h < 0.55) return vec3(1.0, 0.6, 0.3);
  if (h < 0.85) return vec3(0.95, 0.88, 0.75);
  return vec3(0.5, 0.75, 1.0);
}

/**
 * The room behind a pane. p: position on the pane, -1..1 across (to the viewer's right) and up;
 * r: the view ray in the same units, z into the room. Returns the room's colour where the ray
 * meets its walls, floor, ceiling or far wall.
 */
const float ROOM_DEPTH = 2.5455;  // 2.8 m, in half-panes of 1.1 m (blender/rooms.py)
const float ROOM_CAMERA = 1.2;    // 1.32 m
vec3 room(vec2 p, vec3 r, float index, float lod) {
  p = clamp(p, -0.999, 0.999);
  r.x = abs(r.x) < 1e-4 ? 1e-4 : r.x;
  r.y = abs(r.y) < 1e-4 ? 1e-4 : r.y;
  float tx = (sign(r.x) - p.x) / r.x;
  float ty = (sign(r.y) - p.y) / r.y;
  float tz = ROOM_DEPTH / max(r.z, 1e-4);
  float t = min(min(tx, ty), tz);
  vec3 hit = vec3(p, 0.0) + r * t;
  vec2 local = 0.5 + 0.5 * hit.xy * ROOM_CAMERA / (hit.z + ROOM_CAMERA);
  local = clamp(local, 0.01, 0.99);
  float column = mod(index, 4.0);
  float row = floor(index / 4.0);
  vec2 uv = vec2((column + local.x) / 4.0, 1.0 - (row + 1.0) / 2.0 + local.y / 2.0);
  return textureLod(uRooms, uv, lod).rgb * uRoomGain[int(index)];
}

void main() {
  vec3 n = normalize(vNormal);
  float seed = vStyle.x;
  // Each building its own cladding: concrete, dark glass, brick or metal panels.
  float kind = floor(fract(seed * 5.17) * 4.0);
  vec3 facade = kind < 0.5 ? vec3(0.3, 0.28, 0.25) : kind < 1.5 ? vec3(0.06, 0.07, 0.085) : kind < 2.5 ? vec3(0.27, 0.13, 0.08) : vec3(0.2, 0.21, 0.23);
  facade *= 0.75 + 0.5 * fract(seed * 7.31);
  vec3 sky = mix(uHorizon * 0.6, uSkyTop, n.y * 0.5 + 0.5);
  float ao = mix(0.4, 1.0, smoothstep(0.0, 70.0, vWorld.y));
  // Dim by night: what light the walls get then comes from the street below and their windows.
  vec3 lighting = (0.012 + sky * uAmbient * 1.1) * ao;
  // Afterglow from the set sun on the faces turned towards it.
  lighting += uSunColor * max(dot(n, normalize(vec3(uSunDir.x, 0.0, uSunDir.z))), 0.0) * uSun * 0.35;
  vec3 color = facade * lighting;

  if (abs(n.y) < 0.5) {
    bool faceZ = abs(n.z) > 0.5;
    float faceId = faceZ ? (n.z > 0.0 ? 1.0 : 2.0) : (n.x > 0.0 ? 3.0 : 4.0);
    float u = faceZ ? vWorld.x : vWorld.z;
    float edge = (0.5 - abs(faceZ ? vLocal.x : vLocal.z)) * (faceZ ? vScale.x : vScale.z);
    float top = (1.0 - vLocal.y) * vScale.y;

    // Windows: a grid of panes, each lit on its own slow schedule.
    vec2 g = vec2(u / 3.1, vWorld.y / 3.7);
    vec2 cell = floor(g);
    vec2 f = fract(g);
    vec2 fw = max(fwidth(g), vec2(1e-4));
    // The panes' size goes with the cladding: a near-continuous glass wall, ordinary windows in
    // concrete and metal, narrower ones in brick. halfPane is half the pane, in cells.
    vec2 halfPane = kind < 0.5 ? vec2(0.35, 0.3) : kind < 1.5 ? vec2(0.45, 0.37) : kind < 2.5 ? vec2(0.27, 0.3) : vec2(0.4, 0.3);
    vec2 paneLo = vec2(0.5, 0.52) - halfPane;
    vec2 paneHi = vec2(0.5, 0.52) + halfPane;
    vec2 lo = smoothstep(paneLo - fw, paneLo + fw, f);
    vec2 hi = 1.0 - smoothstep(paneHi - fw, paneHi + fw, f);
    float pane = lo.x * lo.y * hi.x * hi.y;
    pane *= step(1.3, edge) * step(1.2, top) * step(4.8, vWorld.y);
    float h = hash13(vec3(cell, seed * 131.0 + faceId * 17.0));
    float occupancy = uLit * (0.4 + 1.2 * fract(seed * 3.7));
    // Lights change on their own slow schedule and fade over a couple of seconds, never snap.
    float clock = uTime / 41.0 + h * 13.0;
    float epoch = floor(clock);
    float was = step(hash13(vec3(cell * 1.31, epoch - 1.0 + seed * 7.0 + faceId)), occupancy);
    float now = step(hash13(vec3(cell * 1.31, epoch + seed * 7.0 + faceId)), occupancy);
    float on = mix(was, now, smoothstep(0.0, 0.05, fract(clock)));

    // The facade itself: a floor slab at each storey, pilasters between the bays, a sill under
    // each pane and a dark frame round it, grime streaking down from the sills. All of it fades
    // out where it would be smaller than a pixel.
    float detail = 1.0 - smoothstep(0.12, 0.45, max(fw.x, fw.y));
    float hasWindow = step(1.3, edge) * step(1.2, top) * step(4.8, vWorld.y);
    float slab = 1.0 - smoothstep(0.07, 0.07 + fw.y * 1.5, f.y);
    float pilaster = (1.0 - smoothstep(0.04, 0.04 + fw.x * 1.5, f.x)) + smoothstep(0.96 - fw.x * 1.5, 0.96, f.x);
    // Sills under the panes, except on the glass wall.
    float isGlass = step(0.5, kind) * step(kind, 1.5);
    float sill = step(paneLo.x - 0.01, f.x) * step(f.x, paneHi.x + 0.01) * step(paneLo.y - 0.05, f.y) * step(f.y, paneLo.y - 0.005) * hasWindow * (1.0 - isGlass);
    float frame = step(paneLo.x - 0.025, f.x) * step(f.x, paneHi.x + 0.025) * step(paneLo.y - 0.025, f.y) * step(f.y, paneHi.y + 0.025) * (1.0 - pane) * hasWindow;
    float streak = smoothstep(0.55, 1.0, hash12(vec2(floor(u * 1.3), seed * 91.0))) * (1.0 - smoothstep(0.0, 0.2, f.y)) * step(f.y, 0.2);
    vec3 cladding = facade * (1.0 + detail * (0.3 * slab + 0.15 * pilaster + 0.5 * sill - 0.7 * frame - 0.3 * streak));
    // Light on the walls: warm street light from below, fading up the facade, and the spill of a
    // lit window onto its frame, its sill and the wall round it.
    vec2 fromPane = max(abs(f - vec2(0.5, 0.52)) - halfPane, 0.0) * vec2(3.1, 3.7);
    float spill = on * hasWindow * exp(-length(fromPane) * 2.6) * (0.3 + 0.35 * fract(h * 29.1));
    vec3 wallLight = vec3(1.0, 0.62, 0.32) * uLamps * exp(-max(vWorld.y, 0.0) / 9.0) * 0.28
      + vec3(1.0, 0.85, 0.65) * spill * (0.4 + 0.6 * detail);
    color = cladding * (lighting + wallLight);

    // A lit window shows its room: follow the view ray into it (see room above).
    vec3 inward = -n;
    vec3 right = normalize(cross(inward, vec3(0.0, 1.0, 0.0)));
    float side = dot(right, faceZ ? vec3(1.0, 0.0, 0.0) : vec3(0.0, 0.0, 1.0));
    vec2 across = vec2((f.x - 0.5) / halfPane.x * side, (f.y - 0.52) / halfPane.y);
    vec3 view = normalize(vWorld - cameraPosition);
    vec3 ray = vec3(dot(view, right) / (halfPane.x * 3.1), view.y / (halfPane.y * 3.7), dot(view, inward) / 1.1);
    float pick = hash13(vec3(cell * 0.73, seed * 57.0 + faceId * 3.0));
    if (pick > 0.5) {
      across.x = -across.x;
      ray.x = -ray.x;
    }
    // Mip level from how many texels of the room picture one pixel covers.
    float lod = log2(max(max(fw.x, fw.y) * 3.1 / 2.2 * 384.0, 1.0));
    vec3 interior = room(across, ray, floor(fract(pick * 7.13) * 8.0), lod);
    vec3 light = interior * (1.1 + 0.8 * fract(h * 29.1)) * mix(vec3(1.0), windowTint(fract(h * 17.3 + vStyle.z)), 0.3);
    // Roll off the brightest lamps inside, so a room reads as a room rather than a white glare.
    light /= 1.0 + 0.3 * max(light.r, max(light.g, light.b));
    // Dark glass still mirrors the glow of the city sky.
    vec3 glass = sky * 0.08 * uAmbient * 4.0 + uHorizon * 0.12 + vec3(0.003, 0.004, 0.007);
    // The pane reflects a little of the sky, more at a glancing angle.
    float fresnel = 0.04 + 0.5 * pow(1.0 - abs(ray.z) / length(ray), 5.0);
    vec3 near = mix(color, mix(glass, light + glass * fresnel, on), pane);
    // Far away the grid is smaller than a pixel: show its average instead of shimmering.
    vec3 average = mix(color, mix(glass, vec3(0.66, 0.48, 0.3), clamp(occupancy, 0.0, 1.0)), 0.3 * step(4.8, vWorld.y));
    color = mix(near, average, smoothstep(0.3, 0.8, max(fw.x, fw.y)));

    // Shopfronts along the street.
    if (vStyle.w > 0.5) {
      float shop = hash13(vec3(floor(u / 7.5), seed * 19.0, faceId));
      float band = smoothstep(0.5, 0.9, vWorld.y) * (1.0 - smoothstep(3.7, 4.1, vWorld.y)) * step(0.8, edge);
      vec3 tint = uNeonColors[int(shop * 5.99)];
      // Mullions break each shopfront into panes.
      float mullion = smoothstep(0.04, 0.08, fract(u / 2.4)) * (1.0 - smoothstep(0.92, 0.96, fract(u / 2.4)));
      if (shop > 0.4 && band > 0.0) {
        // An open shop: its room seen through the glass (an office, kitchen or meeting room
        // picture, stretched to the width of the shopfront), lit warm or in its sign's colour.
        vec2 at = vec2((fract(u / 7.5) - 0.5) * 2.0 * side, (vWorld.y - 2.3) / 1.6);
        vec3 shopRay = vec3(dot(view, right) / 3.75, view.y / 1.6, dot(view, inward) / 1.6);
        float shopLod = log2(max(fwidth(u) / 7.5 * 384.0, 1.0));
        float k = floor(fract(shop * 13.7) * 3.0);
        vec3 inside = room(at, shopRay, k < 0.5 ? 1.0 : k < 1.5 ? 3.0 : 4.0, shopLod);
        vec3 shopTint = step(0.6, fract(shop * 7.0)) > 0.5 ? tint * 1.6 : vec3(1.0, 0.82, 0.62);
        vec3 shopLight = inside * shopTint;
        shopLight /= 1.0 + 0.45 * max(shopLight.r, max(shopLight.g, shopLight.b));
        color = mix(color, shopLight * mix(1.0, mullion, 0.85) * uNeon * 0.9, band);
      }
    }

    // Landmark towers: neon on the vertical edges and rings around the crown.
    if (vStyle.y > 0.5) {
      vec3 neon = uNeonColors[int(vStyle.y) - 1] * uNeon;
      float ew = max(fwidth(edge), 0.02);
      float edgeLine = 1.0 - smoothstep(0.35, 0.35 + ew * 1.5, edge);
      float tw = max(fwidth(top), 0.02);
      float crown = (1.0 - smoothstep(0.5, 0.5 + tw * 1.5, abs(top - 2.5))) + (1.0 - smoothstep(0.35, 0.35 + tw * 1.5, abs(top - 9.0)));
      color += neon * (edgeLine * 2.2 + crown * 2.6);
    }
  } else if (n.y > 0.5) {
    color = facade * 0.3 * (0.05 + sky * uAmbient * 1.5);
  }
  gl_FragColor = vec4(applyFog(color, vWorld), 1.0);
}
`

export function createBuildings(buildings: readonly Building[], uniforms: AtmosphereUniforms, rooms: THREE.Texture): THREE.InstancedMesh {
  const geometry = new THREE.BoxGeometry(1, 1, 1)
  geometry.translate(0, 0.5, 0)
  const style = new Float32Array(buildings.length * 4)
  buildings.forEach((building, index) => {
    style.set([building.seed, building.neon, (building.seed * 13.7) % 1, building.street ? 1 : 0], index * 4)
  })
  geometry.setAttribute('aStyle', new THREE.InstancedBufferAttribute(style, 4))
  const material = new THREE.ShaderMaterial({
    uniforms: {
      uTime: uniforms.uTime,
      uLit: uniforms.uLit,
      uNeon: uniforms.uNeon,
      uLamps: uniforms.uLamps,
      uAmbient: uniforms.uAmbient,
      uSun: uniforms.uSun,
      uSkyTop: uniforms.uSkyTop,
      uHorizon: uniforms.uHorizon,
      uSunDir: uniforms.uSunDir,
      uSunColor: uniforms.uSunColor,
      uNeonColors: uniforms.uNeonColors,
      uFogColor: uniforms.uFogColor,
      uFogDensity: uniforms.uFogDensity,
      uRooms: { value: rooms },
      uRoomGain: { value: ROOM_GAIN },
    },
    vertexShader: VERTEX,
    fragmentShader: FRAGMENT,
  })
  const mesh = new THREE.InstancedMesh(geometry, material, buildings.length)
  const matrix = new THREE.Matrix4()
  const position = new THREE.Vector3()
  const scale = new THREE.Vector3()
  const rotation = new THREE.Quaternion()
  buildings.forEach((building, index) => {
    mesh.setMatrixAt(index, matrix.compose(position.set(building.x, building.y, building.z), rotation, scale.set(building.w, building.h, building.d)))
  })
  mesh.instanceMatrix.needsUpdate = true
  mesh.computeBoundingSphere()
  return mesh
}

/** Red aviation lights on everything tall enough to need one. */
export function aviationLights(buildings: readonly Building[]): THREE.Vector3[] {
  return buildings
    .filter((building) => building.y + building.h > 120 && building.w < 60)
    .map((building) => new THREE.Vector3(building.x, building.y + building.h + 0.8, building.z))
}
