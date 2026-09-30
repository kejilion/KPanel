import { fragmentSource, particleFragmentSource, particleVertexSource, VERTEX_FULLSCREEN } from './glsl'
import { skyAt, type Vec3 } from './sky'
import { SceneUnsupportedError, type SceneFrame, type SceneRenderer, type SceneSurface, type ShaderScene } from './types'

// One WebGL2 context per scene: a full-screen fragment pass, then an optional
// additive point pass. Nothing is uploaded per frame except a few uniforms.

const CONTEXT_ATTRIBUTES: WebGLContextAttributes = {
  alpha: false,
  antialias: false,
  depth: false,
  stencil: false,
  premultipliedAlpha: false,
  preserveDrawingBuffer: false,
  // A backdrop belongs on the integrated GPU, and never on a software rasterizer.
  powerPreference: 'low-power',
  failIfMajorPerformanceCaveat: true,
}

const FRAMES_IN_FLIGHT = 2
const COMPILE_POLL_MS = 16
const COMPILE_TIMEOUT_MS = 20_000

interface Program {
  program: WebGLProgram
  uniforms: Map<string, WebGLUniformLocation | null>
}

interface Programs {
  main: Program
  particles?: Program
}

interface ParallelCompile {
  COMPLETION_STATUS_KHR: number
}

function compileShader(gl: WebGL2RenderingContext, type: number, source: string): WebGLShader {
  const shader = gl.createShader(type)
  if (!shader) throw new Error('scene_shader_unavailable')
  gl.shaderSource(shader, source)
  gl.compileShader(shader)
  return shader
}

function startProgram(gl: WebGL2RenderingContext, vertex: string, fragment: string): WebGLProgram {
  const program = gl.createProgram()
  if (!program) throw new Error('scene_program_unavailable')
  const shaders = [compileShader(gl, gl.VERTEX_SHADER, vertex), compileShader(gl, gl.FRAGMENT_SHADER, fragment)]
  shaders.forEach((shader) => gl.attachShader(program, shader))
  gl.linkProgram(program)
  return program
}

function finishProgram(gl: WebGL2RenderingContext, program: WebGLProgram): Program {
  const shaders = gl.getAttachedShaders(program) ?? []
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    const log = shaders.map((shader) => gl.getShaderInfoLog(shader)).filter(Boolean).join('\n') || gl.getProgramInfoLog(program)
    throw new Error(`scene_shader_failed: ${log ?? ''}`)
  }
  shaders.forEach((shader) => {
    gl.detachShader(program, shader)
    gl.deleteShader(shader)
  })
  return { program, uniforms: new Map() }
}

