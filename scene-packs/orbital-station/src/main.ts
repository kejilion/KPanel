import * as THREE from 'three'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js'
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js'
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js'
import { onHostCommand, postToHost } from './bridge'
import { Director, type Shot } from './director'
import { createPlanet, PLANET_RADIUS } from './planet'
import { createSky } from './sky'
import { createStation, createTraffic } from './station'

/**
 * Orbital Station: a planet with live clouds and night-side cities, a ringed
 * station with shuttle traffic, and a nebula sky. Entrance: one continuous
 * fade up from black while the camera glides in. Idle: rotating habitat,
 * blinking beacons, drifting camera.
 */

function mulberry32(seed: number): () => number {
  let state = seed >>> 0
  return () => {
    state = (state + 0x6d2b79f5) >>> 0
    let value = state
    value = Math.imul(value ^ (value >>> 15), value | 1)
    value ^= value + Math.imul(value ^ (value >>> 7), value | 61)
    return ((value ^ (value >>> 14)) >>> 0) / 4294967296
  }
}

const params = new URLSearchParams(location.search)
const random = mulberry32(20260924)
const SUN = new THREE.Vector3(-1, 0.25, -0.15).normalize()
const STATION = new THREE.Vector3(72, 30, 158)
const EXPOSURE = 0.95

/** A camera on the night side placed so the sun sits just above the planet limb. */
function sunriseShot(distance: number, lift: number): Shot {
  const alpha = Math.asin(PLANET_RADIUS / distance) + lift
  const down = new THREE.Vector3(0, -1, 0).projectOnPlane(SUN).normalize()
  const toCenter = SUN.clone().multiplyScalar(Math.cos(alpha)).addScaledVector(down, Math.sin(alpha))
  const position = toCenter.clone().multiplyScalar(-distance)
  const look = SUN.clone().add(toCenter).normalize()
  return { id: 'sunrise', position, target: position.clone().addScaledVector(look, 120), fov: 50, orbit: 0.035, float: 2 }
}

const SHOTS: Shot[] = [
  // Panorama: a half-lit planet with the station against its night side.
  { id: 'panorama', position: new THREE.Vector3(62, 70, 385), target: new THREE.Vector3(24, 12, 70), fov: 42, orbit: 0.07, float: 5 },
  // Close pass along the habitat ring from the sunlit side.
  { id: 'station', position: STATION.clone().add(new THREE.Vector3(-92, 30, 84)), target: STATION.clone().add(new THREE.Vector3(18, -4, -6)), fov: 40, orbit: 0.1, float: 2.5 },
  // Sunrise over the planet limb.
  sunriseShot(205, 0.035),
]

function createRenderer(): THREE.WebGLRenderer | undefined {
  try {
    const renderer = new THREE.WebGLRenderer({ antialias: true, powerPreference: 'high-performance' })
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5))
    renderer.toneMapping = THREE.ACESFilmicToneMapping
    renderer.toneMappingExposure = EXPOSURE
    renderer.outputColorSpace = THREE.SRGBColorSpace
    return renderer
  } catch {
    return undefined
  }
}

/** A dim studio for reflections: dark space above, planet blue below, a warm sun spot. */
function spaceEnvironment(renderer: THREE.WebGLRenderer): THREE.Texture {
  const scene = new THREE.Scene()
  const dome = new THREE.Mesh(new THREE.SphereGeometry(10, 32, 16), new THREE.ShaderMaterial({
    side: THREE.BackSide,
    uniforms: { uSun: { value: SUN } },
    vertexShader: 'varying vec3 vDirection; void main() { vDirection = normalize(position); gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }',
    fragmentShader: `
      uniform vec3 uSun;
      varying vec3 vDirection;
      void main() {
        vec3 d = normalize(vDirection);
        vec3 color = mix(vec3(0.02, 0.03, 0.05), vec3(0.05, 0.12, 0.22), smoothstep(0.2, -0.6, d.y));
        color += vec3(3.0, 2.6, 2.1) * pow(max(dot(d, uSun), 0.0), 64.0);
        gl_FragColor = vec4(color, 1.0);
      }`,
  }))
  scene.add(dome)
  const generator = new THREE.PMREMGenerator(renderer)
  const texture = generator.fromScene(scene).texture
  generator.dispose()
  return texture
}

