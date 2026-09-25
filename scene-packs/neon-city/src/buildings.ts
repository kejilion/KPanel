import * as THREE from 'three'
import { FOG_GLSL, HASH_GLSL, type AtmosphereUniforms } from './atmosphere'
import { standingOn, type Building } from './layout'

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
attribute vec4 aStyle;  // seed, neon (0 = none, else palette index + 1), warmth, street level
attribute vec4 aFacade; // style (FACADE in layout.ts), storey height, bay width, occupancy
attribute vec3 aTint;   // the wall's colour (the glass's, for a curtain wall)
varying vec3 vWorld;
varying vec3 vNormal;
varying vec3 vLocal;
varying vec3 vScale;
// Flat: interpolating even a constant value is not bit-exact on every GPU, and the window hashes
// turn the tiniest difference into a different pane, which showed as a grainy crawl in lit windows.
flat varying vec4 vStyle;
flat varying vec4 vFacade;
flat varying vec3 vTint;
void main() {
  vec4 world = modelMatrix * instanceMatrix * vec4(position, 1.0);
  vWorld = world.xyz;
  vNormal = normalize(mat3(modelMatrix) * mat3(instanceMatrix) * normal);
  vLocal = position;
  vScale = vec3(length(instanceMatrix[0].xyz), length(instanceMatrix[1].xyz), length(instanceMatrix[2].xyz));
  vStyle = aStyle;
  vFacade = aFacade;
  vTint = aTint;
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
uniform vec3 uSkyMid;
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
flat varying vec4 vFacade;
flat varying vec3 vTint;
${HASH_GLSL}
${FOG_GLSL}

// Facade styles (FACADE in layout.ts).
const float PUNCHED = 0.0;
const float CURTAIN = 1.0;
const float RIBBON = 2.0;
const float FINS = 3.0;
const float GRID = 4.0;
const float BLANK = 5.0;
const float LANTERN = 6.0;
bool isStyle(float style, float value) { return abs(style - value) < 0.5; }

// Offices show the open-plan office, meeting room or blinds; homes the rest.
const float OFFICE_ROOMS[4] = float[4](1.0, 4.0, 1.0, 5.0);
const float HOME_ROOMS[8] = float[8](0.0, 2.0, 3.0, 5.0, 6.0, 0.0, 2.0, 7.0);

vec3 homeTint(float h) {
  if (h < 0.55) return vec3(1.0, 0.6, 0.3);
  if (h < 0.85) return vec3(0.95, 0.88, 0.75);
  return vec3(0.5, 0.75, 1.0);
}

/** The sky (or, below the horizon, the dim city) mirrored in glass along direction r. */
vec3 mirroredSky(vec3 r) {
  vec3 above = mix(uHorizon, mix(uSkyMid, uSkyTop, smoothstep(0.15, 0.6, r.y)), smoothstep(0.0, 0.2, r.y));
  vec3 below = mix(uHorizon * 0.4, uFogColor * 0.5, smoothstep(0.0, -0.25, r.y));
  return mix(below, above, smoothstep(-0.03, 0.03, r.y));
}

/** Antialiased rectangle: 1 inside centre +- extent, in cell units. */
float box(vec2 f, vec2 centre, vec2 extent, vec2 fw) {
  vec2 lo = smoothstep(centre - extent - fw, centre - extent + fw, f);
  vec2 hi = 1.0 - smoothstep(centre + extent - fw, centre + extent + fw, f);
  return lo.x * lo.y * hi.x * hi.y;
}

/**
 * The room behind a pane. p: position on the opening, -1..1 across (to the viewer's right) and up;
 * r: the view ray, x and y in half-openings, z into the room in half-panes of 1.1 m. Returns the
 * room's colour where the ray meets its walls, floor, ceiling or far wall.
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
  float style = vFacade.x;
  float storey = vFacade.y;
  vec3 facade = vTint;
  bool curtain = isStyle(style, CURTAIN);
  bool office = style > 0.5 && style < 3.5;
  vec3 sky = mix(uHorizon * 0.6, uSkyTop, n.y * 0.5 + 0.5);
  float ao = mix(0.4, 1.0, smoothstep(0.0, 70.0, vWorld.y));
  // Dim by night: what light the walls get then comes from the street below and their windows.
  vec3 lighting = (0.012 + sky * uAmbient * 1.1) * ao;
  // Afterglow from the set sun on the faces turned towards it.
  lighting += uSunColor * max(dot(n, normalize(vec3(uSunDir.x, 0.0, uSunDir.z))), 0.0) * uSun * 0.35;
  vec3 view = normalize(vWorld - cameraPosition);
  vec3 mirrored = mirroredSky(reflect(view, n));
  float fresnel = 0.04 + 0.96 * pow(1.0 - abs(dot(view, n)), 5.0);
  vec3 color = facade * lighting;

  if (isStyle(style, LANTERN)) {
    // A glass crown lit from within, in the tower's neon colour or a warm white, steady (never
    // flashing), with dark mullions every couple of metres and the sky over it at a glancing angle.
    vec3 glow = vStyle.y > 0.5 ? uNeonColors[int(vStyle.y) - 1] * uNeon * 0.5 : vec3(1.0, 0.78, 0.52) * (0.12 + uLit * 1.4);
    float along = abs(n.z) > 0.5 ? vWorld.x : vWorld.z;
    float m = fract(along / 2.2);
    float mullion = 1.0 - smoothstep(0.03, 0.03 + max(fwidth(along / 2.2), 1e-4) * 1.5, min(m, 1.0 - m));
    float band = 1.0 - smoothstep(0.08, 0.08 + max(fwidth(vWorld.y / 3.0), 1e-4) * 1.5, fract(vWorld.y / 3.0));
    vec3 lit = glow * (0.75 + 0.25 * vLocal.y) * (1.0 - 0.8 * max(mullion, band) * step(abs(n.y), 0.5));
    gl_FragColor = vec4(applyFog(lit + mirrored * fresnel * 0.5, vWorld), 1.0);
    return;
  }

  if (abs(n.y) < 0.5) {
    bool faceZ = abs(n.z) > 0.5;
    float faceId = faceZ ? (n.z > 0.0 ? 1.0 : 2.0) : (n.x > 0.0 ? 3.0 : 4.0);
    // Along the face from its end (in +x or +z), and down from its top, in metres.
    float width = faceZ ? vScale.x : vScale.z;
    float u = ((faceZ ? vLocal.x : vLocal.z) + 0.5) * width;
    float edge = min(u, width - u);
    float top = (1.0 - vLocal.y) * vScale.y;
    float base = vWorld.y - vLocal.y * vScale.y;
    // Street buildings have a tall shop floor; the storeys start above it.
    float ground = vStyle.w > 0.5 ? 4.8 : 0.0;
    // A whole number of bays to each face, so the grid meets the corners cleanly.
    float bay = width / max(1.0, floor(width / vFacade.z + 0.5));
    vec2 g = vec2(u / bay, (vWorld.y - base - ground) / storey);
    vec2 cell = floor(g);
    vec2 f = fract(g);
    vec2 fw = max(fwidth(g), vec2(1e-4));
    float detail = 1.0 - smoothstep(0.12, 0.45, max(fw.x, fw.y));
    // Storeys that fit under the roof slab get windows; plant floors get none.
    float hasWindow = step(0.0, g.y) * step((cell.y + 1.0) * storey + ground, vScale.y - 0.3) * (isStyle(style, BLANK) ? 0.0 : 1.0);

    // The openings of each style, in cells: punched windows in the wall; a glass wall on thin
    // mullions with an opaque band at each floor; ribbons of glass; tall slots between piers;
    // square windows in a precast frame.
    vec2 centre = vec2(0.5, 0.55);
    vec2 halfPane = vec2(0.3, 0.3);
    if (isStyle(style, PUNCHED)) halfPane = vec2(mix(0.2, 0.3, fract(seed * 3.3)), mix(0.24, 0.3, fract(seed * 5.9)));
    else if (curtain) { halfPane = vec2(0.6, 0.41); centre.y = 0.59; }
    else if (isStyle(style, RIBBON)) halfPane = vec2(0.6, mix(0.24, 0.32, fract(seed * 3.3)));
    else if (isStyle(style, FINS)) { halfPane = vec2(mix(0.26, 0.34, fract(seed * 3.3)), 0.42); centre.y = 0.52; }
    float mullionWidth = curtain ? 0.035 : isStyle(style, RIBBON) ? 0.03 : 0.0;
    float mullion = mullionWidth > 0.0
      ? (1.0 - smoothstep(mullionWidth, mullionWidth + fw.x * 1.5, f.x)) + smoothstep(1.0 - mullionWidth - fw.x * 1.5, 1.0 - mullionWidth, f.x)
      : 0.0;
    float opening = box(f, centre, halfPane, fw) * hasWindow;
    float pane = opening * (1.0 - mullion);

    // The wall: floor slabs, pilasters, sills and frames, grime under the sills, a louvred plant
    // floor, a cornice under the roof. All of it fades out where it would be smaller than a pixel.
    float slab = 1.0 - smoothstep(0.07, 0.07 + fw.y * 1.5, f.y);
    float pilaster = (1.0 - smoothstep(0.04, 0.04 + fw.x * 1.5, f.x)) + smoothstep(0.96 - fw.x * 1.5, 0.96, f.x);
    float frame = box(f, centre, halfPane + 0.03, fw) * (1.0 - opening) * hasWindow;
    float sill = step(centre.x - halfPane.x - 0.01, f.x) * step(f.x, centre.x + halfPane.x + 0.01)
      * step(centre.y - halfPane.y - 0.05, f.y) * step(f.y, centre.y - halfPane.y - 0.005) * hasWindow;
    float streak = smoothstep(0.55, 1.0, hash12(vec2(floor(u * 1.3), seed * 91.0))) * (1.0 - smoothstep(0.0, 0.2, f.y)) * step(f.y, 0.2);
    float cornice = 1.0 - smoothstep(0.7, 0.7 + max(fwidth(top), 1e-4) * 1.5, top);
    float relief = 0.0;
    if (isStyle(style, PUNCHED)) relief = 0.3 * slab + 0.15 * pilaster + 0.5 * sill - 0.7 * frame - 0.3 * streak;
    else if (isStyle(style, GRID)) relief = 0.35 * box(f, centre, halfPane + 0.1, fw) * (1.0 - opening) * hasWindow - 0.5 * frame + 0.2 * slab - 0.25 * streak;
    else if (isStyle(style, FINS)) {
      // Deep piers: lit on one flank, in shadow on the other.
      float flank = smoothstep(centre.x + halfPane.x, centre.x + halfPane.x + 0.08, f.x) * (1.0 - step(0.96, f.x));
      relief = 0.25 - 0.45 * flank + 0.2 * slab;
    } else if (isStyle(style, RIBBON)) relief = 0.15 * slab - 0.4 * frame;
    else if (isStyle(style, BLANK)) {
      float louvre = smoothstep(0.55, 1.0, fract(vWorld.y / 0.3)) * step(1.2, top);
      float j = fract(u / 3.0);
      float joint = 1.0 - smoothstep(0.01, 0.01 + max(fwidth(u / 3.0), 1e-4) * 1.5, min(j, 1.0 - j));
      relief = -0.35 * louvre - 0.3 * joint;
    }
    vec3 cladding = facade * (1.0 + detail * relief);
    if (curtain) {
      // Between the panes: the opaque glass band at each floor, which mirrors the sky too, and the mullions.
      cladding = facade * 0.8;
      cladding = mix(cladding, vec3(0.2, 0.21, 0.22), mullion * detail);
    }
    cladding = mix(cladding, curtain || isStyle(style, BLANK) ? vec3(0.16, 0.17, 0.18) : facade * 1.25, cornice * detail);
    // Light on the walls: warm street light from below, fading up the facade, and the spill of a
    // lit window onto its frame, its sill and the wall round it (not through a glass wall).
    vec2 fromPane = max(abs(f - centre) - halfPane, 0.0) * vec2(bay, storey);

    // Which windows are lit. Flats: each window on its own slow schedule. Offices: a floor at a
    // time (a few desks stay lit on a dark floor), on a slower one. Both fade over a couple of
    // seconds, never snap.
    float h = hash13(vec3(cell, seed * 131.0 + faceId * 17.0));
    float occupancy = uLit * (0.3 + 1.6 * vFacade.w);
    float on;
    if (office) {
      float clock = uTime / 90.0 + hash12(vec2(cell.y, seed * 37.0)) * 13.0;
      float epoch = floor(clock);
      float was = step(hash12(vec2(cell.y * 1.7, epoch - 1.0 + seed * 11.0)), occupancy * 1.3);
      float now = step(hash12(vec2(cell.y * 1.7, epoch + seed * 11.0)), occupancy * 1.3);
      float floorOn = mix(was, now, smoothstep(0.0, 0.025, fract(clock)));
      on = mix(step(h, 0.06), step(h, 0.88), floorOn);
    } else {
      float clock = uTime / 41.0 + h * 13.0;
      float epoch = floor(clock);
      float was = step(hash13(vec3(cell * 1.31, epoch - 1.0 + seed * 7.0 + faceId)), occupancy);
      float now = step(hash13(vec3(cell * 1.31, epoch + seed * 7.0 + faceId)), occupancy);
      on = mix(was, now, smoothstep(0.0, 0.05, fract(clock)));
    }
    float spill = curtain ? 0.0 : on * hasWindow * exp(-length(fromPane) * 2.6) * (0.3 + 0.35 * fract(h * 29.1));
    vec3 wallLight = vec3(1.0, 0.62, 0.32) * uLamps * exp(-max(vWorld.y, 0.0) / 9.0) * 0.28
      + vec3(1.0, 0.85, 0.65) * spill * (0.4 + 0.6 * detail);
    color = cladding * (lighting + wallLight);
    if (curtain) color += mirrored * (0.05 + 0.5 * fresnel) * (1.0 - mullion * detail);

    // A lit window shows its room: follow the view ray into it (see room above). Behind a glass
    // wall or a ribbon window one room spans three bays, so a floor reads as open-plan offices.
    vec3 inward = -n;
    vec3 right = normalize(cross(inward, vec3(0.0, 1.0, 0.0)));
    float side = dot(right, faceZ ? vec3(1.0, 0.0, 0.0) : vec3(0.0, 0.0, 1.0));
    float span = curtain || isStyle(style, RIBBON) ? 3.0 : 1.0;
    vec2 roomCell = vec2(floor(cell.x / span), cell.y);
    vec2 across;
    vec2 opened;
    if (span > 1.5) {
      across.x = ((mod(cell.x, span) + f.x) / span - 0.5) * 2.0;
      opened.x = span * bay * 0.5;
    } else {
      across.x = (f.x - centre.x) / halfPane.x;
      opened.x = halfPane.x * bay;
    }
    across.x *= side;
    across.y = (f.y - centre.y) / halfPane.y;
    opened.y = halfPane.y * storey;
    vec3 ray = vec3(dot(view, right) / opened.x, view.y / opened.y, dot(view, inward) / 1.1);
    float pick = hash13(vec3(roomCell * 0.73, seed * 57.0 + faceId * 3.0));
    if (pick > 0.5) {
      across.x = -across.x;
      ray.x = -ray.x;
    }
    // Mip level from how many texels of the room picture (384 across) one pixel covers.
    float lod = log2(max(max(fw.x * bay / (2.0 * opened.x), fw.y * storey / (2.0 * opened.y)) * 384.0, 1.0));
    float index = office ? OFFICE_ROOMS[int(fract(pick * 7.13) * 4.0)] : HOME_ROOMS[int(fract(pick * 7.13) * 8.0)];
    vec3 interior = room(across, ray, index, lod);
    float bright = hash13(vec3(roomCell, seed * 23.0 + faceId));
    vec3 tint = office ? mix(vec3(0.85, 0.92, 1.0), vec3(1.0, 0.9, 0.76), bright) : homeTint(fract(h * 17.3 + vStyle.z));
    // Offices are lit evenly and not as brightly as a lamp-lit living room.
    vec3 light = interior * (office ? 0.7 + 0.4 * bright : 1.1 + 0.8 * bright) * mix(vec3(1.0), tint, 0.3);
    // Roll off the brightest lamps inside, so a room reads as a room rather than a white glare.
    light /= 1.0 + (office ? 0.6 : 0.3) * max(light.r, max(light.g, light.b));
    // An unlit pane: the dark room, and the sky mirrored in the glass (more at a glancing angle).
    vec3 glass = (curtain ? facade : vec3(0.02, 0.024, 0.03)) * lighting * 0.6 + vec3(0.003, 0.004, 0.007)
      + mirrored * mix(curtain ? 0.1 : 0.05, 1.0, fresnel);
    vec3 near = mix(color, mix(glass, light + mirrored * fresnel, on), pane);
    // Far away the grid is smaller than a pixel: show its average instead of shimmering.
    float share = hasWindow * min(1.0, 2.0 * halfPane.x) * min(1.0, 2.0 * halfPane.y);
    vec3 litAverage = office ? vec3(0.55, 0.56, 0.55) : vec3(0.66, 0.48, 0.3);
    vec3 average = mix(color, mix(glass, litAverage, clamp(occupancy * (office ? 1.1 : 1.0), 0.0, 1.0)), share);
    color = mix(near, average, smoothstep(0.3, 0.8, max(fw.x, fw.y)));

    // Shopfronts along the street.
    if (vStyle.w > 0.5) {
      float shop = hash13(vec3(floor(u / 7.5), seed * 19.0, faceId));
      float band = smoothstep(0.5, 0.9, vWorld.y) * (1.0 - smoothstep(3.7, 4.1, vWorld.y)) * step(0.8, edge);
      vec3 neonTint = uNeonColors[int(shop * 5.99)];
      // Mullions break each shopfront into panes.
      float shopMullion = smoothstep(0.04, 0.08, fract(u / 2.4)) * (1.0 - smoothstep(0.92, 0.96, fract(u / 2.4)));
      if (shop > 0.4 && band > 0.0) {
        // An open shop: its room seen through the glass (an office, kitchen or meeting room
        // picture, stretched to the width of the shopfront), lit warm or in its sign's colour.
        vec2 at = vec2((fract(u / 7.5) - 0.5) * 2.0 * side, (vWorld.y - 2.3) / 1.6);
        vec3 shopRay = vec3(dot(view, right) / 3.75, view.y / 1.6, dot(view, inward) / 1.6);
        float shopLod = log2(max(fwidth(u) / 7.5 * 384.0, 1.0));
        float k = floor(fract(shop * 13.7) * 3.0);
        vec3 inside = room(at, shopRay, k < 0.5 ? 1.0 : k < 1.5 ? 3.0 : 4.0, shopLod);
        vec3 shopTint = step(0.6, fract(shop * 7.0)) > 0.5 ? neonTint * 1.6 : vec3(1.0, 0.82, 0.62);
        vec3 shopLight = inside * shopTint;
        shopLight /= 1.0 + 0.45 * max(shopLight.r, max(shopLight.g, shopLight.b));
        color = mix(color, shopLight * mix(1.0, shopMullion, 0.85) * uNeon * 0.9, band);
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
    color = mix(facade, vec3(0.2), 0.5) * 0.3 * (0.05 + sky * uAmbient * 1.5);
  }
  gl_FragColor = vec4(applyFog(color, vWorld), 1.0);
}
`

/** The towers. The rooms map (uRooms) can be set once it has downloaded; the shader does not wait for it. */
export function createBuildings(buildings: readonly Building[], uniforms: AtmosphereUniforms, rooms: THREE.Texture | null): THREE.InstancedMesh<THREE.BoxGeometry, THREE.ShaderMaterial> {
  const geometry = new THREE.BoxGeometry(1, 1, 1)
  geometry.translate(0, 0.5, 0)
  const style = new Float32Array(buildings.length * 4)
  const facade = new Float32Array(buildings.length * 4)
  const tint = new Float32Array(buildings.length * 3)
  buildings.forEach((building, index) => {
    style.set([building.seed, building.neon, (building.seed * 13.7) % 1, building.street ? 1 : 0], index * 4)
    facade.set([building.facade.style, building.facade.floor, building.facade.bay, building.facade.occupancy], index * 4)
    tint.set(building.facade.tint, index * 3)
  })
  geometry.setAttribute('aStyle', new THREE.InstancedBufferAttribute(style, 4))
  geometry.setAttribute('aFacade', new THREE.InstancedBufferAttribute(facade, 4))
  geometry.setAttribute('aTint', new THREE.InstancedBufferAttribute(tint, 3))
  const material = new THREE.ShaderMaterial({
    uniforms: {
      uTime: uniforms.uTime,
      uLit: uniforms.uLit,
      uNeon: uniforms.uNeon,
      uLamps: uniforms.uLamps,
      uAmbient: uniforms.uAmbient,
      uSun: uniforms.uSun,
      uSkyTop: uniforms.uSkyTop,
      uSkyMid: uniforms.uSkyMid,
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

/** Red aviation lights on the highest point of everything tall enough to need one. */
export function aviationLights(buildings: readonly Building[]): THREE.Vector3[] {
  return buildings
    .filter((building) => building.y + building.h > 120 && building.w < 60 && !standingOn(buildings, building).length)
    .map((building) => new THREE.Vector3(building.x, building.y + building.h + 0.8, building.z))
}