function wait(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

// With KHR_parallel_shader_compile the driver compiles off-thread; poll instead of blocking.
async function settle(gl: WebGL2RenderingContext, programs: WebGLProgram[]): Promise<void> {
  const parallel = gl.getExtension('KHR_parallel_shader_compile') as ParallelCompile | null
  if (!parallel) return
  const started = Date.now()
  while (!programs.every((program) => gl.getProgramParameter(program, parallel.COMPLETION_STATUS_KHR))) {
    if (gl.isContextLost()) return
    if (Date.now() - started > COMPILE_TIMEOUT_MS) throw new Error('scene_shader_timeout')
    await wait(COMPILE_POLL_MS)
  }
}

async function buildPrograms(gl: WebGL2RenderingContext, scene: ShaderScene): Promise<Programs> {
  const main = startProgram(gl, VERTEX_FULLSCREEN, fragmentSource(scene.fragment))
  const particles = scene.particles
    ? startProgram(gl, particleVertexSource(scene.particles.vertex), particleFragmentSource(scene.particles.fragment))
    : undefined
  await settle(gl, particles ? [main, particles] : [main])
  return { main: finishProgram(gl, main), particles: particles ? finishProgram(gl, particles) : undefined }
}

export async function createShaderRenderer(surface: SceneSurface, scene: ShaderScene): Promise<SceneRenderer> {
  const gl = surface.getContext('webgl2', CONTEXT_ATTRIBUTES) as WebGL2RenderingContext | null
  if (!gl) throw new SceneUnsupportedError()

  let programs: Programs | undefined = await buildPrograms(gl, scene)
  let vao = gl.createVertexArray()
  let width = surface.width
  let height = surface.height
  let lost = false
  let disposed = false
  const fences: WebGLSync[] = []

  function onLost(event: Event): void {
    event.preventDefault()
    lost = true
    programs = undefined
    fences.length = 0
  }

  async function onRestored(): Promise<void> {
    try {
      const rebuilt = await buildPrograms(gl!, scene)
      if (disposed) return
      programs = rebuilt
      vao = gl!.createVertexArray()
      lost = false
    } catch {
      // Stay blank over the poster; the next page load starts from scratch.
    }
  }

  surface.addEventListener('webglcontextlost', onLost)
  surface.addEventListener('webglcontextrestored', onRestored)

  function uniform(program: Program, name: string): WebGLUniformLocation | null {
    if (!program.uniforms.has(name)) program.uniforms.set(name, gl!.getUniformLocation(program.program, name))
    return program.uniforms.get(name)!
  }

  function setUniforms(program: Program, frame: SceneFrame): void {
    const sky = skyAt(frame.hour)
    const set3 = (name: string, value: Vec3) => gl!.uniform3f(uniform(program, name), value[0], value[1], value[2])
    gl!.useProgram(program.program)
    gl!.uniform2f(uniform(program, 'uResolution'), width, height)
    gl!.uniform1f(uniform(program, 'uTime'), frame.time)
    gl!.uniform1f(uniform(program, 'uHour'), frame.hour)
    gl!.uniform2f(uniform(program, 'uPointer'), frame.pointer[0], frame.pointer[1])
    gl!.uniform1f(uniform(program, 'uDetail'), frame.detail)
    set3('uZenith', sky.zenith)
    set3('uHorizon', sky.horizon)
    set3('uGlow', sky.glow)
    set3('uSunColor', sky.sunColor)
    set3('uMoonColor', sky.moonColor)
    set3('uSunDir', sky.sunDir)
    set3('uMoonDir', sky.moonDir)
    gl!.uniform1f(uniform(program, 'uLight'), sky.light)
    gl!.uniform1f(uniform(program, 'uStars'), sky.stars)
  }

  return {
    resize(nextWidth, nextHeight) {
      width = nextWidth
      height = nextHeight
    },
    render(frame) {
      if (lost || disposed || !programs || gl.isContextLost()) return
      gl.viewport(0, 0, width, height)
      gl.bindVertexArray(vao)
      gl.disable(gl.BLEND)
      setUniforms(programs.main, frame)
      gl.drawArrays(gl.TRIANGLES, 0, 3)
      if (programs.particles && scene.particles) {
        gl.enable(gl.BLEND)
        gl.blendFunc(gl.ONE, gl.ONE)
        setUniforms(programs.particles, frame)
        gl.drawArrays(gl.POINTS, 0, Math.max(1, Math.round(scene.particles.count * (0.35 + 0.65 * frame.detail))))
      }
      const fence = gl.fenceSync(gl.SYNC_GPU_COMMANDS_COMPLETE, 0)
      if (fence) fences.push(fence)
      gl.flush()
      while (fences.length > FRAMES_IN_FLIGHT + 1) gl.deleteSync(fences.shift()!)
    },
    backlogged() {
      if (lost || disposed) return false
      while (fences.length > 0 && gl.getSyncParameter(fences[0]!, gl.SYNC_STATUS) === gl.SIGNALED) {
        gl.deleteSync(fences.shift()!)
      }
      return fences.length >= FRAMES_IN_FLIGHT
    },
    dispose() {
      if (disposed) return
      disposed = true
      surface.removeEventListener('webglcontextlost', onLost)
      surface.removeEventListener('webglcontextrestored', onRestored)
      if (!gl.isContextLost()) {
        fences.forEach((fence) => gl.deleteSync(fence))
        if (programs) {
          gl.deleteProgram(programs.main.program)
          if (programs.particles) gl.deleteProgram(programs.particles.program)
        }
        gl.deleteVertexArray(vao)
        // Release GPU memory now rather than whenever the canvas is collected.
        gl.getExtension('WEBGL_lose_context')?.loseContext()
      }
      fences.length = 0
      programs = undefined
    },
  }
}
