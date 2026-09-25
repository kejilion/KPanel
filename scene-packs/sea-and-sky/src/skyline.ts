import * as THREE from 'three'
import { LIGHTING_GLSL, type LightingUniforms } from './shading'

/**
 * A far shore to the east, some six and a half kilometres out, with a city on
 * it; the west stays open sea for the sunsets. By day it is a pale silhouette
 * in the haze, at dawn a dark one against the sunrise, and at night its windows
 * light up (steadily: nothing flashes) and lay faint trails of light on the
 * water. Everything is drawn on one ring in the shader, from functions of the
 * azimuth, so the ocean can reflect the same lights.
 */
export const SKYLINE_RADIUS = 6500
/** Azimuth of the city centre, from north towards east (east-south-east). */
export const CITY_AZIMUTH = 2.0

// Azimuths are measured from north (-z) towards east (+x), in radians; s is metres along the shore.
export const SKYLINE_GLSL = /* glsl */ `
const float SKYLINE_RADIUS = ${SKYLINE_RADIUS.toFixed(1)};
const float SHORE_FROM = 0.96;   // 55 degrees: north-east
const float SHORE_TO = 3.49;     // 200 degrees: just west of south
const float CITY_AT = ${CITY_AZIMUTH.toFixed(3)};
const float CITY_WIDTH = 0.21;   // about 12 degrees either side

float shoreHash(float n) {
  return fract(sin(n * 127.1) * 43758.5453);
}
float shoreNoise(float x) {
  float i = floor(x);
  float f = x - i;
  return mix(shoreHash(i), shoreHash(i + 1.0), f * f * (3.0 - 2.0 * f));
}
float shoreAzimuth(vec3 d) {
  float az = atan(d.x, -d.z);
  return az < 0.0 ? az + 6.2831853 : az;
}
/** 1 on the far shore, fading out at its ends. */
float shoreMask(float az) {
  return smoothstep(SHORE_FROM, SHORE_FROM + 0.12, az) * smoothstep(SHORE_TO, SHORE_TO - 0.12, az);
}
/** How built up the shore is: a city, and a couple of towns along the coast. */
float cityDensity(float az) {
  float city = exp(-pow((az - CITY_AT) / CITY_WIDTH, 2.0));
  float towns = 0.35 * exp(-pow((az - 1.35) / 0.05, 2.0)) + 0.3 * exp(-pow((az - 2.85) / 0.06, 2.0));
  return (city + towns) * shoreMask(az);
}
/** Hills behind the shore, in metres; the city sits on a low, flat stretch of it. */
float hillHeight(float s, float az) {
  float rolling = 1.0 - 0.9 * min(cityDensity(az) * 1.6, 1.0);
  return shoreMask(az) * (6.0 + (40.0 * shoreNoise(s * 0.0011) + 20.0 * shoreNoise(s * 0.0047 + 9.0)) * rolling);
}
/** One row of blocks along the shore: each cell holds a building of some width, or a gap. */
float blocks(float s, float cell, float fill, float low, float high, float seed, out float id) {
  id = floor(s / cell) + seed * 7919.0;
  float at = fract(s / cell);
  float width = 0.55 + 0.4 * shoreHash(id + 3.0);
  float offset = (1.0 - width) * shoreHash(id + 9.0);
  float inside = step(offset, at) * step(at, offset + width);
  return inside * step(shoreHash(id + 7.0), fill) * mix(low, high, pow(shoreHash(id), 2.2));
}
/**
 * Building height at s, and the id of the block it belongs to: a dense mass of low buildings,
 * mid-rises thinning out from the centre, a cluster of towers downtown and one landmark tower.
 */
float buildingHeight(float s, float az, out float block) {
  float density = cityDensity(az);
  block = 0.0;
  if (density < 0.03) return 0.0;
  float lowId;
  float midId;
  float towerId;
  float low = blocks(s, 26.0, 0.95, 10.0, 32.0, 1.0, lowId) * smoothstep(0.03, 0.25, density);
  float mid = blocks(s, 60.0, 0.85 * density, 30.0, 75.0, 2.0, midId) * (0.6 + 0.4 * density);
  float tower = blocks(s, 105.0, 0.75 * smoothstep(0.4, 0.9, density), 75.0, 175.0, 3.0, towerId);
  float spire = step(abs(az - CITY_AT) * SKYLINE_RADIUS, 22.0) * 250.0;
  float height = low;
  block = lowId;
  if (mid > height) { height = mid; block = midId; }
  if (tower > height) { height = tower; block = towerId; }
  if (spire > height) { height = spire; block = 1.0; }
  return height;
}
/** Brightness of the city's lights seen in direction d, for their reflections on the sea at night. */
float cityLights(vec3 d) {
  float az = shoreAzimuth(d);
  float block = floor(az * SKYLINE_RADIUS / 60.0);
  return cityDensity(az) * (0.4 + 0.6 * shoreHash(block + 11.0));
}
`

