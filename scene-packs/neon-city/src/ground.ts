import * as THREE from 'three'
import { Reflector } from 'three/examples/jsm/objects/Reflector.js'
import { FOG_GLSL, type AtmosphereUniforms } from './atmosphere'
import { AVENUE, AVENUE_X, EXTENT, PITCH, ROAD } from './layout'
import { NOISE_GLSL } from './noise'

/**
 * Wet streets: a half-resolution mirror of the city, broken up by puddles and
 * rain ripples, over asphalt with lane markings and pools of lamp light that
 * are worked out in the shader from the road grid.
 */
// Street lamps stand every 32 m from -EXTENT + 16 (see traffic.ts); this is their phase.
const LAMP_PHASE = (((-EXTENT + 16) % 32) + 32) % 32

const VERTEX = /* glsl */ `
uniform mat4 textureMatrix;
varying vec4 vReflect;
varying vec3 vWorld;
void main() {
  vReflect = textureMatrix * vec4(position, 1.0);
  vec4 world = modelMatrix * vec4(position, 1.0);
  vWorld = world.xyz;
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const FRAGMENT = /* glsl */ `
uniform sampler2D tDiffuse;
uniform vec3 color;
uniform float uTime;
uniform float uWet;
uniform float uRain;
uniform float uAmbient;
uniform float uLamps;
uniform vec3 uSkyTop;
uniform vec3 uHorizon;
varying vec4 vReflect;
varying vec3 vWorld;
${NOISE_GLSL}
${FOG_GLSL}

const float PITCH = ${PITCH.toFixed(1)};
float roadHalf(float center, bool alongZ) {
  return alongZ && abs(center - ${AVENUE_X.toFixed(1)}) < 1.0 ? ${(AVENUE / 2).toFixed(1)} : ${(ROAD / 2).toFixed(1)};
}

// Warm pool under the nearest lamp on a road running along one axis.
float lampPool(float along, float across, float halfWidth) {
  float nearest = floor((along - ${LAMP_PHASE.toFixed(1)}) / 32.0 + 0.5) * 32.0 + ${LAMP_PHASE.toFixed(1)};
  float lateral = min(abs(across - (halfWidth - 0.8)), abs(across + (halfWidth - 0.8)));
  float d2 = (along - nearest) * (along - nearest) + lateral * lateral;
  return exp(-d2 / 40.0);
}

void main() {
  vec2 p = vWorld.xz;
  float cx = (floor(p.x / PITCH) + 0.5) * PITCH;
  float cz = (floor(p.y / PITCH) + 0.5) * PITCH;
  float dx = p.x - cx;
  float dz = p.y - cz;
  float hx = roadHalf(cx, true);
  float hz = roadHalf(cz, false);
  float onX = 1.0 - step(hx, abs(dx));
  float onZ = 1.0 - step(hz, abs(dz));
  float road = max(onX, onZ);
  float crossing = onX * onZ;

  // Markings: centre lines and dashed lanes, faded out before they alias.
  float fwx = fwidth(dx);
  float fwz = fwidth(dz);
  float centre = onX * (1.0 - crossing) * (1.0 - smoothstep(0.12, 0.12 + fwx, abs(abs(dx) - 0.35)))
    + onZ * (1.0 - crossing) * (1.0 - smoothstep(0.12, 0.12 + fwz, abs(abs(dz) - 0.35)));
  float dashes = onX * (1.0 - crossing) * step(0.55, fract(p.y / 10.0)) * (1.0 - smoothstep(0.1, 0.1 + fwx, abs(abs(dx) - 4.0)))
    + onZ * (1.0 - crossing) * step(0.55, fract(p.x / 10.0)) * (1.0 - smoothstep(0.1, 0.1 + fwz, abs(abs(dz) - 4.0)));
  float markingFade = 1.0 - smoothstep(0.15, 0.5, max(fwx, fwz));
  vec3 paint = vec3(0.9, 0.62, 0.2) * centre + vec3(0.7) * dashes;

  vec3 asphalt = mix(vec3(0.03, 0.03, 0.034), vec3(0.014, 0.014, 0.017), road);
  float lamps = max(onX * lampPool(p.y, dx, hx), onZ * lampPool(p.x, dz, hz));
  vec3 sky = mix(uHorizon, uSkyTop, 0.5);
  vec3 base = asphalt * (0.25 + sky * uAmbient * 6.0) + paint * 0.035 * markingFade + vec3(1.0, 0.64, 0.34) * lamps * 0.16 * uLamps;

  // Puddles and rain ripples bend the reflection; dry patches blur it.
  float puddle = smoothstep(0.05, 0.45, snoise(vec3(p * 0.03, 0.0)) * 0.5 + 0.5 + 0.2 * snoise(vec3(p * 0.12, 3.0)));
  float wet = uWet * mix(0.45, 1.0, puddle);
  vec2 ripple = vec2(snoise(vec3(p * 0.9, uTime * 1.2)), snoise(vec3(p * 0.9 + 17.0, uTime * 1.2))) * 0.0012 * uRain * puddle;
  vec2 uv = vReflect.xy / vReflect.w + ripple;
  float blur = mix(0.02, 0.004, puddle);
  vec3 reflection = vec3(0.0);
  for (int i = -2; i <= 2; i++) {
    reflection += texture2D(tDiffuse, uv + vec2(0.0, float(i) * blur)).rgb;
  }
  reflection /= 5.0;
  vec3 view = normalize(cameraPosition - vWorld);
  float fresnel = mix(0.12, 0.8, pow(1.0 - view.y, 3.0));
  vec3 result = base + reflection * fresnel * wet * 0.85;
  gl_FragColor = vec4(applyFog(result, vWorld), 1.0);
}
`

export function createGround(uniforms: AtmosphereUniforms, width: number, height: number): Reflector {
  const ground = new Reflector(new THREE.PlaneGeometry(9000, 9000), {
    textureWidth: Math.max(256, Math.round(width / 2)),
    textureHeight: Math.max(256, Math.round(height / 2)),
    clipBias: 0.003,
    multisample: 0,
    shader: {
      name: 'NeonCityGround',
      uniforms: {
        color: { value: null },
        tDiffuse: { value: null },
        textureMatrix: { value: null },
        uTime: { value: 0 },
        uWet: { value: 0 },
        uRain: { value: 0 },
        uAmbient: { value: 0 },
        uLamps: { value: 0 },
        uSkyTop: { value: new THREE.Color() },
        uHorizon: { value: new THREE.Color() },
        uFogColor: { value: new THREE.Color() },
        uFogDensity: { value: 0 },
      },
      vertexShader: VERTEX,
      fragmentShader: FRAGMENT,
    },
  })
  // The Reflector clones its uniforms; point the shared ones back at the city's atmosphere.
  const material = ground.material as THREE.ShaderMaterial
  for (const name of ['uTime', 'uWet', 'uRain', 'uAmbient', 'uLamps', 'uSkyTop', 'uHorizon', 'uFogColor', 'uFogDensity'] as const) {
    material.uniforms[name] = uniforms[name]
  }
  ground.rotation.x = -Math.PI / 2
  return ground
}

export function resizeGround(ground: Reflector, width: number, height: number): void {
  ground.getRenderTarget().setSize(Math.max(256, Math.round(width / 2)), Math.max(256, Math.round(height / 2)))
}
