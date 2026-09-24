import * as THREE from 'three'
import type { AtmosphereUniforms } from './atmosphere'

/**
 * Rain streaks in a box that follows the camera. Drops keep their world
 * position between frames (they wrap around the box, not with the camera),
 * so a moving camera flies through the rain instead of dragging it along.
 */
const VERTEX = /* glsl */ `
uniform float uTime;
uniform float uRain;
uniform vec3 uCenter;
attribute vec4 aDrop; // position in the box (0-1), seed
attribute float aEnd; // 0 = head, 1 = tail
varying float vAlpha;
const vec3 BOX = vec3(180.0, 110.0, 180.0);
const vec3 WIND = vec3(0.16, -1.0, 0.07);
void main() {
  float speed = 34.0 + aDrop.w * 12.0;
  vec3 p = aDrop.xyz * BOX + WIND * uTime * speed;
  p = mod(p - uCenter + BOX * 0.5, BOX) - BOX * 0.5 + uCenter;
  p -= normalize(WIND) * aEnd * (1.1 + aDrop.w * 0.9);
  float distance = length(p - cameraPosition);
  vAlpha = uRain * (0.1 + 0.14 * aDrop.w) * (1.0 - smoothstep(30.0, 88.0, distance)) * step(0.0, p.y) * step(aDrop.w, 0.35 + uRain * 0.65);
  gl_Position = projectionMatrix * viewMatrix * vec4(p, 1.0);
}
`

const FRAGMENT = /* glsl */ `
uniform vec3 uGlow;
varying float vAlpha;
void main() {
  gl_FragColor = vec4((vec3(0.55, 0.62, 0.75) + uGlow * 0.8) * vAlpha, 1.0);
}
`

export interface Rain {
  object: THREE.LineSegments
  update(camera: THREE.Camera): void
}

export function createRain(uniforms: AtmosphereUniforms, random: () => number, count = 9000): Rain {
  const drops = new Float32Array(count * 2 * 4)
  const ends = new Float32Array(count * 2)
  for (let index = 0; index < count; index++) {
    const drop = [random(), random(), random(), random()]
    drops.set(drop, index * 8)
    drops.set(drop, index * 8 + 4)
    ends[index * 2 + 1] = 1
  }
  const geometry = new THREE.BufferGeometry()
  // Positions are computed in the shader; the attribute only sizes the draw.
  geometry.setAttribute('position', new THREE.BufferAttribute(new Float32Array(count * 2 * 3), 3))
  geometry.setAttribute('aDrop', new THREE.BufferAttribute(drops, 4))
  geometry.setAttribute('aEnd', new THREE.BufferAttribute(ends, 1))
  const center = new THREE.Vector3()
  const object = new THREE.LineSegments(geometry, new THREE.ShaderMaterial({
    uniforms: { uTime: uniforms.uTime, uRain: uniforms.uRain, uGlow: uniforms.uGlow, uCenter: { value: center } },
    vertexShader: VERTEX,
    fragmentShader: FRAGMENT,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  }))
  object.frustumCulled = false
  return {
    object,
    update(camera) {
      center.copy(camera.position)
      object.visible = uniforms.uRain.value > 0.01
    },
  }
}
