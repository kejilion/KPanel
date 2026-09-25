import * as THREE from 'three'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js'
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js'
import { ShaderPass } from 'three/examples/jsm/postprocessing/ShaderPass.js'
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js'
import { onHostCommand, postToHost } from './bridge'
import { createClock } from './clock'
import { createClouds } from './clouds'
import { createDaylight } from './daylight'
import { Director } from './director'
import { createOcean } from './ocean'
import { createReflection } from './reflection'
import { report } from './loading'
import { run, type View } from './runtime'
import { createRocks, rockMaterial } from './rocks'
import { createLighting } from './shading'
import { createSkyDome, skyUniforms } from './sky'
import { createSkyline } from './skyline'
import { createTour } from './tour'
import { obstacleHeight } from './world'

/**
 * Sea and Sky: the open sea, a few sea stacks, a city far off on the eastern
 * shore, and the sky. A whole day passes in ten minutes, starting from the
 * local time (see clock.ts): the sun and moon cross the sky (the moon in its
 * real phase), the stars and the Milky Way turn through the night, and the
 * camera keeps turning towards whatever lights the scene, the sunrises,
 * sunsets, moonrises and moonsets first (see framing.ts).
 * ?hour=H and ?moon=days-since-new-moon set the starting time for previews,
 * and ?timelapse=off holds to the real clock (or to them).
 */
function createRenderer(view: View): THREE.WebGLRenderer | undefined {
  try {
    const renderer = new THREE.WebGLRenderer({ canvas: view.canvas, antialias: false, powerPreference: 'high-performance' })
    renderer.setPixelRatio(Math.min(view.pixelRatio, 1.5))
    renderer.toneMapping = THREE.ACESFilmicToneMapping
    renderer.outputColorSpace = THREE.SRGBColorSpace
    return renderer
  } catch {
    return undefined
  }
}

