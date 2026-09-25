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
