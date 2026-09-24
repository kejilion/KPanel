/*!
 * Cloud palace original scene: Copyright (c) 2026 Codex. MIT License.
 * Bundled Three.js: Copyright (c) 2010-2026 three.js authors. MIT License.
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
 * of the Software, and to permit persons to whom the Software is furnished to do
 * so, subject to the following conditions: The above copyright notice and this
 * permission notice shall be included in all copies or substantial portions of
 * the Software. THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
 * EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
 * OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
 * ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
 * DEALINGS IN THE SOFTWARE.
 */
import * as T from 'three'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js'
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js'
import { ShaderPass } from 'three/examples/jsm/postprocessing/ShaderPass.js'
import { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js'
import { architecture, astrolabe } from './architecture'
import { atmosphere, waterfalls, lanterns, petals } from './atmosphere'
import { Director, cameraIds } from './director'
import { smooth } from './math'

function send(type: string, payload: Record<string, unknown> = {}): void {
  if (window.parent !== window) window.parent.postMessage({ source: 'kpanel-scene-pack', type, ...payload }, '*')
}

async function start(): Promise<void> {
  const options = new URLSearchParams(location.search)
  const entrance = options.get('entrance') !== 'off'
  const renderer = new T.WebGLRenderer({ antialias: true, alpha: false, powerPreference: 'high-performance' })
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5))
  renderer.setSize(innerWidth, innerHeight)
  renderer.toneMapping = T.ACESFilmicToneMapping; renderer.toneMappingExposure = .94
  renderer.setClearColor('#000000')
  document.body.appendChild(renderer.domElement)
  const scene = new T.Scene()
  scene.fog = new T.FogExp2('#45416b', .0048)
  const camera = new T.PerspectiveCamera(42, innerWidth / Math.max(1, innerHeight), .3, 1800)
  const director = new Director(camera, entrance, Number(options.get('shot') || 0), index => send('camera', { index }))
  scene.add(new T.HemisphereLight('#97aed5', '#352442', .85))
  const moonlight = new T.DirectionalLight('#bad9ff', 2.2); moonlight.position.set(-45, 70, -30); scene.add(moonlight)
  const sunset = new T.DirectionalLight('#ffd1a0', 1.25); sunset.position.set(35, 25, 60); scene.add(sunset)
  const fill = new T.DirectionalLight('#a88bdf', .7); fill.position.set(-35, 8, 40); scene.add(fill)
  architecture(scene)
  const halo = astrolabe(scene)
  const animated = [atmosphere(scene), waterfalls(scene), lanterns(scene), petals(scene)]
  const composer = new EffectComposer(renderer)
  composer.addPass(new RenderPass(scene, camera))
  composer.addPass(new UnrealBloomPass(new T.Vector2(innerWidth, innerHeight), .38, .65, .8))
  composer.addPass(new OutputPass())
  const finish = new ShaderPass({
    uniforms: { tDiffuse: { value: null }, fade: { value: 0 } },
    vertexShader: `varying vec2 uv0;void main(){uv0=uv;gl_Position=projectionMatrix*modelViewMatrix*vec4(position,1.);}`,
    fragmentShader: `uniform sampler2D tDiffuse;uniform float fade;varying vec2 uv0;
void main(){vec3 c=texture2D(tDiffuse,uv0).rgb;vec2 q=uv0-.5;float vignette=1.-.30*dot(q,q);c*=vignette;gl_FragColor=vec4(c*fade,1.);}`,
  }); composer.addPass(finish)

  let elapsed = 0, lastTime = 0, request = 0, ready = false, disposed = false, hostPaused = false
  const isPaused = () => hostPaused || document.hidden
  function draw(t: number): void {
    director.update(t); halo.rotation.z = Math.sin(t * .027) * .07
    animated.forEach(item => item.update(t))
    finish.uniforms.fade!.value = entrance ? smooth((t - .15) / 3.8) : 1
    composer.render()
  }
  function frame(now: number): void {
    request = 0
    if (disposed || isPaused()) return
    const delta = lastTime ? Math.max(0, Math.min((now - lastTime) / 1000, .06)) : 0
    lastTime = now; elapsed += delta; draw(elapsed)
    request = requestAnimationFrame(frame)
  }
  function sync(): void {
    if (request) { cancelAnimationFrame(request); request = 0 }
    lastTime = 0
    if (ready && !disposed && !isPaused()) request = requestAnimationFrame(frame)
  }
  window.addEventListener('message', event => {
    if (event.source !== window.parent) return
    const data = event.data
    if (!data || typeof data !== 'object' || data.source !== 'kpanel-desktop') return
    if (data.type === 'pause') { hostPaused = true; sync() }
    else if (data.type === 'resume') { hostPaused = false; sync() }
    else if (data.type === 'camera' && ready && !disposed) {
      if (data.index === undefined || (Number.isInteger(data.index) && data.index >= 0 && data.index < cameraIds.length)) director.switch(elapsed, data.index)
    }
  })
  document.addEventListener('visibilitychange', sync)
  window.addEventListener('resize', () => {
    camera.aspect = innerWidth / Math.max(innerHeight, 1); camera.updateProjectionMatrix()
    renderer.setPixelRatio(Math.min(devicePixelRatio || 1, 1.5)); renderer.setSize(innerWidth, innerHeight)
    composer.setPixelRatio(renderer.getPixelRatio()); composer.setSize(innerWidth, innerHeight)
    if (ready && !disposed) draw(elapsed)
  })
  renderer.domElement.addEventListener('webglcontextlost', event => {
    event.preventDefault(); disposed = true; sync(); send('error', { reason: 'webgl-context-lost' })
  })
  window.addEventListener('pagehide', () => {
    disposed = true; sync(); composer.dispose(); renderer.dispose()
  }, { once: true })
  director.update(0)
  await renderer.compileAsync(scene, camera)
  if (disposed) return
  draw(0) // Black first frame; ready is sent only after shader compilation and draw.
  ready = true; send('ready', { cameras: [...cameraIds] }); sync()
}

start().catch(error => {
  console.error('Celestial Palace:', error)
  send('error', { reason: 'webgl-initialization-failed' })
})
