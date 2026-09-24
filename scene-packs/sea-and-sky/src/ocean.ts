import * as THREE from 'three'
import { NOISE_GLSL } from './noise'
import { LIGHTING_GLSL, type LightingUniforms, ROCK_SHADOW_GLSL } from './shading'
import { SKY_GLSL, type SkyUniforms } from './sky'
import { SKYLINE_GLSL } from './skyline'

/**
 * The sea. A long swell rolls through (Gerstner waves, calmer in the shallows),
 * fine ripples scroll over it, and together they break the sun into a field of
 * glints; at night the moon lays a silver path. It mirrors the sky, turns clear
 * turquoise over the shoals and deep blue offshore, foams around the rocks, and
 * falls into the stacks' shadow when the light is low.
 *
 * The surface is a disc of rings centred on the camera, a few tens of
 * centimetres apart close by and widening towards the horizon, so the swell is
 * finely sampled wherever it can be seen to move and never wobbles.
 */
const RINGS = 150
const SEGMENTS = 512
const INNER_RADIUS = 0.6
const OUTER_RADIUS = 16000

// Wave direction (towards the shore is +x), steepness and wavelength; the long ones also move the surface.
const WAVES_GLSL = /* glsl */ `
const vec4 WAVES[6] = vec4[6](
  vec4(1.0, 0.18, 0.04, 140.0),
  vec4(0.8, -0.6, 0.045, 83.0),
  vec4(0.7, 0.72, 0.05, 47.0),
  vec4(0.96, -0.28, 0.055, 29.0),
  vec4(0.35, 0.94, 0.05, 17.0),
  vec4(0.62, -0.78, 0.045, 10.5)
);
vec3 gerstner(vec4 wave, vec2 p, float scale, inout vec3 tangent, inout vec3 binormal) {
  float k = 6.2831853 / wave.w;
  float c = sqrt(9.8 / k);
  vec2 d = normalize(wave.xy);
  float f = k * (dot(d, p) - c * uTime);
  float s = wave.z * scale;
  float a = s / k;
  tangent += vec3(-d.x * d.x * s * sin(f), d.x * s * cos(f), -d.x * d.y * s * sin(f));
  binormal += vec3(-d.x * d.y * s * sin(f), d.y * s * cos(f), -d.y * d.y * s * sin(f));
  return vec3(d.x * a * cos(f), a * sin(f), d.y * a * cos(f));
}
`

