import * as THREE from 'three'
import { postToHost } from './bridge'

/**
 * Loading, as the desktop sees it. Every file fetched through three's loaders
 * counts towards the progress the desktop shows if the scene is slow to load;
 * the last stretch, compiling and the first frame, is reported with report().
 * Progress only goes forwards and in whole percents, so a burst of small files
 * is not a burst of messages.
 */
let reported = -1

export function report(value: number): void {
  const percent = Math.floor(Math.min(1, Math.max(0, value)) * 100)
  if (percent <= reported) return
  reported = percent
  postToHost({ source: 'kpanel-scene-pack', type: 'progress', value: percent / 100 })
}

// Files are nine tenths of the way; compiling and the first frame the rest.
THREE.DefaultLoadingManager.onProgress = (_url, loaded, total) => report((0.9 * loaded) / Math.max(total, 1))

/**
 * A picture as a texture, decoded off the main thread (and in a worker, which has no <img>).
 * flipY: stored bottom row first, as three's textures are by default (a glTF's maps are not).
 */
export async function loadTexture(path: string, flipY = true): Promise<THREE.Texture> {
  if (typeof createImageBitmap === 'undefined') {
    const texture = await new THREE.TextureLoader().loadAsync(path)
    texture.flipY = flipY
    return texture
  }
  const bitmap = await new THREE.ImageBitmapLoader()
    .setOptions({ imageOrientation: flipY ? 'flipY' : 'from-image', premultiplyAlpha: 'none', colorSpaceConversion: 'none' })
    .loadAsync(path)
  const texture = new THREE.Texture(bitmap)
  // Flipped (or not) while decoding: a bitmap cannot be flipped on upload.
  texture.flipY = false
  texture.needsUpdate = true
  return texture
}

/** A canvas to draw a texture on, wherever the scene runs. */
export function drawingCanvas(width: number, height: number): HTMLCanvasElement | OffscreenCanvas {
  if (typeof document === 'undefined') return new OffscreenCanvas(width, height)
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  return canvas
}
