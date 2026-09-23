export type SceneSurface = OffscreenCanvas | HTMLCanvasElement
export type SceneContext2D = OffscreenCanvasRenderingContext2D | CanvasRenderingContext2D

export interface SceneFrame {
  /** Seconds since the scene started; only advances while the scene is animating. */
  time: number
  /** Seconds since the previous rendered frame, clamped for resumed tabs. */
  delta: number
  /** Local wall-clock hour in [0, 24), used by time-aware scenes. */
  hour: number
}

export interface SceneEnvironment {
  /** Creates an off-screen buffer for cached layers and sprites. */
  createSurface: (width: number, height: number) => SceneSurface
}

export interface SceneInstance {
  /** Canvas pixel size; the scene rebuilds size-dependent caches here. */
  resize: (width: number, height: number) => void
  render: (frame: SceneFrame) => void
}

export interface SceneModule {
  createScene: (context: SceneContext2D, environment: SceneEnvironment) => SceneInstance
}