const VERTEX = /* glsl */ `
uniform float uTime;
uniform sampler2D uHeight;
uniform vec4 uHeightRect;
varying vec3 vWorld;
varying float vCrest;
${WAVES_GLSL}
void main() {
  vec4 world = modelMatrix * vec4(position, 1.0);
  float depth = -texture2D(uHeight, (world.xz - uHeightRect.xy) / uHeightRect.zw).r;
  // Far out the swell is too small to see move; flatten it there, where the rings are wide apart.
  float scale = smoothstep(0.5, 9.0, depth) * (1.0 - smoothstep(1200.0, 2500.0, length(world.xz - cameraPosition.xz)));
  vec3 tangent = vec3(1.0, 0.0, 0.0);
  vec3 binormal = vec3(0.0, 0.0, 1.0);
  vec3 offset = gerstner(WAVES[0], world.xz, scale, tangent, binormal) + gerstner(WAVES[1], world.xz, scale, tangent, binormal);
  world.xyz += offset;
  vCrest = offset.y / 2.0;
  vWorld = world.xyz;
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const FRAGMENT = /* glsl */ `
uniform sampler2D uRipples;
uniform vec3 uSunDir;
uniform vec3 uMoonDir;
uniform vec3 uSkyTop;
uniform vec3 uGlow;
uniform float uTime;
uniform float uKeyVisible;
uniform float uMoonLight;
uniform mat3 uCelestial;
uniform float uCloudTime;
varying vec3 vWorld;
varying float vCrest;
${NOISE_GLSL}
${LIGHTING_GLSL}
${WAVES_GLSL}
${SKY_GLSL}
${SKYLINE_GLSL}
${ROCK_SHADOW_GLSL}
void main() {
  vec3 p = vWorld;
  float depth = max(-groundAt(p.xz), 0.0);
  float scale = smoothstep(0.5, 9.0, depth);
  float dist = length(p - cameraPosition);

  // Surface normal: every wave, analytically, then two layers of scrolling ripples.
  vec3 tangent = vec3(1.0, 0.0, 0.0);
  vec3 binormal = vec3(0.0, 0.0, 1.0);
  for (int i = 0; i < 6; i++) gerstner(WAVES[i], p.xz, scale, tangent, binormal);
  vec3 n = normalize(cross(binormal, tangent));
  vec2 r1 = texture2D(uRipples, p.xz / 23.0 + uTime * vec2(0.03, 0.011)).xy * 2.0 - 1.0;
  vec2 r2 = texture2D(uRipples, p.xz / 7.3 + uTime * vec2(-0.017, 0.041)).xy * 2.0 - 1.0;
  float detail = mix(1.0, 0.35, smoothstep(150.0, 3000.0, dist));
  n = normalize(n + vec3(r1.x * 0.32 + r2.x * 0.2, 0.0, r1.y * 0.32 + r2.y * 0.2) * detail);

  vec3 view = normalize(cameraPosition - p);
  vec3 reflected = reflect(-view, n);
  reflected.y = max(reflected.y, 0.02);
  reflected = normalize(reflected);
  float fresnel = 0.02 + 0.98 * pow(1.0 - max(dot(n, view), 0.0), 5.0);
  float shadow = min(keyShadow(p + vec3(0.0, 1.0, 0.0)), rockShadow(p + vec3(0.0, 0.5, 0.0)));
  float sunUp = max(uLightDir.y, 0.0);

  // The water itself: clear turquoise over sand, deep blue offshore.
  float clear = exp(-depth * 0.15);
  vec3 body = mix(vec3(0.004, 0.028, 0.058), vec3(0.04, 0.3, 0.33), clear);
  vec3 lighting = uAmbientTop * 1.15 + uLight * sunUp * 0.4 * shadow;
  body *= lighting;
  body = mix(body, vec3(0.8, 0.7, 0.52) * lighting * 0.8, exp(-depth * 0.8) * 0.55);
  // Light through the thin tops of the waves.
  float through = pow(max(dot(view, -uLightDir) * 0.5 + 0.5, 0.0), 3.0) * clamp(vCrest + 0.35, 0.0, 1.0);
  body += vec3(0.02, 0.22, 0.2) * uLight * through * 0.25 * shadow;

  vec3 color = mix(body, skyColor(reflected, true), fresnel);
  // At night the far city's lights trail faintly across the water, broken up by the waves.
  if (uNight > 0.01) {
    float trail = cityLights(reflected) * smoothstep(0.05, 0.02, reflected.y);
    color += vec3(1.0, 0.72, 0.42) * trail * uNight * fresnel * 0.4;
  }

  // Glints: the sun (or the moon) broken up by the ripples.
  float toLight = max(dot(reflected, uLightDir), 0.0);
  // Many small sharp glints and only a faint broad sheen, so the path sparkles instead of glaring.
  float glint = pow(toLight, 1400.0) * 9.0 + pow(toLight, 160.0) * 0.22;
  color += uLight * glint * shadow * mix(1.0, 0.8, uNight) * uKeyVisible;

  // Foam: breakers in the shallows, the swash at the waterline, whitecaps now and then.
  float foamNoise = snoise(vec3(p.xz * 0.11, uTime * 0.22)) * 0.5 + 0.5;
  float bands = sin(depth * 2.4 + uTime * 1.25 + snoise(vec3(p.xz * 0.04, uTime * 0.08)) * 2.2) * 0.5 + 0.5;
  float foam = smoothstep(3.2, 0.3, depth) * smoothstep(0.5, 0.85, bands * 0.6 + foamNoise * 0.55);
  // The swash against the rocks: broken, shifting lace rather than a solid ring.
  float lace = snoise(vec3(p.xz * 0.35, uTime * 0.5)) * 0.5 + 0.5;
  foam = max(foam, smoothstep(0.6, 0.0, depth) * smoothstep(0.3, 0.7, lace * 0.7 + foamNoise * 0.5));
  foam = max(foam, smoothstep(0.55, 0.95, vCrest) * smoothstep(0.62, 0.9, foamNoise) * 0.45);
  vec3 foamColor = vec3(0.92, 0.95, 0.96) * (uLight * sunUp * 0.5 * shadow + uAmbientTop * 0.95);
  color = mix(color, foamColor, clamp(foam, 0.0, 1.0));

  gl_FragColor = vec4(atmosphere(color, p), 1.0);
}
`

/** A tiling field of small ripples, stored as slopes, from waves that fit the tile exactly. */
function rippleTexture(random: () => number, size = 256): THREE.DataTexture {
  const waves = Array.from({ length: 48 }, () => {
    let kx = 0
    let kz = 0
    while (kx === 0 && kz === 0) {
      kx = Math.round((random() * 2 - 1) * 14)
      kz = Math.round((random() * 2 - 1) * 14)
    }
    return { kx, kz, amplitude: 1 / Math.pow(Math.hypot(kx, kz), 1.3), phase: random() * Math.PI * 2 }
  })
  const slopes = new Float32Array(size * size * 2)
  let max = 0
  for (let j = 0; j < size; j++) {
    for (let i = 0; i < size; i++) {
      let dx = 0
      let dz = 0
      for (const wave of waves) {
        const angle = (Math.PI * 2 * (wave.kx * i + wave.kz * j)) / size + wave.phase
        const c = Math.cos(angle) * wave.amplitude * Math.PI * 2
        dx += c * wave.kx
        dz += c * wave.kz
      }
      slopes[(j * size + i) * 2] = dx
      slopes[(j * size + i) * 2 + 1] = dz
      max = Math.max(max, Math.abs(dx), Math.abs(dz))
    }
  }
  const data = new Uint8Array(size * size * 4)
  for (let index = 0; index < size * size; index++) {
    data[index * 4] = Math.round((slopes[index * 2]! / max) * 127.5 + 127.5)
    data[index * 4 + 1] = Math.round((slopes[index * 2 + 1]! / max) * 127.5 + 127.5)
    data[index * 4 + 3] = 255
  }
  const texture = new THREE.DataTexture(data, size, size, THREE.RGBAFormat, THREE.UnsignedByteType)
  texture.wrapS = THREE.RepeatWrapping
  texture.wrapT = THREE.RepeatWrapping
  texture.minFilter = THREE.LinearMipmapLinearFilter
  texture.magFilter = THREE.LinearFilter
  texture.generateMipmaps = true
  texture.anisotropy = 8
  texture.needsUpdate = true
  return texture
}

export interface Ocean {
  mesh: THREE.Mesh
  follow(camera: THREE.Camera): void
}

/** A flat disc of rings, spaced in proportion to their radius, around a centre vertex. */
function ringGeometry(): THREE.BufferGeometry {
  const growth = Math.pow(OUTER_RADIUS / INNER_RADIUS, 1 / (RINGS - 1))
  const positions = new Float32Array((1 + RINGS * SEGMENTS) * 3)
  for (let ring = 0; ring < RINGS; ring++) {
    const radius = INNER_RADIUS * Math.pow(growth, ring)
    for (let segment = 0; segment < SEGMENTS; segment++) {
      const angle = (segment / SEGMENTS) * Math.PI * 2
      const index = 1 + ring * SEGMENTS + segment
      positions[index * 3] = Math.cos(angle) * radius
      positions[index * 3 + 2] = Math.sin(angle) * radius
    }
  }
  const indices: number[] = []
  for (let segment = 0; segment < SEGMENTS; segment++) indices.push(0, 1 + ((segment + 1) % SEGMENTS), 1 + segment)
  for (let ring = 0; ring < RINGS - 1; ring++) {
    for (let segment = 0; segment < SEGMENTS; segment++) {
      const a = 1 + ring * SEGMENTS + segment
      const b = 1 + ring * SEGMENTS + ((segment + 1) % SEGMENTS)
      const c = a + SEGMENTS
      const d = b + SEGMENTS
      indices.push(a, b, c, b, d, c)
    }
  }
  const geometry = new THREE.BufferGeometry()
  geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3))
  geometry.setIndex(indices)
  return geometry
}

export function createOcean(uniforms: LightingUniforms & SkyUniforms, random: () => number): Ocean {
  const geometry = ringGeometry()
  const mesh = new THREE.Mesh(geometry, new THREE.ShaderMaterial({
    uniforms: { ...uniforms, uRipples: { value: rippleTexture(random) } },
    vertexShader: VERTEX,
    fragmentShader: FRAGMENT,
  }))
  mesh.frustumCulled = false
  return {
    mesh,
    follow(camera) {
      // The waves live in world space; the rings just follow the camera to sample them.
      mesh.position.set(camera.position.x, 0, camera.position.z)
    },
  }
}
