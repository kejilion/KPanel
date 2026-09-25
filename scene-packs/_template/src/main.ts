/**
 * Starter scene pack: one full-screen WebGL2 shader, no libraries.
 *
 * It implements the whole host protocol (ready, camera, error; pause, resume,
 * camera) plus the three things every KPanel scene should have: an entrance,
 * a slow idle "breathing" loop, and eased transitions between camera shots.
 * Replace the shader and the shots with your own world — or throw this file
 * away and use any engine you like. See scene-packs/README.md.
 */
interface Shot { id: string, hue: number, zoom: number, drift: number }

// Keep ids in sync with manifest.json "cameras".
const SHOTS: readonly Shot[] = [
  { id: 'aurora', hue: 0.42, zoom: 1, drift: 0 },
  { id: 'dusk', hue: 0.9, zoom: 1.45, drift: 0.6 },
  { id: 'deep-night', hue: 0.66, zoom: 0.8, drift: -0.4 },
]
const ENTRANCE_SECONDS = 4
const HOLD_SECONDS = 30
const TRANSITION_SECONDS = 4
const MAX_PIXEL_RATIO = 1.5

const VERTEX = `#version 300 es
void main() {
  vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}`

const FRAGMENT = `#version 300 es
precision highp float;
uniform vec2 uResolution;
uniform float uTime;
uniform float uHue;
uniform float uZoom;
uniform float uDrift;
uniform float uReveal;
out vec4 color;

vec3 palette(float t) { return 0.5 + 0.5 * cos(6.28318 * (t + vec3(0.0, 0.33, 0.67))); }

void main() {
  vec2 uv = (gl_FragCoord.xy - 0.5 * uResolution) / uResolution.y * uZoom;
  uv.x += uDrift;
  float breath = 0.5 + 0.5 * sin(uTime * 0.35);
  float glow = 0.0;
  for (int i = 0; i < 5; i++) {
    float f = float(i);
    float wave = sin(uv.x * (1.3 + f * 0.55) + uTime * (0.1 + f * 0.04) + f * 1.7) * (0.14 + 0.04 * breath);
    glow += (0.006 + 0.004 * breath) / abs(uv.y - wave - 0.2 + f * 0.11);
  }
  vec3 sky = mix(vec3(0.01, 0.012, 0.03), palette(uHue) * 0.22, smoothstep(-0.7, 0.8, uv.y));
  vec3 col = sky + palette(uHue + uv.x * 0.06) * min(glow, 3.0) * 0.35;
  color = vec4(col * uReveal, 1.0);
}`

type HostCommand = { source: 'kpanel-desktop', type: 'pause' | 'resume' | 'camera', index?: number }

function post(event: { type: 'ready', cameras: string[] } | { type: 'camera', index: number } | { type: 'error', reason: string }): void {
  // The desktop runs the pack in an opaque-origin sandbox, so '*' is the only usable target.
  if (window.parent !== window) window.parent.postMessage({ source: 'kpanel-scene-pack', ...event }, '*')
}

const canvas = document.createElement('canvas')
document.body.append(canvas)
const gl = canvas.getContext('webgl2', { alpha: false, antialias: false, powerPreference: 'high-performance' })
if (!gl) {
  post({ type: 'error', reason: 'webgl2-unavailable' })
  throw new Error('WebGL2 is unavailable')
}

function compile(type: number, source: string): WebGLShader {
  const shader = gl!.createShader(type)!
  gl!.shaderSource(shader, source)
  gl!.compileShader(shader)
  if (!gl!.getShaderParameter(shader, gl!.COMPILE_STATUS)) throw new Error(gl!.getShaderInfoLog(shader) ?? 'shader error')
  return shader
}

