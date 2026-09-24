import * as THREE from 'three'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js'
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js'
import { ShaderPass } from 'three/examples/jsm/postprocessing/ShaderPass.js'
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js'
import { onHostCommand, postToHost } from './bridge'
import { createClock } from './clock'
import { createDaylight } from './daylight'
import { Director, type Shot } from './director'
import { createOcean } from './ocean'
import { createRocks } from './rocks'
import { createLighting } from './shading'
import { createSkyDome, skyUniforms } from './sky'
import { obstacleHeight } from './world'

/**
 * Sea and Sky: nothing but the open sea, a few sea stacks and the sky. A whole
 * day passes in ten minutes, starting from the local time (see clock.ts): the
 * sun and moon cross the sky (the moon in its real phase), the stars and the
 * Milky Way turn through the night, and the camera keeps turning towards
 * whatever lights the scene: the sun and its glittering path, the moon and its
 * silver one, or on a moonless night the heart of the Milky Way.
 * ?hour=H and ?moon=days-since-new-moon set the starting time for previews,
 * and ?timelapse=off holds to the real clock (or to them).
 */
const params = new URLSearchParams(location.search)
const hourParam = params.get('hour')
const hourOverride = hourParam !== null && hourParam !== '' && Number.isFinite(Number(hourParam)) ? ((Number(hourParam) % 24) + 24) % 24 : undefined
const moonParam = params.get('moon')
const moonOverride = moonParam !== null && moonParam !== '' && Number.isFinite(Number(moonParam)) ? Number(moonParam) : undefined

