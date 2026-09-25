import * as THREE from 'three'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js'
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js'
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js'
import { createAtmosphere } from './atmosphere'
import { onHostCommand, postToHost } from './bridge'
import { aviationLights, createBuildings } from './buildings'
import { Director, type Shot } from './director'
import { createGround, resizeGround } from './ground'
import { AVENUE_X, createLayout, mulberry32 } from './layout'
import { createRain } from './rain'
import { createRooftops } from './rooftops'
import { createSigns } from './signs'
import { createSky } from './sky'
import { createTraffic } from './traffic'

/**
 * Neon City: a procedural city from dusk to a rainy night. Towers with living
 * windows, neon signs and screens, street and highway traffic, searchlights,
 * rain and wet reflective streets. Each camera shot has its own time of day,
 * and the moves between shots carry the sky, lights and weather with them.
 */

const params = new URLSearchParams(location.search)
const random = mulberry32(20260924)

const SHOTS: Shot[] = [
  // Dusk: the skyline against the afterglow, the east-west highway in front.
  { id: 'skyline', position: new THREE.Vector3(80, 95, 520), target: new THREE.Vector3(0, 118, -300), fov: 45, orbit: 0.06, float: 3, tod: 0 },
  // Blue hour: just above the elevated highway, looking along it into downtown.
  { id: 'highway', position: new THREE.Vector3(-830, 48, 26), target: new THREE.Vector3(-110, 40, -175), fov: 46, orbit: 0.03, float: 1.2, tod: 1 },
  // Rainy night: standing in the avenue under the signs, the overpass ahead.
  { id: 'street', position: new THREE.Vector3(AVENUE_X, 10, 150), target: new THREE.Vector3(AVENUE_X, 22, -200), fov: 54, orbit: 0.012, float: 0.4, tod: 2 },
]

function createRenderer(): THREE.WebGLRenderer | undefined {
  try {
    const renderer = new THREE.WebGLRenderer({ antialias: false, powerPreference: 'high-performance' })
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5))
    renderer.toneMapping = THREE.ACESFilmicToneMapping
    renderer.toneMappingExposure = 1
    renderer.outputColorSpace = THREE.SRGBColorSpace
    return renderer
  } catch {
    return undefined
  }
}

async function start(): Promise<void> {
  const renderer = createRenderer()
  if (!renderer) {
    postToHost({ source: 'kpanel-scene-pack', type: 'error', reason: 'webgl_unavailable' })
    return
  }
  document.body.appendChild(renderer.domElement)

  const atmosphere = createAtmosphere()
  const { uniforms } = atmosphere
  const scene = new THREE.Scene()
  const camera = new THREE.PerspectiveCamera(45, window.innerWidth / window.innerHeight, 1, 14000)
  scene.add(camera)

  const { buildings, landmarks } = createLayout()
  const rooms = await new THREE.TextureLoader().loadAsync('assets/rooms.webp')
  rooms.colorSpace = THREE.SRGBColorSpace
  rooms.anisotropy = 8
  scene.add(createBuildings(buildings, uniforms, rooms))
  scene.add(await createRooftops(buildings, uniforms))
  scene.add(createSigns(buildings, landmarks, uniforms, random))
  scene.add(createTraffic(uniforms, random))
  const sky = createSky(uniforms, landmarks, aviationLights(buildings), random)
  scene.add(sky.group)
  const rain = createRain(uniforms, random)
  scene.add(rain.object)
  const size = renderer.getDrawingBufferSize(new THREE.Vector2())
  const ground = createGround(uniforms, size.x, size.y)
  scene.add(ground)

  const target = new THREE.WebGLRenderTarget(1, 1, { type: THREE.HalfFloatType, samples: 4 })
  const composer = new EffectComposer(renderer, target)
  composer.addPass(new RenderPass(scene, camera))
  const bloom = new UnrealBloomPass(new THREE.Vector2(window.innerWidth, window.innerHeight), 0.6, 0.55, 0.82)
  composer.addPass(bloom)
  composer.addPass(new OutputPass())

  const resize = () => {
    const width = window.innerWidth
    const height = window.innerHeight
    renderer.setSize(width, height)
    composer.setPixelRatio(renderer.getPixelRatio())
    composer.setSize(width, height)
    camera.aspect = width / height
    camera.updateProjectionMatrix()
    renderer.getDrawingBufferSize(size)
    resizeGround(ground, size.x, size.y)
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
    atmosphere.update(cue.tod, time)
    const tangent = Math.tan(THREE.MathUtils.degToRad(camera.fov) / 2)
    uniforms.uPixelAngle.value = (2 * tangent) / size.y
    uniforms.uPointScale.value = size.y / (2 * tangent)
    sky.update(time, camera)
    rain.update(camera)
    renderer.toneMappingExposure = atmosphere.grade.exposure * cue.fade
    bloom.strength = atmosphere.grade.bloom
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