async function start(): Promise<void> {
  const renderer = createRenderer()
  if (!renderer) {
    postToHost({ source: 'kpanel-scene-pack', type: 'error', reason: 'webgl_unavailable' })
    return
  }
  document.body.appendChild(renderer.domElement)

  const scene = new THREE.Scene()
  scene.environment = spaceEnvironment(renderer)
  scene.environmentIntensity = 0.45
  const camera = new THREE.PerspectiveCamera(42, window.innerWidth / window.innerHeight, 0.5, 16000)
  scene.add(camera)

  // The sun, casting hard shadows across the station (the only thing near enough to need them).
  renderer.shadowMap.enabled = true
  renderer.shadowMap.type = THREE.PCFShadowMap
  const sunLight = new THREE.DirectionalLight(0xfff0dc, 2.6)
  sunLight.position.copy(STATION).addScaledVector(SUN, 200)
  sunLight.target.position.copy(STATION)
  sunLight.castShadow = true
  sunLight.shadow.mapSize.set(2048, 2048)
  sunLight.shadow.camera.left = -62
  sunLight.shadow.camera.right = 62
  sunLight.shadow.camera.top = 62
  sunLight.shadow.camera.bottom = -62
  sunLight.shadow.camera.near = 100
  sunLight.shadow.camera.far = 300
  sunLight.shadow.bias = -0.0004
  sunLight.shadow.normalBias = 0.04
  scene.add(sunLight, sunLight.target, new THREE.HemisphereLight(0x5f7fb0, 0x05070b, 0.22))

  const sky = createSky(SUN, random)
  scene.add(sky.group)
  const planet = await createPlanet(SUN)
  scene.add(planet.group)
  const station = await createStation(SUN)
  station.group.position.copy(STATION)
  station.group.rotation.set(0.32, 0.5, -0.28)
  scene.add(station.group)
  const traffic = createTraffic(STATION, random)
  scene.add(traffic.group)
  station.setPower(1)

  const composer = new EffectComposer(renderer)
  composer.addPass(new RenderPass(scene, camera))
  // Only the sun, its glint, the atmosphere's blaze at sunrise and the lights bloom; sunlit cloud does not.
  const bloom = new UnrealBloomPass(new THREE.Vector2(window.innerWidth, window.innerHeight), 0.7, 0.5, 1.05)
  composer.addPass(bloom)
  composer.addPass(new OutputPass())

  const resize = () => {
    const width = window.innerWidth
    const height = window.innerHeight
    renderer.setSize(width, height)
    composer.setSize(width, height)
    camera.aspect = width / height
    camera.updateProjectionMatrix()
  }
  resize()
  window.addEventListener('resize', resize)

  const requestedShot = Number(params.get('shot'))
  const director = new Director(camera, SHOTS, {
    entrance: params.get('entrance') !== 'off',
    shot: Number.isInteger(requestedShot) && requestedShot >= 0 && requestedShot < SHOTS.length ? requestedShot : 0,
  })
  director.onShotChange = (index) => postToHost({ source: 'kpanel-scene-pack', type: 'camera', index })

  let paused = false
  let handle = 0
  let last = 0
  let time = 0
  const frame = (now: number) => {
    handle = requestAnimationFrame(frame)
    // rAF timestamps can precede the first directly rendered frame: never let time run backwards.
    const dt = last ? Math.min(Math.max((now - last) / 1000, 0), 0.05) : 1 / 60
    last = now
    time += dt
    const cue = director.update(dt, time)
    sky.update(time, dt)
    planet.update(time, dt)
    station.update(time, dt)
    traffic.update(time, dt)
    renderer.toneMappingExposure = EXPOSURE * cue.fade
    composer.render(dt)
  }

  onHostCommand((command) => {
    if (command.type === 'pause' && !paused) {
      paused = true
      cancelAnimationFrame(handle)
    } else if (command.type === 'resume' && paused) {
      paused = false
      last = 0
      handle = requestAnimationFrame(frame)
    } else if (command.type === 'camera') {
      director.cut(command.index)
    }
  })
  renderer.domElement.addEventListener('webglcontextlost', () => {
    cancelAnimationFrame(handle)
    postToHost({ source: 'kpanel-scene-pack', type: 'error', reason: 'webgl_context_lost' })
  })
  // Compile every shader and draw the first (black) frame before reporting ready, so the
  // entrance starts on time instead of stalling on its first frames.
  renderer.compile(scene, camera)
  frame(performance.now())
  postToHost({ source: 'kpanel-scene-pack', type: 'ready', cameras: SHOTS.map((shot) => shot.id) })
}

start().catch(() => postToHost({ source: 'kpanel-scene-pack', type: 'error', reason: 'assets_unavailable' }))
