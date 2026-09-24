import * as THREE from 'three'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js'
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js'
import { ShaderPass } from 'three/examples/jsm/postprocessing/ShaderPass.js'
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js'
import { onHostCommand, postToHost } from './bridge'
import { createDaylight, localHour, moonAge } from './daylight'
import { Director, type Shot } from './director'
import { createGulls } from './gulls'
import { createLighthouse } from './lighthouse'
import { createOcean } from './ocean'
import { createRoad } from './road'
import { createLighting } from './shading'
import { createSkyDome, skyUniforms } from './sky'
import { createTerrain } from './terrain'
import { createTraffic } from './traffic'
import { groundHeight, LIGHTHOUSE, obstacleHeight, roadCurve } from './world'

/**
 * The tour stays on one stretch of coast north of the headland: above the road, on it heading for
 * the lighthouse, and down among the rocks below the cliffs, so every move is short.
 */
const REEF = new THREE.Vector3(-130, 8, 170)
/** Looking inland (a rising sun or moon), from further out, so the cliffs sit low in the frame. */
const REEF_OFFSHORE = new THREE.Vector3(-380, 12, 190)
const ROAD_START = 2770
const ROAD_SPEED = 4

/**
 * Coastal Road: a road winding along sea cliffs, a lighthouse on the headland,
 * sea stacks in a glittering sea, cars now and then and cyclists on the
 * shoulder. Day and night follow the local clock and the moon its real phase
 * (?hour=H and ?moon=days-since-new-moon override them for previews); the
 * camera tour does not change the time.
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
const random = mulberry32(20260926)
const hourParam = params.get('hour')
const hourOverride = hourParam !== null && hourParam !== '' && Number.isFinite(Number(hourParam)) ? ((Number(hourParam) % 24) + 24) % 24 : undefined
const moonParam = params.get('moon')
const moonOverride = moonParam !== null && moonParam !== '' && Number.isFinite(Number(moonParam)) ? Number(moonParam) : undefined

function createRenderer(): THREE.WebGLRenderer | undefined {
  try {
    const renderer = new THREE.WebGLRenderer({ antialias: false, powerPreference: 'high-performance' })
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5))
    renderer.toneMapping = THREE.ACESFilmicToneMapping
    renderer.outputColorSpace = THREE.SRGBColorSpace
    return renderer
  } catch {
    return undefined
  }
}

function start(): void {
  const renderer = createRenderer()
  if (!renderer) {
    postToHost({ source: 'kpanel-scene-pack', type: 'error', reason: 'webgl_unavailable' })
    return
  }
  document.body.appendChild(renderer.domElement)
  const daylight = createDaylight()
  daylight.update(localHour(new Date(), hourOverride), moonAge(new Date(), moonOverride))
  const lighting = createLighting(daylight)
  // One set of uniform objects for sky, land and sea, so a single update moves them all.
  const uniforms = { ...skyUniforms(daylight), ...lighting.uniforms }

  const scene = new THREE.Scene()
  const camera = new THREE.PerspectiveCamera(50, window.innerWidth / window.innerHeight, 1, 30000)
  scene.add(createSkyDome(uniforms))
  scene.add(createTerrain(uniforms, random))
  const ocean = createOcean(uniforms, random)
  scene.add(ocean.mesh)

  const road = roadCurve()
  const roadLength = road.getLength()
  const roadway = createRoad(uniforms, road)
  scene.add(roadway)
  const traffic = createTraffic(uniforms, road, random)
  scene.add(traffic.group)
  const lighthouse = createLighthouse(uniforms)
  scene.add(lighthouse)
  const gulls = createGulls(uniforms, random)
  scene.add(gulls)
  const along = new THREE.Vector3()
  const ahead = new THREE.Vector3()
  const up = new THREE.Vector3(0, 1, 0)
  const seaSide = new THREE.Vector3()
  const lift = new THREE.Vector3()
  const sky = { position: new THREE.Vector3(), heading: new THREE.Vector3(), pitch: 0 }
  /**
   * Frames the sun, or the moon (at night, or by day when the sun is too high to fit in the
   * frame), when one is low enough to sit above the sea; otherwise the open sea to the west.
   */
  const frameTheSky = () => {
    const sun = daylight.elevation > -8 && daylight.elevation < 30
    const moon = daylight.moonElevation > 3 && daylight.moonElevation < 38 && daylight.moonIllumination > 0.12
    const body = sun ? daylight.sunDir : moon ? daylight.moonDir : undefined
    if (body) sky.heading.set(body.x, 0, body.z).normalize()
    else sky.heading.set(-1, 0, 0.45).normalize()
    const elevation = body ? Math.asin(body.y) : 0
    sky.pitch = THREE.MathUtils.clamp(elevation - THREE.MathUtils.degToRad(11), THREE.MathUtils.degToRad(1.5), THREE.MathUtils.degToRad(19))
    sky.position.copy(sky.heading.x > 0.2 ? REEF_OFFSHORE : REEF)
  }
  let time = 0
  const shots: Shot[] = [
    // High over the road: the cliffs running south to the headland and its lighthouse, the stacks and the sea.
    {
      id: 'coast',
      position: new THREE.Vector3(60, groundHeight(60, 120) + 62, 120),
      target: new THREE.Vector3(LIGHTHOUSE.x - 40, 35, LIGHTHOUSE.z - 10),
      fov: 50, orbit: 0.035, float: 2,
    },
    // Low among the stacks, turned towards the sun or the moon.
    {
      id: 'reef', position: new THREE.Vector3(), target: new THREE.Vector3(), fov: 50, orbit: 0, float: 0,
      enter: frameTheSky,
      track(elapsed, position, target) {
        lift.set(sky.heading.z, 0, -sky.heading.x).multiplyScalar(Math.sin(elapsed * 0.05) * 8)
        position.copy(sky.position).add(lift)
        position.y += Math.sin(elapsed * 0.3) * 0.5
        target.copy(sky.heading).multiplyScalar(Math.cos(sky.pitch) * 500).add(position)
        target.y = position.y + Math.sin(sky.pitch) * 500
      },
    },
    // Down on the road heading for the lighthouse, the sea on the right, a cyclist ahead and a car coming.
    {
      id: 'road', position: new THREE.Vector3(), target: new THREE.Vector3(), fov: 52, orbit: 0, float: 0,
      enter: () => traffic.stage(time, ROAD_START + 12, ROAD_START + 350),
      track(elapsed, position, target) {
        const s = Math.min(roadLength - 90, ROAD_START + elapsed * ROAD_SPEED)
        road.getPointAt(s / roadLength, along)
        road.getPointAt(Math.min(1, (s + 70) / roadLength), ahead)
        const tangent = road.getTangentAt(s / roadLength)
        seaSide.crossVectors(tangent, up).normalize()
        position.copy(along).addScaledVector(seaSide, 2.2).add(new THREE.Vector3(0, 4.4, 0))
        target.copy(ahead).addScaledVector(seaSide, 14).add(new THREE.Vector3(0, 2, 0))
      },
    },
  ]

  const target = new THREE.WebGLRenderTarget(1, 1, { type: THREE.HalfFloatType, samples: 4 })
  const composer = new EffectComposer(renderer, target)
  composer.addPass(new RenderPass(scene, camera))
  const bloom = new UnrealBloomPass(new THREE.Vector2(window.innerWidth, window.innerHeight), 0.2, 0.35, 1.8)
  // Glints and lamps bloom; the bright sky and sea by day do not.
  composer.addPass(bloom)
  composer.addPass(new OutputPass())
  // A light grade after tone mapping: lifts the colour of muted tones (sea, grass, sunset sky) without pushing bright ones.
  composer.addPass(new ShaderPass({
    uniforms: { tDiffuse: { value: null }, uVibrance: { value: 0.28 } },
    vertexShader: /* glsl */ `
      varying vec2 vUv;
      void main() { vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }
    `,
    fragmentShader: /* glsl */ `
      uniform sampler2D tDiffuse;
      uniform float uVibrance;
      varying vec2 vUv;
      void main() {
        vec4 color = texture2D(tDiffuse, vUv);
        float luma = dot(color.rgb, vec3(0.2126, 0.7152, 0.0722));
        float saturation = max(color.r, max(color.g, color.b)) - min(color.r, min(color.g, color.b));
        color.rgb = mix(vec3(luma), color.rgb, 1.0 + uVibrance * (1.0 - saturation));
        gl_FragColor = vec4(clamp(color.rgb, 0.0, 1.0), color.a);
      }
    `,
  }))

  const size = new THREE.Vector2()
  const resize = () => {
    renderer.setSize(window.innerWidth, window.innerHeight)
    composer.setPixelRatio(renderer.getPixelRatio())
    composer.setSize(window.innerWidth, window.innerHeight)
    camera.aspect = window.innerWidth / window.innerHeight
    camera.updateProjectionMatrix()
    renderer.getDrawingBufferSize(size)
  }
  resize()
  window.addEventListener('resize', resize)

  const requestedShot = Number(params.get('shot'))
  const director = new Director(camera, shots, {
    entrance: params.get('entrance') !== 'off',
    ground: obstacleHeight,
    shot: Number.isInteger(requestedShot) && requestedShot >= 0 && requestedShot < shots.length ? requestedShot : 0,
  })
  director.onShotChange = (index) => postToHost({ source: 'kpanel-scene-pack', type: 'camera', index })

  let paused = false
  let handle = 0
  let last = 0
  const frame = (now: number) => {
    handle = requestAnimationFrame(frame)
    // rAF timestamps can precede the first directly rendered frame: never let time run backwards.
    const dt = last ? Math.min(Math.max((now - last) / 1000, 0), 0.05) : 1 / 60
    last = now
    time += dt
    daylight.update(localHour(new Date(), hourOverride), moonAge(new Date(), moonOverride))
    uniforms.uNight.value = daylight.night
    uniforms.uKeyVisible.value = daylight.keyVisible
    uniforms.uMoonLight.value = daylight.moonIllumination * THREE.MathUtils.smoothstep(daylight.moonElevation, -2, 8)
    uniforms.uTime.value = time
    const cue = director.update(dt, time)
    ocean.follow(camera)
    roadway.update(daylight.night)
    traffic.update(time, daylight.night)
    lighthouse.update(time, daylight.night)
    gulls.update(time, daylight.night)
    uniforms.uPointScale.value = size.y / (2 * Math.tan(THREE.MathUtils.degToRad(camera.fov) / 2))
    renderer.toneMappingExposure = daylight.exposure * cue.fade
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
  // Compile every shader and draw the first (black) frame before reporting ready.
  renderer.compile(scene, camera)
  frame(performance.now())
  postToHost({ source: 'kpanel-scene-pack', type: 'ready', cameras: shots.map((shot) => shot.id) })
}

start()
