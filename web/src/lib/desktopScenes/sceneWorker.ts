import { isDesktopSceneID } from './loaders'
import { createSceneLoop, type SceneLoop, type SceneLoopStatus } from './sceneLoop'
import type { SceneHostMessage } from './sceneHost'

// Renders a scene on an OffscreenCanvas so wallpaper frames never compete with
// terminal output, window drags or Vue updates on the main thread.

interface WorkerScope {
  onmessage: ((event: MessageEvent<SceneHostMessage>) => void) | null
  postMessage: (message: SceneLoopStatus) => void
  requestAnimationFrame?: (callback: (now: number) => void) => number
  cancelAnimationFrame?: (handle: number) => void
}

const scope = globalThis as unknown as WorkerScope
let loop: SceneLoop | undefined

function schedule(callback: (now: number) => void): () => void {
  if (scope.requestAnimationFrame && scope.cancelAnimationFrame) {
    const handle = scope.requestAnimationFrame(callback)
    return () => scope.cancelAnimationFrame!(handle)
  }
  const handle = setTimeout(() => callback(performance.now()), 16)
  return () => clearTimeout(handle)
}

scope.onmessage = (event) => {
  const message = event.data
  switch (message.type) {
    case 'init':
      if (loop || !isDesktopSceneID(message.sceneId)) return
      loop = createSceneLoop({
        sceneId: message.sceneId,
        surface: message.canvas,
        createSurface: (width, height) => new OffscreenCanvas(width, height),
        schedule,
        now: () => performance.now(),
        onStatus: (status) => scope.postMessage(status),
      })
      loop.setPaused(message.paused)
      loop.setReducedMotion(message.reducedMotion)
      loop.resize(message.width, message.height, message.pixelRatio)
      break
    case 'resize':
      loop?.resize(message.width, message.height, message.pixelRatio)
      break
    case 'paused':
      loop?.setPaused(message.value)
      break
    case 'reducedMotion':
      loop?.setReducedMotion(message.value)
      break
    case 'dispose':
      loop?.dispose()
      loop = undefined
      break
  }
}
