import * as THREE from 'three'
import { FOG_GLSL, HASH_GLSL, type AtmosphereUniforms } from './atmosphere'
import { DOWNTOWN, type Building } from './layout'
import { NOISE_GLSL } from './noise'

const DOME_VERTEX = /* glsl */ `
varying vec3 vDirection;
void main() {
  vDirection = position;
  vec4 world = modelMatrix * vec4(position, 1.0);
  gl_Position = projectionMatrix * viewMatrix * world;
  gl_Position.z = gl_Position.w; // always behind everything
}
`

const DOME_FRAGMENT = /* glsl */ `
uniform vec3 uSkyTop;
uniform vec3 uSkyMid;
uniform vec3 uHorizon;
uniform vec3 uGlow;
uniform vec3 uSunDir;
uniform vec3 uSunColor;
uniform vec3 uMoonDir;
uniform float uSun;
uniform float uMoon;
uniform float uStars;
uniform float uClouds;
uniform float uTime;
varying vec3 vDirection;
${HASH_GLSL}
${NOISE_GLSL}
void main() {
  vec3 d = normalize(vDirection);
  float h = d.y;
  vec3 color = mix(uHorizon, uSkyMid, smoothstep(0.0, 0.16, h));
  color = mix(color, uSkyTop, smoothstep(0.1, 0.62, h));
  // Afterglow of the set sun, spread wide along the horizon.
  vec3 sunFlat = normalize(vec3(uSunDir.x, 0.0, uSunDir.z));
  float toward = max(dot(normalize(vec3(d.x, 0.0, d.z)), sunFlat), 0.0);
  color += uSunColor * uSun * (pow(toward, 3.0) * 0.55 + pow(max(dot(d, uSunDir), 0.0), 40.0) * 0.8) * exp(-max(h, 0.0) * 5.0);
  // Light pollution: the city lights the haze above it.
  color += uGlow * exp(-max(h, 0.0) * 10.0) * 0.9;

  if (h > 0.0) {
    // Stars and moon first, clouds drift over them.
    vec3 cell = floor(d * 420.0);
    float star = step(0.9975, hash13(cell)) * (0.6 + 0.4 * sin(uTime * (1.0 + hash13(cell + 7.0) * 2.0) + hash13(cell) * 30.0));
    color += vec3(0.8, 0.85, 1.0) * star * uStars * smoothstep(0.05, 0.3, h) * 0.9;
    float moon = dot(d, uMoonDir);
    color += vec3(0.9, 0.92, 1.0) * uMoon * (smoothstep(0.99955, 0.99975, moon) * 2.0 + pow(max(moon, 0.0), 300.0) * 0.25);

    vec2 plane = d.xz / (h + 0.06);
    float n = fbm3(vec3(plane * 0.55 + vec2(uTime * 0.006, 0.0), uTime * 0.01));
    float cover = smoothstep(0.42 - uClouds * 0.38, 0.9, n + 0.35);
    vec3 underlit = uGlow * 1.3 + uHorizon * 0.25;
    vec3 sunlit = uSunColor * 0.35 * pow(toward, 1.5) + uSkyMid * 0.6;
    vec3 cloud = mix(underlit, sunlit, clamp(uSun, 0.0, 1.0)) * mix(1.0, 0.45, smoothstep(0.0, 0.5, h));
    color = mix(color, cloud, cover * smoothstep(0.0, 0.06, h) * 0.9);
  }
  gl_FragColor = vec4(color, 1.0);
}
`