const program = gl.createProgram()
gl.attachShader(program, compile(gl.VERTEX_SHADER, VERTEX))
gl.attachShader(program, compile(gl.FRAGMENT_SHADER, FRAGMENT))
gl.linkProgram(program)
gl.useProgram(program)
gl.bindVertexArray(gl.createVertexArray())
const uniform = (name: string) => gl.getUniformLocation(program, name)
const u = {
  resolution: uniform('uResolution'),
  time: uniform('uTime'),
  hue: uniform('uHue'),
  zoom: uniform('uZoom'),
  drift: uniform('uDrift'),
  reveal: uniform('uReveal'),
}

// ?entrance=off&shot=N skips the entrance and opens a shot: handy for capturing poster.webp.
const params = new URLSearchParams(location.search)
const entrance = params.get('entrance') !== 'off'
let shotIndex = Math.min(SHOTS.length - 1, Math.max(0, Number(params.get('shot')) || 0))
const view = { ...SHOTS[shotIndex]! }
let from = { ...view }
let to = SHOTS[shotIndex]!
let transitionStart = -Infinity
let lastCut = 0
let time = 0
let last = performance.now()
let frame = 0
let readySent = false

const clamp = (value: number) => Math.min(1, Math.max(0, value))
const lerp = (a: number, b: number, t: number) => a + (b - a) * t
const ease = (t: number) => (t < 0.5 ? 4 * t * t * t : 1 - (-2 * t + 2) ** 3 / 2)

function cut(index: number): void {
  shotIndex = ((index % SHOTS.length) + SHOTS.length) % SHOTS.length
  from = { ...view }
  to = SHOTS[shotIndex]!
  transitionStart = time
  lastCut = time
  post({ type: 'camera', index: shotIndex })
}

function resize(): void {
  const ratio = Math.min(window.devicePixelRatio || 1, MAX_PIXEL_RATIO)
  canvas.width = Math.round(window.innerWidth * ratio)
  canvas.height = Math.round(window.innerHeight * ratio)
  gl!.viewport(0, 0, canvas.width, canvas.height)
}

function render(now: number): void {
  frame = requestAnimationFrame(render)
  // rAF timestamps can precede performance.now() taken earlier: never let time run backwards.
  time += Math.min(0.1, Math.max(0, (now - last) / 1000))
  last = now
  if (time - lastCut > HOLD_SECONDS) cut(shotIndex + 1)
  const k = ease(clamp((time - transitionStart) / TRANSITION_SECONDS))
  view.hue = lerp(from.hue, to.hue, k)
  view.zoom = lerp(from.zoom, to.zoom, k)
  view.drift = lerp(from.drift, to.drift, k)
  // Entrance: fade in while pulling back from a close zoom.
  const arrival = entrance ? ease(clamp(time / ENTRANCE_SECONDS)) : 1
  gl!.uniform2f(u.resolution, canvas.width, canvas.height)
  gl!.uniform1f(u.time, time)
  gl!.uniform1f(u.hue, view.hue)
  gl!.uniform1f(u.zoom, view.zoom * lerp(0.35, 1, arrival))
  gl!.uniform1f(u.drift, view.drift)
  gl!.uniform1f(u.reveal, arrival)
  gl!.drawArrays(gl!.TRIANGLES, 0, 3)
  if (!readySent) {
    readySent = true
    post({ type: 'ready', cameras: SHOTS.map((shot) => shot.id) })
  }
}

window.addEventListener('message', (event) => {
  if (event.source !== window.parent) return
  const command = event.data as Partial<HostCommand> | null
  if (!command || command.source !== 'kpanel-desktop') return
  if (command.type === 'pause' && frame) {
    cancelAnimationFrame(frame)
    frame = 0
  } else if (command.type === 'resume' && !frame) {
    last = performance.now() // resume without a time jump
    frame = requestAnimationFrame(render)
  } else if (command.type === 'camera') {
    cut(Number.isInteger(command.index) ? command.index! : shotIndex + 1)
  }
})
canvas.addEventListener('webglcontextlost', (event) => {
  event.preventDefault()
  cancelAnimationFrame(frame)
  post({ type: 'error', reason: 'context-lost' })
})
window.addEventListener('resize', resize)
resize()
frame = requestAnimationFrame(render)
