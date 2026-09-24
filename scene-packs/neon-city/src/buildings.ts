import * as THREE from 'three'
import { FOG_GLSL, HASH_GLSL, type AtmosphereUniforms } from './atmosphere'
import type { Building } from './layout'

const VERTEX = /* glsl */ `
attribute vec4 aStyle; // seed, neon (0 = none, else palette index + 1), warmth, street level
varying vec3 vWorld;
varying vec3 vNormal;
varying vec3 vLocal;
varying vec3 vScale;
varying vec4 vStyle;
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
uniform float uAmbient;
uniform float uSun;
uniform vec3 uSkyTop;
uniform vec3 uHorizon;
uniform vec3 uSunDir;
uniform vec3 uSunColor;
uniform vec3 uNeonColors[6];
varying vec3 vWorld;
varying vec3 vNormal;
varying vec3 vLocal;
varying vec3 vScale;
varying vec4 vStyle;
${HASH_GLSL}
${FOG_GLSL}

vec3 windowTint(float h) {
  if (h < 0.55) return vec3(1.0, 0.6, 0.3);
  if (h < 0.85) return vec3(0.95, 0.88, 0.75);
  return vec3(0.5, 0.75, 1.0);
}

void main() {
  vec3 n = normalize(vNormal);
  float seed = vStyle.x;
  vec3 facade = vec3(0.02, 0.022, 0.03) * (0.7 + 0.6 * fract(seed * 7.31));
  vec3 sky = mix(uHorizon * 0.6, uSkyTop, n.y * 0.5 + 0.5);
  float ao = mix(0.4, 1.0, smoothstep(0.0, 70.0, vWorld.y));
  vec3 color = facade * (0.15 + sky * uAmbient * 6.0) * ao;
  // Afterglow from the set sun on the faces turned towards it.
  color += facade * uSunColor * max(dot(n, normalize(vec3(uSunDir.x, 0.0, uSunDir.z))), 0.0) * uSun * 5.0;

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
    vec2 lo = smoothstep(vec2(0.15, 0.22) - fw, vec2(0.15, 0.22) + fw, f);
    vec2 hi = 1.0 - smoothstep(vec2(0.85, 0.82) - fw, vec2(0.85, 0.82) + fw, f);
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
    vec3 light = windowTint(fract(h * 17.3 + vStyle.z)) * (0.35 + 0.8 * fract(h * 29.1));
    vec3 glass = sky * 0.08 * uAmbient * 4.0 + vec3(0.003, 0.004, 0.007);
    vec3 near = mix(color, mix(glass, light, on), pane);
    // Far away the grid is smaller than a pixel: show its average instead of shimmering.
    vec3 average = mix(color, mix(glass, windowTint(0.3) * 0.75, clamp(occupancy, 0.0, 1.0)), 0.3 * step(4.8, vWorld.y));
    color = mix(near, average, smoothstep(0.3, 0.8, max(fw.x, fw.y)));

    // Shopfronts along the street.
    if (vStyle.w > 0.5) {
      float shop = hash13(vec3(floor(u / 7.5), seed * 19.0, faceId));
      float band = smoothstep(0.5, 0.9, vWorld.y) * (1.0 - smoothstep(3.7, 4.1, vWorld.y)) * step(0.8, edge);
      vec3 tint = uNeonColors[int(shop * 5.99)];
      vec3 shopLight = shop > 0.4 ? mix(vec3(1.0, 0.72, 0.45) * 0.55, tint, step(0.6, fract(shop * 7.0))) : vec3(0.0);
      // Mullions break each shopfront into panes.
      float mullion = smoothstep(0.04, 0.08, fract(u / 2.4)) * (1.0 - smoothstep(0.92, 0.96, fract(u / 2.4)));
      color += shopLight * band * mix(1.0, mullion, 0.8) * uNeon * 0.32;
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
    color = facade * 0.5 * (0.15 + sky * uAmbient * 5.0);
  }
  gl_FragColor = vec4(applyFog(color, vWorld), 1.0);
}
`

export function createBuildings(buildings: readonly Building[], uniforms: AtmosphereUniforms): THREE.InstancedMesh {
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
      uAmbient: uniforms.uAmbient,
      uSun: uniforms.uSun,
      uSkyTop: uniforms.uSkyTop,
      uHorizon: uniforms.uHorizon,
      uSunDir: uniforms.uSunDir,
      uSunColor: uniforms.uSunColor,
      uNeonColors: uniforms.uNeonColors,
      uFogColor: uniforms.uFogColor,
      uFogDensity: uniforms.uFogDensity,
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
