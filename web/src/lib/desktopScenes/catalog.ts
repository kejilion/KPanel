import type { MessageKey } from '@/i18n/messages/zh-CN'
import type { ThemeColorIntent } from '@/theme/colors'
import { DESKTOP_SCENE_IDS, isDesktopSceneID, type DesktopSceneID } from './loaders'

export { DESKTOP_SCENE_IDS, isDesktopSceneID, type DesktopSceneID }

export const DESKTOP_SCENE_KEY = 'kpanel:desktop-scene:v1'

export interface DesktopSceneDefinition {
  id: DesktopSceneID
  nameKey: MessageKey
  descriptionKey: MessageKey
  /** Scenes following the local clock are labelled so the changing light is expected. */
  timeAware: boolean
  colors: Readonly<ThemeColorIntent>
}

// The static poster for each scene lives in desktopWallpaper.css (`[data-desktop-scene]`),
// so the first paint, the picker preview and a failed download all share one look.
export const DESKTOP_SCENES: readonly DesktopSceneDefinition[] = [
  {
    id: 'daylight',
    nameKey: 'desktop.sceneDaylight',
    descriptionKey: 'desktop.sceneDaylightDescription',
    timeAware: true,
    colors: { brand: '#2f74c0', neutral: '#3e5468', signatureLinked: false, signature: '#d99a4e' },
  },
  {
    id: 'seaside',
    nameKey: 'desktop.sceneSeaside',
    descriptionKey: 'desktop.sceneSeasideDescription',
    timeAware: true,
    colors: { brand: '#15879c', neutral: '#3b5660', signatureLinked: false, signature: '#d8a560' },
  },
  {
    id: 'aurora',
    nameKey: 'desktop.sceneAurora',
    descriptionKey: 'desktop.sceneAuroraDescription',
    timeAware: false,
    colors: { brand: '#1f9a74', neutral: '#2d3f4c', signatureLinked: false, signature: '#8a68d6' },
  },
  {
    id: 'rain',
    nameKey: 'desktop.sceneRain',
    descriptionKey: 'desktop.sceneRainDescription',
    timeAware: false,
    colors: { brand: '#b8468a', neutral: '#3b3552', signatureLinked: false, signature: '#2fa9cc' },
  },
  {
    id: 'fireflies',
    nameKey: 'desktop.sceneFireflies',
    descriptionKey: 'desktop.sceneFirefliesDescription',
    timeAware: false,
    colors: { brand: '#4f8a3a', neutral: '#34443c', signatureLinked: false, signature: '#c9a93c' },
  },
]

export function findDesktopScene(id: DesktopSceneID): DesktopSceneDefinition {
  return DESKTOP_SCENES.find((scene) => scene.id === id)!
}

export function readDesktopSceneID(): DesktopSceneID | null {
  try {
    const stored = window.localStorage.getItem(DESKTOP_SCENE_KEY)
    return isDesktopSceneID(stored) ? stored : null
  } catch {
    return null
  }
}

export function persistDesktopSceneID(id: DesktopSceneID | null): void {
  try {
    if (id) window.localStorage.setItem(DESKTOP_SCENE_KEY, id)
    else window.localStorage.removeItem(DESKTOP_SCENE_KEY)
  } catch {
    // The scene still applies to this session when storage is unavailable.
  }
}
