import { computed, ref } from 'vue'

/**
 * Live scenes follow the system reduced-motion preference by default. A person
 * who chose a live wallpaper can opt back in for this browser; the choice sits
 * next to the wallpaper key and public/appearance-init.js reads it too.
 */
export const DESKTOP_SCENE_MOTION_KEY = 'kpanel:desktop-scene-motion:v1'
const ALWAYS = 'always'
const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

function readAlways(): boolean {
  try {
    return window.localStorage.getItem(DESKTOP_SCENE_MOTION_KEY) === ALWAYS
  } catch {
    return false
  }
}

function systemReduces(): boolean {
  return typeof window !== 'undefined'
    && typeof window.matchMedia === 'function'
    && window.matchMedia(REDUCED_MOTION_QUERY).matches
}

const motionAlways = ref(typeof window !== 'undefined' && readAlways())
const systemReducedMotion = ref(systemReduces())
let listening = false

function listen(): void {
  if (listening || typeof window === 'undefined') return
  listening = true
  if (typeof window.matchMedia === 'function') {
    window.matchMedia(REDUCED_MOTION_QUERY).addEventListener?.('change', (event) => {
      systemReducedMotion.value = event.matches
    })
  }
  window.addEventListener('storage', (event) => {
    if (event.key === DESKTOP_SCENE_MOTION_KEY || event.key === null) motionAlways.value = readAlways()
  })
}

export function useSceneMotionPreference() {
  listen()
  return {
    /** The system asks for less motion (before any in-app override). */
    systemReducedMotion,
    /** The person asked to play live scenes anyway in this browser. */
    motionAlways,
    /** Scenes render a still frame only when both say so. */
    reducedMotion: computed(() => systemReducedMotion.value && !motionAlways.value),
    setMotionAlways(value: boolean): void {
      motionAlways.value = value
      try {
        if (value) window.localStorage.setItem(DESKTOP_SCENE_MOTION_KEY, ALWAYS)
        else window.localStorage.removeItem(DESKTOP_SCENE_MOTION_KEY)
      } catch {
        // The choice still applies to this page when storage is unavailable.
      }
    },
  }
}

export function resetSceneMotionPreferenceForTest(): void {
  motionAlways.value = readAlways()
  systemReducedMotion.value = systemReduces()
}
