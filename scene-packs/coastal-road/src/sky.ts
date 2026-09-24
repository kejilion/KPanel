import * as THREE from 'three'
import type { Daylight } from './daylight'
import { NOISE_GLSL } from './noise'

/**
 * The sky, as one GLSL function used twice: for the sky dome and for the
 * ocean's reflections, so the sea always mirrors the same sky. A gradient that
 * follows the sun, a soft sun disc and glow, a layer of drifting clouds lit by
 * the sun (glowing orange at sunset), stars at night, and the moon in its real
 * phase: lit on the side facing the sun, a pale disc when it is up by day.
 */
export const SKY_UNIFORMS_GLSL = /* glsl */ `
uniform vec3 uSkyTop;
uniform vec3 uSkyHorizon;
uniform vec3 uSunDir;
uniform vec3 uMoonDir;
uniform vec3 uLightDir;
uniform vec3 uLight;
uniform vec3 uAmbientTop;
uniform vec3 uGlow;
uniform float uNight;
uniform float uTime;
uniform float uMoonLight;
`

export const SKY_GLSL = /* glsl */ `
float skyHash(vec2 p) {
  vec3 p3 = fract(vec3(p.xyx) * 0.1031);
  p3 += dot(p3, p3.yzx + 33.33);
  return fract((p3.x + p3.y) * p3.z);
}

// A cloud layer 1600 m up: cover, and how much sunlight reaches the cloud's underside.
vec2 cloudLayer(vec3 d, bool cheap) {
  if (d.y < 0.015) return vec2(0.0);
  vec2 p = d.xz / d.y * 1600.0 + vec2(uTime * 5.0, uTime * 2.0);
  vec3 q = vec3(p * 0.00032, uTime * 0.003);
  float n = cheap ? fbm3(q) : fbm(q);
  float cover = smoothstep(0.04, 0.45, n + 0.07) * smoothstep(0.015, 0.14, d.y);
  // Thinner towards the sun = brighter; a cheap stand-in for light through the cloud.
  float towards = fbm3(q + vec3(uSunDir.xz * 0.12, 0.0));
  float lit = clamp(0.62 + (n - towards) * 2.2, 0.18, 1.25);
  return vec2(cover, lit);
}

vec3 skyColor(vec3 d, bool cheap) {
  float h = max(d.y, 0.0);
  vec3 color = mix(uSkyHorizon, uSkyTop, pow(h, 0.42));
  float toSun = max(dot(d, uSunDir), 0.0);
  color += uGlow * (pow(toSun, 5.0) * 0.45 + pow(toSun, 40.0) * 0.9);
  // A soft sun disc: bright, but not blinding.
  color += uLight * smoothstep(0.99962, 0.99988, toSun) * 1.6 * (1.0 - uNight);
  // Stars and moon.
  vec2 cell = floor(vec2(atan(d.z, d.x) * 320.0, d.y * 320.0));
  float star = step(0.9968, skyHash(cell)) * (0.7 + 0.3 * sin(uTime * (0.5 + skyHash(cell + 3.1)) + skyHash(cell) * 20.0));
  color += vec3(0.75, 0.82, 1.0) * star * uNight * smoothstep(0.03, 0.2, d.y) * 0.9;
  // The moon: a sphere lit from the sun's direction, so it shows its phase; by day only the lit part shows, faintly.
  float moon = dot(d, uMoonDir);
  color += vec3(0.85, 0.9, 1.0) * uNight * uMoonLight * pow(max(moon, 0.0), 110.0) * 0.14;
  if (moon > 0.9994) {
    vec3 right = normalize(cross(uMoonDir, vec3(0.0, 1.0, 0.0)));
    vec3 up = cross(right, uMoonDir);
    vec2 q = vec2(dot(d, right), dot(d, up)) / 0.0235;
    float r2 = dot(q, q);
    if (r2 < 1.0) {
      vec3 surface = q.x * right + q.y * up - sqrt(1.0 - r2) * uMoonDir;
      float lit = smoothstep(-0.06, 0.1, dot(surface, uSunDir));
      float maria = 0.78 + 0.22 * fbm3(vec3(q * 2.2, 4.0));
      float edge = smoothstep(1.0, 0.86, r2);
      color = mix(color, color * 0.25 + vec3(0.004, 0.005, 0.008), edge * uNight * 0.85);
      color += vec3(0.85, 0.9, 1.0) * lit * maria * edge * mix(0.42, 2.4, uNight);
    }
  }
  // Clouds: lit by the key light, darker and bluer in their shade, a rim glow around the sun.
  vec2 cloud = cloudLayer(d, cheap);
  if (cloud.x > 0.0) {
    vec3 shade = mix(uSkyHorizon * 0.5, uSkyTop * 1.3 + uAmbientTop * 0.45, 0.5);
    vec3 lit = uLight * 0.48 * cloud.y + uAmbientTop * 0.45;
    vec3 cloudColor = mix(shade, lit, clamp(cloud.y, 0.0, 1.0)) + uGlow * pow(toSun, 6.0) * 0.8;
    color = mix(color, cloudColor, cloud.x * 0.92);
  }
  return color;
}
`

export function skyUniforms(daylight: Daylight) {
  return {
    uSkyTop: { value: daylight.skyTop },
    uSkyHorizon: { value: daylight.skyHorizon },
    uSunDir: { value: daylight.sunDir },
    uMoonDir: { value: daylight.moonDir },
    uLightDir: { value: daylight.lightDir },
    uLight: { value: daylight.light },
    uAmbientTop: { value: daylight.ambientTop },
    uAmbientBottom: { value: daylight.ambientBottom },
    uGlow: { value: daylight.glow },
    uNight: { value: 0 },
    uTime: { value: 0 },
    uKeyVisible: { value: 1 },
    uMoonLight: { value: 1 },
  }
}

export type SkyUniforms = ReturnType<typeof skyUniforms>

export function createSkyDome(uniforms: SkyUniforms): THREE.Mesh {
  const dome = new THREE.Mesh(new THREE.SphereGeometry(20000, 64, 32), new THREE.ShaderMaterial({
    uniforms,
    vertexShader: /* glsl */ `
      varying vec3 vDirection;
      void main() {
        vDirection = position;
        gl_Position = projectionMatrix * viewMatrix * modelMatrix * vec4(position, 1.0);
        gl_Position.z = gl_Position.w;
      }
    `,
    fragmentShader: /* glsl */ `
      ${SKY_UNIFORMS_GLSL}
      varying vec3 vDirection;
      ${NOISE_GLSL}
      ${SKY_GLSL}
      void main() {
        vec3 d = normalize(vDirection);
        vec3 color = skyColor(vec3(d.x, max(d.y, 0.0), d.z), false);
        // Below the horizon (only ever seen past the sea's edge): the horizon haze.
        if (d.y < 0.0) color = uSkyHorizon * 0.9;
        gl_FragColor = vec4(color, 1.0);
      }
    `,
    side: THREE.BackSide,
    depthWrite: false,
  }))
  dome.frustumCulled = false
  dome.renderOrder = -1
  return dome
}
