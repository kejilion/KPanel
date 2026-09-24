import * as THREE from 'three'
import type { Daylight } from './daylight'
import { groundHeight, rockBase } from './world'

/**
 * Lighting shared by the rocks and the sea: the key light (sun or moon) with
 * soft shadows traced over a height map of the stacks (so they throw long
 * shadows on the water at sunset), sky light from above, bounce from below, and
 * haze with distance. The same height map gives the sea its depth for colour
 * and foam; beyond it the sea is simply deep.
 */
export const HEIGHT_RECT = new THREE.Vector4(-320, -320, 640, 640) // min x, min z, size x, size z

export const LIGHTING_GLSL = /* glsl */ `
uniform vec3 uLightDir;
uniform vec3 uLight;
uniform vec3 uAmbientTop;
uniform vec3 uAmbientBottom;
uniform vec3 uSkyHorizon;
uniform float uNight;
uniform sampler2D uHeight;
uniform vec4 uHeightRect;
float groundAt(vec2 xz) {
  return texture2D(uHeight, (xz - uHeightRect.xy) / uHeightRect.zw).r;
}
float keyShadow(vec3 p) {
  if (uLightDir.y < 0.005) return 0.0;
  float lit = 1.0;
  float t = 2.5;
  for (int i = 0; i < 22; i++) {
    vec3 q = p + uLightDir * t;
    if (q.y > 330.0) break;
    lit = min(lit, clamp((q.y - groundAt(q.xz)) / (0.05 * t) + 0.5, 0.0, 1.0));
    t *= 1.3;
  }
  return lit;
}
vec3 shade(vec3 albedo, vec3 n, float wrap, float shadow) {
  float light = max((dot(n, uLightDir) + wrap) / (1.0 + wrap), 0.0) * shadow;
  vec3 ambient = mix(uAmbientBottom, uAmbientTop, n.y * 0.5 + 0.5);
  return albedo * (uLight * light * 0.62 + ambient * 0.9);
}
vec3 atmosphere(vec3 color, vec3 world) {
  float dist = length(world - cameraPosition);
  float haze = (1.0 - exp(-dist * 0.00021)) * mix(1.0, 0.65, smoothstep(0.0, 300.0, world.y));
  return mix(color, uSkyHorizon * 0.9, clamp(haze, 0.0, 1.0) * 0.8);
}
`

/** Ground and rock heights on a grid over HEIGHT_RECT, as a filterable half-float texture. */
export function heightTexture(width = 320, height = 320): THREE.DataTexture {
  const data = new Uint16Array(width * height)
  for (let j = 0; j < height; j++) {
    const z = HEIGHT_RECT.y + ((j + 0.5) / height) * HEIGHT_RECT.w
    for (let i = 0; i < width; i++) {
      const x = HEIGHT_RECT.x + ((i + 0.5) / width) * HEIGHT_RECT.z
      data[j * width + i] = THREE.DataUtils.toHalfFloat(Math.max(groundHeight(x, z), rockBase(x, z)))
    }
  }
  const texture = new THREE.DataTexture(data, width, height, THREE.RedFormat, THREE.HalfFloatType)
  texture.minFilter = THREE.LinearFilter
  texture.magFilter = THREE.LinearFilter
  texture.wrapS = THREE.ClampToEdgeWrapping
  texture.wrapT = THREE.ClampToEdgeWrapping
  texture.needsUpdate = true
  return texture
}

export function createLighting(daylight: Daylight) {
  const uniforms = {
    uLightDir: { value: daylight.lightDir },
    uLight: { value: daylight.light },
    uAmbientTop: { value: daylight.ambientTop },
    uAmbientBottom: { value: daylight.ambientBottom },
    uSkyHorizon: { value: daylight.skyHorizon },
    uNight: { value: 0 },
    uTime: { value: 0 },
    uHeight: { value: heightTexture() },
    uHeightRect: { value: HEIGHT_RECT },
  }
  return {
    uniforms,
    update(time: number): void {
      uniforms.uNight.value = daylight.night
      uniforms.uTime.value = time
    },
  }
}

export type LightingUniforms = ReturnType<typeof createLighting>['uniforms']
