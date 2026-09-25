import * as THREE from 'three'
import { DRACOLoader } from 'three/examples/jsm/loaders/DRACOLoader.js'
import { postToHost } from './bridge'

/**
 * Loading, as the desktop sees it. Every file fetched through three's loaders
 * (and any other download wrapped in track()) counts towards the progress the
 * desktop shows if the scene is slow to load; the last stretch, compiling and
 * the first frame, is reported with report(). Progress only goes forwards and
 * in whole percents, so a burst of small files is not a burst of messages.
 */
const manager = THREE.DefaultLoadingManager
let reported = -1

export function report(value: number): void {
  const percent = Math.floor(Math.min(1, Math.max(0, value)) * 100)
  if (percent <= reported) return
  reported = percent
  postToHost({ source: 'kpanel-scene-pack', type: 'progress', value: percent / 100 })
}

// Files are nine tenths of the way; compiling and the first frame the rest.
manager.onProgress = (_url, loaded, total) => report((0.9 * loaded) / Math.max(total, 1))

/** Counts a download made outside three's loaders (a plain fetch) towards the progress. */
export async function track<T>(url: string, work: Promise<T>): Promise<T> {
  manager.itemStart(url)
  try {
    return await work
  } finally {
    manager.itemEnd(url)
  }
}

let draco: DRACOLoader | undefined
/** The Draco mesh decoder, shipped in the pack (assets/draco/) and run in a worker. */
export function dracoLoader(): DRACOLoader {
  if (!draco) {
    draco = new DRACOLoader()
    draco.setDecoderPath({ js: 'assets/draco/draco_wasm_wrapper.js', wasm: 'assets/draco/draco_decoder.wasm' })
    draco.setWorkerLimit(2)
  }
  return draco
}
