import * as THREE from 'three'
import { NOISE_GLSL } from './noise'

const NEBULA_VERTEX = /* glsl */ `
varying vec3 vDirection;
void main() {
  vDirection = normalize(position);
  gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
}
`

const NEBULA_FRAGMENT = /* glsl */ `
varying vec3 vDirection;
${NOISE_GLSL}
void main() {
  vec3 d = normalize(vDirection);
  float cloud = fbm(d * 2.3 + vec3(4.0, 1.0, 2.0));
  float filaments = fbm(d * 5.5 - vec3(1.3));
  vec3 violet = vec3(0.3, 0.07, 0.42);
  vec3 teal = vec3(0.03, 0.26, 0.34);
  vec3 rose = vec3(0.5, 0.1, 0.2);
  vec3 color = mix(violet, teal, smoothstep(-0.25, 0.35, filaments));
  color = mix(color, rose, smoothstep(0.25, 0.6, cloud) * 0.5);
  float density = smoothstep(-0.05, 0.55, cloud + filaments * 0.35);
  // A faint galactic band across the sky.
  vec3 bandNormal = normalize(vec3(0.25, 1.0, -0.35));
  float band = exp(-pow(dot(d, bandNormal) / 0.2, 2.0)) * (0.55 + 0.45 * fbm3(d * 9.0));
  vec3 result = color * density * 0.42 + vec3(0.62, 0.66, 0.8) * band * 0.14;
  gl_FragColor = vec4(result, 1.0);
  #include <tonemapping_fragment>
  #include <colorspace_fragment>
}
`

const STAR_VERTEX = /* glsl */ `
attribute float size;
attribute float phase;
attribute vec3 tint;
uniform float uTime;
uniform float uOpacity;
varying vec3 vTint;
varying float vAlpha;
void main() {
  vTint = tint;
  float twinkle = 0.72 + 0.28 * sin(uTime * (0.8 + phase * 2.4) + phase * 40.0);
  vAlpha = twinkle * uOpacity;
  gl_PointSize = size * twinkle;
  gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
}
`

const STAR_FRAGMENT = /* glsl */ `
varying vec3 vTint;
varying float vAlpha;
void main() {
  float d = length(gl_PointCoord - 0.5);
  float core = smoothstep(0.5, 0.0, d);
  gl_FragColor = vec4(vTint * core * core * 1.6, core * vAlpha);
}
`

function glowTexture(stops: readonly (readonly [number, string])[]): THREE.CanvasTexture {
  const size = 256
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = size
  const context = canvas.getContext('2d')!
  const gradient = context.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2)
  for (const [offset, color] of stops) gradient.addColorStop(offset, color)
  context.fillStyle = gradient
  context.fillRect(0, 0, size, size)
  const texture = new THREE.CanvasTexture(canvas)
  texture.colorSpace = THREE.SRGBColorSpace
  return texture
}

export interface Sky {
  group: THREE.Group
  sun: THREE.Group
  update(time: number, dt: number): void
}