/** The middle of the rock group, which every shot is framed around. */
const CENTRE = new THREE.Vector3(20, 0, 0)
/** The galactic pole and centre on the celestial sphere (equatorial unit vectors, as in the sky shader). */
const GALACTIC_POLE = new THREE.Vector3(-0.8677, -0.1981, 0.456)
const GALACTIC_CENTRE = new THREE.Vector3(-0.0549, -0.8734, -0.4839)

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
  const timelapse = params.get('timelapse') !== 'off'
  const clock = createClock({ timelapse, hour: hourOverride, moon: moonOverride })
  const daylight = createDaylight()
  daylight.update(clock.time.hour, clock.time.moonAge, clock.time.day)
  const lighting = createLighting(daylight)
  // One set of uniform objects for the sky, the rocks and the sea, so a single update moves them all.
  const uniforms = { ...skyUniforms(daylight), ...lighting.uniforms }

  const scene = new THREE.Scene()
  const camera = new THREE.PerspectiveCamera(50, window.innerWidth / window.innerHeight, 1, 30000)
  scene.add(createSkyDome(uniforms))
  scene.add(createRocks(uniforms))
  const ocean = createOcean(uniforms, mulberry32(20260926))
  scene.add(ocean.mesh)

  /**
   * Where the light is: the sun from dawn twilight to dusk twilight, then the moon while it is
   * up, and on a moonless night the Milky Way: its core if it is up, otherwise where the band
   * rises from the horizon on the side nearer the core. The camera follows it continuously and
   * smoothly, so as the day turns it pans after the sun or moon, and swings over gently when
   * the light passes from one to the next.
   */
  const aim = { heading: new THREE.Vector3(), elevation: 0, pitch: 0, fov: 50, yaw: 0 }
  const goal = { yaw: 0, elevation: 0 }
  const toLocal = new THREE.Matrix3()
  const galactic = new THREE.Vector3()
  const pole = new THREE.Vector3()
  const rising = new THREE.Vector3()
  const takeAim = () => {
    let body = daylight.sunDir
    if (daylight.elevation < -10) {
      toLocal.copy(daylight.celestial).transpose()
      galactic.copy(GALACTIC_CENTRE).applyMatrix3(toLocal)
      if (daylight.moonElevation > 2 && daylight.moonIllumination > 0.1) {
        body = daylight.moonDir
      } else if (galactic.y > 0.1) {
        body = galactic
      } else {
        pole.copy(GALACTIC_POLE).applyMatrix3(toLocal)
        rising.crossVectors(pole, new THREE.Vector3(0, 1, 0)).normalize()
        if (rising.dot(galactic) < 0) rising.negate()
        body = rising.setY(0.35).normalize()
      }
    }
    goal.yaw = Math.atan2(body.x, body.z)
    goal.elevation = Math.asin(THREE.MathUtils.clamp(body.y, -1, 1))
  }
  const followAim = (dt: number, snap = false) => {
    takeAim()
    // Eased, and no faster than 10 degrees a second, so the handover from sun to moon is a slow
    // swing rather than a whip round the stacks.
    const ease = snap ? 1 : 1 - Math.exp(-dt / 4)
    const limit = snap ? Math.PI : degrees(10) * dt
    const turn = Math.atan2(Math.sin(goal.yaw - aim.yaw), Math.cos(goal.yaw - aim.yaw))
    aim.yaw += THREE.MathUtils.clamp(turn * ease, -limit, limit)
    aim.elevation += THREE.MathUtils.clamp((goal.elevation - aim.elevation) * ease, -limit, limit)
    aim.heading.set(Math.sin(aim.yaw), 0, Math.cos(aim.yaw))
    // A low sun or moon sits in the upper part of a normal frame; a high one needs a wider lens
    // tilted up, so it still shares the frame with the horizon.
    const elevation = THREE.MathUtils.radToDeg(aim.elevation)
    aim.fov = THREE.MathUtils.clamp(elevation + 12, 50, 78)
    aim.pitch = degrees(elevation < 36 ? THREE.MathUtils.clamp(elevation - 11, 1.5, 19) : elevation / 2 - 1)
    light.fov = aim.fov
    sky.fov = Math.max(aim.fov, 60)
  }
  const degrees = THREE.MathUtils.degToRad
  const turned = (angle: number) => aim.heading.clone().applyAxisAngle(new THREE.Vector3(0, 1, 0), angle)
  const right = () => new THREE.Vector3(-aim.heading.z, 0, aim.heading.x)
  const look = (position: THREE.Vector3, direction: THREE.Vector3, pitch: number, target: THREE.Vector3) =>
    target.copy(direction).multiplyScalar(Math.cos(pitch) * 500).add(position).setY(position.y + Math.sin(pitch) * 500)

  // All three shots stand on the far side of the stacks from the light, a hundred-odd metres apart
  // and facing much the same way, so every move is a short drift, rise or turn.
  // Low over the water, facing the sun or moon and the path it lays on the sea; the stacks to one side.
  const light: Shot = {
    id: 'light', position: new THREE.Vector3(), target: new THREE.Vector3(), fov: 50, orbit: 0, float: 0,
    track(elapsed, position, target) {
      position.copy(CENTRE).addScaledVector(right(), 100 + Math.sin(elapsed * 0.05) * 10).addScaledVector(aim.heading, -140)
      position.y = 6 + Math.sin(elapsed * 0.35) * 0.4
      look(position, aim.heading, aim.pitch, target)
    },
  }
  // Higher and further back, with a wider lens: the stacks small, the long path of light and the whole sky.
  const sky: Shot = {
    id: 'sky', position: new THREE.Vector3(), target: new THREE.Vector3(), fov: 60, orbit: 0, float: 0,
    track(elapsed, position, target) {
      position.copy(CENTRE).addScaledVector(right(), -20 + Math.sin(elapsed * 0.04) * 12).addScaledVector(aim.heading, -270)
      position.y = 42 + Math.sin(elapsed * 0.2) * 1.2
      look(position, turned(degrees(-6)), Math.max(aim.pitch, degrees(5)), target)
    },
  }
  const shots: Shot[] = [
    light,
    // The stacks against the light, drifting slowly round them.
    {
      id: 'rocks', position: new THREE.Vector3(), target: new THREE.Vector3(), fov: 46, orbit: 0, float: 0,
      track(elapsed, position, target) {
        const direction = turned(degrees(24) + Math.sin(elapsed * 0.04) * 0.08)
        position.copy(CENTRE).addScaledVector(direction, -175)
        position.y = 5 + Math.sin(elapsed * 0.3) * 0.3
        look(position, direction, Math.min(aim.pitch, degrees(9)), target)
      },
    },
    sky,
  ]

  const target = new THREE.WebGLRenderTarget(1, 1, { type: THREE.HalfFloatType, samples: 4 })
  const composer = new EffectComposer(renderer, target)
  composer.addPass(new RenderPass(scene, camera))
  // Glints on the water and the moon bloom; the bright sky by day does not.
  composer.addPass(new UnrealBloomPass(new THREE.Vector2(window.innerWidth, window.innerHeight), 0.22, 0.4, 1.8))
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

  const resize = () => {
    renderer.setSize(window.innerWidth, window.innerHeight)
    composer.setPixelRatio(renderer.getPixelRatio())
    composer.setSize(window.innerWidth, window.innerHeight)
    camera.aspect = window.innerWidth / window.innerHeight
    camera.updateProjectionMatrix()
  }
  resize()
  window.addEventListener('resize', resize)

  followAim(0, true)
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
  let time = 0
  const frame = (now: number) => {
    handle = requestAnimationFrame(frame)
    // rAF timestamps can precede the first directly rendered frame: never let time run backwards.
    const dt = last ? Math.min(Math.max((now - last) / 1000, 0), 0.05) : 1 / 60
    last = now
    time += dt
    const moment = clock.advance(dt)
    daylight.update(moment.hour, moment.moonAge, moment.day)
    followAim(dt)
    uniforms.uNight.value = daylight.night
    uniforms.uKeyVisible.value = daylight.keyVisible
    uniforms.uMoonLight.value = daylight.moonIllumination * THREE.MathUtils.smoothstep(daylight.moonElevation, -2, 8)
    uniforms.uTime.value = time
    // In the time-lapse the clouds race a little too; the sea keeps its own pace.
    uniforms.uCloudTime.value = time * (timelapse ? 5 : 1)
    const cue = director.update(dt, time)
    ocean.follow(camera)
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

start()