async function start(view: View): Promise<void> {
  const renderer = createRenderer(view)
  if (!renderer) {
    view.unavailable()
    return
  }
  const { params } = view
  const hourParam = params.get('hour')
  const hourOverride = hourParam !== null && hourParam !== '' && Number.isFinite(Number(hourParam)) ? ((Number(hourParam) % 24) + 24) % 24 : undefined
  const moonParam = params.get('moon')
  const moonOverride = moonParam !== null && moonParam !== '' && Number.isFinite(Number(moonParam)) ? Number(moonParam) : undefined
  const timelapse = params.get('timelapse') !== 'off'
  const clock = createClock({ timelapse, hour: hourOverride, moon: moonOverride })
  const daylight = createDaylight()
  daylight.update(clock.time.hour, clock.time.moonAge, clock.time.day)
  const lighting = createLighting(daylight)
  // One set of uniform objects for the sky, the rocks and the sea, so a single update moves them all.
  const uniforms = { ...skyUniforms(daylight), ...lighting.uniforms }

  const scene = new THREE.Scene()
  const camera = new THREE.PerspectiveCamera(50, view.width / view.height, 1, 30000)
  report(0.02)
  // The downloads start first; everything that does not wait for them is built and its shaders
  // compiled while they arrive (without blocking the page, where the browser allows).
  const rocksLoading = createRocks(uniforms, renderer)
  const clouds = createClouds(renderer, uniforms)
  uniforms.uClouds.value = clouds.texture
  scene.add(createSkyDome(uniforms))
  scene.add(createSkyline(uniforms))
  const ocean = createOcean(uniforms, mulberry32(20260926))
  scene.add(ocean.mesh)
  const reflection = createReflection(renderer, scene, uniforms.uMirrorPass)
  uniforms.uReflection.value = reflection.texture
  uniforms.uReflectionMatrix.value = reflection.matrix
  // A stand-in stack, so the stacks' shader is compiled too before their files are in.
  const standIn = new THREE.Mesh(new THREE.PlaneGeometry(1, 1), rockMaterial(uniforms))
  scene.add(standIn)
  const compiling = Promise.all([renderer.compileAsync(scene, camera), clouds.compile()]).then(() => scene.remove(standIn))
  // The sculpted stacks and the clouds are in before the scene reports ready, so it fades up complete.
  const [rocks, volumes] = await Promise.all([rocksLoading, clouds.loaded, compiling])
  scene.add(rocks.object)
  uniforms.uWaterlines.value = rocks.waterlines
  uniforms.uShape.value = volumes.shape
  uniforms.uDetail.value = volumes.detail

  const tour = createTour(daylight)

  const target = new THREE.WebGLRenderTarget(1, 1, { type: THREE.HalfFloatType, samples: 4 })
  const composer = new EffectComposer(renderer, target)
  composer.addPass(new RenderPass(scene, camera))
  // Glints on the water and the moon bloom; the bright sky by day does not.
  composer.addPass(new UnrealBloomPass(new THREE.Vector2(view.width, view.height), 0.22, 0.4, 1.8))
  composer.addPass(new OutputPass())
  // A light grade after tone mapping: lifts muted colours (sea, dusk sky) without pushing bright
  // ones, and a soft vignette.
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
        vec2 edge = vUv - 0.5;
        color.rgb *= 1.0 - 0.28 * smoothstep(0.25, 0.85, dot(edge, edge) * 2.2);
        gl_FragColor = vec4(clamp(color.rgb, 0.0, 1.0), color.a);
      }
    `,
  }))

  // Quality steps for slower graphics cards: first coarser clouds, then fewer pixels overall.
  // Clouds are soft, so half resolution is plenty; they are the costliest thing in the scene.
  const QUALITY = [{ clouds: 0.5, pixels: 1.5 }, { clouds: 0.35, pixels: 1.5 }, { clouds: 0.35, pixels: 1 }]
  let quality = 0
  const drawingSize = new THREE.Vector2()
  const resize = () => {
    renderer.setPixelRatio(Math.min(view.pixelRatio, QUALITY[quality]!.pixels))
    renderer.setSize(view.width, view.height, false)
    renderer.getDrawingBufferSize(drawingSize)
    clouds.resize(drawingSize.x, drawingSize.y, QUALITY[quality]!.clouds)
    // The reflection is broken up by the waves anyway: half resolution is plenty.
    reflection.resize(drawingSize.x / 2, drawingSize.y / 2)
    uniforms.uScreen.value.copy(drawingSize)
    composer.setPixelRatio(renderer.getPixelRatio())
    composer.setSize(view.width, view.height)
    camera.aspect = view.width / view.height
    camera.updateProjectionMatrix()
  }
  resize()
  view.onResize(resize)

  tour.follow(0, true)
  const requestedShot = Number(params.get('shot'))
  const director = new Director(camera, tour.shots, {
    entrance: params.get('entrance') !== 'off',
    ground: obstacleHeight,
    shot: Number.isInteger(requestedShot) && requestedShot >= 0 && requestedShot < tour.shots.length ? requestedShot : 0,
  })
  director.onShotChange = (index) => postToHost({ source: 'kpanel-scene-pack', type: 'camera', index })

  // While the camera is all but still, the clouds are marched every other frame (they drift slowly,
  // and a frame's lag is a fraction of a pixel); during a move, every frame.
  let cloudFrame = 0
  const facing = new THREE.Vector3()
  const cloudFacing = new THREE.Vector3()
  const cloudPosition = new THREE.Vector3()
  // Frames that take too long, counted up while slow and down while fine; enough in a row and the
  // quality drops a step (never back up, so it cannot see-saw).
  let slow = 0
  let paused = false
  let handle = 0
  let last = 0
  let time = 0
  const frame = (now: number) => {
    handle = view.requestFrame(frame)
    // rAF timestamps can precede the first directly rendered frame: never let time run backwards.
    const elapsed = last ? Math.max((now - last) / 1000, 0) : 1 / 60
    const dt = Math.min(elapsed, 0.05)
    last = now
    time += dt
    // Linger over a moonrise or moonset as over a sunrise or sunset.
    const moonAtHorizon = daylight.moonIllumination > 0.08 ? Math.exp(-(((daylight.moonElevation - 4) / 7) ** 2)) : 0
    const moment = clock.advance(dt, 2 * moonAtHorizon)
    daylight.update(moment.hour, moment.moonAge, moment.day)
    tour.follow(dt)
    uniforms.uNight.value = daylight.night
    uniforms.uKeyVisible.value = daylight.keyVisible
    uniforms.uMoonLight.value = daylight.moonIllumination * THREE.MathUtils.smoothstep(daylight.moonElevation, -2, 8)
    uniforms.uTime.value = time
    // In the time-lapse the clouds race a little too; the sea keeps its own pace.
    uniforms.uCloudTime.value = time * (timelapse ? 5 : 1)
    const cue = director.update(dt, time)
    ocean.follow(camera)
    renderer.toneMappingExposure = daylight.exposure * cue.fade
    // The clouds are marched for this exact view before the sky that shows them is drawn.
    camera.updateMatrixWorld()
    camera.getWorldDirection(facing)
    const still = facing.angleTo(cloudFacing) < 0.004 && camera.position.distanceTo(cloudPosition) < 1
    if (!still || ++cloudFrame % 2 === 0) {
      clouds.render(camera)
      cloudFacing.copy(facing)
      cloudPosition.copy(camera.position)
    }
    reflection.render(camera)
    composer.render(dt)
    if (time > 6 && quality < QUALITY.length - 1) {
      slow = elapsed > 1 / 45 ? slow + 1 : Math.max(0, slow - 1)
      if (slow > 180) {
        quality++
        slow = 0
        resize()
      }
    }
  }

  onHostCommand((command) => {
    if (command.type === 'pause' && !paused) {
      paused = true
      view.cancelFrame(handle)
    } else if (command.type === 'resume' && paused) {
      paused = false
      last = 0
      handle = view.requestFrame(frame)
    } else if (command.type === 'camera') {
      director.cut(command.index)
    }
  })
  renderer.domElement.addEventListener('webglcontextlost', () => {
    view.cancelFrame(handle)
    postToHost({ source: 'kpanel-scene-pack', type: 'error', reason: 'webgl_context_lost' })
  })
  // Anything not compiled yet, then the first (black) frame, before reporting ready.
  report(0.92)
  await renderer.compileAsync(scene, camera)
  frame(performance.now())
  postToHost({ source: 'kpanel-scene-pack', type: 'ready', cameras: tour.shots.map((shot) => shot.id) })
}

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

run(start)