const VERTEX = /* glsl */ `
varying vec3 vWorld;
void main() {
  vec4 world = modelMatrix * vec4(position, 1.0);
  vWorld = world.xyz;
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const FRAGMENT = /* glsl */ `
varying vec3 vWorld;
${LIGHTING_GLSL}
${SKYLINE_GLSL}
void main() {
  float az = shoreAzimuth(vWorld);
  float s = az * SKYLINE_RADIUS;
  float y = vWorld.y;
  float block;
  float building = buildingHeight(s, az, block);
  float hill = hillHeight(s, az);
  if (y > max(building, hill) || shoreMask(az) < 0.01) discard;
  bool isBuilding = y > hill || building > hill;
  // Facades face back across the water, towards the camera: lit when the light is behind it.
  vec3 facing = normalize(vec3(-vWorld.x, 0.0, -vWorld.z));
  float lit = max(dot(facing, uLightDir), 0.0);
  vec3 albedo = isBuilding ? mix(vec3(0.2, 0.23, 0.27), vec3(0.42, 0.43, 0.44), shoreHash(block + 5.0)) : vec3(0.12, 0.15, 0.12);
  // By night the towers are dark shapes: their windows, not the moonlight, should carry them.
  vec3 color = albedo * (uAmbientTop * mix(1.1, 0.6, uNight) + uLight * lit * mix(0.55, 0.08, uNight));
  color = atmosphere(color, vWorld);
  // Windows: lit or dark, fixed for the night. Far away a window is far below a pixel, so the grid
  // is coarsened in powers of two until each cell (a group of windows) covers a couple of pixels:
  // a stable speckle of lit windows that never shimmers as the camera moves.
  float lights = 0.0;
  if (uNight > 0.01) {
    if (isBuilding) {
      float metresPerPixel = max(fwidth(y), fwidth(s));
      float level = max(0.0, ceil(log2(2.2 * metresPerPixel / 4.0)));
      float size = 4.0 * exp2(level);
      vec2 cell = floor(vec2(s, y) / size);
      float share = 0.2 + 0.45 * shoreHash(block + 11.0);
      float roll = shoreHash(cell.x * 13.1 + cell.y * 71.7 + level * 5.3);
      lights = step(roll, share) * (0.45 + 0.8 * fract(roll * 37.0));
    } else {
      // A few lights in the villages and along the coast road.
      lights = step(0.985, shoreHash(floor(s / 14.0) * 3.7 + floor(y / 8.0))) * 0.8;
    }
  }
  vec3 glow = mix(vec3(1.0, 0.68, 0.36), vec3(0.9, 0.9, 0.95), step(0.8, shoreHash(block + 2.0)));
  color += glow * lights * uNight * 1.4;
  gl_FragColor = vec4(color, 1.0);
}
`

export function createSkyline(uniforms: LightingUniforms): THREE.Mesh {
  const geometry = new THREE.CylinderGeometry(SKYLINE_RADIUS, SKYLINE_RADIUS, 480, 720, 1, true)
  geometry.translate(0, 240 - 2, 0)
  const mesh = new THREE.Mesh(geometry, new THREE.ShaderMaterial({
    uniforms: { ...uniforms },
    vertexShader: VERTEX,
    fragmentShader: FRAGMENT,
    side: THREE.BackSide,
  }))
  mesh.frustumCulled = false
  return mesh
}