export function createSky(sunDirection: THREE.Vector3, random: () => number): Sky {
  const group = new THREE.Group()

  const nebula = new THREE.Mesh(
    new THREE.SphereGeometry(7000, 64, 32),
    new THREE.ShaderMaterial({ vertexShader: NEBULA_VERTEX, fragmentShader: NEBULA_FRAGMENT, side: THREE.BackSide, depthWrite: false }),
  )
  nebula.renderOrder = -2
  group.add(nebula)

  const count = 7000
  const positions = new Float32Array(count * 3)
  const sizes = new Float32Array(count)
  const phases = new Float32Array(count)
  const tints = new Float32Array(count * 3)
  const palette = [new THREE.Color('#9bb8ff'), new THREE.Color('#ffffff'), new THREE.Color('#ffe7c2'), new THREE.Color('#ffc59a')]
  for (let index = 0; index < count; index++) {
    const direction = new THREE.Vector3(random() * 2 - 1, random() * 2 - 1, random() * 2 - 1).normalize()
    positions.set(direction.multiplyScalar(5200).toArray(), index * 3)
    sizes[index] = 0.8 + random() ** 6 * 4.2
    phases[index] = random()
    palette[Math.floor(random() * palette.length)]!.toArray(tints, index * 3)
  }
  const starGeometry = new THREE.BufferGeometry()
  starGeometry.setAttribute('position', new THREE.BufferAttribute(positions, 3))
  starGeometry.setAttribute('size', new THREE.BufferAttribute(sizes, 1))
  starGeometry.setAttribute('phase', new THREE.BufferAttribute(phases, 1))
  starGeometry.setAttribute('tint', new THREE.BufferAttribute(tints, 3))
  const starUniforms = { uTime: { value: 0 }, uOpacity: { value: 1 } }
  const stars = new THREE.Points(starGeometry, new THREE.ShaderMaterial({
    vertexShader: STAR_VERTEX,
    fragmentShader: STAR_FRAGMENT,
    uniforms: starUniforms,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  }))
  stars.renderOrder = -1
  group.add(stars)

  // The sun: an HDR core that feeds the bloom pass, and a wide soft corona.
  const sun = new THREE.Group()
  const core = new THREE.Sprite(new THREE.SpriteMaterial({
    map: glowTexture([[0, 'rgba(255,255,255,1)'], [0.12, 'rgba(255,248,230,1)'], [0.3, 'rgba(255,210,150,0.45)'], [1, 'rgba(255,170,90,0)']]),
    color: new THREE.Color(6, 5.4, 4.6),
    blending: THREE.AdditiveBlending,
    depthWrite: false,
    toneMapped: false,
  }))
  core.scale.setScalar(620)
  const corona = new THREE.Sprite(new THREE.SpriteMaterial({
    map: glowTexture([[0, 'rgba(255,220,170,0.55)'], [0.35, 'rgba(255,150,90,0.12)'], [1, 'rgba(255,120,60,0)']]),
    color: new THREE.Color(1.6, 1.3, 1.1),
    blending: THREE.AdditiveBlending,
    depthWrite: false,
    toneMapped: false,
  }))
  corona.scale.setScalar(3400)
  sun.add(corona, core)
  sun.position.copy(sunDirection).multiplyScalar(5000)
  group.add(sun)

  // Occasional meteors far behind the scene.
  const meteorMaterial = new THREE.LineBasicMaterial({ color: new THREE.Color(2.2, 2.2, 2.6), transparent: true, opacity: 0, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false })
  const meteorGeometry = new THREE.BufferGeometry().setFromPoints([new THREE.Vector3(), new THREE.Vector3()])
  const meteor = new THREE.Line(meteorGeometry, meteorMaterial)
  group.add(meteor)
  let meteorWait = 9
  let meteorAge = -1
  const meteorFrom = new THREE.Vector3()
  const meteorDirection = new THREE.Vector3()

  return {
    group,
    sun,
    update(time, dt) {
      starUniforms.uTime.value = time
      nebula.rotation.y += dt * 0.0015
      if (meteorAge < 0) {
        meteorWait -= dt
        if (meteorWait <= 0) {
          meteorAge = 0
          meteorFrom.set(random() * 2 - 1, 0.3 + random() * 0.5, random() * 2 - 1).normalize().multiplyScalar(4200)
          meteorDirection.set(random() * 2 - 1, -0.4 - random() * 0.3, random() * 2 - 1).normalize()
        }
        return
      }
      meteorAge += dt
      const progress = meteorAge / 1.1
      if (progress >= 1) {
        meteorAge = -1
        meteorWait = 14 + random() * 18
        meteorMaterial.opacity = 0
        return
      }
      const head = meteorFrom.clone().addScaledVector(meteorDirection, progress * 900)
      const tail = head.clone().addScaledVector(meteorDirection, -260)
      meteorGeometry.setFromPoints([tail, head])
      meteorMaterial.opacity = Math.sin(progress * Math.PI)
    },
  }
}
