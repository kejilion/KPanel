/*!
 * Abyssal Ruins: Copyright (c) 2026 Codex. MIT License.
 * Three.js: Copyright (c) 2010-2026 three.js authors. MIT License.
 * Full license notices are provided in the accompanying license.txt.
 */
import * as T from 'three'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js'
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js'
import { ShaderPass } from 'three/examples/jsm/postprocessing/ShaderPass.js'
import { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js'
import { createUnderwater } from './underwater'
import { Director, cameraIds } from './director'

declare const __ABYSS_GLB__: string

function send(type: string, payload: Record<string, unknown> = {}): void {
  if (window.parent !== window) window.parent.postMessage({ source: 'kpanel-scene-pack', type, ...payload }, '*')
}

function smooth(value: number): number {
  const t = T.MathUtils.clamp(value, 0, 1)
  return t * t * (3 - 2 * t)
}

function disposeScene(scene: T.Scene): void {
  const geometries = new Set<T.BufferGeometry>()
  const materials = new Set<T.Material>()
  const textures = new Set<T.Texture>()
  scene.traverse(object => {
    if (!(object instanceof T.Mesh)) return
    geometries.add(object.geometry)
    for (const material of Array.isArray(object.material) ? object.material : [object.material]) {
      materials.add(material)
      for (const value of Object.values(material)) if (value instanceof T.Texture) textures.add(value)
    }
  })
  geometries.forEach(geometry => geometry.dispose())
  materials.forEach(material => material.dispose())
  textures.forEach(texture => texture.dispose())
}

async function start(): Promise<void> {
  const options = new URLSearchParams(location.search)
  const entrance = options.get('entrance') !== 'off'
  const renderer = new T.WebGLRenderer({ antialias: true, alpha: false, powerPreference: 'high-performance' })
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5))
  renderer.setSize(innerWidth, innerHeight)
  renderer.toneMapping = T.ACESFilmicToneMapping
  renderer.toneMappingExposure = 1.4
  renderer.shadowMap.enabled = true
  renderer.shadowMap.type = T.PCFShadowMap
  renderer.setClearColor('#000000')
  document.body.appendChild(renderer.domElement)

  const scene = new T.Scene()
  scene.background = new T.Color('#104550')
  scene.fog = new T.FogExp2('#104550', .025)
  const camera = new T.PerspectiveCamera(45, innerWidth / Math.max(1, innerHeight), .08, 300)
  const director = new Director(camera, entrance, Number(options.get('shot') || 0), index => send('camera', { index }))
  scene.add(new T.HemisphereLight('#8dd7df', '#0d2630', .9))
  const sunlight = new T.DirectionalLight('#9cdce4', 4.5)
  sunlight.position.set(22, 34, -5)
  sunlight.target.position.set(-5, 0, -12)
  sunlight.castShadow = true
  sunlight.shadow.mapSize.set(4096, 4096)
  Object.assign(sunlight.shadow.camera, { left: -33, right: 33, top: 33, bottom: -33, near: .5, far: 100 })
  sunlight.shadow.bias = .00005
  sunlight.shadow.normalBias = .065
  scene.add(sunlight, sunlight.target)
  const fill = new T.DirectionalLight('#36718e', .75)
  fill.position.set(-8, 28, 6)
  scene.add(fill)
  const relicGlow = new T.PointLight('#e6bb75', 100, 17, 2)
  relicGlow.position.set(0, 7, -7)
  scene.add(relicGlow)

  const composer = new EffectComposer(renderer)
  composer.addPass(new RenderPass(scene, camera))
  composer.addPass(new UnrealBloomPass(new T.Vector2(innerWidth, innerHeight), .3, .65, .95))
  composer.addPass(new OutputPass())
  const finish = new ShaderPass({
    uniforms: { tDiffuse: { value: null }, fade: { value: 0 } },
    vertexShader: 'varying vec2 uv0;void main(){uv0=uv;gl_Position=projectionMatrix*modelViewMatrix*vec4(position,1.);}',
    fragmentShader: `uniform sampler2D tDiffuse;uniform float fade;varying vec2 uv0;
void main(){vec3 c=texture2D(tDiffuse,uv0).rgb;vec2 q=uv0-.5;c*=1.-.3*dot(q,q);gl_FragColor=vec4(c*fade,1.);}`,
  })
  composer.addPass(finish)

  let underwater: ReturnType<typeof createUnderwater> | undefined
  let seal: T.Object3D | undefined
  const sealPosition = new T.Vector3()
  const sealRotation = new T.Quaternion()
  const sealUp = new T.Vector3(0, 1, 0)
  const sway = new T.Quaternion()
  let elapsed = 0, lastTime = 0, request = 0, ready = false, disposed = false, hostPaused = false
  const isPaused = () => hostPaused || document.hidden
  function draw(t: number): void {
    director.update(t)
    underwater?.update(t, camera)
    if (seal) {
      seal.position.copy(sealPosition).addScaledVector(sealUp, Math.sin(t * .27) * .1)
      seal.quaternion.copy(sealRotation).premultiply(sway.setFromAxisAngle(sealUp, Math.sin(t * .19) * .006))
    }
    relicGlow.intensity = 100 + Math.sin(t * .38) * 4
    finish.uniforms.fade!.value = entrance ? smooth((t - .12) / 4.5) : 1
    composer.render()
  }
  function frame(now: number): void {
    request = 0
    if (disposed || isPaused()) return
    const delta = lastTime ? Math.max(0, Math.min((now - lastTime) / 1000, .06)) : 0
    lastTime = now
    elapsed += delta
    draw(elapsed)
    request = requestAnimationFrame(frame)
  }
  function sync(): void {
    if (request) { cancelAnimationFrame(request); request = 0 }
    lastTime = 0
    if (ready && !disposed && !isPaused()) request = requestAnimationFrame(frame)
  }
  function onMessage(event: MessageEvent): void {
    if (event.source !== window.parent) return
    const data = event.data
    if (!data || typeof data !== 'object' || data.source !== 'kpanel-desktop') return
    if (data.type === 'pause') { hostPaused = true; sync() }
    else if (data.type === 'resume') { hostPaused = false; sync() }
    else if (data.type === 'camera' && ready && !disposed) {
      if (data.index === undefined || (Number.isInteger(data.index) && data.index >= 0 && data.index < cameraIds.length)) director.switch(elapsed, data.index)
    }
  }
  function onResize(): void {
    camera.aspect = innerWidth / Math.max(innerHeight, 1)
    camera.updateProjectionMatrix()
    renderer.setPixelRatio(Math.min(devicePixelRatio || 1, 1.5))
    renderer.setSize(innerWidth, innerHeight)
    composer.setPixelRatio(renderer.getPixelRatio())
    composer.setSize(innerWidth, innerHeight)
    if (ready && !disposed) draw(elapsed)
  }
  function dispose(): void {
    if (disposed) return
    disposed = true
    sync()
    window.removeEventListener('message', onMessage)
    window.removeEventListener('resize', onResize)
    document.removeEventListener('visibilitychange', sync)
    underwater?.dispose()
    disposeScene(scene)
    sunlight.shadow.dispose()
    for (const pass of composer.passes) pass.dispose()
    composer.dispose()
    renderer.dispose()
  }
  window.addEventListener('message', onMessage)
  document.addEventListener('visibilitychange', sync)
  window.addEventListener('resize', onResize)
  renderer.domElement.addEventListener('webglcontextlost', event => {
    event.preventDefault()
    dispose()
    send('error', { reason: 'webgl-context-lost' })
  })
  window.addEventListener('pagehide', dispose, { once: true })

  try {
    const binary = atob(__ABYSS_GLB__)
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
    let textureFailed = false
    const manager = new T.LoadingManager()
    manager.onError = () => { textureFailed = true }
    const loader = new GLTFLoader(manager)
    loader.register(parser => {
      // ImageBitmapLoader fetches even embedded blob URLs. TextureLoader uses
      // an image element, keeping texture decode under img-src in the sandbox.
      parser.textureLoader = new T.TextureLoader(manager)
      return { name: 'KPanelEmbeddedTextureDecode' }
    })
    const gltf = await loader.parseAsync(bytes.buffer, '')
    // A page can close while embedded textures decode.
    if (disposed) { const unused = new T.Scene(); unused.add(gltf.scene); disposeScene(unused); return }
    const anisotropy = Math.min(8, renderer.capabilities.getMaxAnisotropy())
    gltf.scene.traverse(object => {
      if (object instanceof T.Mesh) {
        object.castShadow = true
        object.receiveShadow = true
        for (const material of Array.isArray(object.material) ? object.material : [object.material]) {
          for (const value of Object.values(material)) {
            if (value instanceof T.Texture) { value.anisotropy = anisotropy; value.needsUpdate = true }
          }
        }
      }
    })
    scene.add(gltf.scene)
    if (textureFailed) throw new Error('An embedded Blender texture could not be decoded')
    seal = gltf.scene.getObjectByName('Abyss_Seal')
    if (seal) {
      sealPosition.copy(seal.position)
      sealRotation.copy(seal.quaternion)
      // Preserve the imported pivot and express world-up in its parent's basis.
      if (seal.parent) sealUp.applyQuaternion(seal.parent.getWorldQuaternion(sway).invert()).normalize()
    }
    underwater = createUnderwater(scene, renderer)
    director.update(0)
    underwater.update(0, camera)
    await renderer.compileAsync(scene, camera)
    if (disposed) return
    draw(0)
    ready = true
    send('ready', { cameras: [...cameraIds] })
    sync()
  } catch (error) {
    dispose()
    throw error
  }
}

start().catch(error => {
  console.error('Abyssal Ruins:', error)
  send('error', { reason: 'webgl-initialization-failed' })
})
