import * as THREE from 'three'
import type { Daylight } from './daylight'
import { CLOUD_GLSL } from './clouds'
import { NOISE_GLSL } from './noise'

/**
 * The sky, as one GLSL function used twice: for the sky dome and for the
 * ocean's reflections, so the sea always mirrors the same sky. A gradient that
 * follows the sun, a soft sun disc and glow, the clouds (volumetric on the dome,
 * see clouds.ts; a cheaper take on the same cloud field in the reflections),
 * the moon in its real phase (lit on the
 * side facing the sun, a pale disc when it is up by day), and at night the
 * stars and the Milky Way, fixed to the celestial sphere so they turn through
 * the night as the real ones do.
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
uniform mat3 uCelestial;
uniform float uCloudTime;
`

export const SKY_GLSL = /* glsl */ `
// The galactic north pole and centre, as equatorial unit vectors.
const vec3 GALACTIC_POLE = vec3(-0.8677, -0.1981, 0.456);
const vec3 GALACTIC_CENTRE = vec3(-0.0549, -0.8734, -0.4839);

float skyHash(vec2 p) {
  vec3 p3 = fract(vec3(p.xyx) * 0.1031);
  p3 += dot(p3, p3.yzx + 33.33);
  return fract((p3.x + p3.y) * p3.z);
}

/** flatClouds: draw the clouds here (for reflections); the dome lays the volumetric ones on top instead. */
vec3 skyColor(vec3 d, bool cheap, bool flatClouds) {
  float h = max(d.y, 0.0);
  vec3 color = mix(uSkyHorizon, uSkyTop, pow(h, 0.42));
  float toSun = max(dot(d, uSunDir), 0.0);
  color += uGlow * (pow(toSun, 5.0) * 0.45 + pow(toSun, 40.0) * 0.9);
  // A soft sun disc: bright, but not blinding.
  color += uLight * smoothstep(0.99962, 0.99988, toSun) * 1.6 * (1.0 - uNight);
  // Night sky: stars and the Milky Way on the celestial sphere, washed out by a bright moon.
  if (uNight > 0.01) {
    vec3 c = uCelestial * d;
    float latitude = dot(c, GALACTIC_POLE);
    float centre = dot(c, GALACTIC_CENTRE) * 0.5 + 0.5;
    // A long band, wider and brighter towards the core.
    float band = exp(-latitude * latitude / (0.008 + 0.018 * centre * centre));
    float milky = band * (0.45 + 0.8 * pow(centre, 6.0));
    if (!cheap) {
      // Grainy star clouds, and a dark rift of dust along the middle.
      float clouds = fbm3(c * 11.0 + 2.0) * 0.5 + 0.5;
      float grain = fbm3(c * 90.0) * 0.5 + 0.5;
      milky *= 0.45 + 1.1 * clouds * mix(1.0, grain, 0.5);
      float rift = exp(-pow(latitude - 0.012 * snoise(c * 8.0), 2.0) / 0.0009);
      milky *= 1.0 - 0.72 * rift * smoothstep(0.35, 0.65, clouds + 0.15);
    }
    float dark = uNight * (1.0 - 0.8 * uMoonLight) * smoothstep(0.02, 0.3, d.y);
    color += mix(vec3(0.5, 0.56, 0.75), vec3(0.68, 0.64, 0.6), pow(centre, 5.0)) * milky * dark * 0.2;
    if (!cheap) {
      // Pinpoint stars, a few bright ones and many faint ones (more of them in the Milky Way).
      vec2 p = vec2(atan(c.y, c.x) * 520.0 * sqrt(max(1.0 - c.z * c.z, 0.0)), asin(clamp(c.z, -1.0, 1.0)) * 520.0);
      vec2 id = floor(p);
      float h = skyHash(id);
      vec2 at = vec2(skyHash(id + 17.3), skyHash(id + 41.9)) * 0.6 + 0.2;
      float core = smoothstep(0.42, 0.0, length(fract(p) - at));
      float bright = step(0.9975, h) * 1.0 + step(0.985 - band * 0.02, h) * step(h, 0.9975) * 0.28;
      float twinkle = 0.8 + 0.2 * sin(uTime * (0.4 + skyHash(id + 5.0)) + h * 40.0);
      color += mix(vec3(0.7, 0.8, 1.0), vec3(1.0, 0.86, 0.7), skyHash(id + 9.0)) * core * bright * twinkle * dark * (1.0 - 0.5 * uMoonLight) * 1.4;
    }
  }
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
  vec2 cloud = flatClouds ? cloudCover(d) : vec2(0.0);
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
    uCelestial: { value: daylight.celestial },
    uCloudTime: { value: 0 },
    // Filled in once the cloud volumes have loaded (see clouds.ts).
    uShape: { value: null as THREE.Texture | null },
    uDetail: { value: null as THREE.Texture | null },
    uCloudCover: { value: 0.45 },
    uClouds: { value: null as THREE.Texture | null },
    uScreen: { value: new THREE.Vector2(1, 1) },
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
      uniform sampler2D uClouds;
      uniform vec2 uScreen;
      varying vec3 vDirection;
      ${NOISE_GLSL}
      ${CLOUD_GLSL}
      ${SKY_GLSL}
      void main() {
        vec3 d = normalize(vDirection);
        vec3 color = skyColor(vec3(d.x, max(d.y, 0.0), d.z), false, false);
        // Below the horizon (only ever seen past the sea's edge): the horizon haze.
        if (d.y < 0.0) color = uSkyHorizon * 0.9;
        // The volumetric clouds, marched for this very view at half resolution.
        vec4 clouds = texture2D(uClouds, gl_FragCoord.xy / uScreen);
        gl_FragColor = vec4(color * clouds.a + clouds.rgb, 1.0);
      }
    `,
    side: THREE.BackSide,
    depthWrite: false,
  }))
  dome.frustumCulled = false
  dome.renderOrder = -1
  return dome
}
