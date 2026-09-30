export type SceneSurface = OffscreenCanvas | HTMLCanvasElement

export interface ShaderParticles {
  /** Point count at full detail; lower quality levels draw proportionally fewer. */
  count: number
  /** GLSL ES 3.00 body with `void main()` writing `gl_Position`, `gl_PointSize` and its varyings. */
  vertex: string
  /** GLSL ES 3.00 body with `void main()` writing `fragColor`; points blend additively. */
  fragment: string
}

/**
 * A scene is data: GLSL bodies that the shared renderer compiles on top of one
 * prelude of uniforms, noise, sky and tone-mapping helpers.
 */
export interface ShaderScene {
  /** Defines `vec3 render(vec2 fragCoord)` returning linear light for one pixel. */
  fragment: string
  particles?: ShaderParticles
}

export interface SceneModule {
  scene: ShaderScene
}

export interface SceneFrame {
  /** Seconds since the scene started; only advances while the scene is animating. */
  time: number
  /** Local wall-clock hour in [0, 24), used by time-aware scenes. */
  hour: number
  /** Eased pointer position in [-1, 1] (x right, y down) for parallax. */
  pointer: readonly [number, number]
  /** Shader detail in (0, 1]; lower quality levels trade octaves and particles for speed. */
  detail: number
}

export interface SceneRenderer {
  resize: (width: number, height: number) => void
  render: (frame: SceneFrame) => void
  /** True while the GPU is still working on earlier frames, so the loop skips instead of queueing more. */
  backlogged: () => boolean
  dispose: () => void
}

/** The device cannot run the scene with hardware acceleration; the poster stays instead. */
export class SceneUnsupportedError extends Error {
  constructor(message = 'scene_gpu_unavailable') {
    super(message)
    this.name = 'SceneUnsupportedError'
  }
}