const BEAM_VERTEX = /* glsl */ `
varying vec3 vWorld;
varying vec3 vNormal;
varying float vAlong;
void main() {
  vec4 world = modelMatrix * vec4(position, 1.0);
  vWorld = world.xyz;
  vNormal = normalize(mat3(modelMatrix) * normal);
  vAlong = position.y;
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const BEAM_FRAGMENT = /* glsl */ `
uniform float uSearch;
varying vec3 vWorld;
varying vec3 vNormal;
varying float vAlong;
void main() {
  vec3 view = normalize(cameraPosition - vWorld);
  float core = pow(abs(dot(normalize(vNormal), view)), 3.0);
  float fade = (1.0 - vAlong) * smoothstep(0.0, 0.02, vAlong);
  gl_FragColor = vec4(vec3(0.55, 0.7, 1.0) * core * fade * fade * uSearch * 0.15, 1.0);
}
`

const BLINK_VERTEX = /* glsl */ `
uniform float uPointScale;
uniform float uTime;
attribute vec3 aBlink; // kind (0 tower, 1 red, 2 green, 3 white), phase, size
varying vec3 vColor;
varying float vGain;
varying vec3 vWorld;
void main() {
  vec4 view = viewMatrix * vec4(position, 1.0);
  float size = aBlink.z * uPointScale / max(-view.z, 1.0);
  gl_PointSize = clamp(size, 2.4, 30.0);
  vGain = clamp(size / 2.4, 0.2, 1.0);
  float t = uTime + aBlink.y;
  float on;
  // Everything glows steadily or breathes slowly: no strobes, nothing that flashes.
  if (aBlink.x < 0.5) {
    on = 0.45 + 0.55 * (0.5 + 0.5 * sin(t * 1.1));
    vColor = vec3(1.0, 0.1, 0.05) * 1.2;
  } else if (aBlink.x < 1.5) {
    on = 1.0;
    vColor = vec3(1.0, 0.12, 0.08) * 1.1;
  } else if (aBlink.x < 2.5) {
    on = 1.0;
    vColor = vec3(0.15, 1.0, 0.4) * 1.0;
  } else {
    on = 1.0;
    vColor = vec3(1.0, 0.95, 0.9) * 0.8;
  }
  vColor *= on;
  vWorld = position;
  gl_Position = projectionMatrix * view;
}
`

const BLINK_FRAGMENT = /* glsl */ `
varying vec3 vColor;
varying float vGain;
varying vec3 vWorld;
${FOG_GLSL}
void main() {
  vec2 p = gl_PointCoord - 0.5;
  gl_FragColor = vec4(vColor * exp(-dot(p, p) * 20.0) * vGain * (1.0 - fogAmount(vWorld) * 0.8), 1.0);
}
`

interface Flyer {
  radius: number
  height: number
  speed: number
  phase: number
  center: THREE.Vector2
  tilt: number
}

export interface Sky {
  group: THREE.Group
  update(time: number, camera: THREE.Camera): void
}

export function createSky(uniforms: AtmosphereUniforms, towers: readonly Building[], aviation: readonly THREE.Vector3[], random: () => number): Sky {
  const group = new THREE.Group()

  const dome = new THREE.Mesh(new THREE.SphereGeometry(8000, 64, 32), new THREE.ShaderMaterial({
    uniforms: {
      uSkyTop: uniforms.uSkyTop, uSkyMid: uniforms.uSkyMid, uHorizon: uniforms.uHorizon, uGlow: uniforms.uGlow,
      uSunDir: uniforms.uSunDir, uSunColor: uniforms.uSunColor, uMoonDir: uniforms.uMoonDir,
      uSun: uniforms.uSun, uMoon: uniforms.uMoon, uStars: uniforms.uStars, uClouds: uniforms.uClouds, uTime: uniforms.uTime,
    },
    vertexShader: DOME_VERTEX,
    fragmentShader: DOME_FRAGMENT,
    side: THREE.BackSide,
    depthWrite: false,
  }))
  dome.frustumCulled = false
  dome.renderOrder = -1
  group.add(dome)

  // Searchlights sweeping the clouds from the tallest roofs.
  const beamGeometry = new THREE.CylinderGeometry(1, 0.02, 1, 32, 1, true)
  beamGeometry.translate(0, 0.5, 0)
  const beamMaterial = new THREE.ShaderMaterial({
    uniforms: { uSearch: uniforms.uSearch },
    vertexShader: BEAM_VERTEX,
    fragmentShader: BEAM_FRAGMENT,
    side: THREE.DoubleSide,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  })
  const beams = towers.slice(0, 4).map((tower, index) => {
    const beam = new THREE.Mesh(beamGeometry, beamMaterial)
    beam.position.set(tower.x, tower.y + tower.h + 1, tower.z)
    beam.scale.set(70, 1500, 70)
    beam.userData = { phase: index * 1.9 + random(), speed: 0.05 + random() * 0.04 }
    beam.frustumCulled = false
    group.add(beam)
    return beam
  })

  // Blinking lights: aviation lights on towers and flying cars circling downtown.
  const flyers: Flyer[] = Array.from({ length: 9 }, () => ({
    radius: 180 + random() * 380,
    height: 95 + random() * 120,
    speed: (0.025 + random() * 0.03) * (random() < 0.5 ? -1 : 1),
    phase: random() * Math.PI * 2,
    center: new THREE.Vector2(DOWNTOWN.x + (random() - 0.5) * 200, DOWNTOWN.y + 120 + (random() - 0.5) * 200),
    tilt: (random() - 0.5) * 0.3,
  }))
  const staticCount = aviation.length
  const count = staticCount + flyers.length * 3
  const positions = new Float32Array(count * 3)
  const blink = new Float32Array(count * 3)
  aviation.forEach((point, index) => {
    positions.set([point.x, point.y, point.z], index * 3)
    blink.set([0, random() * 2, 2.2], index * 3)
  })
  flyers.forEach((_, index) => {
    const base = staticCount + index * 3
    blink.set([1, 0, 1.2], base * 3)
    blink.set([2, 0, 1.2], (base + 1) * 3)
    blink.set([3, random() * 3, 1.6], (base + 2) * 3)
  })
  const blinkGeometry = new THREE.BufferGeometry()
  const positionAttribute = new THREE.BufferAttribute(positions, 3)
  positionAttribute.setUsage(THREE.DynamicDrawUsage)
  blinkGeometry.setAttribute('position', positionAttribute)
  blinkGeometry.setAttribute('aBlink', new THREE.BufferAttribute(blink, 3))
  const blinkPoints = new THREE.Points(blinkGeometry, new THREE.ShaderMaterial({
    uniforms: { uPointScale: uniforms.uPointScale, uTime: uniforms.uTime, uFogColor: uniforms.uFogColor, uFogDensity: uniforms.uFogDensity },
    vertexShader: BLINK_VERTEX,
    fragmentShader: BLINK_FRAGMENT,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  }))
  blinkPoints.frustumCulled = false
  group.add(blinkPoints)
  const bodies = new THREE.InstancedMesh(new THREE.BoxGeometry(5.5, 1.3, 2.6), new THREE.MeshBasicMaterial({ color: 0x050608 }), flyers.length)
  bodies.frustumCulled = false
  group.add(bodies)

  const matrix = new THREE.Matrix4()
  const quaternion = new THREE.Quaternion()
  const euler = new THREE.Euler()
  const one = new THREE.Vector3(1, 1, 1)
  const position = new THREE.Vector3()
  const side = new THREE.Vector3()
  const direction = new THREE.Vector3()
  const up = new THREE.Vector3(0, 1, 0)

  return {
    group,
    update(time, camera) {
      dome.position.copy(camera.position)
      for (const beam of beams) {
        const { phase, speed } = beam.userData as { phase: number, speed: number }
        const azimuth = phase + time * speed
        const elevation = 0.95 + Math.sin(time * speed * 1.7 + phase) * 0.28
        direction.set(Math.cos(azimuth) * Math.cos(elevation), Math.sin(elevation), Math.sin(azimuth) * Math.cos(elevation))
        beam.quaternion.setFromUnitVectors(up, direction)
      }
      flyers.forEach((flyer, index) => {
        const angle = flyer.phase + time * flyer.speed
        position.set(flyer.center.x + Math.cos(angle) * flyer.radius, flyer.height + Math.sin(time * 0.3 + flyer.phase) * 6, flyer.center.y + Math.sin(angle) * flyer.radius * 0.7)
        const heading = Math.atan2(-Math.cos(angle) * flyer.radius * 0.7 * Math.sign(flyer.speed), -Math.sin(angle) * flyer.radius * Math.sign(flyer.speed))
        quaternion.setFromEuler(euler.set(0, heading, flyer.tilt))
        bodies.setMatrixAt(index, matrix.compose(position, quaternion, one))
        side.set(0, 0, 1.6).applyQuaternion(quaternion)
        const base = (staticCount + index * 3) * 3
        positions.set([position.x - side.x, position.y, position.z - side.z], base)
        positions.set([position.x + side.x, position.y, position.z + side.z], base + 3)
        positions.set([position.x, position.y + 0.9, position.z], base + 6)
      })
      bodies.instanceMatrix.needsUpdate = true
      positionAttribute.needsUpdate = true
    },
  }
}
